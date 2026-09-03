package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2: series-asset-lock HTTP API ===

// GetSeriesAssetLock GET /api/v1/novel/series-asset-lock?projectId=...
func GetSeriesAssetLock(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 必填")
		return
	}
	lock, err := service.GetLock(user.ID, projectID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	if lock == nil {
		writeJSON(w, map[string]any{"code": 0, "data": nil})
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": lock})
}

// UpdateSeriesAssetLockReq 改主资产包请求。
type UpdateSeriesAssetLockReq struct {
	CharacterIDs     []string `json:"characterIds"`
	SceneIDs         []string `json:"sceneIds"`
	PropIDs          []string `json:"propIds"`
	GlobalStylePrompt string   `json:"globalStylePrompt"`
}

// UpdateSeriesAssetLock PUT /api/v1/novel/series-asset-lock?projectId=...
func UpdateSeriesAssetLock(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 必填")
		return
	}
	var req UpdateSeriesAssetLockReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	lock, err := service.UpdateLock(user.ID, projectID, service.SeriesAssetLockParams{
		CharacterIDs:     req.CharacterIDs,
		SceneIDs:         req.SceneIDs,
		PropIDs:          req.PropIDs,
		GlobalStylePrompt: req.GlobalStylePrompt,
	})
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "更新失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": lock})
}

// LockSeriesAssetLock POST /api/v1/novel/series-asset-lock/lock?projectId=...
func LockSeriesAssetLock(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 必填")
		return
	}
	lock, err := service.Lock(user.ID, projectID)
	if err != nil {
		FailWithStatus(w, http.StatusBadRequest, "锁定失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": lock})
}

// UnlockSeriesAssetLock POST /api/v1/novel/series-asset-lock/unlock?projectId=...
func UnlockSeriesAssetLock(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 必填")
		return
	}
	lock, err := service.Unlock(user.ID, projectID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "解锁失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": lock})
}
