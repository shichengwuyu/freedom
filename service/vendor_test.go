package service

import (
	"testing"

	"github.com/tigerowo/freedom/model"
)

// 覆盖 vendor normalize 的关键路径：DB 缺行 / DB 默认值 cookie 历史值 / 空值兜底 / Dispatch 路径行为。
// vendor normalize 是整套鉴权系统的命门，所有新增 vendor 入口必须先过 normalizeVendorAuthMode。

func TestDefaultVendorAuthMode(t *testing.T) {
	cases := []struct {
		vendorType string
		want       string
	}{
		{model.VendorTypeLibTV, model.VendorAuthModeCustomHeader},
		{model.VendorTypeNewWow, model.VendorAuthModeCustomHeader},
		{model.VendorTypeUpDream, model.VendorAuthModeCustomHeader},
		{model.VendorTypeOfficial, model.VendorAuthModeCookie},
		{"unknown-vendor", model.VendorAuthModeCookie},
	}
	for _, c := range cases {
		got := defaultVendorAuthMode(c.vendorType)
		if got != c.want {
			t.Errorf("defaultVendorAuthMode(%q) = %q, want %q", c.vendorType, got, c.want)
		}
	}
}

func TestDefaultVendorAuthHeaderName(t *testing.T) {
	cases := []struct {
		vendorType string
		want       string
	}{
		{model.VendorTypeNewWow, "accesstoken"},
		{model.VendorTypeLibTV, "Token"},
		{model.VendorTypeUpDream, "Authorization"},
		{model.VendorTypeOfficial, ""},
		{"unknown-vendor", ""},
	}
	for _, c := range cases {
		got := defaultVendorAuthHeaderName(c.vendorType)
		if got != c.want {
			t.Errorf("defaultVendorAuthHeaderName(%q) = %q, want %q", c.vendorType, got, c.want)
		}
	}
}

func TestNormalizeVendorAuthMode_NilDBRow_ReturnsInMemoryDefault(t *testing.T) {
	// DB 没记录（罕见，但 vendor 表空时可能）：构造 in-memory 默认 vendor 走 defaultVendorAuthMode
	got := normalizeVendorAuthMode(model.VendorTypeNewWow, nil)
	if got == nil {
		t.Fatalf("normalizeVendorAuthMode returned nil; want in-memory default")
	}
	if got.Type != model.VendorTypeNewWow {
		t.Errorf("Type = %q, want %q", got.Type, model.VendorTypeNewWow)
	}
	if got.AuthMode != model.VendorAuthModeCustomHeader {
		t.Errorf("AuthMode = %q, want %q", got.AuthMode, model.VendorAuthModeCustomHeader)
	}
	if got.AuthHeaderName != "accesstoken" {
		t.Errorf("AuthHeaderName = %q, want %q", got.AuthHeaderName, "accesstoken")
	}
}

func TestNormalizeVendorAuthMode_CookieHistorical_CorrectedForCustomHeaderVendor(t *testing.T) {
	// 历史 DB row：AuthMode 字段 gorm default:'cookie' 落下来的旧值。
	// NewWow 应该是 custom_header → 必须被矫正。
	v := &model.Vendor{Type: model.VendorTypeNewWow, AuthMode: "cookie", AuthHeaderName: ""}
	got := normalizeVendorAuthMode(model.VendorTypeNewWow, v)
	if got.AuthMode != model.VendorAuthModeCustomHeader {
		t.Errorf("NewWow historical cookie AuthMode not corrected: got %q", got.AuthMode)
	}
	if got.AuthHeaderName != "accesstoken" {
		t.Errorf("NewWow AuthHeaderName not filled: got %q", got.AuthHeaderName)
	}
}

func TestNormalizeVendorAuthMode_CookieKeptForCookieVendor(t *testing.T) {
	// Official 默认就是 cookie：DB 里 cookie 是正确的，不应被误改成 custom_header。
	v := &model.Vendor{Type: model.VendorTypeOfficial, AuthMode: "cookie"}
	got := normalizeVendorAuthMode(model.VendorTypeOfficial, v)
	if got.AuthMode != model.VendorAuthModeCookie {
		t.Errorf("Official cookie mode should be kept; got %q", got.AuthMode)
	}
	if got.AuthHeaderName != "" {
		t.Errorf("Official AuthHeaderName should stay empty; got %q", got.AuthHeaderName)
	}
}

func TestNormalizeVendorAuthMode_EmptyAuthMode_FilledByDefault(t *testing.T) {
	// 新建 vendor 行时 AuthMode 字段未填：应落到 vendor type 对应的 default。
	v := &model.Vendor{Type: model.VendorTypeLibTV, AuthMode: ""}
	got := normalizeVendorAuthMode(model.VendorTypeLibTV, v)
	if got.AuthMode != model.VendorAuthModeCustomHeader {
		t.Errorf("empty AuthMode for LibTV should fill custom_header; got %q", got.AuthMode)
	}
	if got.AuthHeaderName != "Token" {
		t.Errorf("LibTV AuthHeaderName should be Token; got %q", got.AuthHeaderName)
	}
}

func TestNormalizeVendorAuthModeForDispatch_NilRow_ForceEnabledAndAPIRoot(t *testing.T) {
	// Dispatch 路径：DB 没记录的用户已经绑定了该供应商，不应被 !vendor.Enabled 早返回挡掉。
	got := NormalizeVendorAuthModeForDispatch(model.VendorTypeNewWow, nil)
	if got == nil {
		t.Fatalf("NormalizeVendorAuthModeForDispatch returned nil")
	}
	if !got.Enabled {
		t.Errorf("dispatch nil vendor should force Enabled=true")
	}
	if got.APIRootURL == "" {
		t.Errorf("dispatch nil vendor should fill default APIRootURL; got empty")
	}
	if got.AuthMode != model.VendorAuthModeCustomHeader {
		t.Errorf("dispatch nil vendor AuthMode = %q, want %q", got.AuthMode, model.VendorAuthModeCustomHeader)
	}
}

func TestNormalizeVendorAuthModeForDispatch_DBDisabled_Respected(t *testing.T) {
	// Dispatch 路径：DB 有显式 Enabled=false 行，管理员明确停用，不应被强制翻成 true。
	v := &model.Vendor{Type: model.VendorTypeNewWow, Enabled: false, AuthMode: model.VendorAuthModeCustomHeader, AuthHeaderName: "accesstoken"}
	got := NormalizeVendorAuthModeForDispatch(model.VendorTypeNewWow, v)
	if got.Enabled {
		t.Errorf("dispatch with DB Enabled=false should stay disabled")
	}
}
