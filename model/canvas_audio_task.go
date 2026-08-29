package model

type CanvasAudioTask struct {
	ID              string `json:"id" gorm:"primaryKey"`
	UserID          string `json:"userId" gorm:"index:idx_canvas_audio_tasks_user_source_node,priority:1"`
	UserDisplayName string `json:"userDisplayName"`
	Source          string `json:"source" gorm:"index:idx_canvas_audio_tasks_user_source_node,priority:2"`
	SourceID        string `json:"sourceId" gorm:"index:idx_canvas_audio_tasks_user_source_node,priority:3"`
	NodeID          string `json:"nodeId" gorm:"index:idx_canvas_audio_tasks_user_source_node,priority:4"`
	// ClientTaskID 前端生成的稳定幂等键（2026-08-17 改造）。配合后端 ConsumeUserBalanceWithHold
	// 用它当 requestID 实现"网络重试同请求不双扣"；user+clientTaskId 联合 unique 索引也能
	// 防止数据库层创建重复任务。历史 ClientTaskID 为空的任务（改造前）允许，unique 索引只约束非空值。
	ClientTaskID   string `json:"clientTaskId" gorm:"index:idx_canvas_audio_tasks_user_client_task_id,priority:1,unique"`
	Model          string `json:"model"`
	ChannelID      string `json:"channelId"`
	UserChannelID  string `json:"userChannelId"`
	ChannelName    string `json:"channelName"`
	Status         string `json:"status"`
	Progress       int    `json:"progress"`
	Prompt         string `json:"prompt" gorm:"type:text"`
	Endpoint       string `json:"endpoint"`
	ContentType    string `json:"contentType"`
	RequestBody    string `json:"requestBody" gorm:"type:text"`
	ResponseBody   string `json:"responseBody" gorm:"type:text"`
	Error          string `json:"error" gorm:"type:text"`
	ErrorDetail    string `json:"errorDetail" gorm:"type:text"`
	AudioURL       string `json:"audioUrl" gorm:"type:text"`
	StorageKey     string `json:"storageKey"`
	MimeType       string `json:"mimeType"`
	Bytes          int64  `json:"bytes"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	StartedAt      string `json:"startedAt"`
	CompletedAt    string `json:"completedAt"`
}
