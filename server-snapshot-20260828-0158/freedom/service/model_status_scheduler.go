package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tigerowo/freedom/config"
)

// 模型性能状态定时拉取调度器（2026-08-20 引入）。
//
// 每 15 分钟从 ModelStatusURL 拉取上游性能指标汇总（成功率/延迟/TPS），
// 解析后按成功率分级为 normal/unstable/down，缓存到内存供前端 /api/model-status 读取。
// 拉取失败仅打日志，不影响已缓存数据；进程启动时立即拉取一次避免前端首次访问拿到空数据。
const (
	modelStatusCron        = "*/15 * * * *" // 每 15 分钟拉取一次
	modelStatusHTTPTimeout = 10 * time.Second
)

// ModelStatusItem 单个模型的性能指标快照，含健康度分级。
type ModelStatusItem struct {
	ModelName    string  `json:"model_name"`     // 模型名（与前端下拉框 model 一致）
	AvgLatencyMs float64 `json:"avg_latency_ms"` // 平均生成延迟（毫秒）
	AvgTtftMs    float64 `json:"avg_ttft_ms"`    // 平均首字延迟（毫秒）
	SuccessRate  float64 `json:"success_rate"`   // 成功率（百分比 0-100）
	AvgTps       float64 `json:"avg_tps"`        // 平均每秒 token 数
	Health       string  `json:"health"`         // 健康度：normal | unstable | down | unknown
}

// ModelStatusSummary 所有模型性能指标汇总，含最后刷新时间。
type ModelStatusSummary struct {
	Models    []ModelStatusItem `json:"models"`     // 全部模型状态列表
	UpdatedAt int64             `json:"updated_at"` // Unix 秒，最后一次成功刷新时间（0 表示尚未拉取过）
}

// upstreamModelStatusSummary 上游接口返回的原始结构。
type upstreamModelStatusSummary struct {
	Success bool `json:"success"`
	Data    struct {
		Models []struct {
			ModelName    string  `json:"model_name"`
			AvgLatencyMs float64 `json:"avg_latency_ms"`
			AvgTtftMs    float64 `json:"avg_ttft_ms"`
			SuccessRate  float64 `json:"success_rate"`
			AvgTps       float64 `json:"avg_tps"`
		} `json:"models"`
	} `json:"data"`
}

var (
	modelStatusCronInst *cron.Cron
	modelStatusOnce     sync.Once
	modelStatusMu       sync.RWMutex
	modelStatusCache    ModelStatusSummary
)

// StartModelStatusScheduler 启动模型状态定时拉取调度器（幂等，进程内只启动一次）。
func StartModelStatusScheduler() {
	modelStatusOnce.Do(func() {
		modelStatusCronInst = cron.New()
		if _, err := modelStatusCronInst.AddFunc(modelStatusCron, runModelStatusRefresh); err != nil {
			log.Printf("add model status cron failed err=%v", err)
			return
		}
		modelStatusCronInst.Start()
	})
	// 启动后立即拉取一次，避免前端首次访问拿到空数据。
	runModelStatusRefresh()
}

// runModelStatusRefresh 执行一次拉取并刷新内存缓存。
func runModelStatusRefresh() {
	url := config.Cfg.ModelStatusURL
	if url == "" {
		return
	}
	summary, err := fetchModelStatus(url)
	if err != nil {
		log.Printf("refresh model status failed url=%s err=%v", url, err)
		return
	}
	modelStatusMu.Lock()
	modelStatusCache = *summary
	modelStatusMu.Unlock()
	log.Printf("refresh model status done count=%d", len(summary.Models))
}

// fetchModelStatus 请求上游接口并解析为 ModelStatusSummary（含健康度分级）。
func fetchModelStatus(url string) (*ModelStatusSummary, error) {
	client := &http.Client{Timeout: modelStatusHTTPTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// 加浏览器风格的 User-Agent 降低上游风控概率
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw upstreamModelStatusSummary
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if !raw.Success {
		return nil, fmt.Errorf("upstream returned success=false")
	}
	items := make([]ModelStatusItem, 0, len(raw.Data.Models))
	for _, m := range raw.Data.Models {
		items = append(items, ModelStatusItem{
			ModelName:    m.ModelName,
			AvgLatencyMs: m.AvgLatencyMs,
			AvgTtftMs:    m.AvgTtftMs,
			SuccessRate:  m.SuccessRate,
			AvgTps:       m.AvgTps,
			Health:       classifyModelHealth(m.SuccessRate),
		})
	}
	return &ModelStatusSummary{
		Models:    items,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// classifyModelHealth 按成功率判断健康度：≥90% normal / 50-90% unstable / <50% down。
func classifyModelHealth(successRate float64) string {
	if successRate >= 90 {
		return "normal"
	}
	if successRate >= 50 {
		return "unstable"
	}
	return "down"
}

// GetModelStatus 返回当前缓存的模型状态快照（线程安全，只读）。
func GetModelStatus() ModelStatusSummary {
	modelStatusMu.RLock()
	defer modelStatusMu.RUnlock()
	return modelStatusCache
}
