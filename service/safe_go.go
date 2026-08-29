package service

import (
	"log"
	"runtime/debug"
)

// SafeGo 启动一个后台 goroutine 并捕获 panic（2026-08-17 引入）。
//
// 设计动机：video_task 和 storyboard_task 的后台轮询 worker（service/video_task.go runVideoTaskPoller、
// service/storyboard_task.go runStoryboardTaskLoop）内部会调上游 vendor Adapter 上游 HTTP 响应解析；
// 上游响应格式变化或 nil pointer dereference 时会 panic，goroutine 直接挂掉，任务永久卡在 "processing"。
//
// 用法：
//
//	SafeGo("video-task-poller:"+task.ID, func(r any) {
//	    // panic 兜底：标记 task 失败，避免永久 stuck。
//	    _ = UpdateVideoTaskFromPoll(task, VideoTaskPollUpdate{Status: "failed", ErrorDetail: fmt.Sprintf("panic: %v", r)})
//	}, func() {
//	    defer inFlight.Delete(task.ID)
//	    // 业务逻辑
//	})
//
// 注意：
//   - fn 内部的 defer 仍会按 Go runtime 语义先跑（在 recover 之前）。
//   - onPanic 是在 recover 之后跑，fn 自己的 defer 已结束；如需 inFlight.Delete 之类的清理，放 fn 里 defer。
//   - SafeGo 不阻塞调用方。
func SafeGo(name string, onPanic func(r any), fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic-recover] worker=%s panic=%v\n%s", name, r, debug.Stack())
				if onPanic != nil {
					onPanic(r)
				}
			}
		}()
		fn()
	}()
}
