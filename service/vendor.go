package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// ============== 供应商元信息（vendors 表）对外服务 ==============

// PublicVendors 返回前端可用的"脱敏 + 启用"供应商列表（P0 保证即使 DB 为空也返回 4 家内置默认值，便于 UI 展示）
func PublicVendors() ([]model.PublicVendorInfo, error) {
	dbVendors, err := repository.ListEnabledVendors()
	if err != nil {
		return nil, err
	}
	result := make([]model.PublicVendorInfo, 0, len(dbVendors))
	seen := make(map[string]bool)
	for _, v := range dbVendors {
		t := strings.ToLower(strings.TrimSpace(v.Type))
		info := model.PublicVendorInfo{
			Type:           t,
			Name:           strings.TrimSpace(v.Name),
			LogoURL:        strings.TrimSpace(v.LogoURL),
			Enabled:        v.Enabled,
			Sort:           v.Sort,
			HasOAuth:       strings.TrimSpace(v.OAuthAuthURL) != "" || t != model.VendorTypeOfficial,
			APIRootHint:    strings.TrimSpace(v.APIRootURL),
			AuthMode:       strings.ToLower(strings.TrimSpace(v.AuthMode)),
			AuthHeaderName: strings.TrimSpace(v.AuthHeaderName),
		}
		if info.Type == "" || !model.ValidVendorType(info.Type) {
			continue
		}
		if info.Name == "" {
			info.Name = defaultVendorName(info.Type)
		}
		if info.LogoURL == "" {
			info.LogoURL = defaultVendorLogo(info.Type)
		}
		if info.Type == model.VendorTypeOfficial {
			info.HasOAuth = false // 官方不需要 OAuth
		}
		// AuthMode 兜底（通过 helper，两处入口共用）：
		//   - DB gorm default='cookie' 会让 libtv/newwow 历史数据全是 'cookie'，但这两个的正确模式是 custom_header
		//   - helper 内部已经处理"cookie → custom_header 纠正"和"DB 没记录 → 构造 in-memory default"，这里直接复用
		nv := normalizeVendorAuthMode(v.Type, &v)
		info.AuthMode = strings.ToLower(strings.TrimSpace(nv.AuthMode))
		info.AuthHeaderName = strings.TrimSpace(nv.AuthHeaderName)
		result = append(result, info)
		seen[info.Type] = true
	}
	// 关键：即使 DB 里一条没配（全新部署 P0 阶段），也保证内置 4 家都能显示出来，前端不空白
	for _, t := range model.AllVendorTypes {
		if seen[t] {
			continue
		}
		result = append(result, model.PublicVendorInfo{
			Type:           t,
			Name:           defaultVendorName(t),
			LogoURL:        defaultVendorLogo(t),
			Enabled:        true, // P0 阶段默认全启用，管理员后续可通过后台改 enabled=false 禁用
			Sort:           defaultVendorSort(t),
			HasOAuth:       t != model.VendorTypeOfficial,
			AuthMode:       defaultVendorAuthMode(t),
			AuthHeaderName: defaultVendorAuthHeaderName(t),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Sort != result[j].Sort {
			return result[i].Sort < result[j].Sort
		}
		return result[i].Type < result[j].Type
	})
	return result, nil
}

// ⚠️ Vendor 入口约定（2026-08-17 强化）：
//   - DB vendors.auth_mode 列在老部署里保留 gorm default:'cookie'，model/vendor.go 的新 GORM tag 已去掉 default，
//     但已落 schema 不回写；新部署 DB 没有 default，auth_mode 字段为空字符串。
//   - 任何新增的、读 model.Vendor 的代码路径都必须先过 normalizeVendorAuthMode（DB 读后内存纠正）
//     或 NormalizeVendorAuthModeForDispatch（dispatch 路径，DB nil 行强制 Enabled+APIRootURL 兜底）。
//   - 跳过 helper 会让 NewWow/LibTV 被误判为 cookie 鉴权 → accesstoken/Token header 不注入 → 请求被供应商拒绝。
//   - 涉及 dispatch 链路的"取激活账户 + 解析 vendor + 拿 adapter"用 ResolveActiveVendorAdapter（统一 chokepoint）。

// vendor has OAuth 标志 + AuthMode 字段之间的 helper：在读 model.Vendor（来自 DB）后立刻纠正 AuthMode
//   DB gorm default='cookie' 让内置 vendor 历史 AuthMode 全是 'cookie'，但 libtv/newwow 应该是 custom_header。
//   两处入口都调：ListPublicVendors（前端 list 接口）+ BindVendorByCookie（绑定路由校验 vendor.AuthMode 用）。
//   这一层 normalize 不持久化回 DB，仅在内存里保证读出来就是对的——避免 BindVendorByCookie 走 cookie 拒绝分支。
//   当入参 v 为 nil 时（DB 没记录），构造一个 in-memory default vendor 返回——这样 BindVendorByCookie 拿到的不再是 nil，可以继续走 custom_header 校验。
//   vendorType 用于在 v 为 nil 时知道是哪家供应商，从而用 defaultVendorAuthMode 兜底。
func normalizeVendorAuthMode(vendorType string, v *model.Vendor) *model.Vendor {
	expected := defaultVendorAuthMode(vendorType)
	expectedHeader := defaultVendorAuthHeaderName(vendorType)
	if v == nil {
		// DB 没记录，构造一个 in-memory default（不持久化）
		return &model.Vendor{
			Type:           vendorType,
			AuthMode:       expected,
			AuthHeaderName: expectedHeader,
		}
	}
	if v.AuthMode == "" || (v.AuthMode == "cookie" && expected != "cookie") {
		v.AuthMode = expected
		v.AuthHeaderName = expectedHeader
	}
	return v
}

// NormalizeVendorAuthModeForDispatch dispatch 路径专用：DB 没记录时构造的 in-memory default 强制 Enabled=true。
// 走到 dispatch 的用户已经绑定了该供应商，不应再因 vendors 表缺行被 !vendor.Enabled 早返回挡掉。
// PublicVendors 列表等"读 enabled 字段决定要不要展示"的场景继续用私有 normalizeVendorAuthMode，按 DB 字段走。
func NormalizeVendorAuthModeForDispatch(vendorType string, v *model.Vendor) *model.Vendor {
	out := normalizeVendorAuthMode(vendorType, v)
	if out == nil {
		return nil
	}
	if v == nil {
		out.Enabled = true
		if strings.TrimSpace(out.APIRootURL) == "" {
			out.APIRootURL = defaultVendorAPIRoot(vendorType)
		}
	}
	return out
}

// ResolveActiveVendorAdapter 把"用户的激活非官方供应商 → DB vendor 行 → 适配器"这一条流水线封装。
// 原本 dispatchVendorProxy（handler/vendor_proxy.go）和 dispatchVendorVideoProxy（handler/video_task.go）
// 各自拼接，产生重复。任何"用户已激活非官方供应商 → 走适配器路径"的代码都应走这里。
//
// 返回值语义：
//   - err != nil          : 任一前置步骤出错（DB 查询失败等）；调用方通常 log 后回落官方链路。
//   - adapter == nil      : 用户没激活非官方账户 / 是 official / 已停用 / 适配器未实现；
//                           这种情况 err 通常为 nil，调用方应无错地回落官方链路。
//   - adapter != nil      : 命中供应商，可走适配器路径。
//
// 同时返回 account / vendor 便于调用方记日志（如 SaveUserVendorAccountBestEffort 刷新 LastUsedAt）。
func ResolveActiveVendorAdapter(userID string) (adapter VendorAdapter, account *model.UserVendorAccount, vendor *model.Vendor, err error) {
	account, err = repository.GetActiveVendorAccount(userID)
	if err != nil {
		return nil, nil, nil, err
	}
	if account == nil || account.VendorType == model.VendorTypeOfficial {
		return nil, nil, nil, nil
	}
	dbVendor, err := GetVendorByType(account.VendorType)
	if err != nil {
		return nil, account, nil, err
	}
	v := NormalizeVendorAuthModeForDispatch(account.VendorType, dbVendor)
	if v == nil {
		return nil, account, nil, nil
	}
	if !v.Enabled {
		// 管理员已在后台停用此供应商——和 dispatchVendorProxy 原语义保持一致：回落官方链路。
		return nil, account, v, nil
	}
	a, ok := NewVendorAdapter(v)
	if !ok {
		return nil, account, v, nil
	}
	return a, account, v, nil
}

// defaultVendorAPIRoot dispatch 兜底场景下构造的 in-memory vendor 用，避免 vendor_libtv.go / vendor_newwow.go
// 里用 hardcode 常量再次声明。DB 有记录时仍以 v.APIRootURL 为准。
func defaultVendorAPIRoot(t string) string {
	switch t {
	case model.VendorTypeNewWow:
		return "https://neowow.cn"
	default:
		return ""
	}
}

// defaultVendorName P0 兜底：DB 空时显示的默认名称
func defaultVendorName(t string) string {
	switch t {
	case model.VendorTypeOfficial:
		return "官方云端（管理员配置）"
	case model.VendorTypeUpDream:
		return "UpDream 云端"
	case model.VendorTypeLibTV:
		return "LibTV 云端"
	case model.VendorTypeNewWow:
		return "NewWow 云端"
	default:
		return t
	}
}

// defaultVendorLogo P0 兜底：暂时用 emoji + SVG dataURI（不需要额外图片文件），后续可替换成真实 logo URL
func defaultVendorLogo(t string) string {
	emoji := "☁️"
	switch t {
	case model.VendorTypeOfficial:
		emoji = "🏛️"
	case model.VendorTypeUpDream:
		emoji = "🚀"
	case model.VendorTypeLibTV:
		emoji = "📺"
	case model.VendorTypeNewWow:
		emoji = "✨"
	}
	// 简单 inline SVG：白底 + emoji 字符；避免外部依赖，P0 够用
	return "data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='64' height='64' viewBox='0 0 64 64'><rect width='64' height='64' rx='12' fill='%23f3f4f6'/><text x='50%25' y='55%25' text-anchor='middle' font-size='32'>" + emoji + "</text></svg>"
}

// defaultVendorSort 内置供应商默认排序：官方最前
func defaultVendorSort(t string) int {
	switch t {
	case model.VendorTypeOfficial:
		return 0
	case model.VendorTypeUpDream:
		return 10
	case model.VendorTypeLibTV:
		return 20
	case model.VendorTypeNewWow:
		return 30
	default:
		return 99
	}
}

// defaultVendorAuthMode 内置供应商默认鉴权模式（DB AuthMode 字段为空时兜底）
func defaultVendorAuthMode(t string) string {
	switch t {
	case model.VendorTypeLibTV:
		// LibTV 走 liblib.tv 创作站（api.liblib.tv），鉴权用 HTTP header Token（custom_header）。
		// 仅此一种适配器实现（libtvTaskAdapter），不再支持 AK/SK 开放平台路径；AuthMode 固定为 custom_header。
		return model.VendorAuthModeCustomHeader
	case model.VendorTypeNewWow:
		// NewWow 鉴权是自定义 HTTP header accesstoken（不是 cookie）
		return model.VendorAuthModeCustomHeader
	case model.VendorTypeUpDream:
		// UpDream 只需 Bearer JWT（Authorization header），不需要 Cookie
		return model.VendorAuthModeCustomHeader
	default:
		// official 默认 cookie 模式
		return model.VendorAuthModeCookie
	}
}

// defaultVendorAuthHeaderName custom_header 模式下默认的 header 名。
//   - NewWow 走 "accesstoken"（小写）
//   - LibTV 走 "Token"（首字母大写，跟浏览器 DevTools Request Headers 一致，方便用户复制）
func defaultVendorAuthHeaderName(t string) string {
	switch t {
	case model.VendorTypeNewWow:
		return "accesstoken"
	case model.VendorTypeLibTV:
		return "Token"
	case model.VendorTypeUpDream:
		return "Authorization"
	default:
		return ""
	}
}

// ============== 用户绑定账户对外服务 ==============

// PublicBoundAccounts 返回用户已绑定账户（全部脱敏 + 余额文本预渲染）
// P0 阶段 DB 没数据时直接返回空切片，不报错
func PublicBoundAccounts(userID string) ([]model.PublicBoundAccount, error) {
	if userID == "" {
		// 游客模式就一个空数组
		return []model.PublicBoundAccount{}, nil
	}
	accounts, err := repository.ListUserVendorAccounts(userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.PublicBoundAccount, 0, len(accounts))
	for _, a := range accounts {
		info := model.PublicBoundAccount{
			VendorType:          strings.TrimSpace(a.VendorType),
			IsActive:            a.IsActive,
			DisplayName:         strings.TrimSpace(a.DisplayName),
			AvatarURL:           strings.TrimSpace(a.AvatarURL),
			HasModels:           strings.TrimSpace(a.AvailableModelsJSON) != "",
			AvailableModelsJSON: strings.TrimSpace(a.AvailableModelsJSON),
			BoundAt:             a.BoundAt,
			LastUsedAt:          a.LastUsedAt,
			BalanceText:         renderBalanceText(a.BalanceInfoJSON),
			PowerHistory:        extractVendorPowerHistory(a.RawExtraJSON),
		}
		if info.DisplayName == "" {
			// 供应商没给昵称就给个默认
			info.DisplayName = defaultVendorName(info.VendorType) + "账户"
		}
		out = append(out, info)
	}
	return out, nil
}

// extractVendorPowerHistory 从 RawExtraJSON 抽 libtvPowerByModel 字段转成 VendorPowerRecord map。
//	RawExtraJSON 格式：{"libtvPowerByModel": {"<modelKey>": {"power": N, "updatedAt": "..."}}, "access_key": "...", ...}
func extractVendorPowerHistory(rawExtra string) map[string]model.VendorPowerRecord {
	out := map[string]model.VendorPowerRecord{}
	if strings.TrimSpace(rawExtra) == "" {
		return nil
	}
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(rawExtra), &wrapper); err != nil {
		return nil
	}
	history, ok := wrapper["libtvPowerByModel"].(map[string]any)
	if !ok {
		return nil
	}
	for k, v := range history {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		power, _ := entry["power"].(float64)
		updatedAt, _ := entry["updatedAt"].(string)
		out[k] = model.VendorPowerRecord{
			Power:     int(power),
			UpdatedAt: updatedAt,
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// renderBalanceText 把 BalanceInfoJSON 渲染成一行给人看的文案；解析失败或空直接返回""
func renderBalanceText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// 结构兼容两种：对象 {"balance_cents":x, "package":"xxx"} 或直接字符串
	var asString string
	if err := json.Unmarshal([]byte(raw), &asString); err == nil && strings.TrimSpace(asString) != "" {
		return asString
	}
	var obj struct {
		CostCents any `json:"costCents"`
		BalanceText string `json:"balanceText"`
		Package     string `json:"package"`
		Expire      string `json:"expire"`
		RemainDays  int    `json:"remainDays"`
	}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return ""
	}
	if strings.TrimSpace(obj.BalanceText) != "" {
		return obj.BalanceText
	}
	parts := make([]string, 0, 3)
	if obj.CostCents != nil {
		parts = append(parts, "余额 "+toString(obj.CostCents))
	}
	if strings.TrimSpace(obj.Package) != "" {
		parts = append(parts, strings.TrimSpace(obj.Package))
	}
	if obj.RemainDays > 0 {
		parts = append(parts, "剩 "+strconv.Itoa(obj.RemainDays)+" 天")
	} else if strings.TrimSpace(obj.Expire) != "" {
		parts = append(parts, "有效期至 "+strings.TrimSpace(obj.Expire))
	}
	return strings.Join(parts, " · ")
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.Itoa(int(x))
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.Itoa(int(x))
	default:
		return ""
	}
}

// ActivateVendor 切换激活供应商：
// - vendorType = "official" → 把所有第三方账户置 inactive（等价于切回官方）
// - 其他 → 先找到该用户绑定的对应账户，不存在返回明确错误，存在则置 active 其他 inactive
func ActivateVendor(userID string, vendorType string) error {
	if userID == "" {
		return errors.New("请先登录")
	}
	if !model.ValidVendorType(vendorType) {
		return errors.New("未知供应商类型")
	}
	if vendorType == model.VendorTypeOfficial {
		return repository.ActivateUserVendorAccount(userID, "")
	}
	// 找绑定的账户
	account, err := repository.GetUserVendorAccountByType(userID, vendorType)
	if err != nil {
		return err
	}
	if account == nil {
		return errors.New("尚未绑定该供应商账户，请先授权登录")
	}
	return repository.ActivateUserVendorAccount(userID, account.ID)
}

// UnbindVendor 解绑当前用户在某家供应商的绑定账户。
// - 官方模式不允许解绑（无账户）
// - 解绑后若被删的账户原本是激活态，自动回落到官方模式（避免出现"没有激活供应商"的悬空状态）
func UnbindVendor(userID string, vendorType string) error {
	if userID == "" {
		return errors.New("请先登录")
	}
	vendorType = strings.ToLower(strings.TrimSpace(vendorType))
	if !model.ValidVendorType(vendorType) {
		return errors.New("未知供应商类型")
	}
	if vendorType == model.VendorTypeOfficial {
		return errors.New("官方模式无需解绑")
	}
	account, err := repository.GetUserVendorAccountByType(userID, vendorType)
	if err != nil {
		return fmt.Errorf("查询绑定账户失败: %w", err)
	}
	if account == nil {
		return errors.New("尚未绑定该供应商账户")
	}
	wasActive := account.IsActive
	if err := repository.DeleteUserVendorAccountByID(userID, account.ID); err != nil {
		return fmt.Errorf("解绑失败: %w", err)
	}
	// 被删的账户原本激活 → 回落官方（ActivateUserVendorAccount(userID,"") 等价于全部置 inactive）
	if wasActive {
		_ = repository.ActivateUserVendorAccount(userID, "")
	}
	return nil
}

// ============== P1 新增：按 Cookie 绑定供应商账户 ==============
// BindVendorByCookieParams 浏览器插件或前端手动粘贴上传的凭证（AccessToken / Cookie / AccessKey）请求体
type BindVendorByCookieParams struct {
	VendorType     string `json:"vendorType"`
	CookieString   string `json:"cookieString"`
	DisplayName    string `json:"displayName,omitempty"` // 插件已预拉到昵称时直接带过来，后端再校验覆盖
	AvatarURL      string `json:"avatarUrl,omitempty"`
	VendorUserID   string `json:"vendorUserId,omitempty"`
	ExpiresAt      string `json:"expiresAt,omitempty"` // RFC3339 字符串，空=未知
	// AccessKey 模式（LibTV 官方给的 AccessKey/Secret 优先；如果传了就当高优先级凭证，CookieString 可以为空或同时带做兜底）
	AccessKey      string `json:"accessKey,omitempty"`
	AccessSecret   string `json:"accessSecret,omitempty"`
	AppKey         string `json:"appKey,omitempty"`
	// AuthHeaderName / AuthHeaderValue：NewWow 这类"鉴权在自定义 HTTP header"的供应商专用；
	// 存到 account.AccessToken + 用 AuthHeaderName 当 req.Header.Set 的键。
	// 与 CookieString 二选一优先：传 AuthHeaderValue 即视为 custom_header 路径。
	AuthHeaderName  string `json:"authHeaderName,omitempty"`
	AuthHeaderValue string `json:"authHeaderValue,omitempty"`
	// AuthExtraHeaderName / AuthExtraHeaderValue：cookie 模式附加的"叠加型"鉴权 header（旧版向后兼容）。
	// 早期 UpDream 用 Cookie + Authorization Bearer JWT 双链路；现在 UpDream 已改为 custom_header 模式，
	// 仅传 AuthHeaderValue 即可。此字段保留是为了不破坏旧绑定数据，新绑定不再使用。
	// 存到 account.RawExtraJSON["auth_extra_header"]（{name, value} JSON 字符串），
	// applyVendorAuth 在 cookie 模式读出来直接 req.Header.Set。
	AuthExtraHeaderName  string `json:"authExtraHeaderName,omitempty"`
	AuthExtraHeaderValue string `json:"authExtraHeaderValue,omitempty"`
}

// applyVendorAuth 把 vendor 的鉴权凭证注入到 HTTP 请求头上。
// 不同 AuthMode：
//   - "" / "cookie"        → 注入 Cookie 头（默认，account.AccessToken 当 cookie 字符串）
//   - "custom_header"      → 注入 vendor.AuthHeaderName 命名的 header（UpDream Authorization Bearer / NewWow accesstoken 场景）
//   - "openapi_signature"  → 由 adapter 自己签名调用（预留给需要 AK/SK 签名的供应商），本函数不动 req
// 叠加规则（cookie 模式专用）：若 account.RawExtraJSON["auth_extra_header"] 有 {name, value}，
// 会在注入 Cookie 基础上额外 set 一个 header（向后兼容旧的双链路绑定数据）。
// 返回是否做了注入（true = 设置了 header），false = 此模式不需要 header 注入。
func applyVendorAuth(req *http.Request, vendor *model.Vendor, account *model.UserVendorAccount) bool {
	if req == nil || vendor == nil || account == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(vendor.AuthMode))
	injected := false
	switch mode {
	case "", "cookie":
		if c := strings.TrimSpace(account.AccessToken); c != "" {
			// 解密凭证（AES-256-GCM）；旧数据未加密时 DecryptCredential 会失败，回退原值兼容。
			if decrypted, err := DecryptCredential(c); err == nil && decrypted != "" {
				c = decrypted
			}
			req.Header.Set("Cookie", c)
			injected = true
		}
		// 叠加：旧的双链路绑定数据可能带 auth_extra_header，继续注入做向后兼容
		if extra, ok := readAccountExtraHeader(account); ok {
			// 解密 auth_extra_header value
			if decrypted, err := DecryptCredential(extra.Value); err == nil && decrypted != "" {
				extra.Value = decrypted
			}
			req.Header.Set(extra.Name, extra.Value)
			injected = true
		}
	case "custom_header":
		name := strings.TrimSpace(vendor.AuthHeaderName)
		value := strings.TrimSpace(account.AccessToken)
		if value != "" {
			if decrypted, err := DecryptCredential(value); err == nil && decrypted != "" {
				value = decrypted
			}
		}
		if name != "" && value != "" {
			req.Header.Set(name, value)
			injected = true
		}
	}
	return injected
}

// readAccountExtraHeader 从 account.RawExtraJSON 读 auth_extra_header 字段。
// 存储格式：extras map → "auth_extra_header" → JSON.stringify({name,value})。
// 返回 (header, true) 找到且有效；找不到/解析失败返回 zero value + false（不报错——叠加 header 是可选能力）。
func readAccountExtraHeader(account *model.UserVendorAccount) (out struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}, ok bool) {
	if account == nil {
		return out, false
	}
	raw := strings.TrimSpace(account.RawExtraJSON)
	if raw == "" {
		return out, false
	}
	var extras map[string]string
	if err := json.Unmarshal([]byte(raw), &extras); err != nil {
		return out, false
	}
	js := strings.TrimSpace(extras["auth_extra_header"])
	if js == "" {
		return out, false
	}
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return out, false
	}
	if strings.TrimSpace(out.Name) == "" || strings.TrimSpace(out.Value) == "" {
		return out, false
	}
	return out, true
}

// vendorCookieVerifyEndpoint 每家供应商的 userInfo 校验 URL 与"命中必要 Cookie key"列表
// 注意：UpDream 只需 Bearer JWT（custom_header 模式），不走 Cookie 校验；
// NewWow 无开放平台、也没有可用的 userinfo 校验端点（占位地址 404 或返回 SPA 首页），
// 因此 LenientOnly=true：只本地校验必要 Cookie key，真正有效性由生图重放兜底。
var vendorCookieVerifySpec = map[string]struct {
	VerifyURL     string
	Method        string // 校验请求方法，默认 GET；LibTV 新地址要求 POST
	NecessaryKeys []string
	APIHostMatch  []string // 该供应商只允许请求这些 Host（白名单，做 SSRF 第二道防线）
	LenientOnly   bool     // 无干净 userinfo 端点 → 仅本地校验必要 Cookie key，不调远程
}{
	model.VendorTypeUpDream: {
		// UpDream 只需 Bearer JWT（custom_header 模式），不走 Cookie 校验
		VerifyURL:     "https://www.updream.cn/api/user/info",
		NecessaryKeys: []string{},
		APIHostMatch: []string{"updream.cn"},
		LenientOnly:  true,
	},
	model.VendorTypeLibTV: {
		VerifyURL:     "https://api2.liblib.art/api/www/activity/userInfo",
		Method:        http.MethodPost,
		NecessaryKeys: []string{"SESSION", "token", "jwt", "libtv_session", "passport", "access_key", "LIBTV_ACCESS_KEY"},
		APIHostMatch:  []string{"liblib.tv", "liblib.art", "liblibai.cloud"},
	},
	model.VendorTypeNewWow: {
		VerifyURL:     "https://neowow.cn/api/user/info",
		NecessaryKeys: []string{
			// NewWow 实际鉴权走 HTTP header accesstoken（不是 cookie），
			// 但若用户贴 cookie 也要能识别出"设备指纹 cookie + accesstoken 串"
			"_c_WBKFRo", "u", "session",
			// 通用兜底
			"token", "session", "jwt", "SESSION", "passport", "neo_token", "uid",
		},
		APIHostMatch: []string{"neowow.cn"},
		LenientOnly:  true,
	},
}

// VendorCookieVerifyResult 校验成功时提取的用户信息（统一结构，供绑定和后续 UI 展示）
type VendorCookieVerifyResult struct {
	Valid        bool
	DisplayName  string
	AvatarURL    string
	VendorUserID string
	ExpireGuess  string // 从 Cookie 头解析或返回体里拿到的过期时间（RFC3339）
	RawBody      string // 上游响应原文，存到 RawExtraJSON 方便后续排查
}

// VerifyVendorCookieWithSpec 拿 cookie 真的调供应商 userinfo 接口，成功返回用户信息
// 内部使用 SafeProxyHTTPClient（SSRF 防护 + 重定向限制 + host 白名单）。
func VerifyVendorCookieWithSpec(vendorType string, cookieString string) (*VendorCookieVerifyResult, error) {
	spec, ok := vendorCookieVerifySpec[vendorType]
	if !ok {
		return nil, errors.New("未知供应商类型")
	}
	cookieString = strings.TrimSpace(cookieString)
	if cookieString == "" {
		return nil, errors.New("Cookie 字符串为空")
	}
	// 防御性拦截：用户把 JWT 误贴到 Cookie 框（以 "eyJ" 开头 + 两个 "." 分隔 + 不含 "key=value" 形式）。
	// 误贴会导致后续校验全失败 + 让用户怀疑代码有问题；早期返回明确错误更友好。
	if looksLikeJWT(cookieString) {
		return nil, errors.New("Cookie 框内容看起来是 JWT（以 eyJ 开头、有两段 \".\" 分隔），请复制 DevTools Request Headers 里的 Cookie 字段（格式：buvid3=...; b_nut=...; DedeUserID=...），JWT 应该贴到 Authorization 头框")
	}
	// 先粗判必要 key：如果一条都没命中，直接拒绝（避免打无意义请求）
	cookieHasAnyNecessary := false
	for _, key := range spec.NecessaryKeys {
		lk := strings.ToLower(key)
		// Cookie 格式：key1=val1; key2=val2; ... —— 匹配 "key=" 前缀，大小写不敏感
		for _, seg := range strings.Split(cookieString, ";") {
			kv := strings.SplitN(strings.TrimSpace(seg), "=", 2)
			if len(kv) != 2 {
				continue
			}
			if strings.ToLower(strings.TrimSpace(kv[0])) == lk {
				cookieHasAnyNecessary = true
				break
			}
		}
		if cookieHasAnyNecessary {
			break
		}
	}
	if !cookieHasAnyNecessary {
		return nil, fmt.Errorf("未命中必要登录态 Cookie（需包含 %s 等任一）", strings.Join(spec.NecessaryKeys, "/"))
	}

	// 无开放平台/无干净 userinfo 端点的供应商（UpDream/NewWow）只做本地 lenient 校验：
	// 命中必要 Cookie key 即视为有效，真正有效性由后续生图重放兜底。
	if spec.LenientOnly {
		return &VendorCookieVerifyResult{
			Valid:        true,
			DisplayName:  guessVendorNicknameFromCookie(cookieString),
			VendorUserID: guessVendorUserIDFromCookie(cookieString),
		}, nil
	}

	// SSRF 防护：目标 URL 必须属于该供应商的 host 白名单
	verifyURL, err := url.Parse(spec.VerifyURL)
	if err != nil {
		return nil, errors.New("供应商校验 URL 配置错误")
	}
	host := strings.ToLower(verifyURL.Hostname())
	hostAllowed := false
	for _, h := range spec.APIHostMatch {
		lh := strings.ToLower(h)
		if host == lh || strings.HasSuffix(host, "."+lh) {
			hostAllowed = true
			break
		}
	}
	if !hostAllowed {
		return nil, errors.New("供应商校验 URL 不在允许的域名白名单内")
	}

	ctx, cancel := timeoutContext(8 * time.Second)
	defer cancel()
	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, spec.VerifyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Cookie", cookieString)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 FreedomVendorBind/1.0")
	resp, err := SafeProxyHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求供应商校验接口失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 最多读 1MB
	if err != nil {
		return nil, fmt.Errorf("读取供应商响应失败: %w", err)
	}
	raw := string(body)

	// 3xx / 4xx = Cookie 失效或跳登录页
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return &VendorCookieVerifyResult{Valid: false, RawBody: raw}, fmt.Errorf("Cookie 校验失败（HTTP %d），请重新登录后复制", resp.StatusCode)
	}

	// 统一解析 userinfo 结构（兼容常见几种返回格式）
	info := parseVendorUserInfoRaw(raw)
	if info.VendorUserID == "" && info.DisplayName == "" {
		// 有些供应商即使 200 也会返回 { code: 401, msg: "未登录" } 这种结构，再检查下错误码
		if looksLikeUnauthorized(raw) {
			return &VendorCookieVerifyResult{Valid: false, RawBody: raw}, errors.New("响应显示未登录，请重新复制最新 Cookie")
		}
	}

	info.RawBody = raw
	info.Valid = info.VendorUserID != "" || info.DisplayName != "" // 有任一就算有效
	return &info, nil
}

// parseVendorUserInfoRaw 松散解析供应商 userinfo JSON；字段映射参考各家 API 文档占位
func parseVendorUserInfoRaw(raw string) VendorCookieVerifyResult {
	var res VendorCookieVerifyResult
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return res
	}
	// 先用字符串看 JSON 解析
	var anyObj any
	if err := json.Unmarshal([]byte(raw), &anyObj); err != nil {
		return res
	}
	// 层叠解包常见的 {data:..} / {result:..} / {user:..}
	m, ok := anyObj.(map[string]any)
	if !ok {
		return res
	}
	for _, key := range []string{"data", "user", "result", "payload", "info", "profile"} {
		if sub, ok := m[key].(map[string]any); ok {
			m = sub
			break
		}
	}
	readStr := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
			// 兼容驼峰变体
			lower := strings.ToLower(k)
			for mk, mv := range m {
				if strings.ToLower(mk) == lower {
					if s, ok := mv.(string); ok && strings.TrimSpace(s) != "" {
						return strings.TrimSpace(s)
					}
				}
			}
		}
		return ""
	}
	res.DisplayName = readStr("nickname", "nickName", "username", "name", "displayName", "userName", "screen_name", "screenName", "nick_name", "uname")
	res.AvatarURL = readStr("avatar", "avatarUrl", "avatar_url", "headUrl", "head_url", "photo", "headimgurl", "img", "icon", "profileImage", "face", "officialAvatar")
	res.VendorUserID = readStr("id", "userId", "user_id", "uid", "userid", "openid", "open_id", "unionId", "account_id", "memberId", "mid", "DedeUserID")
	return res
}

// guessVendorNicknameFromCookie 从 Cookie 字符串里宽松猜测昵称（lenient 校验用，拿不到返回空）。
func guessVendorNicknameFromCookie(cookieString string) string {
	return guessCookieValue(cookieString, []string{"nickname", "nick_name", "username", "user_name", "display_name", "displayname", "name"})
}

// guessVendorUserIDFromCookie 从 Cookie 字符串里宽松猜测用户 ID（lenient 校验用）。
func guessVendorUserIDFromCookie(cookieString string) string {
	return guessCookieValue(cookieString, []string{"userid", "user_id", "uid", "openid", "member_id", "account_id"})
}

// guessCookieValue 从 Cookie 字符串里按候选 key 查找值（key 大小写不敏感）。
func guessCookieValue(cookieString string, keys []string) string {
	for _, seg := range strings.Split(cookieString, ";") {
		kv := strings.SplitN(strings.TrimSpace(seg), "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(kv[0]))
		for _, k := range keys {
			if name == k {
				if v := strings.TrimSpace(kv[1]); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// looksLikeUnauthorized 判定响应 body 是否表现为"未登录 / 凭证失效"。
// 优先级：JSON 业务码 → 强信号文案，避免被 "token" / "session" 等通用字段误触发。
func looksLikeUnauthorized(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	// 1. JSON 业务错误码（多数国内供应商习惯 code=401/403/40001 等）
	var payload struct {
		Code  any `json:"code"`
		Errno any `json:"errno"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		for _, c := range []any{payload.Code, payload.Errno} {
			if v, ok := c.(float64); ok {
				if int(v) == 401 || int(v) == 403 {
					return true
				}
			}
		}
	}
	// 2. 强信号文案（不被通用字段误触发）
	lower := strings.ToLower(raw)
	for _, s := range []string{
		"未登录", "会话已失效", "会话失效", "凭证已失效", "请重新登录", "please login", "please sign in", "not logged in",
	} {
		if strings.Contains(lower, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

// BindVendorByCookie 把 AccessToken / Cookie 绑定成用户 UserVendorAccount：
//   1. 校验 vendorType
//   2. 如果没有预填 DisplayName/VendorUserID → 调一次 userinfo 校验并回填
//   3. 如果该供应商该用户历史有一条旧记录 → 更新（刷新 token / cookie / 过期时间）；否则新增
//   4. 保存成功后自动把这条账户设为 active（用户点绑定=想用这家）
//   5. 返回脱敏后的绑定记录（不含 Cookie/Token 字符串）
func BindVendorByCookie(userID string, p BindVendorByCookieParams) (*model.PublicBoundAccount, error) {
	if userID == "" {
		return nil, errors.New("请先登录")
	}
	p.VendorType = strings.ToLower(strings.TrimSpace(p.VendorType))
	if !model.ValidVendorType(p.VendorType) {
		return nil, errors.New("未知供应商类型")
	}
	if p.VendorType == model.VendorTypeOfficial {
		return nil, errors.New("官方模式不需要通过 Cookie 绑定账户")
	}
	p.CookieString = strings.TrimSpace(p.CookieString)
	p.AuthHeaderName = strings.TrimSpace(p.AuthHeaderName)
	p.AuthHeaderValue = strings.TrimSpace(p.AuthHeaderValue)
	hasCookie := p.CookieString != ""
	hasAK := strings.TrimSpace(p.AccessKey) != "" || strings.TrimSpace(p.AppKey) != ""
	hasHeader := p.AuthHeaderValue != ""
	if !hasCookie && !hasAK && !hasHeader {
		return nil, errors.New("请至少提供 AccessToken（AuthHeaderValue）、Cookie 或 AccessKey 其一")
	}

	// 加载 vendor 元信息（含 AuthMode / AuthHeaderName 配置）；管理员在后台登记该供应商的鉴权模式
	dbVendor, _ := repository.GetVendorByType(p.VendorType)
	// 关键：DB gorm default='cookie' 会让 libtv/newwow 历史 AuthMode 全是 'cookie'，但实际期望 custom_header；
	// 这条 normalize 让 BindVendorByCookie 校验路径也走正确的鉴权模式，避免"该供应商不是自定义 header 鉴权模式"的误判。
	// DB 完全没有该 vendor 记录时（dbVendor==nil）→ helper 构造 in-memory default vendor，AuthMode/AuthHeaderName 都是代码层默认值。
	// 只在内存中纠正（不持久化到 DB）。
	vendor := normalizeVendorAuthMode(p.VendorType, dbVendor)

	// 凭证鉴权：custom_header 模式不需要必要 key 命中——信任 HTTP header value 本身（实际有效性由适配器首次调用兜底）
	var verify *VendorCookieVerifyResult
	var err error
	if hasHeader {
		// custom_header 模式（如 UpDream Authorization Bearer / NewWow accesstoken）：只做"value 非空 + vendor 配置了 header 名"基础校验
		if vendor == nil || strings.ToLower(strings.TrimSpace(vendor.AuthMode)) != model.VendorAuthModeCustomHeader {
			return nil, errors.New("该供应商不是自定义 header 鉴权模式；请勿直接粘贴 header value")
		}
		if p.AuthHeaderName != "" && p.AuthHeaderName != strings.TrimSpace(vendor.AuthHeaderName) {
			// 浏览器插件带的 header 名可能跟当前 vendor 配置不一致；不一致就按 vendor 配置的来，用用户传的 value
		}
		verify = &VendorCookieVerifyResult{Valid: true, DisplayName: p.DisplayName, AvatarURL: p.AvatarURL, VendorUserID: p.VendorUserID}
	} else if hasCookie {
		verify, err = VerifyVendorCookieWithSpec(p.VendorType, p.CookieString)
		if err != nil {
			return nil, err
		}
		if verify == nil || !verify.Valid {
			return nil, errors.New("Cookie 校验未通过，请重新登录平台后再复制")
		}
	} else {
		// 只用 AccessKey 模式：暂不远程校验（各平台校验方式不同，P1 再补每家专属验签调用），但 DisplayName/VendorUserID 必须至少填一个（插件预拉到就会有）
		if strings.TrimSpace(p.VendorUserID) == "" && strings.TrimSpace(p.DisplayName) == "" {
			return nil, errors.New("AccessKey 模式需同时提供账户昵称或 VendorUserID（插件会自动带上）")
		}
		verify = &VendorCookieVerifyResult{Valid: true, DisplayName: p.DisplayName, AvatarURL: p.AvatarURL, VendorUserID: p.VendorUserID}
	}

	// 合并/回填：以 Verify 拉到的优先（后端权威），没有再取插件预填的
	displayName := strings.TrimSpace(verify.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(p.DisplayName)
	}
	if displayName == "" {
		displayName = defaultVendorName(p.VendorType) + "账户"
	}
	avatar := strings.TrimSpace(verify.AvatarURL)
	if avatar == "" {
		avatar = strings.TrimSpace(p.AvatarURL)
	}
	uid := strings.TrimSpace(verify.VendorUserID)
	if uid == "" {
		uid = strings.TrimSpace(p.VendorUserID)
	}

	// 过期时间优先取 ExpiresAt 参数（插件带过来的）
	var expireAtPtr *time.Time
	if t := strings.TrimSpace(p.ExpiresAt); t != "" {
		if parsed, e := time.Parse(time.RFC3339, t); e == nil && !parsed.IsZero() {
			parsedLocal := parsed
			expireAtPtr = &parsedLocal
		}
	} else if t := strings.TrimSpace(verify.ExpireGuess); t != "" {
		if parsed, e := time.Parse(time.RFC3339, t); e == nil && !parsed.IsZero() {
			parsedLocal := parsed
			expireAtPtr = &parsedLocal
		}
	}

	// 查询是否已存在该供应商绑定记录
	existing, err := repository.GetUserVendorAccountByType(userID, p.VendorType)
	if err != nil {
		return nil, fmt.Errorf("查询历史绑定失败: %w", err)
	}
	var account model.UserVendorAccount
	if existing != nil {
		account = *existing
	} else {
		account.UserID = userID
		account.VendorType = p.VendorType
		// VendorID：先尝试从 vendors 表按 type 查；查不到（管理员没配 vendors 表但 P0 兜底能显示）时留空，后续保存不强制
		if v, _ := repository.GetVendorByType(p.VendorType); v != nil {
			account.VendorID = v.ID
		}
		account.BoundAt = time.Now()
	}

	// 更新凭证：AES-256-GCM 加密后存库，避免数据库泄露时凭证明文暴露。
	if hasHeader {
		// custom_header 模式：把 header value 加密后存到 AccessToken 字段。
		encrypted, err := EncryptCredential(p.AuthHeaderValue)
		if err != nil {
			return nil, fmt.Errorf("加密凭证失败: %w", err)
		}
		account.AccessToken = encrypted
	} else if hasCookie {
		encrypted, err := EncryptCredential(p.CookieString)
		if err != nil {
			return nil, fmt.Errorf("加密凭证失败: %w", err)
		}
		account.AccessToken = encrypted
	}
	// AccessKey/AppKey 存到 RawExtraJSON（加密敏感字段）
	extras := map[string]string{}
	if account.RawExtraJSON != "" {
		_ = json.Unmarshal([]byte(account.RawExtraJSON), &extras)
	}
	if v := strings.TrimSpace(p.AccessKey); v != "" {
		enc, _ := EncryptCredential(v)
		extras["access_key"] = enc
	}
	if v := strings.TrimSpace(p.AccessSecret); v != "" {
		enc, _ := EncryptCredential(v)
		extras["access_secret"] = enc
	}
	if v := strings.TrimSpace(p.AppKey); v != "" {
		enc, _ := EncryptCredential(v)
		extras["app_key"] = enc
	}
	// 叠加鉴权 header（向后兼容旧的双链路绑定）：
	// 存为 JSON 字符串 {"name":"Authorization","value":"Bearer eyJ..."}，applyVendorAuth 在 cookie 模式自动回放。
	// 名称默认 "Authorization"；用户主动传 AuthExtraHeaderName 时才覆盖。
	if v := strings.TrimSpace(p.AuthExtraHeaderValue); v != "" {
		extraName := strings.TrimSpace(p.AuthExtraHeaderName)
		if extraName == "" {
			extraName = "Authorization"
		}
		encValue, _ := EncryptCredential(v)
		extraJSON, _ := json.Marshal(map[string]string{"name": extraName, "value": encValue})
		extras["auth_extra_header"] = string(extraJSON)
	}
	if verify != nil && strings.TrimSpace(verify.RawBody) != "" {
		// 把最近一次 userinfo 原文存到 extra，方便排查模型不生效等问题
		extras["last_userinfo_raw"] = verify.RawBody
	}
	if b, e := json.Marshal(extras); e == nil {
		account.RawExtraJSON = string(b)
	}
	account.DisplayName = displayName
	account.AvatarURL = avatar
	account.VendorUserID = uid
	account.TokenExpiresAt = expireAtPtr
	// 绑定成功后默认设激活，但只在事务里做（repository.Save + Activate 一起）
	saved, err := repository.SaveUserVendorAccount(account)
	if err != nil {
		return nil, fmt.Errorf("保存绑定账户失败: %w", err)
	}
	// 默认激活该供应商：让用户绑定完不需要再切一次
	if err := repository.ActivateUserVendorAccount(userID, saved.ID); err != nil {
		// 激活失败不影响绑定成功结果，只警告（前端看到已绑定但未激活，用户手动再切一下）
		_ = err
	}
	// 绑定成功后立刻拉一次模型快照（适配器未注册时 FetchAndStoreVendorModels 内部静默跳过，不阻塞绑定）
	_ = FetchAndStoreVendorModels(userID, &saved)
	// 拉一次余额（LibTV / NewWow / UpDream）：失败不阻塞绑定
	bindCtx, bindCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer bindCancel()
	_ = FetchAndStoreVendorBalance(bindCtx, &saved)
	// 再查一次绑定列表，拿到脱敏后的最新一条返回
	list, err := PublicBoundAccounts(userID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].VendorType == p.VendorType {
			return &list[i], nil
		}
	}
	// 极端情况刚保存没查到，构造一条默认返回
	return &model.PublicBoundAccount{
		VendorType: p.VendorType,
		IsActive:   true,
		DisplayName: displayName,
		AvatarURL:   avatar,
		BoundAt:     saved.BoundAt,
		LastUsedAt:  saved.LastUsedAt,
		HasModels:   false,
	}, nil
}

// EstimateVendorCost 实时估算指定供应商、指定参数组合的单次扣费额度。
// 返回 (credits, source)。source = "estimate" 表示供应商真实接口返回；source = "fallback" 表示
// 供应商未绑定、未实现估算接口或接口失败，前端应继续用 requestCreditCost 静态估算兜底。
func EstimateVendorCost(ctx context.Context, userID string, vendorType string, input EstimateCostInput) (float64, string, error) {
	if userID == "" {
		return 0, "fallback", errors.New("请先登录")
	}
	vendorType = strings.ToLower(strings.TrimSpace(vendorType))
	if !model.ValidVendorType(vendorType) {
		return 0, "fallback", errors.New("未知供应商类型")
	}
	if vendorType == model.VendorTypeOfficial {
		return 0, "fallback", nil
	}
	account, err := repository.GetUserVendorAccountByType(userID, vendorType)
	if err != nil {
		return 0, "fallback", fmt.Errorf("查询绑定账户失败: %w", err)
	}
	if account == nil {
		return 0, "fallback", nil
	}
	dbVendor, _ := repository.GetVendorByType(vendorType)
	vendor := NormalizeVendorAuthModeForDispatch(vendorType, dbVendor)
	if vendor == nil {
		return 0, "fallback", nil
	}
	adapter, ok := NewVendorAdapter(vendor)
	if !ok {
		return 0, "fallback", nil
	}
	est, ok := adapter.(VendorCostEstimator)
	if !ok {
		return 0, "fallback", nil
	}
	credits, err := est.EstimateCost(ctx, account, input)
	if err != nil {
		return 0, "fallback", err
	}
	return credits, "estimate", nil
}

// timeoutContext 统一超时 context
func timeoutContext(d time.Duration) (context.Context, context.CancelFunc) {
	// 复用 context 包即可（service/context 下若已有同名函数就调整，但看项目里没用到）
	return context.WithTimeout(context.Background(), d)
}

// looksLikeJWT 粗判字符串是否像 JWT（防御性拦 Cookie 框误贴）。
// JWT 形态特征：
//   - 以 "eyJ" 开头（base64 编码的 JSON 头部第一个字符几乎固定）
//   - 用两个 "." 分成三段（header.payload.signature）
//   - 不含 ";"（Cookie 用分号分隔；如果含分号基本可断定为 cookie）
// 三条件同时满足返回 true；任一不满足返回 false（避免误杀合法的 cookie 串）。
func looksLikeJWT(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 16 { // 最短合法 JWT 也得有几十字符，< 16 直接 false
		return false
	}
	if !strings.HasPrefix(s, "eyJ") {
		return false
	}
	if strings.Count(s, ".") != 2 {
		return false
	}
	if strings.Contains(s, ";") {
		return false
	}
	return true
}

