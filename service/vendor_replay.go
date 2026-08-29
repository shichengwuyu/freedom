package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
)

// ============ 通用「样本重放」适配器基类 ============
//
// 适用对象：UpDream / NewWow 这类没有开放平台、后端无法预先知道内部接口形状的供应商。
// 方案（与文档 §4.1.5 对齐）：
//   1. 用户在官网用浏览器插件真实生成一次，插件把这次请求的 URL / 方法 / 头 / 体 + 响应抓回后端落库（capture-sample）。
//   2. 后端 GenerateImage 时读取该用户最新的「生成类」样本，把本次 prompt 注入请求体，
//      并用该用户绑定账户的 Cookie（AccessToken）覆盖请求头里的 Cookie，经 SSRF 安全客户端重放。
//   3. 从响应体里尽量宽松地提取图片直链，映射成 OpenAI 兼容的 {url} 返回。
//
// 局限（P1 范围，已在 pending-test 登记）：
//   - 仅支持同步返回图片地址的供应商；异步（先返回任务 ID 再轮询）暂不支持，会给出清晰中文提示。
//   - 仅支持 JSON / 表单（application/x-www-form-urlencoded）请求体；multipart（图生图上传）暂不支持。
//   - 注入 prompt 依赖样本请求体里存在可识别的 prompt 字段（prompt / text / input / content 等）；
//     找不到会报错，引导用户重新采集一次标准生图请求。
//   - 重放依赖样本请求头里的非 Cookie 头（如 CSRF Token、自定义签名头）；若用户改绑后这些头与 Cookie 不匹配，
//     需重新采集样本（采集应在最新登录后立刻进行）。

// replayVendorAdapter 被 UpDream / NewWow 两个注册文件复用，完整实现 VendorAdapter 接口。
type replayVendorAdapter struct {
	vendor      *model.Vendor
	vendorType  string
	displayName string
	client      *http.Client
}

// replaySampleTimeout 单条样本重放的独立超时：外层 ctx（视频/图片整链路）可能长达 5min，
// 但一条挂死的上游不能把后续 4 条样本的回退机会都吃掉。
const replaySampleTimeout = 45 * time.Second

func newReplayAdapter(v *model.Vendor, vendorType, displayName string) *replayVendorAdapter {
	return &replayVendorAdapter{
		vendor:      v,
		vendorType:  vendorType,
		displayName: displayName,
		client:      &http.Client{Timeout: 5 * time.Minute},
	}
}

// ========== 账户 & 鉴权 ==========

// BuildOAuthAuthorizeURL 走浏览器插件 Cookie 绑定，不需要 OAuth 授权页。
func (a *replayVendorAdapter) BuildOAuthAuthorizeURL(_ context.Context, _ *model.Vendor, _ string) (string, error) {
	return "", ErrNotSupported
}

func (a *replayVendorAdapter) ExchangeOAuthCode(_ context.Context, _ *model.Vendor, _ string, _ string) (*model.UserVendorAccount, error) {
	return nil, ErrNotSupported
}

// RefreshAccessToken Cookie 类账户无 RefreshToken，直接把过期时间推到远期，避免反复触发刷新。
func (a *replayVendorAdapter) RefreshAccessToken(_ context.Context, account *model.UserVendorAccount) error {
	if account == nil {
		return errors.New("账户为空")
	}
	expire := time.Now().Add(365 * 24 * time.Hour)
	account.TokenExpiresAt = &expire
	return nil
}

// GetAccountInfo 无开放平台账户读取端点，只确保已有字段不丢，不覆盖昵称。
func (a *replayVendorAdapter) GetAccountInfo(_ context.Context, account *model.UserVendorAccount) error {
	if account == nil {
		return errors.New("账户为空")
	}
	return nil
}

// VerifyLoginCredentials Cookie 类供应商没有干净的校验端点：Cookie 非空即 best-effort 视为有效，
// 真正有效性由首次生图重放结果兜底。
func (a *replayVendorAdapter) VerifyLoginCredentials(_ context.Context, params VerifyCredentialsParams) (*CredentialVerifyResult, error) {
	if strings.TrimSpace(params.CookieString) == "" {
		return nil, fmt.Errorf("%s 需要提供官网 Cookie 才能使用，请用浏览器插件粘贴登录后的 Cookie", a.displayName)
	}
	return &CredentialVerifyResult{
		Valid:        true,
		VendorUserID: params.VendorUserID,
		DisplayName:  params.VendorUserID,
	}, nil
}

// ========== 模型 ==========

// ListModels 这类供应商的「模型」本质是样本里捕获的接口，重放时不依赖具体模型 id，
// 因此返回一个占位模型，让前端 HasModels=true、模型下拉可用即可。
func (a *replayVendorAdapter) ListModels(_ context.Context, _ *model.UserVendorAccount) (*VendorModels, error) {
	return &VendorModels{
		ImageModels: []VendorModelInfo{
			{
				ID:          "auto",
				Name:        a.displayName + " 采集样本重放（自动）",
				Capability:  "image",
				DefaultFor:  "imageModel",
				Supports:    map[string]bool{"refImage": false},
				Constraints: map[string]any{"sizes": []any{"1024x1024"}, "maxCount": 1},
				Extra:       map[string]any{"replay": true},
			},
		},
	}, nil
}

// ========== 生成（核心） ==========

// GenerateImage 读取该用户最近的生成类样本，逐个尝试注入 prompt 并带账户鉴权凭证（Cookie / custom_header）重放，直到拿到图片。
// 取多条样本是为了兜底：最新一条可能被误采成非标准生图接口（如分页/详情），失败时自动回退到更早的样本。
func (a *replayVendorAdapter) GenerateImage(ctx context.Context, account *model.UserVendorAccount, input GenerateImageInput) (*GenerateMediaOutput, error) {
	creds := vendorAccountCredentials(account)
	if strings.TrimSpace(creds.CookieString) == "" {
		return nil, fmt.Errorf("%s 账户未绑定鉴权凭证，请在「云端供应商」里按对应方式绑定后再试", a.displayName)
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return nil, errors.New("缺少 prompt 参数")
	}

	// 1. 取该用户最近的「生成类」样本（最多 5 条），逐个尝试重放
	const maxReplaySamples = 5
	samples, err := ListVendorApiSamples(account.UserID, a.vendorType, true, maxReplaySamples)
	if err != nil {
		return nil, fmt.Errorf("读取采集样本失败: %w", err)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("%s 还没有可用的生成样本：请先在官网用插件采集一次真实生图请求（见插件「生成样本嗅探」）", a.displayName)
	}

	var lastErr error
	for _, sample := range samples {
		output, err := a.replaySample(ctx, account, sample, input)
		if err == nil {
			return output, nil
		}
		lastErr = err
	}
	if len(samples) == 1 {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%s 最近 %d 条生成样本均重放失败，最后错误：%v；建议重新登录官网采集一次最新的标准生图请求", a.displayName, len(samples), lastErr)
}

// replaySample 用单条样本执行一次重放：注入 prompt → 覆盖鉴权（cookie / custom_header 由 vendor.AuthMode 决定）→ 发起请求 → 提取图片 URL。
func (a *replayVendorAdapter) replaySample(ctx context.Context, account *model.UserVendorAccount, sample model.VendorApiSample, input GenerateImageInput) (*GenerateMediaOutput, error) {
	// 单样本重放独立超时：外层 ctx 可能高达 5min，但一条坏样本不应拖完后续全部回退机会
	sampleCtx, cancel := context.WithTimeout(ctx, replaySampleTimeout)
	defer cancel()

	creds := vendorAccountCredentials(account)


	// 2. 解析样本请求并套用本次 prompt
	replayURL, err := normalizeReplayURL(sample.URL, sample.RequestHeadersJSON)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(sample.Method))
	if method == "" {
		method = http.MethodPost
	}
	capturedHeaders, err := parseHeadersJSON(sample.RequestHeadersJSON)
	if err != nil {
		return nil, fmt.Errorf("解析样本请求头失败: %w", err)
	}
	bodyBytes, contentType, err := buildReplayBody(sample.ContentType, sample.RequestBody, input)
	if err != nil {
		return nil, err
	}

	// 3. 应用样本 header（skip hop-by-hop）+ 最后覆盖鉴权凭证（Cookie 或 custom_header 由 vendor.AuthMode 决定）
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(sampleCtx, method, replayURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("构造重放请求失败: %w", err)
	}
	applyReplayHeaders(req, capturedHeaders, contentType)
	// 鉴权最后注入（覆盖 captured 里可能带的同名头，例如 NewWow 的 accesstoken）
	if a.vendor != nil {
		applyVendorAuth(req, a.vendor, account)
	} else {
		if c := strings.TrimSpace(creds.CookieString); c != "" {
			req.Header.Set("Cookie", c)
		}
	}

	resp, err := SafeProxyHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s 重放请求失败（网络/SSRF 拦截）: %w", a.displayName, err)
	}
	defer resp.Body.Close()
	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 响应失败: %w", a.displayName, err)
	}
	raw := string(rawBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s 接口返回 HTTP %d：%s", a.displayName, resp.StatusCode, truncate(raw, 512))
	}

	// 4. 从响应体提取图片 URL
	items := extractReplayImages(raw)
	if len(items) == 0 {
		return nil, fmt.Errorf("%s 响应中未解析到图片地址；该供应商可能采用异步生成（需轮询），当前样本重放仅支持同步返回图片直链。请确认采集的样本响应包含图片 URL", a.displayName)
	}
	return &GenerateMediaOutput{Items: items, RawBody: raw, TraceID: sample.ID}, nil
}

// ========== P1 范围外能力 ==========

func (a *replayVendorAdapter) GenerateVideo(_ context.Context, _ *model.UserVendorAccount, _ GenerateVideoInput) (*GenerateMediaOutput, error) {
	return nil, fmt.Errorf("%s 暂仅支持图片生图（样本重放），视频生成将在后续接入", a.displayName)
}

func (a *replayVendorAdapter) GenerateAudio(_ context.Context, _ *model.UserVendorAccount, _ GenerateAudioInput) (*GenerateMediaOutput, error) {
	return nil, ErrNotSupported
}

func (a *replayVendorAdapter) GenerateText(_ context.Context, _ *model.UserVendorAccount, _ GenerateTextInput) (*GenerateTextOutput, error) {
	return nil, ErrNotSupported
}

func (a *replayVendorAdapter) GetTaskStatus(_ context.Context, _ *model.UserVendorAccount, _ string) (*TaskStatus, error) {
	return nil, ErrNotSupported
}

func (a *replayVendorAdapter) CancelTask(_ context.Context, _ *model.UserVendorAccount, _ string) error {
	return ErrNotSupported
}

func (a *replayVendorAdapter) ListAssets(_ context.Context, _ *model.UserVendorAccount, _ AssetFilter) ([]VendorAsset, int, error) {
	return nil, 0, ErrNotSupported
}

func (a *replayVendorAdapter) DownloadAsset(_ context.Context, _ *model.UserVendorAccount, _ string) (io.ReadCloser, string, int64, error) {
	return nil, "", 0, ErrNotSupported
}

func (a *replayVendorAdapter) UploadAsset(_ context.Context, _ *model.UserVendorAccount, _ string, _ string, _ io.Reader, _ int64, _ string) (*VendorAsset, error) {
	return nil, ErrNotSupported
}

func (a *replayVendorAdapter) DeleteAsset(_ context.Context, _ *model.UserVendorAccount, _ string) error {
	return ErrNotSupported
}

// ========== 重放辅助 ==========

// prompt / 负向 prompt 字段候选（按优先级匹配，命中第一个即可注入）
var replayPromptKeys = []string{
	"prompt", "prompt_en", "prompt_zh", "promptText", "prompt_text",
	"text", "input", "content", "description", "desc",
	"gen_prompt", "positive_prompt", "t2i_prompt", "image_prompt",
	"word", "caption", "query", "msg", "title",
}

var replayNegPromptKeys = []string{"negative_prompt", "negativePrompt", "neg_prompt", "negative"}

// buildReplayBody 把本次 prompt 注入到样本请求体，返回新字节与重放用的 Content-Type。
// 支持 JSON 与 application/x-www-form-urlencoded；其他类型（multipart 等）返回明确错误。
func buildReplayBody(contentType, capturedBody string, input GenerateImageInput) ([]byte, string, error) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	body := strings.TrimSpace(capturedBody)
	if body == "" {
		return []byte{}, "", nil
	}

	if strings.HasPrefix(ct, "application/json") || strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
		var root any
		if err := json.Unmarshal([]byte(body), &root); err != nil {
			return nil, "", fmt.Errorf("样本请求体不是合法 JSON，无法注入 prompt：%w", err)
		}
		if !injectPromptValue(root, input.Prompt) {
			return nil, "", errors.New("样本中未找到可识别的 prompt 字段（prompt / text / input / content 等），无法重放；请重新采集一次标准 JSON 生图请求")
		}
		if input.NegativePrompt != "" {
			injectNegPromptValue(root, input.NegativePrompt)
		}
		b, err := json.Marshal(root)
		if err != nil {
			return nil, "", fmt.Errorf("重新序列化请求体失败: %w", err)
		}
		return b, "application/json", nil
	}

	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		vals, err := url.ParseQuery(body)
		if err != nil {
			return nil, "", fmt.Errorf("样本表单请求体解析失败: %w", err)
		}
		found := false
		for _, key := range replayPromptKeys {
			if _, ok := vals[key]; ok {
				vals.Set(key, input.Prompt)
				found = true
				break
			}
		}
		if !found {
			return nil, "", errors.New("样本中未找到可识别的 prompt 表单字段，无法重放；请重新采集一次标准生图请求")
		}
		if input.NegativePrompt != "" {
			for _, key := range replayNegPromptKeys {
				if _, ok := vals[key]; ok {
					vals.Set(key, input.NegativePrompt)
					break
				}
			}
		}
		return []byte(vals.Encode()), "application/x-www-form-urlencoded", nil
	}

	return nil, "", errors.New("样本请求体类型暂不支持（仅支持 JSON / 表单），请重新采集一次标准生图请求")
}

// injectPromptValue 深度优先把第一个命中的 prompt 候选键的字符串值替换为新 prompt；成功返回 true。
func injectPromptValue(node any, prompt string) bool {
	switch v := node.(type) {
	case map[string]any:
		for _, key := range replayPromptKeys {
			if cur, ok := v[key]; ok {
				if _, isStr := cur.(string); isStr {
					v[key] = prompt
					return true
				}
			}
		}
		for _, child := range v {
			if injectPromptValue(child, prompt) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if injectPromptValue(item, prompt) {
				return true
			}
		}
	}
	return false
}

func injectNegPromptValue(node any, neg string) {
	switch v := node.(type) {
	case map[string]any:
		for _, key := range replayNegPromptKeys {
			if _, ok := v[key]; ok {
				if _, isStr := v[key].(string); isStr {
					v[key] = neg
					return
				}
			}
		}
		for _, child := range v {
			injectNegPromptValue(child, neg)
		}
	case []any:
		for _, item := range v {
			injectNegPromptValue(item, neg)
		}
	}
}

// parseHeadersJSON 把样本存的请求头 JSON（map[string]string）解析回来；空/非法返回空 map。
func parseHeadersJSON(jsonStr string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(jsonStr) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// normalizeReplayURL 把样本里可能相对的请求 URL 解析成绝对 URL：
// 若没有 scheme，则用捕获的请求头 Referer / Origin 推导出 origin 拼接。
func normalizeReplayURL(rawURL, headersJSON string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("样本请求 URL 为空，无法重放")
	}
	if u, err := url.Parse(rawURL); err == nil && u.Scheme != "" {
		return rawURL, nil
	}
	// 相对路径：从 Referer / Origin 拿 origin
	headers, _ := parseHeadersJSON(headersJSON)
	origin := ""
	if ref := headers["Referer"]; ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Scheme != "" {
			origin = u.Scheme + "://" + u.Host
		}
	}
	if origin == "" {
		if og := headers["Origin"]; og != "" {
			origin = og
		}
	}
	if origin == "" {
		return "", errors.New("样本请求 URL 为相对路径且无法从请求头推导域名，请重新采集一次生图请求")
	}
	base, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("推导请求域名失败: %w", err)
	}
	resolved := base.ResolveReference(&url.URL{Path: rawURL})
	return resolved.String(), nil
}

// applyReplayHeaders 设置重放请求头：保留样本里的非 hop-by-hop 头（含 CSRF / 自定义签名头 / UA / Referer）。
// 鉴权类头（Cookie / accesstoken / Authorization 等）不做 skip，由调用方在 replaySample 里用 applyVendorAuth 强制覆盖为账户最新凭证。
func applyReplayHeaders(req *http.Request, captured map[string]string, contentType string) {
	for k, v := range captured {
		lk := strings.ToLower(k)
		switch lk {
		case "host", "content-length", "connection", "accept-encoding", "transfer-encoding":
			continue
		}
		req.Header.Set(k, v)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
}

// ========== 响应图片提取 ==========

// 已知图片 URL 字段（命中这些 key 且其值为字符串/字符串数组时收集）
var replayImageKeys = map[string]bool{
	"imageurl": true, "image_url": true, "url": true, "imgurl": true, "img_url": true,
	"src": true, "ossurl": true, "oss_url": true, "cdnurl": true, "cdn_url": true,
	"resulturl": true, "result_url": true, "picurl": true, "pic_url": true,
	"imageurls": true, "imgurls": true, "urls": true, "imagelist": true, "img_list": true,
	"urllist": true, "piclist": true,
}

// extractReplayImages 尽量宽松地从响应体提取图片 URL，返回去重后的资产项。
func extractReplayImages(raw string) []GeneratedAssetItem {
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err == nil {
		collected := collectImageURLsByKeys(root)
		if len(collected) == 0 {
			collected = collectImageURLsScan(root)
		}
		if items := buildImageItems(collected); len(items) > 0 {
			return items
		}
	}
	// 兜底：正则扫描原文里的图片 URL
	var urls []string
	for _, m := range imageURLRegex.FindAllString(raw, -1) {
		if looksLikeImageURL(m) {
			urls = append(urls, m)
		}
	}
	return buildImageItems(urls)
}

var imageURLRegex = regexp.MustCompile(`https?://[^\s"'\),\}]+`)

func collectImageURLsByKeys(node any) []string {
	var out []string
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			if replayImageKeys[strings.ToLower(k)] {
				out = append(out, extractStringsFromNode(val)...)
			} else {
				out = append(out, collectImageURLsByKeys(val)...)
			}
		}
	case []any:
		for _, item := range v {
			out = append(out, collectImageURLsByKeys(item)...)
		}
	}
	return out
}

func collectImageURLsScan(node any) []string {
	var out []string
	switch v := node.(type) {
	case map[string]any:
		for _, val := range v {
			out = append(out, collectImageURLsScan(val)...)
		}
	case []any:
		for _, item := range v {
			out = append(out, collectImageURLsScan(item)...)
		}
	case string:
		if looksLikeImageURL(v) {
			out = append(out, v)
		}
	}
	return out
}

func extractStringsFromNode(val any) []string {
	var out []string
	switch t := val.(type) {
	case string:
		if looksLikeImageURL(t) {
			out = append(out, t)
		}
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok && looksLikeImageURL(s) {
				out = append(out, s)
			}
		}
	}
	return out
}

func buildImageItems(urls []string) []GeneratedAssetItem {
	seen := map[string]bool{}
	items := make([]GeneratedAssetItem, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimRight(u, `"'`) // 去掉 JSON 字符串里可能残留的引号
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		items = append(items, GeneratedAssetItem{ID: u, URL: u, MimeType: guessImageMime(u)})
	}
	return items
}

func guessImageMime(u string) string {
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, ".png"):
		return "image/png"
	case strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg"):
		return "image/jpeg"
	case strings.Contains(lower, ".webp"):
		return "image/webp"
	case strings.Contains(lower, ".gif"):
		return "image/gif"
	case strings.Contains(lower, ".avif"):
		return "image/avif"
	case strings.Contains(lower, ".bmp"):
		return "image/bmp"
	default:
		return "image/png"
	}
}

// looksLikeImageURL 粗略判断一个字符串是否像图片直链：必须是 http(s)，且带图片扩展名或含图片相关关键字。
func looksLikeImageURL(s string) bool {
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return false
	}
	lower := strings.ToLower(s)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".avif"} {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	for _, hint := range []string{"/image", "/img", "cdn", "oss", "pic", "photo", "asset"} {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}
