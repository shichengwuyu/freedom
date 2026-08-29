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
	service.StartCanvasProjectCleanupScheduler()
	service.StartBalanceHoldSweepScheduler()
	service.StartModelStatusScheduler()
	service.StartModelPricingScheduler()
	service.StartAffiliateSettlementScheduler()
	handler.StartVideoTaskPoller()
	handler.StartStoryboardTaskRunner()

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
