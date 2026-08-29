package handler

import (
	"net/http"

	"github.com/tigerowo/freedom/service"
)

// ModelStatus 返回各模型性能与健康状态快照，供前端展示模型状态徽章。
// 公开接口（无需登录），数据由后台定时任务每 15 分钟刷新一次。
func ModelStatus(w http.ResponseWriter, r *http.Request) {
	OK(w, service.GetModelStatus())
}
