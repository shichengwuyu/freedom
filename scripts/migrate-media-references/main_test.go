package main

import "testing"

func TestRewriteURLs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "相对路径 + 单个 UUID",
			in:   `{"avatar":"/api/media/references/abc-123.png"}`,
			want: `{"avatar":"/api/v1/media/references/abc-123.png"}`,
		},
		{
			name: "已经是新格式，不动",
			in:   `{"avatar":"/api/v1/media/references/abc-123.png"}`,
			want: `{"avatar":"/api/v1/media/references/abc-123.png"}`,
		},
		{
			name: "绝对 URL https",
			in:   `{"url":"https://freedom.example.com/api/media/references/xyz.png"}`,
			want: `{"url":"https://freedom.example.com/api/v1/media/references/xyz.png"}`,
		},
		{
			name: "绝对 URL 带端口 http",
			in:   `{"url":"http://127.0.0.1:8080/api/media/references/xyz.png"}`,
			want: `{"url":"http://127.0.0.1:8080/api/v1/media/references/xyz.png"}`,
		},
		{
			name: "同字符串里出现两次",
			in:   `["/api/media/references/a.png","/api/media/references/b.mp4"]`,
			want: `["/api/v1/media/references/a.png","/api/v1/media/references/b.mp4"]`,
		},
		{
			name: "绝对 + 相对混在一段 JSON",
			in:   `{"wechatQr":"https://h.example.com/api/media/references/qr1.png","qqGroupQr":"/api/media/references/qr2.png"}`,
			want: `{"wechatQr":"https://h.example.com/api/v1/media/references/qr1.png","qqGroupQr":"/api/v1/media/references/qr2.png"}`,
		},
		{
			name: "不相关文本，不动",
			in:   `这是普通文本，没有 URL`,
			want: `这是普通文本，没有 URL`,
		},
		{
			name: "不替换 /api/v1 前缀已存在的（relRE 已排除 /v1/，但保险起见再验证）",
			in:   `"/api/v1/media/references/foo.png"`,
			want: `"/api/v1/media/references/foo.png"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rewriteURLs(c.in)
			if got != c.want {
				t.Errorf("rewriteURLs(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
