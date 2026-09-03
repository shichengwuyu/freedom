package model

import "encoding/json"

// NovelWriteSession 小说创作会话（每会话独立 system prompt + 变量）。
//
// Sprint（小说创作工作台 v1）：
//   - UserID: 鉴权主键，所有查询 WHERE user_id = ?
//   - SystemPrompt: 作家自配的 system prompt 模板，支持 {{var_name}} 占位符
//   - Variables: JSON 字符串 {"var_name": "value", ...}，调 AI 时替换
//   - Model: 选用的模型 ID（per_call 扣费），immutable
type NovelWriteSession struct {
	ID           string `json:"id"           gorm:"primaryKey;size:64"`
	UserID       string `json:"userId"       gorm:"size:64;index:idx_novel_write_sessions_user_updated,priority:1"`
	Title        string `json:"title"        gorm:"size:255"`
	Model        string `json:"model"        gorm:"size:128"`
	SystemPrompt string `json:"systemPrompt" gorm:"type:text"`
	Variables    string `json:"variables"    gorm:"type:text"` // JSON 字符串，存 map[string]string
	CreatedAt    string `json:"createdAt"    gorm:"size:32"`
	UpdatedAt    string `json:"updatedAt"    gorm:"size:32;index:idx_novel_write_sessions_user_updated,priority:2"`
}

func (NovelWriteSession) TableName() string { return "novel_write_sessions" }

// NovelWriteMessage 小说创作消息（user / assistant / system 三种 role）。
type NovelWriteMessage struct {
	ID        int64  `json:"id"        gorm:"primaryKey;autoIncrement"`
	SessionID string `json:"sessionId" gorm:"size:64;index:idx_novel_write_messages_session,priority:1"`
	Role      string `json:"role"      gorm:"size:16"`  // 'system' | 'user' | 'assistant'
	Content   string `json:"content"   gorm:"type:text"`
	CreatedAt string `json:"createdAt" gorm:"size:32"`
}

func (NovelWriteMessage) TableName() string { return "novel_write_messages" }

// NovelWriteExport 小说创作导出历史（一次导出 = 一条记录，含完整 JSON 备份）。
type NovelWriteExport struct {
	ID              string `json:"id"              gorm:"primaryKey;size:64"`
	UserID          string `json:"userId"          gorm:"size:64;index:idx_novel_write_exports_user_created,priority:1"`
	SessionID       string `json:"sessionId"       gorm:"size:64;index:idx_novel_write_exports_session,priority:1"`
	CanvasProjectID string `json:"canvasProjectId" gorm:"size:64"`
	ShotsCount      int    `json:"shotsCount"`
	ExportJSON      string `json:"exportJson"      gorm:"type:text"`
	CreatedAt       string `json:"createdAt"       gorm:"size:32;index:idx_novel_write_exports_user_created,priority:2"`
}

func (NovelWriteExport) TableName() string { return "novel_write_exports" }

// Variables 变量映射的便利类型。
type Variables map[string]string

// MarshalVariables 把 map 序列化为 JSON 字符串。
func MarshalVariables(v Variables) string {
	if v == nil {
		return "{}"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// UnmarshalVariables 把 JSON 字符串反序列化为 map，失败返回空 map（不阻断）。
func UnmarshalVariables(s string) Variables {
	if s == "" {
		return Variables{}
	}
	var v Variables
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return Variables{}
	}
	return v
}
