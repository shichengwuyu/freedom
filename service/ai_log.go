package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

const (
	aiLogRequestTextLimit = 64 * 1024
	aiLogErrorTextLimit   = 16 * 1024
	aiLogScannerMax       = 16 * 1024 * 1024
	defaultAILogCron      = "0 3 * * *"
	defaultAILogRetention = 14
)

var (
	htmlTagPattern        = regexp.MustCompile(`(?s)<[^>]+>`)
	whitespacePattern     = regexp.MustCompile(`\s+`)
	longDataURLPattern    = regexp.MustCompile(`data:image/[a-zA-Z0-9.+-]+;base64,[A-Za-z0-9+/=\r\n]{512,}`)
	longBase64TextPattern = regexp.MustCompile(`"[A-Za-z0-9+/=]{512,}"`)

	aiLogCleanupCron *cron.Cron
	aiLogCleanupOnce sync.Once
	aiLogCleanupMu   sync.Mutex
)

type AICallLogInput struct {
	UserID          string `json:"userId"`
	UserDisplayName string `json:"userDisplayName"`
	Endpoint        string `json:"endpoint"`
	Method          string `json:"method"`
	Model           string `json:"model"`
	ChannelID       string `json:"channelId"`
	ChannelName     string `json:"channelName"`
	TokenID         string `json:"tokenId"` // Sprint 1.1：Bearer sk- 鉴权时记录 user_token.id
	Status          int    `json:"status"`
	DurationMs      int64  `json:"durationMs"`
	CostCents int `json:"costCents"`
	RequestBody     string `json:"requestBody"`
	ResponseBody    string `json:"responseBody"`
	Error           string `json:"error"`
	// Sprint 2 新增
	AttemptIndex       int    `json:"attemptIndex"`
	UpstreamStatusCode int    `json:"upstreamStatusCode"`
	KeyIndex           int    `json:"keyIndex"`
}

func SaveAICallLog(input AICallLogInput) {
	responseBody := normalizeAICallResponseLog(input.ResponseBody, input.Error)
	errorText := normalizeAICallErrorLog(input.Error, input.ResponseBody)
	item := model.AICallLog{
		ID:              uuid.NewString(),
		UserID:          strings.TrimSpace(input.UserID),
		UserDisplayName: strings.TrimSpace(input.UserDisplayName),
		Endpoint:        strings.TrimSpace(input.Endpoint),
		Method:          strings.TrimSpace(input.Method),
		Model:           strings.TrimSpace(input.Model),
		ChannelID:       strings.TrimSpace(input.ChannelID),
		ChannelName:     strings.TrimSpace(input.ChannelName),
		TokenID:         strings.TrimSpace(input.TokenID),
		Status:          input.Status,
		DurationMs:      input.DurationMs,
		CostCents:         input.CostCents,
		RequestBody:     truncateLogText(input.RequestBody, aiLogRequestTextLimit),
		ResponseBody:    responseBody,
		Error:           truncateLogText(errorText, aiLogErrorTextLimit),
		AttemptIndex:       input.AttemptIndex,
		UpstreamStatusCode: input.UpstreamStatusCode,
		KeyIndex:           input.KeyIndex,
		LastTryAt:          now(),
		CreatedAt:       now(),
	}
	if err := appendAICallLog(item); err != nil {
		log.Printf("write ai call log failed err=%v", err)
	}
}

func ListAICallLogs(q model.Query) (model.AICallLogList, error) {
	q.Normalize()
	items, err := readAICallLogsByDateRange(q.StartDate, q.EndDate)
	if err != nil {
		return model.AICallLogList{}, err
	}
	if keyword := strings.ToLower(strings.TrimSpace(q.Keyword)); keyword != "" {
		filtered := make([]model.AICallLog, 0, len(items))
		for _, item := range items {
			if aiLogMatchesKeyword(item, keyword) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	total := len(items)
	start := q.Offset()
	if start >= total {
		return model.AICallLogList{Items: []model.AICallLog{}, Total: total}, nil
	}
	end := start + q.PageSize
	if end > total {
		end = total
	}
	return model.AICallLogList{Items: items[start:end], Total: total}, nil
}

// ListAICallLogDates 返回所有存在日志的日期列表（倒序），格式为 2006-01-02。
func ListAICallLogDates() ([]string, error) {
	files, err := aiLogFiles()
	if err != nil {
		return nil, err
	}
	dates := make([]string, 0, len(files))
	for _, file := range files {
		if dateStr, ok := aiLogFileDateString(file); ok {
			dates = append(dates, dateStr)
		}
	}
	return dates, nil
}

// DeleteAICallLogsOlderThan 删除超过指定天数的日志文件。
func DeleteAICallLogsOlderThan(days int) (int, error) {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	files, err := aiLogFiles()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, file := range files {
		fileDate, ok := aiLogFileDate(file)
		if !ok || !fileDate.Before(startOfDay(cutoff)) {
			continue
		}
		if err := os.Remove(file); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// DeleteAICallLogsByDates 按具体日期数组删除日志，日期格式为 2006-01-02。
func DeleteAICallLogsByDates(dates []string) (int, error) {
	if len(dates) == 0 {
		return 0, nil
	}
	dateSet := make(map[string]struct{}, len(dates))
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d != "" {
			dateSet[d] = struct{}{}
		}
	}
	if len(dateSet) == 0 {
		return 0, nil
	}
	files, err := aiLogFiles()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, file := range files {
		dateStr, ok := aiLogFileDateString(file)
		if !ok {
			continue
		}
		if _, hit := dateSet[dateStr]; !hit {
			continue
		}
		if err := os.Remove(file); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func StartAILogCleanupScheduler() {
	aiLogCleanupOnce.Do(func() {
		aiLogCleanupCron = cron.New()
		aiLogCleanupCron.Start()
	})
	RefreshAILogCleanupScheduler()
}

func RefreshAILogCleanupScheduler() {
	aiLogCleanupMu.Lock()
	defer aiLogCleanupMu.Unlock()
	if aiLogCleanupCron == nil {
		return
	}
	for _, entry := range aiLogCleanupCron.Entries() {
		aiLogCleanupCron.Remove(entry.ID)
	}
	settings, err := repository.GetSettings()
	if err != nil {
		log.Printf("load ai log cleanup setting failed err=%v", err)
		return
	}
	setting := normalizeAILogCleanupSetting(settings.Private.AILog.Cleanup)
	if setting.Enabled == nil || !*setting.Enabled {
		return
	}
	if _, err := aiLogCleanupCron.AddFunc(setting.Cron, func() {
		removed, err := DeleteAICallLogsOlderThan(setting.RetentionDays)
		if err != nil {
			log.Printf("scheduled ai log cleanup failed err=%v", err)
			return
		}
		log.Printf("scheduled ai log cleanup done removedFiles=%d retentionDays=%d", removed, setting.RetentionDays)
	}); err != nil {
		log.Printf("add ai log cleanup cron failed cron=%s err=%v", setting.Cron, err)
	}
}

func normalizeAILogCleanupSetting(setting model.AILogCleanupSetting) model.AILogCleanupSetting {
	if setting.Cron == "" {
		setting.Cron = defaultAILogCron
	}
	if setting.RetentionDays <= 0 {
		setting.RetentionDays = defaultAILogRetention
	}
	if setting.Enabled == nil {
		enabled := false
		setting.Enabled = &enabled
	}
	return setting
}

func normalizeAILogSetting(setting model.AILogSetting) model.AILogSetting {
	setting.Cleanup = normalizeAILogCleanupSetting(setting.Cleanup)
	if setting.LocalDirectReportEnabled == nil {
		enabled := false
		setting.LocalDirectReportEnabled = &enabled
	}
	return setting
}

func LocalDirectAILogEnabled() bool {
	settings, err := repository.GetSettings()
	if err != nil {
		log.Printf("load local direct ai log setting failed err=%v", err)
		return false
	}
	setting := normalizeAILogSetting(settings.Private.AILog)
	return setting.LocalDirectReportEnabled != nil && *setting.LocalDirectReportEnabled
}

func appendAICallLog(item model.AICallLog) error {
	dir := strings.TrimSpace(config.Cfg.AILogDir)
	if dir == "" {
		dir = filepath.Join("data", "logs", "ai-calls")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filePath := filepath.Join(dir, fmt.Sprintf("ai-calls-%s.log", time.Now().Format("2006-01-02")))
	// 写入前先做大小轮转：避免单文件无限增长导致容器镜像/磁盘被撑爆。
	// 轮转失败不影响写入，只 log 警告。
	rotateAILogFileIfOversize(filePath)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := json.Marshal(item)
	if err != nil {
		return err
	}
	log.New(file, "", 0).Println(string(encoded))
	return nil
}

// maxAILogFileBytes 单文件超过此大小就轮转（100 MiB）。
// 高频调用一天可能产生 GB 级 JSON，单文件过大不便排查/归档。
const maxAILogFileBytes int64 = 100 * 1024 * 1024

// maxAILogRotations 轮转后保留的最大分片数（.1 ~ .N）。超出时覆盖最旧的 .1。
const maxAILogRotations = 99

// rotateAILogFileIfOversize 若 filePath 存在且大小超过阈值，按 .1/.2/.../.N 索引轮转。
// 简单实现：找到第一个不存在的编号 rename 过去；都占满就回收 .1。
// 不依赖 logrotate / cron，部署时无需额外配置。
func rotateAILogFileIfOversize(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil || info.Size() < maxAILogFileBytes {
		return
	}
	rotIdx := 1
	for {
		candidate := fmt.Sprintf("%s.%d", filePath, rotIdx)
		if _, statErr := os.Stat(candidate); os.IsNotExist(statErr) {
			break
		}
		rotIdx++
		if rotIdx > maxAILogRotations {
			// 99 段都满了，回收最旧的 .1 后写入
			_ = os.Remove(fmt.Sprintf("%s.1", filePath))
			rotIdx = 1
			break
		}
	}
	if renameErr := os.Rename(filePath, fmt.Sprintf("%s.%d", filePath, rotIdx)); renameErr != nil {
		log.Printf("ai-calls log rotate failed path=%s err=%v", filePath, renameErr)
	}
}

func readAICallLogs() ([]model.AICallLog, error) {
	return readAICallLogsByDateRange("", "")
}

// readAICallLogsByDateRange 按日期范围读取日志，日期格式 2006-01-02；留空表示不限制。
func readAICallLogsByDateRange(startDate, endDate string) ([]model.AICallLog, error) {
	files, err := aiLogFiles()
	if err != nil {
		return nil, err
	}
	startT, startOK := parseLogDate(startDate)
	endT, endOK := parseLogDate(endDate)
	// 结束日期需包含整天，所以加一天
	if endOK {
		endT = endT.AddDate(0, 0, 1)
	}
	items := []model.AICallLog{}
	for _, file := range files {
		fileDate, ok := aiLogFileDate(file)
		if ok {
			if startOK && fileDate.Before(startOfDay(startT)) {
				continue
			}
			if endOK && !fileDate.Before(startOfDay(endT)) {
				continue
			}
		}
		fileItems, err := readAICallLogFile(file)
		if err != nil {
			log.Printf("read ai call log file failed file=%s err=%v", file, err)
			continue
		}
		items = append(items, fileItems...)
	}
	return items, nil
}

func parseLogDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	return parsed, err == nil
}

func readAICallLogFile(filePath string) ([]model.AICallLog, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), aiLogScannerMax)
	items := []model.AICallLog{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item model.AICallLog
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		item.ResponseBody = normalizeAICallResponseLog(item.ResponseBody, item.Error)
		item.Error = normalizeAICallErrorLog(item.Error, item.ResponseBody)
		items = append(items, item)
	}
	return items, scanner.Err()
}

func aiLogFiles() ([]string, error) {
	dir := strings.TrimSpace(config.Cfg.AILogDir)
	if dir == "" {
		dir = filepath.Join("data", "logs", "ai-calls")
	}
	files, err := filepath.Glob(filepath.Join(dir, "ai-calls-*.log"))
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

func aiLogFileDate(filePath string) (time.Time, bool) {
	name := filepath.Base(filePath)
	value := strings.TrimSuffix(strings.TrimPrefix(name, "ai-calls-"), ".log")
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	return parsed, err == nil
}

// aiLogFileDateString 从文件路径中提取日期字符串，格式 2006-01-02。
func aiLogFileDateString(filePath string) (string, bool) {
	name := filepath.Base(filePath)
	value := strings.TrimSuffix(strings.TrimPrefix(name, "ai-calls-"), ".log")
	if _, err := time.ParseInLocation("2006-01-02", value, time.Local); err != nil {
		return "", false
	}
	return value, true
}

func startOfDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func aiLogMatchesKeyword(item model.AICallLog, keyword string) bool {
	fields := []string{item.UserID, item.UserDisplayName, item.Endpoint, item.Method, item.Model, item.ChannelID, item.ChannelName, item.RequestBody, item.ResponseBody, item.Error, strconv.Itoa(item.Status)}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), keyword) {
			return true
		}
	}
	return false
}

func normalizeAICallResponseLog(responseBody string, errorMessage string) string {
	responseBody = strings.TrimSpace(responseBody)
	if responseBody == "" {
		return ""
	}
	formatted := formatAICallLogPayload(responseBody)
	reason := extractAICallFailureReason(errorMessage)
	if reason == "" {
		reason = extractAICallFailureReason(responseBody)
	}
	if reason == "" {
		return formatted
	}
	if strings.HasPrefix(formatted, "失败原因:") {
		return formatted
	}
	return "失败原因: " + reason + "\n\n原始返回:\n" + formatted
}

func normalizeAICallErrorLog(errorMessage string, responseBody string) string {
	if reason := extractAICallFailureReason(errorMessage); reason != "" {
		return reason
	}
	if reason := extractAICallFailureReason(responseBody); reason != "" {
		return reason
	}
	return cleanPlainLogText(errorMessage)
}

func formatAICallLogPayload(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		redactLargeLogStrings(&payload)
		if encoded, err := json.MarshalIndent(payload, "", "  "); err == nil {
			return string(encoded)
		}
	}
	if strings.Contains(raw, "\ndata:") || strings.HasPrefix(raw, "data:") || strings.Contains(raw, "\nevent:") || strings.HasPrefix(raw, "event:") {
		return formatEventStreamLog(raw)
	}
	return redactLargePlainLogText(raw)
}

func formatEventStreamLog(raw string) string {
	lines := strings.Split(raw, "\n")
	formatted := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data:") {
			formatted = append(formatted, redactLargePlainLogText(line))
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if data == "" || data == "[DONE]" {
			formatted = append(formatted, line)
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			formatted = append(formatted, redactLargePlainLogText(line))
			continue
		}
		redactLargeLogStrings(&payload)
		encoded, err := json.Marshal(payload)
		if err != nil {
			formatted = append(formatted, redactLargePlainLogText(line))
			continue
		}
		formatted = append(formatted, "data: "+string(encoded))
	}
	return strings.TrimSpace(strings.Join(formatted, "\n"))
}

func extractAICallFailureReason(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		reasons := []string{}
		collectAICallFailureReasons(payload, &reasons)
		return strings.Join(dedupeStrings(reasons), "；")
	}
	cleaned := cleanPlainLogText(raw)
	if cleaned == "" || looksLikeSuccessfulLogText(cleaned) {
		return ""
	}
	return cleaned
}

func collectAICallFailureReasons(value any, reasons *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if message := stringField(typed, "message"); message != "" {
			*reasons = append(*reasons, message)
		}
		if message := stringField(typed, "msg"); message != "" {
			*reasons = append(*reasons, message)
		}
		if detail := stringField(typed, "detail"); detail != "" {
			*reasons = append(*reasons, detail)
		}
		if reason := stringField(typed, "reason"); reason != "" {
			*reasons = append(*reasons, reason)
		}
		if errValue, ok := typed["error"]; ok && errValue != nil {
			if message, ok := errValue.(string); ok && strings.TrimSpace(message) != "" {
				*reasons = append(*reasons, strings.TrimSpace(message))
			} else {
				collectAICallFailureReasons(errValue, reasons)
			}
		}
		status := strings.ToLower(stringField(typed, "status"))
		typeName := stringField(typed, "type")
		if status == "failed" || status == "incomplete" || status == "cancelled" || status == "canceled" {
			if typeName == "image_generation_call" {
				*reasons = append(*reasons, "Responses 图像生成调用失败：image_generation_call 状态为 "+status+"，接口未返回具体错误原因")
			} else if typeName != "" {
				*reasons = append(*reasons, typeName+" 状态为 "+status)
			} else {
				*reasons = append(*reasons, "状态为 "+status)
			}
		}
		for key, item := range typed {
			if key == "instructions" || key == "prompt" || key == "requestBody" || key == "responseBody" {
				continue
			}
			collectAICallFailureReasons(item, reasons)
		}
	case []any:
		for _, item := range typed {
			collectAICallFailureReasons(item, reasons)
		}
	}
}

func stringField(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = cleanPlainLogText(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func cleanPlainLogText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = longDataURLPattern.ReplaceAllString(value, "[redacted image data]")
	value = longBase64TextPattern.ReplaceAllString(value, `"[redacted large base64/string]"`)
	value = html.UnescapeString(htmlTagPattern.ReplaceAllString(value, " "))
	value = whitespacePattern.ReplaceAllString(value, " ")
	value = strings.TrimSpace(value)
	if len(value) > aiLogErrorTextLimit {
		return value[:aiLogErrorTextLimit] + "\n... [truncated]"
	}
	return value
}

func redactLargePlainLogText(value string) string {
	value = longDataURLPattern.ReplaceAllString(value, "[redacted image data]")
	value = longBase64TextPattern.ReplaceAllString(value, `"[redacted large base64/string]"`)
	return strings.TrimSpace(value)
}

func redactLargeLogStrings(value *any) {
	switch typed := (*value).(type) {
	case map[string]any:
		for key, item := range typed {
			if text, ok := item.(string); ok && isLargeLogString(text) {
				typed[key] = fmt.Sprintf("[redacted large string len=%d]", len(text))
				continue
			}
			redactLargeLogStrings(&item)
			typed[key] = item
		}
	case []any:
		for index, item := range typed {
			redactLargeLogStrings(&item)
			typed[index] = item
		}
	}
}

func isLargeLogString(value string) bool {
	if strings.HasPrefix(value, "data:image/") {
		return true
	}
	return len(value) > 4096 && looksLikeLogBase64(value)
}

func looksLikeLogBase64(value string) bool {
	for _, char := range value[:min(len(value), 256)] {
		if !(char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '+' || char == '/' || char == '=') {
			return false
		}
	}
	return true
}

func looksLikeSuccessfulLogText(value string) bool {
	lower := strings.ToLower(value)
	return !strings.Contains(lower, "error") &&
		!strings.Contains(lower, "failed") &&
		!strings.Contains(lower, "fail") &&
		!strings.Contains(lower, "timeout") &&
		!strings.Contains(lower, "stream_read_error") &&
		!strings.Contains(lower, "502") &&
		!strings.Contains(lower, "500") &&
		!strings.Contains(lower, "429") &&
		!strings.Contains(lower, "401") &&
		!strings.Contains(lower, "403")
}

func truncateLogText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n... [truncated]"
}
