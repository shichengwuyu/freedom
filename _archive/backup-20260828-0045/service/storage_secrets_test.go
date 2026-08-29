package service

import "testing"

// PR-10：GET /api/v1/user-config 的 storageProvider 节点响应前必须清空明文凭据。
// 这是 P1-11 plan 修复的关键点——之前 syncFlags 只控制整节点是否返回，但 secret 字段
// 本身仍被原样透出，任何登录用户调一次 GET 就能拿到完整 AK/SK / WebDAV 密码。
func TestMaskStorageProviderSecretsClearsBoth(t *testing.T) {
	enabled := true
	providers := &UserStorageProviders{
		S3: &StorageObjectProviderInput{
			Enabled:         &enabled,
			Name:            "my-s3",
			Endpoint:        "https://s3.example.com",
			Bucket:          "media",
			AccessKeyID:     "AKIA-public",
			SecretAccessKey: "VERY-SECRET-AK",
		},
		WebDAV: &StorageObjectProviderInput{
			Enabled:  &enabled,
			Name:     "my-webdav",
			Endpoint: "https://dav.example.com",
			Username: "alice",
			Password: "VERY-SECRET-PWD",
		},
	}
	maskStorageProviderSecrets(providers)

	// 1) 两个 secret 字段被清空
	if providers.S3.SecretAccessKey != "" {
		t.Errorf("S3.SecretAccessKey not cleared: %q", providers.S3.SecretAccessKey)
	}
	if providers.WebDAV.Password != "" {
		t.Errorf("WebDAV.Password not cleared: %q", providers.WebDAV.Password)
	}
	// 2) 非 secret 字段保持不变（前端仍要展示"已配置"状态）
	if providers.S3.AccessKeyID != "AKIA-public" {
		t.Errorf("S3.AccessKeyID should be preserved, got %q", providers.S3.AccessKeyID)
	}
	if providers.S3.Endpoint != "https://s3.example.com" {
		t.Errorf("S3.Endpoint should be preserved, got %q", providers.S3.Endpoint)
	}
	if providers.WebDAV.Username != "alice" {
		t.Errorf("WebDAV.Username should be preserved, got %q", providers.WebDAV.Username)
	}
	if providers.WebDAV.Endpoint != "https://dav.example.com" {
		t.Errorf("WebDAV.Endpoint should be preserved, got %q", providers.WebDAV.Endpoint)
	}
}

// 边界: 只有一个 provider 被配置时，nil 那一侧不能 panic
func TestMaskStorageProviderSecretsHandlesNil(t *testing.T) {
	t.Run("only S3", func(t *testing.T) {
		p := &UserStorageProviders{S3: &StorageObjectProviderInput{SecretAccessKey: "x"}}
		maskStorageProviderSecrets(p)
		if p.S3.SecretAccessKey != "" {
			t.Error("S3 secret not cleared")
		}
	})
	t.Run("only WebDAV", func(t *testing.T) {
		p := &UserStorageProviders{WebDAV: &StorageObjectProviderInput{Password: "x"}}
		maskStorageProviderSecrets(p)
		if p.WebDAV.Password != "" {
			t.Error("WebDAV password not cleared")
		}
	})
	t.Run("empty providers", func(t *testing.T) {
		p := &UserStorageProviders{}
		maskStorageProviderSecrets(p) // 不能 panic
	})
	t.Run("nil pointer", func(t *testing.T) {
		maskStorageProviderSecrets(nil) // 不能 panic
	})
}

// PR-10 配套: SaveCurrentUserStorageProvider 必须在 incoming secret 为空 + 原值非空时保留原值。
// 反向: 显式传新 secret 必须照常覆盖（用户改密码时不能被旧值"粘住"）。
func TestMergeStorageProviderSecretsKeepsExistingWhenIncomingEmpty(t *testing.T) {
	enabled := true
	existing := &UserStorageProviders{
		S3: &StorageObjectProviderInput{SecretAccessKey: "ORIGINAL-S3-SECRET"},
		WebDAV: &StorageObjectProviderInput{Password: "ORIGINAL-WEBDAV-PWD"},
	}
	_ = enabled
	// 场景 1: 前端编辑 endpoint 但 secret 字段不传（典型用法）
	incoming := &UserStorageProviders{
		S3:     &StorageObjectProviderInput{Endpoint: "https://new-s3.example.com", SecretAccessKey: ""},
		WebDAV: &StorageObjectProviderInput{Endpoint: "https://new-dav.example.com", Password: ""},
	}
	mergeStorageProviderSecrets(existing, incoming)
	if incoming.S3.SecretAccessKey != "ORIGINAL-S3-SECRET" {
		t.Errorf("S3 secret lost: got %q, want ORIGINAL-S3-SECRET", incoming.S3.SecretAccessKey)
	}
	if incoming.WebDAV.Password != "ORIGINAL-WEBDAV-PWD" {
		t.Errorf("WebDAV password lost: got %q, want ORIGINAL-WEBDAV-PWD", incoming.WebDAV.Password)
	}
	// 非 secret 字段以 incoming 为准（endpoint 真的换了）
	if incoming.S3.Endpoint != "https://new-s3.example.com" {
		t.Errorf("S3 endpoint should follow incoming, got %q", incoming.S3.Endpoint)
	}
}

func TestMergeStorageProviderSecretsOverridesWhenIncomingSet(t *testing.T) {
	existing := &UserStorageProviders{
		S3:     &StorageObjectProviderInput{SecretAccessKey: "OLD-S3"},
		WebDAV: &StorageObjectProviderInput{Password: "OLD-PWD"},
	}
	// 场景 2: 用户改密码 → 显式提供新 secret
	incoming := &UserStorageProviders{
		S3:     &StorageObjectProviderInput{SecretAccessKey: "NEW-S3"},
		WebDAV: &StorageObjectProviderInput{Password: "NEW-PWD"},
	}
	mergeStorageProviderSecrets(existing, incoming)
	if incoming.S3.SecretAccessKey != "NEW-S3" {
		t.Errorf("S3 secret override blocked: got %q, want NEW-S3", incoming.S3.SecretAccessKey)
	}
	if incoming.WebDAV.Password != "NEW-PWD" {
		t.Errorf("WebDAV password override blocked: got %q, want NEW-PWD", incoming.WebDAV.Password)
	}
}

// 边界: existing 没配置过该 provider，但 incoming 给了空 secret——保留为空（不报错）
func TestMergeStorageProviderSecretsNoExisting(t *testing.T) {
	existing := &UserStorageProviders{} // S3/WebDAV 都是 nil
	incoming := &UserStorageProviders{
		S3:     &StorageObjectProviderInput{SecretAccessKey: ""},
		WebDAV: &StorageObjectProviderInput{Password: ""},
	}
	mergeStorageProviderSecrets(existing, incoming) // 不能 panic
	if incoming.S3.SecretAccessKey != "" || incoming.WebDAV.Password != "" {
		t.Error("empty secret should stay empty when no existing")
	}
}

// 边界: incoming 是 nil（前端传空 body 时）
func TestMergeStorageProviderSecretsNilIncoming(t *testing.T) {
	existing := &UserStorageProviders{S3: &StorageObjectProviderInput{SecretAccessKey: "X"}}
	mergeStorageProviderSecrets(existing, nil) // 不能 panic
}
