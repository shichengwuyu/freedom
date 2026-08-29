package repository

import (
	"time"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// SaveUserGroup 插入或更新 user_groups 记录。AutoMigrate 已建表。
func SaveUserGroup(g *model.UserGroup) error {
	db, err := DB()
	if err != nil {
		return err
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	g.UpdatedAt = time.Now().UTC()
	return db.Save(g).Error
}

// GetUserGroupByID 按主键查。找不到返回 nil, nil（不报错）。
func GetUserGroupByID(id string) (*model.UserGroup, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var g model.UserGroup
	err = db.Where("id = ?", id).First(&g).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ListUserGroups 列出所有 group（含 inactive），按 sort ASC, id ASC。
func ListUserGroups() ([]model.UserGroup, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var groups []model.UserGroup
	if err := db.Order("sort ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

// ListActiveUserGroups 只列 active 的，给公开 pricing API 用。
func ListActiveUserGroups() ([]model.UserGroup, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var groups []model.UserGroup
	if err := db.Where("is_active = ?", true).Order("sort ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}
