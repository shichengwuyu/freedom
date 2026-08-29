package handler

import (
	"net/http"
	"strconv"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/tigerowo/freedom/service"
)

// listUserTasksFromRepo 内部 helper：直查 repository。Sprint 4.5 抽到 service 层。
func listUserTasksFromRepo(userID string, limit, offset int) ([]model.Task, int64, error) {
	return repository.ListUserTasks(userID, limit, offset)
}

// UserTasks 列出当前用户的所有通用 task（Sprint 4 引入）。
// 分页参数：page / pageSize（默认 page=1, pageSize=20, max=100）
//
// 注：返回的 task 是 Sprint 4 之后**新增能力**的 task（如 agent task / image batch）；
// 不返回 video_tasks / canvas_image_tasks / canvas_audio_tasks / storyboard_tasks 这 4 套
// 旧表（它们各自有自己的接口）。
func UserTasks(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// 公开 task 表（暂时走 repository 直查；admin 通用查询在 Sprint 4.5 加）
	tasks, total, err := listUserTasksFromRepo(user.ID, pageSize, offset)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "读取任务失败")
		return
	}
	OK(w, map[string]any{
		"items": tasks,
		"total": total,
		"page":  page,
		"pageSize": pageSize,
	})
}
