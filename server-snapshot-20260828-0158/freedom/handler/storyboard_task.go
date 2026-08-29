package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/tigerowo/freedom/service"
)

// storyboardTaskCreateRequest 创建分镜任务的前端请求体
type storyboardTaskCreateRequest struct {
	ClientTaskID string `json:"clientTaskId"` // 可选，前端预生成的 id，用于幂等
	SourceID     string `json:"sourceId"`     // 前端 NovelProject.id
	Model        string `json:"model"`
	ChannelID    string `json:"channelId"`    // X-Model-Channel-ID
	UserChannelID string `json:"userChannelId"`
	ShotDuration int    `json:"shotDuration"`
	ScriptPrompt string `json:"scriptPrompt"`
	Chapters     string `json:"chapters"` // JSON 字符串 [{title,content}]
	Assets       string `json:"assets"`   // JSON 字符串 [{alias,type,description,name}]
}

// CreateStoryboardTaskHandler POST /api/v1/storyboard-tasks 创建分镜生成任务
func CreateStoryboardTaskHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var req storyboardTaskCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, "请求参数解析失败")
		return
	}
	task, err := service.CreateStoryboardTask(service.StoryboardTaskCreateInput{
		ClientTaskID:  strings.TrimSpace(req.ClientTaskID),
		UserID:        user.ID,
		UserDisplayName: user.DisplayName,
		SourceID:      strings.TrimSpace(req.SourceID),
		Model:         strings.TrimSpace(req.Model),
		ChannelID:     strings.TrimSpace(req.ChannelID),
		UserChannelID: strings.TrimSpace(req.UserChannelID),
		ShotDuration:  req.ShotDuration,
		ScriptPrompt:  req.ScriptPrompt,
		Chapters:      req.Chapters,
		Assets:        req.Assets,
	})
	if err != nil {
		if errors.Is(err, repository.ErrStoryboardTaskConflict) {
			// 幂等性：同一 ClientTaskID 已处于 terminal 状态，前端不应复用。
			FailWithStatus(w, http.StatusConflict, "该分镜任务已结束，请重新创建（clientTaskId 已被占用）")
			return
		}
		log.Printf("create storyboard task failed user=%s err=%v", user.ID, err)
		FailError(w, err)
		return
	}
	// 创建后唤醒后台 worker 立即拉取执行
	service.WakeStoryboardTaskRunner()
	OK(w, service.StoryboardTaskResponse(task))
}

// GetStoryboardTaskHandler GET /api/v1/storyboard-tasks/:id 查询单个任务（轮询用）
func GetStoryboardTaskHandler(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		Fail(w, "分镜任务不存在")
		return
	}
	task, found, err := service.GetUserStoryboardTask(user.ID, id)
	if err != nil {
		log.Printf("get storyboard task failed user=%s id=%s err=%v", user.ID, id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	if !found {
		Fail(w, "分镜任务不存在")
		return
	}
	OK(w, service.StoryboardTaskResponse(task))
}

// ListUserStoryboardTasksHandler GET /api/v1/storyboard-tasks 列出当前用户的分镜任务
func ListUserStoryboardTasksHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	tasks, err := service.ListUserStoryboardTasks(user.ID, 100)
	if err != nil {
		log.Printf("list storyboard tasks failed user=%s err=%v", user.ID, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, tasks)
}

// DeleteUserStoryboardTaskHandler DELETE /api/v1/storyboard-tasks/:id 删除任务
func DeleteUserStoryboardTaskHandler(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		Fail(w, "分镜任务不存在")
		return
	}
	if err := service.DeleteUserStoryboardTask(user.ID, id); err != nil {
		log.Printf("delete storyboard task failed user=%s id=%s err=%v", user.ID, id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, map[string]any{"deleted": true})
}

// CancelStoryboardTaskHandler POST /api/v1/storyboard-tasks/:id/cancel
// 把任务标记为 canceled：worker 下一轮检测到 status=="canceled" 会跳过；已完成的章节保留 result。
// 前端点"停止"时可调用此端点，避免再来回重置客户端 state。
func CancelStoryboardTaskHandler(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		Fail(w, "分镜任务不存在")
		return
	}
	task, found, err := service.GetUserStoryboardTask(user.ID, id)
	if err != nil {
		log.Printf("cancel storyboard task read failed user=%s id=%s err=%v", user.ID, id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	if !found {
		Fail(w, "分镜任务不存在")
		return
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "canceled" {
		// 已结束的任务直接返回当前状态，幂等且不抛错
		OK(w, service.StoryboardTaskResponse(task))
		return
	}
	task.Status = "canceled"
	task.Error = "用户手动取消"
	task.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.UpdatedAt = task.CompletedAt
	if _, err := repository.UpdateStoryboardTask(task); err != nil {
		log.Printf("cancel storyboard task save failed id=%s err=%v", id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, service.StoryboardTaskResponse(task))
}

// StartStoryboardTaskRunner 启动分镜任务后台 worker。
// 由 main.go 调用，注入 executeStoryboardTask 作为执行函数。
func StartStoryboardTaskRunner() {
	service.StartStoryboardTaskRunner(executeStoryboardTask)
}

// storyboardChapterInput 章节输入（与前端 Chapter 结构对齐）
// ShotCount：P1 b1/b2 — 这章希望拆成几个分镜（默认 1；>=2 时后端 prompt 让模型输出 ShotCount 条用 ===SHOT=== 分隔）
type storyboardChapterInput struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	ShotCount int    `json:"shotCount,omitempty"` // 0/缺省视为 1
}

// storyboardAssetInput 资产输入（与前端 Asset 结构对齐）
type storyboardAssetInput struct {
	Alias       string `json:"alias"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Name        string `json:"name"`
}

// storyboardResultEntry 单条分镜结果（与前端 Storyboard 对齐，前端据 groupIndex/shotIndex 重建列表）
//
// status 取代了老的 "⚠" 占位前缀：
//   - status="completed"：content 是真实分镜剧本；
//   - status="failed"：content 为空，error 保存失败原因；前端据 status 字段过滤失败分镜。
type storyboardResultEntry struct {
	GroupIndex int    `json:"groupIndex"`
	ShotIndex  int    `json:"shotIndex,omitempty"` // 同组内的分镜序号（一组多条时用；单条时省略=0）
	Status     string `json:"status"`              // "completed" | "failed"
	Content    string `json:"content,omitempty"`
	Error      string `json:"error,omitempty"`
}

// storyboardLeadingLabelRegex 匹配模型输出开头可能残留的【分镜N】/场景N/镜头N 起始标记，清理后保留纯描述词
var storyboardLeadingLabelRegex = regexp.MustCompile(`(?i)^\s*【?\s*(?:场景|分镜|镜头|视频|Shot|Scene)\s*\d+\s*】?\s*[:：]?\s*`)

// executeStoryboardTask 分镜任务执行函数：逐章调文本模型，把每章整合为 1 条分镜剧本。
// 逻辑对齐前端 rewriteGroupToStoryboards：一章一组、一次模型调用产出一条完整视频描述词。
// onProgress 在每章完成后调用，把累计结果落库，前端轮询即可看到实时进度与已产出分镜。
//
// ⚠ 与前端共享语义：本函数内嵌 user content（资产参考文档 + 章节拼接 + shotDuration 注入）的 Go 副本，
// 同步对象是 web/src/lib/prompts/storyboard.ts。改提示词时务必同步两个文件，避免前后端分裂。
func executeStoryboardTask(task model.StoryboardTask, onProgress func(doneCount int, result string) error) error {
	log.Printf("[storyboard] task started id=%s model=%s chapters=%d", task.ID, task.Model, task.TotalCount)
	// 1) 解析渠道：优先用户本地渠道，其次云端渠道（与 pollVideoTaskFromUpstream 一致）
	var channel model.ModelChannel
	var err error
	if strings.TrimSpace(task.UserChannelID) != "" {
		channel, err = service.SelectUserLocalModelChannelForModel(task.UserID, task.Model, task.UserChannelID)
	} else {
		channel, err = service.SelectModelChannelForModel(task.Model, task.ChannelID)
	}
	if err != nil {
		return fmt.Errorf("分镜渠道不可用：%v", err)
	}

	// 2) 解析章节数组（前端传入的 [{title, content}]）
	var chapters []storyboardChapterInput
	if err := json.Unmarshal([]byte(task.Chapters), &chapters); err != nil {
		return fmt.Errorf("章节数据解析失败：%v", err)
	}
	if len(chapters) == 0 {
		return fmt.Errorf("没有可分镜的章节")
	}

	// 3) 解析资产数组（可为空）
	var assets []storyboardAssetInput
	if strings.TrimSpace(task.Assets) != "" {
		_ = json.Unmarshal([]byte(task.Assets), &assets)
	}

	// 4) 时长约束（注入提示词，限制单条分镜总时长）。
	//    不再硬截到 15 秒：shotDuration 由前端根据所选视频模型的合法时长区间传入（默认 8/15/30），
	//    prompt 内所有"15秒"等固定引用按 dur 替换；服务端只负责保底防呆 + 注入到提示词。
	dur := task.ShotDuration
	if dur < 1 {
		dur = 8
	}
	if dur > 120 {
		dur = 120
	}
	// 把提示词里所有硬编码的"15秒"按实际 dur 替换（不限 15 以下），保证长分镜也能让模型理解目标时长。
	dynamicScriptPrompt := strings.ReplaceAll(task.ScriptPrompt, "15秒", fmt.Sprintf("%d秒", dur))

	// 5) 构建资产参考文档段（注入提示词，让模型知道每个角色/场景的外观描述）
	assetRefSection := ""
	if len(assets) > 0 {
		lines := []string{
			`【角色/场景/道具参考文档】（以下资产的名称用于"出场角色/场景"行引用，描述用于了解外观，分镜描述必须严格参考）：`,
		}
		for _, a := range assets {
			typeLabel := "道具"
			switch a.Type {
			case "character":
				typeLabel = "角色"
			case "scene":
				typeLabel = "场景"
			case "reference":
				typeLabel = "参考"
			}
			desc := a.Description
			if desc == "" {
				desc = a.Name
			}
			lines = append(lines, fmt.Sprintf("- %s（%s）：%s", a.Alias, typeLabel, desc))
		}
		lines = append(lines, `注意：出场角色/场景行中的名称必须与上述列表一致，角色外观、服装、场景描述需严格参考上述描述，不可自行编造。`, "")
		assetRefSection = strings.Join(lines, "\n")
	}

	// 6) 逐章调文本模型（一章一条分镜，与前端 CHAPTERS_PER_GROUP=1 一致）
	results := make([]storyboardResultEntry, 0, len(chapters))
	const maxAttempts = 2 // 单章失败自动重试 1 次（共 2 次尝试），对齐前端逻辑
	for gi, ch := range chapters {
		startCh := gi + 1
		endCh := startCh
		chapterLabel := fmt.Sprintf("第%d章", startCh)
		if startCh != endCh {
			chapterLabel = fmt.Sprintf("第%d~%d章", startCh, endCh)
		}
		// 组合本章正文（含章节标题，帮助模型理解上下文与衔接）
		groupText := ch.Title + "\n" + ch.Content
		if strings.TrimSpace(ch.Title) == "" {
			groupText = ch.Content
		}

		// 构建用户提示词（与前端 rewriteGroupToStoryboards 完全对齐）
		// P1 b1/b2：shotCount > 1 时让模型用 ===SHOT=== 分隔出 N 条
		shotCount := ch.ShotCount
		if shotCount < 1 {
			shotCount = 1
		}
		multiShotHint := ""
		if shotCount > 1 {
			multiShotHint = fmt.Sprintf(`- 【分镜数】本章需要拆成 %d 条分镜。请输出 %d 条独立分镜，每条之间用 3 个连续的 "===SHOT==="（英文等号）独占一行分隔：`, shotCount, shotCount)
		}
		userContentParts := []string{}
		if assetRefSection != "" {
			userContentParts = append(userContentParts, assetRefSection)
		}
		userContentParts = append(userContentParts, []string{
			fmt.Sprintf(`以下是小说%s的正文，请你作为导演把这一整章剧情【整合成 %d 条分镜视频描述词】：`, chapterLabel, shotCount),
			multiShotHint,
			fmt.Sprintf(`- 总时长不超过 %d 秒；用 0-Xs｜、Xs-Ys｜ 这样的时间段把镜头在同一条描述内部自然分层，每个时间段是一次运镜/一个机位，至少 2 个运镜段；`, dur),
			`- 开头单独两行标注「出场角色：角色1；角色2；角色3」和「场景：场景1；场景2」，随后用 { } 包裹按时间段展开的运镜描述；`,
			`- 详细描述画面构图、人物位置关系、连贯的人物动作、台词、运镜、光影，按剧情推进自然衔接，承接上一章；`,
			`- 台词零更改一字不动，说话/动作/表情前必须用"@角色名"的形式标注，以便系统自动关联角色资产；不要遗漏关键情节，也不要凭空添加剧本外的剧情；`,
			`- 【重要】角色外观、服装、场景描述必须严格参考上方【角色/场景/道具参考文档】中的描述，不可自行编造。`,
			`- 只输出分镜描述词本身，不要输出解释、总结或额外文字。`,
			``,
			groupText,
		}...)
		userContent := strings.Join(userContentParts, "\n")

		// 重试循环：产出非空分镜即成功；抛错或空返回则重试
		var content string
		var lastErr string
		succeeded := false
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			draft, callErr := service.CallChatCompletion(channel, task.Model, dynamicScriptPrompt, userContent)
			if callErr != nil {
				lastErr = callErr.Error()
				continue
			}
			// 清理开头残留的【分镜N】/场景N 标记
			cleaned := strings.TrimSpace(storyboardLeadingLabelRegex.ReplaceAllString(draft, ""))
			if cleaned != "" {
				content = cleaned
				succeeded = true
				break
			}
			lastErr = "模型返回为空"
		}

		// 失败章节：不再用 ⚠ 占位字符串，改用 status="failed"。
		// 给前端提供 error 字段直接展示失败原因，避免 UI 还得扫字符串前缀判断状态。
		var failedEntry storyboardResultEntry
		if !succeeded {
			failedEntry = storyboardResultEntry{
				GroupIndex: gi,
				ShotIndex:  0,
				Status:     "failed",
				Error:      fmt.Sprintf("%s 未生成（%s），可点\"重新生成\"重试", chapterLabel, firstNonEmpty(lastErr, "生成失败")),
			}
		}

		// 追加到结果数组：支持一章产出多条分镜
		// 拆分规则（按优先级）：
		//   1) 用显式分隔符 ===SHOT=== 拆分（用户提示词里约定了分隔符时）
		//   2) 否则整条视为 1 条（保持兼容，默认/纯输出时行为不变）
		var shotEntries []storyboardResultEntry
		if !succeeded {
			shotEntries = []storyboardResultEntry{failedEntry}
		} else if strings.Contains(content, "===SHOT===") {
			for si, part := range strings.Split(content, "===SHOT===") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				shotEntries = append(shotEntries, storyboardResultEntry{
					GroupIndex: gi,
					ShotIndex:  si,
					Status:     "completed",
					Content:    part,
				})
			}
		} else {
			shotEntries = []storyboardResultEntry{{
				GroupIndex: gi,
				Status:     "completed",
				Content:    content,
			}}
		}
		results = append(results, shotEntries...)

		// 落库实时进度（前端轮询即可看到已产出分镜）
		// onProgress 返回 ErrStoryboardCanceled 表示用户中途取消：原样上抛，不要包成普通失败错误，
		// service 层据此判断是否覆盖 canceled 状态。
		resultJSON, _ := json.Marshal(results)
		if err := onProgress(gi+1, string(resultJSON)); err != nil {
			if errors.Is(err, service.ErrStoryboardCanceled) {
				return err
			}
			return fmt.Errorf("进度落库失败：%v", err)
		}
	}

	return nil
}
