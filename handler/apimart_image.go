package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

// apimartImageGeminiImagePreviewUnsupported 列已确认被 apimart /images/generations 上游拒绝的
// gemini-*-image-preview 模型。上游错误信息：「not supported model for image generation,
// only imagen models are supported」。该项目将它们接到了 image gen，但 apimart 实际上不接受。
// 在路由到上游前拦截，节约一次无谓的 4xx+log（管理台日志里现在是 5 次失败记录）。
// 若 apimart 后续升级支持，把下列模型从这里去掉即可恢复。
var apimartImageGeminiImagePreviewUnsupported = []string{
	"gemini-3-pro-image-preview",
	"gemini-3.1-flash-image-preview",
}

func apimartImageModelUnsupportedByUpstream(modelName string) (string, bool) {
	// Extract the actual model name after channel prefix (e.g., "channel-xxx::gemini-3-pro-image-preview" -> "gemini-3-pro-image-preview")
	actualName := modelName
	if idx := strings.LastIndex(modelName, "::"); idx != -1 {
		actualName = modelName[idx+2:]
	} else if idx := strings.LastIndex(modelName, "/"); idx != -1 {
		actualName = modelName[idx+1:]
	}
	normalized := normalizeAPIMartModelName(actualName)
	for _, bad := range apimartImageGeminiImagePreviewUnsupported {
		badNormalized := normalizeAPIMartModelName(bad)
		if normalized == badNormalized {
			return bad, true
		}
	}
	return "", false
}

func normalizeAPIMartImageBody(body []byte, contentType string, modelName string, channel model.ModelChannel) ([]byte, string, error) {
	if bad, ok := apimartImageModelUnsupportedByUpstream(modelName); ok {
		return body, contentType, fmt.Errorf("apimart 渠道当前不接受 %q 做图像生成（上游 /images/generations 仅支持 imagen-* 模型，例如 imagen-3.0-generate-002 / imagen-4.0-generate-001）。请在 admin 系统设置→公开模型里把 %q 改用 imagen-* 系列或换一个 Imagen 系模型。", bad, bad)
	}
	payload, err := readAPIMartPayload(body, contentType, channel)
	if err != nil {
		return body, contentType, err
	}
	finalModel := strings.TrimSpace(modelName)
	if finalModel == "" {
		finalModel = strings.TrimSpace(toStringSafe(payload["model"]))
	}
	if finalModel != "" {
		payload["model"] = finalModel
	}
	normalizeAPIMartImageParams(payload, finalModel, channel)
	if errMessage := strings.TrimSpace(toStringSafe(payload["_apimart_reference_error"])); errMessage != "" {
		return body, contentType, errors.New(errMessage)
	}
	delete(payload, "_apimart_reference_error")

	encoded, err := json.Marshal(payload)
	if err != nil {
		return body, contentType, err
	}
	return encoded, "application/json", nil
}

func normalizeAPIMartImageParams(payload map[string]any, modelName string, channel model.ModelChannel) {
	config := apimartImageConfig(modelName)
	if config.aspectField == "" {
		config.aspectField = "size"
	}

	normalizeAPIMartResolution(payload, config)
	normalizeAPIMartAspect(payload, config)
	// Gemini 图片模型（nano-banana / gemini-2-5 / gemini-3 / imagen）的 aspectRatio 只接受
	// 1:1 / 3:4 / 4:3 / 9:16 / 16:9 五个值，其它宽高比（如 2:3、4:5、16:10、21:9）或 auto
	// 会被上游拒绝："unsupported image size; use a supported aspect ratio like 3:4 ..."。
	// 这里把 size 里的宽高比吸附到最近的可接受值。
	if apimartImageModelIsGemini(modelName) {
		if ratio := strings.TrimSpace(toStringSafe(payload["size"])); ratio != "" {
			payload["size"] = snapGeminiAspectRatio(ratio)
		}
	}
	normalizeAPIMartImageCount(payload, config)
	normalizeAPIMartImageQuality(payload, config)
	if apimartImageReferenceExcluded(modelName) {
		clearAPIMartImageReferenceFields(payload)
	} else {
		normalizeAPIMartReferenceInputs(payload, modelName, config, channel)
	}
	if err := validateAPIMartImageRequiredInputs(payload, modelName); err != nil {
		payload["_apimart_reference_error"] = err.Error()
	}
}

func apimartImageConfig(modelName string) apimartInputConfig {
	model := normalizeAPIMartModelName(modelName)
	config := apimartInputConfig{
		aspectField:    "size",
		hasResolution:  true,
		resolutionCase: "upper",
		hasCount:       true,
		imageRefField:  "image_urls",
		imageRefKind:   "array",
	}

	switch {
	case strings.Contains(model, "gpt-image-2") && strings.Contains(model, "official"):
		config.resolutionCase = "lower"
		config.hasQuality = true
		config.hasOutput = true
	case strings.Contains(model, "gpt-image-2"):
		config.resolutionCase = "lower"
		config.hasQuality = true
		config.hasOutput = false
	case strings.Contains(model, "gpt-4o-image"):
		config.hasResolution = false
	case strings.Contains(model, "gpt-image-1"):
		config.hasResolution = false
		config.hasQuality = true
		config.hasOutput = true
	case strings.Contains(model, "gemini-3-1-flash-lite"):
		config.resolutionCase = "upper"
		config.maxResolution = "1K"
	case strings.Contains(model, "gemini-3-1"), strings.Contains(model, "gemini-31"), strings.Contains(model, "nano-banana2"):
		config.resolutionCase = "upper"
		config.hasCount = false
	case strings.Contains(model, "gemini-3-pro"), strings.Contains(model, "nano-banana-pro"):
		config.resolutionCase = "upper"
		config.hasCount = false
	case strings.Contains(model, "gemini-2-5"), strings.Contains(model, "nano-banana"):
		config.resolutionCase = "upper"
		config.maxResolution = "1K"
		config.hasCount = false
	case strings.Contains(model, "imagen"):
		config.hasResolution = false
		config.hasQuality = false
		config.hasCount = false
		config.imageRefField = ""
	case strings.Contains(model, "seedream-5-0-pro"):
		config.resolutionCase = "upper"
		config.maxResolution = "2K"
		config.hasCount = false
		config.maxImageRefs = 10
	case strings.Contains(model, "seedream-5"):
		config.resolutionCase = "upper"
		config.minResolution = "2K"
		config.hasOutput = true
	case strings.Contains(model, "seedream-4-5"), strings.Contains(model, "seedance-4-5"):
		config.resolutionCase = "upper"
		config.minResolution = "2K"
	case strings.Contains(model, "seedream"), strings.Contains(model, "seedance-4"):
		config.resolutionCase = "upper"
	case strings.Contains(model, "qwen"):
		config.resolutionCase = "upper"
		config.maxResolution = "2K"
	case strings.Contains(model, "z-image"):
		config.resolutionCase = "upper"
		config.maxResolution = "2K"
		config.hasCount = false
		config.imageRefField = ""
	case strings.Contains(model, "grok-imagine"):
		config.hasResolution = false
	case strings.Contains(model, "wan2-7"), strings.Contains(model, "wan2.7"):
		config.resolutionCase = "upper"
	case strings.Contains(model, "flux-2"):
		config.resolutionCase = "upper"
		config.hasCount = false
	}
	return config
}

// apimartImageModelIsGemini 判断模型是否为 Gemini 系图片生成模型（nano-banana / gemini-2-5 /
// gemini-3 / imagen）。与 apimartImageConfig 中的 gemini 分支保持一致。
func apimartImageModelIsGemini(modelName string) bool {
	model := normalizeAPIMartModelName(modelName)
	return strings.Contains(model, "gemini-3-1-flash-lite") ||
		strings.Contains(model, "gemini-3-1") ||
		strings.Contains(model, "gemini-31") ||
		strings.Contains(model, "nano-banana2") ||
		strings.Contains(model, "gemini-3-pro") ||
		strings.Contains(model, "nano-banana-pro") ||
		strings.Contains(model, "gemini-2-5") ||
		strings.Contains(model, "nano-banana") ||
		strings.Contains(model, "imagen")
}

// geminiSupportedAspectRatios 是 Gemini 图片生成 API 允许的全部 aspectRatio。
var geminiSupportedAspectRatios = []struct {
	width  int
	height int
	ratio  string
}{
	{1, 1, "1:1"},
	{3, 4, "3:4"},
	{4, 3, "4:3"},
	{9, 16, "9:16"},
	{16, 9, "16:9"},
}

// snapGeminiAspectRatio 把任意宽高比/像素尺寸吸附到 Gemini 允许的 5 个值之一；
// auto 或无法识别时回退到 1:1。
func snapGeminiAspectRatio(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "auto" {
		return "1:1"
	}
	if width, height, ok := parseAPIMartSize(value); ok {
		if ratio := normalizeAPIMartSizeRatio(width, height); ratio != "" {
			value = ratio
		}
	}
	for _, item := range geminiSupportedAspectRatios {
		if value == item.ratio {
			return item.ratio
		}
	}
	parts := strings.Split(value, ":")
	if len(parts) == 2 {
		width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
		height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errW == nil && errH == nil && width > 0 && height > 0 {
			best := geminiSupportedAspectRatios[0].ratio
			bestScore := -1
			for _, item := range geminiSupportedAspectRatios {
				diff := width*item.height - height*item.width
				if diff < 0 {
					diff = -diff
				}
				score := diff * 100 / (width * item.height)
				if bestScore < 0 || score < bestScore {
					bestScore = score
					best = item.ratio
				}
			}
			return best
		}
	}
	return "1:1"
}

func normalizeAPIMartImageQuality(payload map[string]any, config apimartInputConfig) {
	if config.hasQuality {
		if value := strings.TrimSpace(toStringSafe(payload["quality"])); value != "" {
			payload["quality"] = strings.ToLower(value)
		}
	} else {
		delete(payload, "quality")
	}

	if config.hasOutput {
		value := firstNonEmpty(toStringSafe(payload["output_format"]), toStringSafe(payload["format"]))
		if strings.TrimSpace(value) != "" {
			payload["output_format"] = normalizeAPIMartOutputFormat(value)
		}
	} else {
		delete(payload, "output_format")
	}
	delete(payload, "format")
}

func apimartImageReferenceExcluded(modelName string) bool {
	switch normalizeAPIMartModelName(modelName) {
	case "grok-imagine-1-5-apimart", "imagen-4-0-apimart":
		return true
	default:
		return false
	}
}

func clearAPIMartImageReferenceFields(payload map[string]any) {
	for _, key := range []string{
		"image",
		"images",
		"image_url",
		"image_urls",
		"input_url",
		"input_urls",
		"input_reference",
		"input_reference[]",
		"image_input",
		"reference_image",
		"reference_images",
		"reference_image_url",
		"reference_image_urls",
		"first_frame_url",
		"first_frame_image",
		"last_frame_url",
		"last_frame_image",
	} {
		delete(payload, key)
	}
}

func validateAPIMartImageRequiredInputs(payload map[string]any, modelName string) error {
	model := normalizeAPIMartModelName(modelName)
	if strings.Contains(model, "grok-imagine") && strings.Contains(model, "edit") {
		return requireAPIMartAnyInput(payload, "image_urls")
	}
	return nil
}

func normalizeAPIMartOutputFormat(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "jpg":
		return "jpeg"
	case "jpeg", "png", "webp":
		return value
	default:
		return value
	}
}

func copyAPIMartImageResponse(w http.ResponseWriter, response *http.Response, request *http.Request, channel model.ModelChannel, logContext aiLogContext, onFailure func()) bool {
	if !strings.Contains(request.URL.Path, "/images/generations") && !strings.Contains(request.URL.Path, "/images/edits") {
		return false
	}

	payload, _ := io.ReadAll(response.Body)
	if imageURLs, ok := readAPIMartDirectImageURLs(payload); ok {
		writeAPIMartImagesResponse(w, response.StatusCode, imageURLs, logContext)
		return true
	}

	taskID, _, ok := readAPIMartCreateTask(payload)
	if !ok {
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(payload)
		saveAIProxyLog(logContext, response.StatusCode, string(payload), "")
		return true
	}

	imageURLs, errorMessage := pollAPIMartImageTask(request, channel, taskID)
	if errorMessage != "" {
		if onFailure != nil {
			onFailure()
		}
		writeAPIMartImageError(w, response.StatusCode, errorMessage, logContext)
		return true
	}
	writeAPIMartImagesResponse(w, response.StatusCode, imageURLs, logContext)
	return true
}

func pollAPIMartImageTask(request *http.Request, channel model.ModelChannel, taskID string) ([]string, string) {
	pollURL := buildAPIMartTaskURL(channel, taskID)
	for attempt := 0; attempt < 300; attempt++ {
		if attempt > 0 {
			select {
			case <-request.Context().Done():
				return nil, request.Context().Err().Error()
			case <-time.After(2 * time.Second):
			}
		}

		pollRequest, err := http.NewRequestWithContext(request.Context(), http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, err.Error()
		}
		pollRequest.Header.Set("Authorization", "Bearer "+channel.APIKey)
		response, err := service.HTTPClientForChannel(channel).Do(pollRequest)
		if err != nil {
			return nil, err.Error()
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512*1024))
		_ = response.Body.Close()
		if response.StatusCode >= http.StatusBadRequest {
			return nil, readUpstreamAIErrorMessage(body, response.StatusCode)
		}
		imageURLs, done, errorMessage := readAPIMartImageTaskResult(body)
		if errorMessage != "" {
			return nil, errorMessage
		}
		if done {
			if len(imageURLs) == 0 {
				return nil, "APIMart image task completed but returned no image URL"
			}
			return imageURLs, ""
		}
	}
	return nil, "APIMart image task timed out"
}

func readAPIMartImageTaskResult(payload []byte) ([]string, bool, string) {
	var result struct {
		Code int `json:"code"`
		Data struct {
			Status string         `json:"status"`
			Result map[string]any `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, false, err.Error()
	}
	if result.Code != 200 {
		return nil, false, firstNonEmpty(result.Msg, "APIMart image task query failed")
	}

	imageURLs := extractAPIMartImageURLs(result.Data.Result)
	if len(imageURLs) > 0 {
		return imageURLs, true, ""
	}
	switch normalizeAPIMartTaskStatus(result.Data.Status) {
	case "completed":
		return imageURLs, true, ""
	case "failed":
		if result.Data.Error != nil && strings.TrimSpace(result.Data.Error.Message) != "" {
			return nil, false, result.Data.Error.Message
		}
		return nil, false, firstNonEmpty(result.Msg, "APIMart image task failed")
	default:
		return nil, false, ""
	}
}

func extractAPIMartImageURLs(result map[string]any) []string {
	if result == nil {
		return nil
	}
	values := collectAPIMartURLs(result, 0)
	seen := map[string]bool{}
	urls := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] || !(strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")) {
			continue
		}
		seen[value] = true
		urls = append(urls, value)
	}
	return urls
}

func collectAPIMartURLs(value any, depth int) []string {
	if depth > 6 || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
			return []string{text}
		}
		var parsed any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			return collectAPIMartURLs(parsed, depth+1)
		}
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, collectAPIMartURLs(item, depth+1)...)
		}
		return result
	case map[string]any:
		var result []string
		for _, key := range []string{"images", "image", "url", "urls", "image_url", "imageUrl", "download_url", "downloadUrl", "data", "result"} {
			result = append(result, collectAPIMartURLs(typed[key], depth+1)...)
		}
		return result
	}
	return nil
}

func readAPIMartDirectImageURLs(payload []byte) ([]string, bool) {
	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &result) != nil || len(result.Data) == 0 {
		return nil, false
	}
	urls := make([]string, 0, len(result.Data))
	for _, item := range result.Data {
		if strings.TrimSpace(item.URL) != "" {
			urls = append(urls, strings.TrimSpace(item.URL))
		}
	}
	return urls, len(urls) > 0
}

func writeAPIMartImagesResponse(w http.ResponseWriter, statusCode int, imageURLs []string, logContext aiLogContext) {
	items := make([]map[string]any, 0, len(imageURLs))
	for _, imageURL := range imageURLs {
		items = append(items, map[string]any{"url": imageURL})
	}
	converted := map[string]any{
		"created": time.Now().Unix(),
		"data":    items,
	}
	encoded, err := json.Marshal(converted)
	if err != nil {
		writeAPIMartImageError(w, statusCode, err.Error(), logContext)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(encoded)
	saveAIProxyLog(logContext, statusCode, string(encoded), "")
}

func writeAPIMartImageError(w http.ResponseWriter, statusCode int, message string, logContext aiLogContext) {
	if statusCode < http.StatusBadRequest {
		statusCode = http.StatusBadGateway
	}
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
		},
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
	saveAIProxyLog(logContext, statusCode, string(body), message)
}
