package model

// StoryboardTask 分镜生成任务：后端循环调文本模型，把小说章节逐章整合为分镜剧本。
// 任务化目的——前端刷新后可通过轮询恢复进度，已产出的分镜落库不丢，不再依赖前端进程存活。
// 执行模式与 VideoTask 不同：VideoTask 是后端轮询上游视频 API；StoryboardTask 是后端自己调文本模型生成。
//
// Result JSON 单条分镜 = { groupIndex, shotIndex?, status, content?, error? }：
//   - status: "completed" | "failed"（failed 分镜 content 为空，error 字段保存失败原因）
//   - 老的"⚠"占位字符串已弃用，前端/前端兜底会忽略 history 数据；新写入只走 status 字段。
type StoryboardTask struct {
	ID              string `json:"id" gorm:"primaryKey"`
	UserID          string `json:"userId" gorm:"index"`
	UserDisplayName string `json:"userDisplayName"`
	Model           string `json:"model" gorm:"index"`
	ChannelID       string `json:"channelId" gorm:"index"`
	UserChannelID   string `json:"userChannelId" gorm:"index"`
	ChannelName     string `json:"channelName"`
	Source          string `json:"source" gorm:"index"`   // 固定 novel-workbench
	SourceID        string `json:"sourceId" gorm:"index"` // 前端 NovelProject.id，便于关联
	Status          string `json:"status" gorm:"index"`   // queued | running | completed | failed
	Progress        int    `json:"progress"`              // 0-100
	DoneCount       int    `json:"doneCount"`
	TotalCount      int    `json:"totalCount"`
	ShotDuration    int    `json:"shotDuration"` // 单条分镜目标时长（秒），注入提示词约束总时长
	ScriptPrompt    string `json:"scriptPrompt" gorm:"type:text"`     // 系统提示词（小说→分镜改写风格）
	Chapters        string `json:"chapters" gorm:"type:longtext"`     // 输入：[{title,content}]
	Assets          string `json:"assets" gorm:"type:longtext"`       // 输入：[{alias,type,description,name}]，可为空
	Result          string `json:"result" gorm:"type:longtext"`       // 输出：[{groupIndex,content}]，逐章追加
	Error           string `json:"error" gorm:"type:text"`
	// HoldID 关联的余额占用记录（2026-08-27 引入）。
	// 与 model.VideoTask.HoldID 语义一致：分镜文本模型按章节预估扣费（taskID 当 requestID），
	// worker 完成时 SettleBalanceHold 标记结算，全章失败/取消时 CancelBalanceHold 退款。
	HoldID          string `json:"holdId" gorm:"index"`
	CreatedAt       string `json:"createdAt" gorm:"index"`
	UpdatedAt       string `json:"updatedAt" gorm:"index"`
	StartedAt       string `json:"startedAt"`
	CompletedAt     string `json:"completedAt"`
}
