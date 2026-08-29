package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetAuthCookieIsHttpOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	rec := httptest.NewRecorder()
	setAuthCookie(rec, req, "fake-jwt-token")

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != AuthCookieName {
		t.Errorf("cookie name = %q, want %q", c.Name, AuthCookieName)
	}
	if !c.HttpOnly {
		t.Errorf("setAuthCookie must set HttpOnly=true (got false); this enables XSS token theft otherwise")
	}
	if c.Value != "fake-jwt-token" {
		t.Errorf("cookie value = %q, want %q", c.Value, "fake-jwt-token")
	}
	if c.MaxAge <= 0 {
		t.Errorf("cookie MaxAge = %d, want > 0", c.MaxAge)
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", c.SameSite)
	}
}

func TestSetAuthCookieSecureOnTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	setAuthCookie(rec, req, "x")
	if got := rec.Result().Cookies()[0].Secure; !got {
		t.Errorf("setAuthCookie with X-Forwarded-Proto=https must set Secure=true")
	}
}

func TestClearAuthCookieIsHttpOnlyAndExpired(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()
	clearAuthCookie(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != AuthCookieName {
		t.Errorf("cookie name = %q, want %q", c.Name, AuthCookieName)
	}
	if !c.HttpOnly {
		t.Errorf("clearAuthCookie must set HttpOnly=true to match setAuthCookie")
	}
	if c.Value != "" {
		t.Errorf("clearAuthCookie value = %q, want empty", c.Value)
	}
	if c.MaxAge >= 0 {
		t.Errorf("clearAuthCookie MaxAge = %d, want < 0", c.MaxAge)
	}
}

func TestLogoutClearsAuthCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	// 不带 cookie 的请求直接调 handler：handler 不读 cookie，只负责写清除 cookie。
	rec := httptest.NewRecorder()
	Logout(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Logout status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Logout Content-Type = %q, want application/json", got)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Logout must set 1 clearing cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != AuthCookieName {
		t.Errorf("Logout cookie name = %q, want %q", c.Name, AuthCookieName)
	}
	if c.MaxAge >= 0 {
		t.Errorf("Logout cookie MaxAge = %d, want < 0", c.MaxAge)
	}
	if !c.HttpOnly {
		t.Errorf("Logout clearing cookie must be HttpOnly=true")
	}
}

// tlsConnectionState 用最小可用类型避免在测试里引入 crypto/tls 复杂包。
// (unused after switching to X-Forwarded-Proto header to fake HTTPS)
