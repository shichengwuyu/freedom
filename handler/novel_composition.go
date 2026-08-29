package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tigerowo/freedom/repository"
	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2: composition-layer HTTP API ===

// CreateCompositionTaskReq 创建合成任务。
type CreateCompositionTaskReq struct {
	ProjectID       string                     `json:"projectId"`
	WorkflowRunID   string                     `json:"workflowRunId,omitempty"`
	WorkflowNodeID  string                     `json:"workflowNodeId,omitempty"`
	Input           service.CompositionInput  `json:"input"`
}

// CreateCompositionTask POST /api/v1/novel/composition
func CreateCompositionTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req CreateCompositionTaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if req.ProjectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 不能为空")
		return
	}
	if len(req.Input.ShotVideos) == 0 {
		FailWithStatus(w, http.StatusBadRequest, "input.shotVideos 不能为空")
		return
	}

	task, err := service.CreateCompositionTask(user.ID, req.ProjectID, req.WorkflowRunID, req.WorkflowNodeID, req.Input)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "创建任务失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": task})
}

// StartCompositionTask POST /api/v1/novel/composition/:id/start
//
// v2 同步执行：会阻塞直到 ffmpeg 完成（长任务可能 30s-5min）。
// 真实部署应改异步（写 status=queued + worker 拉取）。
func StartCompositionTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		FailWithStatus(w, http.StatusBadRequest, "id 不能为空")
		return
	}
	task, err := service.GetCompositionTask(id, user.ID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	if task == nil {
		FailWithStatus(w, http.StatusNotFound, "任务不存在")
		return
	}
	if err := service.RunCompositionTask(r.Context(), task); err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "合成失败: "+err.Error())
		return
	}
	// 重新读一次（status 已被 service 改）
	task, _ = service.GetCompositionTask(id, user.ID)
	writeJSON(w, map[string]any{"code": 0, "data": task})
}

// StopCompositionTask POST /api/v1/novel/composition/:id/stop
//
// v2 简化：仅把 status 标 canceled（不真 kill ffmpeg）；v3 接 ctx cancel。
func StopCompositionTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	id := r.PathValue("id")
	if err := service.CancelCompositionTask(id, user.ID); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "取消失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// RetryCompositionTask POST /api/v1/novel/composition/:id/retry
func RetryCompositionTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	id := r.PathValue("id")
	task, err := service.GetCompositionTask(id, user.ID)
	if err != nil || task == nil {
		FailWithStatus(w, http.StatusNotFound, "任务不存在")
		return
	}
	if err := service.RunCompositionTask(r.Context(), task); err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "重试失败: "+err.Error())
		return
	}
	task, _ = service.GetCompositionTask(id, user.ID)
	writeJSON(w, map[string]any{"code": 0, "data": task})
}

// GetCompositionTask GET /api/v1/novel/composition/:id
func GetCompositionTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	id := r.PathValue("id")
	task, err := service.GetCompositionTask(id, user.ID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	if task == nil {
		FailWithStatus(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": task})
}

// ListCompositionTasks GET /api/v1/novel/composition?projectId=...
func ListCompositionTasks(w http.ResponseWriter, r *http.Request) {
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
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	tasks, total, err := repository.ListCompositionTasksByProject(projectID, pageSize, (page-1)*pageSize)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	// 过滤当前用户
	filtered := make([]interface{}, 0)
	for _, t := range tasks {
		if t.UserID == user.ID {
			filtered = append(filtered, t)
		}
	}
	writeJSON(w, map[string]any{
		"code": 0,
		"data": map[string]any{
			"tasks": filtered,
			"total": total,
			"page":  page,
			"pageSize": pageSize,
		},
	})
}
