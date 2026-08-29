package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tigerowo/freedom/repository"
	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2：HTTP API ===

// CreateNovelWorkflowRunReq 新建 run 的请求体。
type CreateNovelWorkflowRunReq struct {
	ProjectID   string   `json:"projectId"`
	Mode        string   `json:"mode"`        // auto | manual | quick | custom
	ShotIDs     []string `json:"shotIds"`     // 当前项目分镜 ID 列表（前端从 NovelProject.shotList 读）
	ConfigJSON  string   `json:"configJson"`  // 自定义模式用
}

// CreateNovelWorkflowRun POST /api/v1/novel/workflows
func CreateNovelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req CreateNovelWorkflowRunReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	if req.ProjectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 不能为空")
		return
	}
	run, err := service.CreateNovelWorkflowRun(user.ID, user.GroupID, req.ProjectID, req.Mode, req.ShotIDs, req.ConfigJSON)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "创建 run 失败: "+err.Error())
		return
	}
	writeJSONWithStatus(w, http.StatusCreated, map[string]any{"code": 0, "data": run})
}

// StartNovelWorkflowRun POST /api/v1/novel/workflows/:id/start
//
// novel-workflow v2 fix: 改用 wrapper 传 id 参数（r.PathValue 在 gin wrapF 下拿不到）
func StartNovelWorkflowRun(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	if id == "" {
		FailWithStatus(w, http.StatusBadRequest, "id 不能为空")
		return
	}
	run, _ := repository.GetNovelWorkflowRun(id)
	if run == nil || run.UserID != user.ID {
		FailWithStatus(w, http.StatusNotFound, "run 不存在")
		return
	}
	if err := service.StartNovelWorkflowRun(id); err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "启动失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": run})
}

// GetNovelWorkflowRun GET /api/v1/novel/workflows/:id
func GetNovelWorkflowRun(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	run, _ := repository.GetNovelWorkflowRun(id)
	if run == nil || run.UserID != user.ID {
		FailWithStatus(w, http.StatusNotFound, "run 不存在")
		return
	}
	nodes, _ := repository.ListNovelWorkflowNodesByRun(id)
	writeJSON(w, map[string]any{
		"code": 0,
		"data": map[string]any{
			"run":   run,
			"nodes": nodes,
		},
	})
}

// StartNovelWorkflowNode POST /api/v1/novel/workflows/:id/nodes/:nodeId/start
func StartNovelWorkflowNode(w http.ResponseWriter, r *http.Request, runID, nodeID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	run, _ := repository.GetNovelWorkflowRun(runID)
	if run == nil || run.UserID != user.ID {
		FailWithStatus(w, http.StatusNotFound, "run 不存在")
		return
	}
	if err := service.StartNovelWorkflowNode(runID, nodeID); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "启动节点失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// CancelNovelWorkflowNode POST /api/v1/novel/workflows/:id/nodes/:nodeId/cancel
func CancelNovelWorkflowNode(w http.ResponseWriter, r *http.Request, runID, nodeID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	run, _ := repository.GetNovelWorkflowRun(runID)
	if run == nil || run.UserID != user.ID {
		FailWithStatus(w, http.StatusNotFound, "run 不存在")
		return
	}
	if err := service.CancelNovelWorkflowNode(runID, nodeID); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "取消节点失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// RetryNovelWorkflowNode POST /api/v1/novel/workflows/:id/nodes/:nodeId/retry
func RetryNovelWorkflowNode(w http.ResponseWriter, r *http.Request, runID, nodeID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	run, _ := repository.GetNovelWorkflowRun(runID)
	if run == nil || run.UserID != user.ID {
		FailWithStatus(w, http.StatusNotFound, "run 不存在")
		return
	}
	if err := service.RetryNovelWorkflowNode(runID, nodeID); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "重试节点失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}

// ListNovelWorkflowRuns GET /api/v1/novel/workflows?projectId=...&page=1&pageSize=20
func ListNovelWorkflowRuns(w http.ResponseWriter, r *http.Request) {
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
	runs, total, err := repository.ListNovelWorkflowRunsByProject(projectID, pageSize, (page-1)*pageSize)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	// 仅返回当前用户的 run
	filtered := make([]interface{}, 0)
	for _, run := range runs {
		if run.UserID == user.ID {
			filtered = append(filtered, run)
		}
	}
	writeJSON(w, map[string]any{
		"code": 0,
		"data": map[string]any{
			"runs": filtered,
			"total": total,
			"page": page,
			"pageSize": pageSize,
		},
	})
}
