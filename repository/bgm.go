package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === novel-workflow v2: bgm-layer: BgmCustom CRUD ===

func bgmDB() (*gorm.DB, error) { return DB() }

// CreateBgmCustom 插入用户上传 BGM。
func CreateBgmCustom(b *model.BgmCustom) error {
	db, err := bgmDB()
	if err != nil {
		return err
	}
	return db.Create(b).Error
}

// DeleteBgmCustom 删一条 BGM（同时调用方负责删对象存储文件）。
func DeleteBgmCustom(id, userID string) error {
	db, err := bgmDB()
	if err != nil {
		return err
	}
	return db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.BgmCustom{}).Error
}

// ListBgmCustomsByProject 列项目所有自定义 BGM。
func ListBgmCustomsByProject(projectID string) ([]model.BgmCustom, error) {
	db, err := bgmDB()
	if err != nil {
		return nil, err
	}
	var rows []model.BgmCustom
	if err := db.Where("project_id = ?", projectID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetBgmCustom 取一条。
func GetBgmCustom(id, userID string) (*model.BgmCustom, error) {
	db, err := bgmDB()
	if err != nil {
		return nil, err
	}
	var b model.BgmCustom
	if err := db.First(&b, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}
