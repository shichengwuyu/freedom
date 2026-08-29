package repository

import (
	"context"
	"database/sql"
	"errors"
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
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.SetMaxOpenConns(50)
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetConnMaxLifetime(30 * time.Minute)
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
			// P0 新增：多供应商云端切换相关（文档 §3.1 / §3.2）
			&model.Vendor{},
			&model.UserVendorAccount{},
			// P1 新增：浏览器插件嗅探样本（UpDream/NewWow 内部接口学习用）
			&model.VendorApiSample{},
			// 邀请返佣流水（一级直推）
			&model.AffCommissionLog{},
			// Sprint 1.1：用户自建 API Key（sk- token）
			&model.UserToken{},
			// Sprint 3：用户组（阶梯定价维度）
			&model.UserGroup{},
			// Sprint 4：通用 task 模型（新能力入口；不替代 4 套旧 task 表）
			&model.Task{},
			// novel-workflow v2：工作流编排层（5 层 + 节点状态机）
			&model.NovelWorkflowRun{},
			&model.NovelWorkflowNode{},
			// novel-workflow v2：shot-dubbing-node
			&model.ShotDubbing{},
		)
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
