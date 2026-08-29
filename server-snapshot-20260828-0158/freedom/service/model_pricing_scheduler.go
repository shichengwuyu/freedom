package service

import (
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// 自动定价定时拉取调度器（2026-08-21 引入）。
//
// 每天从 PricingURL 拉取一次上游模型定价（人民币元），按「视频 +50%、图片 +20%」的加价率
// 换算成人民币分，合并写回 settings.public.modelChannel.modelCosts，实现全自动定价：
//  - 前端下拉框价格展示、后台价格列表、扣费逻辑都直接读 modelCosts，无需再手工设置定价；
//  - pricing 里出现的模型（rolldek 图片/视频）总是覆盖为自动价，其它渠道模型定价原样保留；
//  - 拉取/写入失败仅打日志，不影响已写入的价格。
const (
	// pricingCron 自动定价拉取频率：每小时整点一次（上游定价会实时变动，缩短间隔让新增/改价模型更快生效）
	pricingCron        = "0 * * * *"
	pricingHTTPTimeout = 10 * time.Second

	// pricingImageMarkup 图片加价率（+20%）
	pricingImageMarkup = 1.2
	// pricingVideoMarkup 视频加价率（+50%）
	pricingVideoMarkup = 1.5
)

// upstreamPricingResp 上游定价接口返回的原始结构（只取用到的字段）。
type upstreamPricingResp struct {
	Data []struct {
		ModelName   string  `json:"model_name"`
		ModelPrice  float64 `json:"model_price"` // 人民币单价（元）：per_call=每次 / per_second=每秒
		BillingMode string  `json:"billing_mode"` // per_call | per_second
	} `json:"data"`
}

var (
	pricingCronInst *cron.Cron
	pricingOnce     sync.Once
)

// StartModelPricingScheduler 启动自动定价定时拉取调度器（幂等，进程内只启动一次），
// 启动后立即执行一次，避免首次请求拿不到自动价格。
func StartModelPricingScheduler() {
	pricingOnce.Do(func() {
		pricingCronInst = cron.New()
		if _, err := pricingCronInst.AddFunc(pricingCron, runAutoPricingRefresh); err != nil {
			log.Printf("add auto pricing cron failed err=%v", err)
			return
		}
		pricingCronInst.Start()
	})
	// 不依赖 PricingURL：启动时立即为 sd-* 简写模型合成 Seedance 价格一次。
	// 即使未配置上游定价（PricingURL 为空），也能让 sd-* 拥有正确价格、不再显示"免费/被隐藏"。
	synthesizeSDCostsNow()
	runAutoPricingRefresh()
}

// synthesizeSDCostsNow 独立于自动定价：读取当前 settings，为渠道里 sd-* 简写模型按核心签名
// 匹配上游/已有 seedance-* 价格并合成写回。无论 PricingURL 是否配置都会执行一次（进程启动时）。
func synthesizeSDCostsNow() {
	settings, err := repository.GetSettings()
	if err != nil {
		log.Printf("[WARN] synthesize sd-* costs read settings failed err=%v", err)
		return
	}
	// 已有价来源：当前 settings 里的 seedance-*（上游全名），以及上游若已配置则一并参与。
	base := append([]model.ModelCost{}, settings.Public.ModelChannel.ModelCosts...)
	if config.Cfg.PricingURL != "" {
		if up, err := fetchPricing(config.Cfg.PricingURL); err == nil {
			base = append(base, up...)
		}
	}
	synthesized := synthesizeSDCosts(settings, base)
	if len(synthesized) == 0 {
		return
	}
	// 仅追加尚未存在的 sd-* 记录，避免重复。
	have := map[string]bool{}
	for _, c := range settings.Public.ModelChannel.ModelCosts {
		have[strings.TrimSpace(c.Model)] = true
	}
	added := 0
	for _, c := range synthesized {
		if have[strings.TrimSpace(c.Model)] {
			continue
		}
		settings.Public.ModelChannel.ModelCosts = append(settings.Public.ModelChannel.ModelCosts, c)
		have[strings.TrimSpace(c.Model)] = true
		added++
	}
	if added == 0 {
		return
	}
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		log.Printf("[ERROR] synthesize sd-* costs save settings failed err=%v", err)
		return
	}
	log.Printf("synthesize sd-* costs done added=%d: %v", added, modelNames(synthesized))
}

// runAutoPricingRefresh 执行一次拉取并合并写回 modelCosts。
func runAutoPricingRefresh() {
	if config.Cfg.PricingURL == "" {
		log.Printf("[WARN] PricingURL 未配置，自动定价已禁用（modelCosts 不变，生成可能按 CostCents:0 免费）")
		return
	}
	costs, err := fetchPricing(config.Cfg.PricingURL)
	if err != nil {
		log.Printf("[ERROR] refresh auto pricing failed url=%s err=%v（modelCosts 不变，生成可能免费）", config.Cfg.PricingURL, err)
		return
	}
	if len(costs) == 0 {
		log.Printf("[ERROR] refresh auto pricing got 0 models from upstream url=%s（上游无数据或分类全部未识别；modelCosts 不变，生成可能免费）", config.Cfg.PricingURL)
		return
	}
	settings, err := repository.GetSettings()
	if err != nil {
		log.Printf("refresh auto pricing read settings failed err=%v", err)
		return
	}
	// 为渠道简写 sd-* 模型合成 Seedance 价格（上游定价用 seedance-* 全名，按核心签名匹配）。
	// 否则 sd-* 永远匹配不到上游价 → CostCents:0 → 前端显示"免费"且被隐藏。
	if synthesized := synthesizeSDCosts(settings, costs); len(synthesized) > 0 {
		costs = append(costs, synthesized...)
		log.Printf("refresh auto pricing synthesized sd-* alias costs count=%d: %v", len(synthesized), modelNames(synthesized))
	}
	// 自动定价覆盖集合：pricing 里能识别分类的模型名 → 用自动价覆盖
	autoSet := map[string]bool{}
	for _, c := range costs {
		autoSet[c.Model] = true
	}
	// 保留手工定价；只清理"曾经由本调度器自动定价、但本次上游已下架"的模型，
	// 避免误删 agnes 等非 rolldek 渠道的手工价（误删会让模型变成 CostCents:0 免费）。
	kept := []model.ModelCost{}
	// 按模型名索引旧的自动价条目，用于在自动覆盖时继承手工配置（别名/能力开关）
	oldByModel := map[string]model.ModelCost{}
	cleanedCount := 0
	for _, old := range settings.Public.ModelChannel.ModelCosts {
		m := strings.TrimSpace(old.Model)
		if autoSet[m] {
			// 上游仍在售 → 自动覆盖（后面合并时会处理）
			oldByModel[m] = old
			continue
		}
		if old.AutoPriced {
			// 曾自动定价、现已上游下架 → 清理（价格失效，避免继续按旧自动价售卖）
			cleanedCount++
			continue
		}
		// 手工定价 / 非 rolldek 渠道 → 原样保留
		kept = append(kept, old)
	}
	// 自动价条目上继承旧条目的别名与参考/音频能力开关，避免每天覆盖时把手工配置清空
	merged := make([]model.ModelCost, 0, len(costs))
	for _, c := range costs {
		if old, ok := oldByModel[c.Model]; ok {
			c.Label = old.Label
			c.RefVideo = old.RefVideo
			c.RefAudio = old.RefAudio
			c.GenAudio = old.GenAudio
		}
		merged = append(merged, c)
	}
	settings.Public.ModelChannel.ModelCosts = append(kept, merged...)
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		log.Printf("refresh auto pricing save settings failed err=%v", err)
		return
	}
	log.Printf("refresh auto pricing done count=%d cleaned=%d", len(costs), cleanedCount)
}

// fetchPricing 请求上游定价接口并计算成人民币分（model.ModelCost 列表）。
func fetchPricing(url string) ([]model.ModelCost, error) {
	client := &http.Client{Timeout: pricingHTTPTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// 加浏览器风格的 User-Agent 降低上游风控概率（与模型状态任务保持一致）
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw upstreamPricingResp
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	costs := make([]model.ModelCost, 0, len(raw.Data))
	// 按 model_name 去重：上游同一模型可能返回多行且价格不同
	// （如 seedance-2.0-fast-431-480p 同时出现 1.5 与 2.5 两档），保留较高价避免少收；
	// 同时标记 AutoPriced=true，供后续只清理"自动价"、保留手工价。
	byName := make(map[string]*model.ModelCost, len(raw.Data))
	for i := range raw.Data {
		m := raw.Data[i]
		// 归一化模型名：上游偶发首尾空白/脏字符，若不清理会让存储的 model 与请求用的干净名
		// 精确匹配不上（service.ModelCost 是 == 匹配），导致该模型变成 CostCents:0 免费；
		// 同时 TrimSpace 也是同名去重能正确生效的前提。
		m.ModelName = strings.TrimSpace(m.ModelName)
		if m.ModelName == "" {
			continue
		}
		category := classifyPricingModel(m.ModelName, m.BillingMode)
		if category == "" {
			// 无法识别的模型（目前 pricing 里没有文本模型）不自动定价，保持管理员手工配置
			continue
		}
		cents := pricingCostCents(m.ModelPrice, category)
		item := model.ModelCost{
			Model:      m.ModelName,
			Unit:       m.BillingMode,
			AutoPriced: true,
		}
		if m.BillingMode == model.ModelCostUnitPerSecond {
			item.CostCentsPerSecond = cents
		} else {
			item.CostCents = cents
		}
		if prev, ok := byName[m.ModelName]; ok {
			// 同名冲突：同一计费单位下保留总价更高的那一档（防止少收）
			var prevVal, newVal int
			if m.BillingMode == model.ModelCostUnitPerSecond {
				prevVal, newVal = prev.CostCentsPerSecond, item.CostCentsPerSecond
			} else {
				prevVal, newVal = prev.CostCents, item.CostCents
			}
			if newVal > prevVal {
				byName[m.ModelName] = &item
			}
			continue
		}
		byName[m.ModelName] = &item
	}
	for _, v := range byName {
		costs = append(costs, *v)
	}
	return costs, nil
}

// classifyPricingModel 判断 pricing 里某模型是图片还是视频（返回 "image" / "video"；"" 表示不识别，跳过）。
// 优先按计费模式（per_second 必然是视频），再按模型名关键词兜底。
func classifyPricingModel(modelName, billingMode string) string {
	n := strings.ToLower(strings.TrimSpace(modelName))
	// 按秒计费的必然是视频（kling / seedance-2.5 / sd-2.5 等）
	if billingMode == model.ModelCostUnitPerSecond {
		return "video"
	}
	// 按次计费的视频：seedance-2.0*、sd-2.0*（Stable Video Diffusion）
	if isVideoModelName(n) || strings.Contains(n, "sd-") {
		return "video"
	}
	// 图片模型：gpt-image / gemini-3*-image-preview 等
	if isImageModelName(n) {
		return "image"
	}
	return ""
}

// pricingCostCents 按分类加价率把上游人民币单价（元）换算成分（1 元 = 100 分），四舍五入。
// 视频加价 50%、图片加价 20%，保证利润率。
func pricingCostCents(yuan float64, category string) int {
	markup := pricingImageMarkup
	if category == "video" {
		markup = pricingVideoMarkup
	}
	return int(math.Round(yuan * 100 * markup))
}

// ─────────────────────────────────────────────────────────────────────────────
// sd-* 别名合成：把渠道简写 sd-* 映射到上游 seedance-* 全名价格
// ─────────────────────────────────────────────────────────────────────────────

// synthesizeSDCosts 为渠道里所有 sd-* 简写模型，按"核心签名"匹配上游 seedance-* 全名价格，
// 合成一条带正确价格的 ModelCost。渠道模型名（如 sd-2.0-fast-720p）与上游名（seedance-2.0-fast-431-720p）
// 仅差一段纯数字渠道码（-431-），因此先精确匹配核心签名、失败再去掉数字段匹配。
//
// 设计约束：合成记录的 RefVideo/RefAudio/GenAudio 保持 nil（指针零值），让前端白名单
// （video-model-capabilities.ts 里 sd-* 即 Seedance 的能力推断）继续生效，不会被显式 false 覆盖。
func synthesizeSDCosts(settings model.Settings, costs []model.ModelCost) []model.ModelCost {
	// 1. 收集已在 upstream 有价的模型（精确名 + 已去掉数字段的核心签名），避免重复合成
	exactHas := map[string]bool{}
	sigHas := map[string]bool{}
	for _, c := range costs {
		exactHas[strings.TrimSpace(c.Model)] = true
		sigHas[coreSignature(strings.TrimSpace(c.Model))] = true
	}
	// 2. 收集渠道里所有模型名（公开 + 私有），定位 sd-* 简写
	channelModels := map[string]bool{}
	for _, ch := range settings.Public.ModelChannel.Channels {
		for _, m := range ch.Models {
			channelModels[strings.TrimSpace(m)] = true
		}
	}
	for _, ch := range settings.Private.Channels {
		for _, m := range ch.Models {
			channelModels[strings.TrimSpace(m)] = true
		}
	}
	out := make([]model.ModelCost, 0)
	for m := range channelModels {
		n := strings.ToLower(m)
		if !strings.HasPrefix(n, "sd-") {
			continue
		}
		if exactHas[m] {
			// 已经在上游价表里（不常见，但保险），不重复合成
			continue
		}
		sig := coreSignature(n)
		if sig == "" || !sigHas[sig] {
			// 上游没有任何 seedance 同签名模型 → 没法合成，跳过（维持现状，由人工补价）
			continue
		}
		out = append(out, model.ModelCost{
			Model:      m,
			AutoPriced: true,
			// Unit/CostCents/CostCentsPerSecond 留零值，由下方"继承旧条目/按 upstream 名匹配"步骤填充
		})
	}
	// 3. 为每个合成记录填充价格：优先继承对应 sd-* 旧 modelCosts 条目；
	//    否则按核心签名从 costs 里找上游 seedance 同价条目填充。
	oldByModel := map[string]model.ModelCost{}
	for _, old := range settings.Public.ModelChannel.ModelCosts {
		oldByModel[strings.TrimSpace(old.Model)] = old
	}
	// 按核心签名索引上游价，供找不到旧条目时回退
	sigCost := map[string]model.ModelCost{}
	for _, c := range costs {
		s := coreSignature(strings.TrimSpace(c.Model))
		if s != "" && s != strings.TrimSpace(c.Model) {
			sigCost[s] = c // 跳过精确等于签名的（那是上游名本身），只存"带数字段的长名"
		}
	}
	for i := range out {
		m := out[i].Model
		finalModel := out[i]
		if old, ok := oldByModel[m]; ok {
			finalModel.Unit = old.Unit
			finalModel.CostCents = old.CostCents
			finalModel.CostCentsPerSecond = old.CostCentsPerSecond
		} else if up, ok := sigCost[coreSignature(strings.ToLower(m))]; ok {
			finalModel.Unit = up.Unit
			finalModel.CostCents = up.CostCents
			finalModel.CostCentsPerSecond = up.CostCentsPerSecond
		}
		out[i] = finalModel
	}
	return out
}

// coreSignature 把模型名归一化为"去数字段前缀"的核心签名，用于 sd-* 与 seedance-* 的宽松匹配。
// 例：sd-2.0-fast-720p → sd-2-0-fast-720p（内部点/下划线转横线，保持数字）；
//     seedance-2.0-fast-431-720p → seedance-2-0-fast-431-720p；
//     去掉纯数字段后两者核心签名均为 seedance-2-0-fast-720p → 可匹配。
func coreSignature(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "_", "-")
	n = strings.ReplaceAll(n, ".", "-")
	if n == "" {
		return ""
	}
	// 去掉所有独立的纯数字段（保留含字母的，如 2k/4k），用于"附带渠道码"的宽松匹配。
	parts := strings.Split(n, "-")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if isAllDigits(p) {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "-")
}

func modelNames(costs []model.ModelCost) []string {
	names := make([]string, 0, len(costs))
	for _, c := range costs {
		names = append(names, c.Model)
	}
	return names
}
