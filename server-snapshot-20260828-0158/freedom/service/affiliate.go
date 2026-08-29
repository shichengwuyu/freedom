package service

import (
	"math"
	"strconv"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// AffiliateRateForCount 按邀请人当前邀请人数计算阶梯返佣比例。
// 规则（默认）：1人=5%，每多1人+1%，封顶10%。
//   - count <= 0：无邀请人资格，返回 0
//   - rate = min(baseRate + (count-1)*stepRate, maxRate)
func AffiliateRateForCount(count int, aff model.AffiliateSetting) float64 {
	if count <= 0 || !aff.Enabled {
		return 0
	}
	rate := aff.BaseRate + float64(count-1)*aff.StepRate
	if rate > aff.MaxRate {
		rate = aff.MaxRate
	}
	if rate < 0 {
		rate = 0
	}
	return rate
}

// SettleCommissionOnConsume 被邀请人每次模型消费后，给邀请人记一笔阶梯返佣（一级直推，待结算）。
//
// 触发点：ConsumeUserBalanceWithHold（所有生图/视频/音频消费扣款）成功后调用。
// 规则：
//   - 消费用户（invitee）必须有 inviter_id，否则跳过；
//   - 只结算给直接邀请人，不向上追溯二级（防多级分销）；
//   - 比例 = 邀请人当前邀请人数对应的阶梯比例（AffiliateRateForCount），封顶 MaxRate；
//   - 佣金 = 本次消费额 × 比例；
//   - 幂等：同一 consumeID（消费占用 hold.ID）重复调用不重复记（UNIQUE 索引 + 先查后写）；
//   - 低于 minSettleCents 阈值则跳过（如返佣 < 1 分不记）；
//   - 关键点：本函数只写入 status=pending 的返佣流水，**不直接给邀请人入账**。余额入账
//     由每日批结算调度器 StartAffiliateSettlementScheduler 统一处理（T+1 日结，降低抖动与争议退款风险）。
//
// 失败不影响主消费流程（仅 log）。
func SettleCommissionOnConsume(inviteeID, consumeID string, consumeCents int) error {
	if consumeCents <= 0 || consumeID == "" {
		return nil
	}

	// 幂等：已记过直接返回
	if _, ok, err := repository.GetAffCommissionLogByRecharge(consumeID); err != nil {
		return err
	} else if ok {
		return nil
	}

	settings, err := repository.GetSettings()
	if err != nil {
		return err
	}
	aff := normalizeSettings(settings).Private.Affiliate
	if !aff.Enabled {
		return nil
	}

	invitee, ok, err := repository.GetUserByID(inviteeID)
	if err != nil {
		return err
	}
	if !ok || invitee.InviterID == "" {
		return nil
	}
	// 防自邀
	if invitee.InviterID == invitee.ID {
		return nil
	}

	// 按邀请人当前邀请人数算阶梯比例
	rate := AffiliateRateForCount(invitee.AffCount, aff)
	if rate <= 0 {
		return nil
	}
	commission := int(math.Round(float64(consumeCents) * rate))
	if commission <= 0 {
		return nil
	}
	if commission < aff.MinSettleCents {
		return nil
	}

	inviter, ok, err := repository.GetUserByID(invitee.InviterID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	rateStr := strconv.FormatFloat(rate, 'f', 4, 64)
	tNow := now()

	// 仅写 pending 流水，等待每日批结算入账
	log := model.AffCommissionLog{
		ID:              newID("aff"),
		InviterID:       inviter.ID,
		InviteeID:       invitee.ID,
		RechargeID:      consumeID,
		RechargeCents:   consumeCents,
		Rate:            rateStr,
		CommissionCents: commission,
		Status:          model.AffCommissionStatusPending,
		SettledAt:       "",
		CreatedAt:       tNow,
	}
	if _, err := repository.SaveAffCommissionLog(log); err != nil {
		return err
	}
	return nil
}

// MyAffiliateInfo 返回当前用户的邀请信息：自己的邀请码、邀请人数、累计佣金（分）、当前等级比例。
// 用于前端「我的邀请」页展示。
func MyAffiliateInfo(userID string) (map[string]any, error) {
	user, ok, err := repository.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, safeMessageError{message: "用户不存在"}
	}
	totalCents, count, err := repository.SumAffCommissionByInviter(user.ID)
	if err != nil {
		return nil, err
	}
	pendingCents, pendingCount, err := repository.SumAffCommissionPendingByInviter(user.ID)
	if err != nil {
		return nil, err
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	aff := normalizeSettings(settings).Private.Affiliate
	currentRate := AffiliateRateForCount(user.AffCount, aff)
	nextRate := AffiliateRateForCount(user.AffCount+1, aff)
	return map[string]any{
		"affCode":              user.AffCode,
		"inviterId":            user.InviterID,
		"affCount":             user.AffCount,
		"totalCommissionCents": totalCents,
		"commissionCount":      count,
		"pendingCommissionCents": pendingCents,
		"pendingCommissionCount":  pendingCount,
		"currentRate":          currentRate,
		"nextRate":             nextRate,
	}, nil
}

// ListMyAffiliateCommissions 当前用户作为邀请人的返佣流水（分页）。
func ListMyAffiliateCommissions(userID string, q model.Query) ([]model.AffCommissionLog, int64, error) {
	return repository.ListAffCommissionLogsByInviter(userID, q)
}
