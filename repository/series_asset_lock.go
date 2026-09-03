package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === novel-workflow v2: series-asset-lock CRUD ===

func seriesAssetLockDB() (*gorm.DB, error) { return DB() }

// GetSeriesAssetLockByProject 按 project 取主资产包。
func GetSeriesAssetLockByProject(userID, projectID string) (*model.SeriesAssetLock, error) {
	db, err := seriesAssetLockDB()
	if err != nil {
		return nil, err
	}
	var s model.SeriesAssetLock
	if err := db.First(&s, "user_id = ? AND project_id = ?", userID, projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// UpsertSeriesAssetLock 插入或更新主资产包。
func UpsertSeriesAssetLock(s *model.SeriesAssetLock) error {
	db, err := seriesAssetLockDB()
	if err != nil {
		return err
	}
	return db.Save(s).Error
}

// DeleteSeriesAssetLock 删除主资产包。
func DeleteSeriesAssetLock(userID, projectID string) error {
	db, err := seriesAssetLockDB()
	if err != nil {
		return err
	}
	return db.Where("user_id = ? AND project_id = ?", userID, projectID).Delete(&model.SeriesAssetLock{}).Error
}
