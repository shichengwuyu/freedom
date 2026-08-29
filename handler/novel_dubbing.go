package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2: shot-dubbing-node HTTP API ===

// DispatchShotDubbingReq 调度单条配音请求。
type DispatchShotDubbingReq struct {
	ProjectID string  `json:"projectId"`
	ShotID    string  `json:"shotId"`
	Text      string  `json:"text"`
	VoiceID   string  `json:"voiceId"`
	Speed     float64 `json:"speed"`
}

// DispatchShotDubbing POST /api/v1/novel/dubbing/dispatch
func DispatchShotDubbing(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req DispatchShotDubbingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if req.ProjectID == "" || req.ShotID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId/shotId 不能为空")
		return
	}
	if err := service.DispatchForShot(r.Context(), user.ID, req.ProjectID, req.ShotID, req.Text, req.VoiceID, req.Speed); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "调度失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// DispatchProjectDubbingReq 调度项目所有配音请求。
type DispatchProjectDubbingReq struct {
	ProjectID string                 `json:"projectId"`
	VoiceID   string                 `json:"voiceId"`
	Speed     float64                `json:"speed"`
	Shots     []service.ShotForDubbing `json:"shots"`
}

// DispatchProjectDubbing POST /api/v1/novel/dubbing/dispatch-project
func DispatchProjectDubbing(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req DispatchProjectDubbingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if req.ProjectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 不能为空")
		return
	}
	if err := service.DispatchForProject(r.Context(), user.ID, req.ProjectID, req.Shots, req.VoiceID, req.Speed); err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "调度失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// ListShotDubbings GET /api/v1/novel/dubbing?projectId=...
func ListShotDubbings(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 不能为空")
		return
	}
	rows, err := service.ListForProject(projectID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	// 仅返回当前用户项目的配音（防御性检查）
	filtered := make([]interface{}, 0)
	for _, d := range rows {
		if d.UserID == user.ID {
			filtered = append(filtered, d)
		}
	}
	writeJSON(w, map[string]any{"code": 0, "data": filtered})
}
