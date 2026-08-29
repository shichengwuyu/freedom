package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === novel-workflow v2: shot-subtitle-node CRUD ===

func shotSubtitleDB() (*gorm.DB, error) { return DB() }

// UpsertShotSubtitle 按 (project_id, shot_id) upsert。
func UpsertShotSubtitle(s *model.ShotSubtitle) error {
	db, err := shotSubtitleDB()
	if err != nil {
		return err
	}
	// v2 fix: 通过 (project_id, shot_id) 找现有 ID, 找不到就当新建
	if s.ID == "" {
		var es model.ShotSubtitle
		err = db.First(&es, "project_id = ? AND shot_id = ?", s.ProjectID, s.ShotID).Error
		if err == nil {
			s.ID = es.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	// v2 fix: GORM 字符串主键不会自动生成 ID, 留空会插空串
	// 如果到这里 ID 还空, 说明真的是新建, 用 project+shot 拼接作为 ID（保证唯一）
	if s.ID == "" {
		s.ID = "sub:" + s.ProjectID + ":" + s.ShotID
	}
	return db.Save(s).Error
}

// GetShotSubtitle 取最新一条（按 project_id + shot_id，按 updated_at desc）。
func GetShotSubtitle(projectID, shotID string) (*model.ShotSubtitle, error) {
	db, err := shotSubtitleDB()
	if err != nil {
		return nil, err
	}
	var s model.ShotSubtitle
	// v2 fix: 同 (project_id, shot_id) 可能有重做历史多条, 取最新
	if err := db.Where("project_id = ? AND shot_id = ?", projectID, shotID).
		Order("updated_at DESC, created_at DESC").
		First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// ListShotSubtitlesByProject 列项目所有字幕。
func ListShotSubtitlesByProject(projectID string) ([]model.ShotSubtitle, error) {
	db, err := shotSubtitleDB()
	if err != nil {
		return nil, err
	}
	var rows []model.ShotSubtitle
	if err := db.Where("project_id = ?", projectID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
