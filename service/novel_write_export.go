package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// EXPORT_INSTRUCTION 追加到 system prompt 后，强制 AI 输出标准 JSON 格式。
// 关键约束：shots 必须非空、shot_id 必须唯一、duration_sec ∈ [1, 60]。
const EXPORT_INSTRUCTION = `

[重要指令] 上面是用户与你的对话历史，包含小说设定和分镜内容。请根据对话内容，把其中的分镜部分抽取为标准 JSON 格式输出。
JSON 必须严格符合以下结构，不要包含其他文字、markdown 围栏或解释：

{
  "novel_name": "小说名",
  "protagonist": "主角名",
  "shots": [
    {
      "shot_id": "shot-1",
      "scene": "场景描述",
      "duration_sec": 5,
      "description": "镜头描述（画面/构图/运镜）",
      "dialogue": "对白（可空字符串）"
    }
  ]
}

要求：
- shots 数组至少 1 个元素
- shot_id 格式为 "shot-N"（N 从 1 开始递增）
- shot_id 必须唯一
- duration_sec 范围 1-60 秒
- 只输出 JSON，不要任何前缀或后缀文字`

// storyboardJSON 解析 AI 输出的分镜结构。
type storyboardJSON struct {
	NovelName   string `json:"novel_name"`
	Protagonist string `json:"protagonist"`
	Shots       []struct {
		ShotID      string `json:"shot_id"`
		Scene       string `json:"scene"`
		DurationSec int    `json:"duration_sec"`
		Description string `json:"description"`
		Dialogue    string `json:"dialogue"`
	} `json:"shots"`
}

// ExportNovelWriteStoryboard 导出分镜 JSON → canvas_project。
//
// 流程：
//  1. 拿 session 校验归属
//  2. 拼 export context（对话历史 + 变量替换后的 system prompt + EXPORT_INSTRUCTION）
//  3. 调 AI 抽 JSON
//  4. 解析 + 校验（shots 非空 / shot_id 唯一 / duration_sec ∈ [1, 60]）
//  5. 写 canvas_project（source="novel"）
//  6. 写 novel_write_exports 历史
//  7. 返回 {canvasProjectId, shotsCount}
func ExportNovelWriteStoryboard(ctx context.Context, userID, sessionID string) (canvasProjectID string, shotsCount int, err error) {
	// 1. 校验 session
	sess, err := repository.GetNovelWriteSession(sessionID, userID)
	if err != nil {
		return "", 0, err
	}
	if sess == nil {
		return "", 0, errors.New("会话不存在或无权访问")
	}

	// 2. 拼 export context
	history, err := repository.ListNovelWriteMessagesBySession(sessionID)
	if err != nil {
		return "", 0, err
	}
	contextText := buildExportContext(history)

	// 3. 拼 system prompt（变量替换 + 追加 EXPORT_INSTRUCTION）
	vars := model.UnmarshalVariables(sess.Variables)
	systemContent := substituteVariables(sess.SystemPrompt, vars) + EXPORT_INSTRUCTION

	messages := []map[string]any{
		{"role": "system", "content": systemContent},
		{"role": "user", "content": "以下是小说内容，请抽取分镜 JSON：\n\n" + contextText},
	}

	// 4. 调 AI
	rawJSON, err := callNovelWriteChat(ctx, sess.Model, messages)
	if err != nil {
		return "", 0, fmt.Errorf("调 AI 失败: %w", err)
	}

	// 5. 解析
	cleanJSON := stripJSONFence(rawJSON) // 兼容 AI 偶尔包 ```json``` 围栏
	var storyboard storyboardJSON
	if err := json.Unmarshal([]byte(cleanJSON), &storyboard); err != nil {
		return "", 0, fmt.Errorf("AI 输出不是合法 JSON: %w\nAI 原文: %s", err, truncateForError(rawJSON, 200))
	}

	// 6. 校验
	if len(storyboard.Shots) == 0 {
		return "", 0, errors.New("AI 未能抽取分镜（shots 为空），请调整 prompt 重试")
	}
	seen := map[string]bool{}
	for _, s := range storyboard.Shots {
		if s.ShotID == "" {
			return "", 0, fmt.Errorf("shot_id 不能为空")
		}
		if seen[s.ShotID] {
			return "", 0, fmt.Errorf("shot_id 重复: %s", s.ShotID)
		}
		seen[s.ShotID] = true
		if s.DurationSec < 1 || s.DurationSec > 60 {
			return "", 0, fmt.Errorf("shot %s duration_sec 越界 (1-60): %d", s.ShotID, s.DurationSec)
		}
	}

	// 7. 写 canvas_project
	canvasProjectID, err = upsertCanvasProjectForExport(userID, sess, &storyboard)
	if err != nil {
		return "", 0, err
	}

	// 8. 写 export 历史
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_ = repository.CreateNovelWriteExport(&model.NovelWriteExport{
		ID:              newID("nwe"),
		UserID:          userID,
		SessionID:       sessionID,
		CanvasProjectID: canvasProjectID,
		ShotsCount:      len(storyboard.Shots),
		ExportJSON:      cleanJSON,
		CreatedAt:       nowStr,
	})

	return canvasProjectID, len(storyboard.Shots), nil
}

// ListExports 列出当前用户的导出历史。
func ListNovelWriteExports(userID string, limit int) ([]model.NovelWriteExport, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return repository.ListNovelWriteExportsByUser(userID, limit)
}

// === helpers ===

// buildExportContext 把消息历史拼成可读文本（user/assistant 区分）。
func buildExportContext(history []model.NovelWriteMessage) string {
	var sb strings.Builder
	for _, m := range history {
		if m.Role == "system" {
			continue
		}
		if m.Role == "user" {
			sb.WriteString("[用户] ")
		} else {
			sb.WriteString("[助手] ")
		}
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// stripJSONFence 去掉 AI 输出里可能包着的 ```json ... ``` 围栏。
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// 去掉首行 ```json 或 ```
		firstNL := strings.Index(s, "\n")
		if firstNL > 0 {
			s = s[firstNL+1:]
		}
		// 去掉末尾 ```
		if idx := strings.LastIndex(s, "```"); idx > 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// truncateForError 截断长字符串用于错误信息（不与 vendor_libtv_task.go 的 truncate 冲突）。
func truncateForError(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// upsertCanvasProjectForExport 写 canvas_project（每次新建，复用 canvas_project 现有结构）。
//
// v1 简化：每次都新建 project，作家可手动删。
// 后续可改为"找/更新"复用。
func upsertCanvasProjectForExport(userID string, sess *model.NovelWriteSession, storyboard *storyboardJSON) (string, error) {
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	projectID := newID("cp")

	// Data 字段：把分镜 + 来源信息一起存
	projectData := map[string]any{
		"storyboard":             storyboard,
		"novelWriteSessionId":    sess.ID,
		"novelWriteSessionTitle": sess.Title,
	}
	dataJSON, _ := json.Marshal(projectData)

	cp := model.CanvasProject{
		ID:          projectID,
		UserID:      userID,
		ProjectData: string(dataJSON),
		CreatedAt:   nowStr,
		UpdatedAt:   nowStr,
	}
	if _, err := repository.SaveUserCanvasProject(cp); err != nil {
		return "", err
	}
	return projectID, nil
}
