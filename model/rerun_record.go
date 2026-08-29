package model

// RerunRecord novel-workflow v2 - novel-rerun-layer (核心 UX) 的版本记录。
//
// 每次"重做某层"（单分镜配音 / 字幕 / 整部字幕烧录 / 整部合成）写一条。
// 每层独立版本号（v1, v2, v3...），用户可回滚。
//
// scope 枚举：
//   - "shot" — 单分镜（具体 layer 必填）
//   - "full" — 整部成片
//
// layer 枚举：
//   - "video" / "dubbing" / "subtitle" / "composition" / "full"
type RerunRecord struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"index"`
	ProjectID string `json:"projectId" gorm:"index"`
	RunID     string `json:"runId" gorm:"index"`

	// 范围
	Scope string `json:"scope" gorm:"index"` // "shot" | "full"
	Layer string `json:"layer" gorm:"index"` // "video" | "dubbing" | "subtitle" | "composition" | "full"

	// 关联的 shot（scope=shot 时）
	ShotID string `json:"shotId" gorm:"index"`

	// 版本号（per scope+layer+shot 递增）
	Version int `json:"version"`

	// payload 快照（重做前的输入数据 + 状态）
	// 单分镜配音: { shotId, voiceId, speed, text, ttsProvider }
	// 单分镜字幕: { shotId, lines }
	// 整部字幕烧录: { subtitleStyle, allShotLines }
	// 整部合成: { input: CompositionInput }
	PayloadJSON string `json:"payloadJson" gorm:"type:longtext"`

	// 重做结果
	Status    string `json:"status" gorm:"index"` // "running" | "success" | "failure" | "canceled"
	OutputURL string `json:"outputUrl" gorm:"type:text"`
	Error     string `json:"error" gorm:"type:text"`

	// 关联的通用 task（shot-dubbing / composition 等走通用 worker）
	GenericTaskID string `json:"genericTaskId" gorm:"index"`

	CreatedAt   string `json:"createdAt" gorm:"index"`
	UpdatedAt   string `json:"updatedAt" gorm:"index"`
	CompletedAt string `json:"completedAt"`
}
