package model

type AICallLog struct {
	ID              string `json:"id" gorm:"primaryKey"`
	UserID          string `json:"userId" gorm:"index"`
	UserDisplayName string `json:"userDisplayName" gorm:"->;-:migration"`
	Endpoint        string `json:"endpoint" gorm:"index"`
	Method          string `json:"method"`
	Model           string `json:"model" gorm:"index"`
	ChannelID       string `json:"channelId" gorm:"index"`
	ChannelName     string `json:"channelName"`
	// TokenID 关联 user_tokens.id；空字符串表示走 cookie 会话鉴权（Sprint 1.1 引入）。
	TokenID         string `json:"tokenId" gorm:"index"`
	Status          int    `json:"status" gorm:"index"`
	DurationMs      int64  `json:"durationMs"`
	CostCents int `json:"costCents"`
	RequestBody     string `json:"requestBody" gorm:"type:text"`
	ResponseBody    string `json:"responseBody" gorm:"type:text"`
	Error           string `json:"error" gorm:"type:text"`
	// Sprint 2 新增：渠道选择器 retry 诊断字段
	AttemptIndex       int    `json:"attemptIndex"`               // 0-based retry index；0=一次成功
	UpstreamStatusCode int    `json:"upstreamStatusCode"`         // 上游最终返回的 HTTP status code（最后一次）
	LastTryAt          string `json:"lastTryAt"`                 // 最后一次尝试发起时间（RFC3339 字符串，匹配项目其他表风格）
	KeyIndex           int    `json:"keyIndex"`                  // 多 key 模式下用了第几个 key（0-based）
	CreatedAt          string `json:"createdAt" gorm:"index"`
}

type AICallLogList struct {
	Items []AICallLog `json:"items"`
	Total int         `json:"total"`
}
