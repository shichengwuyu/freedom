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
	validState, err := signOAuthState(nonce, "/canvas/1", "invite123")
	if err != nil {
		t.Fatalf("signOAuthState failed: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/callback?state="+validState, nil)
	r.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: nonce})
	if got, _ := verifyOAuthState(r, validState); got != "/canvas/1" {
		t.Errorf("verifyOAuthState(valid) = %q, want /canvas/1", got)
	}

	// 开放重定向：redirect = //evil.com → 被安全过滤为 /
	evilState, _ := signOAuthState(nonce, "//evil.com", "")
	r2 := httptest.NewRequest(http.MethodGet, "/callback?state="+evilState, nil)
	r2.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: nonce})
	if got, _ := verifyOAuthState(r2, evilState); got != "/" {
		t.Errorf("verifyOAuthState(evil redirect) = %q, want /", got)
	}

	// 篡改 state（签名不匹配）
	tampered := base64.RawURLEncoding.EncodeToString([]byte(`{"n":"` + nonce + `","r":"/canvas/1"}`)) + ".invalid-sig"
	r3 := httptest.NewRequest(http.MethodGet, "/callback?state="+tampered, nil)
	r3.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: nonce})
	if got, _ := verifyOAuthState(r3, tampered); got != "/" {
		t.Errorf("verifyOAuthState(tampered sig) = %q, want /", got)
	}

	// 缺少 cookie → CSRF 防护触发
	r4 := httptest.NewRequest(http.MethodGet, "/callback?state="+validState, nil)
	if got, _ := verifyOAuthState(r4, validState); got != "/" {
		t.Errorf("verifyOAuthState(no cookie) = %q, want /", got)
	}

	// cookie nonce 不匹配
	r5 := httptest.NewRequest(http.MethodGet, "/callback?state="+validState, nil)
	r5.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "wrong-nonce"})
	if got, _ := verifyOAuthState(r5, validState); got != "/" {
		t.Errorf("verifyOAuthState(wrong nonce) = %q, want /", got)
	}
}
