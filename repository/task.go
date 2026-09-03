package repository

import (
	"time"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// SaveTask 插入或更新通用 task 记录。
func SaveTask(t *model.Task) error {
	db, err := DB()
	if err != nil {
		return err
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	t.UpdatedAt = time.Now().UTC()
	return db.Save(t).Error
}

// GetTaskByID 按主键查。找不到返回 nil, nil。
func GetTaskByID(id string) (*model.Task, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var t model.Task
	if err := db.Where("id = ?", id).First(&t).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// ListPendingTasks 列 pending/running 状态的 task（worker 用）。
// 按 created_at ASC 拉（老的优先）。limit 防止单次扫太多。
func ListPendingTasks(limit int) ([]model.Task, error) {
	if limit <= 0 {
		limit = 50
	}
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var tasks []model.Task
	if err := db.Where("status IN (?, ?)", model.TaskStatusPending, model.TaskStatusRunning).
		Order("created_at ASC").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// ListUserTasks 列某用户的所有 task（按时间倒序；给用户端查询用）。
func ListUserTasks(userID string, limit, offset int) ([]model.Task, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := db.Model(&model.Task{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tasks []model.Task
	if err := db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// UpdateTaskStatus 更新 task 状态 + progress + result/error。
func UpdateTaskStatus(id, status string, progress int, resultJSON, errorMessage string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status":        status,
		"progress":      progress,
		"updated_at":    time.Now().UTC(),
		"result_json":   resultJSON,
		"error_message": errorMessage,
	}
	if status == model.TaskStatusRunning {
		now := time.Now().UTC()
		updates["started_at"] = &now
	}
	if status == model.TaskStatusSuccess || status == model.TaskStatusFailure || status == model.TaskStatusCanceled {
		now := time.Now().UTC()
		updates["finished_at"] = &now
	}
	return db.Model(&model.Task{}).Where("id = ?", id).Updates(updates).Error
}

// IncrementTaskAttempts 自增 attempts + 更新 last_polled_at。
func IncrementTaskAttempts(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return db.Model(&model.Task{}).Where("id = ?", id).Updates(map[string]any{
		"attempts":      gorm.Expr("attempts + ?", 1),
		"last_polled_at": &now,
		"updated_at":    now,
	}).Error
}
