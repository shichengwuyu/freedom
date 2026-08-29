package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

const (
	taskWorkerInterval = 5 * time.Second
	taskWorkerBatchSize = 50
)

// ErrTaskNotSupported task handler 不支持此 task（vendor 嗅探契约缺失等）。
// 上层可据此 fallback 到其他 handler。
var ErrTaskNotSupported = errors.New("task handler not supported")

// TaskHandler 通用 task 处理器接口（Sprint 4 引入）。
//
// Submit 提交到上游（首次执行），返回 upstreamTaskID + error。
//   - err == ErrTaskNotSupported 时上层会 fallback 到其他 handler / 官方渠道。
//   - 成功时返回的 upstreamTaskID 写入 task.upstream_task_id 之类字段（v2 扩展）。
//
// Poll 轮询上游状态；返回 status / progress / result（result 任务成功时填）。
//
// Cancel 主动取消（用户点取消时调用）。
type TaskHandler interface {
	Submit(ctx context.Context, t *model.Task) (upstreamTaskID string, err error)
	Poll(ctx context.Context, t *model.Task) (status string, progress int, result json.RawMessage, err error)
	Cancel(ctx context.Context, t *model.Task) error
}

var (
	taskHandlers   = map[string]TaskHandler{} // key = task.Type
	taskHandlersMu sync.RWMutex
	taskWorkerOnce sync.Once
)

// RegisterTaskHandler 注册通用 task handler（main.go 启动期调用）。
func RegisterTaskHandler(taskType string, h TaskHandler) {
	taskHandlersMu.Lock()
	defer taskHandlersMu.Unlock()
	taskHandlers[taskType] = h
}

// getTaskHandler 查 handler；找不到返回 nil。
func getTaskHandler(taskType string) TaskHandler {
	taskHandlersMu.RLock()
	defer taskHandlersMu.RUnlock()
	return taskHandlers[taskType]
}

// StartTaskWorker 启动通用 task 后台 worker。Sprint 4 引入。
// 启动期由 main.go 调一次即可。
func StartTaskWorker() {
	taskWorkerOnce.Do(func() {
		go runTaskWorker()
	})
}

func runTaskWorker() {
	ticker := time.NewTicker(taskWorkerInterval)
	defer ticker.Stop()
	for range ticker.C {
		processTaskBatch()
	}
}

// processTaskBatch 拉一批 pending/running 的 task 逐个处理。
func processTaskBatch() {
	tasks, err := repository.ListPendingTasks(taskWorkerBatchSize)
	if err != nil {
		log.Printf("task worker: list pending tasks failed: %v", err)
		return
	}
	for i := range tasks {
		SafeGo("task-worker:"+tasks[i].ID, func(r any) {
			log.Printf("task worker panic: task=%s panic=%v\n%s", tasks[i].ID, r, debug.Stack())
		}, func() {
			processOneTask(&tasks[i])
		})
	}
}

// processOneTask 处理单个 task。
//
// 状态机：
//   - status=pending  → handler.Submit() → 成功后改 status=running；失败改 status=failure
//   - status=running  → handler.Poll()   → success/failure/继续 running
//   - attempts > MaxAttempts → status=failure + hold refund
func processOneTask(t *model.Task) {
	if t == nil {
		return
	}
	handler := getTaskHandler(t.Type)
	if handler == nil {
		log.Printf("task worker: no handler for type=%s task=%s", t.Type, t.ID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// attempts 自增（不论提交还是轮询都算一次尝试）
	_ = repository.IncrementTaskAttempts(t.ID)
	t.Attempts++

	// 超过最大尝试 → 标 failure + 退 hold
	if t.MaxAttempts > 0 && t.Attempts > t.MaxAttempts {
		_ = repository.UpdateTaskStatus(t.ID, model.TaskStatusFailure, t.Progress, "", "max attempts exceeded")
		if t.HoldID != "" {
			_ = CancelBalanceHold(t.HoldID)
		}
		return
	}

	switch t.Status {
	case model.TaskStatusPending:
		upstreamID, err := handler.Submit(ctx, t)
		if err != nil {
			errMsg := err.Error()
			if errors.Is(err, ErrTaskNotSupported) {
				errMsg = "vendor contract not available: " + errMsg
			}
			_ = repository.UpdateTaskStatus(t.ID, model.TaskStatusFailure, 0, "", errMsg)
			if t.HoldID != "" {
				_ = CancelBalanceHold(t.HoldID)
			}
			return
		}
		_ = upstreamID // Sprint 4 暂不存；v2 扩展加 task.UpstreamTaskID 字段
		_ = repository.UpdateTaskStatus(t.ID, model.TaskStatusRunning, 0, "", "")

	case model.TaskStatusRunning:
		status, progress, result, err := handler.Poll(ctx, t)
		if err != nil {
			// 轮询失败（网络）→ 保持 running 等下一轮；attempts 仍自增
			log.Printf("task worker: poll task=%s type=%s err=%v", t.ID, t.Type, err)
			return
		}
		_ = repository.UpdateTaskStatus(t.ID, status, progress, string(result), "")

		if status == model.TaskStatusSuccess {
			if t.HoldID != "" {
				_ = SettleBalanceHold(t.HoldID)
			}
		} else if status == model.TaskStatusFailure {
			if t.HoldID != "" {
				_ = CancelBalanceHold(t.HoldID)
			}
		}

	case model.TaskStatusSuccess, model.TaskStatusFailure, model.TaskStatusCanceled:
		// 终态，不再处理
	}
}
