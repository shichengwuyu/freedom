package model

type VideoTask struct {
	ID              string `json:"id" gorm:"primaryKey"`
	UserID          string `json:"userId" gorm:"index"`
	UserDisplayName string `json:"userDisplayName"`
	Model           string `json:"model" gorm:"index"`
	ChannelID       string `json:"channelId" gorm:"index"`
	UserChannelID   string `json:"userChannelId" gorm:"index"`
	ChannelName     string `json:"channelName"`
	VendorType      string `json:"vendorType" gorm:"type:varchar(32);index"` // 非空=供应商视频任务（如 libtv），空=官方渠道任务
	Source          string `json:"source" gorm:"index"`
	SourceID        string `json:"source_id" gorm:"index"`
	UpstreamTaskID  string `json:"upstreamTaskId" gorm:"index"`
	UpstreamVideoID string `json:"upstreamVideoId" gorm:"index"`
	Status          string `json:"status" gorm:"index:idx_video_tasks_status_created_at,priority:1"`
	Progress        int    `json:"progress"`
	Seconds         string `json:"seconds"`
	Size            string `json:"size"`
	VideoURL        string `json:"videoUrl" gorm:"type:text"`
	Error           string `json:"error" gorm:"type:text"`
	ErrorDetail     string `json:"errorDetail" gorm:"type:text"`
	RequestBody     string `json:"requestBody" gorm:"type:text"`
	ResponseBody    string `json:"responseBody" gorm:"type:text"`
	LastResponse    string `json:"lastResponse" gorm:"type:text"`
	CostCents int `json:"costCents"`
	HoldID    string `json:"holdId" gorm:"index"` // 关联的余额占用 ID；失败退款走它，保证与 hold 状态机互斥、不双退
	CreatedAt       string `json:"createdAt" gorm:"index;index:idx_video_tasks_status_created_at,priority:2"`
	UpdatedAt       string `json:"updatedAt" gorm:"index"`
	StartedAt       string `json:"startedAt"`
	CompletedAt     string `json:"completedAt"`
	LastPolledAt    string `json:"lastPolledAt" gorm:"index"`
}
