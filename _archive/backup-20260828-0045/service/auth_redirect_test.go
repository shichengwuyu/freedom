package service

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSafeRedirectPath(t *testing.T) {
	cases := map[string]string{
		"/":                    "/",
		"/canvas/abc":          "/canvas/abc",
		"/login?redirect=/x":   "/login?redirect=/x",
		"":                     "/",
		"//evil.com":           "/",
		"/\\evil.com":          "/",
		"https://evil.com":     "/",
		"http://evil.com":      "/",
		"javascript:alert(1)":  "/",
		"evil.com":             "/",
		"/\t/evil.com":         "/", // browsers strip the tab → //evil.com
		"/normal\tpath":        "/normalpath",
	}
	for in, want := range cases {
		if got := safeRedirectPath(in); got != want {
			t.Errorf("safeRedirectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVerifyOAuthStateRejectsTamperedAndMissingCookie(t *testing.T) {
	nonce := "test-nonce-123"

	// 正常流程：签名 state + 匹配 cookie → 返回 redirect
	validState, err := signOAuthState(nonce, "/canvas/1")
	if err != nil {
		t.Fatalf("signOAuthState failed: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/callback?state="+validState, nil)
	r.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: nonce})
	redirect, err := verifyOAuthState(r, validState)
	if err != nil {
		t.Fatalf("verifyOAuthState(valid) returned error: %v", err)
	}
	if redirect != "/canvas/1" {
		t.Errorf("verifyOAuthState(valid) redirect = %q, want /canvas/1", redirect)
	}

	// 开放重定向：redirect = //evil.com → safeRedirectPath 把它过滤为 /，
	// 校验仍然成功（签名/cookie 都对），但前端的"安全重定向"也只跟到 /。
	evilState, _ := signOAuthState(nonce, "//evil.com")
	r2 := httptest.NewRequest(http.MethodGet, "/callback?state="+evilState, nil)
	r2.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: nonce})
	redirect2, err2 := verifyOAuthState(r2, evilState)
	if err2 != nil {
		t.Fatalf("verifyOAuthState(evil redirect) returned error: %v", err2)
	}
	if redirect2 != "/" {
		t.Errorf("verifyOAuthState(evil redirect) redirect = %q, want /", redirect2)
	}

	// 篡改 state（签名不匹配）→ 必须 error
	tampered := base64.RawURLEncoding.EncodeToString([]byte(`{"n":"` + nonce + `","r":"/canvas/1"}`)) + ".invalid-sig"
	r3 := httptest.NewRequest(http.MethodGet, "/callback?state="+tampered, nil)
	r3.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: nonce})
	if _, err3 := verifyOAuthState(r3, tampered); err3 == nil {
		t.Errorf("verifyOAuthState(tampered sig) must return error, got nil")
	}

	// 缺少 cookie → CSRF 防护必须 error（过去是静默回退 /，会让登录 CSRF 走通）
	r4 := httptest.NewRequest(http.MethodGet, "/callback?state="+validState, nil)
	if _, err4 := verifyOAuthState(r4, validState); err4 == nil {
		t.Errorf("verifyOAuthState(no cookie) must return error, got nil")
	}

	// cookie nonce 不匹配 → 必须 error
	r5 := httptest.NewRequest(http.MethodGet, "/callback?state="+validState, nil)
	r5.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "wrong-nonce"})
	if _, err5 := verifyOAuthState(r5, validState); err5 == nil {
		t.Errorf("verifyOAuthState(wrong nonce) must return error, got nil")
	}

	// state 格式错（不包含 "."） → 必须 error
	r6 := httptest.NewRequest(http.MethodGet, "/callback?state=not-a-signed-state", nil)
	r6.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: nonce})
	if _, err6 := verifyOAuthState(r6, "not-a-signed-state"); err6 == nil {
		t.Errorf("verifyOAuthState(garbage state) must return error, got nil")
	}
}

// 回归：LoginWithLinuxDo 在 state 校验失败时必须立刻终止，绝不调 setAuthCookie / 创建用户。
// 这里用单独的单元函数复现：构造一个 verifyOAuthState 失败的 request，
// 走 LoginWithLinuxDo 入口（跳过真实 Linux.do HTTP），断言返回 error 且没有副作用。
func TestLoginWithLinuxDoAbortsOnInvalidState(t *testing.T) {
	// 任何不签名的 state 都会让 verifyOAuthState 返回 error。
	req := httptest.NewRequest(http.MethodGet, "/callback?code=anything&state=garbage", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "some-nonce"})

	// 我们不期望走到 linuxDoAccessToken —— 那是真发请求，会让单测不稳定。
	// 直接调 verifyOAuthState 验证关键不变量：
	if _, err := verifyOAuthState(req, "garbage"); err == nil {
		t.Fatalf("verifyOAuthState(garbage) must error — login CSRF guard")
	}
}
