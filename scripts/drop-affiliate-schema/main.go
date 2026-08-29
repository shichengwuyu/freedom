// 邀请返佣下线后的 DB 清理脚本。
//
// 删除的内容（与代码删除配套，需要在停服期间手动跑一次）：
//   - users 表三列：aff_code, aff_count, inviter_id
//   - aff_commission_logs 整张表（含其索引与唯一键）
//   - balance_logs 里 type='aff_commission' 的历史流水（保留可读可追溯性，但与下线无关；这里**不删**，避免历史余额对账缺口）
//
// 用法（在项目根目录）：
//   go run ./scripts/drop-affiliate-schema
//
// 安全特性：
//   - 默认 dry-run：先打印将执行的 SQL + 受影响行数，确认无误后传 -apply 真正执行。
//   - 单事务执行，失败回滚。
//   - 不删 balance_logs 历史流水（即使 type=aff_commission 也不删），保留账目可追溯。
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/repository"
)

func main() {
	apply := flag.Bool("apply", false, "真正写入数据库（缺省仅 dry-run，打印将执行的 SQL 与影响行数）")
	flag.Parse()

	if err := config.Load(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := repository.DB()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 1) 统计 users 表的 aff_code/aff_count/inviter_id 三列实际存了多少非默认值。
	//    GORM 字段零值：aff_code="" aff_count=0 inviter_id=""，三者均有可能为"无意义空值"。
	//    这里只统计"有值"行数，让用户知道真要清多少。
	var affCodeCount, affCountCount, inviterIdCount int64
	if err := db.Model(&struct{}{}).Table("users").
		Where("aff_code IS NOT NULL AND aff_code <> ''").
		Count(&affCodeCount).Error; err != nil {
		log.Fatalf("统计 aff_code 失败: %v", err)
	}
	if err := db.Model(&struct{}{}).Table("users").
		Where("aff_count IS NOT NULL AND aff_count <> 0").
		Count(&affCountCount).Error; err != nil {
		log.Fatalf("统计 aff_count 失败: %v", err)
	}
	if err := db.Model(&struct{}{}).Table("users").
		Where("inviter_id IS NOT NULL AND inviter_id <> ''").
		Count(&inviterIdCount).Error; err != nil {
		log.Fatalf("统计 inviter_id 失败: %v", err)
	}

	// 2) 统计 aff_commission_logs 行数。
	var affLogCount int64
	if err := db.Model(&struct{}{}).Table("aff_commission_logs").
		Count(&affLogCount).Error; err != nil {
		// 表可能不存在（部署从来没有过返佣数据），视为 0；不要直接 fatal。
		affLogCount = 0
		fmt.Println("提示: aff_commission_logs 表不存在，已按 0 行处理（无需 drop）。")
	}

	// 3) 列出每个要执行的 SQL（按依赖顺序：先删外键/索引，再删列/表）。
	plans := []string{
		// 删除 aff_commission_logs 表的所有索引（含 idx_aff_inviter / unique(recharge_id)）。
		// MySQL 会随 DROP TABLE 自动级联索引，但显式 DROP INDEX 更清晰。
		"DROP TABLE IF EXISTS `aff_commission_logs`;",
		// 删除 users 表的索引与列。
		// GORM 用 uniqueIndex(aff_code) 生成的索引名是 aff_code，按 GORM 命名约定。
		"ALTER TABLE `users` DROP INDEX `aff_code`;",
		"ALTER TABLE `users` DROP INDEX `idx_aff_inviter`;",
		"ALTER TABLE `users` DROP COLUMN `aff_code`;",
		"ALTER TABLE `users` DROP COLUMN `aff_count`;",
		"ALTER TABLE `users` DROP COLUMN `inviter_id`;",
	}

	fmt.Printf("扫描结果：\n")
	fmt.Printf("  users.aff_code    非空行数: %d\n", affCodeCount)
	fmt.Printf("  users.aff_count   非零行数: %d\n", affCountCount)
	fmt.Printf("  users.inviter_id  非空行数: %d\n", inviterIdCount)
	fmt.Printf("  aff_commission_logs 表行数: %d\n\n", affLogCount)

	if affCodeCount == 0 && affCountCount == 0 && inviterIdCount == 0 && affLogCount == 0 {
		fmt.Println("无残留数据，但 schema 列与表可能仍存在；仍会按下方 SQL 清理。")
	}

	fmt.Println("将执行以下 SQL：")
	for _, p := range plans {
		fmt.Printf("  %s\n", p)
	}

	if !*apply {
		fmt.Println("\n未传 -apply，仅打印计划。重新执行 `go run ./scripts/drop-affiliate-schema -apply` 真正写入。")
		return
	}

	// 真正写入：单事务。
	tx := db.Begin()
	if tx.Error != nil {
		log.Fatalf("开启事务失败: %v", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Fatalf("清理异常回滚: %v", r)
		}
	}()

	for _, p := range plans {
		if err := tx.Exec(p).Error; err != nil {
			tx.Rollback()
			log.Fatalf("执行失败: %s\nerr: %v", p, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatalf("提交失败: %v", err)
	}
	fmt.Println("\n清理完成 ✓")
}
