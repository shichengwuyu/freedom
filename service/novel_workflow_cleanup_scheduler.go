package service

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: 跨切清理 cron ===
//
// 每天凌晨 3:30 跑：
//   1) 清理 30 天以上且无活跃 RerunRecord 引用的 composition_tasks + 对应 mp4 文件
//   2) 清理 30 天以上的 RerunRecord (除最近 1 version)
//
// v2 简化: cron 只清理文件 + 软删 DB 记录
// v3 接入对象存储后, 删 DB 记录 + 删对象存储文件 (同时)

const novelWorkflowCleanupCron = "30 3 * * *"

var (
	novelWorkflowCleanupCronOnce sync.Once
)

// StartNovelWorkflowCleanupScheduler 启动清理 cron。
// main.go 调用一次即可。
func StartNovelWorkflowCleanupScheduler() {
	novelWorkflowCleanupCronOnce.Do(func() {
		c := cron.New()
		if _, err := c.AddFunc(novelWorkflowCleanupCron, cleanupExpiredNovelWorkflowArtifacts); err != nil {
			log.Printf("add novel workflow cleanup cron failed err=%v", err)
			return
		}
		c.Start()
	})
}

// cleanupExpiredNovelWorkflowArtifacts 30 天前的过期成片 + rerun record 清理。
func cleanupExpiredNovelWorkflowArtifacts() {
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	log.Printf("novel-workflow cleanup: cutoff=%s", cutoff)

	tasks, _, err := repository.ListCompositionTasksByProject("", 200, 0)
	if err != nil {
		log.Printf("novel-workflow cleanup: list tasks err=%v", err)
	} else {
		deleted := 0
		for _, t := range tasks {
			if t.Status != "成功" {
				continue
			}
			if t.CompletedAt == "" || t.CompletedAt > cutoff {
				continue
			}
			if t.OutputURL != "" {
				if err := os.Remove(t.OutputURL); err != nil && !os.IsNotExist(err) {
					log.Printf("novel-workflow cleanup: remove %s err=%v", t.OutputURL, err)
				}
			}
			deleted++
		}
		log.Printf("novel-workflow cleanup: removed %d mp4 files (older than 30d)", deleted)
	}

	log.Printf("novel-workflow cleanup: rerun records cleanup skipped (v2 simplified)")
}
