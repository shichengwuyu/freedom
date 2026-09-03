package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

// === 小说创作工作台 HTTP API（11 个端点） ===

// ListNovelWriteSessions GET /api/v1/novel-write/sessions
func ListNovelWriteSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	rows, err := service.ListNovelWriteSessions(user.ID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"sessions": rows}})
}

// createNovelWriteSessionReq 新建会话请求体。
type createNovelWriteSessionReq struct {
	Model        string            `json:"model"`
	SystemPrompt string            `json:"systemPrompt"`
	Variables    model.Variables   `json:"variables"`
}

// CreateNovelWriteSession POST /api/v1/novel-write/sessions
func CreateNovelWriteSession(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req createNovelWriteSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	sess, err := service.CreateNovelWriteSession(user.ID, req.Model, req.SystemPrompt, req.Variables)
	if err != nil {
		FailWithStatus(w, http.StatusBadRequest, "创建失败: "+err.Error())
		return
	}
	writeJSONWithStatus(w, http.StatusCreated, map[string]any{"code": 0, "data": sess})
}

// UpdateNovelWriteSession PATCH /api/v1/novel-write/sessions/:id
func UpdateNovelWriteSession(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if err := service.UpdateNovelWriteSession(user.ID, id, updates); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "更新失败: "+err.Error())
		return
	}
	// 回写最新 session
	sess, _ := service.GetNovelWriteSession(user.ID, id)
	writeJSON(w, map[string]any{"code": 0, "data": sess})
}

// DeleteNovelWriteSession DELETE /api/v1/novel-write/sessions/:id
func DeleteNovelWriteSession(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	if err := service.DeleteNovelWriteSession(user.ID, id); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "删除失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// ListNovelWriteMessages GET /api/v1/novel-write/sessions/:id/messages
func ListNovelWriteMessages(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	rows, err := service.ListNovelWriteMessages(user.ID, id)
	if err != nil {
		FailWithStatus(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"messages": rows}})
}

// sendNovelWriteMessageReq 发消息请求体。
type sendNovelWriteMessageReq struct {
	Content string `json:"content"`
}

// SendNovelWriteMessage POST /api/v1/novel-write/sessions/:id/messages
func SendNovelWriteMessage(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req sendNovelWriteMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	assistant, err := service.SendNovelWriteMessage(r.Context(), user.ID, id, req.Content)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "发送失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"assistant": assistant}})
}

// ContinueNovelWriteMessage POST /api/v1/novel-write/sessions/:id/messages/continue
func ContinueNovelWriteMessage(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	assistant, err := service.ContinueNovelWriteMessage(r.Context(), user.ID, id)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "续写失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"assistant": assistant}})
}

// GetNovelWritePrompt GET /api/v1/novel-write/sessions/:id/prompt
func GetNovelWritePrompt(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	systemPrompt, vars, err := service.GetNovelWritePrompt(user.ID, id)
	if err != nil {
		FailWithStatus(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{
		"systemPrompt": systemPrompt,
		"variables":    vars,
	}})
}

// putNovelWritePromptReq 替换 prompt 请求体。
type putNovelWritePromptReq struct {
	SystemPrompt string          `json:"systemPrompt"`
	Variables    model.Variables `json:"variables"`
}

// PutNovelWritePrompt PUT /api/v1/novel-write/sessions/:id/prompt
func PutNovelWritePrompt(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req putNovelWritePromptReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if err := service.PutNovelWritePrompt(user.ID, id, req.SystemPrompt, req.Variables); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// ExportNovelWriteStoryboard POST /api/v1/novel-write/sessions/:id/export
func ExportNovelWriteStoryboard(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	canvasProjectID, shotsCount, err := service.ExportNovelWriteStoryboard(r.Context(), user.ID, id)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "导出失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{
		"canvasProjectId": canvasProjectID,
		"shotsCount":      shotsCount,
	}})
}

// ListNovelWriteExports GET /api/v1/novel-write/exports
func ListNovelWriteExports(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	rows, err := service.ListNovelWriteExports(user.ID, 20)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"exports": rows}})
}
