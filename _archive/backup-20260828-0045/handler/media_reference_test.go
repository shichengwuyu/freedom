package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestNormalizeReferenceMediaTypeSupportsAudio(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		ext         string
		wantMime    string
		wantExt     string
	}{
		{name: "mp3 mime", contentType: "audio/mpeg", ext: ".bin", wantMime: "audio/mpeg", wantExt: ".mp3"},
		{name: "wav ext fallback", contentType: "application/octet-stream", ext: ".wav", wantMime: "audio/wav", wantExt: ".wav"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mimeType, ext, ok := normalizeReferenceMediaType(tt.contentType, tt.ext)
			if !ok {
				t.Fatal("expected media type to be accepted")
			}
			if mimeType != tt.wantMime || ext != tt.wantExt {
				t.Fatalf("got (%q, %q), want (%q, %q)", mimeType, ext, tt.wantMime, tt.wantExt)
			}
		})
	}
}

func TestReferenceMediaTypeMaxBytes(t *testing.T) {
	if got := referenceMediaTypeMaxBytes("audio/mpeg"); got != referenceAudioMaxBytes {
		t.Fatalf("audio max bytes = %d, want %d", got, referenceAudioMaxBytes)
	}
	if got := referenceMediaTypeMaxBytes("video/mp4"); got != referenceVideoMaxBytes {
		t.Fatalf("video max bytes = %d, want %d", got, referenceVideoMaxBytes)
	}
	if got := referenceMediaTypeMaxBytes("image/png"); got != referenceImageMaxBytes {
		t.Fatalf("image max bytes = %d, want %d", got, referenceImageMaxBytes)
	}
}

func TestReferenceMediaDirReturnsDefault(t *testing.T) {
	want := filepath.Join("data", "reference-media")
	if got := referenceMediaDir(); got != want {
		t.Fatalf("referenceMediaDir = %q, want %q", got, want)
	}
}

// PR-5: 即使读路由已搬到 /api/v1 (登录后)，handler 自身仍要拒绝路径遍历 id —— 这是
// 一道独立防御层，防止未来若有别的入口（admin 调试、批量回填脚本）直接调到 handler 时漏掉校验。
func TestReferenceMediaRejectsPathTraversal(t *testing.T) {
	cases := []string{
		"",
		"..",
		"../etc/passwd",
		"/etc/passwd",
		"a/b.png",
		"a\\b.png",
	}
	for _, id := range cases {
		// 直接用空 recorder 不必构造完整 http 流量：handler 命中非法 id 时只调 http.NotFound，
		// 不会触碰文件系统。期望所有非法 id 都进入 404 分支。
		req := httptest.NewRequest("GET", "/api/v1/media/references/"+id, nil)
		rec := httptest.NewRecorder()
		ReferenceMedia(rec, req, id)
		if rec.Code != http.StatusNotFound {
			t.Errorf("id=%q expected 404, got %d", id, rec.Code)
		}
	}
}
