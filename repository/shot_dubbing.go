package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === novel-workflow v2: shot-dubbing-node CRUD ===

func shotDubbingDB() (*gorm.DB, error) { return DB() }

// UpsertShotDubbing 按 (project_id, shot_id) upsert。
func UpsertShotDubbing(d *model.ShotDubbing) error {
	db, err := shotDubbingDB()
	if err != nil {
		return err
	}
	// v2 fix: 通过 (project_id, shot_id) 找现有 ID, 找不到就当新建
	if d.ID == "" {
		var existing model.ShotDubbing
		err = db.First(&existing, "project_id = ? AND shot_id = ?", d.ProjectID, d.ShotID).Error
		if err == nil {
			d.ID = existing.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	// v2 fix: GORM 字符串主键不会自动生成 ID, 留空会插空串
	// 如果到这里 ID 还空, 用 project+shot 拼接作为 ID
	if d.ID == "" {
		d.ID = "dub:" + d.ProjectID + ":" + d.ShotID
	}
	return db.Save(d).Error
}

// GetShotDubbing 取最新一条（按 project_id + shot_id，按 updated_at desc）。
func GetShotDubbing(projectID, shotID string) (*model.ShotDubbing, error) {
	db, err := shotDubbingDB()
	if err != nil {
		return nil, err
	}
	var d model.ShotDubbing
	// v2 fix: 取最新, 避免重做历史多条时取错
	if err := db.Where("project_id = ? AND shot_id = ?", projectID, shotID).
		Order("updated_at DESC, created_at DESC").
		First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// ListShotDubbingsByProject 列项目所有配音。
func ListShotDubbingsByProject(projectID string) ([]model.ShotDubbing, error) {
	db, err := shotDubbingDB()
	if err != nil {
		return nil, err
	}
	var rows []model.ShotDubbing
	if err := db.Where("project_id = ?", projectID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
