package service

import (
	"strings"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/google/uuid"
)

type CanvasAudioTaskCreateInput struct {
	UserID          string
	UserDisplayName string
	SourceID        string
	NodeID          string
	ClientTaskID    string
	Model           string
	ChannelID       string
	UserChannelID   string
	ChannelName     string
	Prompt          string
	Endpoint        string
	ContentType     string
	RequestBody     string
}

func CreateCanvasAudioTask(input CanvasAudioTaskCreateInput) (model.CanvasAudioTask, error) {
	current := now()
	task := model.CanvasAudioTask{
		// PR-8：主键始终服务端生成，禁止把客户端可控的 ClientTaskID 当作 id，
		// 否则不同用户传相同 clientTaskId 会互相覆盖。
		// ClientTaskID 字段仍保留作为前端去重 / 幂等查询的辅助键。
		ID:              newCanvasAudioTaskID(),
		UserID:          strings.TrimSpace(input.UserID),
		UserDisplayName: strings.TrimSpace(input.UserDisplayName),
		Source:          "canvas",
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
		Endpoint:        strings.TrimSpace(input.Endpoint),
		ContentType:     strings.TrimSpace(input.ContentType),
		RequestBody:     input.RequestBody,
		CreatedAt:       current,
		UpdatedAt:       current,
	}
	return repository.SaveCanvasAudioTask(task)
}

func GetUserCanvasAudioTask(userID string, id string) (model.CanvasAudioTask, bool, error) {
	return repository.GetUserCanvasAudioTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func SaveCanvasAudioTask(task model.CanvasAudioTask) (model.CanvasAudioTask, error) {
	task.UpdatedAt = now()
	return repository.UpdateCanvasAudioTask(task)
}

// PR-8: 服务端生成的主键，参见 CreateCanvasAudioTask 注释。
func newCanvasAudioTaskID() string {
	return "canvas_audio_task_" + uuid.NewString()
}

func CanvasAudioTaskResponse(task model.CanvasAudioTask) map[string]any {
	result := map[string]any{
		"id":           task.ID,
		"object":       "canvas.audio.task",
		"source":       task.Source,
		"source_id":    task.SourceID,
		"node_id":      task.NodeID,
		"model":        task.Model,
		"status":       task.Status,
		"progress":     task.Progress,
		"prompt":       task.Prompt,
		"created_at":   task.CreatedAt,
		"updated_at":   task.UpdatedAt,
		"started_at":   task.StartedAt,
		"completed_at": task.CompletedAt,
		"createdAt":    task.CreatedAt,
		"updatedAt":    task.UpdatedAt,
	}
	if task.AudioURL != "" {
		result["url"] = task.AudioURL
		result["audio_url"] = task.AudioURL
		result["storageKey"] = task.StorageKey
		result["mimeType"] = task.MimeType
		result["bytes"] = task.Bytes
	}
	if task.Error != "" || task.ErrorDetail != "" {
		result["error"] = map[string]any{"message": firstVideoTaskValue(task.Error, task.ErrorDetail)}
		result["error_detail"] = task.ErrorDetail
	}
	return result
}
