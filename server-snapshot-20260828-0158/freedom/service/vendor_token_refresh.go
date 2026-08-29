package service

import (
	"context"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"golang.org/x/sync/singleflight"
)

// tokenRefreshGroup 按 account.ID 合并并发的 Token 刷新请求，避免一次性 RefreshToken 被并发重复触发。
var tokenRefreshGroup singleflight.Group

// needsRefresh 判断是否需要刷新：无过期时间默认刷；过期前 5 分钟内也刷。
func needsRefresh(a *model.UserVendorAccount) bool {
	if a == nil || a.ID == "" {
		return false
	}
	if a.TokenExpiresAt == nil {
		return true
	}
	return time.Now().After(a.TokenExpiresAt.Add(-5 * time.Minute))
}

// NeedsVendorTokenRefresh 导出给 handler 层判断是否该触发刷新。
func NeedsVendorTokenRefresh(a *model.UserVendorAccount) bool {
	return needsRefresh(a)
}

// SingleflightRefreshToken 并发安全的刷新入口：同一 account.ID 同时只执行一次 RefreshAccessToken。
// 刷新成功后回写数据库（adapter.RefreshAccessToken 负责修改 account 字段，本函数负责落库）。
func SingleflightRefreshToken(ctx context.Context, account *model.UserVendorAccount, adapter VendorAdapter) error {
	if account == nil || account.ID == "" {
		return nil
	}
	_, err, _ := tokenRefreshGroup.Do(account.ID, func() (any, error) {
		// 双检查：等锁过程中可能已被其他 goroutine 刷新好了
		if !needsRefresh(account) {
			return nil, nil
		}
		if err := adapter.RefreshAccessToken(ctx, account); err != nil {
			return nil, err
		}
		account.LastUsedAt = time.Now()
		if _, saveErr := repository.SaveUserVendorAccount(*account); saveErr != nil {
			return nil, saveErr
		}
		return nil, nil
	})
	return err
}
