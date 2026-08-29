package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

// StorageConfig 返回公开存储配置。
func StorageConfig(w http.ResponseWriter, r *http.Request) {
	config, err := service.PublicStorageConfig()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, config)
}

// SaveUserStorageProvider 保存用户配置的存储提供商。
func SaveUserStorageProvider(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Provider service.UserStorageProviders `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "配置内容格式错误")
		return
	}
	config, err := service.SaveCurrentUserStorageProvider(r.Context(), request.Provider)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, config)
}

// MeasureUserStorageProvider 统计用户存储提供商的已用容量。
func MeasureUserStorageProvider(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Provider service.StorageObjectProviderInput `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "配置内容格式错误")
		return
	}
	result, err := service.MeasureUserStorageProvider(r.Context(), request.Provider)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

// UploadFile 上传文件到对象存储。
// uploadFileMaxBytes 限制单次文件上传大小。
const uploadFileMaxBytes = 50 << 20 // 50MB

func UploadFile(w http.ResponseWriter, r *http.Request) {
	// 限制整个 multipart 表单大小（含文件 + 字段）。
	r.Body = http.MaxBytesReader(w, r.Body, uploadFileMaxBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		Fail(w, "文件上传失败：文件过大或格式错误")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		Fail(w, "请选择要上传的文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, uploadFileMaxBytes+1))
	if err != nil {
		FailError(w, err)
		return
	}
	if len(data) > uploadFileMaxBytes {
		Fail(w, "文件超过大小限制（50MB）")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = http.DetectContentType(data)
	}
	var provider *service.StorageObjectProviderInput
	if raw := strings.TrimSpace(r.FormValue("provider")); raw != "" {
		var parsed service.StorageObjectProviderInput
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			Fail(w, "用户对象存储配置格式错误")
			return
		}
		provider = &parsed
	}
	object, err := service.UploadStorageObjectWithProvider(r.Context(), header.Filename, contentType, data, provider)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, object)
}

// DeleteFile 删除文件。
func DeleteFile(w http.ResponseWriter, r *http.Request, id string) {
	var request struct {
		Provider *service.StorageObjectProviderInput `json:"provider"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&request)
	}
	if err := service.DeleteStorageObject(r.Context(), id, request.Provider); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

// FileContent 获取文件内容。
func FileContent(w http.ResponseWriter, r *http.Request, id string) {
	// 优先尝试流式下载（本地存储零内存消耗）
	if streamed, err := service.DownloadStorageObjectStream(id); err == nil && streamed.IsLocal {
		w.Header().Set("Content-Type", streamed.Object.MimeType)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, streamed.FilePath)
		return
	}

	// 回退到原有逻辑（远程存储或其他情况）
	download, err := service.DownloadStorageObjectStreaming(id)
	if err != nil {
		FailError(w, err)
		return
	}
	if download.RedirectURL != "" {
		http.Redirect(w, r, download.RedirectURL, http.StatusTemporaryRedirect)
		return
	}
	w.Header().Set("Content-Type", download.Object.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	// 禁用 nginx/反向代理缓冲：慢网边收边发，避免整块读内存 + 被代理超时掐断
	w.Header().Set("X-Accel-Buffering", "no")
	if download.Body != nil {
		defer download.Body.Close()
		_, _ = io.Copy(w, download.Body)
		return
	}
	_, _ = w.Write(download.Data)
}

// FileInfo 获取文件元数据。
func FileInfo(w http.ResponseWriter, r *http.Request, id string) {
	object, err := service.StorageObjectInfo(id)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, object)
}

// AdminMeasureStorageProvider 管理员统计存储容量。
func AdminMeasureStorageProvider(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Index    int                    `json:"index"`
		Provider *model.StorageProvider `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "配置内容格式错误")
		return
	}
	result, err := service.MeasureAdminStorageProvider(request.Index, request.Provider)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

// proxyImageMaxBytes 限制图片代理响应体大小，防止被用作开放代理下载大文件。
const proxyImageMaxBytes = 32 << 20 // 32MB

// proxyMediaMaxBytes 限制媒体（图片/视频）代理响应体大小；视频通常更大，上限放宽。
const proxyMediaMaxBytes = 100 << 20 // 100MB

// proxyRemoteContent 代理远程内容（图片，可选视频），解决跨域与机器人检测问题。
// allowVideo 控制是否同时放行 video/*，用于区分"仅图片代理"与"媒体代理"；
// maxBytes 为响应体大小上限。仅放行媒体类型，拒绝其他响应，防止被用作通用开放代理。
func proxyRemoteContent(w http.ResponseWriter, r *http.Request, allowVideo bool, maxBytes int) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		Fail(w, "url 参数不能为空")
		return
	}
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		Fail(w, "无效的 url")
		return
	}
	client := service.SafeProxyHTTPClient()
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		FailError(w, err)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		FailWithStatus(w, http.StatusBadGateway, "代理图片请求失败")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		FailWithStatus(w, http.StatusBadGateway, "代理图片请求失败: "+resp.Status)
		return
	}
	contentType := resp.Header.Get("Content-Type")
	isImage := strings.HasPrefix(contentType, "image/")
	isVideo := strings.HasPrefix(contentType, "video/")
	// 仅放行媒体类型，拒绝其他响应（防止被用作通用代理）。
	if !isImage && !(allowVideo && isVideo) {
		if allowVideo {
			FailWithStatus(w, http.StatusUnsupportedMediaType, "仅支持图片或视频代理")
		} else {
			FailWithStatus(w, http.StatusUnsupportedMediaType, "仅支持图片代理")
		}
		return
	}
	// 已知 Content-Length 超限时，在写入任何字节前直接拒绝（保持开放代理防护）。
	if resp.ContentLength > int64(maxBytes) {
		FailWithStatus(w, http.StatusRequestEntityTooLarge, "媒体超过大小限制")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// 禁用反代缓冲：边收上游边发客户端，避免整块读 100MB 进内存导致的极慢与高内存占用。
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(resp.StatusCode)
	// 流式转发；LimitReader 兜底大小上限（Chunked 传输，无需先知道总长度）。
	_, _ = io.Copy(w, io.LimitReader(resp.Body, int64(maxBytes)))
}

// ProxyImage 仅代理图片（保持原有图片专用语义，防止被用作通用代理）。
func ProxyImage(w http.ResponseWriter, r *http.Request) {
	proxyRemoteContent(w, r, false, proxyImageMaxBytes)
}

// ProxyMedia 代理图片或视频（供 downloadRemoteMedia 等媒体下载/上传场景使用，解决跨域）。
func ProxyMedia(w http.ResponseWriter, r *http.Request) {
	proxyRemoteContent(w, r, true, proxyMediaMaxBytes)
}
