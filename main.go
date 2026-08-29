package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/handler"
	"github.com/tigerowo/freedom/repository"
	"github.com/tigerowo/freedom/router"
	"github.com/tigerowo/freedom/service"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatal(err)
	}
	// novel-workflow v2：ffmpeg 启动 smoke check。
	// ffmpeg 缺失只 warn（不阻塞），dev 机器可继续开发；合成 worker 在需要时才报错。
	if out, err := service.CheckFfmpegAvailable(config.Cfg.FfmpegBinaryPath); err != nil {
		log.Printf("[WARN] ffmpeg 不可用 (%v)：成片合成功能将不可用。Docker 镜像已预装 ffmpeg，本地 dev 请安装并设置 FFMPEG_BINARY_PATH", err)
	} else {
		log.Printf("[INFO] ffmpeg 可用: %s", out)
	}
	// 启动期触发 DB 连接 + AutoMigrate，避免首次请求才暴露 DB 故障。
	if _, err := repository.DB(); err != nil {
		log.Fatal("数据库初始化失败: ", err)
	}
	if err := service.EnsureDefaultAdmin(); err != nil {
		log.Fatal(err)
	}
	service.StartPromptSyncScheduler()
	// Sprint 2：启动期构建渠道选择器倒排索引（admin 改 channels 后 SaveSettings 会重建）
	if err := service.BuildAbilityCache(); err != nil {
		log.Printf("build ability cache failed: %v", err)
	}
	// Sprint 3：seed 4 个内置 user group（default/plus/pro/enterprise）
	if err := service.SeedDefaultUserGroups(); err != nil {
		log.Printf("seed default user groups failed: %v", err)
	}
	// Sprint 4：通用 task 后台 worker（5s 轮询 pending/running task）
	// Sprint 4 暂不注册任何 handler；worker 启动后空闲不报错
	// 新能力接入：调 service.RegisterTaskHandler(typeStr, handlerImpl) 即可
	// 参考实现：service/task_handler_example.go
	service.StartTaskWorker()
	service.StartCanvasProjectCleanupScheduler()
	service.StartBalanceHoldSweepScheduler()
	service.StartModelStatusScheduler()
	service.StartModelPricingScheduler()
	service.StartAffiliateSettlementScheduler()
	handler.StartVideoTaskPoller()
	handler.StartStoryboardTaskRunner()
	// novel-workflow v2：启动工作流状态聚合 worker（5s 轮询）
	service.StartNovelWorkflowWorker(context.Background())
	// novel-workflow v2：加载 BGM 预设（manifest + mp3 校验）
	if err := service.LoadBgmPresets(); err != nil {
		log.Printf("load bgm presets failed: %v", err)
	}

	// 优雅关闭：捕获 SIGINT/SIGTERM，等待 in-flight 请求完成再退出。
	srv := &http.Server{
		Addr:              ":" + config.Cfg.Port,
		Handler:           router.New(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second, // AI 生成请求可能较长
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("server starting on :%s", config.Cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server forced shutdown: %v", err)
	}
	log.Println("server exited")
}
