package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// ============ LibTV 适配器（liblib.tv 创作站 api.liblib.tv，Token 鉴权） ============
//
// 鉴权：HTTP header `Token: <16进制>`（首字母大写），不是 cookie 不是 AK/SK。
// 域：api.liblib.tv（liblib.tv 创作站内部）。
// 协议：/api/task/generation/create（POST）→ taskId；/api/task/generation/progress（POST）→ 任务进度 + 图片 URL。
// 凭证：用户在 liblib.tv 浏览器登录后，DevTools → Network → Request Headers → 复制 `Token` 字段 value。
//
// 与 NewWow（accesstoken）/ UpDream（buvid3 Cookie）的关系：都是 `vendor.AuthMode = custom_header` 范式，
// 唯一区别是 vendor.AuthHeaderName 字段不同（"Token" vs "accesstoken"）。
// 默认值在 vendor 表的初始化逻辑里（service/vendor.go 的 defaultVendor*）。
//
// 本文件是 libtv 唯一的适配器实现与注册入口（init()）。原先另有一份开放平台适配器
// vendor_libtv.go（openapi.liblibai.cloud + AK/SK HMAC-SHA1，未真机验证、默认路径下从不生效），
// 已删除，避免两个 init() 靠文件名顺序 last-wins 覆盖导致生效适配器被静默翻转。

const (
	libTVTaskAPIBase     = "https://api.liblib.tv"
	libTVTaskURICreate   = "/api/task/generation/create"
	libTVTaskURIProgress = "/api/task/generation/progress"

	// LibTV 任务状态（来自 progress 响应 queueInfo.status / progresses[].status）：
	//   1 = 排队/运行中
	//   2 = 成功（待 taskResult JSON 字符串里含图片 URL）
	//   3 = 失败
	// （实际值待用户嗅探进度 100% 的 progress 响应再校准；这里给保守默认值。）
	libTVTaskStatusRunning = 1
	libTVTaskStatusSuccess = 2
	libTVTaskStatusFailed  = 3

	// LibTV 任务类型（params.taskType）
	libTVTaskTypeImage = "image"
	libTVTaskTypeVideo = "video"
)

// libtvTaskAdapter 走 liblib.tv 创作站（api.liblib.tv）的"提交任务 + 轮询进度"路径。
// 鉴权由 applyVendorAuth(req, vendor, account) 统一注入 vendor.AuthHeaderName 命名的 header。
type libtvTaskAdapter struct {
	vendor *model.Vendor
	client *http.Client
}

// init 注册 libtv 唯一的适配器实现。不再依赖"两个 init 谁后注册谁覆盖"的文件名顺序，
// 避免改文件名 / 重构时静默翻转生效适配器。
func init() {
	RegisterVendorAdapter(model.VendorTypeLibTV, func(v *model.Vendor) VendorAdapter {
		return &libtvTaskAdapter{
			vendor: v,
			client: &http.Client{Timeout: 5 * time.Minute},
		}
	})
}

// ============== 共享响应解析辅助（原 vendor_libtv.go，已合并到这里） ==============

// readJSONResponse 读取响应并解析成 map；同时返回原始 body 供日志。
func readJSONResponse(resp *http.Response) (map[string]any, string, error) {
	defer resp.Body.Close()
	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", fmt.Errorf("读取响应失败: %w", err)
	}
	raw := string(rawBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, raw, fmt.Errorf("LibTV 接口返回 HTTP %d: %s", resp.StatusCode, truncate(raw, 512))
	}
	var payload map[string]any
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		return nil, raw, fmt.Errorf("解析 LibTV 响应 JSON 失败: %w", err)
	}
	return payload, raw, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ============== 鉴权 & 账户 ==============

func (a *libtvTaskAdapter) BuildOAuthAuthorizeURL(_ context.Context, _ *model.Vendor, _ string) (string, error) {
	// liblib.tv 创作站不走 OAuth，用户手动登录后复制 Token header 值
	return "https://www.liblib.tv/", nil
}

func (a *libtvTaskAdapter) ExchangeOAuthCode(_ context.Context, _ *model.Vendor, _ string, _ string) (*model.UserVendorAccount, error) {
	return nil, ErrNotSupported
}

// RefreshAccessToken Token 无刷新机制；把过期时间置远期避免反复触发刷新。
func (a *libtvTaskAdapter) RefreshAccessToken(_ context.Context, account *model.UserVendorAccount) error {
	if account == nil {
		return errors.New("账户为空")
	}
	expire := time.Now().Add(365 * 24 * time.Hour)
	account.TokenExpiresAt = &expire
	return nil
}

// GetAccountInfo liblib.tv 没有干净的余额/账户信息端点（避免误覆盖已有昵称）。
func (a *libtvTaskAdapter) GetAccountInfo(_ context.Context, account *model.UserVendorAccount) error {
	if account == nil {
		return errors.New("账户为空")
	}
	return nil
}

// VerifyLoginCredentials Token 模式没法远端验证（没公开的"用 Token 就能查用户"端点），
// 只检查 token 非空 + 形式上像 16 进制字符串。具体有效性由首次生图提交兜底。
func (a *libtvTaskAdapter) VerifyLoginCredentials(_ context.Context, params VerifyCredentialsParams) (*CredentialVerifyResult, error) {
	tok := strings.TrimSpace(params.CookieString)
	if tok == "" {
		return nil, errors.New("未提供 Token（从 DevTools Request Headers → Token 字段复制 value）")
	}
	if !looksLikeHexToken(tok) {
		return nil, errors.New("Token 格式不像 16 进制字符串（应为 32-64 位十六进制），请重新复制")
	}
	return &CredentialVerifyResult{
		Valid:       true,
		VendorUserID: params.VendorUserID,
		DisplayName:  params.VendorUserID,
	}, nil
}

// looksLikeHexToken 启发式：32-128 位 [0-9a-f]，覆盖 liblib.tv 那种 16 进制 token。
func looksLikeHexToken(s string) bool {
	if len(s) < 16 || len(s) > 256 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// ============== 模型列表 ==============
//
// 真实 liblib.tv 创作站支持 seedream / kling / sora / 即梦 / ... 等模型，
// 完整清单需要嗅探 /list?productTypes=... 这类请求的响应。本文件给出最小可用集，
// 等用户嗅探到完整 model list 再扩。
func (a *libtvTaskAdapter) ListModels(ctx context.Context, account *model.UserVendorAccount) (*VendorModels, error) {
	// 优先从 liblib.tv 真实接口拉（带 Token），返回非空就用远端；失败 fallback 到 hard-coded 7 个
	if account != nil && strings.TrimSpace(account.AccessToken) != "" {
		token := strings.TrimSpace(account.AccessToken)
		if decrypted, err := DecryptCredential(token); err == nil && decrypted != "" {
			token = decrypted
		}
		if remote, err := fetchLibTVModelsFromAPI(ctx, token); err == nil && (len(remote.ImageModels) > 0 || len(remote.VideoModels) > 0) {
			return remote, nil
		}
	}
	return libtvHardcodedModels(), nil
}

// libtvHardcodedModels 远端 API 失败时的兜底模型列表
func libtvHardcodedModels() *VendorModels {
	return &VendorModels{
		ImageModels: []VendorModelInfo{
			{ID: "seedream-4.5", Name: "LibTV Seedream 4.5 文生图", Capability: "image", DefaultFor: "imageModel",
				Constraints: map[string]any{"ratios": []any{"1:1", "16:9", "9:16", "4:3", "3:4"}, "qualities": []any{"1K", "2K", "4K"}, "maxCount": 4}},
		{ID: "seedream-4", Name: "LibTV Seedream 4 文生图", Capability: "image",
			Constraints: map[string]any{"ratios": []any{"1:1", "16:9", "9:16", "4:3", "3:4"}, "qualities": []any{"1K", "2K", "4K"}, "maxCount": 4}},
		},
		VideoModels: []VendorModelInfo{
			{ID: "star-video2.5", Name: "LibTV Seedance 2.5 720P", Capability: "video", DefaultFor: "videoModel",
				Constraints: map[string]any{"durations": []any{1, 2, 3, 4, 5, 6, 8, 10}, "aspects": []any{"16:9", "9:16", "1:1"}, "resolutions": []any{"720p"}}},
			{ID: "star-video2-mini", Name: "LibTV Seedance 2.0 Mini", Capability: "video",
				Constraints: map[string]any{"durations": []any{1, 2, 3, 4, 5, 6, 8, 10}, "aspects": []any{"16:9", "9:16", "1:1"}}},
			{ID: "star-video2-fast", Name: "LibTV Seedance 2.0 Fast VIP", Capability: "video",
				Constraints: map[string]any{"durations": []any{1, 2, 3, 4, 5, 6, 8, 10}}},
			{ID: "happy-horse-1.1", Name: "LibTV Happy Horse 1.1", Capability: "video",
				Constraints: map[string]any{"durations": []any{1, 2, 3, 4, 5, 6, 8, 10}}},
			{ID: "MiniMax-Hailuo-H3", Name: "LibTV MiniMax H3", Capability: "video",
				Constraints: map[string]any{"durations": []any{1, 2, 3, 4, 5, 6, 8, 10}, "resolutions": []any{"2K", "768P"}}},
		},
	}
}

// fetchLibTVModelsFromAPI 调 liblib.tv 真实接口拉模型清单。
//
//	策略：直接 GET 主页 HTML，从 Next.js 的 self.__next_f.push([1,"..."], "...") chunks 中
//	解析出嵌入的 RSC payload（含 modelKey/modelName 列表）。
//
//	为什么用这个方法而不是 _rsc=ID 直接 fetch：
//	  - _rsc=ID 是 Next.js 每次 SSR 时生成的 session-specific ID，无法预知；下次刷新就过期
//	  - HTML 页本身就包含了完整的 RSC payload（通过 __next_f.push 调用注入到页面）
//	  - 这种方式更稳定：每次访问 liblib.tv 首页都能拿到完整模型清单（实测 96 个）
//
//	端点: GET https://www.liblib.tv/
//	鉴权: HTTP header Token: <account.AccessToken>（不带 Token 也能拿到数据，但带是惯例）
//	响应: HTML（~700KB），里头嵌入 27-29 个 __next_f.push chunks
//	  chunk 内容格式：\"modelKey\":\"...\", \"modelName\":\"...\", \"description\":\"...\"（含转义）
//
//	辅端点: GET https://api2.liblib.art/api/www/commerce/activity/benefit/list?productTypes=N
//	  - 只返回当前用户有访问权限 + 有活跃促销的模型
//	  - 提供 modelVersionId / targetParams 等附加信息（task 提交时可能用 versionId）
//
//	去重逻辑：同一 modelKey 跨端点出现只保留一次；优先用 benefit/list 的版本信息。
func fetchLibTVModelsFromAPI(ctx context.Context, token string) (*VendorModels, error) {
	seen := make(map[string]bool)
	var imageList, videoList, textList, audioList []VendorModelInfo
	var versionByKey = make(map[string]map[string]any)

	// 第一步：HTML 页面 → 解析 __next_f.push chunks 里的 modelKey
	homeURL := "https://www.liblib.tv/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, homeURL, nil)
	if err == nil {
		req.Header.Set("Token", strings.TrimSpace(token))
		req.Header.Set("Accept", "text/html")
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			for _, m := range parseLibTVHomePageHTML(string(raw)) {
				if seen[m.ID] {
					continue
				}
				seen[m.ID] = true
				cap := inferLibTVCapability(m.ID)
				m.Capability = cap
				switch cap {
				case "video":
					videoList = append(videoList, m)
				case "text":
					textList = append(textList, m)
				case "audio":
					audioList = append(audioList, m)
				default:
					imageList = append(imageList, m)
				}
			}
		}
	}

	// 第二步：benefit/list（补 versionId / targetParams）
	urls := []string{
		"https://api2.liblib.art/api/www/commerce/activity/benefit/list?productTypes=1",
		"https://api2.liblib.art/api/www/commerce/activity/benefit/list?productTypes=3",
	}
	client := &http.Client{Timeout: 6 * time.Second}
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Token", strings.TrimSpace(token))
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		raw, _ := func() ([]byte, error) {
			defer resp.Body.Close()
			return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		}()
		var parsed struct {
			Code int `json:"code"`
			Data struct {
				List []struct {
					TargetType   string `json:"targetType"`
					TargetKey    string `json:"targetKey"`
					TargetName   string `json:"targetName"`
					TargetParams string `json:"targetParams"`
				} `json:"list"`
			} `json:"data"`
		}
		_ = json.Unmarshal(raw, &parsed)
		if parsed.Code != 0 {
			continue
		}
		for _, item := range parsed.Data.List {
			if item.TargetType != "modelVersion" && item.TargetType != "modelKey" {
				continue
			}
			modelKey := extractLibTVModelKey(item.TargetKey, item.TargetParams)
			if modelKey == "" {
				continue
			}
			// 收集版本信息（用于 task 提交时用 versionId）
			if item.TargetParams != "" {
				var p map[string]any
				if json.Unmarshal([]byte(item.TargetParams), &p) == nil {
					versionByKey[modelKey] = p
				}
			}
			// 不重复添加；只补 Extra 信息
			if !seen[modelKey] {
				seen[modelKey] = true
				info := VendorModelInfo{
					ID:   modelKey,
					Name: "LibTV " + strings.TrimSpace(item.TargetName),
				}
				cap := inferLibTVCapability(modelKey)
				info.Capability = cap
				switch cap {
				case "video":
					videoList = append(videoList, info)
				case "text":
					textList = append(textList, info)
				case "audio":
					audioList = append(audioList, info)
				default:
					imageList = append(imageList, info)
				}
			}
		}
	}

	// 把 versionByKey 信息塞到每个已存在的 VendorModelInfo.Extra
	for _, list := range [][]*VendorModelInfo{toPtrSlice(imageList), toPtrSlice(videoList), toPtrSlice(textList), toPtrSlice(audioList)} {
		for _, m := range list {
			if v, ok := versionByKey[m.ID]; ok {
				if m.Extra == nil {
					m.Extra = map[string]any{}
				}
				for k, vv := range v {
					m.Extra[k] = vv
				}
			}
		}
	}

	if len(imageList) == 0 && len(videoList) == 0 && len(textList) == 0 && len(audioList) == 0 {
		return nil, errors.New("libtv 模型清单为空")
	}
	return &VendorModels{
		ImageModels: imageList,
		VideoModels: videoList,
		TextModels:  textList,
		AudioModels: audioList,
	}, nil
}

// parseLibTVHomePageHTML 从 liblib.tv 主页 HTML 中提取所有 modelKey/modelName/description
//
//	Next.js SSR 把 RSC payload 注入到页面：self.__next_f.push([1,"...content..."], "...")
//
//	content 是转义过的 JSON-like 文本（含 \\n 换行、\\" 引号），需要先 unescape 再正则匹配。
func parseLibTVHomePageHTML(html string) []VendorModelInfo {
	var out []VendorModelInfo
	seen := make(map[string]bool)
	chunkRe := regexp.MustCompile(`self\.__next_f\.push\(\[\s*1\s*,\s*"(.+?)"\s*\]\)`)
	mkRe := regexp.MustCompile(`"modelKey"\s*:\s*"([^"]+)"\s*,\s*"modelName"\s*:\s*"([^"]+)"(?:[^}]{0,200}"description"\s*:\s*"([^"]*)")?`)
	for _, chunk := range chunkRe.FindAllStringSubmatch(html, -1) {
		content := chunk[1]
		// unescape: \" → "，\\ → \
		unescaped := strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(content)
		for _, m := range mkRe.FindAllStringSubmatch(unescaped, -1) {
			key := m[1]
			if seen[key] {
				continue
			}
			seen[key] = true
			desc := m[3]
			out = append(out, VendorModelInfo{
				ID:   key,
				Name: "LibTV " + strings.TrimSpace(m[2]),
				Extra: map[string]any{
					"description": desc,
				},
			})
		}
	}
	return out
}

// toPtrSlice 取一个值类型的 slice 转成指针 slice（仅 in-place 改 Extra 用，避免代码重复）
func toPtrSlice(s []VendorModelInfo) []*VendorModelInfo {
	out := make([]*VendorModelInfo, len(s))
	for i := range s {
		out[i] = &s[i]
	}
	return out
}

// extractLibTVModelKey 从 API 响应里抽真正的 modelKey
// modelVersion 类型时，targetKey 是数字 ID，modelKey 在 targetParams JSON 里
func extractLibTVModelKey(targetKey, targetParams string) string {
	if strings.Contains(targetParams, "modelKey") {
		var p struct {
			ModelKey string `json:"modelKey"`
		}
		if json.Unmarshal([]byte(targetParams), &p) == nil && p.ModelKey != "" {
			return p.ModelKey
		}
	}
	return targetKey
}

// inferLibTVCapability 根据 modelKey 推断模型是图、视频、文本/LLM 还是音频。
//
//	优先级（精确匹配 > 前缀匹配）：
//	  - 音频：mureka-*（音乐生成）/ vocal-*（语音/歌唱）/ speech-*（语音合成）/ minimax-voice-* / seed-audio-*
//	  - 文本/LLM：aurora-*（多模态文本）/ cvlm-*（computer vision LM）/ deepseek-* / qwen-3-*（不含 qwen-image-*）
//	  - 视频生成：kling-video-* / kling-v*（含 kling-v2/kling-2.1）/ pixverse-* / vidu* / wan* / motion-* / MiniMax-Hailuo-* / happy-horse-* / seedance-* / star-video* / doubao-seedance-* / wanxiang-* / kling-v3-omni / midjourney-video / scene-* / omnihuman-*
//	  - 视频处理：topaz-video-* / volcano-video-* / volcano-subtitle-* / volcano-portrait-*
//	  - 图片生成：seedream / flux / mj-* / jimeng / nebula / lib-image / z-image / qwen(-image-*) / kling-multi-shot / multiple-angles / image-editor* / topaz-image-* / hd-upscaling / seed-evolving / orbit-* / doubao-seedream-*
//	默认 fallthrough 到 image（多数创作者平台以图为主）。
func inferLibTVCapability(modelKey string) string {
	k := strings.ToLower(modelKey)
	// 1) 音频
	switch {
	case strings.HasPrefix(k, "mureka-"),
		strings.HasPrefix(k, "vocal-"),
		strings.HasPrefix(k, "speech-"),
		strings.HasPrefix(k, "minimax-voice-"),
		strings.HasPrefix(k, "seed-audio-"):
		return "audio"
	}
	// 2) 文本 LLM
	switch {
	case strings.HasPrefix(k, "aurora-"):
		return "text"
	case strings.HasPrefix(k, "cvlm-"):
		return "text"
	case strings.HasPrefix(k, "deepseek-"):
		return "text"
	case strings.HasPrefix(k, "qwen-3"):
		return "text"
	}
	// 3) 视频生成
	switch {
	case strings.HasPrefix(k, "kling-video"),
		strings.HasPrefix(k, "kling-v"),
		strings.HasPrefix(k, "pixverse-"),
		strings.HasPrefix(k, "vidu"),
		strings.HasPrefix(k, "wan"),
		strings.HasPrefix(k, "wanx"),
		strings.HasPrefix(k, "motion-"),
		strings.HasPrefix(k, "minimax-hailuo"),
		strings.HasPrefix(k, "happy-horse"),
		strings.HasPrefix(k, "seedance"),
		strings.HasPrefix(k, "star-video"),
		strings.HasPrefix(k, "doubao-seedance"),
		k == "kling-multi-shot",
		strings.HasPrefix(k, "midjourney-video"),
		strings.HasPrefix(k, "scene-"),
		strings.HasPrefix(k, "omnihuman-"):
		return "video"
	}
	// 4) 视频处理工具（upscaler / eraser / matting）
	switch {
	case strings.Contains(k, "video-upscaler"),
		strings.Contains(k, "subtitle-eraser"),
		strings.Contains(k, "portrait-matting"):
		return "video"
	}
	// 5) 图片（含生成、编辑、超分）
	return "image"
}

// fetchLibTVBalanceInto 调 liblib.tv 真实接口拉余额，写到 account.BalanceInfoJSON。
//
//	余额口径（2026-08-17 用 token 实测确认）：
//	  真实「积分余额」来自积分明细汇总接口 /power/translogs/management/summary（POST），
//	  请求体 {"opTypeCode": 1|2|3}，响应 {"code":0,"data":{"totalCount":N,"totalAmount":M}}
//	    - opTypeCode=1 → 获取（收入）
//	    - opTypeCode=2 → 消耗（支出）
//	    - opTypeCode=3 → 已返还
//	  余额 = 获取 − 消耗 + 已返还（生成失败返还的积分加回可用余额；已验证：120 − 33 + 0 = 87）
//
//	为什么不沿用 /api/www/member/account：
//	  /member/account 的 attr.libtvUsablePower 是「会员自由积分配额」（会员免费额度），
//	  跟积分明细里的真实积分（积分余额）不是一回事，之前误把它当余额显示，导致数值不符。
//	  member/account 仅保留用于拿 accountLevelName（会员等级名，做包名展示）。
//
//	BalanceInfoJSON 格式（与 renderBalanceText 兼容）：
//	  {"balance_cents":87,"package":"非会员","balanceText":"积分 87（获取 120 · 消耗 33）"}
//
//	失败时返回 nil（best-effort，不阻塞绑定）。
func fetchLibTVBalanceInto(ctx context.Context, vendor *model.Vendor, account *model.UserVendorAccount) error {
	if account == nil || account.ID == "" {
		return nil
	}
	client := &http.Client{Timeout: 8 * time.Second}

	// 1) 真实积分余额：/power/translogs/management/summary（opTypeCode 1/2/3）
	income, incomeOK := fetchLibTVSummaryAmount(ctx, client, vendor, account, 1)
	expense, expenseOK := fetchLibTVSummaryAmount(ctx, client, vendor, account, 2)
	returned, returnedOK := fetchLibTVSummaryAmount(ctx, client, vendor, account, 3)
	if !incomeOK || !expenseOK {
		// 汇总接口拿不到 → 不覆盖成错误数据，直接放行（沿用旧 BalanceInfoJSON）
		return nil
	}
	// 余额 = 获取 − 消耗 + 已返还（生成失败返还的积分应加回可用余额）
	balance := income - expense + returned
	if balance < 0 {
		balance = 0
	}

	// 2) 会员等级名（来自 /member/account，best-effort；失败就给默认值）
	levelName := fetchLibTVAccountLevelName(ctx, client, vendor, account)

	// 3) 渲染 BalanceInfoJSON（renderBalanceText 优先用 balanceText，credits 供数字徽标）
	balanceText := fmt.Sprintf("积分 %d（获取 %d · 消耗 %d", balance, income, expense)
	if returnedOK && returned > 0 {
		balanceText += fmt.Sprintf(" · 已返还 %d", returned)
	}
	balanceText += "）"

	info := struct {
		CostCents int `json:"costCents"`
		Package     string `json:"package"`
		BalanceText string `json:"balanceText"`
	}{
		CostCents:     balance,
		Package:     levelName,
		BalanceText: balanceText,
	}
	if encoded, err := json.Marshal(info); err == nil {
		if account.BalanceInfoJSON != string(encoded) {
			account.BalanceInfoJSON = string(encoded)
			_, _ = repository.SaveUserVendorAccount(*account)
		}
	}
	return nil
}

// fetchLibTVSummaryAmount 调 /power/translogs/management/summary 拿某一类（opTypeCode）的汇总。
// 返回 (totalAmount, ok)；ok=false 表示请求失败或响应异常（调用方需自行决定兜底）。
func fetchLibTVSummaryAmount(ctx context.Context, client *http.Client, vendor *model.Vendor, account *model.UserVendorAccount, opTypeCode int) (amount int, ok bool) {
	bodyBytes, _ := json.Marshal(map[string]any{"opTypeCode": opTypeCode})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api2.liblib.art/api/www/power/translogs/management/summary", bytes.NewReader(bodyBytes))
	if err != nil {
		return 0, false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://www.liblib.tv")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 FreedomVendorBind/1.0")
	if !applyVendorAuth(req, vendor, account) {
		return 0, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, false
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var payload struct {
		Code int `json:"code"`
		Data struct {
			TotalCount  int `json:"totalCount"`
			TotalAmount int `json:"totalAmount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Code != 0 {
		return 0, false
	}
	return payload.Data.TotalAmount, true
}

// fetchLibTVAccountLevelName 调 /api/www/member/account 拿 accountLevelName（会员等级名）。
// 失败返回 "LibTV"（best-effort，不阻塞余额计算）。
func fetchLibTVAccountLevelName(ctx context.Context, client *http.Client, vendor *model.Vendor, account *model.UserVendorAccount) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api2.liblib.art/api/www/member/account", nil)
	if err != nil {
		return "LibTV"
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://www.liblib.tv")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 FreedomVendorBind/1.0")
	if !applyVendorAuth(req, vendor, account) {
		return "LibTV"
	}
	resp, err := client.Do(req)
	if err != nil {
		return "LibTV"
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "LibTV"
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var payload struct {
		Code int `json:"code"`
		Data *libTVAccountInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Data == nil || payload.Code != 0 {
		return "LibTV"
	}
	name := strings.TrimSpace(payload.Data.AccountLevelName)
	if name == "" {
		name = strings.TrimSpace(payload.Data.Name)
	}
	if name == "" {
		name = "LibTV"
	}
	return name
}

// libTVAccountInfo / attr 是 /api/www/member/account 响应的子结构（容忍字段缺失）。
type libTVAccountInfo struct {
	Name             string          `json:"name"`
	AccountLevelName string          `json:"accountLevelName"`
	Attr             *libTVAccountAttr `json:"attr"`
}

type libTVAccountAttr struct {
	LibtvUsablePower     int `json:"libtvUsablePower"`
	LibtvTotalPower      int `json:"libtvTotalPower"`
	RechargeUsablePower  int `json:"rechargeUsablePower"`
	UsedPower            int `json:"usedPower"`
	FreeUsablePower      int `json:"freeUsablePower"`
	UsablePower          int `json:"usablePower"`
}

// fetchUpDreamBalanceInto 已迁移到 service/vendor_updream.go（不再 no-op 占位）
// 旧占位函数删除 —— 当前实现：调 /api/user-settings + /api/notifications/unread-count 做 best-effort 探活。

// fetchNewWowBalanceInto 真实实现已迁移到 service/vendor_newwow.go（用 Playwright 嗅探 neowow.cn
// 后接入 /user/profile + /user/points-history/v2 + /agent/membership/current）。

// ============== 生成（核心）==============

// GenerateImage 文生图：调 create → 拿到 taskId → 轮询 progress → 拿图片 URL 列表。
func (a *libtvTaskAdapter) GenerateImage(ctx context.Context, account *model.UserVendorAccount, input GenerateImageInput) (*GenerateMediaOutput, error) {
	if a.vendor == nil {
		return nil, errors.New("vendor 未配置")
	}
	if strings.TrimSpace(a.vendor.AuthHeaderName) == "" || strings.TrimSpace(account.AccessToken) == "" {
		return nil, errors.New("LibTV 账户未配置 Token，请重新绑定（DevTools → Request Headers → Token 字段）")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return nil, errors.New("缺少 prompt 参数")
	}
	count := input.Count
	if count < 1 {
		count = 1
	}
	if count > 4 {
		count = 4
	}
	quality, ratio := normalizeLibTVImageMeta(input.Size, input.Quality)
	modelID := strings.TrimSpace(input.Model)
	if modelID == "" {
		modelID = "seedream-4"
	}

	taskID, err := a.submitTask(ctx, account, libtvCreateReq{
		NodeID:    libTVNodeIDImage,
		ProjectID: libTVProjectIDImage,
		Model:     modelID,
		Params: libtvParams{
			Prompt:     input.Prompt,
			Model:      modelID,
			Count:      count,
			ModelType:  "text2image",
			Quality:    quality,
			Ratio:      ratio,
			ModeType:   "text2image",
			Sequential: 1,
			TaskType:   libTVTaskTypeImage,
			Provider:   libTVProviderForImage(modelID),
			RequestID:  strings.ReplaceAll(uuid.NewString(), "-", ""),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LibTV 提交生图失败: %w", err)
	}

	items, raw, err := a.pollTask(ctx, account, taskID, 100, 3*time.Second)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("LibTV 生成成功但未解析到图片 URL: %s", truncate(raw, 512))
	}
	return &GenerateMediaOutput{Items: items, RawBody: raw, TraceID: taskID}, nil
}

// SubmitVideo 实现 VendorVideoSubmitter：视频走相同的 create 接口，taskType=video。
func (a *libtvTaskAdapter) SubmitVideo(ctx context.Context, account *model.UserVendorAccount, input GenerateVideoInput) (string, error) {
	if a.vendor == nil {
		return "", errors.New("vendor 未配置")
	}
	if strings.TrimSpace(a.vendor.AuthHeaderName) == "" || strings.TrimSpace(account.AccessToken) == "" {
		return "", errors.New("LibTV 账户未配置 Token")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return "", errors.New("缺少 prompt 参数")
	}
	duration := input.Seconds
	if duration < 1 {
		duration = 1
	}
	if duration > 60 {
		duration = 60
	}
	aspectRatio := input.Size
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}
	modelID := strings.TrimSpace(input.Model)
	if modelID == "" {
		modelID = "viduq3-pro"
	}
	resolution := "720p"
	if r, ok := input.Extra["resolution"].(string); ok && r != "" {
		resolution = r
	}
	style := "general"
	if s, ok := input.Extra["style"].(string); ok && s != "" {
		style = s
	}
	enableSound := "off"
	if input.GenerateAudio {
		enableSound = "on"
	}
	return a.submitTask(ctx, account, libtvCreateReq{
		NodeID:    libTVNodeIDVideo,
		ProjectID: libTVProjectIDVideo,
		Model:     modelID,
		Params: libtvParams{
			Prompt:       input.Prompt,
			Model:        modelID,
			Count:        1,
			ModelType:    "text2video",
			Quality:      "2K",
			Ratio:        aspectRatio,
			ModeType:     "text2video",
			Sequential:   1,
			TaskType:     libTVTaskTypeVideo,
			Provider:     libTVProviderForVideo(modelID),
			RequestID:    strings.ReplaceAll(uuid.NewString(), "-", ""),
			EnableSound:  enableSound,
			Resolution:   resolution,
			Style:        style,
			Extra:        map[string]any{"duration": duration},
		},
	})
}

// GenerateVideo liblib.tv 视频生成走异步提交 + 任务轮询；这里直接复用 GenerateImage 的轮询，
// taskType 在 submit 时已传 video，由 pollTask 内部按 status 分流。
func (a *libtvTaskAdapter) GenerateVideo(ctx context.Context, account *model.UserVendorAccount, input GenerateVideoInput) (*GenerateMediaOutput, error) {
	taskID, err := a.SubmitVideo(ctx, account, input)
	if err != nil {
		return nil, err
	}
	items, raw, err := a.pollTask(ctx, account, taskID, 100, 3*time.Second)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("LibTV 视频生成成功但未解析到视频 URL: %s", truncate(raw, 512))
	}
	return &GenerateMediaOutput{Items: items, RawBody: raw, TraceID: taskID}, nil
}

// GenerateAudio / GenerateText 暂不实现。
func (a *libtvTaskAdapter) GenerateAudio(_ context.Context, _ *model.UserVendorAccount, _ GenerateAudioInput) (*GenerateMediaOutput, error) {
	return nil, ErrNotSupported
}
func (a *libtvTaskAdapter) GenerateText(_ context.Context, _ *model.UserVendorAccount, _ GenerateTextInput) (*GenerateTextOutput, error) {
	return nil, ErrNotSupported
}

// GetTaskStatus 由 video_task.go 异步轮询链路调用：复用 pollTask 的单次快照。
func (a *libtvTaskAdapter) GetTaskStatus(ctx context.Context, account *model.UserVendorAccount, taskID string) (*TaskStatus, error) {
	if a.vendor == nil || strings.TrimSpace(account.AccessToken) == "" {
		return nil, errors.New("LibTV 账户未配置 Token")
	}
	req, err := a.newProgressReq(ctx, account, []string{taskID})
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LibTV 任务进度查询失败: %w", err)
	}
	defer resp.Body.Close()
	payload, raw, err := readJSONResponse(resp)
	if err != nil {
		return nil, err
	}
	prog, ok := firstLibTVProgress(payload)
	if !ok {
		return nil, fmt.Errorf("LibTV 任务状态返回异常: %s", truncate(raw, 512))
	}
	status, percent, items, errMsg := parseLibTVProgress(prog)
	if errMsg != "" {
		return &TaskStatus{ID: taskID, Status: "failed", Message: errMsg, Extra: map[string]any{"raw": truncate(raw, 512)}}, nil
	}
	out := &TaskStatus{ID: taskID, Progress: percent}
	switch status {
	case libTVTaskStatusSuccess:
		out.Status = "completed"
		out.OutputURL = firstURL(items)
		out.Output = &GenerateMediaOutput{Items: items, RawBody: raw}
	case libTVTaskStatusFailed:
		out.Status = "failed"
	default:
		out.Status = "processing"
	}
	return out, nil
}

func (a *libtvTaskAdapter) CancelTask(_ context.Context, _ *model.UserVendorAccount, _ string) error {
	return ErrNotSupported
}

// 资产库 / 上传 / 下载 liblib.tv 创作站不开放，直接不支持。
func (a *libtvTaskAdapter) ListAssets(_ context.Context, _ *model.UserVendorAccount, _ AssetFilter) ([]VendorAsset, int, error) {
	return nil, 0, ErrNotSupported
}
func (a *libtvTaskAdapter) DownloadAsset(_ context.Context, _ *model.UserVendorAccount, _ string) (io.ReadCloser, string, int64, error) {
	return nil, "", 0, ErrNotSupported
}
func (a *libtvTaskAdapter) UploadAsset(_ context.Context, _ *model.UserVendorAccount, _ string, _ string, _ io.Reader, _ int64, _ string) (*VendorAsset, error) {
	return nil, ErrNotSupported
}
func (a *libtvTaskAdapter) DeleteAsset(_ context.Context, _ *model.UserVendorAccount, _ string) error {
	return ErrNotSupported
}

// ============== 提交 / 轮询 内部 ==============

// libtvCreateReq POST /api/task/generation/create 请求体。
// 字段映射见 devtools 嗅探样本（seedream-4，2026-08-16 用户实测）。
//
// 关键：服务端 code=10002 报错 "taskType is required" 表明 taskType 必须放在**顶层**（不在 params 内）。
// params 内的 taskType 字段保留（向后兼容部分 SDK 工具），但服务端校验的是顶层字段。
type libtvCreateReq struct {
	Metadata  libtvMeta   `json:"metadata"`
	NodeID    string      `json:"node_id"`
	ProjectID string      `json:"project_id"`
	Model     string      `json:"model"`
	TaskType  string      `json:"taskType"` // 顶层任务类型（"image" / "video"），服务端校验必需
	Params    libtvParams `json:"params"`
}

type libtvMeta struct {
	NodeID    string `json:"node_id"`
	ProjectID string `json:"project_id"`
}

type libtvParams struct {
	Prompt     string         `json:"prompt"`
	Model      string         `json:"model"`
	Count      int            `json:"count"`
	ModelType  string         `json:"modelType"`   // 图片字段名（devtools seedream-4 实测大写 MT）
	Quality    string         `json:"quality"`
	Ratio      string         `json:"ratio"`
	AudioList  []any          `json:"audioList"`
	ImageLabelList []any      `json:"imageLabelList"`
	ImageList  []any          `json:"imageList"`
	InfiniteSwitch int        `json:"infiniteSwitch"`
	ModeType   string         `json:"modeType"`    // 两种 modeType 都是大写 MT（图片视频一致）
	Sequential int            `json:"sequential"`
	TextList   []any          `json:"textList"`
	VideoList  []any          `json:"videoList"`
	Provider   string         `json:"provider"`
	RequestID  string         `json:"requestId"`
	TaskType   string         `json:"taskType"`
	// 视频场景专用字段（devtools viduq3-pro 实测）：
	EnableSound string `json:"enableSound,omitempty"` // "on"/"off"
	Resolution  string `json:"resolution,omitempty"`  // "720p"/"1080p"
	Style       string `json:"style,omitempty"`       // "general"/其他
	// 注：视频 payload 里 modeltype 是小写 mt（modelType 大小写在图片视频间不一致），
	// liblib 服务端大小写不敏感地接受，所以这里不重复声明；duration / model / prompt /
	// ratio 等字段在 SDK 序列化时会重复（保留最后一个值即可），所以结构体里也不重复。
	Extra      map[string]any `json:"-"` // 透传但 SDK 不写入正式字段时塞进顶层
}

// libtvCreateResp create 响应。
type libtvCreateResp struct {
	Code    int `json:"code"`
	Data    libtvCreateData `json:"data"`
	Msg     string `json:"msg"`
	TraceID string `json:"trace_id"`
}

type libtvCreateData struct {
	Power  int    `json:"power"`
	TaskID string `json:"taskId"`
}

// libtvProgressResp progress 响应。
type libtvProgressResp struct {
	Code    int               `json:"code"`
	Data    libtvProgressData `json:"data"`
	Msg     string            `json:"msg"`
	TraceID string            `json:"trace_id"`
}

type libtvProgressData struct {
	Progresses []libtvProgressItem `json:"progresses"`
}

type libtvProgressItem struct {
	TaskID         string             `json:"taskId"`
	Status         int                `json:"status"`
	StartTimeMs    int64              `json:"startTimeMs"`
	ProgressPercent int               `json:"progressPercent"`
	Power          int                `json:"power"`
	QueueInfo      *libtvQueueInfo    `json:"queueInfo"`
	// 直接展开 queueInfo 的字段（liblib SDK 有时把 status 也放在 taskResult 字符串里）
	TaskResult     string             `json:"taskResult"`
}

type libtvQueueInfo struct {
	Status           int    `json:"status"`
	PercentCompleted int    `json:"percentCompleted"`
	StartTimeMs      int64  `json:"startTimeMs"`
	TaskID           string `json:"taskId"`
	TaskResult       string `json:"taskResult"`
}

// ============== 实时额度估算（power/calculator） ==============
//
// LibTV 创作站提供独立的算力预检端点：
//   POST https://api.liblib.tv/api/task/generation/power/calculator
// 请求体结构与 /api/task/generation/create 基本一致（model / taskType / provider /
// params{count,quality,ratio}），但**不会真正创建生成任务、也不扣费**，仅返回 data.power
// （本次将消耗的金额）。这与 UpDream 的 /api/estimate 等价，是画布节点实时额度估算的数据源。
// 任何异常（网络/鉴权/code!=0/power<=0）一律返回 error → 上层 EstimateVendorCost 降级到前端静态估算。

// libtvCalcResp power/calculator 响应（data.power 与 create 响应同名字段）。
type libtvCalcResp struct {
	Code int           `json:"code"`
	Data libtvCalcData `json:"data"`
	Msg  string        `json:"msg"`
}

type libtvCalcData struct {
	Power         int `json:"power"`
	OriginalPrice int `json:"originalPrice"`
}

// EstimateCost 实现 VendorCostEstimator：调 power/calculator 返回本次将消耗的金额。
func (a *libtvTaskAdapter) EstimateCost(ctx context.Context, account *model.UserVendorAccount, input EstimateCostInput) (float64, error) {
	if a.vendor == nil {
		return 0, errors.New("vendor 未配置")
	}
	if strings.TrimSpace(a.vendor.AuthHeaderName) == "" || strings.TrimSpace(account.AccessToken) == "" {
		return 0, errors.New("LibTV 账户未配置 Token")
	}
	capability := strings.ToLower(strings.TrimSpace(input.Capability))
	if capability == "" {
		capability = "image"
	}
	count := input.Count
	if count < 1 {
		count = 1
	}
	if count > 4 {
		count = 4
	}
	modelID := strings.TrimSpace(input.Model)
	if modelID == "" {
		if capability == "video" {
			modelID = "viduq3-pro"
		} else {
			modelID = "seedream-4"
		}
	}

	taskType := "image"
	modeType := "text2image"
	provider := libTVProviderForImage(modelID)
	quality := "auto"
	ratio := "1:1"
	if capability == "video" {
		taskType = "video"
		modeType = "text2video"
		provider = libTVProviderForVideo(modelID)
		quality = "2K"
		ratio = strings.TrimSpace(input.Size)
		if ratio == "" {
			ratio = "16:9"
		}
	} else {
		q, r := normalizeLibTVImageMeta(input.Size, input.Quality)
		quality = q
		ratio = r
	}

	// 复用 create 请求结构（与 devtools 实测的种子结构一致），但不真正提交任务。
	body := libtvCreateReq{
		NodeID:    libTVNodeIDImage,
		ProjectID: libTVProjectIDImage,
		Model:     modelID,
		TaskType:  taskType,
		Params: libtvParams{
			Prompt:    "",
			Model:     modelID,
			Count:     count,
			ModelType: modeType,
			Quality:   quality,
			Ratio:     ratio,
			ModeType:  modeType,
			Sequential: 1,
			TaskType:  taskType,
			Provider:  provider,
			RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""),
		},
	}
	body.Metadata.NodeID = body.NodeID
	body.Metadata.ProjectID = body.ProjectID
	if capability == "video" {
		body.NodeID = libTVNodeIDVideo
		body.ProjectID = libTVProjectIDVideo
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("序列化 LibTV 估算请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, libTVTaskAPIBase+"/api/task/generation/power/calculator", bytes.NewReader(raw))
	if err != nil {
		return 0, fmt.Errorf("构造 LibTV 估算请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://www.liblib.tv")
	// 鉴权：Token header（依赖 vendor.AuthHeaderName="Token" 与 account.AccessToken 非空）
	if !applyVendorAuth(req, a.vendor, account) {
		return 0, errors.New("LibTV 鉴权未注入：vendor.AuthHeaderName 或 account.AccessToken 为空")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("LibTV 算力预检网络失败: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	rawStr := string(bodyBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("LibTV 算力预检返回 HTTP %d: %s", resp.StatusCode, truncate(rawStr, 256))
	}
	var cr libtvCalcResp
	if err := json.Unmarshal(bodyBytes, &cr); err != nil {
		return 0, fmt.Errorf("LibTV 算力预检响应解析失败: %w (%s)", err, truncate(rawStr, 256))
	}
	if cr.Code != 0 {
		return 0, fmt.Errorf("LibTV 算力预检业务失败（code=%d, msg=%s）", cr.Code, cr.Msg)
	}
	if cr.Data.Power <= 0 {
		return 0, errors.New("LibTV 算力预检未返回有效 power")
	}
	return float64(cr.Data.Power), nil
}

const (
	libTVNodeIDImage    = "1-5n7vzq1hhe" // devtools 实测 seedream-4 提交时带的 node_id
	libTVProjectIDImage = "979b3ded27d54c6e9b17b41773b91fae"
	libTVNodeIDVideo    = "v-1V6accwmtu" // devtools 实测 viduq3-pro 提交时带的 node_id（v- 前缀，与图片不同）
	libTVProjectIDVideo = "979b3ded27d54c6e9b17b41773b91fae"
)

func libTVProviderForImage(modelID string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(modelID), "seedream"):
		return "seedream"
	case strings.HasPrefix(strings.ToLower(modelID), "kling"):
		return "kling"
	case strings.HasPrefix(strings.ToLower(modelID), "jimeng"):
		return "jimeng"
	}
	return modelID
}

func libTVProviderForVideo(modelID string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(modelID), "vidu"):
		return "vidu"
	case strings.HasPrefix(strings.ToLower(modelID), "kling"):
		return "kling"
	case strings.HasPrefix(strings.ToLower(modelID), "sora"):
		return "sora"
	}
	return modelID
}

// submitTask 提交生图/生视频任务，返回 taskId。
func (a *libtvTaskAdapter) submitTask(ctx context.Context, account *model.UserVendorAccount, body libtvCreateReq) (string, error) {
	// metadata 子结构也填充
	body.Metadata.NodeID = body.NodeID
	body.Metadata.ProjectID = body.ProjectID
	// 顶层 taskType 兜底：调用方如果没传，从 Params.TaskType 取；再不行用 params.taskType 的常见值
	// （服务端 code=10002 报错证明这是必需的）
	if body.TaskType == "" {
		body.TaskType = body.Params.TaskType
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, libTVTaskAPIBase+libTVTaskURICreate, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("构造提交请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://www.liblib.tv")
	// 鉴权：Token header（依赖 vendor.AuthHeaderName="Token" 与 account.AccessToken 非空）
	if !applyVendorAuth(req, a.vendor, account) {
		return "", errors.New("LibTV 鉴权未注入：vendor.AuthHeaderName 或 account.AccessToken 为空")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("提交 LibTV 任务网络失败: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	rawStr := string(bodyBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("LibTV 提交返回 HTTP %d: %s", resp.StatusCode, truncate(rawStr, 512))
	}
	var cr libtvCreateResp
	if err := json.Unmarshal(bodyBytes, &cr); err != nil {
		return "", fmt.Errorf("LibTV 提交响应解析失败: %w (%s)", err, truncate(rawStr, 256))
	}
	if cr.Code != 0 {
		return "", fmt.Errorf("LibTV 提交业务失败（code=%d, msg=%s）", cr.Code, cr.Msg)
	}
	if strings.TrimSpace(cr.Data.TaskID) == "" {
		return "", fmt.Errorf("LibTV 提交未返回 taskId: %s", truncate(rawStr, 256))
	}
	return cr.Data.TaskID, nil
}

// newProgressReq 构造 POST /api/task/generation/progress 请求，body 含 taskIds 数组。
func (a *libtvTaskAdapter) newProgressReq(ctx context.Context, account *model.UserVendorAccount, taskIDs []string) (*http.Request, error) {
	body, _ := json.Marshal(map[string]any{"taskIds": taskIDs})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, libTVTaskAPIBase+libTVTaskURIProgress, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://www.liblib.tv")
	if !applyVendorAuth(req, a.vendor, account) {
		return nil, errors.New("LibTV 鉴权未注入")
	}
	return req, nil
}

// pollTask 反复 POST progress 直到 success/failed 或超时。
func (a *libtvTaskAdapter) pollTask(ctx context.Context, account *model.UserVendorAccount, taskID string, maxAttempts int, interval time.Duration) ([]GeneratedAssetItem, string, error) {
	var lastRaw string
	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, lastRaw, fmt.Errorf("LibTV 任务轮询被取消: %w", ctx.Err())
		default:
		}
		req, err := a.newProgressReq(ctx, account, []string{taskID})
		if err != nil {
			return nil, lastRaw, err
		}
		resp, err := a.client.Do(req)
		if err != nil {
			return nil, lastRaw, fmt.Errorf("LibTV 进度查询网络失败: %w", err)
		}
		payload, raw, err := readJSONResponse(resp) // readJSONResponse 内部已 Close
		if err != nil {
			return nil, lastRaw, err
		}
		lastRaw = raw
		prog, ok := firstLibTVProgress(payload)
		if !ok {
			return nil, lastRaw, fmt.Errorf("LibTV 进度响应未包含 task: %s", truncate(raw, 256))
		}
		status, percent, items, errMsg := parseLibTVProgress(prog)
		if errMsg != "" {
			return nil, lastRaw, fmt.Errorf("LibTV 任务失败: %s (%s)", errMsg, truncate(raw, 256))
		}
		// 把 power（任务消耗积分，devtools 实测：图=1、视频=10）透传到 GeneratedAssetItem.RawExtra，
		// 供前端展示"本次消耗 X 积分"。
		if power, ok := prog["power"].(float64); ok && int(power) > 0 {
			for i := range items {
				if items[i].RawExtra == nil {
					items[i].RawExtra = map[string]any{}
				}
				items[i].RawExtra["libtvPower"] = int(power)
			}
		}
		switch status {
		case libTVTaskStatusSuccess:
			if len(items) == 0 {
				return nil, lastRaw, fmt.Errorf("LibTV 任务成功但未解析到结果: %s", truncate(raw, 256))
			}
			// 任务成功 → 刷新账户余额 + 记录 modelKey 消耗 power 到历史
			a.recordSuccess(ctx, account, prog)
			return items, lastRaw, nil
		case libTVTaskStatusFailed:
			return nil, lastRaw, fmt.Errorf("LibTV 任务失败 (status=%d, percent=%d): %s", status, percent, truncate(raw, 256))
		default:
			time.Sleep(interval)
		}
	}
	return nil, lastRaw, fmt.Errorf("LibTV 任务轮询超时（%d 次）: %s", maxAttempts, truncate(lastRaw, 256))
}

// recordSuccess 任务成功后做的事：
//   1. 从 progress 响应里读 power（任务消耗积分），按 modelKey 写到 account.RawExtra["libtvPowerByModel"]
//      → 下次前端模型下拉能展示"上次消耗 X power"
//   2. 重新调 fetchLibTVBalanceInto 拉最新 totalPower → 覆盖 account.BalanceInfoJSON
//   3. 错误一律不外抛（best-effort，不阻塞生成本身）
func (a *libtvTaskAdapter) recordSuccess(ctx context.Context, account *model.UserVendorAccount, prog map[string]any) {
	if account == nil || account.ID == "" {
		return
	}
	power, _ := prog["power"].(float64)
	modelKey, _ := prog["model"].(string)
	if modelKey == "" {
		modelKey, _ = prog["modelKey"].(string)
	}
	if modelKey == "" {
		// 从 taskType 兜底拿不到 modelKey 就算了，power 仍可累计到默认项
		modelKey = "unknown"
	}

	// 1) 记录 power history
	if power > 0 {
		extras := map[string]any{}
		if account.RawExtraJSON != "" {
			_ = json.Unmarshal([]byte(account.RawExtraJSON), &extras)
		}
		history, _ := extras["libtvPowerByModel"].(map[string]any)
		if history == nil {
			history = map[string]any{}
		}
		// 用最近一次的 power 覆盖（同样的 modelKey 取最新值）
		history[modelKey] = map[string]any{
			"power":     int(power),
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}
		extras["libtvPowerByModel"] = history
		if b, err := json.Marshal(extras); err == nil {
			account.RawExtraJSON = string(b)
		}
	}

	// 2) 刷新余额（best-effort，不阻塞任务）
	balCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := fetchLibTVBalanceInto(balCtx, a.vendor, account); err != nil {
		// 静默失败，刷新余额失败不影响主流程
		_ = err
	} else {
		// fetchLibTVBalanceInto 内部 SaveUserVendorAccount 已保存余额，这里再保存 power history
		_, _ = repository.SaveUserVendorAccount(*account)
	}
}

// firstLibTVProgress 在 progress 响应里找到与 taskID 匹配的那条；没有时返回 (nil, false)。
func firstLibTVProgress(payload map[string]any) (map[string]any, bool) {
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return nil, false
	}
	progs, _ := data["progresses"].([]any)
	if len(progs) == 0 {
		return nil, false
	}
	first, _ := progs[0].(map[string]any)
	return first, first != nil
}

// parseLibTVProgress 解析单条 progress item：返回 (status, percent%, items, errorMsg)。
// 实际图片/视频 URL 在 queueInfo.taskResult 字符串里，taskResult 是 JSON 字符串。
func parseLibTVProgress(prog map[string]any) (status, percent int, items []GeneratedAssetItem, errMsg string) {
	if s, ok := prog["status"].(float64); ok {
		status = int(s)
	}
	if p, ok := prog["progressPercent"].(float64); ok {
		percent = int(p)
	}
	taskResult, _ := prog["taskResult"].(string)
	if strings.TrimSpace(taskResult) == "" || taskResult == "{}" {
		// 任务未完成；taskResult 是 "{}"，按 running 处理
		if queueInfo, ok := prog["queueInfo"].(map[string]any); ok {
			if s, ok := queueInfo["status"].(float64); ok && status == 0 {
				status = int(s)
			}
			if tr, ok := queueInfo["taskResult"].(string); ok && strings.TrimSpace(tr) != "" && tr != "{}" {
				taskResult = tr
			}
		}
	}
	if strings.TrimSpace(taskResult) == "" || taskResult == "{}" {
		// 仍然没有 → running
		return status, percent, nil, ""
	}
	// taskResult 是 JSON 字符串，先反序列化
	var result map[string]any
	if err := json.Unmarshal([]byte(taskResult), &result); err != nil {
		errMsg = fmt.Sprintf("taskResult 反序列化失败: %v (raw=%s)", err, truncate(taskResult, 256))
		return status, percent, nil, errMsg
	}
	if items = extractLibTVTaskResult(result); len(items) > 0 {
		if status == 0 {
			status = libTVTaskStatusSuccess
		}
	}
	return status, percent, items, ""
}

// extractLibTVTaskResult 从 taskResult JSON 对象里尽可能宽松地提取图片/视频 URL 列表。
// 待真实响应嗅探后再校准字段名；现有 fallback 覆盖几种常见 schema。
func extractLibTVTaskResult(result map[string]any) []GeneratedAssetItem {
	var urls []string
	// 图片类
	for _, key := range []string{"images", "imageList", "imageUrls", "imgUrls"} {
		if urls = appendURLs(urls, result[key]); len(urls) > 0 {
			return makeItems(urls, "image/png")
		}
	}
	// 视频类
	for _, key := range []string{"videos", "videoList", "videoUrls"} {
		if urls = appendURLs(urls, result[key]); len(urls) > 0 {
			return makeItems(urls, "video/mp4")
		}
	}
	// 单字段兜底
	for _, key := range []string{"imageUrl", "url", "resultUrl", "firstFrame", "videoUrl"} {
		if s, ok := result[key].(string); ok && s != "" {
			return makeItems([]string{s}, "image/png")
		}
	}
	return nil
}

func appendURLs(dst []string, v any) []string {
	if v == nil {
		return dst
	}
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			switch x := item.(type) {
			case string:
				if x != "" {
					dst = append(dst, x)
				}
			case map[string]any:
				// 真实字段名 previewPath 是 liblib.tv 2026-08-16 devtools 实测值（前置以保命中）；
				// 后面跟一组兼容通用命名。图片视频共用同一 fallback 列表（liblib 命名一致）。
				for _, key := range []string{
					"previewPath", "preview",     // liblib.tv 实际字段
					"imageUrl", "url", "imgUrl", "image_url",
					"videoUrl", "video_url", "playUrl", "play_url", "mp4Url",
				} {
					if s, ok := x[key].(string); ok && s != "" {
						dst = append(dst, s)
						break
					}
				}
			}
		}
	case string:
		if t != "" {
			dst = append(dst, t)
		}
	}
	return dst
}

func makeItems(urls []string, mime string) []GeneratedAssetItem {
	items := make([]GeneratedAssetItem, 0, len(urls))
	for _, u := range urls {
		items = append(items, GeneratedAssetItem{ID: u, URL: u, MimeType: mime})
	}
	return items
}

func firstURL(items []GeneratedAssetItem) string {
	if len(items) > 0 {
		return items[0].URL
	}
	return ""
}

// normalizeLibTVImageMeta 把 input.Size ("1024x1024" / "1:1") / input.Quality 转成 liblib.tv 的 quality + ratio。
func normalizeLibTVImageMeta(size, quality string) (string, string) {
	q := strings.ToLower(strings.TrimSpace(quality))
	if q == "" {
		q = "2k"
	}
	switch q {
	case "low", "1k", "sd":
		q = "1K"
	case "medium", "2k", "hd":
		q = "2K"
	case "high", "4k", "ultra":
		q = "4K"
	}
	ratio := strings.TrimSpace(size)
	if ratio == "" {
		ratio = "1:1"
	}
	// "1024x1024" / "1152x864" 转 ratio
	if strings.Contains(ratio, "x") {
		parts := strings.SplitN(ratio, "x", 2)
		if w, err := strconvAtoiSafe(parts[0]); err == nil {
			if h, err := strconvAtoiSafe(parts[1]); err == nil && w > 0 && h > 0 {
				// 简化：宽高比近似的标准比例
				switch {
				case w == h:
					ratio = "1:1"
				case float64(w)/float64(h) > 1.7:
					ratio = "16:9"
				case float64(h)/float64(w) > 1.7:
					ratio = "9:16"
				default:
					ratio = "4:3"
				}
			}
		}
	}
	return q, ratio
}

func strconvAtoiSafe(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit: %c", r)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}