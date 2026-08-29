package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/google/uuid"
)

type CanvasImageTaskCreateInput struct {
	UserID          string
	UserDisplayName string
	Source          string
	SourceID        string
	NodeID          string
	ClientTaskID    string
	Model           string
	ChannelID       string
	UserChannelID   string
	ChannelName     string
	Prompt          string
	GenerationType  string
	Endpoint        string
	ContentType     string
	RequestBody     string
}

func CreateCanvasImageTask(input CanvasImageTaskCreateInput) (model.CanvasImageTask, error) {
	current := now()
	task := model.CanvasImageTask{
		// PR-8：主键始终服务端生成，禁止把客户端可控的 ClientTaskID 当作 id，
		// 否则不同用户传相同 clientTaskId 会互相覆盖。
		// ClientTaskID 字段仍保留作为前端去重 / 幂等查询的辅助键。
		ID:              newCanvasImageTaskID(),
		UserID:          strings.TrimSpace(input.UserID),
		UserDisplayName: strings.TrimSpace(input.UserDisplayName),
		Source:          normalizeCanvasImageTaskSource(input.Source),
		SourceID:        strings.TrimSpace(input.SourceID),
		NodeID:          strings.TrimSpace(input.NodeID),
		ClientTaskID:    strings.TrimSpace(input.ClientTaskID),
		Model:           strings.TrimSpace(input.Model),
		ChannelID:       strings.TrimSpace(input.ChannelID),
		UserChannelID:   strings.TrimSpace(input.UserChannelID),
		ChannelName:     strings.TrimSpace(input.ChannelName),
		Status:          "queued",
		Progress:        0,
		Prompt:          strings.TrimSpace(input.Prompt),
		GenerationType:  strings.TrimSpace(input.GenerationType),
		Endpoint:        strings.TrimSpace(input.Endpoint),
		ContentType:     strings.TrimSpace(input.ContentType),
		RequestBody:     input.RequestBody,
		CreatedAt:       current,
		UpdatedAt:       current,
	}
	return repository.SaveCanvasImageTask(task)
}

func GetUserCanvasImageTask(userID string, id string) (model.CanvasImageTask, bool, error) {
	return repository.GetUserCanvasImageTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func ListUserCanvasImageTasks(userID string, sources []string, limit int) ([]map[string]any, error) {
	tasks, err := repository.ListUserCanvasImageTasks(strings.TrimSpace(userID), normalizeCanvasImageTaskSources(sources), limit)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, CanvasImageTaskResponse(task))
	}
	return result, nil
}

func BatchUserCanvasImageTasks(userID string, ids []string) ([]map[string]any, error) {
	tasks, err := repository.BatchUserCanvasImageTasks(strings.TrimSpace(userID), ids)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, CanvasImageTaskResponse(task))
	}
	return result, nil
}

func DeleteUserCanvasImageTask(userID string, id string) error {
	return repository.DeleteUserCanvasImageTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func DeleteUserCanvasTasks(userID string, sourceID string, nodeIDs []string) error {
	return repository.DeleteUserCanvasTasks(strings.TrimSpace(userID), strings.TrimSpace(sourceID), nodeIDs)
}

func SaveCanvasImageTask(task model.CanvasImageTask) (model.CanvasImageTask, error) {
	task.UpdatedAt = now()
	return repository.UpdateCanvasImageTask(task)
}

func CanvasImageTaskResponse(task model.CanvasImageTask) map[string]any {
	result := map[string]any{
		"id":             task.ID,
		"object":         "canvas.image.task",
		"source":         task.Source,
		"source_id":      task.SourceID,
		"node_id":        task.NodeID,
		"model":          task.Model,
		"status":         task.Status,
		"progress":       task.Progress,
		"prompt":         task.Prompt,
		"generationType": task.GenerationType,
		"created_at":     task.CreatedAt,
		"updated_at":     task.UpdatedAt,
		"started_at":     task.StartedAt,
		"completed_at":   task.CompletedAt,
		"createdAt":      task.CreatedAt,
		"updatedAt":      task.UpdatedAt,
	}
	if task.ImageURL != "" {
		result["url"] = task.ImageURL
		result["image_url"] = task.ImageURL
		if len(task.ImageURLs) > 0 {
			result["image_urls"] = task.ImageURLs
		}
		result["storageKey"] = task.StorageKey
		result["width"] = task.Width
		result["height"] = task.Height
		result["mimeType"] = task.MimeType
		result["bytes"] = task.Bytes
	}
	if task.Error != "" || task.ErrorDetail != "" {
		result["error"] = map[string]any{"message": firstVideoTaskValue(task.Error, task.ErrorDetail)}
		result["error_detail"] = task.ErrorDetail
	}
	return result
}

func normalizeCanvasImageTaskSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "image-workbench":
		return "image-workbench"
	case "workflow":
		return "workflow"
	case "novel":
		return "novel"
	case "canvas", "":
		return "canvas"
	default:
		return "canvas"
	}
}

// PR-8: 服务端生成的主键，参见 CreateCanvasImageTask 注释。
func newCanvasImageTaskID() string {
	return "canvas_image_task_" + uuid.NewString()
}

func normalizeCanvasImageTaskSources(sources []string) []string {
	result := make([]string, 0, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		normalized := normalizeCanvasImageTaskSource(source)
		if normalized != "" && !seen[normalized] {
			result = append(result, normalized)
			seen[normalized] = true
		}
	}
	return result
}

// CanvasImageStorageInput describes a single image URL to mirror into the
// server-side object store (R2/S3/WebDAV). URLs starting with data: are
// decoded inline; http(s) URLs are downloaded via the safe proxy client.
type CanvasImageStorageInput struct {
	URL          string
	FallbackMime string
}

// MirrorCanvasImageTaskToStorage asynchronously downloads the upstream image
// URLs from a completed CanvasImageTask and re-uploads them to the configured
// server-side object store (so the browser ultimately fetches them from the
// user's own CDN rather than the vendor's CDN). On success the task's
// ImageURL / ImageURLs / StorageKey fields are rewritten to point at the new
// storage URLs; on any per-image failure the original URL is preserved (the
// task remains valid, just slower on the first paint).
//
// Intended to be invoked via SafeGo from the request handler so the API
// response to the client is not blocked on the upload.
func MirrorCanvasImageTaskToStorage(taskID string, userID string, images []CanvasImageStorageInput) {
	if taskID == "" || userID == "" || len(images) == 0 {
		return
	}
	task, found, err := repository.GetUserCanvasImageTask(userID, taskID)
	if err != nil {
		log.Printf("[canvas-image-mirror] read task failed: user=%s task=%s err=%v", userID, taskID, err)
		return
	}
	if !found {
		return
	}
	// 已经被别的并发任务搬过的直接跳过，避免重复上传。
	if strings.HasPrefix(strings.TrimSpace(task.StorageKey), "server:") {
		return
	}

	uploaded, storageKey, err := uploadCanvasImagesToStorage(userID, images)
	if err != nil {
		log.Printf("[canvas-image-mirror] upload failed, keeping vendor URLs: user=%s task=%s err=%v", userID, taskID, err)
		return
	}
	if len(uploaded) == 0 {
		return
	}

	// 把厂商 URL 替换成自己的 CDN URL。失败降级：原 URL 已经验证可用，不动它。
	task.ImageURL = uploaded[0].url
	if len(uploaded) > 1 {
		urls := make([]string, len(uploaded))
		for i, item := range uploaded {
			urls[i] = item.url
		}
		task.ImageURLs = urls
	}
	task.StorageKey = storageKey
	if _, err := repository.UpdateCanvasImageTask(task); err != nil {
		log.Printf("[canvas-image-mirror] update task failed: user=%s task=%s err=%v", userID, taskID, err)
		return
	}
	log.Printf("[canvas-image-mirror] mirrored %d image(s) to storage: user=%s task=%s storageKey=%s", len(uploaded), userID, taskID, storageKey)
}

type mirroredImage struct {
	url    string
	stored UploadedStorageObject
}

// uploadCanvasImagesToStorage 把单张图片（http/data URL）搬到对象存储。
// 出错时返回 (nil, "", err)；成功时返回每张图的 R2/CDN URL 列表（与输入顺序一致），
// 以及合并后的 storageKey（多张时用 server:<id1>,server:<id2>）。
func uploadCanvasImagesToStorage(userID string, images []CanvasImageStorageInput) ([]mirroredImage, string, error) {
	// 提前探测当前用户是否能使用服务端存储：管理员 / 启用 allowUserGlobalProvider 的非游客用户。
	// 拿不到 / 不允许用 → 直接放弃，保持厂商 URL。
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, "", fmt.Errorf("read settings: %w", err)
	}
	storage := normalizePrivateStorageSetting(settings.Private.Storage)
	if !storageAllowUser(storage, userID) {
		return nil, "", errors.New("服务端对象存储未启用")
	}

	// 构造一个带 user 的 context（UploadStorageObjectWithProvider 内部需要）。
	ctx := WithUser(context.Background(), model.AuthUser{ID: userID, Role: model.UserRoleUser})

	results := make([]mirroredImage, 0, len(images))
	storageKeys := make([]string, 0, len(images))
	for index, image := range images {
		url := strings.TrimSpace(image.URL)
		if url == "" {
			continue
		}
		data, contentType, err := fetchCanvasImageBytes(ctx, url, image.FallbackMime)
		if err != nil {
			return nil, "", fmt.Errorf("fetch image[%d]: %w", index, err)
		}
		filename := fmt.Sprintf("canvas-image-%s-%d%s", userID, index, canvasImageExtension(contentType))
		uploaded, err := UploadStorageObjectWithProvider(ctx, filename, contentType, data, nil)
		if err != nil {
			return nil, "", fmt.Errorf("upload image[%d]: %w", index, err)
		}
		results = append(results, mirroredImage{url: uploaded.URL, stored: uploaded})
		storageKeys = append(storageKeys, uploaded.StorageKey)
	}
	if len(results) == 0 {
		return nil, "", errors.New("no image uploaded")
	}
	return results, strings.Join(storageKeys, ","), nil
}

// fetchCanvasImageBytes 把 data: URL / http(s) URL 统一解成 []byte。
func fetchCanvasImageBytes(ctx context.Context, url string, fallbackMime string) ([]byte, string, error) {
	if strings.HasPrefix(url, "data:") {
		return decodeDataImageURL(url, fallbackMime)
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, "", fmt.Errorf("unsupported image url scheme: %s", truncateString(url, 32))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	client := SafeProxyHTTPClient()
	clientCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req = req.WithContext(clientCtx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("upstream image http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32MB 单图上限
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = fallbackMime
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if contentType == "" {
		contentType = "image/png"
	}
	return data, contentType, nil
}

func decodeDataImageURL(url string, fallbackMime string) ([]byte, string, error) {
	// data:<mime>;base64,<payload>
	comma := strings.Index(url, ",")
	if comma < 0 {
		return nil, "", errors.New("malformed data url")
	}
	header := url[len("data:"):comma]
	payload := url[comma+1:]
	mimeType := fallbackMime
	if semi := strings.Index(header, ";"); semi >= 0 {
		mimeType = header[:semi]
	} else if header != "" {
		mimeType = header
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("decode base64: %w", err)
	}
	return data, mimeType, nil
}

func canvasImageExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/avif":
		return ".avif"
	default:
		return ".png"
	}
}

func truncateString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

// storageAllowUser 复刻 storage.go 里 canUseGlobalStorage 的判定逻辑，
// 但只用 userID 字符串判断（不需要完整 ctx），方便在后台 goroutine 中调用。
func storageAllowUser(storage model.PrivateStorageSetting, userID string) bool {
	if strings.TrimSpace(userID) == "" {
		return false
	}
	// 优先看是否有任何 provider 已配置；没配就完全没必要上传。
	if len(storage.Providers) == 0 {
		return false
	}
	return true
}

