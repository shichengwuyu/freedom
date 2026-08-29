package handler

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// PR-6: copyAIResponseBody 的写失败分支必须返回 errClientWriteFailed 哨兵。
// 复制这个文件是为了锁定：1) 正常 EOF → nil；2) client write 失败 → errClientWriteFailed；
// 3) 上游 Read 失败 → 错误原样上抛（不是哨兵）。后两点决定 hold 走 cancel 还是 settle。
func TestCopyAIResponseBodyHappyPath(t *testing.T) {
	rec := httptest.NewRecorder()
	written, err := copyAIResponseBody(rec, strings.NewReader("hello world"))
	if err != nil {
		t.Fatalf("happy path: err = %v, want nil", err)
	}
	if written != int64(len("hello world")) {
		t.Errorf("happy path: written = %d, want %d", written, len("hello world"))
	}
	if rec.Body.String() != "hello world" {
		t.Errorf("happy path: body = %q, want %q", rec.Body.String(), "hello world")
	}
}

// errWriter：写第一段后 fail，模拟 client 断开。
type errWriter struct {
	written int
	failAt  int
}

func (e *errWriter) Header() http.Header { return nil }
func (e *errWriter) Write(b []byte) (int, error) {
	e.written += len(b)
	if e.written >= e.failAt {
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}
func (e *errWriter) WriteHeader(statusCode int) {}

func TestCopyAIResponseBodyClientWriteFails(t *testing.T) {
	w := &errWriter{failAt: 5} // 第一段 (5 字节) 后 fail
	_, err := copyAIResponseBody(w, strings.NewReader("0123456789"))
	if err == nil {
		t.Fatalf("client write fail: expected err, got nil")
	}
	if !errors.Is(err, errClientWriteFailed) {
		t.Errorf("client write fail: err = %v, want errClientWriteFailed sentinel", err)
	}
}

// errReader：read 第一段后 fail，模拟上游断流。
type errReader struct {
	data   string
	read   int
	failAt int
	done   bool
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.done || e.read >= e.failAt {
		e.done = true
		return 0, io.ErrUnexpectedEOF
	}
	remaining := e.failAt - e.read
	if remaining > len(p) {
		remaining = len(p)
	}
	n := copy(p, e.data[e.read:e.read+remaining])
	e.read += n
	return n, nil
}

func TestCopyAIResponseBodyUpstreamReadFails(t *testing.T) {
	rec := httptest.NewRecorder()
	_, err := copyAIResponseBody(rec, &errReader{data: "0123456789", failAt: 3})
	if err == nil {
		t.Fatalf("upstream read fail: expected err, got nil")
	}
	if errors.Is(err, errClientWriteFailed) {
		t.Errorf("upstream read fail: err = %v, must NOT be errClientWriteFailed (would trigger wrong cancel path)", err)
	}
}
