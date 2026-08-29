package service

import (
	"fmt"
	"log"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// 邀请返佣每日批结算调度器（2026-08-23 引入）。
//
// 返佣采用 T+1 日结：被邀请人消费时只记 status=pending 的佣金流水（见 SettleCommissionOnConsume），
// 本调度器每天固定时刻扫描所有 pending 佣金，按邀请人聚合后一次性入账并标记 settled。
// 日结的好处：降低消费抖动、避免中途退款产生返佣争议、批量入账减少 DB 写放大。
const (
	affiliateSettlementCron = "10 0 * * *" // 每天 00:10 结算（UTC+8 部署时区下即北京时间 00:10）
	affiliateSettlementBatch = 500         // 每批最多处理多少位邀请人
)

var (
	affiliateSettlementCronInst *cron.Cron
	affiliateSettlementOnce     sync.Once
)

func StartAffiliateSettlementScheduler() {
	affiliateSettlementOnce.Do(func() {
		affiliateSettlementCronInst = cron.New()
		if _, err := affiliateSettlementCronInst.AddFunc(
			affiliateSettlementCron,
			runAffiliateSettlement,
		); err != nil {
			log.Printf("add affiliate settlement cron failed err=%v", err)
			return
		}
		affiliateSettlementCronInst.Start()
	})
	// 启动后立即跑一次，把进程宕机/重启期间累计的 pending 佣金补结算。
	runAffiliateSettlement()
}

// runAffiliateSettlement 扫描所有待结算佣金并逐邀请人批结算。
// 设计为可重复安全执行：以事务 + 状态机保证同一笔 pending 不会被重复入账。
func runAffiliateSettlement() {
	totalInviters, totalCents, err := RunAffiliateSettlementBatch()
	if err != nil {
		log.Printf("affiliate settlement failed err=%v", err)
		return
	}
	if totalInviters > 0 {
		log.Printf("affiliate settlement done: %d inviters, %d cents credited", totalInviters, totalCents)
	}
}

// RunAffiliateSettlementBatch 单次批结算（供调度器与手动触发复用）。
// 返回（处理的邀请人数, 入账总额分, error）。
func RunAffiliateSettlementBatch() (inviterCount int, creditedCents int, err error) {
	inviterIDs, err := repository.ListPendingAffCommissionInviterIDs(affiliateSettlementBatch)
	if err != nil {
		return 0, 0, err
	}
	for _, inviterID := range inviterIDs {
		pending, err := repository.ListPendingAffCommissionLogsByInviter(inviterID)
		if err != nil {
			return inviterCount, creditedCents, err
		}
		if len(pending) == 0 {
			continue
		}
		if e := settleOneInviter(inviterID, pending); e != nil {
			// 单人失败不阻断其他人；记日志后继续。
			log.Printf("settle affiliate for inviter %s failed err=%v", inviterID, e)
			continue
		}
		sum := 0
		for _, l := range pending {
			sum += l.CommissionCents
		}
		inviterCount++
		creditedCents += sum
	}
	return inviterCount, creditedCents, nil
}

func settleOneInviter(inviterID string, pending []model.AffCommissionLog) error {
	db, err := repository.DB()
	if err != nil {
		return err
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	// 在事务里重新查询一次该邀请人的 pending，避免与并发结算重复；
	// 若已被别的进程结算（状态非 pending），这里查到空则直接提交空事务。
	livePending, err := repository.ListPendingAffCommissionLogsByInviterTx(tx, inviterID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if len(livePending) == 0 {
		return tx.Commit().Error
	}
	balanceLog := model.BalanceLog{
		ID:     newID("bal"),
		UserID: inviterID,
		Type:   model.BalanceLogTypeAffCommission,
		Amount: 0, // 由 repo 内按聚合金额填充
		Remark: fmt.Sprintf("邀请返佣日结（%d 笔待结算合并入账）", len(livePending)),
	}
	if _, _, err := repository.SettlePendingAffCommissionsByInviterTx(tx, inviterID, livePending, balanceLog, now()); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}
