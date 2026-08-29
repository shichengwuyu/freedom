package model

import "time"

// VendorType 云端供应商类型常量（唯一业务标识，vendors.type & user_vendor_accounts.vendor_type 共用）
const (
	VendorTypeOfficial = "official" // 官方：沿用现有 ModelChannel（管理员后台配置的渠道），不需要 OAuth
	VendorTypeUpDream  = "updream"  // UpDream 云端平台
	VendorTypeLibTV    = "libtv"    // LibTV 云端平台
	VendorTypeNewWow   = "newwow"   // NewWow 云端平台
)

// VendorAuthMode 鉴权模式常量（vendors.auth_mode 字段使用）
const (
	// VendorAuthModeCookie 默认：account.AccessToken 当 Cookie 字符串注入 HTTP 请求头
	VendorAuthModeCookie = "cookie"
	// VendorAuthModeCustomHeader 走自定义 HTTP header：account.AccessToken 注入到 vendor.AuthHeaderName 命名的 header
	VendorAuthModeCustomHeader = "custom_header"
	// VendorAuthModeOpenAPISignature 走 AK/SK + 签名（如 LibTV 开放平台）；不通过 AccessToken 字段注入 header
	VendorAuthModeOpenAPISignature = "openapi_signature"
)

// Vendor 系统级供应商元信息（管理员在后台配置，所有用户共享；不含用户任何数据）
type Vendor struct {
	// ID 主键，建议由 stableVendorID() 生成
	ID string `json:"id" gorm:"primaryKey"`
	// Type 供应商类型枚举：official / updream / libtv / newwow（业务真正依赖的标识，唯一索引业务上要求不重复）
	Type string `json:"type" gorm:"type:varchar(32);index"`
	// Name 前端展示名，如"UpDream 云端创作平台"
	Name string `json:"name"`
	// LogoURL 前端下拉框 / 绑定卡片的 logo 图片地址
	LogoURL string `json:"logoUrl"`
	// OAuthAuthURL 第三方授权页地址（official 为空；其他供应商如 https://auth.updream.com/authorize）
	OAuthAuthURL string `json:"oauthAuthUrl"`
	// OAuthTokenURL 用 code 换 token 的上游地址
	OAuthTokenURL string `json:"oauthTokenUrl"`
	// OAuthClientID 本项目在供应商侧注册的 App ClientID
	OAuthClientID string `json:"oauthClientId"`
	// OAuthClientSecret 供应商侧 App 密钥（后端保存，脱敏接口返回空字符串）
	OAuthClientSecret string `json:"-" gorm:"column:oauth_client_secret"` // json:"-" 确保任何序列化都不返回给前端
	// OAuthRedirectURI 本项目的回调地址，如 https://xxx.com/api/vendor/oauth/callback/updream
	OAuthRedirectURI string `json:"oauthRedirectUri"`
	// APIRootURL 供应商 API 根地址（Adapter 里拼路径用）
	APIRootURL string `json:"apiRootUrl"`
	// AuthMode 鉴权模式（决定凭证注入到 HTTP 哪个部位）：
	//   "" / "cookie"             → req.Header.Set("Cookie", account.AccessToken)（默认）
	//   "custom_header"           → req.Header.Set(AuthHeaderName, account.AccessToken)（NewWow 用 accesstoken header）
	//   "openapi_signature"       → 走 AK/SK 签名（LibTV），不通过 AccessToken 字段
	AuthMode string `json:"authMode" gorm:"column:auth_mode;type:varchar(32)"`
	// AuthHeaderName 仅 custom_header 模式生效，例如 NewWow 的 "accesstoken"
	AuthHeaderName string `json:"authHeaderName" gorm:"column:auth_header_name;type:varchar(64)"`
	// Enabled 是否启用：管理员停用后，前端不展示，后端 API 也拦截
	Enabled bool `json:"enabled"`
	// Sort 前端下拉排序（小的在前）
	Sort int `json:"sort"`
	// ExtraConfigJSON 供应商专属配置兜底（字段名规范、额外 Header 名等），P0 暂时空即可
	ExtraConfigJSON string `json:"extraConfigJson,omitempty" gorm:"type:longtext"`
	// CreatedAt 创建时间（GORM 惯例不用 pointer；string 类型保持和现有 model 一致：存 RFC3339）
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 最近更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// PublicVendorInfo 返回给前端的脱敏供应商信息（不含 ClientSecret 等敏感字段）
// 对应 GET /api/vendors 的返回元素
type PublicVendorInfo struct {
	Type           string `json:"type"`           // 供应商类型：official/updream/...
	Name           string `json:"name"`           // 显示名
	LogoURL        string `json:"logoUrl"`        // logo
	Enabled        bool   `json:"enabled"`        // 是否启用
	Sort           int    `json:"sort"`           // 排序
	HasOAuth       bool   `json:"hasOAuth"`       // 是否需要 OAuth 登录（official=false，其他=true）
	APIRootHint    string `json:"apiRootHint"`    // 可选：API 根地址提示，前端展示用
	AuthMode       string `json:"authMode"`       // 鉴权模式（前端 placeholder 用："cookie"/"custom_header"/"openapi_signature"）
	AuthHeaderName string `json:"authHeaderName,omitempty"` // 仅 custom_header 模式：前端 placeholder 用
}

// UserVendorAccount 用户绑定的某一家供应商账户（每个 user + vendor 最多一条）
type UserVendorAccount struct {
	// ID 主键
	ID string `json:"id" gorm:"primaryKey"`
	// UserID 关联的用户 ID（来自 users 表）
	UserID string `json:"userId" gorm:"index"`
	// VendorType 供应商类型（对应 vendors.type）
	VendorType string `json:"vendorType" gorm:"type:varchar(32);index"`
	// VendorID 冗余关联 vendors.id（方便管理员换供应商配置时排查）
	VendorID string `json:"vendorId"`
	// DisplayName 供应商侧的昵称，如"UpDream-小明同学"（前端账户卡片展示）
	DisplayName string `json:"displayName"`
	// AvatarURL 供应商侧头像 URL
	AvatarURL string `json:"avatarUrl,omitempty"`
	// AccessToken 加密后的 OAuth Access Token（实际存储策略：应用层 AES-GCM，Serializer 层处理；P0 先明文存空串，P1 再上加密）
	AccessToken string `json:"-" gorm:"column:access_token;type:text"`
	// RefreshToken 加密后的 Refresh Token
	RefreshToken string `json:"-" gorm:"column:refresh_token;type:text"`
	// TokenExpiresAt AccessToken 过期时间；nil 表示未知/永不过期
	TokenExpiresAt *time.Time `json:"tokenExpiresAt,omitempty"`
	// Scope 授权 scope 透传（例："images:write assets:read video:generate"）
	Scope string `json:"scope,omitempty"`
	// IsActive 当前激活标记：每个 user_id 同时只能有一条为 true（切换供应商时其他全部置 false）
	IsActive bool `json:"isActive" gorm:"index"`
	// AvailableModelsJSON 该账户可用模型快照 JSON（结构见文档 §3.3），绑定成功/手动刷新时更新
	AvailableModelsJSON string `json:"availableModelsJson,omitempty" gorm:"type:longtext"`
	// BalanceInfoJSON 余额/套餐快照（前端直接展示，如 "余额 ¥128.50 / Pro 年卡 362 天"）
	BalanceInfoJSON string `json:"balanceInfoJson,omitempty" gorm:"type:longtext"`
	// VendorUserID 供应商侧用户唯一 ID（用于防止同一个供应商账户绑多个本项目用户，或重绑时识别原账户）
	VendorUserID string `json:"vendorUserId,omitempty"`
	// RawExtraJSON 供应商返回的个性化数据兜底，避免后续字段反复加表
	RawExtraJSON string `json:"rawExtraJson,omitempty" gorm:"type:longtext"`
	// BoundAt 首次绑定时间
	BoundAt time.Time `json:"boundAt"`
	// LastUsedAt 最近一次用这个账户发起 AI 请求的时间（排序/清理/审计用）
	LastUsedAt time.Time `json:"lastUsedAt"`
	// CreatedAt / UpdatedAt 常规时间戳
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PublicBoundAccount 返回给前端的脱敏绑定账户信息（绝对不含 token）
type PublicBoundAccount struct {
	VendorType      string     `json:"vendorType"`      // 供应商类型
	IsActive        bool       `json:"isActive"`        // 是否当前激活
	DisplayName     string     `json:"displayName"`     // 昵称
	AvatarURL       string     `json:"avatarUrl,omitempty"` // 头像
	BalanceText     string     `json:"balanceText,omitempty"` // 余额文案（由 BalanceInfoJSON 预渲染）
	HasModels       bool       `json:"hasModels"`       // 是否已经拉过模型快照
	AvailableModelsJSON string `json:"availableModelsJson,omitempty"` // 模型快照原文（前端 buildVendorEffectiveConfig 直接消费）
	// PowerHistory 模型消耗积分历史：{ "<modelKey>": { "power": 1, "updatedAt": "2026-08-16T..." } }
	// 每次任务成功后由 recordSuccess 写入；前端可在模型下拉里显示"上次消耗 X power"
	PowerHistory    map[string]VendorPowerRecord `json:"powerHistory,omitempty"`
	BoundAt         time.Time  `json:"boundAt"`         // 绑定时间
	LastUsedAt      time.Time  `json:"lastUsedAt"`      // 最近使用时间
}

// VendorPowerRecord 单条 modelKey 的积分消耗记录
type VendorPowerRecord struct {
	Power     int    `json:"power"`     // 单次消耗积分
	UpdatedAt string `json:"updatedAt"` // ISO 时间
}

// AllVendorTypes 全部内置供应商类型数组（用于遍历校验）
var AllVendorTypes = []string{
	VendorTypeOfficial,
	VendorTypeUpDream,
	VendorTypeLibTV,
	VendorTypeNewWow,
}

// ValidVendorType 校验供应商类型是否合法
func ValidVendorType(t string) bool {
	for _, v := range AllVendorTypes {
		if v == t {
			return true
		}
	}
	return false
}
