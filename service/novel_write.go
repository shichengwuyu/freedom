package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === 小说创作工作台（v1） ===
//
// 入口：
//   - CreateSession / ListSessions / GetSession / UpdateSession / DeleteSession
//   - ListMessages / SendMessage / ContinueMessage
//   - GetPrompt / PutPrompt
//   - ExportStoryboard
//   - ListExports
//
// 安全：所有 user-scoped 操作按 user_id 过滤；Session model 字段 immutable。

// 长度限制（promotion 写到前端 + 后端双校验）
const (
	maxSystemPromptRunes = 8000 // system prompt 模板上限
	maxUserContentRunes   = 4000 // 单条 user 消息上限
	maxSessionsPerUser    = 50   // 每用户最多 50 个 session
	novelWriteEndpoint    = "/api/v1/novel-write/messages"
)

// varSubPattern 匹配 {{var_name}}。变量名必须以字母或下划线开头，
// 后续字符 [a-zA-Z0-9_]，防注入（不能让用户通过变量名塞 prompt 指令）。
// 注意：不接受数字开头的变量名（如 {{2var}}），因为正规标识符不应数字开头。
var varSubPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// substituteVariables 替换模板里的 {{var_name}} → vars[name]。
// 未定义的变量保留原样（不静默替换为空），让用户能看到"我没填"。
func substituteVariables(template string, vars map[string]string) string {
	return varSubPattern.ReplaceAllStringFunc(template, func(match string) string {
		name := varSubPattern.FindStringSubmatch(match)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		return match
	})
}

// === Session CRUD ===

// ListSessions 列当前用户所有 session（按 updated_at DESC）。
func ListNovelWriteSessions(userID string) ([]model.NovelWriteSession, error) {
	return repository.ListNovelWriteSessionsByUser(userID, maxSessionsPerUser)
}

// CreateSession 新建 session；满 maxSessionsPerUser 自动删最旧。
func CreateNovelWriteSession(userID, modelName, systemPrompt string, variables model.Variables) (*model.NovelWriteSession, error) {
	// 校验
	if strings.TrimSpace(modelName) == "" {
		return nil, errors.New("model 不能为空")
	}
	if len([]rune(systemPrompt)) > maxSystemPromptRunes {
		return nil, fmt.Errorf("systemPrompt 不能超过 %d 字", maxSystemPromptRunes)
	}

	// 满了删最旧
	count, _ := repository.CountNovelWriteSessionsByUser(userID)
	if count >= maxSessionsPerUser {
		_ = repository.DeleteOldestNovelWriteSession(userID)
	}

	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s := &model.NovelWriteSession{
		ID:           newID("nws"),
		UserID:       userID,
		Title:        "新会话",
		Model:        modelName,
		SystemPrompt: systemPrompt,
		Variables:    model.MarshalVariables(variables),
		CreatedAt:    nowStr,
		UpdatedAt:    nowStr,
	}
	if err := repository.CreateNovelWriteSession(s); err != nil {
		return nil, err
	}
	return s, nil
}

// GetSession 拿一条 session（按 user_id 过滤）。
func GetNovelWriteSession(userID, id string) (*model.NovelWriteSession, error) {
	return repository.GetNovelWriteSession(id, userID)
}

// UpdateSession 更新 session 的 title / systemPrompt / variables / model（任意子集）。
// 注：v2 改成允许改 model（用户选错模型可重选）；id / user_id / created_at 不可改。
func UpdateNovelWriteSession(userID, id string, updates map[string]any) error {
	if updates == nil {
		return nil
	}
	// 禁止改 immutable 字段
	delete(updates, "id")
	delete(updates, "user_id")
	delete(updates, "created_at")
	// systemPrompt 长度校验
	if sp, ok := updates["systemPrompt"].(string); ok {
		if len([]rune(sp)) > maxSystemPromptRunes {
			return fmt.Errorf("systemPrompt 不能超过 %d 字", maxSystemPromptRunes)
		}
	}
	return repository.UpdateNovelWriteSession(id, userID, updates)
}

// DeleteSession 删 session + 级联 messages。
func DeleteNovelWriteSession(userID, id string) error {
	return repository.DeleteNovelWriteSession(id, userID)
}

// === Messages ===

// ListMessages 列某 session 的全部消息（前置校验归属）。
func ListNovelWriteMessages(userID, sessionID string) ([]model.NovelWriteMessage, error) {
	sess, err := repository.GetNovelWriteSession(sessionID, userID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, errors.New("会话不存在或无权访问")
	}
	return repository.ListNovelWriteMessagesBySession(sessionID)
}

// SendMessage 发新消息：扣费 + 调 AI + 持久化。
func SendNovelWriteMessage(ctx context.Context, userID, sessionID, userContent string) (assistantContent string, err error) {
	// 1. 长度校验
	if len([]rune(userContent)) > maxUserContentRunes {
		return "", fmt.Errorf("单条消息不超过 %d 字", maxUserContentRunes)
	}

	// 2. 拿 session（校验归属）
	sess, err := repository.GetNovelWriteSession(sessionID, userID)
	if err != nil {
		return "", err
	}
	if sess == nil {
		return "", errors.New("会话不存在或无权访问")
	}

	// 3. 拿历史 messages
	history, err := repository.ListNovelWriteMessagesBySession(sessionID)
	if err != nil {
		return "", err
	}

	// 4. 拼 chat messages（system 经变量替换 + 历史 + 新 user）
	vars := model.UnmarshalVariables(sess.Variables)
	systemContent := substituteVariables(sess.SystemPrompt, vars)
	messages := buildChatMessages(systemContent, history, userContent)

	// 5. 拿模型单价
	modelCost, err := ModelCost(sess.Model)
	if err != nil {
		return "", errors.New("该模型暂未配置价格")
	}
	cents := modelCost.CostCents
	if cents <= 0 {
		return "", errors.New("该模型当前价格为 0 元")
	}

	// 6. 扣费 hold
	requestID := "novel-write:" + sessionID + ":" + newID("msg")
	holdID, err := ConsumeUserBalanceWithHold(userID, sess.Model, cents, novelWriteEndpoint, requestID)
	if err != nil {
		return "", err
	}

	// 7. 持久化 user 消息
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_ = repository.AppendNovelWriteMessage(&model.NovelWriteMessage{
		SessionID: sessionID,
		Role:      "user",
		Content:   userContent,
		CreatedAt: nowStr,
	})

	// 8. 调 chat
	assistantContent, err = callNovelWriteChat(ctx, sess.Model, messages)
	if err != nil {
		_ = CancelBalanceHold(holdID)
		return "", err
	}
	_ = SettleBalanceHold(holdID)

	// 9. 持久化 assistant 消息
	_ = repository.AppendNovelWriteMessage(&model.NovelWriteMessage{
		SessionID: sessionID,
		Role:      "assistant",
		Content:   assistantContent,
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	})

	// 10. touch session
	_ = repository.TouchNovelWriteSession(sessionID, userID)

	return assistantContent, nil
}

// ContinueMessage 续写：等价于 SendMessage(userContent="请从上次的末尾继续，不要重复")。
func ContinueNovelWriteMessage(ctx context.Context, userID, sessionID string) (string, error) {
	return SendNovelWriteMessage(ctx, userID, sessionID, "请从上次的末尾继续，不要重复")
}

// === Prompt ===

// GetPrompt 拿 session 的 prompt + variables（独立端点）。
func GetNovelWritePrompt(userID, sessionID string) (systemPrompt string, variables model.Variables, err error) {
	sess, err := repository.GetNovelWriteSession(sessionID, userID)
	if err != nil {
		return "", nil, err
	}
	if sess == nil {
		return "", nil, errors.New("会话不存在或无权访问")
	}
	return sess.SystemPrompt, model.UnmarshalVariables(sess.Variables), nil
}

// PutPrompt 整替换 prompt + variables。
func PutNovelWritePrompt(userID, sessionID, systemPrompt string, variables model.Variables) error {
	if len([]rune(systemPrompt)) > maxSystemPromptRunes {
		return fmt.Errorf("systemPrompt 不能超过 %d 字", maxSystemPromptRunes)
	}
	return repository.UpdateNovelWriteSession(sessionID, userID, map[string]any{
		"system_prompt": systemPrompt,
		"variables":     model.MarshalVariables(variables),
	})
}

// === 内部 helpers ===

// buildChatMessages 拼 chat messages 数组。
// system: 替换过变量的 systemPrompt
// 后续: 历史 user/assistant（跳过 system，避免覆盖）
// 最后: 新 user
func buildChatMessages(systemContent string, history []model.NovelWriteMessage, newUserContent string) []map[string]any {
	out := []map[string]any{
		{"role": "system", "content": systemContent},
	}
	for _, m := range history {
		if m.Role == "system" {
			continue
		}
		out = append(out, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	out = append(out, map[string]any{
		"role":    "user",
		"content": newUserContent,
	})
	return out
}

// callNovelWriteChat 调 chat/completions 拿 assistant 内容。
// 复用 PickChannelWithRetry 选渠道，复用 BuildModelChannelURL 拼 URL。
func callNovelWriteChat(ctx context.Context, modelName string, messages []map[string]any) (string, error) {
	channel, err := PickChannelWithRetry(ChannelSelectorRequest{
		Group:      "",         // default group
		Model:      modelName,
		Capability: "default",  // channel 默认 capability=空 → BuildAbilityCache 把空回退到 "default"
		RetryIndex: 0,
	})
	if err != nil {
		return "", err
	}

	body, _ := json.Marshal(map[string]any{
		"model":       modelName,
		"messages":    messages,
		"temperature": 0.7,
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		BuildModelChannelURL(*channel.Channel, "/chat/completions"),
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+channel.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("上游 HTTP %d: %s", resp.StatusCode, string(bodyBytes)[:min(200, len(bodyBytes))])
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("模型未返回内容")
	}
	return result.Choices[0].Message.Content, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
