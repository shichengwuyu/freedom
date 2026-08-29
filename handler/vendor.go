package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tigerowo/freedom/service"
)

// Vendors 返回公开可用的供应商列表（脱敏，不需要登录）
// 对应 GET /api/vendors
// P0 阶段即使 DB 空也返回 4 家内置默认，保证前端 UI 不空白。
func Vendors(w http.ResponseWriter, r *http.Request) {
	vendors, err := service.PublicVendors()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, vendors)
}

// VendorAccounts 返回当前用户绑定的供应商账户列表（需登录；游客返回空数组）
// 对应 GET /api/v1/vendor/accounts
func VendorAccounts(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	var userID string
	if ok {
		userID = user.ID
	}
	accounts, err := service.PublicBoundAccounts(userID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, accounts)
}

// activateVendorRequest 切换激活供应商的请求体
type activateVendorRequest struct {
	// VendorType 要激活的供应商类型：official / updream / libtv / newwow
	VendorType string `json:"vendorType"`
}

// ActivateVendor 切换当前激活的供应商（需登录）
// 对应 POST /api/v1/vendor/activate
// - VendorType = "official"：切回官方云端（保留现有 channelMode / 本地渠道能力）
// - 其他：先要有绑定账户，然后把该账户标 IsActive=true，其他全部 false
func ActivateVendor(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req activateVendorRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求体格式错误")
		return
	}
	req.VendorType = strings.ToLower(strings.TrimSpace(req.VendorType))
	if err := service.ActivateVendor(user.ID, req.VendorType); err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{
		"activated": req.VendorType,
	})
}

// ========== P1 新增：按 AccessToken / Cookie / AccessKey 绑定供应商账户 ==========

// BindVendorByCookieRequest 绑定请求体（浏览器插件或手动粘贴 AccessToken / Cookie）
type BindVendorByCookieRequest struct {
	VendorType   string `json:"vendorType"`
	CookieString string `json:"cookieString"`
	DisplayName  string `json:"displayName,omitempty"`
	AvatarURL    string `json:"avatarUrl,omitempty"`
	VendorUserID string `json:"vendorUserId,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	AccessKey    string `json:"accessKey,omitempty"`
	AccessSecret string `json:"accessSecret,omitempty"`
	AppKey       string `json:"appKey,omitempty"`
	// custom_header 鉴权模式（UpDream Authorization Bearer / NewWow accesstoken）：AuthHeaderName 一般由后端 vendor 元信息决定，
	// 前端仅传 value 即可；若前端想自定义 header 名（极小概率），也可以传 AuthHeaderName。
	AuthHeaderName  string `json:"authHeaderName,omitempty"`
	AuthHeaderValue string `json:"authHeaderValue,omitempty"`
	// AuthExtraHeader*：cookie 模式叠加鉴权 header（旧版 UpDream 双链路兼容，新绑定用 AuthHeaderValue 即可）。
	// 用户粘完整的 "Authorization: Bearer <JWT>" 时，把 Bearer 前缀也写在 AuthExtraHeaderValue 里即可。
	AuthExtraHeaderName  string `json:"authExtraHeaderName,omitempty"`
	AuthExtraHeaderValue string `json:"authExtraHeaderValue,omitempty"`
}

// BindVendorByCookie 把用户提供的 AccessToken / Cookie / AccessKey 绑定成供应商账户。
// 对应 POST /api/v1/vendor/bind-cookie
// 内部会先拿凭证调供应商用户信息接口做一次真实校验（SSRF 防护 + host 白名单），通过后再落库。
// 绑定成功后自动把该供应商设为激活（返回 200 前端自动切 activeVendorType）。
func BindVendorByCookie(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req BindVendorByCookieRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求体格式错误")
		return
	}
	req.VendorType = strings.ToLower(strings.TrimSpace(req.VendorType))
	bound, err := service.BindVendorByCookie(user.ID, service.BindVendorByCookieParams{
		VendorType:           req.VendorType,
		CookieString:         req.CookieString,
		DisplayName:          req.DisplayName,
		AvatarURL:            req.AvatarURL,
		VendorUserID:         req.VendorUserID,
		ExpiresAt:            req.ExpiresAt,
		AccessKey:            req.AccessKey,
		AccessSecret:         req.AccessSecret,
		AppKey:               req.AppKey,
		AuthHeaderName:       req.AuthHeaderName,
		AuthHeaderValue:      req.AuthHeaderValue,
		AuthExtraHeaderName:  req.AuthExtraHeaderName,
		AuthExtraHeaderValue: req.AuthExtraHeaderValue,
	})
	if err != nil {
		// 绑定失败大多是业务校验错误（Token 失效 / Cookie 失效 / 未命中必要 Key 等），直接透出具体原因给用户/插件
		Fail(w, err.Error())
		return
	}
	OK(w, map[string]any{
		"vendorType": req.VendorType,
		"account":    bound,
	})
}

// ========== P1 新增：刷新模型快照 / 解绑供应商 ==========

// refreshVendorModelsRequest 刷新模型快照请求体
type refreshVendorModelsRequest struct {
	VendorType string `json:"vendorType"`
}

// VendorRefreshModels 手动重新拉取某家供应商的可用模型快照并落库。
// 对应 POST /api/v1/vendor/refresh-models（body: { vendorType }）
// 返回更新后的脱敏账户（含最新 availableModelsJson），前端据此刷新模型下拉。
func VendorRefreshModels(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req refreshVendorModelsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求体格式错误")
		return
	}
	req.VendorType = strings.ToLower(strings.TrimSpace(req.VendorType))
	account, err := service.RefreshVendorModels(user.ID, req.VendorType)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, account)
}

// VendorRefreshBalance 手动重新拉取某家供应商的余额/套餐快照并落库。
// 对应 POST /api/v1/vendor/refresh-balance（body: { vendorType }）
// 返回更新后的脱敏账户（含最新 balanceText），前端据此刷新右上角余额 chip。
func VendorRefreshBalance(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req refreshVendorModelsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求体格式错误")
		return
	}
	req.VendorType = strings.ToLower(strings.TrimSpace(req.VendorType))
	account, err := service.RefreshVendorBalance(r.Context(), user.ID, req.VendorType)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, account)
}

// unbindVendorRequest 解绑请求体
type unbindVendorRequest struct {
	VendorType string `json:"vendorType"`
}

// VendorUnbind 解绑当前用户在某家供应商的绑定账户。
// 对应 POST /api/v1/vendor/unbind（body: { vendorType }）
// 解绑成功后若被删账户原本激活，后端自动回落官方模式。
func VendorUnbind(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req unbindVendorRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求体格式错误")
		return
	}
	req.VendorType = strings.ToLower(strings.TrimSpace(req.VendorType))
	if err := service.UnbindVendor(user.ID, req.VendorType); err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{
		"vendorType": req.VendorType,
		"unbound":    true,
	})
}

// ========== P1 新增：浏览器插件嗅探样本回传（UpDream/NewWow 无开放平台，靠插件抓内部接口样本）==========

// vendorCaptureSampleRequest 接收插件回传的样本请求体
type vendorCaptureSampleRequest struct {
	VendorType string                  `json:"vendorType"`
	Sample     service.VendorSampleInput `json:"sample"`
}

// VendorCaptureSample 接收浏览器插件嗅探到的供应商内部接口样本并存库。
// 对应 POST /api/v1/vendor/capture-sample（body: { vendorType, sample }）
// 样本是后端后续用该用户 Cookie 重放生图请求的依据，因此要求用户已绑定该供应商账户。
func VendorCaptureSample(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req vendorCaptureSampleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求体格式错误")
		return
	}
	req.VendorType = strings.ToLower(strings.TrimSpace(req.VendorType))
	if req.VendorType == "" {
		Fail(w, "缺少 vendorType")
		return
	}
	saved, err := service.SaveVendorApiSample(user.ID, req.VendorType, req.Sample)
	if err != nil {
		// 透出具体原因（如「请先绑定该供应商账户」），否则插件/用户只能看到笼统的「操作失败」
		Fail(w, err.Error())
		return
	}
	OK(w, map[string]any{
		"stored":            true,
		"id":                saved.ID,
		"isLikelyGeneration": saved.IsLikelyGeneration,
		"endpointGroup":     saved.EndpointGroup,
	})
}

// VendorListSamples 列出当前用户被嗅探到的样本（调试/核对用）。
// 对应 GET /api/v1/vendor/samples?vendorType=&onlyGeneration=
func VendorListSamples(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	vendorType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("vendorType")))
	onlyGen := r.URL.Query().Get("onlyGeneration") == "true" || r.URL.Query().Get("onlyGeneration") == "1"
	samples, err := service.ListVendorApiSamples(user.ID, vendorType, onlyGen, 200)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, samples)
}

// VendorClearSamples 清空某用户某供应商的样本（vendorType 空则清空该用户全部）。
// 对应 POST /api/v1/vendor/clear-samples（body: { vendorType }）
func VendorClearSamples(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req unbindVendorRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	req.VendorType = strings.ToLower(strings.TrimSpace(req.VendorType))
	n, err := service.DeleteVendorApiSamples(user.ID, req.VendorType)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"deleted": n})
}

// vendorEstimateCostRequest 供应商额度估算请求
// 对应 POST /api/v1/vendor/estimate-cost
type vendorEstimateCostRequest struct {
	VendorType    string `json:"vendorType"`
	Capability    string `json:"capability"`
	Model         string `json:"model"`
	Quality       string `json:"quality,omitempty"`
	Size          string `json:"size,omitempty"`
	Count         int    `json:"count,omitempty"`
	RefImageCount int    `json:"refImageCount,omitempty"`
	RefVideoCount int    `json:"refVideoCount,omitempty"`
	HasSound      bool   `json:"hasSound,omitempty"`
}

// VendorEstimateCost 实时估算当前参数组合要扣除的供应商 credits。
// 目前仅 UpDream 实现真实估算；其他供应商或失败时返回 source="fallback"，前端继续用静态 requestCreditCost 兜底。
func VendorEstimateCost(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req vendorEstimateCostRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求体格式错误")
		return
	}
	credits, source, err := service.EstimateVendorCost(r.Context(), user.ID, req.VendorType, service.EstimateCostInput{
		Capability:    req.Capability,
		Model:         req.Model,
		Quality:       req.Quality,
		Size:          req.Size,
		Count:         req.Count,
		RefImageCount: req.RefImageCount,
		RefVideoCount: req.RefVideoCount,
		HasSound:      req.HasSound,
	})
	if err != nil {
		// 估算失败不阻断前端：返回 fallback，让前端继续展示静态成本或 0
		OK(w, map[string]any{"credits": 0, "source": "fallback", "error": err.Error()})
		return
	}
	OK(w, map[string]any{"credits": credits, "source": source})
}
