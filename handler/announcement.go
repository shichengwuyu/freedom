package handler

import (
	"net/http"

	"github.com/tigerowo/freedom/service"
)

// LatestAnnouncements 公共接口：返回最新公告列表（最新10条，按创建时间倒序）。
func LatestAnnouncements(w http.ResponseWriter, r *http.Request) {
	items, err := service.ListLatestAnnouncements()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"items": items})
}
