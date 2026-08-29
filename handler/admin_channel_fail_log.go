package handler

import (
	"net/http"
	"strconv"

	"github.com/tigerowo/freedom/service"
)

// AdminChannelFailLogs 列出最近的渠道失败记录（Sprint 2 引入）。
// 仅 admin 路由下挂载；普通用户无权访问。
// 内存 ring buffer 不持久化，进程重启即清空。
func AdminChannelFailLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	items := service.ListChannelFailLogs(limit)
	OK(w, map[string]any{
		"items": items,
		"total": len(items),
	})
}
