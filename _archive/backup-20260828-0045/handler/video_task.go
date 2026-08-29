package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
	"github.com/google/uuid"
)

func StartVideoTaskPoller() {
	service.StartVideoTaskPoller(pollVideoTaskFromUpstream)
}

func UserVideoTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	tasks, err := service.ListUserVideoTasks(user.ID, "video-workbench", 100)
	if err != nil {
		log.Printf("list video tasks failed: user=%s err=%v", user.ID, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, tasks)
}

func DeleteUserVideoTask(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		Fail(w, "视频任务不存在")
		return
	}
	if err := service.DeleteUserVideoTask(user.ID, id); err != nil {
		log.Printf("delete video task failed: user=%s id=%s err=%v", user.ID, id, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	OK(w, map[string]any{"deleted": true})
}

func proxyAIVideoTaskRequest(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	body, contentType, modelName, err := readAIRequest(r)
	if err != nil {
		log.Printf("AI video request read failed: %v", err)
		Fail(w, "AI 接口请求失败")
		return
	}
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	channel, userChannelID, err := selectAIRequestChannel(user, modelName, r.Header.Get("X-Model-Channel-ID"), r.Header.Get(userModelChannelHeader))
	if err != nil {
		log.Printf("AI video select channel failed: model=%s err=%v", modelName, err)
		failAIChannelSelect(w, err, "AI 接口请求失败")
		return
	}
	cents := 0
	if userChannelID == "" {
		modelCost, modelCostErr := service.ModelCost(modelName)
		if modelCostErr != nil {
			log.Printf("AI video read model cost failed: model=%s err=%v", modelName, modelCostErr)
			Fail(w, "AI 接口请求失败")
			return
		}
		count := readAIRequestCount(body, contentType)
		// 视频模型扣费两种模式：
		//   - per_second（按秒）：CostCentsPerSecond * 视频秒数 * 生成个数
		//   - per_call  （按次）：账户余额 * 生成个数（默认）
		if modelCost.Unit == model.ModelCostUnitPerSecond && modelCost.CostCentsPerSecond > 0 {
			seconds := readVideoSecondsFromBody(body, contentType)
			if seconds <= 0 {
				seconds = 1
			}
			cents = modelCost.CostCentsPerSecond * seconds * count
		} else {
			cents = modelCost.CostCents * count
		}
	}
	upstreamPath := resolveAIProxyPath(channel, modelName, "/videos")
	body, contentType, err = normalizeVideoCreateBody(body, contentType, modelName, channel, upstreamPath)
	if err != nil {
		log.Printf("AI video normalize request failed: model=%s err=%v", modelName, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	// PR-7：透传 r.Context()，客户端断网/取消时立刻中断上游调用，避免继续扣额度。
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, service.BuildModelChannelURL(channel, upstreamPath), bytes.NewReader(body))
	if err != nil {
		log.Printf("AI video build request failed: url=%s err=%v", service.BuildModelChannelURL(channel, upstreamPath), err)
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	logContext := aiLogContext{
		StartedAt:       startedAt,
		Endpoint:        "/videos",
		Method:          http.MethodPost,
		Model:           modelName,
		Channel:         channel,
		UserID:          user.ID,
		UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
		CostCents:         cents,
		RequestBody:     summarizeAIRequest(body, contentType),
	}
	holdID := ""
	if cents > 0 {
		holdID, err = service.ConsumeUserBalanceWithHold(user.ID, modelName, cents, upstreamPath, readClientVideoTaskIDOrRequestID(r))
		if err != nil {
			FailError(w, err)
			return
		}
	}
	payload, status, err := doAIRequest(request, channel)
	if err != nil {
		if cents > 0 {
			cancelVideoHold(holdID)
		}
		saveAIProxyLog(logContext, 0, "", err.Error())
		Fail(w, "AI 接口请求失败")
		return
	}
	if status >= http.StatusBadRequest {
		message := readUpstreamAIErrorMessage(payload, status)
		if cents > 0 {
			cancelVideoHold(holdID)
		}
		saveAIProxyLog(logContext, status, string(payload), strings.TrimSpace(string(payload)))
		Fail(w, message)
		return
	}
	transformed := transformVideoCreatePayload(payload, request, channel, modelName)
	if message := readVideoCreateErrorMessage(payload, transformed, channel, modelName); message != "" {
		if cents > 0 {
			cancelVideoHold(holdID)
		}
		saveAIProxyLog(logContext, status, string(payload), message)
		Fail(w, message)
		return
	}
	parsed := parseVideoTaskPayload(transformed, modelName)
	if parsed.UpstreamTaskID == "" && parsed.UpstreamVideoID == "" {
		if cents > 0 {
			cancelVideoHold(holdID)
		}
		saveAIProxyLog(logContext, status, string(transformed), "视频接口没有返回任务 ID")
		Fail(w, "视频接口没有返回任务 ID")
		return
	}
	task, err := service.CreateVideoTask(service.VideoTaskCreateInput{
		UserID:          user.ID,
		UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
		Model:           modelName,
		ChannelID:       channel.ID,
		UserChannelID:   userChannelID,
		ChannelName:     channel.Name,
		Source:          readVideoTaskSource(r),
		SourceID:        readVideoTaskSourceID(r),
		ClientTaskID:     readClientVideoTaskID(r),
		UpstreamTaskID:  parsed.UpstreamTaskID,
		UpstreamVideoID: parsed.UpstreamVideoID,
		Status:          parsed.Status,
		Progress:        parsed.Progress,
		Seconds:         parsed.Seconds,
		Size:            parsed.Size,
		VideoURL:        parsed.VideoURL,
		Error:           parsed.Error,
		ErrorDetail:     parsed.ErrorDetail,
		RequestBody:     logContext.RequestBody,
		ResponseBody:    string(transformed),
		CostCents:         cents,
		HoldID:            holdID,
	})
	if err != nil {
		log.Printf("save video task failed: model=%s err=%v", modelName, err)
		cancelVideoHold(holdID)
		Fail(w, "AI 接口请求失败")
		return
	}
	// 任务已成功入库 → 业务已接受（"预付费"模式）。settle 后即便上游异步生成失败，
	// 也不会被误 cancel（settled 状态拒绝 cancel）。如果将来要做"异步失败退款"，
	// 由 task polling 路径单独处理，这里不动。
	settleVideoHold(holdID)
	saveAIProxyLog(logContext, status, string(transformed), "")
	OK(w, service.VideoTaskResponse(task))
}

func readClientVideoTaskID(r *http.Request) string {
	id := strings.TrimSpace(r.Header.Get("X-Client-Video-Task-ID"))
	if isClientVideoTaskID(id) {
		return id
	}
	return ""
}

// readClientVideoTaskIDOrRequestID 视频任务幂等键：优先 X-Client-Video-Task-ID（视频专用），
// 再退 X-Request-Id（通用），最后 uuid.NewString() 保底。2026-08-17 改造：让 ConsumeUserBalanceWithHold
// 用稳定幂等键，前端重复提交不再双扣。
func readClientVideoTaskIDOrRequestID(r *http.Request) string {
	if id := readClientVideoTaskID(r); id != "" {
		return id
	}
	if r != nil {
		if id := strings.TrimSpace(r.Header.Get("X-Request-Id")); id != "" {
			return id
		}
	}
	return uuid.NewString()
}

// isPublicHTTPURL 判断是否为公网 HTTP(S) URL（保留供其他 handler 复用；vendor 视频分发 2026-08-27 移除）。
func isPublicHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func readVideoTaskSource(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Video-Task-Source"))
}

func readVideoTaskSourceID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Video-Task-Source-ID"))
}

func isClientVideoTaskID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "client_video_task_")
}

func serveAIVideoTask(w http.ResponseWriter, r *http.Request, id string) bool {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		return false
	}
	task, found, err := service.GetUserVideoTask(user.ID, id)
	if err != nil {
		log.Printf("read video task failed: id=%s user=%s err=%v", id, user.ID, err)
		Fail(w, "AI 接口请求失败")
		return true
	}
	if !found {
		return false
	}
	OK(w, service.VideoTaskResponse(task))
	return true
}

func pollVideoTaskFromUpstream(task model.VideoTask) (service.VideoTaskPollUpdate, error) {
	var channel model.ModelChannel
	var err error
	if strings.TrimSpace(task.UserChannelID) != "" {
		channel, err = service.SelectUserLocalModelChannelForModel(task.UserID, task.Model, task.UserChannelID)
	} else {
		channel, err = service.SelectModelChannelForModel(task.Model, task.ChannelID)
	}
	if err != nil {
		return service.VideoTaskPollUpdate{}, err
	}
	pollID := firstNonEmpty(task.UpstreamTaskID, task.ID)
	if isAgnesVideoModel(task.Model) && strings.HasPrefix(task.UpstreamVideoID, "video_") {
		pollID = task.UpstreamVideoID
	}
	if strings.TrimSpace(pollID) == "" {
		return service.VideoTaskPollUpdate{}, errors.New("视频任务缺少上游任务 ID")
	}
	endpoint := "/videos/" + pollID
	upstreamPath := resolveAIProxyPath(channel, task.Model, endpoint)
	// PR-7：轮询加 60s 单次上限，防止某个任务卡死阻塞 worker 整条流水线。
	pollCtx, cancelPoll := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelPoll()
	request, err := http.NewRequestWithContext(pollCtx, http.MethodGet, resolveAIProxyURL(channel, task.Model, upstreamPath), nil)
	if err != nil {
		return service.VideoTaskPollUpdate{}, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	startedAt := time.Now()
	logContext := aiLogContext{
		StartedAt:       startedAt,
		Endpoint:        endpoint,
		Method:          http.MethodGet,
		Model:           task.Model,
		Channel:         channel,
		UserID:          task.UserID,
		UserDisplayName: task.UserDisplayName,
		RequestBody:     fmt.Sprintf(`{"taskId":%q}`, pollID),
	}
	payload, status, err := doAIRequest(request, channel)
	if err != nil {
		saveAIProxyLog(logContext, 0, "", err.Error())
		return service.VideoTaskPollUpdate{}, err
	}
	if status >= http.StatusBadRequest {
		message := readUpstreamAIErrorMessage(payload, status)
		saveAIProxyLog(logContext, status, string(payload), strings.TrimSpace(string(payload)))
		if status == http.StatusTooManyRequests {
			return service.VideoTaskPollUpdate{Status: task.Status, ErrorDetail: message, ResponseBody: string(payload)}, nil
		}
		return service.VideoTaskPollUpdate{Status: "failed", Error: message, ErrorDetail: message, ResponseBody: string(payload)}, nil
	}
	transformed := transformVideoStatusPayload(payload, request, channel, task.Model)
	parsed := parseVideoTaskPayload(transformed, task.Model)
	if parsed.Status == "failed" && parsed.Error == "" {
		parsed.Error = firstNonEmpty(parsed.ErrorDetail, "视频任务生成失败")
	}
	if errMessage := readVideoStatusErrorMessage(payload, transformed, channel, task.Model); errMessage != "" {
		if parsed.Error == "" {
			parsed.Error = errMessage
		}
		parsed.Status = "failed"
	}
	if parsed.ErrorDetail == "" && len(payload) > 0 && parsed.Error != "" {
		parsed.ErrorDetail = string(payload)
	}
	saveAIProxyLog(logContext, status, string(transformed), firstNonEmpty(parsed.Error, ""))
	return service.VideoTaskPollUpdate{
		Status:       parsed.Status,
		Progress:     parsed.Progress,
		Seconds:      parsed.Seconds,
		Size:         parsed.Size,
		VideoURL:     parsed.VideoURL,
		Error:        parsed.Error,
		ErrorDetail:  parsed.ErrorDetail,
		ResponseBody: string(transformed),
	}, nil
}

func normalizeVideoCreateBody(body []byte, contentType string, modelName string, channel model.ModelChannel, upstreamPath string) ([]byte, string, error) {
	if isKIEChannel(channel, modelName) && upstreamPath == "/jobs/createTask" {
		return normalizeKIEVideoBody(body, contentType, modelName, channel)
	}
	if isAPIMartChannel(channel, modelName) && upstreamPath == "/videos/generations" {
		return normalizeAPIMartVideoBody(body, contentType, modelName, channel)
	}
	// Agnes 视频接口要求 seconds 为字符串类型，数字会被上游拒绝（taskSubmitReqAlias.seconds of type string）
	if isAgnesVideoModel(modelName) && upstreamPath == "/videos" {
		return normalizeAgnesVideoBody(body, contentType)
	}
	return body, contentType, nil
}

// normalizeAgnesVideoBody 将 Agnes 视频请求 JSON body 中的数字类型 seconds 转为字符串，
// 兼容客户端发送 {"seconds": 5} 这类数值写法（Agnes 上游要求 seconds 必须是 string）。
func normalizeAgnesVideoBody(body []byte, contentType string) ([]byte, string, error) {
	// 仅处理 JSON 请求；FormData（如前端真实 Agnes 流程使用 num_frames/frame_rate）不在此处理
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "application/json") {
		return body, contentType, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		// 非 JSON 或解析失败，原样透传，交由上游返回错误
		return body, contentType, nil
	}
	if seconds, ok := payload["seconds"]; ok {
		// 数字/其他类型统一转成字符串，字符串则保持不变
		if _, isStr := seconds.(string); !isStr {
			payload["seconds"] = fmt.Sprintf("%v", seconds)
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body, contentType, nil
	}
	return encoded, "application/json", nil
}

func doAIRequest(request *http.Request, channel model.ModelChannel) ([]byte, int, error) {
	response, err := service.HTTPClientForChannel(channel).Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	return payload, response.StatusCode, nil
}

func transformVideoCreatePayload(payload []byte, request *http.Request, channel model.ModelChannel, modelName string) []byte {
	if isKIEChannel(channel, modelName) && strings.Contains(request.URL.Path, "/jobs/createTask") {
		if transformed, ok := transformKIECreateVideoResponse(payload, modelName); ok {
			return transformed
		}
	}
	if isAPIMartChannel(channel, modelName) && strings.Contains(request.URL.Path, "/videos/generations") {
		if transformed, ok := transformAPIMartCreateVideoResponse(payload, modelName); ok {
			return transformed
		}
	}
	return payload
}

func transformVideoStatusPayload(payload []byte, request *http.Request, channel model.ModelChannel, modelName string) []byte {
	if isKIEChannel(channel, modelName) && strings.Contains(request.URL.Path, "/jobs/recordInfo") {
		if transformed, ok := transformKIETaskResponse(payload, modelName); ok {
			return transformed
		}
	}
	if isAPIMartChannel(channel, modelName) && strings.Contains(request.URL.Path, "/tasks/") {
		if transformed, ok := transformAPIMartTaskResponse(payload, modelName); ok {
			return transformed
		}
	}
	return payload
}

func readVideoCreateErrorMessage(raw []byte, transformed []byte, channel model.ModelChannel, modelName string) string {
	if isKIEChannel(channel, modelName) {
		return firstNonEmpty(readKIECreateTaskErrorMessage(raw), readProviderPayloadError(raw), readNormalizedVideoError(transformed))
	}
	return firstNonEmpty(readProviderPayloadError(raw), readNormalizedVideoError(transformed))
}

func readVideoStatusErrorMessage(raw []byte, transformed []byte, channel model.ModelChannel, modelName string) string {
	if isKIEChannel(channel, modelName) {
		return firstNonEmpty(readKIERecordInfoErrorMessage(raw), readProviderPayloadError(raw), readNormalizedVideoError(transformed))
	}
	return firstNonEmpty(readProviderPayloadError(raw), readNormalizedVideoError(transformed))
}

type parsedVideoTaskPayload struct {
	UpstreamTaskID  string
	UpstreamVideoID string
	Status          string
	Progress        int
	Seconds         string
	Size            string
	VideoURL        string
	Error           string
	ErrorDetail     string
}

func parseVideoTaskPayload(payload []byte, modelName string) parsedVideoTaskPayload {
	var root any
	if len(payload) == 0 || json.Unmarshal(payload, &root) != nil {
		return parsedVideoTaskPayload{Status: "processing"}
	}
	data := normalizeVideoPayloadMap(root)
	result := parsedVideoTaskPayload{
		UpstreamTaskID:  firstNonEmpty(readStringPath(data, "task_id"), readStringPath(data, "taskId"), readStringPath(data, "id"), readStringPath(data, "request_id")),
		UpstreamVideoID: firstNonEmpty(readStringPath(data, "video_id"), readStringPath(data, "videoId")),
		Status:          service.NormalizeVideoTaskStatus(firstNonEmpty(readStringPath(data, "status"), readStringPath(data, "state"))),
		Progress:        readIntPath(data, "progress"),
		Seconds:         firstNonEmpty(readStringPath(data, "seconds"), readStringPath(data, "duration")),
		Size:            firstNonEmpty(readStringPath(data, "size"), readSizeFromDimensions(data)),
		VideoURL:        firstNonEmpty(readStringPath(data, "video_url"), readStringPath(data, "url"), readStringPath(data, "remixed_from_video_id"), readStringPath(data, "output_url"), readStringPath(data, "download_url"), findFirstHTTPURL(data)),
		Error:           firstNonEmpty(readStringPath(data, "error.message"), readStringPath(data, "error")),
		ErrorDetail:     "",
	}
	if result.UpstreamTaskID == result.UpstreamVideoID && strings.HasPrefix(result.UpstreamVideoID, "video_") {
		result.UpstreamTaskID = ""
	}
	if result.Status == "" {
		result.Status = "processing"
	}
	if result.VideoURL != "" {
		result.Status = "completed"
		result.Progress = 100
	}
	if result.Status == "failed" && result.Error == "" {
		result.Error = firstNonEmpty(readStringPath(data, "message"), readStringPath(data, "msg"), "视频任务生成失败")
	}
	if result.UpstreamVideoID == "" && isAgnesVideoModel(modelName) && strings.HasPrefix(result.VideoURL, "video_") {
		result.UpstreamVideoID = result.VideoURL
	}
	if result.Error != "" {
		result.ErrorDetail = string(payload)
	}
	return result
}

func normalizeVideoPayloadMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if data, ok := typed["data"].(map[string]any); ok {
			for key, item := range typed {
				if _, exists := data[key]; !exists {
					data[key] = item
				}
			}
			return data
		}
		if data, ok := typed["data"].([]any); ok && len(data) > 0 {
			if item, ok := data[0].(map[string]any); ok {
				for key, value := range typed {
					if _, exists := item[key]; !exists {
						item[key] = value
					}
				}
				return item
			}
		}
		return typed
	default:
		return map[string]any{}
	}
}

func readNormalizedVideoError(payload []byte) string {
	parsed := parseVideoTaskPayload(payload, "")
	if parsed.Status == "failed" || parsed.Error != "" {
		return firstNonEmpty(parsed.Error, "视频任务生成失败")
	}
	return ""
}

func readProviderPayloadError(payload []byte) string {
	var value map[string]any
	if len(payload) == 0 || json.Unmarshal(payload, &value) != nil {
		return ""
	}
	code, hasCode := value["code"]
	if !hasCode {
		return ""
	}
	successCode := false
	switch typed := code.(type) {
	case float64:
		successCode = typed == 0 || typed == 200
	case string:
		text := strings.TrimSpace(strings.ToLower(typed))
		successCode = text == "" || text == "0" || text == "200" || text == "success" || text == "ok"
	default:
		successCode = false
	}
	if successCode {
		return ""
	}
	return firstNonEmpty(readStringPath(value, "error.message"), readStringPath(value, "error"), readStringPath(value, "message"), readStringPath(value, "msg"), fmt.Sprint(code))
}

func readStringPath(data map[string]any, path string) string {
	var current any = data
	for _, part := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[part]
	}
	return strings.TrimSpace(toStringSafe(current))
}

func readIntPath(data map[string]any, key string) int {
	value := data[key]
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		number, _ := typed.Int64()
		return int(number)
	case string:
		var number int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &number)
		return number
	default:
		return 0
	}
}

func readSizeFromDimensions(data map[string]any) string {
	width := readIntPath(data, "width")
	height := readIntPath(data, "height")
	if width > 0 && height > 0 {
		return fmt.Sprintf("%dx%d", width, height)
	}
	return ""
}

func findFirstHTTPURL(value any) string {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
			return text
		}
		var parsed any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			return findFirstHTTPURL(parsed)
		}
	case []any:
		for _, item := range typed {
			if url := findFirstHTTPURL(item); url != "" {
				return url
			}
		}
	case map[string]any:
		for _, key := range []string{"url", "video_url", "videoUrl", "download_url", "downloadUrl", "output_url", "outputUrl", "resultUrls", "result_urls", "videoUrls", "video_urls", "urls", "videos", "video", "data", "result", "metadata"} {
			if url := findFirstHTTPURL(typed[key]); url != "" {
				return url
			}
		}
	}
	return ""
}

func cancelVideoHold(holdID string) {
	if strings.TrimSpace(holdID) == "" {
		return
	}
	if err := service.CancelBalanceHold(holdID); err != nil {
		log.Printf("AI video cancel balance hold failed: holdID=%s err=%v", holdID, err)
	}
}

func settleVideoHold(holdID string) {
	if strings.TrimSpace(holdID) == "" {
		return
	}
	if err := service.SettleBalanceHold(holdID); err != nil {
		log.Printf("AI video settle balance hold failed: holdID=%s err=%v", holdID, err)
	}
}
