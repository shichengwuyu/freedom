package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: composition-layer 调度层 ===
//
// 与 novel_composition.go（5 步 ffmpeg 实现）配套：调度层负责 task 的 CRUD + 状态推进。
// v2 同步执行；v3 接通用 task worker 派发。

// CreateCompositionTask 新建任务（status=queued）。
func CreateCompositionTask(userID, projectID, workflowRunID, workflowNodeID string, input CompositionInput) (*model.CompositionTask, error) {
	if userID == "" || projectID == "" {
		return nil, errors.New("userID/projectID required")
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	task := &model.CompositionTask{
		ID:             newID("ct"),
		UserID:         userID,
		ProjectID:      projectID,
		WorkflowRunID:  workflowRunID,
		WorkflowNodeID: workflowNodeID,
		InputJSON:      string(inputJSON),
		Status:         string(statusQueued),
		ProgressJSON:   fmt.Sprintf(`{"currentStep":0,"totalSteps":5,"lastMessage":"任务已提交，等待 worker"}`),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repository.CreateCompositionTask(task); err != nil {
		return nil, err
	}
	return task, nil
}

// RunCompositionTask 启动合成（v2 同步执行）。
func RunCompositionTask(ctx context.Context, task *model.CompositionTask) error {
	if task == nil {
		return errors.New("task required")
	}
	// v2 简化：直接调 ComposeFull（内部串行跑 5 步）
	return ComposeFull(ctx, task)
}

// CancelCompositionTask 取消（仅标 canceled；v2 不真 kill ffmpeg）。
func CancelCompositionTask(id, userID string) error {
	task, err := repository.GetCompositionTask(id, userID)
	if err != nil {
		return err
	}
	if task == nil {
		return errors.New("任务不存在")
	}
	if task.Status != string(statusQueued) && task.Status != string(statusRunning) {
		return errors.New("任务不在活跃状态")
	}
	task.Status = string(statusCanceled)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	task.UpdatedAt = now
	task.CompletedAt = now
	return repository.UpdateCompositionTask(task)
}

// GetCompositionTask 取一条（带 user 防御）。
func GetCompositionTask(id, userID string) (*model.CompositionTask, error) {
	return repository.GetCompositionTask(id, userID)
}
