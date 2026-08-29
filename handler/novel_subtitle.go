package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2: shot-subtitle-node HTTP API ===

// DispatchShotSubtitleReq 单条字幕调度。
type DispatchShotSubtitleReq struct {
	ProjectID      string `json:"projectId"`
	ShotID         string `json:"shotId"`
	Text           string `json:"text"`
	ShotDurationMs int    `json:"shotDurationMs"`
}

// DispatchShotSubtitle POST /api/v1/novel/subtitle/dispatch
func DispatchShotSubtitle(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req DispatchShotSubtitleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if req.ProjectID == "" || req.ShotID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId/shotId 不能为空")
		return
	}
	if err := service.DispatchSubtitleForShot(user.ID, req.ProjectID, req.ShotID, req.Text, req.ShotDurationMs); err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "调度失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// DispatchProjectSubtitleReq 项目级字幕调度。
type DispatchProjectSubtitleReq struct {
	ProjectID string                     `json:"projectId"`
	Shots     []service.ShotForSubtitle  `json:"shots"`
}

// DispatchProjectSubtitle POST /api/v1/novel/subtitle/dispatch-project
func DispatchProjectSubtitle(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req DispatchProjectSubtitleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if req.ProjectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 不能为空")
		return
	}
	if err := service.DispatchSubtitleForProject(user.ID, req.ProjectID, req.Shots); err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "调度失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// UpdateSubtitleLinesReq 手动改字幕行。
type UpdateSubtitleLinesReq struct {
	ProjectID string                `json:"projectId"`
	ShotID    string                `json:"shotId"`
	Lines     []model.SubtitleLine  `json:"lines"`
}

// UpdateSubtitleLines PUT /api/v1/novel/subtitle/:projectId/:shotId/lines
func UpdateSubtitleLines(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.PathValue("projectId")
	shotID := r.PathValue("shotId")
	if projectID == "" || shotID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId/shotId 不能为空")
		return
	}
	var body struct {
		Lines []model.SubtitleLine `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if err := service.UpdateLines(user.ID, projectID, shotID, body.Lines); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "更新失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// GetShotSubtitle GET /api/v1/novel/subtitle?projectId=...&shotId=...
func GetShotSubtitle(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	shotID := r.URL.Query().Get("shotId")
	if projectID == "" || shotID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId/shotId 不能为空")
		return
	}
	s, lines, err := service.GetSubtitleForShot(projectID, shotID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	if s == nil {
		writeJSON(w, map[string]any{"code": 0, "data": nil})
		return
	}
	// 防御：仅返回当前用户的
	if s.UserID != user.ID {
		FailWithStatus(w, http.StatusNotFound, "字幕不存在")
		return
	}
	writeJSON(w, map[string]any{
		"code": 0,
		"data": map[string]any{
			"subtitle": s,
			"lines":    lines,
		},
	})
}

// ListShotSubtitles GET /api/v1/novel/subtitle?projectId=...
func ListShotSubtitles(w http.ResponseWriter, r *http.Request) {
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
	rows, err := service.ListSubtitleForProject(projectID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	filtered := make([]interface{}, 0)
	for _, s := range rows {
		if s.UserID == user.ID {
			filtered = append(filtered, s)
		}
	}
	writeJSON(w, map[string]any{"code": 0, "data": filtered})
}
