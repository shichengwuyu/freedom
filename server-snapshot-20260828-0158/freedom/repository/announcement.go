package repository

import (
	"strings"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

func CreateAnnouncement(item model.Announcement) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Create(&item).Error
}

func UpdateAnnouncement(id, content, updatedAt string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	res := db.Model(&model.Announcement{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"content":    content,
			"updated_at": updatedAt,
		})
	return res.Error
}

func DeleteAnnouncement(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.Announcement{}, "id = ?", id).Error
}

func GetAnnouncementByID(id string) (model.Announcement, bool, error) {
	db, err := DB()
	if err != nil {
		return model.Announcement{}, false, err
	}
	var item model.Announcement
	err = db.Where("id = ?", id).First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return model.Announcement{}, false, nil
	}
	return item, err == nil, err
}

// ListLatestAnnouncements 返回最新的 limit 条公告（按创建时间倒序）。
func ListLatestAnnouncements(limit int) ([]model.Announcement, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	var items []model.Announcement
	err = db.Order("created_at desc").Limit(limit).Find(&items).Error
	return items, err
}

// ListAnnouncements 管理员分页列表（支持关键词搜索内容）。
func ListAnnouncements(q model.Query, keyword string) ([]model.Announcement, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.Announcement{})
	if kw := strings.TrimSpace(keyword); kw != "" {
		tx = tx.Where("content LIKE ?", "%"+kw+"%")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.Announcement
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}
