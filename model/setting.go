package model

import (
	"encoding/json"
	"strings"
)

type SettingKey string

const (
	SettingKeyPublic  SettingKey = "public"
	SettingKeyPrivate SettingKey = "private"

	StorageProviderTypeS3     = "s3"
	StorageProviderTypeWebDAV = "webdav"
	StorageProviderTypeLocal   = "local"
)

// ModelChannel 模型渠道配置。
type ModelChannel struct {
	ID          string            `json:"id"`
	Protocol    string            `json:"protocol"`
	Name        string            `json:"name"`
	BaseURL     string            `json:"baseUrl"`
	APIKey      string            `json:"apiKey"`
	Models      []string          `json:"models"`
	ModelLabels map[string]string `json:"modelLabels,omitempty"` // 模型ID → 显示别名（前端下拉框显示别名，API请求仍用模型ID）
	Weight      int               `json:"weight"`
	Timeout     int               `json:"timeout"`
	Enabled     bool              `json:"enabled"`
	Remark      string            `json:"remark"`

	// Sprint 2 新增：渠道选择器（多 key + 优先级 + 状态码 failover）
	// 字段兼容：以下字段均为 omitempty，老配置 JSON 反序列化时全部为默认值，行为不变。
	Priority          int      `json:"priority,omitempty"`           // 数字小=优先（默认 0，与 Weight 随机选择兼容）
	StatusCodeMapping string   `json:"statusCodeMapping,omitempty"`  // "429,500,502,503,504" 命中即视为该渠道失败；空=默认 429/5xx
	CooldownSeconds   int      `json:"cooldownSeconds,omitempty"`    // 失败后冷却秒数（默认 60s）
	Keys              []string `json:"keys,omitempty"`              // 多 key 列表；空时回退到 APIKey（兼容老配置）
	Group             string   `json:"group,omitempty"`             // 预留 Sprint 3（Sprint 2 不强校验；所有 enabled 都在 default group）
	Capability        string   `json:"capability,omitempty"`        // "text"/"image"/"video"/"audio"；空=通用（匹配所有 capability 查询）
	AutoBan           bool     `json:"autoBan,omitempty"`           // 失败后是否自动冷却（默认 true，false=不冷却）
}

// ChannelKeys 返回该渠道的完整 key 列表（兼容老配置：Keys 为空时返回 [APIKey]）。
func (c *ModelChannel) ChannelKeys() []string {
	if len(c.Keys) > 0 {
		return c.Keys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// ModelCostUnit 扣费单位。
// per_call  : 按次扣费（默认），图片/文本/视频生成一次扣 CostCents 分；图片再乘以请求中的 count 数量
// per_second: 按秒扣费（仅视频模型），实际扣额 = CostCentsPerSecond * 视频秒数；当 CostCentsPerSecond>0 时优先用按秒模式
const (
	ModelCostUnitPerCall   = "per_call"
	ModelCostUnitPerSecond = "per_second"
)

// ModelCost 模型扣费配置（单位：分 cents，1 元 = 100 cents）。
//   - 图片模型：CostCents 表示"每张/每次"扣多少分，最终扣额 = CostCents * count（请求的生成数量）
//   - 文本模型：CostCents 表示"每请求"扣多少分
//   - 视频模型：Unit=per_second 时按 CostCentsPerSecond * 视频秒数 扣；否则按 CostCents * 任务数扣
type ModelCost struct {
	Model              string `json:"model"`
	Label              string `json:"label,omitempty"`               // 模型显示别名（前端下拉/展示用；空=显示真实模型ID。API 请求仍用 Model）
	CostCents          int    `json:"costCents"`                    // 单次扣费（分）
	Unit               string `json:"unit,omitempty"`               // per_call | per_second
	AutoPriced         bool   `json:"autoPriced,omitempty"`         // 是否由自动定价调度器写入（区分自动价与手工价；清理下架模型时只删自动价，避免误删手工价导致变免费）
	CostCentsPerSecond int    `json:"costCentsPerSecond,omitempty"` // Unit=per_second 时，每秒扣费（分）
	RefVideo           *bool  `json:"refVideo,omitempty"`           // 是否支持视频参考上传（仅视频模型有意义；nil=回退白名单推断）
	RefAudio           *bool  `json:"refAudio,omitempty"`           // 是否支持音频参考上传（nil=回退白名单推断）
	GenAudio           *bool  `json:"genAudio,omitempty"`           // 是否支持生成同步音频（nil=回退白名单推断）
	// Sprint 3：per-model per-group 倍率覆盖
	// 格式：{"plus": 0.5, "pro": 0.3}（0.5 = 5 折；空=不覆盖，走 groupRatio）
	// 实际计费 = baseCents * groupRatio * modelGroupRatio，向下取整
	GroupPricingJSON string `json:"groupPricingJson,omitempty" gorm:"type:text"`
}

// GetGroupPricingRatio 返回该 model 对指定 group 的倍率（0~1，空=1.0）。
// JSON 解析失败时安全返回 1.0（不阻断计费）。
func (m *ModelCost) GetGroupPricingRatio(groupID string) float64 {
	if strings.TrimSpace(m.GroupPricingJSON) == "" || strings.TrimSpace(groupID) == "" {
		return 1.0
	}
	var ratios map[string]float64
	if err := json.Unmarshal([]byte(m.GroupPricingJSON), &ratios); err != nil {
		return 1.0
	}
	if r, ok := ratios[groupID]; ok && r > 0 {
		return r
	}
	return 1.0
}

// PublicModelChannelSetting 公开模型渠道配置。
type PublicModelChannelSetting struct {
	AvailableModels        []string                 `json:"availableModels"`
	ModelCosts             []ModelCost              `json:"modelCosts"`
	Channels               []PublicModelChannelInfo `json:"channels"`
	SystemPrompt           string                   `json:"systemPrompt"`
	SystemPrompts          SystemPromptSetting      `json:"systemPrompts"`
	AllowCustomChannel     *bool                    `json:"allowCustomChannel"`
	AllowUserRemoteChannel *bool                    `json:"allowUserRemoteChannel"`
}

type SystemPromptSetting struct {
	Image         string `json:"image"`
	Video         string `json:"video"`
	Text          string `json:"text"`
	Workflow      string `json:"workflow"`
	WorkflowAgent string `json:"workflowAgent"`
	// 分镜剧本提示词：把小说章节整合成完整分镜剧本的改写风格
	StoryboardScript string `json:"storyboardScript"`
	// 分镜视频提示词：把分镜剧本转成视频描述词的改写风格
	StoryboardVideo string `json:"storyboardVideo"`
	// 分镜图片提示词：角色三视图/场景四宫格/道具标准图等资产生图模板
	StoryboardImage string `json:"storyboardImage"`
}

type PublicModelChannelInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	BaseURL     string            `json:"baseUrl"`
	Models      []string          `json:"models"`
	ModelLabels map[string]string `json:"modelLabels,omitempty"` // 模型ID → 显示别名
	Weight      int               `json:"weight"`
	Timeout     int               `json:"timeout"`
	Enabled     bool              `json:"enabled"`
	Remark      string            `json:"remark"`

	// Sprint 2 新增：前端展示用
	Priority          int      `json:"priority,omitempty"`
	StatusCodeMapping string   `json:"statusCodeMapping,omitempty"`
	CooldownSeconds   int      `json:"cooldownSeconds,omitempty"`
	KeyCount          int      `json:"keyCount,omitempty"` // 多 key 数量（不返明文，仅数量）
	Group             string   `json:"group,omitempty"`
	Capability        string   `json:"capability,omitempty"`
}

// PublicSetting 公开配置。
type PublicSetting struct {
	ModelChannel   PublicModelChannelSetting `json:"modelChannel"`
	Auth           PublicAuthSetting         `json:"auth"`
	Storage        PublicStorageSetting      `json:"storage"`
	SiteNotice     SiteNoticeSetting         `json:"siteNotice"`
	ContactSupport ContactSupportSetting     `json:"contactSupport"`
}

type SiteNoticeSetting struct {
	Enabled  bool     `json:"enabled"`
	Title    string   `json:"title"`
	Contents []string `json:"contents"`
}

// ContactSupportSetting 联系客服配置。
type ContactSupportSetting struct {
	Enabled   bool   `json:"enabled"`
	WeChat    string `json:"wechat"`
	QQ        string `json:"qq"`
	WeChatQR  string `json:"wechatQr"`
	QQGroup   string `json:"qqGroup"`
	QQGroupQR string `json:"qqGroupQr"`
	Remark    string `json:"remark"`
}

type PublicStorageSetting struct {
	Mode                    string `json:"mode"`
	AllowUserProvider       bool   `json:"allowUserProvider"`
	AllowUserGlobalProvider bool   `json:"allowUserGlobalProvider"`
}

type PublicAuthSetting struct {
	AllowRegister *bool                    `json:"allowRegister"`
	LinuxDo       PublicLinuxDoAuthSetting `json:"linuxDo"`
}

type PublicLinuxDoAuthSetting struct {
	Enabled bool `json:"enabled"`
}

// PrivateSetting 私有配置。
type PrivateSetting struct {
	Channels    []ModelChannel        `json:"channels"`
	PromptSync  PromptSyncSetting     `json:"promptSync"`
	AILog       AILogSetting          `json:"aiLog"`
	Auth        PrivateAuthSetting    `json:"auth"`
	Storage     PrivateStorageSetting `json:"storage"`
	Affiliate   AffiliateSetting      `json:"affiliate"`
	// Sprint 3：group 维度统一倍率（key=groupID, value=倍率 0~1；缺省=1.0）
	// 典型：{"default": 1.0, "plus": 0.8, "pro": 0.6, "enterprise": 0.4}
	GroupRatios map[string]float64 `json:"groupRatios,omitempty"`
}

// AffiliateSetting 邀请返佣配置（仅官方托管版生效；自部署 fork 不结算）。
// 只做一级直推返佣，避免多级分销合规风险。
// 返佣锚定「被邀请人的实际消费额」，按邀请人当前邀请人数阶梯计算比例。
type AffiliateSetting struct {
	Enabled bool `json:"enabled"`
	// 阶梯返佣：邀请人每多邀请 1 人，比例 +StepRate，封顶 MaxRate。
	// 例：BaseRate=0.05, StepRate=0.01, MaxRate=0.10 →
	//   1人=5%, 2人=6%, 3人=7%, 4人=8%, 5人=9%, 6人及以上=10%（封顶）。
	BaseRate float64 `json:"baseRate"` // 邀请 1 人时的比例，如 0.05 = 5%
	StepRate float64 `json:"stepRate"` // 每多邀请 1 人增加的比例，如 0.01 = 1%
	MaxRate  float64 `json:"maxRate"`  // 比例上限，如 0.10 = 10%
	MinSettleCents int `json:"minSettleCents"` // 单次返佣低于此分（如 1 分）则不结算，避免零头噪音
}

type AILogSetting struct {
	LocalDirectReportEnabled *bool               `json:"localDirectReportEnabled"`
	Cleanup                  AILogCleanupSetting `json:"cleanup"`
}

type AILogCleanupSetting struct {
	Enabled       *bool  `json:"enabled"`
	RetentionDays int    `json:"retentionDays"`
	Cron          string `json:"cron"`
}

type PrivateStorageSetting struct {
	Mode                    string                      `json:"mode"`
	AllowUserProvider       bool                        `json:"allowUserProvider"`
	AllowUserGlobalProvider bool                        `json:"allowUserGlobalProvider"`
	Providers               []StorageProvider           `json:"providers"`
	RoundRobinCursor        int                         `json:"roundRobinCursor"`
	CapacityCheck           StorageCapacityCheckSetting `json:"capacityCheck"`
	CapacityLimitBytes      int64                       `json:"capacityLimitBytes"`
}

type StorageProvider struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"`
	Endpoint          string `json:"endpoint"`
	Region            string `json:"region"`
	Bucket            string `json:"bucket"`
	AccessKeyID       string `json:"accessKeyId"`
	SecretAccessKey   string `json:"secretAccessKey"`
	PublicBaseURL     string `json:"publicBaseUrl"`
	PathPrefix        string `json:"pathPrefix"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	Weight            int    `json:"weight"`
	Enabled           bool   `json:"enabled"`
	OwnerUserID       string `json:"ownerUserId"`
	CapacityBytes     int64  `json:"capacityBytes"`
	CapacityCheckedAt string `json:"capacityCheckedAt"`
	CapacityExceeded  bool   `json:"capacityExceeded"`
}

type StorageCapacityCheckSetting struct {
	Enabled *bool  `json:"enabled"`
	Cron    string `json:"cron"`
}

// PromptSyncSetting 提示词定时同步配置。
type PromptSyncSetting struct {
	Enabled *bool  `json:"enabled"`
	Cron    string `json:"cron"`
}

type PrivateAuthSetting struct {
	LinuxDo PrivateLinuxDoAuthSetting `json:"linuxDo"`
}

type PrivateLinuxDoAuthSetting struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// Setting 系统配置。
type Setting struct {
	Key       SettingKey      `json:"key" gorm:"primaryKey"`
	// 系统配置 JSON 体积大（含多段 systemPrompt），必须用 longtext，否则在 MySQL 下会被全局 DefaultStringSize(191) 建成 varchar(191) 导致 "Data too long" 写入失败。
	Value     json.RawMessage `json:"value" gorm:"serializer:json;type:longtext"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

// Settings 系统公开和私有配置。
type Settings struct {
	Public  PublicSetting  `json:"public"`
	Private PrivateSetting `json:"private"`
}
