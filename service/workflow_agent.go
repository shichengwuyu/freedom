package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

func DraftCreativeWorkflow(ctx context.Context, request WorkflowAgentDraftRequest) (WorkflowAgentDraftResponse, error) {
	startedAt := time.Now()
	user, ok := UserFromContext(ctx)
	if !ok || user.ID == "" {
		return WorkflowAgentDraftResponse{}, safeMessageError{message: "请先登录"}
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return WorkflowAgentDraftResponse{}, safeMessageError{message: "请输入工作流需求"}
	}

	modelName, err := workflowDraftModel(request.Model)
	if err != nil {
		return WorkflowAgentDraftResponse{}, err
	}
	if request.ChannelMode != "local" && !UserCanUseRemoteModelChannel(user) {
		return WorkflowAgentDraftResponse{}, safeMessageError{message: "当前账号未开放云端渠道"}
	}
	channel, err := workflowDraftChannel(request, modelName)
	if err != nil {
		return WorkflowAgentDraftResponse{}, err
	}

	// 价格未配置：云端模式直接拒绝（防 0 元白嫖）；local 模式不计费放行。
	modelCost, err := ModelCost(modelName)
	if err != nil {
		if request.ChannelMode != "local" {
			return WorkflowAgentDraftResponse{}, safeMessageError{message: "该模型暂未配置价格，请联系管理员或换一个模型"}
		}
		// local 通道不计费，给一个零值让下面 cents 路径走 "不扣费" 分支。
		modelCost = model.ModelCost{Model: modelName, Unit: model.ModelCostUnitPerCall}
	}
	// 工作流 agent 按文本模型处理：per_call 模式，单请求只扣 账户余额（忽略按秒配置）
	cents := modelCost.CostCents
	if modelCost.Unit == model.ModelCostUnitPerSecond && modelCost.CostCentsPerSecond > 0 {
		cents = modelCost.CostCentsPerSecond
	}
	// 兜底：云端模式下 cents 必须 > 0；否则拒绝（防 0 元白嫖）。
	if request.ChannelMode != "local" && cents <= 0 {
		return WorkflowAgentDraftResponse{}, safeMessageError{message: "该模型当前价格为 0 元，请联系管理员核对价格配置"}
	}
	charged账户余额 := request.ChannelMode != "local" && cents > 0
	holdID := ""
	if charged账户余额 {
		// 2026-08-17 改造：改走 WithHold 路径，提供幂等键 + 状态机正确性，让网络重试+退款失败
		// 不再成为"凭空涨余额"风险。每条请求用 uuid 当 requestID（workflow agent 这里没有
		// 外部 clientTaskId 概念，本端生成唯一 ID 即可）。
		// Sprint 1.1：可选 tokenID（Bearer sk- 鉴权时由 ctx 携带）
		tokenID := ""
		if t, ok := UserTokenFromContext(ctx); ok {
			tokenID = t.ID
		}
		holdID, err = ConsumeUserBalanceWithHold(user.ID, modelName, cents, "/workflows/agent-draft", uuid.NewString(), tokenID)
		if err != nil {
			return WorkflowAgentDraftResponse{}, err
		}
	}
	cancelHold := func() {
		if charged账户余额 && holdID != "" {
			_ = CancelBalanceHold(holdID)
		}
	}
	settleHold := func() {
		if charged账户余额 && holdID != "" {
			_ = SettleBalanceHold(holdID)
		}
	}

	body, _ := json.Marshal(map[string]any{
		"model":       modelName,
		"messages":    workflowAgentMessages(prompt, request.References),
		"temperature": 0.2,
	})

	httpRequest, err := http.NewRequest(
		http.MethodPost,
		BuildModelChannelURL(channel, "/chat/completions"),
		bytes.NewReader(body),
	)
	if err != nil {
		cancelHold()
		return WorkflowAgentDraftResponse{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+channel.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(maxInt(channel.Timeout, 600)) * time.Second}
	response, err := client.Do(httpRequest)
	if err != nil {
		cancelHold()
		SaveAICallLog(AICallLogInput{
			UserID:          user.ID,
			UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
			Endpoint:        "/workflows/agent-draft",
			Method:          http.MethodPost,
			Model:           modelName,
			ChannelID:       channel.ID,
			ChannelName:     channel.Name,
			Status:          0,
			DurationMs:      time.Since(startedAt).Milliseconds(),
			CostCents:         cents,
			RequestBody:     string(body),
			Error:           err.Error(),
		})
		return WorkflowAgentDraftResponse{}, err
	}
	defer response.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode >= http.StatusBadRequest {
		cancelHold()
		SaveAICallLog(AICallLogInput{
			UserID:          user.ID,
			UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
			Endpoint:        "/workflows/agent-draft",
			Method:          http.MethodPost,
			Model:           modelName,
			ChannelID:       channel.ID,
			ChannelName:     channel.Name,
			Status:          response.StatusCode,
			DurationMs:      time.Since(startedAt).Milliseconds(),
			CostCents:         cents,
			RequestBody:     string(body),
			ResponseBody:    string(responseBody),
			Error:           string(responseBody),
		})
		return WorkflowAgentDraftResponse{}, readChannelError(string(responseBody), "工作流 Agent 请求失败")
	}

	content := extractChatMessage(string(responseBody))
	draft, warnings, err := normalizeWorkflowDraft(content, request.Scope)
	if err != nil {
		cancelHold()
		SaveAICallLog(AICallLogInput{
			UserID:          user.ID,
			UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
			Endpoint:        "/workflows/agent-draft",
			Method:          http.MethodPost,
			Model:           modelName,
			ChannelID:       channel.ID,
			ChannelName:     channel.Name,
			Status:          response.StatusCode,
			DurationMs:      time.Since(startedAt).Milliseconds(),
			CostCents:         cents,
			RequestBody:     string(body),
			ResponseBody:    string(responseBody),
			Error:           err.Error(),
		})
		return WorkflowAgentDraftResponse{}, err
	}

	SaveAICallLog(AICallLogInput{
		UserID:          user.ID,
		UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
		Endpoint:        "/workflows/agent-draft",
		Method:          http.MethodPost,
		Model:           modelName,
		ChannelID:       channel.ID,
		ChannelName:     channel.Name,
		Status:          response.StatusCode,
		DurationMs:      time.Since(startedAt).Milliseconds(),
		CostCents:         cents,
		RequestBody:     string(body),
		ResponseBody:    string(responseBody),
	})
	settleHold()
	return WorkflowAgentDraftResponse{Draft: draft, Warnings: warnings, Model: modelName}, nil
}

func workflowDraftModel(modelName string) (string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName != "" {
		return modelName, nil
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return "", err
	}
	normalized := normalizeSettings(settings)
	for _, channel := range normalized.Private.Channels {
		for _, model := range channel.Models {
			if strings.TrimSpace(model) != "" {
				return strings.TrimSpace(model), nil
			}
		}
	}
	return "", safeMessageError{message: "请先配置文本模型"}
}

func workflowDraftChannel(request WorkflowAgentDraftRequest, modelName string) (model.ModelChannel, error) {
	if request.ChannelMode == "local" {
		channel := model.ModelChannel{
			ID:       strings.TrimSpace(request.ChannelID),
			Name:     "用户本地直连",
			BaseURL:  strings.TrimSpace(request.BaseURL),
			APIKey:   strings.TrimSpace(request.APIKey),
			Models:   []string{modelName},
			Weight:   1,
			Timeout:  600,
		}
		if channel.BaseURL == "" || channel.APIKey == "" {
			return model.ModelChannel{}, safeMessageError{message: "文本模型本地直连渠道配置不完整"}
		}
		return channel, nil
	}
	return SelectModelChannel(modelName)
}

func workflowAgentMessages(prompt string, references []string) []map[string]any {
	systemPrompt := ""
	if settings, err := repository.GetSettings(); err == nil {
		normalized := normalizeSettings(settings)
		systemPrompt = strings.TrimSpace(normalized.Public.ModelChannel.SystemPrompts.WorkflowAgent)
	}
	if systemPrompt == "" {
		systemPrompt = "你是一个创意工作流设计助手。根据用户描述生成一个JSON格式的工作流模板。"
	}

	messages := []map[string]any{{"role": "system", "content": systemPrompt}}
	var content []map[string]any
	content = append(content, map[string]any{"type": "text", "text": prompt})
	for _, dataURL := range references {
		dataURL = strings.TrimSpace(dataURL)
		if strings.HasPrefix(dataURL, "data:image/") {
			content = append(content, map[string]any{
				"type":      "image_url",
				"image_url": map[string]string{"url": dataURL},
			})
		}
	}
	if len(content) == 1 {
		messages = append(messages, map[string]any{"role": "user", "content": prompt})
	} else {
		messages = append(messages, map[string]any{"role": "user", "content": content})
	}
	return messages
}

func extractChatMessage(responseBody string) string {
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return responseBody
	}
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content
	}
	return responseBody
}

func normalizeWorkflowDraft(content string, scope string) (any, []string, error) {
	content = strings.TrimSpace(content)
	jsonStart := strings.Index(content, "{")
	if jsonStart < 0 {
		jsonStart = strings.Index(content, "[")
	}
	if jsonStart >= 0 {
		content = content[jsonStart:]
	}
	jsonEnd := strings.LastIndex(content, "}")
	if bracketEnd := strings.LastIndex(content, "]"); bracketEnd > jsonEnd {
		jsonEnd = bracketEnd
	}
	if jsonEnd >= 0 {
		content = content[:jsonEnd+1]
	}

	var draft map[string]any
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		return nil, nil, safeMessageError{message: "工作流 Agent 返回内容格式异常，请重试"}
	}

	warnings := []string{}
	if scope != "public" {
		draft["scope"] = "private"
	}

	// Sanitize variable keys: enforce [a-zA-Z0-9_-]
	if variables, ok := draft["variables"].([]any); ok {
		for i, v := range variables {
			if vmap, ok := v.(map[string]any); ok {
				if key, ok := vmap["key"].(string); ok {
					vmap["key"] = sanitizeVariableKey(key)
				}
				variables[i] = vmap
			}
		}
		draft["variables"] = variables
	}

	return draft, warnings, nil
}

func sanitizeVariableKey(key string) string {
	var result strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	out := result.String()
	if out == "" {
		return "var"
	}
	return out
}

func readChannelError(body string, fallback string) safeMessageError {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err == nil {
		if strings.TrimSpace(payload.Error.Message) != "" {
			return safeMessageError{message: payload.Error.Message}
		}
		if strings.TrimSpace(payload.Msg) != "" {
			return safeMessageError{message: payload.Msg}
		}
	}
	return safeMessageError{message: fallback}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}


