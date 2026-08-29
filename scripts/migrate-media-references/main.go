// 一键迁移脚本：把数据库中所有残留的 `/api/media/references/<id>`（无 /v1 段）
// 替换为 `/api/v1/media/references/<id>`。
//
// 背景：参考素材读取路由在 router.go 中挂在 /api/v1 分组下（带 UserAuth），
// 但 handler.UploadReferenceMedia 历史上一直返回无 /v1 的 URL，因此所有通过该
// 接口上传的图片/视频/音频 URL 都被存成了旧的 /api/media/references/<id> 形式，
// 浏览器加载时全部 404。本次修复后新数据已正确，老数据需要批量回填。
//
// 用法（在项目根目录）：
//   go run ./scripts/migrate-media-references
//
// 安全特性：
//   - 仅替换形如 `/api/media/references/<...>` 的 URL，**不**替换以
//     `https?://...` 开头的（即使是绝对 URL）。`UploadReferenceMedia` 在
//     PublicBaseURL 已配置时会返回绝对 URL，绝对 URL 也需要替换（否则老
//     客户端仍指向旧主机/旧路径）；所以脚本同时支持相对与绝对两种形式。
//   - 不替换 `/api/v1/media/references/...`（防止对已修复行做二次替换）。
//   - 默认 dry-run 模式：先打印计划改动的行数与样例，确认无误后传 `-apply` 真正写入。
//   - 整个迁移在一个事务里执行，任何错误回滚。
package main

import (
	"flag"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/repository"
	"gorm.io/gorm"
)

const (
	oldRelativePrefix = "/api/media/references/"
	newRelativePrefix = "/api/v1/media/references/"
)

// relRE 匹配相对路径形式的旧 URL（不带 host）。Go regexp 不支持 lookahead，
// 因此这里只匹配前缀；`rewriteURLs` 里再做二次过滤排除已经修过的行。
// 路径段允许字母数字 + 短横线 + 点（UUID + 扩展名）。
var relRE = regexp.MustCompile(`/api/media/references/([\w\-./]+)`)

// absRE 匹配带 host 的绝对 URL 形式。例如
//   https://example.com/api/media/references/abc.png
//   http://127.0.0.1:8080/api/media/references/abc.png
// 用户配置的 PublicBaseURL 可能带路径尾巴（如 `https://h.example.com` vs
// `https://h.example.com/api`），但因为匹配是带 host 锚定的 `<scheme>://<host>/
// /api/media/references/`，PublicBaseURL 里多余的路径前缀会被保留下来
// （如 `https://h.example.com/api/api/v1/...`），所以绝对 URL 形式实际产出的
// 是「PublicBaseURL + 旧 URL 的 path」。如果历史 PublicBaseURL 已经包含
// `/api`，那么绝对 URL 修复后会是 `/api/api/v1/...`——但服务端 GORM 路由
// 是按 group 拼出来的，原始 URL 也是 `PublicBaseURL + /api/v1/...`，不会
// 出现这种重复。因此绝对 URL 修复方式与原 URL 等价。
var absRE = regexp.MustCompile(`(https?://[^/\s"']+)/api/media/references/([\w\-./]+)`)

func main() {
	apply := flag.Bool("apply", false, "真正写入数据库（缺省仅 dry-run，打印将改动的行数）")
	flag.Parse()

	if err := config.Load(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := repository.DB()
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 1. users.avatar_url：纯字符串列，直接 REPLACE。
	var userCount int64
	if err := db.Model(&struct{}{}).Table("users").
		Where("avatar_url LIKE ?", "%/api/media/references/%").
		Where("avatar_url NOT LIKE ?", "%/api/v1/media/references/%").
		Count(&userCount).Error; err != nil {
		log.Fatalf("统计 users 命中失败: %v", err)
	}

	// 2. settings.value：JSON longtext，整体 REPLACE（覆盖 public.contactSupport.wechatQr / qqGroupQr
	//    以及其它任何字段中可能存了同样 URL 的情况，比基于 JSON 路径解析更稳）。
	var settingCount int64
	if err := db.Model(&struct{}{}).Table("settings").
		Where("value LIKE ?", "%/api/media/references/%").
		Where("value NOT LIKE ?", "%/api/v1/media/references/%").
		Count(&settingCount).Error; err != nil {
		log.Fatalf("统计 settings 命中失败: %v", err)
	}

	fmt.Printf("扫描结果：\n  users.avatar_url      命中 %d 行\n  settings.value (JSON) 命中 %d 行\n", userCount, settingCount)
	if userCount == 0 && settingCount == 0 {
		fmt.Println("无残留旧 URL，无需迁移。")
		return
	}

	// 抽 3 条样例供人工核对。
	fmt.Println("\n样例（dry-run，未写入）：")
	printSamples(db, "users", "avatar_url", 3)
	printSamples(db, "settings", "value", 3)

	if !*apply {
		fmt.Println("\n未传 -apply，仅打印计划改动。重新执行 `go run ./scripts/migrate-media-references -apply` 真正写入。")
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
			log.Fatalf("迁移异常回滚: %v", r)
		}
	}()

	if userCount > 0 {
		if err := migrateColumn(tx, "users", "avatar_url"); err != nil {
			tx.Rollback()
			log.Fatalf("users.avatar_url 迁移失败: %v", err)
		}
	}
	if settingCount > 0 {
		if err := migrateColumn(tx, "settings", "value"); err != nil {
			tx.Rollback()
			log.Fatalf("settings.value 迁移失败: %v", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatalf("提交失败: %v", err)
	}
	fmt.Printf("\n迁移完成 %s。\n", time.Now().Format(time.RFC3339))
}

func migrateColumn(tx *gorm.DB, table, column string) error {
	type row struct {
		ID  string `gorm:"primaryKey"`
		Val string `gorm:"column:" + column`
	}
	var rows []row
	if err := tx.Table(table).
		Select("id, "+column+" AS val").
		Where(column+" LIKE ?", "%/api/media/references/%").
		Where(column+" NOT LIKE ?", "%/api/v1/media/references/%").
		Scan(&rows).Error; err != nil {
		return err
	}
	updated := 0
	for _, r := range rows {
		newVal := rewriteURLs(r.Val)
		if newVal == r.Val {
			continue
		}
		if err := tx.Table(table).
			Where("id = ?", r.ID).
			Update(column, newVal).Error; err != nil {
			return fmt.Errorf("更新 %s[id=%s] 失败: %w", table, r.ID, err)
		}
		updated++
	}
	fmt.Printf("  -> %s.%s 已更新 %d 行\n", table, column, updated)
	return nil
}

func rewriteURLs(s string) string {
	// 先处理绝对 URL（避免与相对 URL 混淆顺序），再做相对 URL。
	// 注意正则里捕获 host 与 path 两部分，重组时 host 保持不变。
	s = absRE.ReplaceAllString(s, `$1`+newRelativePrefix+`$2`)
	s = relRE.ReplaceAllString(s, newRelativePrefix+`$1`)
	// 防御性自检：替换完不应再残留 /api/media/references/（除非后面紧跟 /v1，但我们的正则不会保留这种情形，所以一定干净）。
	if strings.Contains(s, "/api/media/references/") {
		log.Printf("[WARN] 替换后仍含旧前缀，疑似有更复杂的形式未覆盖：%q", s)
	}
	return s
}

func printSamples(db *gorm.DB, table, column string, n int) {
	type row struct {
		ID  string `gorm:"primaryKey"`
		Val string `gorm:"column:" + column`
	}
	var rows []row
	if err := db.Table(table).
		Select("id, "+column+" AS val").
		Where(column+" LIKE ?", "%/api/media/references/%").
		Where(column+" NOT LIKE ?", "%/api/v1/media/references/%").
		Limit(n).Scan(&rows).Error; err != nil {
		fmt.Printf("  (采样 %s.%s 失败: %v)\n", table, column, err)
		return
	}
	if len(rows) == 0 {
		return
	}
	for _, r := range rows {
		fmt.Printf("  %s[id=%s]:\n    before: %s\n    after : %s\n", table, r.ID, trimForLog(r.Val), trimForLog(rewriteURLs(r.Val)))
	}
}

func trimForLog(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(截断,原长=" + fmt.Sprint(len(s)) + ")"
}

// 防止 import 未用报错
