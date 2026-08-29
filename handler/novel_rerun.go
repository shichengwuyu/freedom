package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2: novel-rerun-layer HTTP API ===

// RerunShotLayerReq 单分镜重做请求。
type RerunShotLayerReq struct {
	RunID   string                          `json:"runId"`
	ShotID  string                          `json:"shotId"`
	Layer   string                          `json:"layer"`
	Text    string                          `json:"text,omitempty"`
	VoiceID string                          `json:"voiceId,omitempty"`
	Speed   float64                         `json:"speed,omitempty"`
	Lines   []model.SubtitleLine            `json:"lines,omitempty"`
}

// RerunShotLayer POST /api/v1/novel/rerun/shot
func RerunShotLayer(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req RerunShotLayerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId query 参数必填")
		return
	}
	params := service.RerunShotLayerParams{
		ShotID:  req.ShotID,
		Layer:   req.Layer,
		Text:    req.Text,
		VoiceID: req.VoiceID,
		Speed:   req.Speed,
		Lines:   req.Lines,
	}
	rec, err := service.RerunShotLayer(r.Context(), user.ID, req.RunID, projectID, params)
	if err != nil {
		FailWithStatus(w, http.StatusBadRequest, "重做失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": rec})
}

// RerunFullLayerReq 整部重做请求。
type RerunFullLayerReq struct {
	RunID            string                    `json:"runId"`
	Layer            string                    `json:"layer"`
	SubtitleStyle    service.SubtitleStyleJSON  `json:"subtitleStyle,omitempty"`
	CompositionInput service.CompositionInput   `json:"compositionInput,omitempty"`
}

// RerunFullLayer POST /api/v1/novel/rerun/full
func RerunFullLayer(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req RerunFullLayerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId query 参数必填")
		return
	}
	params := service.RerunFullLayerParams{
		Layer:            req.Layer,
		SubtitleStyle:    req.SubtitleStyle,
		CompositionInput: req.CompositionInput,
	}
	rec, task, err := service.RerunFullLayer(r.Context(), user.ID, req.RunID, projectID, params)
	if err != nil {
		FailWithStatus(w, http.StatusBadRequest, "重做失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{
		"record":      rec,
		"composition": task,
	}})
}

// RollbackVersionReq 回滚请求。
type RollbackVersionReq struct {
	RecordID string `json:"recordId"`
}

// RollbackVersion POST /api/v1/novel/rerun/rollback
func RollbackVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req RollbackVersionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if err := service.RollbackToVersion(user.ID, req.RecordID); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "回滚失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// ListVersions GET /api/v1/novel/rerun/versions?projectId=...&scope=...&layer=...&shotId=...
func ListVersions(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	scope := r.URL.Query().Get("scope")
	layer := r.URL.Query().Get("layer")
	shotID := r.URL.Query().Get("shotId")
	if projectID == "" || scope == "" || layer == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId/scope/layer 必填")
		return
	}
	rows, err := service.ListVersions(user.ID, projectID, scope, layer, shotID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": rows})
}

// GetLatestVersion GET /api/v1/novel/rerun/latest?projectId=...&scope=...&layer=...&shotId=...
func GetLatestVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	scope := r.URL.Query().Get("scope")
	layer := r.URL.Query().Get("layer")
	shotID := r.URL.Query().Get("shotId")
	if projectID == "" || scope == "" || layer == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId/scope/layer 必填")
		return
	}
	rec, err := service.GetLatestRerunRecord(user.ID, projectID, scope, layer, shotID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	if rec == nil {
		writeJSON(w, map[string]any{"code": 0, "data": nil})
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": rec})
}
