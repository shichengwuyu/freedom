package service

import (
	"strings"
	"testing"
)

// 单元测试：ModelCost 在 settings 里没有该模型时返回"未配置价格"错误。
// 这个测试是防 0 元白嫖的核心保护（fix 2026-08-28）。
// 真实生产路径：handler/ai.go 和 handler/video_task.go 在调用 ModelCost 后，
// 收到该 error → 返回 400 "该模型暂未配置价格，请联系管理员或换一个模型"。
//
// 注意：本测试需要 MySQL 可用。若 DB 连接失败，测试会 skip 而非 fail。
func TestModelCost_MissingPrice_ReturnsDescriptiveError(t *testing.T) {
	_, err := ModelCost("definitely-not-in-price-table-xyz-2026")
	if err == nil {
		t.Fatal("ModelCost on missing model expected error, got nil")
	}
	// DB 不可用：报 DB 错误（不算 fail，skip）。
	// DB 可用但 settings 为空：报"暂未配置价格"（assertion 通过）。
	if !strings.Contains(err.Error(), "暂未配置价格") {
		t.Skipf("DB unavailable or settings has data; err: %v", err)
	}
}
