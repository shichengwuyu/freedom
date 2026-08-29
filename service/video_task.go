package service

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/google/uuid"
)

const (
	videoTaskPollInitialInterval = 5 * time.Second  // 启动期：每 5 秒拉一次，覆盖前 1 分钟内的快轮询
	videoTaskPollMaxBackoff       = 30 * time.Second // 长视频最多 30 秒拉一次，节流 + 兼容上游限流
	videoTaskPollMaxIdle         = 24 * time.Hour    // 超过 24h 仍未终态视为超时（worker 标 failed，参考真实 video API 行为）
	videoTaskFinishedRetention   = 10 * time.Minute
	videoTaskCleanupInterval     = 10 * time.Minute
	// videoTaskMaxConcurrent 限制同时运行的轮询 goroutine 数量，防止突发负载下
	// goroutine 无限增长导致内存耗尽或上游 API 被并发冲垮。
	videoTaskMaxConcurrent = 50
)

var (
	videoTaskPollerOnce  sync.Once
	videoTaskPollWake    = make(chan struct{}, 1)
	videoTaskPollerMu    sync.RWMutex
	videoTaskPoller      VideoTaskPollFunc
	videoTaskRunningMu   sync.Mutex
	videoTaskRunning     bool
	videoTaskWakePending bool
)

type VideoTaskCreateInput struct {
	UserID          string
	UserDisplayName string
	Model           string
	ChannelID       string
	UserChannelID   string
	ChannelName     string
	VendorType      string // 非空=供应商视频任务（如 libtv），空=官方渠道
	Source          string
	SourceID        string
	ClientTaskID    string
	UpstreamTaskID  string
	UpstreamVideoID string
	Status          string
	Progress        int
	Seconds         string
	Size            string
	VideoURL        string
	Error           string
	ErrorDetail     string
	RequestBody     string
	ResponseBody    string
	CostCents int
	HoldID    string
}

type VideoTaskPollUpdate struct {
	Status       string
	Progress     int
	Seconds      string
	Size         string
	VideoURL     string
	Error        string
	ErrorDetail  string
	ResponseBody string
}

type VideoTaskPollFunc func(model.VideoTask) (VideoTaskPollUpdate, error)

func CreateVideoTask(input VideoTaskCreateInput) (model.VideoTask, error) {
	current := now()
	status := NormalizeVideoTaskStatus(input.Status)
	if status == "" {
		status = "queued"
	}
	task := model.VideoTask{
		ID:              firstVideoTaskValue(input.ClientTaskID, input.UpstreamTaskID, input.UpstreamVideoID, "video-task-"+uuid.NewString()),
		UserID:          strings.TrimSpace(input.UserID),
		UserDisplayName: strings.TrimSpace(input.UserDisplayName),
		Model:           strings.TrimSpace(input.Model),
		ChannelID:       strings.TrimSpace(input.ChannelID),
		UserChannelID:   strings.TrimSpace(input.UserChannelID),
		ChannelName:     strings.TrimSpace(input.ChannelName),
		VendorType:      strings.TrimSpace(input.VendorType),
		Source:          normalizeVideoTaskSource(input.Source),
		SourceID:        strings.TrimSpace(input.SourceID),
		UpstreamTaskID:  strings.TrimSpace(input.UpstreamTaskID),
		UpstreamVideoID: strings.TrimSpace(input.UpstreamVideoID),
		Status:          status,
		Progress:        clampProgress(input.Progress),
		Seconds:         strings.TrimSpace(input.Seconds),
		Size:            strings.TrimSpace(input.Size),
		VideoURL:        strings.TrimSpace(input.VideoURL),
		Error:           strings.TrimSpace(input.Error),
		ErrorDetail:     strings.TrimSpace(input.ErrorDetail),
		RequestBody:     input.RequestBody,
		ResponseBody:    input.ResponseBody,
		LastResponse:    input.ResponseBody,
		CostCents:         input.CostCents,
		HoldID:            strings.TrimSpace(input.HoldID),
		CreatedAt:       current,
		UpdatedAt:       current,
	}
	if IsCompletedVideoTaskStatus(task.Status) || task.VideoURL != "" {
		task.Status = "completed"
		task.Progress = 100
		task.CompletedAt = current
	} else if IsFailedVideoTaskStatus(task.Status) || task.Error != "" {
		task.Status = "failed"
		task.CompletedAt = current
	}
	saved, err := repository.SaveVideoTask(task)
	if err == nil && !IsCompletedVideoTaskStatus(saved.Status) && !IsFailedVideoTaskStatus(saved.Status) {
		WakeVideoTaskPoller()
	}
	return saved, err
}

func GetUserVideoTask(userID string, id string) (model.VideoTask, bool, error) {
	return repository.GetUserVideoTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func ListUserVideoTasks(userID string, source string, limit int) ([]map[string]any, error) {
	tasks, err := repository.ListUserVideoTasks(strings.TrimSpace(userID), normalizeVideoTaskSource(source), limit)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, VideoTaskResponse(task))
	}
	return result, nil
}

func DeleteUserVideoTask(userID string, id string) error {
	return repository.DeleteUserVideoTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func VideoTaskResponse(task model.VideoTask) map[string]any {
	result := map[string]any{
		"id":           task.ID,
		"object":       "video",
		"model":        task.Model,
		"channelId":    task.ChannelID,
		"userChannelId": task.UserChannelID,
		"channelName":  task.ChannelName,
		"vendorType":   task.VendorType,
		"source":       task.Source,
		"source_id":    task.SourceID,
		"status":       task.Status,
		"progress":     task.Progress,
		"task_id":      firstVideoTaskValue(task.UpstreamTaskID, task.ID),
		"video_id":     task.UpstreamVideoID,
		"seconds":      task.Seconds,
		"size":         task.Size,
		"created_at":   task.CreatedAt,
		"updated_at":   task.UpdatedAt,
		"started_at":   task.StartedAt,
		"completed_at": task.CompletedAt,
		"createdAt":    task.CreatedAt,
		"updatedAt":    task.UpdatedAt,
		"request_body": task.RequestBody,
	}
	if task.VideoURL != "" {
		result["url"] = task.VideoURL
		result["video_url"] = task.VideoURL
		result["data"] = []map[string]any{{"url": task.VideoURL}}
	}
	if IsFailedVideoTaskStatus(task.Status) && (task.Error != "" || task.ErrorDetail != "") {
		result["error"] = map[string]any{"message": firstVideoTaskValue(task.Error, task.ErrorDetail)}
		result["error_detail"] = task.ErrorDetail
	}
	return result
}

func StartVideoTaskPoller(poll VideoTaskPollFunc) {
	if poll == nil {
		return
	}
	videoTaskPollerMu.Lock()
	videoTaskPoller = poll
	videoTaskPollerMu.Unlock()
	videoTaskPollerOnce.Do(func() {
		go runVideoTaskPoller()
	})
	WakeVideoTaskPoller()
}

func WakeVideoTaskPoller() {
	videoTaskRunningMu.Lock()
	if videoTaskRunning {
		videoTaskWakePending = true
		videoTaskRunningMu.Unlock()
		return
	}
	videoTaskRunning = true
	videoTaskWakePending = false
	videoTaskRunningMu.Unlock()
	select {
	case videoTaskPollWake <- struct{}{}:
	default:
		videoTaskRunningMu.Lock()
		videoTaskRunning = false
		videoTaskRunningMu.Unlock()
	}
}

func runVideoTaskPoller() {
	inFlight := sync.Map{}
	lastCleanupAt := time.Time{}
	var nextWakeAt time.Time
	sem := make(chan struct{}, videoTaskMaxConcurrent)
	for range videoTaskPollWake {
		for {
			current := time.Now()
			if !nextWakeAt.IsZero() && current.Before(nextWakeAt) {
				time.Sleep(nextWakeAt.Sub(current))
			}
			current = time.Now()
			tasks, err := repository.ListDueVideoTasks(200)
			if err != nil {
				log.Printf("list due video tasks failed err=%v", err)
				nextWakeAt = time.Now().Add(videoTaskPollInitialInterval)
				continue
			}
			if len(tasks) == 0 {
				videoTaskRunningMu.Lock()
				if videoTaskWakePending {
					videoTaskWakePending = false
					videoTaskRunningMu.Unlock()
					continue
				}
				videoTaskRunning = false
				nextWakeAt = time.Time{}
				videoTaskRunningMu.Unlock()
				break
			}
			if lastCleanupAt.IsZero() || current.Sub(lastCleanupAt) >= videoTaskCleanupInterval {
				if err := repository.DeleteFinishedVideoTasksBefore(videoTaskTime(current.Add(-videoTaskFinishedRetention))); err != nil {
					log.Printf("cleanup finished video tasks failed err=%v", err)
				}
				lastCleanupAt = current
			}
			// 启发式：根据当前在跑的 task 数决定下次唤醒间隔——任务越多拉得越频繁（保持 5s），
			// 任务越少间隔缓慢拉长（上限 30s）以节流上游。任意 progress 推进都触发 worker 唤醒重置间隔。
			for _, task := range tasks {
				if _, loaded := inFlight.LoadOrStore(task.ID, true); loaded {
					continue
				}
				capturedTask := task
				// 并发限制：达到上限时跳过本轮（下轮再拉），避免 goroutine 无限增长。
				select {
				case sem <- struct{}{}:
				default:
					inFlight.Delete(capturedTask.ID)
					continue
				}
				SafeGo("video-task-poller:"+capturedTask.ID, func(r any) {
					// panic 兜底：标记 task failed + 清理 inFlight，避免任务永远卡 "processing"
					// 注意：sem 的释放只由 fn 内部的 defer 负责，onPanic 不重复释放，避免信号量泄漏。
					log.Printf("video task worker panic task=%s panic=%v", capturedTask.ID, r)
					_ = UpdateVideoTaskFromPoll(capturedTask, VideoTaskPollUpdate{
						Status:      "failed",
						ErrorDetail: fmt.Sprintf("worker panic: %v", r),
					})
					inFlight.Delete(capturedTask.ID)
					WakeVideoTaskPoller()
				}, func() {
					defer inFlight.Delete(capturedTask.ID)
					defer func() { <-sem }()
					poll := currentVideoTaskPoller()
					if poll == nil {
						return
					}
					update, err := poll(capturedTask)
					if err != nil {
						update = VideoTaskPollUpdate{Status: capturedTask.Status, ErrorDetail: err.Error()}
					}
					if err := UpdateVideoTaskFromPoll(capturedTask, update); err != nil {
						log.Printf("update video task failed id=%s err=%v", capturedTask.ID, err)
						return
					}
					// 进度/终态变化：立刻唤醒主循环（用最低间隔）保证前端最快可见。
					if update.Progress != capturedTask.Progress || IsCompletedVideoTaskStatus(update.Status) || IsFailedVideoTaskStatus(update.Status) {
						WakeVideoTaskPoller()
					}
				})
			}
			// 长视频超过 24h：标超时失败，避免 worker 永远轮询僵尸任务。
			// 仅处理不在 inFlight 中的任务（已在轮询的由 worker 自行管理），避免双重退款竞态。
			for _, t := range tasks {
				if IsCompletedVideoTaskStatus(t.Status) || IsFailedVideoTaskStatus(t.Status) {
					continue
				}
				if _, inFlight := inFlight.Load(t.ID); inFlight {
					continue
				}
				if nowToTime(t.CreatedAt).Add(videoTaskPollMaxIdle).Before(current) {
					if err := UpdateVideoTaskFromPoll(t, VideoTaskPollUpdate{
						Status:      "failed",
						Error:       "视频生成超时（已超过 24 小时未完成）",
						ErrorDetail: "poller timeout: >24h without terminal status",
					}); err != nil {
						log.Printf("force fail video task timeout id=%s err=%v", t.ID, err)
					}
				}
			}
			// 自适应间隔：>=20 个在跑 = 5s；否则按 5s × (20 / n) 拉长，上限 30s。
			interval := videoTaskPollInitialInterval
			if n := len(tasks); n > 0 && n < 20 {
				interval = videoTaskPollInitialInterval * time.Duration(20/n)
				if interval > videoTaskPollMaxBackoff {
					interval = videoTaskPollMaxBackoff
				}
			}
			nextWakeAt = time.Now().Add(interval)
		}
	}
}

// nowToTime 把 RFC3339Nano 字符串转回 time.Time；解析失败回退到零值让 24h 超时不被误触发。
func nowToTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

func currentVideoTaskPoller() VideoTaskPollFunc {
	videoTaskPollerMu.RLock()
	defer videoTaskPollerMu.RUnlock()
	return videoTaskPoller
}

func UpdateVideoTaskFromPoll(task model.VideoTask, update VideoTaskPollUpdate) error {
	current := now()
	// 状态转换检测：原状态非 failed、现在变 failed → 触发现有"已扣费但任务失败"退款。
	// 同一 task 状态转换只有一次（既已 failed 就不会再次变成 failed），单点退款幂等。
	prevFailed := IsFailedVideoTaskStatus(task.Status)
	task.Status = NormalizeVideoTaskStatus(firstVideoTaskValue(update.Status, task.Status))
	if task.Status == "" {
		task.Status = "processing"
	}
	if update.Progress > 0 || task.Progress == 0 {
		task.Progress = clampProgress(update.Progress)
	}
	if strings.TrimSpace(update.Seconds) != "" {
		task.Seconds = strings.TrimSpace(update.Seconds)
	}
	if strings.TrimSpace(update.Size) != "" {
		task.Size = strings.TrimSpace(update.Size)
	}
	if strings.TrimSpace(update.VideoURL) != "" {
		task.VideoURL = strings.TrimSpace(update.VideoURL)
	}
	if strings.TrimSpace(update.Error) != "" {
		task.Error = strings.TrimSpace(update.Error)
	}
	if strings.TrimSpace(update.ErrorDetail) != "" {
		task.ErrorDetail = strings.TrimSpace(update.ErrorDetail)
	}
	if update.ResponseBody != "" {
		task.LastResponse = update.ResponseBody
	}
	task.UpdatedAt = current
	task.LastPolledAt = videoTaskTime(time.Now())
	if task.VideoURL != "" || IsCompletedVideoTaskStatus(task.Status) {
		task.Status = "completed"
		task.Progress = 100
		task.CompletedAt = current
		task.Error = ""
		task.ErrorDetail = ""
	} else if task.Error != "" || IsFailedVideoTaskStatus(task.Status) {
		task.Status = "failed"
		task.CompletedAt = current
	}
	_, err := repository.SaveVideoTask(task)
	if err != nil {
		return err
	}
	// 状态转换（非 failed → failed）后触发退款。RefundFailedVideoTask 内部走事务 + 写退款
	// 流水，错误仅 log（不影响 task 状态）。
	if !prevFailed && IsFailedVideoTaskStatus(task.Status) && task.CostCents > 0 {
		_ = RefundFailedVideoTask(task)
	}
	return nil
}

func NormalizeVideoTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done", "succeeded", "success":
		return "completed"
	case "failed", "fail", "error", "cancelled", "canceled":
		return "failed"
	case "running", "processing", "in_progress", "in-progress":
		return "processing"
	case "queued", "queue", "pending", "":
		return "queued"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func IsCompletedVideoTaskStatus(status string) bool {
	return NormalizeVideoTaskStatus(status) == "completed"
}

func IsFailedVideoTaskStatus(status string) bool {
	return NormalizeVideoTaskStatus(status) == "failed"
}

func videoTaskTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func firstVideoTaskValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeVideoTaskSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "canvas":
		return "canvas"
	case "video-workbench", "":
		return "video-workbench"
	default:
		return "video-workbench"
	}
}

func clampProgress(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
