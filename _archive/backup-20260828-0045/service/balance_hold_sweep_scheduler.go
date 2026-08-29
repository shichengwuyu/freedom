package service

import (
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// 余额占用卡死兜底扫描调度器（2026-08-17 引入）。
//
// 兜底 holdSettle/holdCancel 失败时只 log 不重试的盲区：周期性把卡在 held 状态超过 30 分钟
// 的余额占用 cancel 退款。CancelBalanceHold 自身幂等，重复跑安全。
const (
	balanceHoldSweepCron = "*/5 * * * *" // 每 5 分钟扫一次
	balanceHoldSweepAge  = 30 * time.Minute
)

var (
	balanceHoldSweepCronInst *cron.Cron
	balanceHoldSweepOnce     sync.Once
)

func StartBalanceHoldSweepScheduler() {
	balanceHoldSweepOnce.Do(func() {
		balanceHoldSweepCronInst = cron.New()
		if _, err := balanceHoldSweepCronInst.AddFunc(
			balanceHoldSweepCron,
			runBalanceHoldSweep,
		); err != nil {
			log.Printf("add balance hold sweep cron failed err=%v", err)
			return
		}
		balanceHoldSweepCronInst.Start()
	})
	// 启动后立即跑一次，把进程宕机期间累计的卡死 hold 一次清掉。
	runBalanceHoldSweep()
}

func runBalanceHoldSweep() {
	if _, _, err := SweepStuckBalanceHolds(balanceHoldSweepAge); err != nil {
		log.Printf("sweep stuck balance holds failed err=%v", err)
	}
}
