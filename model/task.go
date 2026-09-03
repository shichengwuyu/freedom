package model

import "time"

// Task 通用任务模型（Sprint 4 引入）。
//
// 用于 Sprint 4 之后**新增的能力**（agent task / batch task / image batch）；
// 不替代现有 video_tasks / canvas_image_tasks / canvas_audio_tasks / storyboard_tasks
// 这 4 套专用表（前端 type 依赖巨大，迁移留 Sprint 4.5）。
//
// Type 枚举：video / image / audio / storyboard / image_batch / asset_batch
// Status 枚举：pending / running / success / failure / canceled
type Task struct {
	ID           string     `json:"id" gorm:"primaryKey"`
	UserID       string     `json:"userId" gorm:"index"`
	Type         string     `json:"type" gorm:"index"`
	Capability   string     `json:"capability" gorm:"index"` // text/image/video/audio
	Status       string     `json:"status" gorm:"index"`
	Progress     int        `json:"progress"`
	VendorType   string     `json:"vendorType"`              // ""=official / "libtv"/"updream"/"newwow"
	ModelName    string     `json:"modelName" gorm:"index"`
	ChannelID    string     `json:"channelId"`               // 官方渠道 ID（vendor 模式为空）
	KeyIndex     int        `json:"keyIndex"`                 // 多 key 轮询
	PayloadJSON  string     `json:"payloadJson" gorm:"type:longtext"`
	ResultJSON   string     `json:"resultJson" gorm:"type:longtext"`
	ErrorMessage string     `json:"errorMessage" gorm:"type:text"`
	Attempts     int        `json:"attempts"`
	MaxAttempts  int        `json:"maxAttempts"`
	CostCents    int        `json:"costCents"`
	HoldID       string     `json:"holdId" gorm:"index"` // 关联 BalanceHold（pre-consume）
	CreatedAt    time.Time  `json:"createdAt" gorm:"index"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	LastPolledAt *time.Time `json:"lastPolledAt,omitempty"`
}

// Task type 常量
const (
	TaskTypeVideo      = "video"
	TaskTypeImage      = "image"
	TaskTypeAudio      = "audio"
	TaskTypeStoryboard = "storyboard"
	TaskTypeImageBatch = "image_batch"
	TaskTypeAssetBatch = "asset_batch"
)

// Task status 常量
const (
	TaskStatusPending  = "pending"
	TaskStatusRunning  = "running"
	TaskStatusSuccess  = "success"
	TaskStatusFailure  = "failure"
	TaskStatusCanceled = "canceled"
)
