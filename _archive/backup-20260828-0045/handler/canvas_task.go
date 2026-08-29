package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

func CreateCanvasImageTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	body, contentType, endpoint, source, nodeID, sourceID, clientTaskID, prompt, channelID, err := readCanvasTaskAIRequest(r, "/images/generations")
	if err != nil {
		log.Printf("read canvas task AI request failed: %v", err)
		Fail(w, "任务请求参数无效")
		return
	}
	modelName := readAIModelFromBody(body, contentType)
	if strings.TrimSpace(modelName) == "" {
		Fail(w, "缺少模型名称")
		return
	}
	// Bug #3（2026-08-24 扩展到 canvas-image-tasks 端点）：
	//   rolldek / apimart 的 /v1/images/generations 上游都拒收 gemini-3-pro-image-preview /
	//   gemini-3.1-flash-image-preview（错误文案一字不差，提示 rolldek 透传到 apimart）。
	//   ai.go 里的拦截只在 isAPIMartChannel 守护，novel 资产生图、画布图片节点走 canvas-image-tasks
	//   端点不经过那段逻辑，所以这里补一道：命中 → 4xx + 明确错误，告知用户换 imagen-* 模型。
	if bad, blocked := apimartImageModelUnsupportedByUpstream(modelName); blocked {
		errMsg := fmt.Sprintf("图片生成拒绝：%q 不被当前上游接受（rolldek / apimart 公共 OpenAI 兼容渠道都拒收 /v1/images/generations 的 gemini-3-pro-image-preview / gemini-3.1-flash-image-preview）。请改用 imagen-* 系列（imagen-3.0-generate-002 / imagen-4.0-generate-001）或 official-image-* 系列。", bad)
		log.Printf("canvas image task blocked unsupported model: model=%s reason=%s", modelName, errMsg)
		Fail(w, errMsg)
		return
	}
	// 2026-08-27：删除 libtv/updream/newwow 第三方云端供应商。原先"vendor 模式不查 ModelChannel"的分支
	// 已废弃，所有画布图片任务统一走官方 ModelChannel（管理员后台配置的渠道）。
	channelID = firstNonEmpty(channelID, r.Header.Get("X-Model-Channel-ID"))
	userChannelID := r.Header.Get(userModelChannelHeader)
	if strings.TrimSpace(channelID) == "" && strings.TrimSpace(userChannelID) == "" {
		Fail(w, "缺少模型渠道")
		return
	}
	channel, resolvedUserChannelID, err := selectAIRequestChannel(user, modelName, channelID, userChannelID)
	if err != nil {
		log.Printf("canvas image task select channel failed: model=%s err=%v", modelName, err)
		failAIChannelSelect(w, err, "AI 接口请求失败")
		return
	}
	task, err := service.CreateCanvasImageTask(service.CanvasImageTaskCreateInput{
		UserID:          user.ID,
		UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
		Source:          source,
		SourceID:        sourceID,
		NodeID:          nodeID,
		ClientTaskID:    clientTaskID,
		Model:           modelName,
		ChannelID:       channel.ID,
		UserChannelID:   resolvedUserChannelID,
		ChannelName:     channel.Name,
		Prompt:          prompt,
		GenerationType:  strings.TrimPrefix(endpoint, "/images/"),
		Endpoint:        endpoint,
		ContentType:     contentType,
		RequestBody:     summarizeAIRequest(body, contentType),
	})
	if err != nil {
		log.Printf("create canvas image task failed: user=%s err=%v", user.ID, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, service.CanvasImageTaskResponse(task))
	service.SafeGo("canvas-image-task:"+task.ID, func(r any) {
		saveFailedCanvasImageTask(task, fmt.Sprintf("panic: %v", r), fmt.Sprintf("panic: %v", r))
	}, func() {
		runCanvasImageTask(task, user, body, contentType, task.ChannelID, task.UserChannelID)
	})
}

func GetCanvasImageTask(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	task, found, err := service.GetUserCanvasImageTask(user.ID, id)
	if err != nil {
		log.Printf("read canvas image task failed: user=%s id=%s err=%v", user.ID, id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	if !found {
		Fail(w, "图片任务不存在")
		return
	}
	OK(w, service.CanvasImageTaskResponse(task))
}

func UserCanvasImageTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	tasks, err := service.ListUserCanvasImageTasks(user.ID, readCanvasTaskSources(r), 100)
	if err != nil {
		log.Printf("list canvas image tasks failed: user=%s err=%v", user.ID, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, tasks)
}

func BatchCanvasImageTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var request struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "图片任务参数无效")
		return
	}
	tasks, err := service.BatchUserCanvasImageTasks(user.ID, request.IDs)
	if err != nil {
		log.Printf("batch canvas image tasks failed: user=%s err=%v", user.ID, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, tasks)
}

func DeleteUserCanvasImageTask(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	if strings.TrimSpace(id) == "" {
		Fail(w, "图片任务不存在")
		return
	}
	if err := service.DeleteUserCanvasImageTask(user.ID, id); err != nil {
		log.Printf("delete canvas image task failed: user=%s id=%s err=%v", user.ID, id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, map[string]any{"deleted": true})
}

func DeleteUserCanvasTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}

	var request struct {
		SourceID string   `json:"source_id"`
		NodeIDs  []string `json:"node_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.SourceID) == "" {
		Fail(w, "画布任务参数无效")
		return
	}

	if err := service.DeleteUserCanvasTasks(user.ID, request.SourceID, request.NodeIDs); err != nil {
		log.Printf("delete canvas tasks failed: user=%s source=%s err=%v", user.ID, request.SourceID, err)
		Fail(w, "AI 接口请求失败")
		return
	}

	OK(w, map[string]any{"deleted": true})
}

func CreateCanvasAudioTask(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	body, contentType, endpoint, _, nodeID, sourceID, clientTaskID, prompt, channelID, err := readCanvasTaskAIRequest(r, "/audio/speech")
	if err != nil {
		log.Printf("read canvas audio task AI request failed: %v", err)
		Fail(w, "任务请求参数无效")
		return
	}
	modelName := readAIModelFromBody(body, contentType)
	if strings.TrimSpace(modelName) == "" {
		Fail(w, "缺少模型名称")
		return
	}
	channelID = firstNonEmpty(channelID, r.Header.Get("X-Model-Channel-ID"))
	userChannelID := r.Header.Get(userModelChannelHeader)
	if strings.TrimSpace(channelID) == "" && strings.TrimSpace(userChannelID) == "" {
		Fail(w, "缺少模型渠道")
		return
	}
	channel, resolvedUserChannelID, err := selectAIRequestChannel(user, modelName, channelID, userChannelID)
	if err != nil {
		log.Printf("canvas audio task select channel failed: model=%s err=%v", modelName, err)
		failAIChannelSelect(w, err, "AI 接口请求失败")
		return
	}
	task, err := service.CreateCanvasAudioTask(service.CanvasAudioTaskCreateInput{
		UserID:          user.ID,
		UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
		SourceID:        sourceID,
		NodeID:          nodeID,
		ClientTaskID:    clientTaskID,
		Model:           modelName,
		ChannelID:       channel.ID,
		UserChannelID:   resolvedUserChannelID,
		ChannelName:     channel.Name,
		Prompt:          prompt,
		Endpoint:        endpoint,
		ContentType:     contentType,
		RequestBody:     summarizeAIRequest(body, contentType),
	})
	if err != nil {
		log.Printf("create canvas audio task failed: user=%s err=%v", user.ID, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, service.CanvasAudioTaskResponse(task))
	service.SafeGo("canvas-audio-task:"+task.ID, func(r any) {
		saveFailedCanvasAudioTask(task, fmt.Sprintf("panic: %v", r), fmt.Sprintf("panic: %v", r))
	}, func() {
		runCanvasAudioTask(task, user, body, contentType, task.ChannelID, task.UserChannelID)
	})
}

func GetCanvasAudioTask(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	task, found, err := service.GetUserCanvasAudioTask(user.ID, id)
	if err != nil {
		log.Printf("read canvas audio task failed: user=%s id=%s err=%v", user.ID, id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	if !found {
		Fail(w, "音频任务不存在")
		return
	}
	OK(w, service.CanvasAudioTaskResponse(task))
}

func runCanvasImageTask(task model.CanvasImageTask, user model.AuthUser, body []byte, contentType string, channelID string, userChannelID string) {
	current := taskTime()
	task.Status = "processing"
	task.Progress = 10
	task.StartedAt = current
	task, _ = service.SaveCanvasImageTask(task)

	payload, status, responseContentType, upstreamBody, err := executeCanvasAIRequest(user, task.Endpoint, body, contentType, channelID, userChannelID, task.ClientTaskID)
	if err != nil {
		saveFailedCanvasImageTask(task, err.Error(), err.Error())
		return
	}
	// 错误路径上保留上游原始 body（由 copyAIResponse 通过 X-Upstream-Body-Base64 透传），
	// 避免把 handler.Fail() 的 {code,data,msg} 信封当成上游响应落库到 task.ErrorDetail，
	// 排查时丢细节。
	errorDetail := string(payload)
	if len(upstreamBody) > 0 {
		errorDetail = string(upstreamBody)
	}
	if status >= http.StatusBadRequest {
		message := readUpstreamAIErrorMessage(payload, status)
		saveFailedCanvasImageTask(task, message, errorDetail)
		return
	}
	if message := readWrappedTaskError(payload); message != "" {
		saveFailedCanvasImageTask(task, message, errorDetail)
		return
	}
	collectAll := isKIESeedreamLayerDecompositionModel(task.Model)
	imageURLs, mimeType, bytes, err := imageURLsFromAIResponse(payload, responseContentType, collectAll)
	if err != nil {
		saveFailedCanvasImageTask(task, err.Error(), errorDetail)
		return
	}
	task.Status = "completed"
	task.Progress = 100
	task.CompletedAt = taskTime()
	task.ResponseBody = string(payload)
	task.ImageURL = imageURLs[0]
	if collectAll {
		task.ImageURLs = imageURLs
	}
	task.StorageKey = ""
	task.MimeType = mimeType
	task.Bytes = bytes
	task.Width = 0
	task.Height = 0
	task.Error = ""
	task.ErrorDetail = ""
	if _, err := service.SaveCanvasImageTask(task); err != nil {
		log.Printf("save completed canvas image task failed: user=%s task=%s err=%v", user.ID, task.ID, err)
		return
	}
	// 异步把厂商 CDN 上的图片搬到我们自己的 R2/CDN。失败时回退到厂商 URL，
	// 不影响任务结果；用户首次打开仍然可用，只是没那么快。
	if len(imageURLs) > 0 {
		images := make([]service.CanvasImageStorageInput, 0, len(imageURLs))
		images = append(images, service.CanvasImageStorageInput{URL: imageURLs[0], FallbackMime: mimeType})
		if collectAll {
			for index := 1; index < len(imageURLs); index++ {
				images = append(images, service.CanvasImageStorageInput{URL: imageURLs[index], FallbackMime: mimeType})
			}
		}
		service.SafeGo("canvas-image-mirror:"+task.ID, func(r any) {
			log.Printf("[canvas-image-mirror] panic: task=%s panic=%v", task.ID, r)
		}, func() {
			service.MirrorCanvasImageTaskToStorage(task.ID, user.ID, images)
		})
	}
}

func runCanvasAudioTask(task model.CanvasAudioTask, user model.AuthUser, body []byte, contentType string, channelID string, userChannelID string) {
	current := taskTime()
	task.Status = "processing"
	task.Progress = 10
	task.StartedAt = current
	task, _ = service.SaveCanvasAudioTask(task)

	payload, status, responseContentType, upstreamBody, err := executeCanvasAIRequest(user, task.Endpoint, body, contentType, channelID, userChannelID, task.ClientTaskID)
	if err != nil {
		saveFailedCanvasAudioTask(task, err.Error(), err.Error())
		return
	}
	errorDetail := string(payload)
	if len(upstreamBody) > 0 {
		errorDetail = string(upstreamBody)
	}
	if status >= http.StatusBadRequest {
		message := readUpstreamAIErrorMessage(payload, status)
		saveFailedCanvasAudioTask(task, message, errorDetail)
		return
	}
	if message := readWrappedTaskError(payload); message != "" {
		saveFailedCanvasAudioTask(task, message, errorDetail)
		return
	}
	mimeType := strings.TrimSpace(strings.Split(responseContentType, ";")[0])
	if mimeType == "" {
		mimeType = strings.TrimSpace(http.DetectContentType(payload))
	}
	if strings.Contains(mimeType, "json") {
		saveFailedCanvasAudioTask(task, "音频接口没有返回音频文件", errorDetail)
		return
	}
	if task.ContentType != "" && strings.HasPrefix(task.ContentType, "audio/") {
		mimeType = task.ContentType
	}
	task.Status = "completed"
	task.Progress = 100
	task.CompletedAt = taskTime()
	task.ResponseBody = "[binary audio]"
	task.AudioURL = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(payload)
	task.StorageKey = ""
	task.MimeType = mimeType
	task.Bytes = int64(len(payload))
	task.Error = ""
	task.ErrorDetail = ""
	_, _ = service.SaveCanvasAudioTask(task)
}

func executeCanvasAIRequest(user model.AuthUser, endpoint string, body []byte, contentType string, channelID string, userChannelID string, clientTaskID string) ([]byte, int, string, []byte, error) {
	request := httptest.NewRequest(http.MethodPost, "http://canvas.local/api/v1"+endpoint, bytes.NewReader(body))
	request = request.WithContext(service.WithUser(context.Background(), user))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if strings.TrimSpace(userChannelID) != "" {
		request.Header.Set(userModelChannelHeader, userChannelID)
	} else if strings.TrimSpace(channelID) != "" {
		request.Header.Set("X-Model-Channel-ID", channelID)
	}
	// 透传 clientTaskId → proxyAIRequest 用它当 ConsumeUserBalanceWithHold 的 requestID，
	// 网络重试同一客户端任务不会双扣（2026-08-17 改造）。
	if strings.TrimSpace(clientTaskID) != "" {
		request.Header.Set(requestIDHeader, clientTaskID)
	}
	recorder := httptest.NewRecorder()
	proxyAIRequest(recorder, request, endpoint)
	response := recorder.Result()
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 32*1024*1024))
	upstreamBody := decodeUpstreamBodyHeader(response.Header.Get("X-Upstream-Body-Base64"))
	return payload, response.StatusCode, response.Header.Get("Content-Type"), upstreamBody, nil
}

// decodeUpstreamBodyHeader 解出 copyAIResponse 在 status>=400 时放在
// X-Upstream-Body-Base64 header 里的上游 raw body；拿到原始字节便于写 task.ErrorDetail。
// header 缺失或解码失败返回 nil。
func decodeUpstreamBodyHeader(encoded string) []byte {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil
	}
	return decoded
}

func saveFailedCanvasImageTask(task model.CanvasImageTask, message string, detail string) {
	task.Status = "failed"
	task.CompletedAt = taskTime()
	task.Error = firstNonEmpty(message, "图片生成失败")
	task.ErrorDetail = detail
	_, _ = service.SaveCanvasImageTask(task)
}

func saveFailedCanvasAudioTask(task model.CanvasAudioTask, message string, detail string) {
	task.Status = "failed"
	task.CompletedAt = taskTime()
	task.Error = firstNonEmpty(message, "音频生成失败")
	task.ErrorDetail = detail
	_, _ = service.SaveCanvasAudioTask(task)
}

func readCanvasTaskAIRequest(r *http.Request, fallbackEndpoint string) ([]byte, string, string, string, string, string, string, string, string, error) {
	contentType := r.Header.Get("Content-Type")
	raw, err := io.ReadAll(io.LimitReader(r.Body, aiRequestBodyLimit+1))
	if err != nil {
		return nil, "", "", "", "", "", "", "", "", err
	}
	if len(raw) > aiRequestBodyLimit {
		return nil, "", "", "", "", "", "", "", "", errors.New("请求体超过大小限制")
	}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		body, cleanedContentType, meta, err := stripCanvasTaskMultipartFields(raw, contentType)
		if err != nil {
			return nil, "", "", "", "", "", "", "", "", err
		}
		endpoint := firstNonEmpty(meta["_canvas_endpoint"], fallbackEndpoint)
		return body, cleanedContentType, endpoint, meta["_canvas_source"], meta["_canvas_node_id"], meta["_canvas_source_id"], meta["_canvas_task_id"], meta["_canvas_prompt"], meta["_canvas_channel_id"], nil
	}
	var wrapper struct {
		Endpoint     string          `json:"endpoint"`
		Source       string          `json:"source"`
		NodeID       string          `json:"nodeId"`
		SourceID     string          `json:"sourceId"`
		ClientTaskID string          `json:"clientTaskId"`
		TaskID       string          `json:"taskId"`
		Prompt       string          `json:"prompt"`
		ChannelID    string          `json:"channelId"`
		RequestBody  json.RawMessage `json:"requestBody"`
		Request      json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, "", "", "", "", "", "", "", "", err
	}
	body := wrapper.RequestBody
	if len(body) == 0 {
		body = wrapper.Request
	}
	if len(body) == 0 {
		return nil, "", "", "", "", "", "", "", "", errors.New("任务请求体不能为空")
	}
	endpoint := firstNonEmpty(wrapper.Endpoint, fallbackEndpoint)
	return body, "application/json", endpoint, wrapper.Source, wrapper.NodeID, wrapper.SourceID, firstNonEmpty(wrapper.ClientTaskID, wrapper.TaskID), wrapper.Prompt, wrapper.ChannelID, nil
}

func stripCanvasTaskMultipartFields(raw []byte, contentType string) ([]byte, string, map[string]string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", nil, err
	}
	form, err := multipart.NewReader(bytes.NewReader(raw), params["boundary"]).ReadForm(256 << 20)
	if err != nil {
		return nil, "", nil, err
	}
	defer form.RemoveAll()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	meta := map[string]string{}
	for key, values := range form.Value {
		if strings.HasPrefix(key, "_canvas_") {
			if len(values) > 0 {
				meta[key] = values[0]
			}
			continue
		}
		for _, value := range values {
			_ = writer.WriteField(key, value)
		}
	}
	for key, files := range form.File {
		for _, header := range files {
			file, err := header.Open()
			if err != nil {
				_ = writer.Close()
				return nil, "", nil, err
			}
			part, err := writer.CreateFormFile(key, header.Filename)
			if err != nil {
				_ = file.Close()
				_ = writer.Close()
				return nil, "", nil, err
			}
			_, copyErr := io.Copy(part, file)
			_ = file.Close()
			if copyErr != nil {
				_ = writer.Close()
				return nil, "", nil, copyErr
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", nil, err
	}
	return buffer.Bytes(), writer.FormDataContentType(), meta, nil
}

func readAIModelFromBody(body []byte, contentType string) string {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return readMultipartModel(body, contentType)
	}
	var payload struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &payload)
	return strings.TrimSpace(payload.Model)
}

func readWrappedTaskError(payload []byte) string {
	var root struct {
		Code  *int   `json:"code"`
		Msg   string `json:"msg"`
		Error any    `json:"error"`
		Data  any    `json:"data"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &root) != nil {
		return ""
	}
	if root.Code != nil && *root.Code != 0 {
		return firstNonEmpty(root.Msg, "AI 接口请求失败")
	}
	if root.Error != nil {
		if errMap, ok := root.Error.(map[string]any); ok {
			return firstNonEmpty(toStringSafe(errMap["message"]), toStringSafe(errMap["msg"]), toStringSafe(root.Error))
		}
		return toStringSafe(root.Error)
	}
	return ""
}

func imageBytesFromAIResponse(payload []byte) ([]byte, string, error) {
	candidates, err := imageCandidatesFromAIResponse(payload, "")
	if err != nil {
		return nil, "", err
	}
	for _, candidate := range candidates {
		data, mimeType, err := imageCandidateBytes(candidate)
		if err == nil && len(data) > 0 {
			return data, mimeType, nil
		}
	}
	return nil, "", errors.New("图片接口没有返回图片")
}

func imageURLsFromAIResponse(payload []byte, contentType string, collectAll bool) ([]string, string, int64, error) {
	candidates, err := imageCandidatesFromAIResponse(payload, contentType)
	if err != nil {
		return nil, "", 0, err
	}
	urls := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	firstMimeType := ""
	var firstBytes int64
	for _, candidate := range candidates {
		url := candidate
		mimeType := ""
		var bytes int64
		if !strings.HasPrefix(candidate, "http://") && !strings.HasPrefix(candidate, "https://") {
			data, detectedMimeType, err := imageCandidateBytes(candidate)
			if err != nil || len(data) == 0 {
				continue
			}
			mimeType = detectedMimeType
			bytes = int64(len(data))
			if !strings.HasPrefix(candidate, "data:image/") {
				url = "data:" + mimeType + ";base64," + candidate
			}
		}
		if seen[url] {
			continue
		}
		seen[url] = true
		urls = append(urls, url)
		if len(urls) == 1 {
			firstMimeType = mimeType
			firstBytes = bytes
		}
		if !collectAll {
			return urls, firstMimeType, firstBytes, nil
		}
	}
	if len(urls) == 0 {
		return nil, "", 0, errors.New("图片接口没有返回图片")
	}
	return urls, firstMimeType, firstBytes, nil
}

type serverSentJSONEvent struct {
	name string
	data any
}

func imageCandidatesFromAIResponse(payload []byte, contentType string) ([]string, error) {
	if !isServerSentEventResponse(payload, contentType) {
		var root any
		if err := json.Unmarshal(payload, &root); err != nil {
			return nil, err
		}
		return collectImageCandidates(root, 0), nil
	}

	events, err := parseServerSentJSONEvents(payload)
	if err != nil {
		return nil, err
	}
	var candidates []string
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		encoded, _ := json.Marshal(event.data)
		if message := readWrappedTaskError(encoded); message != "" {
			return nil, errors.New(message)
		}
		if strings.EqualFold(event.name, "error") {
			return nil, errors.New("图片流式接口返回错误")
		}
		candidates = append(candidates, collectImageCandidates(event.data, 0)...)
	}
	return candidates, nil
}

func isServerSentEventResponse(payload []byte, contentType string) bool {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return true
	}
	trimmed := bytes.TrimSpace(payload)
	return bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:"))
}

func parseServerSentJSONEvents(payload []byte) ([]serverSentJSONEvent, error) {
	normalized := strings.ReplaceAll(string(payload), "\r\n", "\n")
	events := make([]serverSentJSONEvent, 0)
	for _, block := range strings.Split(normalized, "\n\n") {
		name := ""
		dataLines := make([]string, 0)
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(data), &decoded); err != nil {
			return nil, err
		}
		events = append(events, serverSentJSONEvent{name: name, data: decoded})
	}
	if len(events) == 0 {
		return nil, errors.New("图片流式接口没有返回可解析事件")
	}
	return events, nil
}

func collectImageCandidates(value any, depth int) []string {
	if depth > 7 || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "data:image/") || looksLikeBase64(text) {
			return []string{text}
		}
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, collectImageCandidates(item, depth+1)...)
		}
		return result
	case map[string]any:
		keys := []string{"url", "b64_json", "partial_image_b64", "image_url", "image", "image_data", "base64", "result", "response", "data", "output", "candidates", "content", "parts", "inlineData"}
		var result []string
		for _, key := range keys {
			result = append(result, collectImageCandidates(typed[key], depth+1)...)
		}
		return result
	}
	return nil
}

func imageCandidateBytes(value string) ([]byte, string, error) {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		response, err := service.SafeProxyHTTPClient().Get(value)
		if err != nil {
			return nil, "", err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, "", errors.New(response.Status)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 32*1024*1024))
		if err != nil {
			return nil, "", err
		}
		mimeType := response.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		return data, strings.Split(mimeType, ";")[0], nil
	}
	if strings.HasPrefix(value, "data:image/") {
		parts := strings.SplitN(value, ",", 2)
		if len(parts) != 2 {
			return nil, "", errors.New("无效图片 data url")
		}
		mimeType := strings.TrimPrefix(strings.Split(strings.TrimPrefix(parts[0], "data:"), ";")[0], " ")
		data, err := base64.StdEncoding.DecodeString(parts[1])
		return data, mimeType, err
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, "", err
	}
	return data, http.DetectContentType(data), nil
}

func imageSize(data []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func taskTime() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func readCanvasTaskSources(r *http.Request) []string {
	values := r.URL.Query()["source"]
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			if strings.TrimSpace(item) != "" {
				result = append(result, strings.TrimSpace(item))
			}
		}
	}
	return result
}
