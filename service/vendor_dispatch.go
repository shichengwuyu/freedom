package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// GetActiveVendorAccount 取当前用户激活的供应商账户（包装 repository，供 handler 直接调用）。
func GetActiveVendorAccount(userID string) (*model.UserVendorAccount, error) {
	return repository.GetActiveVendorAccount(userID)
}

// GetVendorByType 按类型取供应商元信息（包装 repository）。
func GetVendorByType(t string) (*model.Vendor, error) {
	return repository.GetVendorByType(t)
}

// vendorModelsSnapshot 模型快照结构（与文档 §3.3 对齐，前端 buildVendorEffectiveConfig 直接消费）。
type vendorModelsSnapshot struct {
	ImageModels []map[string]any `json:"imageModels"`
	VideoModels []map[string]any `json:"videoModels"`
	TextModels  []map[string]any `json:"textModels"`
	AudioModels []map[string]any `json:"audioModels"`
	ModelLabels map[string]string `json:"modelLabels,omitempty"`
	FetchedAt   string           `json:"fetchedAt"`
}

func modelInfoToMap(m VendorModelInfo) map[string]any {
	out := map[string]any{
		"id":         m.ID,
		"name":       m.Name,
		"capability": m.Capability,
	}
	if m.DefaultFor != "" {
		out["defaultFor"] = m.DefaultFor
	}
	if len(m.Supports) > 0 {
		out["supports"] = m.Supports
	}
	if len(m.Constraints) > 0 {
		out["constraints"] = m.Constraints
	}
	if len(m.Extra) > 0 {
		out["extra"] = m.Extra
	}
	return out
}

func vendorModelsToSnapshot(models *VendorModels) vendorModelsSnapshot {
	snap := vendorModelsSnapshot{
		ImageModels: []map[string]any{},
		VideoModels: []map[string]any{},
		TextModels:  []map[string]any{},
		AudioModels: []map[string]any{},
		ModelLabels: map[string]string{},
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if models == nil {
		return snap
	}
	for _, m := range models.ImageModels {
		snap.ImageModels = append(snap.ImageModels, modelInfoToMap(m))
		if m.Name != "" {
			snap.ModelLabels[m.ID] = m.Name
		}
	}
	for _, m := range models.VideoModels {
		snap.VideoModels = append(snap.VideoModels, modelInfoToMap(m))
		if m.Name != "" {
			snap.ModelLabels[m.ID] = m.Name
		}
	}
	for _, m := range models.TextModels {
		snap.TextModels = append(snap.TextModels, modelInfoToMap(m))
		if m.Name != "" {
			snap.ModelLabels[m.ID] = m.Name
		}
	}
	for _, m := range models.AudioModels {
		snap.AudioModels = append(snap.AudioModels, modelInfoToMap(m))
		if m.Name != "" {
			snap.ModelLabels[m.ID] = m.Name
		}
	}
	return snap
}

// FetchAndStoreVendorModels 调 adapter.ListModels 拉模型快照，写入 account.AvailableModelsJSON 并落库。
// 绑定成功后、以及前端手动"刷新模型"时调用。
func FetchAndStoreVendorModels(userID string, account *model.UserVendorAccount) error {
	if account == nil || account.ID == "" {
		return nil
	}
	dbVendor, err := repository.GetVendorByType(account.VendorType)
	if err != nil {
		return err
	}
	// 关键：DB 没 vendor 记录时（典型情况：内置 libtv/newwow 没在 admin 后台显式 insert 行），
	// 用 service 层的 normalizeVendorAuthMode helper 构造 in-memory default vendor（AuthMode/AuthHeaderName 走代码层 defaultVendorAuthMode）。
	// 这样 NewVendorAdapter / ListModels 路径可以走通，而不是直接放弃（之前直接 return nil 导致 availableModelsJson 一直空）。
	vendor := normalizeVendorAuthMode(account.VendorType, dbVendor)
	adapter, ok := NewVendorAdapter(vendor)
	if !ok {
		return nil // 适配器未注册（如 P0 阶段），不阻塞
	}
	models, err := adapter.ListModels(context.Background(), account)
	if err != nil {
		return err
	}
	// 跨供应商统一过滤：去掉音频能力 + 非生成的编辑/后处理工具，下拉只留可生成的模型
	models = filterVendorModels(models)
	snapshot := vendorModelsToSnapshot(models)
	if b, e := json.Marshal(snapshot); e == nil {
		account.AvailableModelsJSON = string(b)
	}
	_, err = repository.SaveUserVendorAccount(*account)
	return err
}

// RefreshVendorModels 手动刷新某家供应商的模型快照（前端"刷新模型"按钮调用）。
// 返回更新后的脱敏账户（含最新 availableModelsJson）。
func RefreshVendorModels(userID string, vendorType string) (*model.PublicBoundAccount, error) {
	if userID == "" {
		return nil, errors.New("请先登录")
	}
	vendorType = strings.ToLower(strings.TrimSpace(vendorType))
	account, err := repository.GetUserVendorAccountByType(userID, vendorType)
	if err != nil {
		return nil, fmt.Errorf("查询绑定账户失败: %w", err)
	}
	if account == nil {
		return nil, errors.New("尚未绑定该供应商账户")
	}
	if err := FetchAndStoreVendorModels(userID, account); err != nil {
		return nil, fmt.Errorf("刷新模型快照失败: %w", err)
	}
	list, err := PublicBoundAccounts(userID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].VendorType == vendorType {
			return &list[i], nil
		}
	}
	return nil, errors.New("刷新成功但未查到账户")
}

// FetchAndStoreVendorBalance 调该供应商的真实接口拉余额/套餐，写入 account.BalanceInfoJSON。
//
//	失败的常见原因：Token 过期、接口域名错位、vendor 未实现 fetchBalance。
//	设计上不阻塞绑定（best-effort），绑定后/前端"刷新余额"按钮都会调。
//
//  兼容字段：
//   - BalanceInfoJSON 存 JSON：{"balance_cents":11700,"package":"TRAIN 包","remainDays":31}
//   - renderBalanceText() 自动从该 JSON 渲染 "余额 11700 · TRAIN 包 · 剩 31 天"
//
//	BalanceInfoJSON 与 vendor store 兼容；详见 service/vendor.go renderBalanceText。
func FetchAndStoreVendorBalance(ctx context.Context, account *model.UserVendorAccount) error {
	if account == nil || account.ID == "" {
		return nil
	}
	dbVendor, err := repository.GetVendorByType(account.VendorType)
	if err != nil {
		return err
	}
	vendor := normalizeVendorAuthMode(account.VendorType, dbVendor)
	if vendor == nil {
		return nil
	}
	switch account.VendorType {
	case model.VendorTypeLibTV:
		return fetchLibTVBalanceInto(ctx, vendor, account)
	case model.VendorTypeUpDream:
		return fetchUpDreamBalanceInto(ctx, vendor, account)
	case model.VendorTypeNewWow:
		return fetchNewWowBalanceInto(ctx, vendor, account)
	}
	return nil
}

// RefreshVendorBalance 手动刷新某家供应商的余额（前端"刷新余额"按钮调用）。
func RefreshVendorBalance(ctx context.Context, userID string, vendorType string) (*model.PublicBoundAccount, error) {
	if userID == "" {
		return nil, errors.New("请先登录")
	}
	vendorType = strings.ToLower(strings.TrimSpace(vendorType))
	account, err := repository.GetUserVendorAccountByType(userID, vendorType)
	if err != nil {
		return nil, fmt.Errorf("查询绑定账户失败: %w", err)
	}
	if account == nil {
		return nil, errors.New("尚未绑定该供应商账户")
	}
	if err := FetchAndStoreVendorBalance(ctx, account); err != nil {
		return nil, fmt.Errorf("刷新余额失败: %w", err)
	}
	list, err := PublicBoundAccounts(userID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].VendorType == vendorType {
			return &list[i], nil
		}
	}
	return nil, errors.New("刷新成功但未查到账户")
}
