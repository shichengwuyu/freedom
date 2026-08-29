package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

const storyboardTaskPollInterval = 3 * time.Second   // worker 拉取间隔
const storyboardTaskFinishedRetention = 30 * time.Minute // 已完成任务保留时长（分镜结果需留久些供前端恢复）
const storyboardTaskCleanupInterval = 10 * time.Minute
// storyboardMaxAttempts 与 handler/storyboard_task.go executeStoryboardTask 内嵌的 maxAttempts 对齐：
// 每章最多 2 次尝试（首调 + 1 次重试），预扣费按"章节数 × 2"封顶。
const storyboardMaxAttempts = 2

// PR-8: 服务端生成的主键，参见 CreateStoryboardTask 注释。
func newStoryboardTaskID() string {
	return "storyboard-task-" + uuid.NewString()
}

// storyboardTaskMaxConcurrent 限制同时运行的分镜任务 goroutine 数量，防止突发负载下
// goroutine 无限增长导致内存耗尽或上游 LLM API 被并发冲垮。
const storyboardTaskMaxConcurrent = 10

// StoryboardTaskCreateInput 创建分镜任务入参
type StoryboardTaskCreateInput struct {
	ClientTaskID    string
	UserID          string
	UserDisplayName string
	SourceID        string // 前端 NovelProject.id
	Model           string
	ChannelID       string
	UserChannelID   string
	ShotDuration    int
	ScriptPrompt    string
	Chapters        string // JSON [{title,content}]
	Assets          string // JSON [{alias,type,description,name}]，可为空
}

// StoryboardTaskExecFunc 执行单个分镜任务：循环调文本模型，每章完成调 onProgress 落库实时进度。
// 由 handler 注入（负责选渠道 + 构造 prompt + 调模型），service 只管调度与状态。
type StoryboardTaskExecFunc func(task model.StoryboardTask, onProgress func(doneCount int, result string) error) error

// ErrStoryboardCanceled onProgress 在检测到 status="canceled" 时返回的哨兵错误。
// 执行器（handler）遇到此错误应原样上抛，worker 据此不覆盖 canceled 状态。
var ErrStoryboardCanceled = errors.New("storyboard task canceled")

var (
	storyboardRunnerOnce  sync.Once
	storyboardWake        = make(chan struct{}, 1)
	storyboardRunningMu   sync.Mutex
	storyboardRunning     bool
	storyboardWakePending bool
	storyboardExecer      StoryboardTaskExecFunc
	storyboardExecerMu    sync.RWMutex
)

// CreateStoryboardTask 创建任务并落库（状态 queued），随后唤醒 worker
// 2026-08-27 改造：分镜文本模型之前绕过了余额扣费，登录用户可无限消耗付费 LLM。
// 这里按章节数 + maxAttempts 系数预估扣费，写入 HoldID，worker 完事按状态机 settle/cancel。
func CreateStoryboardTask(input StoryboardTaskCreateInput) (model.StoryboardTask, error) {
	if strings.TrimSpace(input.Chapters) == "" {
		return model.StoryboardTask{}, errors.New("缺少章节内容")
	}
	if strings.TrimSpace(input.Model) == "" {
		return model.StoryboardTask{}, errors.New("缺少文本模型")
	}
	current := now()
	task := model.StoryboardTask{
		// PR-8：主键始终服务端生成，禁止把客户端可控的 ClientTaskID 当作 id。
		// ClientTaskID 字段仍保留作为前端去重 / 幂等查询的辅助键。
		ID:              newStoryboardTaskID(),
		UserID:          strings.TrimSpace(input.UserID),
		UserDisplayName: strings.TrimSpace(input.UserDisplayName),
		Model:           strings.TrimSpace(input.Model),
		ChannelID:       strings.TrimSpace(input.ChannelID),
		UserChannelID:   strings.TrimSpace(input.UserChannelID),
		Source:          "novel-workbench",
		SourceID:        strings.TrimSpace(input.SourceID),
		Status:          "queued",
		ShotDuration:    input.ShotDuration,
		ScriptPrompt:    input.ScriptPrompt,
		Chapters:        input.Chapters,
		Assets:          input.Assets,
		Result:          "[]",
		CreatedAt:       current,
		UpdatedAt:       current,
	}
	// 解析章节数算 TotalCount，供扣费估算与进度计算
	var chapters []struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(input.Chapters), &chapters); err == nil {
		task.TotalCount = len(chapters)
	}
	// 预扣余额：每次文本模型调用的单价 × 章节数 × maxAttempts 系数（覆盖每章最多 2 次尝试）。
	// taskID 当作 requestID：与 ConsumeUserBalanceWithHold 的幂等键语义对齐（PR-3 收紧后，
	// 同一 taskID 复用时要求 amount/model/path 完全一致；不同 taskID 走新 hold）。
	preChargeCents, modelErr := estimateStoryboardCost(task.Model, task.TotalCount)
	if modelErr != nil {
		return model.StoryboardTask{}, modelErr
	}
	if preChargeCents > 0 {
		holdID, err := ConsumeUserBalanceWithHold(task.UserID, task.Model, preChargeCents, "/chat/completions", task.ID)
		if err != nil {
			return model.StoryboardTask{}, err
		}
		task.HoldID = holdID
	}
	saved, err := repository.SaveStoryboardTask(task)
	if err == nil {
		WakeStoryboardTaskRunner()
	}
	return saved, err
}

// estimateStoryboardCost 按章节数 + maxAttempts 计算预估扣费；模型单价为 0 视为免费。
func estimateStoryboardCost(modelName string, totalCount int) (int, error) {
	if totalCount <= 0 {
		return 0, nil
	}
	cost, err := ModelCost(modelName)
	if err != nil {
		return 0, err
	}
	cents := cost.CostCents
	if cents <= 0 {
		return 0, nil
	}
	return cents * totalCount * storyboardMaxAttempts, nil
}

func GetUserStoryboardTask(userID string, id string) (model.StoryboardTask, bool, error) {
	return repository.GetUserStoryboardTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func ListUserStoryboardTasks(userID string, limit int) ([]map[string]any, error) {
	tasks, err := repository.ListUserStoryboardTasks(strings.TrimSpace(userID), limit)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, StoryboardTaskResponse(task))
	}
	return result, nil
}

func DeleteUserStoryboardTask(userID string, id string) error {
	return repository.DeleteUserStoryboardTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

// StoryboardTaskResponse 任务对外响应（前端轮询拿 result 与 progress）
func StoryboardTaskResponse(task model.StoryboardTask) map[string]any {
	return map[string]any{
		"id":          task.ID,
		"source":      task.Source,
		"sourceId":    task.SourceID,
		"model":       task.Model,
		"status":      task.Status,
		"progress":    task.Progress,
		"doneCount":   task.DoneCount,
		"totalCount":  task.TotalCount,
		"shotDuration": task.ShotDuration,
		"result":      task.Result,
		"error":       task.Error,
		"createdAt":   task.CreatedAt,
		"updatedAt":   task.UpdatedAt,
		"startedAt":   task.StartedAt,
		"completedAt": task.CompletedAt,
	}
}

// StartStoryboardTaskRunner 启动后台 worker，exec 由 handler 注入
func StartStoryboardTaskRunner(exec StoryboardTaskExecFunc) {
	if exec == nil {
		return
	}
	storyboardExecerMu.Lock()
	storyboardExecer = exec
	storyboardExecerMu.Unlock()
	storyboardRunnerOnce.Do(func() {
		go runStoryboardTaskRunner()
	})
	WakeStoryboardTaskRunner()
}

// WakeStoryboardTaskRunner 唤醒 worker 立即拉取待执行任务
// 修复：去掉 default 分支，改为阻塞发送（channel buffer=1 不会死锁），
// 避免并发时序下丢弃唤醒信号导致 worker 永远阻塞在 for range storyboardWake 上。
func WakeStoryboardTaskRunner() {
	storyboardRunningMu.Lock()
	if storyboardRunning {
		storyboardWakePending = true
		storyboardRunningMu.Unlock()
		return
	}
	storyboardRunning = true
	storyboardWakePending = false
	storyboardRunningMu.Unlock()
	storyboardWake <- struct{}{}
}

func currentStoryboardExecer() StoryboardTaskExecFunc {
	storyboardExecerMu.RLock()
	defer storyboardExecerMu.RUnlock()
	return storyboardExecer
}

// runStoryboardTaskRunner 事件驱动 + 定时拉取，并发执行多个任务，sync.Map 防重入
func runStoryboardTaskRunner() {
	inFlight := sync.Map{}
	lastCleanupAt := time.Time{}
	sem := make(chan struct{}, storyboardTaskMaxConcurrent)
	for range storyboardWake {
		for {
			current := time.Now()
			tasks, err := repository.ListDueStoryboardTasks(50)
			if err != nil {
				log.Printf("list due storyboard tasks failed err=%v", err)
				time.Sleep(storyboardTaskPollInterval)
				continue
			}
			if len(tasks) == 0 {
				storyboardRunningMu.Lock()
				if storyboardWakePending {
					storyboardWakePending = false
					storyboardRunningMu.Unlock()
					continue
				}
				storyboardRunning = false
				storyboardRunningMu.Unlock()
				break
			}
			if lastCleanupAt.IsZero() || current.Sub(lastCleanupAt) >= storyboardTaskCleanupInterval {
				if err := repository.DeleteFinishedStoryboardTasksBefore(videoTaskTime(current.Add(-storyboardTaskFinishedRetention))); err != nil {
					log.Printf("cleanup finished storyboard tasks failed err=%v", err)
				}
				lastCleanupAt = current
			}
			for _, task := range tasks {
				if _, loaded := inFlight.LoadOrStore(task.ID, true); loaded {
					continue
				}
				// 用户中途取消：worker 跳过；cancel 接口已标记 status=canceled，无需再次落库。
				if task.Status == "canceled" {
					inFlight.Delete(task.ID)
					continue
				}
				// 并发限制：达到上限时跳过本轮（下轮再拉），避免 goroutine 无限增长。
				select {
				case sem <- struct{}{}:
				default:
					inFlight.Delete(task.ID)
					continue
				}
SafeGo("storyboard-task-exec:"+task.ID, func(r any) {
					// panic 兜底：标记 task failed + 清理 inFlight + 退款（2026-08-27 加固）
					log.Printf("storyboard task worker panic task=%s panic=%v", task.ID, r)
					task.Status = "failed"
					task.Error = fmt.Sprintf("worker panic: %v", r)
					task.UpdatedAt = now()
					task.CompletedAt = now()
					_, _ = repository.UpdateStoryboardTask(task)
					if task.HoldID != "" {
						if err := CancelBalanceHold(task.HoldID); err != nil {
							log.Printf("storyboard panic cancel hold failed id=%s holdID=%s err=%v", task.ID, task.HoldID, err)
						}
					}
					inFlight.Delete(task.ID)
				}, func() {
					defer inFlight.Delete(task.ID)
					defer func() { <-sem }()
					exec := currentStoryboardExecer()
					if exec == nil {
						// exec 未注入：等同于失败，全额退款。
						if task.HoldID != "" {
							_ = CancelBalanceHold(task.HoldID)
						}
						return
					}
					// 再次确认：进入 worker 前再读一次 DB 状态，避免用户在我们拿到的快照后点了取消。
					if latest, f, ferr := repository.GetUserStoryboardTask(task.UserID, task.ID); ferr == nil && f {
						if latest.Status == "canceled" {
							// 用户在我们启动 worker 前已经取消 → 退全款。
							if latest.HoldID != "" {
								_ = CancelBalanceHold(latest.HoldID)
							}
							return
						}
						// 同步刷新 HoldID：避免 SaveStoryboardTask 后 holdID 写入被其他流程回滚时拿到陈旧值。
						task = latest
					}
					// 标记 running
					task.Status = "running"
					task.StartedAt = now()
					task.UpdatedAt = task.StartedAt
					if _, saveErr := repository.UpdateStoryboardTask(task); saveErr != nil {
						log.Printf("save storyboard task running state failed id=%s err=%v", task.ID, saveErr)
					}
					// onProgress：每章完成落库实时进度，前端轮询可见
					onProgress := func(doneCount int, result string) error {
						// 章节回调：每次先重读一次状态，中途取消时立刻停手（不写新章节、保留已有分镜）
						if latest, f, ferr := repository.GetUserStoryboardTask(task.UserID, task.ID); ferr == nil && f && latest.Status == "canceled" {
							return ErrStoryboardCanceled
						}
						task.DoneCount = doneCount
						task.Result = result
						if task.TotalCount > 0 {
							task.Progress = clampProgress(doneCount * 100 / task.TotalCount)
						}
						task.Status = "running"
						task.UpdatedAt = now()
						_, err := repository.UpdateStoryboardTask(task)
						return err
					}
					execErr := exec(task, onProgress)
					// 已被取消的 task 不覆盖 canceled 状态（执行器已基于 canceled error 提前返回），退款。
					if errors.Is(execErr, ErrStoryboardCanceled) {
						if task.HoldID != "" {
							if err := CancelBalanceHold(task.HoldID); err != nil {
								log.Printf("storyboard canceled cancel hold failed id=%s holdID=%s err=%v", task.ID, task.HoldID, err)
							}
						}
						return
					}
					finish := now()
					task.UpdatedAt = finish
					task.CompletedAt = finish
					if execErr != nil {
						task.Status = "failed"
						task.Error = execErr.Error()
					} else {
						task.Status = "completed"
						task.Progress = 100
						task.DoneCount = task.TotalCount
					}
					if _, err := repository.UpdateStoryboardTask(task); err != nil {
						log.Printf("save storyboard task failed id=%s err=%v", task.ID, err)
					}
					// hold 闭环：成功 → settle（标记结算，不再退款）；失败 → cancel（全额退款）。
					// 即使 SaveStoryboardTask 落库失败也要继续做 hold 操作，否则用户余额被扣但看不到结果。
					if task.HoldID != "" {
						if execErr != nil {
							if err := CancelBalanceHold(task.HoldID); err != nil {
								log.Printf("storyboard failed cancel hold failed id=%s holdID=%s err=%v", task.ID, task.HoldID, err)
							}
						} else {
							if err := SettleBalanceHold(task.HoldID); err != nil {
								log.Printf("storyboard settle hold failed id=%s holdID=%s err=%v", task.ID, task.HoldID, err)
							}
						}
					}
				})
			}
			time.Sleep(storyboardTaskPollInterval)
		}
	}
}


// CallChatCompletion 调用 OpenAI 兼容的 /chat/completions，返回 assistant 文本。
// 复用渠道 URL 构造与 HTTP client，供分镜任务 worker（及未来文本类任务）使用。
func CallChatCompletion(channel model.ModelChannel, modelName, systemPrompt, userContent string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"stream": false,
	})
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, "/chat/completions"), strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return "", errors.New("文本模型无响应或网络不可达")
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("文本模型返回状态 %d：%s", response.StatusCode, string(responseBody))
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(responseBody, &payload)
	if len(payload.Choices) > 0 && strings.TrimSpace(payload.Choices[0].Message.Content) != "" {
		return payload.Choices[0].Message.Content, nil
	}
	return "", errors.New("文本模型返回空内容")
}
