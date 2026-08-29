package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

const userModelChannelHeader = "X-User-Model-Channel-ID"

// requestIDHeader 透传请求幂等键的 HTTP header（2026-08-17 引入）。
//
// 前端在发起图片/文本/音频请求时把 clientTaskId（例如 `client_image_task_<nanoid>`）放到
// X-Request-Id header；后端 proxyAIRequest 把它当 ConsumeUserBalanceWithHold 的 requestID，
// 同一 clientTaskId 重复提交 → 复用 hold 不再扣款，彻底避免网络重试双扣。
const requestIDHeader = "X-Request-Id"

// readRequestID 读 X-Request-Id header；缺失退到 uuid.NewString()（保底，保证幂等键永远非空）。
func readRequestID(r *http.Request) string {
	if r == nil {
		return uuid.NewString()
	}
	if id := strings.TrimSpace(r.Header.Get(requestIDHeader)); id != "" {
		return id
	}
	return uuid.NewString()
}

func selectAIRequestChannel(user model.AuthUser, modelName string, channelID string, userChannelID string) (model.ModelChannel, string, error) {
	userChannelID = strings.TrimSpace(userChannelID)
	if userChannelID != "" {
		channel, err := service.SelectUserLocalModelChannelForModel(user.ID, modelName, userChannelID)
		return channel, userChannelID, err
	}
	if !service.UserCanUseRemoteModelChannel(user) {
		return model.ModelChannel{}, "", fmt.Errorf("当前账号未开放云端渠道")
	}
	channel, err := service.SelectModelChannelForModel(modelName, channelID)
	return channel, "", err
}

func failAIChannelSelect(w http.ResponseWriter, err error, fallback string) {
	message := strings.TrimSpace(err.Error())
	switch message {
	case "当前账号未开放云端渠道", "请先登录", "缺少模型名称", "缺少模型渠道", "本地渠道不存在", "本地渠道配置不完整", "本地渠道不支持该模型", "指定模型渠道不可用":
		Fail(w, message)
	default:
		Fail(w, fallback)
	}
}

func AIImagesGenerations(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/generations")
}

func AIImagesEdits(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/edits")
}

func AIChatCompletions(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/chat/completions")
}

func AIResponses(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/responses")
}

func AIVideos(w http.ResponseWriter, r *http.Request) {
	proxyAIVideoTaskRequest(w, r)
}

func AIVideo(w http.ResponseWriter, r *http.Request, id string) {
	if serveAIVideoTask(w, r, id) {
		return
	}
	if isClientVideoTaskID(id) {
		OK(w, map[string]any{"id": id, "task_id": id, "object": "video", "status": "queued", "progress": 0})
		return
	}
	proxyAIGetRequest(w, r, "/videos/"+id)
}

func AIVideoContent(w http.ResponseWriter, r *http.Request, id string) {
	proxyAIGetRequest(w, r, "/videos/"+id+"/content")
}

func AIAudioSpeech(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/audio/speech")
}

func proxyAIGetRequest(w http.ResponseWriter, r *http.Request, path string) {
	startedAt := time.Now()
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	modelName := r.URL.Query().Get("model")
	if strings.TrimSpace(modelName) == "" {
		modelName = "Agnes-Video-V2.0"
	}
	channel, _, err := selectAIRequestChannel(user, modelName, r.Header.Get("X-Model-Channel-ID"), r.Header.Get(userModelChannelHeader))
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		failAIChannelSelect(w, err, "AI 接口请求失败")
		return
	}
	upstreamPath := resolveAIProxyPath(channel, modelName, path)
	request, err := http.NewRequest(http.MethodGet, resolveAIProxyURL(channel, modelName, upstreamPath), nil)
	if err != nil {
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	copyAIResponse(w, request, channel, aiLogContext{StartedAt: startedAt, Endpoint: path, Method: http.MethodGet, Model: modelName, Channel: channel, UserID: user.ID, UserDisplayName: firstNonEmpty(user.DisplayName, user.Username), RequestBody: summarizeQueryParams(r.URL.Query())}, nil)
}

func proxyAIRequest(w http.ResponseWriter, r *http.Request, path string) {
	startedAt := time.Now()
	body, contentType, modelName, err := readAIRequest(r)
	if err != nil {
		log.Printf("AI proxy request read failed: %v", err)
		Fail(w, "AI 接口请求失败")
		return
	}
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	// 供应商代理分发：若用户激活了第三方供应商账户且本次是图片请求，走适配器（用户自购额度，不进官方渠道/积分）。
	if handled := dispatchVendorProxy(w, r, path, user, body, contentType, modelName); handled {
		return
	}
	channel, userChannelID, err := selectAIRequestChannel(user, modelName, r.Header.Get("X-Model-Channel-ID"), r.Header.Get(userModelChannelHeader))
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		failAIChannelSelect(w, err, "AI 接口请求失败")
		return
	}
	cents := 0
	if userChannelID == "" {
		modelCost, modelCostErr := service.ModelCost(modelName)
		if modelCostErr != nil {
			// 价格未配置：直接拒绝（防 0 元白嫖）。本地/免费渠道走 userChannelID != "" 分支不命中这里。
			log.Printf("AI proxy read model cost failed: model=%s err=%v", modelName, modelCostErr)
			FailWithStatus(w, http.StatusBadRequest, "该模型暂未配置价格，请联系管理员或换一个模型")
			return
		}
		// 图片/文本等非视频模型：按 账户余额 乘以生成数量 count
		// 视频模型（KIE/APIMart 视频路径也走 handler/video_task.go 不在这里）：默认 per_call
		if modelCost.Unit == model.ModelCostUnitPerSecond && modelCost.CostCentsPerSecond > 0 {
			seconds := readVideoSecondsFromBody(body, contentType)
			if seconds <= 0 {
				seconds = 1
			}
			cents = modelCost.CostCentsPerSecond * seconds * readAIRequestCount(body, contentType)
		} else {
			cents = modelCost.CostCents * readAIRequestCount(body, contentType)
		}
		// 兜底：配置存在但金额为 0（手工配了 0 元或自动价 0 元）也拒绝，
		// 防止 admin 误配或上游定价 0 元导致免费用付费云端渠道。
		if cents <= 0 {
			log.Printf("AI proxy rejected zero-cost paid channel: model=%s", modelName)
			FailWithStatus(w, http.StatusBadRequest, "该模型当前价格为 0 元，请联系管理员核对价格配置")
			return
		}
	}
	upstreamPath := resolveAIProxyPath(channel, modelName, path)
	if service.IsMiMoTTSModelName(modelName) && path == "/audio/speech" {
		body, contentType, err = normalizeMiMoTTSBody(body, contentType, modelName)
		if err != nil {
			log.Printf("AI proxy normalize MiMo TTS request failed: model=%s err=%v", modelName, err)
			Fail(w, "请求参数解析失败")
			return
		}
	} else if isKIEChannel(channel, modelName) && upstreamPath == "/jobs/createTask" {
		body, contentType, err = normalizeKIEVideoBody(body, contentType, modelName, channel)
		if err != nil {
			log.Printf("AI proxy normalize KIE request failed: model=%s err=%v", modelName, err)
			Fail(w, "请求参数解析失败")
			return
		}
	} else if isAPIMartChannel(channel, modelName) && upstreamPath == "/videos/generations" {
		body, contentType, err = normalizeAPIMartVideoBody(body, contentType, modelName, channel)
		if err != nil {
			log.Printf("AI proxy normalize APIMart video request failed: model=%s err=%v", modelName, err)
			Fail(w, "请求参数解析失败")
			return
		}
	} else if upstreamPath == "/images/generations" || upstreamPath == "/images/edits" {
		// 通用 snap：所有发到 /images/generations 的请求里，gemini-* / nano-banana / imagen 系模型
		// 的 size 只接受 1:1 / 3:4 / 4:3 / 9:16 / 16:9 五个值；非 apimart 渠道（rolldek/gemini-direct）
		// 也吃这个限制。原 snap 逻辑只在 apimart 渠道内（apimart_image.go:85-89）跑，
		// 但 rolldek 同样报 400 "unsupported image size"——必须提到 ai.go 通用层。
		// 仅处理 JSON body（multipart 走 apimart 渠道的 ReadForm 路径）。
		if !strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") && apimartImageModelIsGemini(modelName) {
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err == nil {
				if size, _ := payload["size"].(string); strings.TrimSpace(size) != "" {
					snapped := snapGeminiAspectRatio(size)
					if snapped != size {
						if newBody, err := json.Marshal(payload); err == nil {
							body = newBody
							log.Printf("AI proxy snapped gemini aspect ratio: model=%s from=%q to=%q", modelName, size, snapped)
						}
					}
				}
			}
		}
		// Pre-check: only block unsupported gemini-*-image-preview when sending to apimart upstream.
		// Other channels (rolldek/gemini-direct) have their own behaviour and must NOT be
		// intercepted here. (2026-08-24 fix: previously this if had no isAPIMartChannel guard,
		// causing every /images/generations request to be rejected with the "apimart 不接受" message.)
		if isAPIMartChannel(channel, modelName) {
			if bad, blocked := apimartImageModelUnsupportedByUpstream(modelName); blocked {
				errMsg := fmt.Sprintf("apimart 渠道当前不接受 %q 做图像生成（上游 /images/generations 仅支持 imagen-* 模型，例如 imagen-3.0-generate-002 / imagen-4.0-generate-001）。请在 admin 系统设置→公开模型里把 %q 改用 imagen-* 系列或换一个 Imagen 系模型。", bad, bad)
				log.Printf("AI proxy blocked unsupported model: model=%s reason=%s", modelName, errMsg)
				Fail(w, errMsg)
				return
			}
			body, contentType, err = normalizeAPIMartImageBody(body, contentType, modelName, channel)
			if err != nil {
				log.Printf("AI proxy normalize APIMart image request failed: model=%s err=%v", modelName, err)
				Fail(w, "请求参数解析失败")
				return
			}
		}
	}
	request, err := http.NewRequest(http.MethodPost, service.BuildModelChannelURL(channel, upstreamPath), bytes.NewReader(body))
	if err != nil {
		log.Printf("AI proxy build request failed: url=%s err=%v", service.BuildModelChannelURL(channel, upstreamPath), err)
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	var holdID string
	if cents > 0 {
		holdID, err = service.ConsumeUserBalanceWithHold(user.ID, modelName, cents, upstreamPath, readRequestID(r))
		if err != nil {
			FailError(w, err)
			return
		}
	}
	copyAIResponse(w, request, channel, aiLogContext{
		StartedAt:       startedAt,
		Endpoint:        path,
		Method:          http.MethodPost,
		Model:           modelName,
		Channel:         channel,
		UserID:          user.ID,
		UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
		CostCents:         cents,
		RequestBody:     summarizeAIRequest(body, contentType),
		HoldID:          holdID,
	}, nil)
}

type aiLogContext struct {
	StartedAt       time.Time
	Endpoint        string
	Method          string
	Model           string
	Channel         model.ModelChannel
	UserID          string
	UserDisplayName string
	CostCents int
	RequestBody     string
	HoldID          string // BalanceHold ID（2026-08-17 引入，copyAIResponse 用它做 settle/cancel 闭环）
}

func copyAIResponse(w http.ResponseWriter, request *http.Request, channel model.ModelChannel, logContext aiLogContext, onFailure func()) {
	response, err := service.HTTPClientForChannel(channel).Do(request)
	if err != nil {
		log.Printf("AI proxy request failed: url=%s err=%v", request.URL.String(), err)
		holdCancel(logContext.HoldID, "request failed")
		if onFailure != nil {
			onFailure()
		}
		saveAIProxyLog(logContext, 0, "", err.Error())
		Fail(w, "AI 接口请求失败")
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 256*1024))
		log.Printf("AI upstream error: url=%s status=%d body=%s", request.URL.String(), response.StatusCode, strings.TrimSpace(string(payload)))
		holdCancel(logContext.HoldID, "upstream status>=400")
		if onFailure != nil {
			onFailure()
		}
		saveAIProxyLog(logContext, response.StatusCode, string(payload), strings.TrimSpace(string(payload)))
		// 透传上游原始 body 到响应 header，canvas image task 用它把 raw error body 落库到 task.ErrorDetail，
		// 避免被 Fail() 信封二次包装后丢失细节。前端不解析这个 header，无副作用。
		// 限制 16KB 后再 base64 保险：超过这个长度的 raw body 一般是堆栈/binary，调试价值有限。
		if len(payload) > 0 && len(payload) <= 16*1024 {
			w.Header().Set("X-Upstream-Body-Base64", base64.StdEncoding.EncodeToString(payload))
		}
		Fail(w, readUpstreamAIErrorMessage(payload, response.StatusCode))
		return
	}

	wrappedOnFailure := func() {
		holdCancel(logContext.HoldID, "sub-process failure")
		if onFailure != nil {
			onFailure()
		}
	}
	if copyMiMoTTSResponse(w, response, logContext, wrappedOnFailure) {
		holdSettle(logContext.HoldID)
		return
	}
	if copyKIEVideoResponse(w, response, request, channel, logContext, wrappedOnFailure) {
		holdSettle(logContext.HoldID)
		return
	}
	if isAPIMartChannel(channel, logContext.Model) {
		if copyAPIMartImageResponse(w, response, request, channel, logContext, wrappedOnFailure) {
			holdSettle(logContext.HoldID)
			return
		}
		// APIMart video 子函数不接 onFailure，自身处理失败时 return false → 我们 settle（它已经写完响应）
		if copyAPIMartVideoResponse(w, response, request, channel, logContext) {
			holdSettle(logContext.HoldID)
			return
		}
	}

	for key, values := range response.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	responseBody := copyAIResponseBody(w, response.Body)
	saveAIProxyLog(logContext, response.StatusCode, responseBody, "")
	holdSettle(logContext.HoldID)
}

// holdSettle 成功结算 hold（业务成功 → 标记完成）。holdID 为空 → no-op。
func holdSettle(holdID string) {
	if strings.TrimSpace(holdID) == "" {
		return
	}
	if err := service.SettleBalanceHold(holdID); err != nil {
		log.Printf("settle balance hold failed: holdID=%s err=%v", holdID, err)
	}
}

// holdCancel 失败取消 hold（业务失败 → 退余额）。holdID 为空 → no-op。
func holdCancel(holdID, reason string) {
	if strings.TrimSpace(holdID) == "" {
		return
	}
	if err := service.CancelBalanceHold(holdID); err != nil {
		log.Printf("cancel balance hold failed: holdID=%s reason=%s err=%v", holdID, reason, err)
	}
}

func copyAIResponseBody(w http.ResponseWriter, body io.Reader) string {
	flusher, canFlush := w.(http.Flusher)
	buffer := make([]byte, 32*1024)
	var logBuffer strings.Builder
	for {
		n, err := body.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return logBuffer.String()
			}
			if logBuffer.Len() < 64*1024 {
				_, _ = logBuffer.Write(buffer[:min(n, 64*1024-logBuffer.Len())])
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return logBuffer.String()
		}
	}
}

func saveAIProxyLog(context aiLogContext, status int, responseBody string, errorMessage string) {
	if context.StartedAt.IsZero() {
		context.StartedAt = time.Now()
	}
	service.SaveAICallLog(service.AICallLogInput{
		UserID:          context.UserID,
		UserDisplayName: context.UserDisplayName,
		Endpoint:        context.Endpoint,
		Method:          context.Method,
		Model:           context.Model,
		ChannelID:       context.Channel.ID,
		ChannelName:     context.Channel.Name,
		Status:          status,
		DurationMs:      time.Since(context.StartedAt).Milliseconds(),
		CostCents:         context.CostCents,
		RequestBody:     context.RequestBody,
		ResponseBody:    responseBody,
		Error:           errorMessage,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func summarizeAIRequest(body []byte, contentType string) string {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return summarizeMultipartAIRequest(body, contentType)
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err == nil {
		redactLargeImages(&payload)
		if encoded, err := json.MarshalIndent(payload, "", "  "); err == nil {
			return string(encoded)
		}
	}
	return string(body)
}

func summarizeQueryParams(values map[string][]string) string {
	if len(values) == 0 {
		return ""
	}
	encoded, _ := json.MarshalIndent(values, "", "  ")
	return string(encoded)
}

func summarizeMultipartAIRequest(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "multipart/form-data"
	}
	form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
	if err != nil {
		return "multipart/form-data"
	}
	defer form.RemoveAll()
	summary := map[string]any{"fields": form.Value}
	files := []map[string]any{}
	for field, headers := range form.File {
		for _, header := range headers {
			files = append(files, map[string]any{"field": field, "filename": header.Filename, "size": header.Size, "contentType": header.Header.Get("Content-Type")})
		}
	}
	summary["files"] = files
	encoded, _ := json.MarshalIndent(summary, "", "  ")
	return string(encoded)
}

func readUpstreamAIErrorMessage(body []byte, statusCode int) string {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return payload.Error.Message
		}
		if strings.TrimSpace(payload.Msg) != "" {
			return payload.Msg
		}
		if strings.TrimSpace(payload.Message) != "" {
			return payload.Message
		}
	}
	if statusCode > 0 {
		// 上游返回体没有可读文案时，按状态码给出更易懂的中文说明，避免用户只看到裸状态码。
		switch statusCode {
		case http.StatusTooManyRequests: // 429
			return "当前模型渠道被上游限流或额度不足（429）。同一渠道下换模型仍会共用同一 API Key，请稍后重试，或在 /admin/settings 检查该渠道的额度与 API Key。"
		case http.StatusUnauthorized, http.StatusForbidden: // 401 / 403
			return fmt.Sprintf("上游渠道鉴权失败（%d），请在 /admin/settings 检查该渠道的 API Key 是否正确/有效。", statusCode)
		case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout: // 503 / 502 / 504
			return fmt.Sprintf("上游模型服务暂时不可用或繁忙（%d），请稍后重试。", statusCode)
		default:
			return fmt.Sprintf("AI 接口请求失败：%d", statusCode)
		}
	}
	return "AI 接口请求失败"
}

func redactLargeImages(value *any) {
	switch typed := (*value).(type) {
	case map[string]any:
		for key, item := range typed {
			if text, ok := item.(string); ok && (strings.HasPrefix(text, "data:image/") || strings.HasPrefix(text, "data:audio/") || len(text) > 2048 && looksLikeBase64(text)) {
				typed[key] = fmt.Sprintf("[redacted media/string len=%d]", len(text))
				continue
			}
			redactLargeImages(&item)
			typed[key] = item
		}
	case []any:
		for index, item := range typed {
			redactLargeImages(&item)
			typed[index] = item
		}
	}
}

func looksLikeBase64(value string) bool {
	for _, char := range value[:min(len(value), 200)] {
		if !(char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '+' || char == '/' || char == '=') {
			return false
		}
	}
	return true
}

// aiRequestBodyLimit 限制 AI 代理请求体大小（含 multipart 图片上传）。
// 与 nginx client_max_body_size 50m 对齐，留余量给 base64 编码膨胀。
const aiRequestBodyLimit = 64 << 20 // 64MB

func readAIRequest(r *http.Request) ([]byte, string, string, error) {
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(io.LimitReader(r.Body, aiRequestBodyLimit+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(body) > aiRequestBodyLimit {
		return nil, "", "", errors.New("请求体超过大小限制")
	}
	modelName := ""
	if strings.HasPrefix(contentType, "multipart/form-data") {
		modelName = readMultipartModel(body, contentType)
	} else {
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &payload)
		modelName = payload.Model
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, "", "", errMissingModel
	}
	return body, contentType, modelName, nil
}

func readMultipartModel(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return ""
	}
	defer form.RemoveAll()
	if values := form.Value["model"]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func readAIRequestCount(body []byte, contentType string) int {
	count := 1
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return count
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return count
		}
		defer form.RemoveAll()
		if values := form.Value["n"]; len(values) > 0 {
			_, _ = fmt.Sscan(values[0], &count)
		}
	} else {
		var payload struct {
			N int `json:"n"`
		}
		_ = json.Unmarshal(body, &payload)
		count = payload.N
	}
	if count < 1 {
		return 1
	}
	// 防止用户传入超大 n 值导致扣费倍数爆炸或上游 API 过载。
	if count > 20 {
		count = 20
	}
	return count
}

// readVideoSecondsFromBody 解析视频请求 body 中的"秒数/时长"字段。
// 兼容多种常见命名：seconds / duration / second / videoSeconds / time / length
func readVideoSecondsFromBody(body []byte, contentType string) int {
	seconds := 0
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return 0
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return 0
		}
		defer form.RemoveAll()
		for _, key := range []string{"seconds", "duration", "second", "videoSeconds", "time", "length"} {
			if values := form.Value[key]; len(values) > 0 {
				var v int
				if _, sErr := fmt.Sscan(values[0], &v); sErr == nil && v > 0 {
					if v > 60 {
						v = 60
					}
					return v
				}
			}
		}
		return 0
	}
	var payload struct {
		Seconds      int `json:"seconds"`
		Second       int `json:"second"`
		Duration     int `json:"duration"`
		VideoSeconds int `json:"videoSeconds"`
		Time         int `json:"time"`
		Length       int `json:"length"`
	}
	_ = json.Unmarshal(body, &payload)
	for _, v := range []int{payload.Seconds, payload.Second, payload.Duration, payload.VideoSeconds, payload.Time, payload.Length} {
		if v > 0 {
			seconds = v
			break
		}
	}
	// 防止用户传入超大秒数导致扣费倍数爆炸（视频通常 ≤ 60 秒）。
	if seconds > 60 {
		seconds = 60
	}
	return seconds
}

func resolveAIProxyURL(channel model.ModelChannel, modelName string, path string) string {
	if videoID, ok := agnesVideoQueryID(modelName, path); ok {
		baseURL := strings.TrimRight(strings.TrimSpace(channel.BaseURL), "/")
		if strings.HasSuffix(strings.ToLower(baseURL), "/v1") {
			baseURL = strings.TrimRight(baseURL[:len(baseURL)-len("/v1")], "/")
		}
		values := url.Values{}
		values.Set("video_id", videoID)
		values.Set("model_name", modelName)
		return baseURL + "/agnesapi?" + values.Encode()
	}
	return service.BuildModelChannelURL(channel, path)
}

func agnesVideoQueryID(modelName string, path string) (string, bool) {
	if !isAgnesVideoModel(modelName) || !strings.HasPrefix(path, "/videos/") || strings.HasSuffix(path, "/content") {
		return "", false
	}
	id := strings.TrimPrefix(path, "/videos/")
	if strings.HasPrefix(id, "video_") {
		return id, true
	}
	return "", false
}

func resolveAIProxyPath(channel model.ModelChannel, modelName string, path string) string {
	if service.IsMiMoTTSModelName(modelName) && path == "/audio/speech" {
		return "/chat/completions"
	}
	if isKIEChannel(channel, modelName) {
		if path == "/videos" || path == "/images/generations" || path == "/images/edits" {
			return "/jobs/createTask"
		}
		if strings.HasPrefix(path, "/videos/") && !strings.HasSuffix(path, "/content") {
			taskID := strings.TrimSpace(strings.TrimPrefix(path, "/videos/"))
			if taskID != "" && !strings.Contains(taskID, "/") {
				return "/jobs/recordInfo?taskId=" + url.QueryEscape(taskID)
			}
		}
		return path
	}
	if isAPIMartChannel(channel, modelName) {
		if path == "/videos" {
			return "/videos/generations"
		}
		if path == "/images/edits" {
			model := normalizeAPIMartModelName(modelName)
			if strings.Contains(model, "grok-imagine") && strings.Contains(model, "edit") {
				return path
			}
			return "/images/generations"
		}
		if strings.HasPrefix(path, "/videos/") && !strings.HasSuffix(path, "/content") {
			taskID := strings.TrimSpace(strings.TrimPrefix(path, "/videos/"))
			if taskID != "" && !strings.Contains(taskID, "/") {
				return "/tasks/" + url.PathEscape(taskID) + "?language=zh"
			}
		}
		return path
	}
	if strings.EqualFold(strings.TrimSpace(modelName), "grok-imagine-video") && path == "/videos" {
		return "/videos/generations"
	}
	if isArkSeedanceVideo(channel.BaseURL, modelName) {
		if path == "/videos" {
			return "/contents/generations/tasks"
		}
		if strings.HasPrefix(path, "/videos/") && !strings.HasSuffix(path, "/content") {
			return "/contents/generations/tasks/" + strings.TrimPrefix(path, "/videos/")
		}
	}
	return path
}

// isArkSeedanceVideo 判定"火山方舟 Agent Plan 的 seedance 视频通道"。
// 必须同时满足：baseURL 含 /api/plan/v3（方舟 ark 协议端点）+ 模型名含 seedance。
// 单看模型名（之前仅此判断）会把任意"上游"渠道（rolldek 等非 ark 中转）的 seedance
// 也重写到 /contents/generations/tasks，导致转发到错误端点。已收紧为双条件。
func isArkSeedanceVideo(baseURL string, modelName string) bool {
	base := strings.ToLower(baseURL)
	model := strings.ToLower(modelName)
	arkBase := strings.Contains(base, "/api/plan/v3")
	seedance := strings.Contains(model, "seedance") || strings.Contains(model, "doubao-seedance")
	return arkBase && seedance
}

func isAgnesVideoModel(modelName string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "agnes-video")
}

var errMissingModel = &aiError{"缺少模型名称"}

type aiError struct {
	message string
}

func (err *aiError) Error() string {
	return err.message
}
