package model

// CompositionTask novel 工作流 - 成片合成节点的任务模型（composition-layer）。
//
// 一次合成任务 = 一次 ffmpeg 调用 = 一段最终 mp4 输出。
// 任务本身独立于 novel_workflow_nodes 表（composition 是 full-composition 单节点；
// 但 composition 任务有自己的进度：5 个步骤 / progress_json / step_message）。
//
// 状态机（7 态，与 NovelWorkflowNode 一致）：
//   未启动 → 排队中 → 进行中 → （成功 / 失败 / 跳过 / 已取消）
type CompositionTask struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"index"`
	ProjectID string `json:"projectId" gorm:"index"`

	// 关联
	WorkflowRunID    string `json:"workflowRunId" gorm:"index"`
	WorkflowNodeID   string `json:"workflowNodeId" gorm:"index"`
	GenericTaskID    string `json:"genericTaskId" gorm:"index"`

	// 输入：ffmpeg 命令执行依赖的数据快照
	// { shotVideos: [{shotId, url, durationMs}], shotDubbings: [...], shotSubtitles: [...], bgmPresetId?: bgmCustomId?, bgmVolume: 0-1, subtitleStyle: {...}, introUrl?, outroUrl? }
	InputJSON string `json:"inputJson" gorm:"type:longtext"`

	// 进度：5 步 JSON
	// { currentStep: 1-5, totalSteps: 5, lastMessage: "归一化镜头 3 / 8", stepStartedAt, totalDurationSec }
	ProgressJSON string `json:"progressJson" gorm:"type:text"`

	// 输出
	OutputURL  string `json:"outputUrl" gorm:"type:text"`
	OutputSize int64  `json:"outputSize"`
	OutputMime string `json:"outputMime"`

	// 状态
	Status string `json:"status" gorm:"index"`
	Error  string `json:"error" gorm:"type:text"`
	// ffmpeg stderr 末尾 N 行（失败时存）
	StderrTail string `json:"stderrTail" gorm:"type:text"`

	CreatedAt   string `json:"createdAt" gorm:"index"`
	UpdatedAt   string `json:"updatedAt" gorm:"index"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
}
