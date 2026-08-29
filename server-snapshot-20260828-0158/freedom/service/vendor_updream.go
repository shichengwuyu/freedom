package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// ============ UpDream 云端平台适配器 ============
//
// 鉴权事实（2026-08-18 更新）：
//   - 主站 API: https://www.updream.cn
//   - 鉴权只需 Authorization: Bearer <JWT>，不需要 Cookie
//   - AuthMode=custom_header，AuthHeaderName=Authorization
//   - JWT 是浏览器登录后由前端 SPA 缓存的会话 token，从 DevTools Request Headers 的 Authorization 字段复制
//
// 已嗅到的可调通端点（截至 2026-08-16）：
//   - GET /api/notifications/unread-count      → 200（鉴权连通性快速探针）
//   - GET /api/user-settings                   → 401（强鉴权，需要完整 Chrome session）
//   - GET /api/skills/community                → 200（公开，无需鉴权）—— 但返回的是 skills/plugins 列表，不是生图模型
//
// 当前能力（P1 阶段，已落地）：
//   - ListModels       覆盖 replayVendorAdapter 占位：image + video 两组都返回"UpDream 自动（样本重放）"，
//                      前端两个下拉分组都非空，标注真实模型端点尚未接入。
//   - fetchBalance     best-effort 探活 /api/user-settings → 失败回退到 /api/notifications/unread-count 鉴权连通性，
//                      失败错误统一写到 RawExtraJSON["updream_last_balance_error"]，前端 UI 能看到"接口尚未接入"的明确提示。
//   - GenerateImage    复用 replayVendorAdapter 的样本重放链路（applyVendorAuth 注入 Authorization Bearer token）。

const updreamDefaultAPIRoot = "https://www.updream.cn"

// ============== 适配器注册 ==============

func init() {
	RegisterVendorAdapter(model.VendorTypeUpDream, func(v *model.Vendor) VendorAdapter {
		return newUpDreamAdapter(v)
	})
}

// upDreamAdapter 在 replayVendorAdapter 之上覆盖「需要真实 UpDream 鉴权上下文」的能力。
// 生图链路完全复用嵌入的 replayVendorAdapter。
type upDreamAdapter struct {
	*replayVendorAdapter
}

func newUpDreamAdapter(v *model.Vendor) *upDreamAdapter {
	return &upDreamAdapter{
		replayVendorAdapter: newReplayAdapter(v, model.VendorTypeUpDream, "UpDream"),
	}
}

// ============== ListModels ==============

// ListModels 调 UpDream 真实模型列表端点（2026-08-17 走代理嗅探确认）：
//   - GET /api/ai/models         → 综合（含 flux2.0pro / grok-image1.0 / seedream-5.0-pro 等）
//   - GET /api/ai/video-models   → Seedance 2.0（支持 t2v / i2v / ref2v / video_edit）
//   - GET /api/ai/text-models    → gemini-3.1-pro / gpt-5.5 / deepseek-v3.2 / qwen3-max / kimi-k2.5 / MiniMax-M2.5
//   - GET /api/ai/audio-models   → speech-2.8-hd / speech-2.8-turbo + system_voices
//   - GET /api/ai/music-models   → music-2.6 / music-2.6-free / music-cover / music-cover-free
// 任何端点失败 → 占位 + 写 RawExtraJSON["updream_models_raw"] 方便排查。
// 鉴权需要走代理（UPDREAM_PROXY 环境变量）：UpDream CDN 对直连请求返回 404 假象。
func (a *upDreamAdapter) ListModels(ctx context.Context, account *model.UserVendorAccount) (*VendorModels, error) {
	placeholder := placeholderUpDreamModels()

	// 没绑 / 没鉴权 → 占位
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return placeholder, nil
	}

	out := &VendorModels{}
	rawSummary := map[string]any{}

	// 5 个端点并发拉（简化：串行；接口 6s 超时，全部加起来 < 30s 还在容忍范围）
	type slot struct {
		capability string
		path       string
		defaultFor string
	}
	slots := []slot{
		{"image", "/api/ai/models", "imageModel"},
		{"video", "/api/ai/video-models", "videoModel"},
		{"text", "/api/ai/text-models", "textModel"},
		{"audio", "/api/ai/audio-models", "audioModel"},
		{"audio", "/api/ai/music-models", ""}, // music 也归到 audio capability
	}
	for _, s := range slots {
		body, status, err := updreamHTTPGetRaw(ctx, a.vendor, account, s.path)
		if err != nil || status != 200 || strings.TrimSpace(body) == "" {
			rawSummary[s.path] = map[string]any{"status": status, "err": fmt.Sprintf("%v", err), "body": truncate(body, 100)}
			continue
		}
		models := parseUpDreamModelsBody(body, s.capability)
		rawSummary[s.path] = map[string]any{"status": status, "count": len(models)}
		switch s.capability {
		case "image":
			for i, m := range models {
				if i == 0 && s.defaultFor != "" {
					m.DefaultFor = s.defaultFor
					models[i] = m
				}
			}
			out.ImageModels = append(out.ImageModels, models...)
		case "video":
			for i, m := range models {
				if i == 0 && s.defaultFor != "" {
					m.DefaultFor = s.defaultFor
					models[i] = m
				}
			}
			out.VideoModels = append(out.VideoModels, models...)
		case "text":
			for i, m := range models {
				if i == 0 && s.defaultFor != "" {
					m.DefaultFor = s.defaultFor
					models[i] = m
				}
			}
			out.TextModels = append(out.TextModels, models...)
		case "audio":
			for i, m := range models {
				if i == 0 && s.defaultFor != "" && len(out.AudioModels) == 0 {
					m.DefaultFor = s.defaultFor
					models[i] = m
				}
			}
			out.AudioModels = append(out.AudioModels, models...)
		}
	}

	recordUpDreamRaw(account, "updream_models_raw", rawSummary)

	total := len(out.ImageModels) + len(out.VideoModels) + len(out.TextModels) + len(out.AudioModels)
	if total == 0 {
		// 全部端点都失败（多半是代理未配 / token 失效）→ 占位
		return placeholder, nil
	}
	return out, nil
}

// parseUpDreamModelsBody 从 UpDream `/api/ai/*` 响应里抽 model 数组。
// 响应 envelope：{"user_tier":"...", "confirm_model_ids":["b-pro",...], "models":[{"value":"...", "label":"...", ...}]}
// capability 参数用于显式指定分类（避免靠 schema 推断）。
//
// 关键：confirm_model_ids 是 UpDream 告知"当前实际可用"的模型白名单。
// models 数组里会包含已下架/未上线/临时不可用的模型（如 flux2.0pro、grok-image1.0、qwen），
// 直接把 models 全量返回前端会导致用户选了不可用模型 → 报 MODEL_NOT_AVAILABLE。
// 有 confirm_model_ids 时严格过滤，只保留白名单内的模型。
//
// 实际字段名（2026-08-17 用户截图 `/api/ai/models` 确认）：
//   ratios / default_ratio / resolutions / default_resolution
//   min_images / max_images / max_reference_images
//   supports_image / supports_custom_size / custom_size / resolution_size_map
//   supports_audio / supports_emotion / supports_lyrics / supports_instrumental / supports_ref_video
//   durations / default_duration / generate_types / operations
//   audio_formats / default_format / min_tier / is_local / description
//
// 视频模型字段名（2026-08-17 用户截图 /api/ai/video-models 确认）：
//   aspect_ratios / default_aspect_ratio（不是 image 的 ratios / default_ratio）
//   sizes / default_size / bitrate_modes / default_bitrate_mode / fps_options / default_fps
//   operation_constraints（嵌套：每种 operation 的 images/videos/audios min/max + label）
//   output_formats / default_output_format / modes / default_mode
func parseUpDreamModelsBody(body string, capability string) []VendorModelInfo {
	var root struct {
		UserTier         string `json:"user_tier"`
		ConfirmModelIDs  []string `json:"confirm_model_ids"`
		Models           []struct {
			Value              string   `json:"value"`
			Label              string   `json:"label"`
			Description        string   `json:"description"`
			SupportsImage      bool     `json:"supports_image"`
			SupportsAudio      bool     `json:"supports_audio"`
			SupportsEmotion    bool     `json:"supports_emotion"`
			SupportsLyrics     bool     `json:"supports_lyrics"`
			SupportsInstrumental bool   `json:"supports_instrumental"`
			SupportsSound      bool     `json:"supports_sound"`
			SupportsRefVideo   bool     `json:"supports_ref_video"`
			SupportsCustomSize bool     `json:"supports_custom_size"`
			MaxReferenceImages int      `json:"max_reference_images"`
			MaxRefVideos       int      `json:"max_ref_videos"`
			MaxRefAudios       int      `json:"max_ref_audios"`
			MinImages          int      `json:"min_images"`
			MaxImages          int      `json:"max_images"`
			MaxVideos          int      `json:"max_videos"`
			MinTier            string   `json:"min_tier"`
			Ratios             []string `json:"ratios"`
			DefaultRatio       string   `json:"default_ratio"`
			AspectRatios       []string `json:"aspect_ratios"`
			DefaultAspectRatio string   `json:"default_aspect_ratio"`
			Resolutions        []string `json:"resolutions"`
			DefaultResolution  string   `json:"default_resolution"`
			Sizes              []string `json:"sizes"`
			DefaultSize        string   `json:"default_size"`
			Durations          []int    `json:"durations"`
			DefaultDuration    int      `json:"default_duration"`
			GenerateTypes      []string `json:"generate_types"`
			Operations         []string `json:"operations"`
			OperationConstraints map[string]map[string]any `json:"operation_constraints"`
			Modes              []string `json:"modes"`
			DefaultMode        string   `json:"default_mode"`
			BitrateModes       []string `json:"bitrate_modes"`
			DefaultBitrateMode string   `json:"default_bitrate_mode"`
			FpsOptions         []string `json:"fps_options"`
			DefaultFps         int      `json:"default_fps"`
			OutputFormats      []string `json:"output_formats"`
			DefaultOutputFormat string  `json:"default_output_format"`
			AudioFormats       []string `json:"audio_formats"`
			DefaultFormat      string   `json:"default_format"`
			SpeedRange         []float64 `json:"speed_range"`
			PitchRange         []float64 `json:"pitch_range"`
			VolRange           []float64 `json:"vol_range"`
			Emotions           []string `json:"emotions"`
			IsLocal            bool     `json:"is_local"`
			CustomSize         map[string]any `json:"custom_size"`
			ResolutionSizeMap  map[string]any `json:"resolution_size_map"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return nil
	}
	// confirm_model_ids 是 UpDream 告知前端"实际可用"的模型白名单。
	// models 数组包含所有模型（含已下架/未上线的），直接用会导致用户选了不可用模型报 MODEL_NOT_AVAILABLE。
	// 有 confirm_model_ids 时严格过滤；没有时（老版本响应/空）退回全量，避免误删。
	confirmSet := make(map[string]bool, len(root.ConfirmModelIDs))
	for _, id := range root.ConfirmModelIDs {
		confirmSet[strings.TrimSpace(id)] = true
	}
	out := make([]VendorModelInfo, 0, len(root.Models))
	for _, m := range root.Models {
		id := strings.TrimSpace(m.Value)
		if id == "" {
			continue
		}
		// 有白名单但当前模型不在白名单里 → 跳过
		if len(confirmSet) > 0 && !confirmSet[id] {
			continue
		}
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = id
		}
		supports := map[string]bool{}
		// 按 capability 选正确的比例字段（image 用 ratios, video 用 aspect_ratios）
		aspectRatios := m.Ratios
		defaultAspectRatio := m.DefaultRatio
		if capability == "video" {
			aspectRatios = m.AspectRatios
			defaultAspectRatio = m.DefaultAspectRatio
		}
		switch capability {
		case "image":
			supports["refImage"] = m.SupportsImage && m.MaxReferenceImages > 0
			supports["customSize"] = m.SupportsCustomSize && len(m.CustomSize) > 0
		case "video":
			supports["refImage"] = m.SupportsImage
			supports["refVideo"] = m.SupportsRefVideo && m.MaxRefVideos > 0
			supports["audio"] = m.SupportsAudio
			supports["sound"] = m.SupportsSound
		case "text":
			supports["image"] = m.SupportsImage
		case "audio":
			supports["emotion"] = m.SupportsEmotion
			supports["lyrics"] = m.SupportsLyrics
			supports["instrumental"] = m.SupportsInstrumental
		}
		out = append(out, VendorModelInfo{
			ID:          id,
			Name:        label,
			Capability:  capability,
			Supports:    supports,
			Constraints: map[string]any{
				"aspectRatios":        aspectRatios,
				"defaultAspectRatio":  defaultAspectRatio,
				"sizes":               m.Sizes,
				"defaultSize":         m.DefaultSize,
				"resolutions":         m.Resolutions,
				"defaultResolution":   m.DefaultResolution,
				"durations":           m.Durations,
				"defaultDuration":     m.DefaultDuration,
				"generateTypes":       m.GenerateTypes,
				"operations":          m.Operations,
				"operationConstraints": m.OperationConstraints,
				"modes":               m.Modes,
				"defaultMode":         m.DefaultMode,
				"bitrateModes":        m.BitrateModes,
				"defaultBitrateMode":  m.DefaultBitrateMode,
				"fpsOptions":          m.FpsOptions,
				"defaultFps":          m.DefaultFps,
				"outputFormats":       m.OutputFormats,
				"defaultOutputFormat": m.DefaultOutputFormat,
				"audioFormats":        m.AudioFormats,
				"defaultFormat":       m.DefaultFormat,
				"speedRange":          m.SpeedRange,
				"pitchRange":          m.PitchRange,
				"volRange":            m.VolRange,
				"emotions":            m.Emotions,
				"minTier":             m.MinTier,
				"minImages":           m.MinImages,
				"maxImages":           m.MaxImages,
				"maxVideos":           m.MaxVideos,
				"maxRefImages":        m.MaxReferenceImages,
				"maxRefVideos":        m.MaxRefVideos,
				"maxRefAudios":        m.MaxRefAudios,
				"customSize":          m.CustomSize,
				"resolutionSizeMap":   m.ResolutionSizeMap,
			},
			Extra: map[string]any{
				"source":      "updream-ai-models",
				"userTier":    root.UserTier,
				"description": m.Description,
				"isLocal":     m.IsLocal,
			},
		})
	}
	return out
}

// placeholderUpDreamModels ListModels 占位（嗅探失败 / 无鉴权时返回）。
func placeholderUpDreamModels() *VendorModels {
	mk := func(capability, defaultFor string) VendorModelInfo {
		return VendorModelInfo{
			ID:         "updream-replay",
			Name:       "UpDream 自动（样本重放）",
			Capability: capability,
			DefaultFor: defaultFor,
			Supports:   map[string]bool{"refImage": false},
			Constraints: map[string]any{
				"sizes":    []any{"1024x1024"},
				"maxCount": 1,
			},
			Extra: map[string]any{
				"replay":  true,
				"reason":  "UpDream /api/xv-config/advanced_video_edit_models 鉴权失败或响应为空；当前生成依赖浏览器插件采集的真实样本 + Authorization Bearer JWT 鉴权",
				"hintUrl": "https://www.updream.cn/",
			},
		}
	}
	return &VendorModels{
		ImageModels: []VendorModelInfo{mk("image", "imageModel")},
		VideoModels: []VendorModelInfo{mk("video", "")},
	}
}

// firstNonEmptyString 从 map 里按候选 key 找第一个非空字符串值。
func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// recordUpDreamRaw 把诊断信息写到 account.RawExtraJSON（key 已存在则覆盖；其它 key 保留）。
func recordUpDreamRaw(account *model.UserVendorAccount, key string, payload map[string]any) {
	if account == nil {
		return
	}
	extras := map[string]string{}
	if account.RawExtraJSON != "" {
		_ = json.Unmarshal([]byte(account.RawExtraJSON), &extras)
	}
	if b, err := json.Marshal(payload); err == nil {
		extras[key] = string(b)
		if buf, err := json.Marshal(extras); err == nil {
			account.RawExtraJSON = string(buf)
			_, _ = repository.SaveUserVendorAccount(*account)
		}
	}
}

// updreamHTTPGetRaw 调 UpDream 任意 GET 端点，自动注入 Authorization Bearer token。
func updreamHTTPGetRaw(ctx context.Context, vendor *model.Vendor, account *model.UserVendorAccount, path string) (string, int, error) {
	if account == nil {
		return "", 0, errors.New("account 为空")
	}
	base := updreamDefaultAPIRoot
	if vendor != nil {
		if r := strings.TrimSpace(vendor.APIRootURL); r != "" {
			base = strings.TrimRight(r, "/")
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, base+path, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 FreedomUpDreamAdapter/1.0")
	req.Header.Set("Referer", base+"/")
	req.Header.Set("Origin", base)
	v := &model.Vendor{Type: model.VendorTypeUpDream, AuthMode: model.VendorAuthModeCustomHeader, AuthHeaderName: "Authorization"}
	applyVendorAuth(req, v, account)
	client := updreamHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(raw), resp.StatusCode, nil
}

// updreamHTTPClient 构造 UpDream 请求客户端，按环境变量决定是否走代理。
//
// 为什么需要代理：UpDream CDN 在直连（非中国大陆 IP）请求时把所有非公开端点（/api/ai/*、/api/user/account 等）
// 返回 `{"detail":"Not Found"}` 假象；只有从浏览器经过本地代理（Clash 等）的请求才能拿到真接口。
// 部署服务器（149.88.78.8 香港节点）通过代理也能拿到，但直连会失败——这跟用户的浏览器现象一致。
//
// 代理配置读取顺序（按风险递增）：
//   1. 环境变量 UPDREAM_PROXY（精确控制，只影响 UpDream 适配器）
//   2. 环境变量 HTTPS_PROXY / HTTP_PROXY（全局，影响所有 outbound HTTP；不推荐用作 UpDream 单独代理）
//   3. 直连（兜底；如果用户/服务器本身就在 UpDream CDN 白名单内就 OK）
func updreamHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: nil, // 默认直连
		// 复用项目 ssrf.go 里的安全配置（如果有 SafeDialContext）；这里用默认 Dial
	}
	proxy := strings.TrimSpace(os.Getenv("UPDREAM_PROXY"))
	if proxy == "" {
		// 退到全局 env：golang http.Transport 自带 ProxyFromEnvironment
		transport.Proxy = http.ProxyFromEnvironment
	} else {
		if proxyURL, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{
		Timeout:   6 * time.Second,
		Transport: transport,
	}
}

// ============== 余额（best-effort 探活 + 失败可观察）==============

// ============== 真实生成端点（2026-08-17 走代理实测）==============
//
// 5 个能力端点（统一走 POST 提交 + GET 轮询 task_id）：
//   - POST /api/ai/generate-text/async   入参：{model_name, prompt}
//   - POST /api/ai/generate-image/async  入参：{model_name, prompt, ratio, resolution, [reference_images]}
//   - POST /api/ai/generate-video/async  入参：{model_name, prompt, duration, aspect_ratio, resolution, generate_type}
//   - POST /api/ai/generate-audio/async  入参：{model_name, text, voice, format}
//   - POST /api/ai/generate-music/async  入参：{model_name, prompt, lyrics?, instrumental?}
//   - GET  /api/ai/task/{task_id}        轮询，返回 {status: pending|processing|completed|failed, progress, result:{...}}
//
// 字段命名跟 OpenAI 不一样：模型字段是 `model_name`（不是 `model`），文本字段是 `prompt` 或 `text`。
// 用户积分 0 时（balance_credits=0）所有能力都会返回 402 INSUFFICIENT_CREDITS，不会进入任务队列。

// upDreamSubmitTask 通用提交：POST body + 解析 task_id + frozen_credits
// frozen_credits 是 UpDream 在提交成功时返回的本次冻结积分数（= 本次消耗），
// 被记录到 account.RawExtraJSON["updreamCostHistory"] 供 EstimateCost 读取。
func upDreamSubmitTask(ctx context.Context, vendor *model.Vendor, account *model.UserVendorAccount, path string, body map[string]any) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, baseURL(vendor)+path, strings.NewReader(mustJSON(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 FreedomUpDreamAdapter/1.0")
	req.Header.Set("Referer", baseURL(vendor)+"/")
	req.Header.Set("Origin", baseURL(vendor))
	v := &model.Vendor{Type: model.VendorTypeUpDream, AuthMode: model.VendorAuthModeCustomHeader, AuthHeaderName: "Authorization"}
	applyVendorAuth(req, v, account)
	resp, err := updreamHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("submit HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var parsed struct {
		TaskID         string  `json:"task_id"`
		FrozenCredits  float64 `json:"frozen_credits"`
		Data           struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &parsed)
	// 记录 frozen_credits 到 updreamCostHistory（按 model_name 索引）
	if parsed.FrozenCredits > 0 {
		modelName, _ := body["model_name"].(string)
		if modelName != "" {
			recordUpDreamCostHistory(account, modelName, int(parsed.FrozenCredits))
		}
	}
	taskID := strings.TrimSpace(parsed.TaskID)
	if taskID == "" {
		taskID = strings.TrimSpace(parsed.Data.TaskID)
	}
	if taskID == "" {
		return "", fmt.Errorf("submit 200 但无 task_id: %s", truncate(string(raw), 300))
	}
	return taskID, nil
}

// upDreamPollTask 通用轮询：返回 status / progress / result / error
type upDreamTaskSnapshot struct {
	Status    string          `json:"status"`     // pending / processing / completed / failed
	Progress  int             `json:"progress"`   // 0-100
	Result    json.RawMessage `json:"result"`     // 完成时的载荷（结构因能力而异）
	Error     string          `json:"error"`
	ErrorCode string          `json:"error_code"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func upDreamPollTask(ctx context.Context, vendor *model.Vendor, account *model.UserVendorAccount, taskID string) (*upDreamTaskSnapshot, error) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, baseURL(vendor)+"/api/ai/task/"+taskID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 FreedomUpDreamAdapter/1.0")
	v := &model.Vendor{Type: model.VendorTypeUpDream, AuthMode: model.VendorAuthModeCustomHeader, AuthHeaderName: "Authorization"}
	applyVendorAuth(req, v, account)
	resp, err := updreamHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("poll HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var snap upDreamTaskSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("parse poll response: %w body=%s", err, truncate(string(raw), 200))
	}
	return &snap, nil
}

// upDreamWaitForCompletion 循环 poll 直到 status=completed/failed 或超时。
// interval=3s 默认；maxWait 默认 5 分钟（视频生成长）。每次 poll 之间 sleep。
func upDreamWaitForCompletion(ctx context.Context, vendor *model.Vendor, account *model.UserVendorAccount, taskID string, maxWait time.Duration) (*upDreamTaskSnapshot, error) {
	deadline := time.Now().Add(maxWait)
	for {
		snap, err := upDreamPollTask(ctx, vendor, account, taskID)
		if err != nil {
			return nil, err
		}
		if snap.Status == "completed" || snap.Status == "failed" || snap.Status == "canceled" {
			return snap, nil
		}
		if time.Now().After(deadline) {
			return snap, fmt.Errorf("poll timeout after %v, last status=%s progress=%d", maxWait, snap.Status, snap.Progress)
		}
		select {
		case <-ctx.Done():
			return snap, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// baseURL 返回 vendor 配的 APIRootURL（默认 https://www.updream.cn）
func baseURL(vendor *model.Vendor) string {
	if vendor != nil {
		if r := strings.TrimSpace(vendor.APIRootURL); r != "" {
			return strings.TrimRight(r, "/")
		}
	}
	return updreamDefaultAPIRoot
}

// mustJSON 把 map 序列化成 JSON 字符串（用于 POST body）；失败返回 "{}" 让上游给 4xx 而不是 panic。
func mustJSON(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ============== GenerateText 实现 ==============

// GenerateText 调 /api/ai/generate-text/async 提交 + 轮询拿 result.text。
// 多轮对话通过 buildMultiTurnPrompt 把 messages 历史拼成单轮 prompt（"[用户]...\n\n[助手]...\n\n" 格式）。
// UpDream API 单轮接口的限制用降级方案绕开——大多数 LLM 能识别这种角色前缀。
func (a *upDreamAdapter) GenerateText(ctx context.Context, account *model.UserVendorAccount, input GenerateTextInput) (*GenerateTextOutput, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return nil, errors.New("UpDream 未绑定")
	}
	prompt := buildMultiTurnPrompt(input)
	if prompt == "" {
		return nil, errors.New("UpDream 文本生成缺少有效 prompt（messages 全部为空）")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "gemini-3.1-pro"
	}
	body := map[string]any{"model_name": model, "prompt": prompt}
	if t := input.Temperature; t != nil {
		body["temperature"] = *t
	}
	if input.MaxTokens != nil {
		body["max_tokens"] = *input.MaxTokens
	}
	taskID, err := upDreamSubmitTask(ctx, a.vendor, account, "/api/ai/generate-text/async", body)
	if err != nil {
		return nil, err
	}
	snap, err := upDreamWaitForCompletion(ctx, a.vendor, account, taskID, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	if snap.Status == "failed" {
		return nil, fmt.Errorf("UpDream 文本任务失败: code=%s msg=%s", snap.ErrorCode, snap.Error)
	}
	// result: {text: "..."}
	var textResult struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(snap.Result, &textResult)
	return &GenerateTextOutput{
		Text:    textResult.Text,
		RawBody: string(snap.Result),
		TraceID: taskID,
	}, nil
}

// GetTaskStatus 把 upDreamTaskSnapshot 适配成统一 TaskStatus 结构（供 handler 轮询链路复用）。
func upDreamConvertTaskStatus(taskID string, snap *upDreamTaskSnapshot) *TaskStatus {
	if snap == nil {
		return &TaskStatus{ID: taskID, Status: "failed", Message: "nil snapshot"}
	}
	status := strings.ToLower(strings.TrimSpace(snap.Status))
	out := &TaskStatus{
		ID:       taskID,
		Status:   status,
		Progress: snap.Progress,
		Message:  snap.Error,
		Extra: map[string]any{
			"error_code": snap.ErrorCode,
			"created_at": snap.CreatedAt,
			"updated_at": snap.UpdatedAt,
		},
	}
	if len(snap.Result) > 0 {
		out.OutputURL = extractFirstURL(snap.Result)
		out.Extra["result"] = string(snap.Result)
	}
	return out
}

// extractFirstURL 从 JSON 文本里挖第一个看起来像 URL 的字符串（用于 OutputURL 字段）。
func extractFirstURL(raw json.RawMessage) string {
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return ""
	}
	var found string
	var walk func(n any) bool
	walk = func(n any) bool {
		switch v := n.(type) {
		case string:
			if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
				found = v
				return true
			}
		case map[string]any:
			for _, k := range []string{"url", "image_url", "video_url", "audio_url", "oss_url", "src"} {
				if s, ok := v[k].(string); ok && (strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) {
					found = s
					return true
				}
			}
			for _, c := range v {
				if walk(c) {
					return true
				}
			}
		case []any:
			for _, c := range v {
				if walk(c) {
					return true
				}
			}
		}
		return false
	}
	walk(node)
	return found
}

// GetTaskStatus 实现 VendorAdapter 接口：调 /api/ai/task/{id} 拿状态，包装成统一 TaskStatus。
func (a *upDreamAdapter) GetTaskStatus(ctx context.Context, account *model.UserVendorAccount, taskID string) (*TaskStatus, error) {
	if account == nil || taskID == "" {
		return nil, errors.New("UpDream GetTaskStatus: account/taskID 为空")
	}
	snap, err := upDreamPollTask(ctx, a.vendor, account, taskID)
	if err != nil {
		return nil, err
	}
	return upDreamConvertTaskStatus(taskID, snap), nil
}

// CancelTask UpDream API 暂未确认有取消端点；保留占位，返回 ErrNotSupported。
func (a *upDreamAdapter) CancelTask(_ context.Context, _ *model.UserVendorAccount, _ string) error {
	return ErrNotSupported
}

// ============== GenerateMusic 实现（music 端点已存在，部分模型 MODEL_NOT_AVAILABLE）==============

// GenerateAudio 同时支持 speech（speech-*）和 music（music-*）两种模型，按 model 前缀自动选 endpoint。
// speech 走 /api/ai/generate-audio/async + {text, voice, format}
// music 走 /api/ai/generate-music/async + {prompt, [lyrics], [instrumental]}
func (a *upDreamAdapter) GenerateAudio(ctx context.Context, account *model.UserVendorAccount, input GenerateAudioInput) (*GenerateMediaOutput, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return nil, errors.New("UpDream 未绑定")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return nil, errors.New("UpDream 音频/音乐生成缺少 model_name")
	}

	// music-* 模型走 /api/ai/generate-music/async
	if strings.HasPrefix(model, "music-") {
		body := map[string]any{
			"model_name": model,
			// music API 接受 prompt（歌词或描述），text 也兼容
			"prompt": pickFirstNonEmpty(input.Text, input.Extra["prompt"], input.Extra["lyrics"]),
		}
		if lyrics := strings.TrimSpace(toString(input.Extra["lyrics"])); lyrics != "" && body["prompt"] == "" {
			body["prompt"] = lyrics
		}
		if instr := strings.TrimSpace(toString(input.Extra["instrumental"])); instr != "" {
			body["instrumental"] = instr == "true" || instr == "1"
		}
		if format := strings.TrimSpace(toString(input.Extra["format"])); format != "" {
			body["format"] = format
		}
		taskID, err := upDreamSubmitTask(ctx, a.vendor, account, "/api/ai/generate-music/async", body)
		if err != nil {
			return nil, err
		}
		snap, err := upDreamWaitForCompletion(ctx, a.vendor, account, taskID, 3*time.Minute)
		if err != nil {
			return nil, err
		}
		if snap.Status == "failed" {
			return nil, fmt.Errorf("UpDream 音乐任务失败: code=%s msg=%s", snap.ErrorCode, snap.Error)
		}
		return upDreamParseMediaResult(taskID, snap.Result, "audio")
	}

	// 其他（speech-*）走 /api/ai/generate-audio/async
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return nil, errors.New("UpDream 音频生成缺少 text 字段")
	}
	body := map[string]any{
		"model_name": model,
		"text":       text,
	}
	if voice := strings.TrimSpace(toString(input.Extra["voice"])); voice != "" {
		body["voice"] = voice
	}
	if format := strings.TrimSpace(toString(input.Extra["format"])); format != "" {
		body["format"] = format
	}
	taskID, err := upDreamSubmitTask(ctx, a.vendor, account, "/api/ai/generate-audio/async", body)
	if err != nil {
		return nil, err
	}
	snap, err := upDreamWaitForCompletion(ctx, a.vendor, account, taskID, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	if snap.Status == "failed" {
		return nil, fmt.Errorf("UpDream 音频任务失败: code=%s msg=%s", snap.ErrorCode, snap.Error)
	}
	return upDreamParseMediaResult(taskID, snap.Result, "audio")
}

// ============== SubmitVideo 异步视频接口（VendorVideoSubmitter）==============

// SubmitVideo 异步提交视频任务，立即返回 task_id；调用方通过 GetTaskStatus 轮询。
// 实现 service.VendorVideoSubmitter 接口（handler 类型断言探测）：
//   - 返回 (task_id string, error)
//   - 失败/不支持返回 ErrNotSupported 即可
// 与同步 GenerateVideo 的差异：GenerateVideo 内部阻塞轮询等结果（最多 6 分钟）；
// SubmitVideo 只提交不等待，handler 走异步 pollTask 链路（适合长任务）。
func (a *upDreamAdapter) SubmitVideo(ctx context.Context, account *model.UserVendorAccount, input GenerateVideoInput) (string, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return "", errors.New("UpDream 未绑定")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "sed2"
	}
	body := map[string]any{
		"model_name": model,
		"prompt":     strings.TrimSpace(input.Prompt),
	}
	if input.Seconds > 0 {
		body["duration"] = input.Seconds
	}
	if ar := pickFirstNonEmpty(input.Extra["aspect_ratio"], input.Size); ar != "" {
		body["aspect_ratio"] = ar
	}
	if r := strings.TrimSpace(toString(input.Extra["resolution"])); r != "" {
		body["resolution"] = r
	}
	// generate_type 自动判断：reference images > 0 走 i2v，否则 t2v
	if gt := strings.TrimSpace(toString(input.Extra["generate_type"])); gt != "" {
		body["generate_type"] = gt
	} else if len(input.ReferenceImages) > 0 {
		body["generate_type"] = "i2v"
	}
	if input.GenerateAudio {
		body["generate_audio"] = true
	}
	// reference_images 透传：拿 URL 列表（UpDream API 是否真支持待实测）
	if len(input.ReferenceImages) > 0 {
		urls := make([]string, 0, len(input.ReferenceImages))
		for _, ref := range input.ReferenceImages {
			if u := strings.TrimSpace(ref.URL); u != "" {
				urls = append(urls, u)
			}
		}
		if len(urls) > 0 {
			body["reference_images"] = urls
		}
	}
	if input.NegativePrompt != "" {
		body["negative_prompt"] = input.NegativePrompt
	}
	taskID, err := upDreamSubmitTask(ctx, a.vendor, account, "/api/ai/generate-video/async", body)
	if err != nil {
		return "", err
	}
	return taskID, nil
}

// ============== 多轮对话支持（messages 拼成单轮 prompt）==============

// buildMultiTurnPrompt 把 chat 历史的 role/content 拼成单轮 prompt 字符串。
// UpDream /api/ai/generate-text/async 只接受单轮 prompt；多轮对话降级为"用户/助手"前缀拼接，
// 模型一般能识别这种格式。SystemPrompt 作为顶层 system 消息。
func buildMultiTurnPrompt(input GenerateTextInput) string {
	var sb strings.Builder
	if sp := strings.TrimSpace(input.SystemPrompt); sp != "" {
		sb.WriteString("[系统提示]\n")
		sb.WriteString(sp)
		sb.WriteString("\n\n")
	}
	for _, m := range input.Messages {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		// 中文友好：role 翻译
		switch strings.ToLower(role) {
		case "system":
			sb.WriteString("[系统]\n")
		case "user":
			sb.WriteString("[用户]\n")
		case "assistant":
			sb.WriteString("[助手]\n")
		default:
			sb.WriteString("[")
			sb.WriteString(role)
			sb.WriteString("]\n")
		}
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// ============== GenerateImage 实现 ==============

// GenerateImage 调 /api/ai/generate-image/async 提交 + 轮询拿 result.images[].url。
// input.Model 必填（如 flux2.0pro / seedream-5.0-pro）；input.Size / input.Extra[ratio] / input.Extra[resolution] 透传。
// input.ReferenceImages 透传：拿 URL 列表放进 reference_images 字段（UpDream 是否真支持待实测；
//   当前嗅探未拿图生图任务响应，但 generate-image/async 接受 reference 字段概率较高）。
func (a *upDreamAdapter) GenerateImage(ctx context.Context, account *model.UserVendorAccount, input GenerateImageInput) (*GenerateMediaOutput, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return nil, errors.New("UpDream 未绑定")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "flux2.0pro"
	}
	body := map[string]any{
		"model_name": model,
		"prompt":     strings.TrimSpace(input.Prompt),
	}
	if ratio := pickFirstNonEmpty(input.Extra["ratio"], input.Size, input.Extra["size"]); ratio != "" {
		body["ratio"] = ratio
	}
	if res := strings.TrimSpace(toString(input.Extra["resolution"])); res != "" {
		body["resolution"] = res
	}
	if input.NegativePrompt != "" {
		body["negative_prompt"] = input.NegativePrompt
	}
	if input.Count > 0 {
		body["count"] = input.Count
	}
	// reference_images 透传（图生图场景）
	if len(input.ReferenceImages) > 0 {
		urls := make([]string, 0, len(input.ReferenceImages))
		for _, ref := range input.ReferenceImages {
			if u := strings.TrimSpace(ref.URL); u != "" {
				urls = append(urls, u)
			}
		}
		if len(urls) > 0 {
			body["reference_images"] = urls
		}
	}
	taskID, err := upDreamSubmitTask(ctx, a.vendor, account, "/api/ai/generate-image/async", body)
	if err != nil {
		return nil, err
	}
	snap, err := upDreamWaitForCompletion(ctx, a.vendor, account, taskID, 3*time.Minute)
	if err != nil {
		return nil, err
	}
	if snap.Status == "failed" {
		return nil, fmt.Errorf("UpDream 图片任务失败: code=%s msg=%s", snap.ErrorCode, snap.Error)
	}
	return upDreamParseMediaResult(taskID, snap.Result, "image")
}

// pickFirstNonEmpty 依次返回第一个非空字符串值
func pickFirstNonEmpty(vals ...any) string {
	for _, v := range vals {
		s := strings.TrimSpace(toString(v))
		if s != "" {
			return s
		}
	}
	return ""
}

// upDreamParseMediaResult 从 result JSON 抽 GenerateMediaOutput；按 capability 推断 MIME。
// result 实际结构嗅探未拿到（积分 0 没法实测），做宽容解析：递归找 URL 字符串。
func upDreamParseMediaResult(taskID string, result json.RawMessage, capability string) (*GenerateMediaOutput, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, fmt.Errorf("UpDream %s 任务 result 为空", capability)
	}
	url := extractFirstURL(result)
	if url == "" {
		return nil, fmt.Errorf("UpDream %s 任务 result 找不到 URL: %s", capability, truncate(string(result), 300))
	}
	mime := "image/png"
	switch capability {
	case "image":
		mime = "image/png"
	case "video":
		mime = "video/mp4"
	case "audio":
		mime = "audio/mpeg"
	}
	return &GenerateMediaOutput{
		Items: []GeneratedAssetItem{{
			ID:       url,
			URL:      url,
			MimeType: mime,
		}},
		RawBody: string(result),
		TraceID: taskID,
	}, nil
}

// ============== GenerateVideo 实现 ==============

// GenerateVideo 调 /api/ai/generate-video/async。duration / aspect_ratio / resolution / generate_type 透传。
// 视频生成通常 1-5 分钟，maxWait 设为 6 分钟。
func (a *upDreamAdapter) GenerateVideo(ctx context.Context, account *model.UserVendorAccount, input GenerateVideoInput) (*GenerateMediaOutput, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return nil, errors.New("UpDream 未绑定")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "sed2"
	}
	body := map[string]any{
		"model_name": model,
		"prompt":     strings.TrimSpace(input.Prompt),
	}
	if input.Seconds > 0 {
		body["duration"] = input.Seconds
	}
	if ar := pickFirstNonEmpty(input.Extra["aspect_ratio"], input.Size); ar != "" {
		body["aspect_ratio"] = ar
	}
	if r := strings.TrimSpace(toString(input.Extra["resolution"])); r != "" {
		body["resolution"] = r
	}
	if gt := strings.TrimSpace(toString(input.Extra["generate_type"])); gt != "" {
		body["generate_type"] = gt
	}
	if input.GenerateAudio {
		body["generate_audio"] = true
	}
	taskID, err := upDreamSubmitTask(ctx, a.vendor, account, "/api/ai/generate-video/async", body)
	if err != nil {
		return nil, err
	}
	snap, err := upDreamWaitForCompletion(ctx, a.vendor, account, taskID, 6*time.Minute)
	if err != nil {
		return nil, err
	}
	if snap.Status == "failed" {
		return nil, fmt.Errorf("UpDream 视频任务失败: code=%s msg=%s", snap.ErrorCode, snap.Error)
	}
	return upDreamParseMediaResult(taskID, snap.Result, "video")
}

// ============== 额度估算 ==============

// recordUpDreamCostHistory 把 frozen_credits 写到 account.RawExtraJSON["updreamCostHistory"][model]，
// 供 EstimateCost 下次读取。与 LibTV 的 recordSuccess/powerHistory 机制对称。
func recordUpDreamCostHistory(account *model.UserVendorAccount, model string, credits int) {
	if account == nil || model == "" || credits <= 0 {
		return
	}
	extras := map[string]any{}
	if account.RawExtraJSON != "" {
		_ = json.Unmarshal([]byte(account.RawExtraJSON), &extras)
	}
	history, _ := extras["updreamCostHistory"].(map[string]any)
	if history == nil {
		history = map[string]any{}
	}
	history[model] = map[string]any{
		"credits":   credits,
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	extras["updreamCostHistory"] = history
	if b, err := json.Marshal(extras); err == nil {
		account.RawExtraJSON = string(b)
		_, _ = repository.SaveUserVendorAccount(*account)
	}
}

// EstimateCost 从 updreamCostHistory 读上次该模型实际消耗的 frozen_credits。
// UpDream 的 /api/estimate 端点已废弃（返回 404），改为用提交任务时返回的 frozen_credits 做历史参考。
// 这意味着首次使用某模型时无法预估（返回 0 + error），跑过一次后才有值。
func (a *upDreamAdapter) EstimateCost(ctx context.Context, account *model.UserVendorAccount, input EstimateCostInput) (float64, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return 0, errors.New("UpDream 未绑定")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return 0, errors.New("缺少 model_name")
	}

	// 从 updreamCostHistory 读上次消耗
	extras := map[string]any{}
	if account.RawExtraJSON != "" {
		_ = json.Unmarshal([]byte(account.RawExtraJSON), &extras)
	}
	history, _ := extras["updreamCostHistory"].(map[string]any)
	if history != nil {
		if entry, ok := history[model].(map[string]any); ok {
			if credits, ok := entry["credits"].(float64); ok && credits > 0 {
				return credits, nil
			}
		}
	}
	return 0, fmt.Errorf("UpDream 模型 %s 暂无积分历史（首次使用后才会有预估值）", model)
}

// upDreamResolveQuality 把前端 quality/size 转成 UpDream estimate 接口认的 quality（如 1K/2K/4K）。
// 优先用 input.Quality；空的话按 size 推断；再空就取该模型快照里的 defaultResolution。
func upDreamResolveQuality(input EstimateCostInput, account *model.UserVendorAccount) string {
	quality := strings.TrimSpace(input.Quality)
	if quality != "" {
		if strings.HasSuffix(quality, "k") || strings.HasSuffix(quality, "K") {
			return strings.ToUpper(quality)
		}
	}

	size := strings.ToLower(strings.TrimSpace(input.Size))
	if strings.Contains(size, "4k") || strings.Contains(size, "3840") || strings.Contains(size, "2160") {
		return "4K"
	}
	if strings.Contains(size, "2k") || strings.Contains(size, "2048") || strings.Contains(size, "1152") {
		return "2K"
	}

	resolutions, defaultRes := upDreamModelResolutions(account, input.Model, input.Capability)
	if defaultRes != "" {
		return defaultRes
	}
	// 再没有就用前端 quality 做一层兜底映射
	if quality == "high" && containsString(resolutions, "4K") {
		return "4K"
	}
	if quality == "medium" && containsString(resolutions, "2K") {
		return "2K"
	}
	if containsString(resolutions, "1K") {
		return "1K"
	}
	if len(resolutions) > 0 {
		return resolutions[0]
	}
	return ""
}

// upDreamModelResolutions 从账户模型快照里找对应模型的 resolutions + defaultResolution。
func upDreamModelResolutions(account *model.UserVendorAccount, modelID, capability string) (resolutions []string, defaultResolution string) {
	if account == nil || strings.TrimSpace(account.AvailableModelsJSON) == "" || modelID == "" {
		return nil, ""
	}
	var snap vendorModelsSnapshot
	if err := json.Unmarshal([]byte(account.AvailableModelsJSON), &snap); err != nil {
		return nil, ""
	}
	var list []map[string]any
	switch capability {
	case "video":
		list = snap.VideoModels
	case "audio":
		list = snap.AudioModels
	case "text":
		list = snap.TextModels
	default:
		list = snap.ImageModels
	}
	for _, m := range list {
		id, _ := m["id"].(string)
		if strings.TrimSpace(id) != modelID {
			continue
		}
		constraints, _ := m["constraints"].(map[string]any)
		if constraints == nil {
			continue
		}
		if raw, ok := constraints["resolutions"].([]any); ok {
			for _, r := range raw {
				if s, ok := r.(string); ok && s != "" {
					resolutions = append(resolutions, s)
				}
			}
		}
		if s, ok := constraints["defaultResolution"].(string); ok {
			defaultResolution = s
		}
		break
	}
	return
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}

func boolIntString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// ============== 余额（best-effort 探活 + 失败可观察）==============

// fetchUpDreamBalanceInto 真实余额接口（2026-08-17 走代理嗅探确认）。
//
// 真端点：GET /api/user/account（走代理时返回 200）
// 响应示例：{"data":{"balance_credits":"0","today_credits":"0","month_credits":"24400",
//                    "permanent_credits":"0","temporary_credits":"0","frozen_credits":"0",
//                    "membership":"Max","membership_end_at":"2027-07-28"}}
//
// 字段语义（基于字段名推测，文档未公开）：
//   - balance_credits / today_credits / month_credits / permanent_credits / temporary_credits / frozen_credits 都是 string 类型
//   - membership 当前会员套餐名（"Max"）
//   - membership_end_at 到期时间 ISO 字符串
//
// BalanceInfoJSON 与 renderBalanceText() 兼容。
func fetchUpDreamBalanceInto(ctx context.Context, vendor *model.Vendor, account *model.UserVendorAccount) error {
	if account == nil || account.ID == "" {
		return errors.New("UpDream 账户为空")
	}
	if strings.TrimSpace(account.AccessToken) == "" {
		return errors.New("UpDream 鉴权缺失（请在绑定时提供 Authorization Bearer JWT）")
	}

	extras := map[string]string{}
	if account.RawExtraJSON != "" {
		_ = json.Unmarshal([]byte(account.RawExtraJSON), &extras)
	}

	// 真端点：/api/user/account（走代理才能拿到真数据）
	body, status, err := updreamHTTPGetRaw(ctx, vendor, account, "/api/user/account")
	if status == 200 && err == nil {
		var parsed struct {
			Data struct {
				BalanceCredits    string `json:"balance_credits"`
				TodayCredits      string `json:"today_credits"`
				MonthCredits      string `json:"month_credits"`
				PermanentCredits  string `json:"permanent_credits"`
				TemporaryCredits  string `json:"temporary_credits"`
				FrozenCredits     string `json:"frozen_credits"`
				Membership        string `json:"membership"`
				MembershipEndAt   string `json:"membership_end_at"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &parsed); err == nil {
			d := parsed.Data
			// credits 字段值是 string，转 int（如果非数字则保留 string 给 UI 看）
			toInt := func(s string) int {
				s = strings.TrimSpace(s)
				if s == "" {
					return 0
				}
				n, _ := strconv.Atoi(s)
				return n
			}
			info := map[string]any{
				"balance_cents":     toInt(d.BalanceCredits),
				"package":     "UpDream " + strings.TrimSpace(d.Membership),
				"monthCredits": toInt(d.MonthCredits),
				"todayCredits": toInt(d.TodayCredits),
				"frozenCredits": toInt(d.FrozenCredits),
				"membership":  strings.TrimSpace(d.Membership),
				"membershipEndAt": strings.TrimSpace(d.MembershipEndAt),
			}
			// balanceText 拼接：套餐 + 余额（含永久/临时拆分）+ 本月已用 + 到期
			parts := []string{fmt.Sprintf("UpDream %s", strings.TrimSpace(d.Membership))}
			if toInt(d.BalanceCredits) > 0 {
				balanceLabel := fmt.Sprintf("积分 %d", toInt(d.BalanceCredits))
				perm := toInt(d.PermanentCredits)
				temp := toInt(d.TemporaryCredits)
				if perm > 0 && temp > 0 {
					balanceLabel += fmt.Sprintf("（永久 %d + 临时 %d）", perm, temp)
				} else if perm > 0 {
					balanceLabel += fmt.Sprintf("（永久 %d）", perm)
				} else if temp > 0 {
					balanceLabel += fmt.Sprintf("（临时 %d）", temp)
				}
				parts = append(parts, balanceLabel)
			}
			if toInt(d.MonthCredits) > 0 {
				parts = append(parts, fmt.Sprintf("本月 %d", toInt(d.MonthCredits)))
			}
			if toInt(d.FrozenCredits) > 0 {
				parts = append(parts, fmt.Sprintf("冻结 %d", toInt(d.FrozenCredits)))
			}
			if strings.TrimSpace(d.MembershipEndAt) != "" {
				parts = append(parts, "至 "+strings.TrimSpace(d.MembershipEndAt))
			}
			info["balanceText"] = strings.Join(parts, " · ")

			if b, e := json.Marshal(info); e == nil {
				account.BalanceInfoJSON = string(b)
			}
			delete(extras, "updream_last_balance_error")
			extras["updream_last_balance_ok"] = time.Now().UTC().Format(time.RFC3339)
			extras["updream_account_raw"] = truncate(body, 500)
			if b, e := json.Marshal(extras); e == nil {
				account.RawExtraJSON = string(b)
			}
			if _, err := repository.SaveUserVendorAccount(*account); err != nil {
				return fmt.Errorf("保存 UpDream 余额失败: %w", err)
			}
			return nil
		}
		extras["updream_last_balance_error"] = fmt.Sprintf("user/account parse failed: %v body=%s", err, truncate(body, 200))
	} else {
		extras["updream_last_balance_error"] = fmt.Sprintf("user/account HTTP %d err=%v", status, err)
		extras["updream_account_probe"] = truncate(body, 200)
	}

	if b, e := json.Marshal(extras); e == nil {
		account.RawExtraJSON = string(b)
	}
	if _, err := repository.SaveUserVendorAccount(*account); err != nil {
		return fmt.Errorf("保存 UpDream 余额失败: %w", err)
	}
	return nil
}