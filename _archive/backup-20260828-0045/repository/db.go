package repository

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

var promptCategories = []model.PromptCategory{
	{Category: "system", Name: "系统", Description: "系统提示词分类"},
	{Category: "text", Name: "文本", Description: "文本创作类提示词"},
	{Category: "image", Name: "图片", Description: "图片生成类提示词（聚合精选 GitHub 图像提示词仓库）", GithubURL: "https://github.com/tigerowo/awesome-gpt-image-2-prompts", Remote: true},
	{Category: "video", Name: "视频", Description: "视频生成类提示词（聚合精选 GitHub 视频提示词仓库）", GithubURL: "https://github.com/YouMind-OpenLab/awesome-seedance-2-prompts", Remote: true},
}

var (
	db     *gorm.DB
	dbOnce sync.Once
	dbErr  error
)

// DB 初始化并返回全局数据库连接。
func DB() (*gorm.DB, error) {
	dbOnce.Do(func() {
		dsn := config.Cfg.DatabaseDSN
		if driver := strings.ToLower(strings.TrimSpace(config.Cfg.StorageDriver)); driver == "mysql" {
			dbErr = ensureMySQLDatabase(dsn)
			if dbErr != nil {
				return
			}
		}
		db, dbErr = gorm.Open(dialector(dsn), &gorm.Config{})
		if dbErr != nil {
			return
		}
		// 2026-08-27 验收发现：MySQL server 端 character_set_client 默认 gbk，
		// 写入中文 batch_name 时触发 Error 1366 'Incorrect string value'。
		// 修复：DSN 里已带 charset=utf8mb4，go-sql-driver 会在每条新连接建立时自动发
		// SET NAMES utf8mb4 COLLATE <collation>（见 driver 源码 connection.go:64-66）。
		// 这里再发一次保险（影响当前已拿到的连接；连接池新连接靠 driver 协商）。
		if err := db.Exec("SET NAMES utf8mb4").Error; err != nil {
			log.Printf("set names utf8mb4 failed: %v", err)
		}
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.SetMaxOpenConns(50)
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetConnMaxLifetime(30 * time.Minute)
			// 关键：关掉所有现存 idle conn，让后续查询必须新建（新建时由 driver 自动 SET）
			// 实际 mysql driver 默认就会按 DSN charset 协商
		}
		dbErr = db.AutoMigrate(
			&model.User{},
			&model.BalanceLog{},
			&model.BalanceHold{},
			&model.Prompt{},
			&model.Asset{},
			&model.Setting{},
			&model.CreativeWorkflow{},
			&model.UserConfig{},
			&model.AICallLog{},
			&model.StorageObject{},
			&model.VideoTask{},
			&model.StoryboardTask{},
			&model.VideoGenerationLog{},
			&model.ImageGenerationLog{},
			&model.CanvasImageTask{},
			&model.CanvasAudioTask{},
			&model.CanvasProject{},
			&model.LicenseKey{},
			&model.LicenseRedeemLog{},
			&model.Announcement{},
		)
		if dbErr == nil {
			// 2026-08-27：删除 libtv/updream/newwow 第三方云端供应商。GORM AutoMigrate 不会 DROP 已存在的表，
			// 所以这里在 migrate 完成后手动 DROP，并清掉 user_vendor_accounts 表里非 official 的存量绑定数据。
			// 顺序：先 DELETE 行（柔和），再 DROP TABLE（彻底），保证幂等且不依赖 DROP 成功。
			if tx := db.Exec("DELETE FROM user_vendor_accounts WHERE vendor_type <> 'official'"); tx.Error != nil {
				log.Printf("drop legacy vendor accounts (delete) failed: %v", tx.Error)
			}
			if db.Migrator().HasTable("vendor_api_samples") {
				if err := db.Migrator().DropTable("vendor_api_samples"); err != nil {
					log.Printf("drop legacy vendor_api_samples table failed: %v", err)
				} else {
					log.Printf("dropped legacy vendor_api_samples table (2026-08-27 清理)")
				}
			}
			if db.Migrator().HasTable("user_vendor_accounts") {
				if err := db.Migrator().DropTable("user_vendor_accounts"); err != nil {
					log.Printf("drop legacy user_vendor_accounts table failed: %v", err)
				} else {
					log.Printf("dropped legacy user_vendor_accounts table (2026-08-27 清理)")
				}
			}
			if db.Migrator().HasTable("vendors") {
				if err := db.Migrator().DropTable("vendors"); err != nil {
					log.Printf("drop legacy vendors table failed: %v", err)
				} else {
					log.Printf("dropped legacy vendors table (2026-08-27 清理)")
				}
			}
		}
	})
	return db, dbErr
}

func dialector(dsn string) gorm.Dialector {
	// 仅支持 MySQL（生产）。DSN 为 user:pass@tcp(...) 格式。
	// 未指定长度的 string 默认为 varchar(191)，避免建成 longtext 后无法建索引，且兼容 utf8mb4。
	return gormmysql.New(gormmysql.Config{DSN: dsn, DefaultStringSize: 191})
}

func ensureMySQLDatabase(dsn string) error {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(cfg.DBName)
	if target == "" {
		return nil
	}
	ctx := context.Background()
	targetDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	err = targetDB.PingContext(ctx)
	_ = targetDB.Close()
	if err == nil {
		return nil
	}
	if !isMySQLError(err, 1049) {
		return err
	}

	maintenance := cfg.Clone()
	maintenance.DBName = ""
	serverDB, err := sql.Open("mysql", maintenance.FormatDSN())
	if err != nil {
		return err
	}
	defer serverDB.Close()

	_, err = serverDB.ExecContext(ctx, "CREATE DATABASE "+quoteMySQLIdentifier(target)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if isMySQLError(err, 1007) {
		return nil
	}
	return err
}

func isMySQLError(err error, number uint16) bool {
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == number
}

func quoteMySQLIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
