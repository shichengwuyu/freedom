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
	if s.ID != "" {
		return db.Save(s).Error
	}
	var existing model.ShotSubtitle
	err = db.First(&existing, "project_id = ? AND shot_id = ?", s.ProjectID, s.ShotID).Error
	if err == nil {
		existing.LinesJSON = s.LinesJSON
		existing.Status = s.Status
		existing.Error = s.Error
		existing.UpdatedAt = s.UpdatedAt
		s.ID = existing.ID
		return db.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(s).Error
}

// GetShotSubtitle 取一条（按 project_id + shot_id）。
func GetShotSubtitle(projectID, shotID string) (*model.ShotSubtitle, error) {
	db, err := shotSubtitleDB()
	if err != nil {
		return nil, err
	}
	var s model.ShotSubtitle
	if err := db.First(&s, "project_id = ? AND shot_id = ?", projectID, shotID).Error; err != nil {
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
