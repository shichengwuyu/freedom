package handler

import (
	"net/http"

	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2: bgm-layer HTTP API ===

// ListBgmPresets GET /api/v1/bgm/presets?tag=...
// 公开接口（不需登录）：浏览系统预设 BGM。
func ListBgmPresets(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	presets := service.ListPresets(tag)
	writeJSON(w, map[string]any{"code": 0, "data": presets})
}

// ListBgmCustoms GET /api/v1/bgm/custom?projectId=...
// 登录：列项目自定义 BGM。
func ListBgmCustoms(w http.ResponseWriter, r *http.Request) {
	_, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId 不能为空")
		return
	}
	rows, err := service.ListCustomForProject(projectID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": rows})
}

// UploadBgmCustom POST /api/v1/bgm/custom/upload
// multipart/form-data: projectId, title, tags, file
func UploadBgmCustom(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	// 限制请求体大小
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	projectID := r.FormValue("projectId")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	if projectID == "" || title == "" {
		FailWithStatus(w, http.StatusBadRequest, "projectId/title 不能为空")
		return
	}
	_, fileHeader, err := r.FormFile("file")
	if err != nil {
		FailWithStatus(w, http.StatusBadRequest, "file 字段缺失: "+err.Error())
		return
	}
	bc, err := service.UploadCustom(user.ID, projectID, title, tags, fileHeader)
	if err != nil {
		FailWithStatus(w, http.StatusBadRequest, "上传失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": bc})
}

// DeleteBgmCustom DELETE /api/v1/bgm/custom/:id
func DeleteBgmCustom(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	if id == "" {
		FailWithStatus(w, http.StatusBadRequest, "id 不能为空")
		return
	}
	if err := service.DeleteCustom(user.ID, id); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "删除失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0})
}
