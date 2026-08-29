package model

// ShotDubbing novel 工作流 - 镜头配音节点的数据模型。
//
// 每个 shot 最多 1 条 ShotDubbing（per-project 唯一性由 service 层保证）：
//   - 成功 TTS 调用后写入；失败保留空记录
//   - 配音失败 → shot 合成时用静音兜底（composition-layer 处理）
//
// 与 VideoTask / StoryboardTask 的关系：shot-dubbing 不复用通用 task 表（与 Sprint 4 决议一致
// ——4 套旧表 + 通用 task 各管各的；shot-dubbing 是 v2 新增能力，按"通用 task + 独立 ShotDubbing"
// 双轨：通用 task 用于派发 / 状态机，ShotDubbing 用于存结果 / 试听 URL）。
type ShotDubbing struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"index"`
	ProjectID string `json:"projectId" gorm:"index"`
	ShotID    string `json:"shotId" gorm:"index"` // 关联前端分镜 ID

	// TTS 参数（用户可改）
	Text    string  `json:"text" gorm:"type:text"`     // 输入文本
	VoiceID string  `json:"voiceId"`                  // "成熟男声" / "温柔女声" / 具体 provider voice id
	Speed   float64 `json:"speed"`                    // 0.5 - 2.0
	TtsModel string `json:"ttsModel" gorm:"index"`    // "mimo" / "volcano" / ...

	// TTS 产出
	AudioURL   string `json:"audioUrl" gorm:"type:text"`
	DurationMs int64  `json:"durationMs"`
	Bytes      int64  `json:"bytes"`
	MimeType   string `json:"mimeType"`

	// 状态：空（未生成）| success | failure | skipped
	Status string `json:"status" gorm:"index"`
	Error  string `json:"error" gorm:"type:text"`

	// 通用 task 关联（novel-workflow 派发用）
	GenericTaskID string `json:"genericTaskId" gorm:"index"`

	// 余额流水关联（type=generation_consume / generation_refund）
	BalanceLogID string `json:"balanceLogId" gorm:"index"`
	CostCents    int    `json:"costCents"`

	CreatedAt   string `json:"createdAt" gorm:"index"`
	UpdatedAt   string `json:"updatedAt" gorm:"index"`
	CompletedAt string `json:"completedAt"`
}
