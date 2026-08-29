package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tigerowo/freedom/service"
)

// === novel-workflow v2: export-layer HTTP API ===

// GetExportMetadata GET /api/v1/novel/export/metadata?compositionId=...
func GetExportMetadata(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	compositionID := r.URL.Query().Get("compositionId")
	if compositionID == "" {
		FailWithStatus(w, http.StatusBadRequest, "compositionId 不能为空")
		return
	}
	meta, err := service.GetExportMetadata(compositionID, user.ID)
	if err != nil {
		FailWithStatus(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": meta})
}

// GeneratePlatformCaptionReq 文案生成请求。
type GeneratePlatformCaptionReq struct {
	Platform     string `json:"platform"`     // "douyin" / "xiaohongshu" / "shipinhao"
	ProjectTitle string `json:"projectTitle"` // 项目标题
	Description  string `json:"description"`  // 一句话描述（可选）
}

// GeneratePlatformCaption POST /api/v1/novel/export/caption
func GeneratePlatformCaption(w http.ResponseWriter, r *http.Request) {
	var req GeneratePlatformCaptionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		FailWithStatus(w, http.StatusBadRequest, "请求体不合法")
		return
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform == "" {
		platform = "douyin"
	}
	caption := service.GeneratePlatformCaption(platform, req.ProjectTitle, req.Description)
	writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"caption": caption}})
}

// ListExportHistory GET /api/v1/novel/export/history?projectId=...
func ListExportHistory(w http.ResponseWriter, r *http.Request) {
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
	rows, err := service.ListExportHistory(projectID, user.ID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"code": 0, "data": rows})
}

// DownloadComposition GET /api/v1/novel/export/download?compositionId=...
//
// v2 简化: 直接 stream 文件 (开发模式本地路径, 生产改 signed URL 重定向)
func DownloadComposition(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	compositionID := r.URL.Query().Get("compositionId")
	if compositionID == "" {
		FailWithStatus(w, http.StatusBadRequest, "compositionId 不能为空")
		return
	}
	task, err := service.GetCompositionTask(compositionID, user.ID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	if task == nil {
		FailWithStatus(w, http.StatusNotFound, "成片不存在")
		return
	}
	if task.OutputURL == "" {
		FailWithStatus(w, http.StatusNotFound, "成片尚未生成")
		return
	}
	// 防御: 确保 output url 在预期目录（防路径穿越）
	if !strings.HasPrefix(task.OutputURL, "data/compositions/") {
		FailWithStatus(w, http.StatusForbidden, "非法的成片路径")
		return
	}
	http.ServeFile(w, r, task.OutputURL)
}

// (本文件用 json.NewDecoder 直接解码; 保留注释以便未来扩展为统一 helper)
