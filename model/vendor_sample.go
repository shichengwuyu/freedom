package model

import "time"

// VendorApiSample 浏览器插件嗅探到的供应商内部接口“样本”。
//
// 背景：UpDream / NewWow 这类供应商没有开放平台，后端无法预先知道它们的生图内部接口。
// 方案是“用户在自己浏览器里生成一次，插件把这次真实请求的 URL / 方法 / 头 / 体 + 响应”抓回来，
// 后端据此学习接口形状，之后用该用户自己的 Cookie 在后端重放（带用户凭据调各站内部接口）。
//
// 样本与“用户 + 供应商”绑定：因为每个用户的 Cookie 不同，重放必须用自己的样本对应的凭据。
type VendorApiSample struct {
	ID                 string    `json:"id" gorm:"primaryKey"`
	UserID             string    `json:"userId" gorm:"index"`
	VendorType         string    `json:"vendorType" gorm:"type:varchar(32);index"`
	URL                string    `json:"url" gorm:"type:longtext"`
	Method             string    `json:"method"`
	RequestHeadersJSON string    `json:"requestHeadersJson,omitempty" gorm:"type:longtext"`
	RequestBody        string    `json:"requestBody,omitempty" gorm:"type:longtext"`
	ResponseStatus     int       `json:"responseStatus"`
	ResponseHeadersJSON string   `json:"responseHeadersJson,omitempty" gorm:"type:longtext"`
	ResponseBody       string    `json:"responseBody,omitempty" gorm:"type:longtext"`
	ContentType        string    `json:"contentType,omitempty"`
	IsLikelyGeneration bool      `json:"isLikelyGeneration"`
	EndpointGroup      string    `json:"endpointGroup,omitempty" gorm:"type:varchar(255)"`
	CreatedAt          time.Time `json:"createdAt"`
}

// TableName 显式指定表名，避免 GORM 默认复数推断
func (VendorApiSample) TableName() string { return "vendor_api_samples" }
