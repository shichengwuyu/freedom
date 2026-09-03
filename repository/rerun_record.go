package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === novel-workflow v2: novel-rerun-layer CRUD ===

func rerunDB() (*gorm.DB, error) { return DB() }

// CreateRerunRecord 插入一条重做记录。
func CreateRerunRecord(r *model.RerunRecord) error {
	db, err := rerunDB()
	if err != nil {
		return err
	}
	return db.Create(r).Error
}

// GetRerunRecord 取一条。
func GetRerunRecord(id, userID string) (*model.RerunRecord, error) {
	db, err := rerunDB()
	if err != nil {
		return nil, err
	}
	var r model.RerunRecord
	if err := db.First(&r, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// UpdateRerunRecord 写回。
func UpdateRerunRecord(r *model.RerunRecord) error {
	db, err := rerunDB()
	if err != nil {
		return err
	}
	return db.Save(r).Error
}

// ListRerunRecordsByScope 列某 scope+layer+shot 的所有版本（按 version 倒序）。
func ListRerunRecordsByScope(userID, projectID, scope, layer, shotID string) ([]model.RerunRecord, error) {
	db, err := rerunDB()
	if err != nil {
		return nil, err
	}
	q := db.Where("user_id = ? AND project_id = ? AND scope = ? AND layer = ?", userID, projectID, scope, layer)
	if shotID != "" {
		q = q.Where("shot_id = ?", shotID)
	}
	var rows []model.RerunRecord
	if err := q.Order("version DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// LatestRerunRecordByScope 取最新一条。
func LatestRerunRecordByScope(userID, projectID, scope, layer, shotID string) (*model.RerunRecord, error) {
	rows, err := ListRerunRecordsByScope(userID, projectID, scope, layer, shotID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// NextRerunRecordVersion 算下一个版本号。
func NextRerunRecordVersion(userID, projectID, scope, layer, shotID string) (int, error) {
	rows, err := ListRerunRecordsByScope(userID, projectID, scope, layer, shotID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 1, nil
	}
	return rows[0].Version + 1, nil
}
