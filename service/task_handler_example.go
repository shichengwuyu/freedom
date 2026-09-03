package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tigerowo/freedom/model"
)

// ExampleTaskHandler 通用 worker handler 的**完整实现模板**（Sprint 4.2 引入）。
//
// 设计目标：
//  1. 演示 TaskHandler 三个方法（Submit / Poll / Cancel）的正确写法
//  2. 演示状态机迁移：pending → running → success / failure / canceled
//  3. 演示 hold 集成：success → SettleHold，failure → CancelHold（worker 自动调）
//  4. 演示 vendor 契约缺失时的 ErrTaskNotSupported fallback 模式
//  5. 演示 panic 兜底：worker 端已用 SafeGo 包裹，本 handler 内部 panic 也会被捕获
//
// 注册方式（**仅测试 / demo 用；生产不要启用**）：
//
//	func init() {
//	    RegisterTaskHandler(model.TaskTypeImageBatch, &ExampleTaskHandler{})
//	}
//
// 或者在 main.go 启动期显式调：
//
//	service.RegisterTaskHandler(model.TaskTypeImageBatch, &ExampleTaskHandler{})
//
// 实际新能力接入时，**直接 fork 本文件**，把 ExampleTaskHandler 改成你的实现。
// 不需要重写 service/task_worker.go（worker 框架已完整）。
type ExampleTaskHandler struct{}

// ExamplePayload ExampleTaskHandler 接受的入参。
//
// 真实场景：vendor API 请求体 / 官方渠道 selector 输入。
type ExamplePayload struct {
	// SimulateFail true → Submit 立即返 ErrTaskNotSupported（演示 fallback）
	SimulateFail bool `json:"simulateFail"`
	// ImageCount 模拟"批处理图片数量"，决定 Poll 多少次后 success
	ImageCount int `json:"imageCount"`
}

// Submit 模拟"提交一个图片批处理任务"。
//
// 真实实现：调 vendor API / 调官方渠道 selector → 拿 upstreamTaskID → 写库。
func (h *ExampleTaskHandler) Submit(ctx context.Context, t *model.Task) (string, error) {
	// 1) 解析 PayloadJSON（task 输入）
	var payload ExamplePayload
	if err := json.Unmarshal([]byte(t.PayloadJSON), &payload); err != nil {
		return "", fmt.Errorf("example handler: parse payload: %w", err)
	}

	// 2) 默认 imageCount = 5
	if payload.ImageCount <= 0 {
		payload.ImageCount = 5
	}

	// 3) 模拟失败路径：演示 vendor 契约缺失时返 ErrTaskNotSupported
	if payload.SimulateFail {
		// 上层 worker 看到 ErrTaskNotSupported 会标 task = failure + 退 hold
		return "", ErrTaskNotSupported
	}

	// 4) 真实场景：调 vendor / 官方渠道 → 拿 task ID
	//    vendor 示例：return vendorClient.SubmitGenerateImage(...)
	//    官方示例：return service.PickChannelWithRetry(...).SubmitVideo(...).UpstreamTaskID
	upstreamTaskID := "example-" + t.ID
	return upstreamTaskID, nil
}

// Poll 模拟"轮询任务状态"。
//
// 真实实现：调 vendor API 查 status / progress / result。
func (h *ExampleTaskHandler) Poll(ctx context.Context, t *model.Task) (string, int, json.RawMessage, error) {
	var payload ExamplePayload
	if err := json.Unmarshal([]byte(t.PayloadJSON), &payload); err != nil {
		return model.TaskStatusFailure, t.Progress, nil, fmt.Errorf("example handler: parse payload: %w", err)
	}
	if payload.ImageCount <= 0 {
		payload.ImageCount = 5
	}

	// 1) 模拟进度：每调一次 Poll + (100 / imageCount)%
	step := 100 / payload.ImageCount
	newProgress := t.Progress + step
	if newProgress > 100 {
		newProgress = 100
	}

	// 2) 模拟延时：实际场景是 HTTP 请求；这里 0 延迟即可（worker 5s 一轮）
	_ = time.Millisecond

	if newProgress < 100 {
		return model.TaskStatusRunning, newProgress, nil, nil
	}

	// 3) 完成：构造模拟 result（真实场景是 vendor 返回的图片 URL 列表）
	result := json.RawMessage(fmt.Sprintf(`{"items":[{"url":"https://example.com/img-%s.png","width":1024,"height":1024}]}`, t.ID))
	return model.TaskStatusSuccess, 100, result, nil
}

// Cancel 模拟"取消任务"。
//
// 真实实现：调 vendor API cancel + 标记 task canceled + 退 hold（worker 端会处理）。
func (h *ExampleTaskHandler) Cancel(ctx context.Context, t *model.Task) error {
	// 真实场景：调 vendor API cancel 端点
	// vendor 示例：return vendorClient.CancelTask(ctx, account, t.UpstreamTaskID)
	// 官方场景：不需要（任务在官方侧无"取消"概念）
	return nil
}

// RegisterExampleTaskHandler 在 main.go 启动期显式调用的 helper。
// 不自动注册（避免污染生产；用户必须显式 opt-in）。
func RegisterExampleTaskHandler() {
	RegisterTaskHandler(model.TaskTypeImageBatch, &ExampleTaskHandler{})
}
