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
	// 优先按 ID upsert；ID 为空时按 (project_id, shot_id) upsert
	if d.ID != "" {
		return db.Save(d).Error
	}
	var existing model.ShotDubbing
	err = db.First(&existing, "project_id = ? AND shot_id = ?", d.ProjectID, d.ShotID).Error
	if err == nil {
		// 已存在：更新关键字段
		existing.Text = d.Text
		existing.VoiceID = d.VoiceID
		existing.Speed = d.Speed
		existing.TtsModel = d.TtsModel
		existing.AudioURL = d.AudioURL
		existing.DurationMs = d.DurationMs
		existing.Bytes = d.Bytes
		existing.MimeType = d.MimeType
		existing.Status = d.Status
		existing.Error = d.Error
		existing.GenericTaskID = d.GenericTaskID
		existing.BalanceLogID = d.BalanceLogID
		existing.CostCents = d.CostCents
		if d.CompletedAt != "" {
			existing.CompletedAt = d.CompletedAt
		}
		existing.UpdatedAt = d.UpdatedAt
		d.ID = existing.ID
		return db.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// 不存在：create
	return db.Create(d).Error
}

// GetShotDubbing 取一条（按 project_id + shot_id）。
func GetShotDubbing(projectID, shotID string) (*model.ShotDubbing, error) {
	db, err := shotDubbingDB()
	if err != nil {
		return nil, err
	}
	var d model.ShotDubbing
	if err := db.First(&d, "project_id = ? AND shot_id = ?", projectID, shotID).Error; err != nil {
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
