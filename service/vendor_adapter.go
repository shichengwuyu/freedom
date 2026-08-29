package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
)

// ErrNotSupported 表示某适配器不支持该能力（如某家供应商不提供资产库）。
// 调用方判断 errors.Is(err, ErrNotSupported) 后给用户弹"该供应商暂不支持"。
var ErrNotSupported = errors.New("vendor adapter: operation not supported")

// VendorVideoSubmitter 可选接口：支持「提交即返回供应商任务 ID、后续异步轮询」视频生成的适配器实现。
// 视频任务分发链路（handler/video_task.go）用类型断言探测；未实现该接口的适配器视为不支持视频任务，
// 会给用户清晰的"该供应商暂不支持视频生成"提示。
type VendorVideoSubmitter interface {
	SubmitVideo(ctx context.Context, account *model.UserVendorAccount, input GenerateVideoInput) (string, error)
}

// ========== 通用返回 ==========

// GeneratedAssetItem 统一生成输出（图片/视频/音频通用）
type GeneratedAssetItem struct {
	ID         string         // 供应商侧任务/资产 ID
	URL        string         // 供应商 CDN URL（临时或永久）
	StorageKey string         // 如果触发了双写，这里是本项目 S3/WebDAV 的 key
	Data       []byte         // 内联返回的字节（小图/短音频可选，否则留空走 URL 下载）
	Width      int
	Height     int
	DurationMs int    // 视频 / 音频时长
	Bytes      int
	MimeType   string // 如 image/png, video/mp4
	RawExtra   map[string]any // 供应商透传字段（如水印、种子数等）
}

// GenerateMediaOutput 多结果统一输出
type GenerateMediaOutput struct {
	Items   []GeneratedAssetItem
	RawBody string // 供应商原始响应体（用于日志/排错）
	TraceID string // 供应商请求 ID（对接客服用）
}

// VendorModelInfo 模型信息
type VendorModelInfo struct {
	ID          string            // 模型 ID（请求用，LibTV 即模板 UUID）
	Name        string            // 显示名（中文）
	Capability  string            // image / video / text / audio
	DefaultFor  string            // 可选：建议作为 imageModel / videoModel / textModel / audioModel 默认值
	Supports    map[string]bool   // 能力开关：如 { "refVideo":true, "genAudio":true }
	Constraints map[string]any    // 约束：如 { "maxSeconds":15, "sizes":["1024x1024"] }
	ModelLabels map[string]string // 别名映射（同 id 可覆盖显示）
	Extra       map[string]any    // 其他
}

// VendorModels 分组模型列表
type VendorModels struct {
	ImageModels []VendorModelInfo
	VideoModels []VendorModelInfo
	TextModels  []VendorModelInfo
	AudioModels []VendorModelInfo
}

// VendorAsset 供应商侧素材库条目
type VendorAsset struct {
	ID           string    `json:"id"`
	VendorType   string    `json:"vendorType"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"` // image / video / audio / project / scene / character
	ThumbnailURL string    `json:"thumbnailUrl"`
	SizeBytes    int64     `json:"sizeBytes"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	DurationMs   int       `json:"durationMs"`
	MimeType     string    `json:"mimeType"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	RawExtra     map[string]any `json:"rawExtra,omitempty"`
}

// AssetFilter 资产库筛选
type AssetFilter struct {
	Kind     string // image/video/audio，空=全部
	Keyword  string
	Tags     []string
	Page     int
	PageSize int
}

// ========== 生成入参 ==========

type ReferenceImageInput struct {
	URL        string  // 本项目可访问的 URL（如 /api/files/xxx 或供应商 CDN）
	StorageKey string  // 可选，避免重复下载
	Kind       string  // init / reference / mask / controlnet
	Weight     *float64
}

type GenerateImageInput struct {
	Prompt           string
	Model            string // LibTV 即模板 UUID
	Size             string // "1024x1024" / "1:1"（Adapter 内部折算）
	Count            int
	Quality          string // auto/low/medium/high
	NegativePrompt   string
	Seed             *int64
	ReferenceImages  []ReferenceImageInput
	Extra            map[string]any
}

type GenerateVideoInput struct {
	Prompt          string
	Model           string
	Seconds         int
	Size            string
	FPS             int
	NegativePrompt  string
	ReferenceImages []ReferenceImageInput
	ReferenceVideo  *ReferenceImageInput
	ReferenceAudio  *ReferenceImageInput
	GenerateAudio   bool
	Watermark       bool
	Seed            *int64
	Extra           map[string]any
}

type GenerateTextInput struct {
	Model       string
	SystemPrompt string
	Messages     []ChatMessage
	Temperature  *float64
	MaxTokens    *int
	Stream       bool
	Extra        map[string]any
}

type ChatMessage struct {
	Role    string
	Content string
}

type GenerateTextOutput struct {
	Text    string
	Chunks  <-chan string
	Usage   *TokenUsage
	RawBody string
	TraceID string
}

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostCredits      int
}

type GenerateAudioInput struct {
	Model          string
	Text           string
	Voice          string
	Format         string
	Speed          *float64
	Instruction    string
	ReferenceAudio *ReferenceImageInput
	Extra          map[string]any
}

// TaskStatus 异步任务统一状态
type TaskStatus struct {
	ID       string
	Status   string // queued / processing / completed / failed / canceled
	Progress int    // 0-100
	Message  string
	OutputURL string
	Output    *GenerateMediaOutput
	Extra     map[string]any
}

// ========== 账户 & 鉴权 ==========

// VerifyCredentialsParams 传入的登录凭证（Cookie / AK 组合）
type VerifyCredentialsParams struct {
	CookieString string
	AccessKey    string
	AccessSecret string
	AppKey       string
	VendorUserID string
}

// CredentialVerifyResult 校验通过后得到的基础账户信息
type CredentialVerifyResult struct {
	Valid        bool
	VendorUserID string
	DisplayName  string
	AvatarURL    string
	ExpiresAt    *time.Time
	BalanceInfo  map[string]any
	TraceID      string
}

// ========== 主接口 ==========

// VendorAdapter 每家供应商必须实现的全部能力（没用到的方法返回 ErrNotSupported 即可）
type VendorAdapter interface {
	// ── 账户 & 鉴权 ──
	BuildOAuthAuthorizeURL(ctx context.Context, vendor *model.Vendor, state string) (string, error)
	ExchangeOAuthCode(ctx context.Context, vendor *model.Vendor, code string, redirectURI string) (*model.UserVendorAccount, error)
	RefreshAccessToken(ctx context.Context, account *model.UserVendorAccount) error
	GetAccountInfo(ctx context.Context, account *model.UserVendorAccount) error
	VerifyLoginCredentials(ctx context.Context, params VerifyCredentialsParams) (*CredentialVerifyResult, error)

	// ── 模型 ──
	ListModels(ctx context.Context, account *model.UserVendorAccount) (*VendorModels, error)

	// ── 生成（核心） ──
	GenerateImage(ctx context.Context, account *model.UserVendorAccount, input GenerateImageInput) (*GenerateMediaOutput, error)
	GenerateVideo(ctx context.Context, account *model.UserVendorAccount, input GenerateVideoInput) (*GenerateMediaOutput, error)
	GenerateAudio(ctx context.Context, account *model.UserVendorAccount, input GenerateAudioInput) (*GenerateMediaOutput, error)
	GenerateText(ctx context.Context, account *model.UserVendorAccount, input GenerateTextInput) (*GenerateTextOutput, error)

	// ── 异步任务状态 ──
	GetTaskStatus(ctx context.Context, account *model.UserVendorAccount, taskID string) (*TaskStatus, error)
	CancelTask(ctx context.Context, account *model.UserVendorAccount, taskID string) error

	// ── 资产库 ──
	ListAssets(ctx context.Context, account *model.UserVendorAccount, filter AssetFilter) ([]VendorAsset, int, error)
	DownloadAsset(ctx context.Context, account *model.UserVendorAccount, assetID string) (reader io.ReadCloser, mimeType string, size int64, err error)
	UploadAsset(ctx context.Context, account *model.UserVendorAccount, name string, kind string, data io.Reader, size int64, mimeType string) (*VendorAsset, error)
	DeleteAsset(ctx context.Context, account *model.UserVendorAccount, assetID string) error
}

// EstimateCostInput 供应商额度估算入参
// 只有实现 VendorCostEstimator 的适配器才会被调用。
type EstimateCostInput struct {
	Capability    string // image / video / audio / text
	Model         string
	Quality       string // 供应商侧质量标识，例如 UpDream 的 "1K"/"2K"/"4K"
	Size          string // 前端尺寸/比例，供后端兜底推导 quality
	Count         int
	RefImageCount int
	RefVideoCount int
	HasSound      bool
}

// VendorCostEstimator 可选接口：实时估算单次请求扣除额度。
// 未实现该接口的供应商直接走前端静态 requestCreditCost 兜底。
type VendorCostEstimator interface {
	EstimateCost(ctx context.Context, account *model.UserVendorAccount, input EstimateCostInput) (float64, error)
}

// ========== 注册中心 ==========

var adapterRegistry = make(map[string]func(vendor *model.Vendor) VendorAdapter)

// RegisterVendorAdapter 各供应商适配器在 init() 里调用，注册工厂函数。新增一家供应商 = 新文件 + 一行 init()。
func RegisterVendorAdapter(vendorType string, factory func(vendor *model.Vendor) VendorAdapter) {
	adapterRegistry[strings.ToLower(vendorType)] = factory
}

// NewVendorAdapter 按供应商类型拿到适配器实例；未注册返回 (nil, false)。
func NewVendorAdapter(vendor *model.Vendor) (VendorAdapter, bool) {
	if vendor == nil {
		return nil, false
	}
	f, ok := adapterRegistry[strings.ToLower(vendor.Type)]
	if !ok {
		return nil, false
	}
	return f(vendor), true
}

// ========== 凭证解码辅助 ==========

// vendorAccountCredentials 从 UserVendorAccount 还原实际登录凭证。
// 约定（与 service.BindVendorByCookie 的存储方式一致）：
//   - Cookie 复用 AccessToken 字段（用户粘的网站 Cookie 字符串）
//   - AccessKey / AccessSecret / AppKey 存在 RawExtraJSON 的 access_key / access_secret / app_key
func vendorAccountCredentials(account *model.UserVendorAccount) VerifyCredentialsParams {
	creds := VerifyCredentialsParams{}
	if account == nil {
		return creds
	}
	// 解密 AccessToken（Cookie 或 custom_header value）
	if c := strings.TrimSpace(account.AccessToken); c != "" {
		if decrypted, err := DecryptCredential(c); err == nil && decrypted != "" {
			creds.CookieString = decrypted
		} else {
			// 旧数据未加密，回退原值
			creds.CookieString = c
		}
	}
	creds.VendorUserID = strings.TrimSpace(account.VendorUserID)
	if account.RawExtraJSON != "" {
		var extra map[string]string
		if json.Unmarshal([]byte(account.RawExtraJSON), &extra) == nil {
			// 解密 AK/SK/AppKey
			creds.AccessKey = decryptExtra(extra["access_key"])
			creds.AccessSecret = decryptExtra(extra["access_secret"])
			creds.AppKey = decryptExtra(extra["app_key"])
		}
	}
	return creds
}

// decryptExtra 解密 RawExtraJSON 中的加密字段；旧数据未加密时回退原值。
func decryptExtra(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if decrypted, err := DecryptCredential(v); err == nil && decrypted != "" {
		return decrypted
	}
	return v
}

// hasVendorAK 判断账户是否以 AccessKey 方式鉴权（优先于 Cookie）。
func hasVendorAK(creds VerifyCredentialsParams) bool {
	return creds.AccessKey != "" && creds.AccessSecret != ""
}

// ========== 模型列表过滤（跨供应商统一） ==========

// vendorModelHiddenPatterns 命中即不进入前端模型下拉：项目不需要的音频能力 + 非生成的编辑/后处理工具。
// 在 FetchAndStoreVendorModels 落库前统一调用 filterVendorModels，保证 libtv / updream / newwow 三家
// 下拉都只出现「可生成」的模型——没有多（编辑/超分/去字幕/抠像/运镜控制/导演类），也没有少。
// 匹配规则：对 model.ID + " " + model.Name 转小写后做子串包含判断（不区分大小写、不要求完整词）。
var vendorModelHiddenPatterns = []string{
	// 视频/图片后处理 & 编辑工具（非生图/生视频）
	"upscaler", "upscaling", // 超分：topaz-video-upscaler / volcano-video-upscaler / topaz-image-upscaler / hd-upscaling
	"subtitle-eraser", "subtitle eraser", // 去字幕
	"portrait-matting", "matting", // 抠像
	"motion-control", "motion control", // 运镜控制：kling-v3-motion-control / wanx-motion-control
	"image-editor", // 图片编辑器（非生图）
	// 导演 / 非纯生成类（用户确认不进下拉）
	"motion-3",         // motion-3.1 / motion-3-prime / motion-3-rapid / motion-3-lite
	"scene-2",          // scene-2 / scene-2-ultra
	"omnihuman",        // omnihuman-1.5 图生视频人像（归为导演类）
	"midjourney-video", // midjourney-video
	"kling-multi-shot", // 可灵多镜头导演
	// LibTV 编辑/增强/演化类（提交报错或非纯生成，用户确认不进下拉）
	"seed-evolving",   // 种子演化（code=10000 未知错误）
	"multiple-angles",  // 多角度生成（code=10000）
	"qwen-edit",        // Qwen 图片编辑（非纯生图）
	"video-enhance",    // 视频增强：kling-video-enhance-o1
	"seedream_hd4k",    // 超清4k（code=10000）
	// Bug #3：rolldek / apimart 上游 /v1/images/generations 都拒收 gemini 这两个 image preview 模型
	// （错误文案一字不差，提示 rolldek 透传到 apimart）。供应商接口把它们列进 imageModels，
	// 但提交必败。落库前统一剔除，前端既看不到也不会默认选中。
	"gemini-3-pro-image-preview",
	"gemini-3.1-flash-image-preview",
}

// filterVendorModels 移除项目不需要的模型：整体去掉音频能力，并剔除非生成的编辑/后处理工具。
// 任何供应商的 ListModels 结果在写入 AvailableModelsJSON 前都过一遍，保证下拉纯净。
func filterVendorModels(models *VendorModels) *VendorModels {
	if models == nil {
		return models
	}
	// 项目统一不需要音频模型
	models.AudioModels = nil
	models.ImageModels = filterHiddenModels(models.ImageModels)
	models.VideoModels = filterHiddenModels(models.VideoModels)
	models.TextModels = filterHiddenModels(models.TextModels)
	return models
}

func filterHiddenModels(list []VendorModelInfo) []VendorModelInfo {
	out := make([]VendorModelInfo, 0, len(list))
	for _, m := range list {
		if isVendorModelHidden(m.ID, m.Name) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func isVendorModelHidden(id, name string) bool {
	hay := strings.ToLower(id + " " + name)
	for _, p := range vendorModelHiddenPatterns {
		if p != "" && strings.Contains(hay, p) {
			return true
		}
	}
	return false
}
