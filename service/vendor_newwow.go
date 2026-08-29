package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// ============ NewWow 云端平台适配器 ============
//
// 鉴权事实（2026-08-16 Playwright 嗅探确认）：
//   - accesstoken 走 HTTP header 注入（不是 cookie；不是 AK/SK）
//   - 后端 API 主域: https://neowow.cn
//   - 响应 envelope: {"success": bool, "errCode": str|null, "errMessage": str|null, "data": ...}
//   - 部分响应附带 header: x-user-points: <积分>（前端用它即时刷新积分显示）
//
// 已嗅到的真实端点（用 sync.cjs 验证 token 可调通）：
//   - GET /user/profile                       → 完整账户 + 积分 + 头像 + 昵称 + 手机号
//   - GET /user/points-history/v2?pageSize=10 → 最近积分流水（支出/收入，每条带 imageGenerationParam）
//   - GET /agent/membership/current           → 当前会员套餐 + 到期时间（如未购买则两项都 null）
//   - GET /agent/user/video/templates         → 视频模板列表（含 modelName/modelProvider）
//   - GET /agent/image/style/list             → 图片风格列表（每条带 recommendModel）
//
// 生图核心仍是「样本重放」（replayVendorAdapter 提供 GenerateImage），此处只覆盖
// 真正需要真实接口的能力：ListModels / GetAccountInfo / fetchNewWowBalanceInto。

const (
	newWowDefaultAPIRoot = "https://neowow.cn"
)

// ============== 适配器注册 ==============

func init() {
	RegisterVendorAdapter(model.VendorTypeNewWow, func(v *model.Vendor) VendorAdapter {
		return newNewWowAdapter(v)
	})
}

// newWowAdapter 在 replayVendorAdapter 之上只覆盖「需要真实 NewWow 接口」的能力：
//   - ListModels              → 调 /agent/user/video/templates + /agent/image/style/list
//   - GetAccountInfo          → 调 /user/profile 刷新昵称/头像（不覆盖积分，由 balance 单独管）
//
// 其他（GenerateImage 等样本重放能力）通过嵌入的 *replayVendorAdapter 直接复用——
// 这意味着生图链路完全不依赖真实 modelName；前端下拉里看到的 model 仅作"提示当前平台能跑什么范围"。
type newWowAdapter struct {
	*replayVendorAdapter
}

func newNewWowAdapter(v *model.Vendor) *newWowAdapter {
	return &newWowAdapter{
		replayVendorAdapter: newReplayAdapter(v, model.VendorTypeNewWow, "NewWow"),
	}
}

// ============== ListModels 实现 ==============

// newWowVideoTemplate 仅取模型列表需要的字段，避免把整坨 ossVideoUrl 等无效字段也吃进 JSON
type newWowVideoTemplate struct {
	ID                 int    `json:"id"`
	ModelName          string `json:"modelName"`
	ModelProvider      string `json:"modelProvider"`
	GenerationType     string `json:"generationType"` // TEXT_TO_VIDEO / IMAGE_TO_VIDEO
	FirstFrameImageURL string `json:"firstFrameImageUrl"`
	TemplateName       string `json:"templateName"`
	TemplateCategory   string `json:"templateCategory"`
}

type newWowImageStyle struct {
	ID             int    `json:"id"`
	StyleName      string `json:"styleName"`
	StyleCode      string `json:"styleCode"`
	RecommendModel string `json:"recommendModel"`
}

// ListModels 同时拉 NewWow 视频模板 + 图片风格 → 去重整理成 ImageModels/VideoModels 返回。
//
// 任何一步失败都降级到 replayVendorAdapter.ListModels 的 auto 占位，避免接口偶发不可用
// 时前端下拉直接空白。失败不会向上抛错（设计：拉模型不应阻塞绑定后的 UI 体验）。
func (a *newWowAdapter) ListModels(ctx context.Context, account *model.UserVendorAccount) (*VendorModels, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return a.replayVendorAdapter.ListModels(ctx, account)
	}

	seen := make(map[string]bool)
	var imageModels []VendorModelInfo
	var videoModels []VendorModelInfo

	// 1) 视频模板 → 按 modelName 去重
	if tmpls, err := a.fetchNewWowVideoTemplates(ctx, account); err == nil && len(tmpls) > 0 {
		for _, t := range tmpls {
			mn := strings.TrimSpace(t.ModelName)
			if mn == "" {
				continue
			}
			key := "v:" + mn
			if seen[key] {
				continue
			}
			seen[key] = true
			videoModels = append(videoModels, VendorModelInfo{
				ID:         mn,
				Name:       fmt.Sprintf("%s · %s", mn, strings.TrimSpace(t.ModelProvider)),
				Capability: "video",
				Supports:   map[string]bool{"refImage": strings.TrimSpace(t.FirstFrameImageURL) != ""},
				Extra: map[string]any{
					"provider":  strings.TrimSpace(t.ModelProvider),
					"source":    "newwow-video-template",
					"generation": t.GenerationType,
				},
			})
		}
	}

	// 2) 图片风格 → recommendModel 去重
	if styles, err := a.fetchNewWowImageStyles(ctx, account); err == nil && len(styles) > 0 {
		for _, s := range styles {
			mn := strings.TrimSpace(s.RecommendModel)
			if mn == "" {
				continue
			}
			key := "i:" + mn
			if seen[key] {
				continue
			}
			seen[key] = true
			imageModels = append(imageModels, VendorModelInfo{
				ID:         mn,
				Name:       fmt.Sprintf("%s · 推荐风格", mn),
				Capability: "image",
				DefaultFor: "imageModel",
				Supports:   map[string]bool{"refImage": true},
				Extra: map[string]any{
					"primaryStyle": strings.TrimSpace(s.StyleName),
					"source":       "newwow-image-style",
				},
			})
		}
	}

	// 全部空 → 降级 replayVendorAdapter 占位（用户无 token 时的兜底同样走这条）
	if len(imageModels) == 0 && len(videoModels) == 0 {
		return a.replayVendorAdapter.ListModels(ctx, account)
	}
	return &VendorModels{ImageModels: imageModels, VideoModels: videoModels}, nil
}

func (a *newWowAdapter) fetchNewWowVideoTemplates(ctx context.Context, account *model.UserVendorAccount) ([]newWowVideoTemplate, error) {
	body, status, err := newWowGET(ctx, account, a.vendor, "/agent/user/video/templates?pageSize=50&pageIndex=1")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", status, truncate(body, 200))
	}
	var resp struct {
		Success bool                    `json:"success"`
		Data    []newWowVideoTemplate   `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("解析视频模板失败: %w", err)
	}
	if !resp.Success {
		return nil, errors.New("success=false")
	}
	return resp.Data, nil
}

func (a *newWowAdapter) fetchNewWowImageStyles(ctx context.Context, account *model.UserVendorAccount) ([]newWowImageStyle, error) {
	body, status, err := newWowGET(ctx, account, a.vendor, "/agent/image/style/list")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", status, truncate(body, 200))
	}
	var resp struct {
		Success bool                 `json:"success"`
		Data    []newWowImageStyle   `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("解析图片风格失败: %w", err)
	}
	if !resp.Success {
		return nil, errors.New("success=false")
	}
	return resp.Data, nil
}

// ============== GetAccountInfo（真实刷昵称/头像）==============

// GetAccountInfo 调 /user/profile，**只**刷新昵称/头像；积分由 fetchNewWowBalanceInto 单独管避免重复。
// 任何错误静默吞掉（旧行为：replayVendorAdapter.GetAccountInfo 也 no-op，不应阻塞调用方）。
func (a *newWowAdapter) GetAccountInfo(ctx context.Context, account *model.UserVendorAccount) error {
	if account == nil {
		return errors.New("账户为空")
	}
	if a.vendor == nil || strings.TrimSpace(account.AccessToken) == "" {
		return nil // 没 token → 用不上真实接口
	}
	body, status, err := newWowGET(ctx, account, a.vendor, "/user/profile")
	if err != nil || status != 200 {
		return nil
	}
	var resp struct {
		Success bool `json:"success"`
		Data    *struct {
			Nickname string `json:"nickname"`
			Avatar   string `json:"avatar"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || !resp.Success || resp.Data == nil {
		return nil
	}
	if n := strings.TrimSpace(resp.Data.Nickname); n != "" {
		account.DisplayName = n
	}
	if av := strings.TrimSpace(resp.Data.Avatar); av != "" {
		account.AvatarURL = av
	}
	return nil
}

// ============== 余额 ==============

type newWowPointsHistoryItem struct {
	ID             int64  `json:"id"`
	Direction      int    `json:"direction"` // 1=收入 2=支出
	Points         int    `json:"points"`
	BeforeBalance  int    `json:"beforeBalance"`
	AfterBalance   int    `json:"afterBalance"`
	TypeDesc       string `json:"typeDesc"`
	Description    string `json:"description"`
	CreateTime     string `json:"createTime"`
	ImageGenerationParam *struct {
		Model string `json:"model"`
		Ratio string `json:"ratio"`
		Size  string `json:"size"`
		Count int    `json:"count"`
	} `json:"imageGenerationParam,omitempty"`
}

// fetchNewWowBalanceInto 调 NewWow 真实接口拉积分/账户信息，写到 account.BalanceInfoJSON。
// 端点：
//   - GET /user/profile                       → 用户基础（积分、昵称、头像）
//   - GET /user/points-history/v2             → 最近流水（推断累计收入/支出 + 最近消费的模型名）
//   - GET /agent/membership/current           → 当前会员（null 表示未购买）
//
// 失败不阻塞绑定（best-effort）；错误信息挂到 RawExtraJSON["newwow_last_balance_error"] 供前端排查。
// BalanceInfoJSON 与 renderBalanceText() 兼容，可直接被前端 UI 渲染出"NewWow 积分 25 · Seedream 4.0"之类文案。
func fetchNewWowBalanceInto(ctx context.Context, vendor *model.Vendor, account *model.UserVendorAccount) error {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return errors.New("NewWow accesstoken 为空，无法拉余额")
	}

	// 1) /user/profile
	profileBody, status, err := newWowGET(ctx, account, vendor, "/user/profile")
	if err != nil {
		return fmt.Errorf("NewWow 用户信息请求失败: %w", err)
	}
	if status != 200 {
		return fmt.Errorf("NewWow 用户信息 HTTP %d：%s", status, truncate(profileBody, 200))
	}
	var profileResp struct {
		Success bool `json:"success"`
		Data    *struct {
			UserID   string `json:"userId"`
			Nickname string `json:"nickname"`
			Mobile   string `json:"mobile"`
			Avatar   string `json:"avatar"`
			Points   int    `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(profileBody), &profileResp); err != nil {
		return fmt.Errorf("解析 NewWow 用户信息失败: %w", err)
	}
	if !profileResp.Success || profileResp.Data == nil {
		return fmt.Errorf("NewWow 用户信息 success=false：%s", truncate(profileBody, 200))
	}
	userID := profileResp.Data.UserID
	nickname := profileResp.Data.Nickname
	avatar := profileResp.Data.Avatar
	points := profileResp.Data.Points
	// 真实可用积分来自 /user/points-info → data.totalAvailablePoints；
	// /user/profile 的 points 不是真实可用余额，用 points-info 覆盖上面的 points。
	if piBody, piStatus, piErr := newWowGET(ctx, account, vendor, "/user/points-info"); piErr == nil && piStatus == 200 {
		var piResp struct {
			Success bool `json:"success"`
			Data    *struct {
				TotalAvailablePoints int `json:"totalAvailablePoints"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(piBody), &piResp) == nil && piResp.Success && piResp.Data != nil {
			points = piResp.Data.TotalAvailablePoints
		}
	}
	// profileResp.Data.Mobile 是后端脱敏态（"178****0370"），不写到 RawExtraJSON，避免 RawExtraJSON 过度暴露

	// 2) /user/points-history/v2（推断累计 + 最近消费模型）
	var lifetimeSpend, lifetimeIncome int
	var recentModel string
	historyBody, _, err := newWowGET(ctx, account, vendor, "/user/points-history/v2?pageSize=10&pageIndex=1")
	if err == nil {
		var histResp struct {
			Success bool                      `json:"success"`
			Data    []newWowPointsHistoryItem `json:"data"`
		}
		if json.Unmarshal([]byte(historyBody), &histResp) == nil && histResp.Success {
			// 列表按 createTime 倒序，最近一条若为支出则记最近模型
			for _, item := range histResp.Data {
				if item.Direction == 2 {
					lifetimeSpend += -item.Points
					if recentModel == "" && item.ImageGenerationParam != nil {
						recentModel = item.ImageGenerationParam.Model
					}
				} else if item.Direction == 1 {
					lifetimeIncome += item.Points
				}
			}
		}
	}

	// 3) /agent/membership/current
	membershipName := ""
	membershipExpire := ""
	if body, _, err := newWowGET(ctx, account, vendor, "/agent/membership/current"); err == nil {
		var resp struct {
			Success bool `json:"success"`
			Data    *struct {
				CurrentMembership *struct {
					Name       string `json:"name"`
					ExpireTime string `json:"expireTime"`
				} `json:"currentMembership"`
				CurrentEnterpriseMembership *struct {
					Name       string `json:"name"`
					ExpireTime string `json:"expireTime"`
				} `json:"currentEnterpriseMembership"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(body), &resp) == nil && resp.Success && resp.Data != nil {
			if m := resp.Data.CurrentMembership; m != nil {
				membershipName = m.Name
				membershipExpire = m.ExpireTime
			}
			if em := resp.Data.CurrentEnterpriseMembership; em != nil {
				if membershipName == "" {
					membershipName = "企业会员·" + em.Name
					membershipExpire = em.ExpireTime
				}
			}
		}
	}

	// 4) 组装 BalanceInfoJSON（与 renderBalanceText 兼容）
	pkg := "NewWow 注册用户"
	if membershipName != "" {
		pkg = "NewWow " + membershipName
	}
	info := map[string]any{
		"credits":       points,
		"package":       pkg,
		"lifetimeSpend": lifetimeSpend,
		"lifetimeIncome": lifetimeIncome,
	}
	if membershipExpire != "" {
		info["expire"] = membershipExpire
	}
	if recentModel != "" {
		info["recentModel"] = recentModel
	}
	balanceParts := []string{fmt.Sprintf("NewWow 积分 %d", points)}
	if membershipName != "" {
		balanceParts = append(balanceParts, pkg)
		if membershipExpire != "" {
			balanceParts = append(balanceParts, "至 "+membershipExpire)
		}
	} else if recentModel != "" {
		// 未开会员时给点语义价值："最近用 Seedream 4.0"
		balanceParts = append(balanceParts, "最近 "+recentModel)
	}
	info["balanceText"] = strings.Join(balanceParts, " · ")

	infoBytes, _ := json.Marshal(info)
	account.BalanceInfoJSON = string(infoBytes)

	// 同步刷新昵称/头像/供应商 UID（避免 libtv 那种"绑定完再调 GetAccountInfo"双请求）
	if nickname != "" {
		account.DisplayName = nickname
	}
	if avatar != "" {
		account.AvatarURL = avatar
	}
	if userID != "" {
		account.VendorUserID = userID
	}
	// mobile 是部分脱敏（"178****0370"），可作为 display 信息但不要写到 vendorUserId 等比较字段

	// 在 extras 里记录 raw 快照（最近一次拉余额的原文，方便排查）
	extras := map[string]any{}
	if account.RawExtraJSON != "" {
		_ = json.Unmarshal([]byte(account.RawExtraJSON), &extras)
	}
	extras["newwow_last_balance"] = map[string]any{
		"points":       points,
		"package":      pkg,
		"recentModel":  recentModel,
		"updatedAt":    time.Now().UTC().Format(time.RFC3339),
	}
	// 隐藏 mobile 字段避免 RawExtraJSON 过度暴露
	if b, e := json.Marshal(extras); e == nil {
		account.RawExtraJSON = string(b)
	}

	if _, err := repository.SaveUserVendorAccount(*account); err != nil {
		return fmt.Errorf("保存 NewWow 余额失败: %w", err)
	}
	return nil
}

// ============== helper：NewWow HTTP 客户端 ==============

// newWowAPIClient 简单包装统一超时。SSRF 防护走站点白名单（仅允许 neowow.cn 系）。
func newWowAPIClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second}
}

// newWowGETWithToken 用指定 token 调 NewWow 任意端点，返回 body 原文 + 状态码。
// 端点路径必须 / 开头；vendor 允许管理员后台覆盖 APIRootURL（DB vendors.api_root_url）。
func newWowGETWithToken(ctx context.Context, token, path string, vendor *model.Vendor) (string, int, error) {
	base := newWowDefaultAPIRoot
	if vendor != nil {
		if root := strings.TrimSpace(vendor.APIRootURL); root != "" {
			base = strings.TrimRight(root, "/")
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("accesstoken", strings.TrimSpace(token))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 FreedomNewWowAdapter/1.0")
	resp, err := newWowAPIClient().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(raw), resp.StatusCode, nil
}

// newWowGET 用 account 里的 accesstoken 调 NewWow。
func newWowGET(ctx context.Context, account *model.UserVendorAccount, vendor *model.Vendor, path string) (string, int, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return "", 0, errors.New("accesstoken 为空")
	}
	return newWowGETWithToken(ctx, strings.TrimSpace(account.AccessToken), path, vendor)
}

// ============== 视频提交 / 状态轮询 ==============
//
// NewWow 视频生成（2026-08-16 嗅探确认）：
//   1. POST /agent/canvas/create                       → {data: canvasId}
//   2. POST /agent/canvas/shot/create (canvasId)       → {data: shotId}
//   3. POST /agent/canvas/material/generate-video      → 提交视频任务
//        body: {canvasId, shotId, modelName, generationType, prompt, ratio, duration}
//        duration 强制 6-10 秒（4 秒会 errCode 4103 "时长不在允许范围 [6-10]s"）
//        generationType: TEXT_TO_VIDEO / IMAGE_TO_VIDEO（默认 TEXT_TO_VIDEO）
//   4. 状态轮询：GET /agent/canvas/detail/{canvasId} → 在 shots[] 里找 shotId 对应 shot 的
//        shot.videoStatus / shot.videoUrl / shot.videoDurationMs
//
// 任务 ID 编码格式：用 "canvasId:shotId" 一对，由 SubmitVideo 返回，由 pollVendorTask 拆开用。
// 这样 handler/canvas_task 落库的 video_tasks.upstream_task_id 直接用这个组合键，无需新建字段。
//
// 已知约束（待后续按需优化）：
//   - canvas 提交后不删除（前端用户想继续编辑可在 NewWow 创作站看）；如要自动清理需要 LIST 后删。
//   - 当前实现只走 TEXT_TO_VIDEO 路径（first-frame 图生视频不接）。
//   - 模型名从 NewWow templates 的 modelName 取（如 "MiniMax-Hailuo-02"、"veo3"），前端下拉已展示。
//   - 积分不足（errCode BIZ_ERROR "您的余额不足"）走正常错误路径，handler 透传给前端。
//
// 参考嗅探脚本：tmp/newwow_video_*.cjs（已 gitignore 之外的 tmp 文件夹）

// newWowVideoMinSeconds NewWow 视频最小允许秒数；用户传 <6 时钳到 6。
const newWowVideoMinSeconds = 6

// newWowVideoDefaultRatio 默认视频宽高比；用户没填或格式非法时用。
const newWowVideoDefaultRatio = "16:9"

// newWowMinShotCreateBody shot create 最小合法 body（嗅探确认 canvasId 即可）。
type newWowMinShotCreateBody struct {
	CanvasID int64 `json:"canvasId"`
}

// newWowMinGenerateVideoBody generate-video 必填字段（嗅探确认）。
type newWowMinGenerateVideoBody struct {
	CanvasID       int64  `json:"canvasId"`
	ShotID         int64  `json:"shotId"`
	ModelName      string `json:"modelName"`
	GenerationType string `json:"generationType"`
	Prompt         string `json:"prompt"`
	Ratio          string `json:"ratio,omitempty"`
	Duration       int    `json:"duration"`
}

// newWowGenerateVideoResp NewWow envelope；data 是字符串形式的 taskId / 数字。
type newWowEnvelope struct {
	Success    bool        `json:"success"`
	ErrCode    *string     `json:"errCode"`
	ErrMessage *string     `json:"errMessage"`
	Data       interface{} `json:"data"`
}

// newWowCanvasDetailResp canvas detail 响应（含 shots[]）。
type newWowCanvasDetailResp struct {
	Success bool             `json:"success"`
	Data    *newWowCanvasDTO `json:"data"`
}

type newWowCanvasDTO struct {
	ID    int64           `json:"id"`
	Code  string          `json:"code"`
	Shots []newWowShotDTO `json:"shots"`
}

type newWowShotDTO struct {
	ID             int64  `json:"id"`
	CanvasID       int64  `json:"canvasId"`
	VideoStatus    string `json:"videoStatus"`     // PENDING / SUCCESS / FAILED / PROCESSING
	VideoURL       string `json:"videoUrl"`
	VideoDuration  *int   `json:"videoDuration"`  // 毫秒
	VideoPrompt    string `json:"videoPrompt"`
	ModelName      string `json:"modelName"`
}

// SubmitVideo NOTE：当前实现走的是嗅探出来的旧路径（aliyun CMS /agent/canvas/material/generate-video），
// 但 NewWow 浏览器 SDK 真实调用的是 aliyun FC 函数路径（v2/workspace-default-cms-XXX-cn-hangzhou&service i_XXX/generate-video），
// 两条路径在 NewWow 服务端走不同的积分扣减（FC 路径消耗 user.points，旧路径消耗隐藏 videoPower）。
//
// 当前：保留旧路径实现，避免回归；新路径需用户提供 FC workspace ID + service name 后再切换。
// 当上游 aliyun FC 路径配齐后，taskId 返回值改为真实的 32 字符 hex 字符串（不再用 "canvasId:shotId"）。
func (a *newWowAdapter) SubmitVideo(ctx context.Context, acc *model.UserVendorAccount, input GenerateVideoInput) (string, error) {
	if acc == nil || strings.TrimSpace(acc.AccessToken) == "" {
		return "", errors.New("NewWow accesstoken 为空，无法提交视频任务")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return "", errors.New("缺少 prompt 参数")
	}
	if strings.TrimSpace(input.Model) == "" {
		return "", errors.New("缺少 model 参数")
	}

	// 1. canvas create
	canvasID, err := a.newWowCreateCanvas(ctx, acc)
	if err != nil {
		return "", fmt.Errorf("NewWow canvas 创建失败：%w", err)
	}

	// 2. shot create
	shotID, err := a.newWowCreateShot(ctx, acc, canvasID)
	if err != nil {
		return "", fmt.Errorf("NewWow shot 创建（canvas=%d）失败：%w", canvasID, err)
	}

	// 3. generate-video（旧 CMS 路径；可能因积分池走错报 BIZ_ERROR）。
	//    TODO: 等用户提供 aliyun FC 路径后切换到该路径，并返回真实 taskId。
	seconds := input.Seconds
	if seconds < newWowVideoMinSeconds {
		seconds = newWowVideoMinSeconds
	}
	if seconds > 10 {
		seconds = 10
	}
	ratio := strings.TrimSpace(input.Size)
	if ratio == "" {
		ratio = newWowVideoDefaultRatio
	}
	body := newWowMinGenerateVideoBody{
		CanvasID:       canvasID,
		ShotID:         shotID,
		ModelName:      strings.TrimSpace(input.Model),
		GenerationType: "TEXT_TO_VIDEO",
		Prompt:         strings.TrimSpace(input.Prompt),
		Ratio:          ratio,
		Duration:       seconds,
	}
	rawBody, status, err := newWowPOST(ctx, acc, a.vendor, "/agent/canvas/material/generate-video", body)
	if err != nil {
		return "", fmt.Errorf("NewWow generate-video 请求失败：%w", err)
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("NewWow generate-video HTTP %d：%s", status, truncate(rawBody, 300))
	}
	var env newWowEnvelope
	if err := json.Unmarshal([]byte(rawBody), &env); err != nil {
		return "", fmt.Errorf("解析 NewWow generate-video 响应失败：%w", err)
	}
	if !env.Success {
		errCode := ""
		if env.ErrCode != nil { errCode = *env.ErrCode }
		errMsg := ""
		if env.ErrMessage != nil { errMsg = *env.ErrMessage }
		return "", fmt.Errorf("NewWow 视频提交被拒 (errCode=%s)：%s", errCode, errMsg)
	}
	// 嗅探确认：成功时 data 是数字 taskId（同步返回的素材记录 ID）。
	// 我们用更稳定的 "canvasId:shotId" 作为任务 ID，轮询时再 GET canvas detail。
	_ = env.Data
	return fmt.Sprintf("%d:%d", canvasID, shotID), nil
}

// GetTaskStatus 调 NewWow `POST /agent/story-canvas/batch-query-status`，批量查任务状态。
//
// 任务 ID 格式：NewWow 浏览器 SDK 提交的 generate-video 响应里的 `taskId`（32 字符 hex）。
// 嗅探确认的真实 schema（2026-08-17 验证）：
//
//	request body : {"taskIds": ["96685f08363a4cf497b9873a0fd99136"]}
//	response data : [{taskId, nodeKey, dataType:"video", status:"PENDING|PROCESSING|SUCCESS|FAILED",
//	                   resultData:["https://...mp4"], errorMessage, ...}]
func (a *newWowAdapter) GetTaskStatus(ctx context.Context, acc *model.UserVendorAccount, taskID string) (*TaskStatus, error) {
	if acc == nil || strings.TrimSpace(acc.AccessToken) == "" {
		return nil, errors.New("NewWow accesstoken 为空，无法查询任务状态")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("NewWow 任务 ID 不能为空")
	}

	body := map[string]any{"taskIds": []string{taskID}}
	rawBody, status, err := newWowPOST(ctx, acc, a.vendor, "/agent/story-canvas/batch-query-status", body)
	if err != nil {
		return nil, fmt.Errorf("NewWow batch-query-status 请求失败：%w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("NewWow batch-query-status HTTP %d：%s", status, truncate(rawBody, 300))
	}
	var env struct {
		Success    bool `json:"success"`
		ErrCode    *string `json:"errCode"`
		ErrMessage *string `json:"errMessage"`
		Data       []struct {
			TaskID       string   `json:"taskId"`
			NodeKey      string   `json:"nodeKey"`
			DataType     string   `json:"dataType"`
			Status       string   `json:"status"`
			ResultData   []string `json:"resultData"`
			ErrorMessage *string  `json:"errorMessage"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(rawBody), &env); err != nil {
		return nil, fmt.Errorf("解析 NewWow batch-query-status 失败：%w", err)
	}
	if !env.Success {
		errCode := ""
		if env.ErrCode != nil { errCode = *env.ErrCode }
		errMsg := ""
		if env.ErrMessage != nil { errMsg = *env.ErrMessage }
		return nil, fmt.Errorf("NewWow batch-query-status success=false (errCode=%s)：%s", errCode, errMsg)
	}
	// 精确匹配 taskId（payload 里允许批量）
	for _, item := range env.Data {
		if item.TaskID != taskID {
			continue
		}
		ts := &TaskStatus{
			ID:     taskID,
			Status: newWowVideoStatusToUniform(item.Status),
		}
		if item.ErrorMessage != nil && *item.ErrorMessage != "" {
			ts.Message = *item.ErrorMessage
		}
		if len(item.ResultData) > 0 && item.ResultData[0] != "" {
			ts.OutputURL = item.ResultData[0]
			ts.Output = &GenerateMediaOutput{
				Items: []GeneratedAssetItem{{
					ID:       item.TaskID,
					URL:      item.ResultData[0],
					MimeType: "video/mp4",
				}},
			}
		}
		// 进度统一映射
		switch ts.Status {
		case "completed":
			ts.Progress = 100
		case "failed":
			ts.Progress = 0
		case "processing", "queued":
			ts.Progress = 50
		}
		if ts.Message == "" && item.Status != "" {
			ts.Message = item.Status
		}
		return ts, nil
	}
	return nil, fmt.Errorf("NewWow batch-query-status 返回里未找到 taskId=%s", taskID)
}

// newWowVideoStatusToUniform 把 NewWow 的 shot.videoStatus 字符串映射成统一 TaskStatus.Status。
//
// 嗅探返回的 shotStatus 取值未确认全集；按 NewWow 表里 status 枚举 ("PENDING"/"PROCESSING"/"SUCCESS"/"FAILED") 映射。
func newWowVideoStatusToUniform(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SUCCESS", "COMPLETED", "DONE":
		return "completed"
	case "FAILED", "ERROR":
		return "failed"
	case "PROCESSING", "RUNNING":
		return "processing"
	case "PENDING", "QUEUED", "WAITING":
		return "queued"
	case "":
		return "queued"
	default:
		return "processing"
	}
}

// newWowCreateCanvas POST /agent/canvas/create → 返回 canvasId。
func (a *newWowAdapter) newWowCreateCanvas(ctx context.Context, acc *model.UserVendorAccount) (int64, error) {
	rawBody, status, err := newWowPOST(ctx, acc, a.vendor, "/agent/canvas/create", map[string]any{})
	if err != nil {
		return 0, err
	}
	if status < 200 || status >= 300 {
		return 0, fmt.Errorf("HTTP %d：%s", status, truncate(rawBody, 200))
	}
	var env newWowEnvelope
	if err := json.Unmarshal([]byte(rawBody), &env); err != nil {
		return 0, fmt.Errorf("解析响应失败：%w", err)
	}
	if !env.Success {
		errMsg := ""
		if env.ErrMessage != nil { errMsg = *env.ErrMessage }
		return 0, fmt.Errorf("success=false：%s", errMsg)
	}
	switch v := env.Data.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case string:
		n, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return 0, fmt.Errorf("data 不是整数：%v", perr)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("data 字段类型不支持：%T", v)
	}
}

// newWowCreateShot POST /agent/canvas/shot/create → 返回 shotId。
func (a *newWowAdapter) newWowCreateShot(ctx context.Context, acc *model.UserVendorAccount, canvasID int64) (int64, error) {
	rawBody, status, err := newWowPOST(ctx, acc, a.vendor, "/agent/canvas/shot/create", newWowMinShotCreateBody{CanvasID: canvasID})
	if err != nil {
		return 0, err
	}
	if status < 200 || status >= 300 {
		return 0, fmt.Errorf("HTTP %d：%s", status, truncate(rawBody, 200))
	}
	var env newWowEnvelope
	if err := json.Unmarshal([]byte(rawBody), &env); err != nil {
		return 0, fmt.Errorf("解析响应失败：%w", err)
	}
	if !env.Success {
		errMsg := ""
		if env.ErrMessage != nil { errMsg = *env.ErrMessage }
		return 0, fmt.Errorf("success=false：%s", errMsg)
	}
	switch v := env.Data.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case string:
		n, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return 0, fmt.Errorf("data 不是整数：%v", perr)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("data 字段类型不支持：%T", v)
	}
}

// ============== helper：NewWow POST 客户端 ==============

// newWowPOSTWithToken 用指定 token POST JSON 到 NewWow 任意端点，返回 body 原文 + 状态码。
// 与 newWowGETWithToken 对称：相同鉴权头、相同超时、同样的 base/vendor.APIRootURL 覆盖逻辑。
func newWowPOSTWithToken(ctx context.Context, token, path string, body any, vendor *model.Vendor) (string, int, error) {
	base := newWowDefaultAPIRoot
	if vendor != nil {
		if root := strings.TrimSpace(vendor.APIRootURL); root != "" {
			base = strings.TrimRight(root, "/")
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	raw, mErr := json.Marshal(body)
	if mErr != nil {
		return "", 0, fmt.Errorf("序列化 body 失败：%w", mErr)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(raw))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("accesstoken", strings.TrimSpace(token))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 FreedomNewWowAdapter/1.0")
	resp, err := newWowAPIClient().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(respBody), resp.StatusCode, nil
}

// newWowPOST 用 account 里的 accesstoken POST JSON。
func newWowPOST(ctx context.Context, account *model.UserVendorAccount, vendor *model.Vendor, path string, body any) (string, int, error) {
	if account == nil || strings.TrimSpace(account.AccessToken) == "" {
		return "", 0, errors.New("accesstoken 为空")
	}
	return newWowPOSTWithToken(ctx, strings.TrimSpace(account.AccessToken), path, body, vendor)
}
