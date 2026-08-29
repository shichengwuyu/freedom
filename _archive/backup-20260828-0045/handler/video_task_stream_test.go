package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// PR-7：video_task.go 改用 http.NewRequestWithContext 透传 r.Context() 后，
// 客户端断开时上游 HTTP 调用必须被取消（不再继续扣额度）。
// 这条用例只测 ctx 透传语义本身：构造一个会挂住的上游，client cancel 立刻报错。
func TestVideoProxyRequestHonorsClientContextCancel(t *testing.T) {
	// 1) 上游 server：阻塞直到 client 端关闭连接（用 select 等 cancel）
	upstreamReleased := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // 模拟"客户端断开后 server 也跟着退出"
		close(upstreamReleased)
		_, _ = io.WriteString(w, "{}")
	}))
	defer upstream.Close()

	// 2) 模拟 user ctx 取消（用户关浏览器 / abort fetch）
	userCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/videos", nil).WithContext(userCtx)
	req.Header.Set("Content-Type", "application/json")

	// 3) 用 video_task.go 同样的方式构造上游 request
	upReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, upstream.URL, nil)
	if err != nil {
		t.Fatalf("build upstream request: %v", err)
	}

	// 4) 触发 cancel,跑 Do()，必须立刻返回 ctx.Err 而不是等
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	_, err = client.Do(upReq)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected ctx-canceled error from Do(), got nil")
	}
	// 标准库错误是 *url.Error 包 context.Canceled; 用 err 超时不应超过 1s
	if elapsed > time.Second {
		t.Errorf("Do() returned after %v, want <1s (ctx cancel not honored)", elapsed)
	}
	select {
	case <-upstreamReleased:
		// ok: 上游 handler 的 r.Context().Done() 也确实被触发了
	case <-time.After(2 * time.Second):
		t.Error("upstream handler did not observe ctx cancel within 2s")
	}
}

// PR-7: video 轮询加了 60s 单次上限。验证 context.WithTimeout 行为本身被正确使用：
// 如果父 ctx 提前 cancel（worker shutdown），轮询 ctx 必须随之 cancel。
func TestVideoPollContextRespectsParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	pollCtx, cancelPoll := context.WithTimeout(parent, 60*time.Second)
	defer cancelPoll()

	select {
	case <-pollCtx.Done():
		t.Fatal("pollCtx done before parent cancel")
	default:
	}

	cancel() // 模拟 worker 收到 shutdown 信号

	select {
	case <-pollCtx.Done():
		// ok: parent cancel 透传到 child
	case <-time.After(time.Second):
		t.Fatal("pollCtx did not observe parent cancel within 1s")
	}
}
