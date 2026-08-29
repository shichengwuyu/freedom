package service

import (
	"strings"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// PricingGroup 公开定价 API 的 group 信息（Sprint 3 引入）。
type PricingGroup struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	Ratio       float64 `json:"ratio"`
	IsDefault   bool    `json:"isDefault,omitempty"`
}

// PricingModelCell 单 model 在某 group 下的价格。
type PricingModelCell struct {
	Group     string `json:"group"`     // group ID
	UnitCents int    `json:"unitCents"` // 该 group 实际单位价
	Discount  string `json:"discount,omitempty"` // 折扣文案："" / "20% off" / "10% off"
}

// PricingModel 公开定价 API 的 model 信息。
type PricingModel struct {
	Model      string             `json:"model"`
	Label      string             `json:"label,omitempty"`
	Capability string             `json:"capability,omitempty"` // 来自 PublicModelChannelInfo.AvailableModels 推断（best-effort）
	Unit       string             `json:"unit"` // "per_call" / "per_second"
	BaseCents  int                `json:"baseCents"`
	BasePerSec int                `json:"basePerSec,omitempty"`
	Groups     []PricingModelCell `json:"groups"`
}

// GetGroupRatio 返回 groupID 的统一倍率（0~1；缺省=1.0）。
// 优先级：settings.private.groupRatios[groupID] > 1.0（fallback 安全默认）
func GetGroupRatio(groupID string) float64 {
	if strings.TrimSpace(groupID) == "" {
		return 1.0
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return 1.0
	}
	ratios := settings.Private.GroupRatios
	if r, ok := ratios[groupID]; ok && r > 0 && r <= 1.0 {
		return r
	}
	return 1.0
}

// CalcUnitCostCents 计算指定 model 在指定 group 的"每单位"实际成本（cents）。
// units 由调用方单独乘（per_call 乘 count，per_second 乘 seconds * count）。
//
// 公式：baseUnit * groupRatio * modelGroupRatio，向下取整
func CalcUnitCostCents(modelName, groupID string) (int, error) {
	base, err := ModelCost(modelName)
	if err != nil {
		return 0, err
	}
	var unitCents int
	if base.Unit == model.ModelCostUnitPerSecond && base.CostCentsPerSecond > 0 {
		unitCents = base.CostCentsPerSecond
	} else {
		unitCents = base.CostCents
	}
	if unitCents <= 0 {
		// 兜底：与现有一致（cents<=0 拒收），调用方原本就会 400
		return unitCents, nil
	}
	groupRatio := GetGroupRatio(groupID)
	modelGroupRatio := base.GetGroupPricingRatio(groupID)
	actual := float64(unitCents) * groupRatio * modelGroupRatio
	return int(actual), nil // 向下取整（Go int() truncation）
}

// ListPublicPricing 返回公开定价页所需的全部数据（所有 model × 所有 active group）。
// 用途：handler.GetPricing 调用
func ListPublicPricing() ([]PricingModel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	normalized := normalizePublicSetting(settings.Public)
	groups, err := ListActiveUserGroups()
	if err != nil {
		return nil, err
	}
	models := normalized.ModelChannel.ModelCosts
	out := make([]PricingModel, 0, len(models))
	for _, m := range models {
		// 推断 capability：availableModels + 当前 model 名 → 简单按 known models 推断
		// Sprint 3 这里用"模型名 fallback"即可；后续 Sprint 4 改能力检测
		capability := inferCapabilityFromModel(m.Model)
		baseUnit := m.CostCents
		basePerSec := m.CostCentsPerSecond
		unit := m.Unit
		if unit == "" {
			unit = model.ModelCostUnitPerCall
		}

		cells := make([]PricingModelCell, 0, len(groups))
		for _, g := range groups {
			var cents int
			if unit == model.ModelCostUnitPerSecond && basePerSec > 0 {
				cents = int(float64(basePerSec) * GetGroupRatio(g.ID) * m.GetGroupPricingRatio(g.ID))
			} else {
				cents = int(float64(baseUnit) * GetGroupRatio(g.ID) * m.GetGroupPricingRatio(g.ID))
			}
			cells = append(cells, PricingModelCell{
				Group:     g.ID,
				UnitCents: cents,
				Discount:  formatDiscount(GetGroupRatio(g.ID), m.GetGroupPricingRatio(g.ID)),
			})
		}

		out = append(out, PricingModel{
			Model:      m.Model,
			Label:      m.Label,
			Capability: capability,
			Unit:       unit,
			BaseCents:  baseUnit,
			BasePerSec: basePerSec,
			Groups:     cells,
		})
	}
	return out, nil
}

// inferCapabilityFromModel best-effort 推断 model 属于哪种能力（text/image/video/audio）。
// 简单按模型名前缀 / 关键词匹配；Sprint 4 改用 ModelChannel 显式 capability 字段。
func inferCapabilityFromModel(modelName string) string {
	n := strings.ToLower(modelName)
	switch {
	case strings.Contains(n, "video"), strings.Contains(n, "veo"), strings.Contains(n, "sora"), strings.Contains(n, "kling"), strings.Contains(n, "runway"), strings.Contains(n, "seedance"), strings.Contains(n, "hailuo"), strings.Contains(n, "wan"):
		return "video"
	case strings.Contains(n, "image"), strings.Contains(n, "imagen"), strings.Contains(n, "dalle"), strings.Contains(n, "flux"), strings.Contains(n, "midjourney"), strings.Contains(n, "nano-banana"), strings.Contains(n, "gpt-image"), strings.Contains(n, "sd3"), strings.Contains(n, "sdxl"):
		return "image"
	case strings.Contains(n, "audio"), strings.Contains(n, "tts"), strings.Contains(n, "speech"), strings.Contains(n, "mimo"), strings.Contains(n, "whisper"), strings.Contains(n, "music"):
		return "audio"
	case strings.Contains(n, "embedding"), strings.Contains(n, "embed"), strings.Contains(n, "rerank"):
		return "embedding"
	default:
		return "text"
	}
}

// formatDiscount 格式化折扣文案。base=1.0 → ""；<1.0 → "20% off" 等。
// groupRatio * modelGroupRatio 才是最终倍率（两层都乘进去）。
func formatDiscount(groupRatio, modelGroupRatio float64) string {
	final := groupRatio * modelGroupRatio
	if final >= 0.999 {
		return ""
	}
	off := int((1.0 - final) * 100)
	if off <= 0 {
		return ""
	}
	return formatPercent(off) + " off"
}

func formatPercent(n int) string {
	if n%10 == 0 {
		return intToStr(n) + "%"
	}
	return intToStr(n) + "%"
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
