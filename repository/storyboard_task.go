package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
)

// ErrStoryboardTaskConflict 创建分镜任务时遇到已存在且非可恢复状态的记录，返回 409。
// 仅用于幂等性场景：同一 ClientTaskID 已完成/已失败/已取消时，前端不应再次复用同一 id。
var ErrStoryboardTaskConflict = errors.New("storyboard task already in terminal state")

// SaveStoryboardTask 新增或全量更新分镜任务；ClientTaskID 已存在时按状态分流实现真幂等：
//   - queued / running：原样返回已存在记录（worker 已接手或正在跑，前端再提就当成功）；
//   - completed / failed / canceled：返回 ErrStoryboardTaskConflict 让 handler 409；
//   - id 为空：直接 Save 新增。
func SaveStoryboardTask(task model.StoryboardTask) (model.StoryboardTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}
	if task.ID != "" {
		var existing model.StoryboardTask
		if e := db.First(&existing, "id = ?", task.ID).Error; e == nil {
			switch existing.Status {
			case "queued", "running":
				// 已在排队/执行中：直接返回已有记录，前端重复提交 = 无害幂等
				return existing, nil
			case "completed", "failed", "canceled":
				return task, ErrStoryboardTaskConflict
			}
		}
	}
	return task, db.Save(&task).Error
}

// UpdateStoryboardTask 无条件全量更新分镜任务，供 worker 持久化执行进度与终态
// （running / completed / failed / canceled）。
//
// 不要用 SaveStoryboardTask：后者在任务已处于 queued/running 时会短路返回已有记录
// （幂等创建语义），导致 worker 的状态变更全部丢失、任务永远停留在 due 集合里被反复拉起。
// 本函数只做纯写入，不读已有状态，专用于"已知要更新"的 worker/取消路径。
func UpdateStoryboardTask(task model.StoryboardTask) (model.StoryboardTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}
	return task, db.Save(&task).Error
}

// GetStoryboardTask 按 id 取单条（不限用户，鉴权由 service/handler 层保证）
func GetStoryboardTask(id string) (model.StoryboardTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.StoryboardTask{}, false, err
	}
	var task model.StoryboardTask
	err = db.First(&task, "id = ?", id).Error
	if err != nil {
		return model.StoryboardTask{}, false, nil
	}
	return task, true, nil
}

// GetUserStoryboardTask 取当前用户的单条任务
func GetUserStoryboardTask(userID string, id string) (model.StoryboardTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.StoryboardTask{}, false, err
	}
	var task model.StoryboardTask
	err = db.First(&task, "user_id = ? AND id = ?", userID, id).Error
	if err != nil {
		return model.StoryboardTask{}, false, nil
	}
	return task, true, nil
}

// ListUserStoryboardTasks 列出用户的分镜任务（含已完成，前端用于恢复进度与查看历史）
func ListUserStoryboardTasks(userID string, limit int) ([]model.StoryboardTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var tasks []model.StoryboardTask
	err = db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// DeleteUserStoryboardTask 删除当前用户的单条分镜任务（鉴权由 service/handler 层保证）
func DeleteUserStoryboardTask(userID string, id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Where("user_id = ? AND id = ?", userID, id).Delete(&model.StoryboardTask{}).Error
}

// ListDueStoryboardTasks 取所有待执行/执行中的任务，供后台 worker 拉取
func ListDueStoryboardTasks(limit int) ([]model.StoryboardTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var tasks []model.StoryboardTask
	err = db.Where("status IN ?", []string{"queued", "running"}).
		Order("created_at ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// DeleteFinishedStoryboardTasksBefore 清理已完成/失败超过保留期的任务
func DeleteFinishedStoryboardTasksBefore(before string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.
		Where("completed_at <> ? AND completed_at < ?", "", before).
		Where("status IN ?", []string{"completed", "failed"}).
		Delete(&model.StoryboardTask{}).Error
}
