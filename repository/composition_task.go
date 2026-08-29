package repository

import (
	"errors"
	"time"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === novel-workflow v2: composition-layer CRUD ===

func compositionDB() (*gorm.DB, error) { return DB() }

// CreateCompositionTask 插入。
func CreateCompositionTask(t *model.CompositionTask) error {
	db, err := compositionDB()
	if err != nil {
		return err
	}
	return db.Create(t).Error
}

// GetCompositionTask 取一条。
func GetCompositionTask(id, userID string) (*model.CompositionTask, error) {
	db, err := compositionDB()
	if err != nil {
		return nil, err
	}
	var t model.CompositionTask
	if err := db.First(&t, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// UpdateCompositionTask 写回。
func UpdateCompositionTask(t *model.CompositionTask) error {
	db, err := compositionDB()
	if err != nil {
		return err
	}
	return db.Save(t).Error
}

// UpdateCompositionTaskProgress 仅写 progress_json / status / step。
func UpdateCompositionTaskProgress(id, status, progressJSON, errorMsg, stderrTail string) error {
	db, err := compositionDB()
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status":        status,
		"progress_json": progressJSON,
		"updated_at":    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if errorMsg != "" {
		updates["error"] = errorMsg
	}
	if stderrTail != "" {
		updates["stderr_tail"] = stderrTail
	}
	return db.Model(&model.CompositionTask{}).Where("id = ?", id).Updates(updates).Error
}

// ListCompositionTasksByProject 列项目所有合成任务。
func ListCompositionTasksByProject(projectID string, limit, offset int) ([]model.CompositionTask, int64, error) {
	db, err := compositionDB()
	if err != nil {
		return nil, 0, err
	}
	var rows []model.CompositionTask
	var total int64
	q := db.Model(&model.CompositionTask{}).Where("project_id = ?", projectID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
