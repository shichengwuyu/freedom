package repository

import (
	"errors"
	"time"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === Session CRUD ===
//
// 所有查询按 user_id 过滤（多用户隔离）。删除走事务级联 messages。

// CreateNovelWriteSession 插入一条 session 记录。
func CreateNovelWriteSession(s *model.NovelWriteSession) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Create(s).Error
}

// ListNovelWriteSessionsByUser 列用户的 session（按 updated_at DESC）。
func ListNovelWriteSessionsByUser(userID string, limit int) ([]model.NovelWriteSession, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var rows []model.NovelWriteSession
	if err := db.Where("user_id = ?", userID).Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetNovelWriteSession 拿一条 session（按 user_id 过滤，找不到返回 nil）。
func GetNovelWriteSession(id, userID string) (*model.NovelWriteSession, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var s model.NovelWriteSession
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// UpdateNovelWriteSession 更新 session 的任意子集字段（map[string]any）。
// 调用方必须确保 map 里只有合法字段名。
// service 层过滤 id / user_id / created_at；本函数不主动过滤（model 允许改）。
func UpdateNovelWriteSession(id, userID string, updates map[string]any) error {
	db, err := DB()
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	return db.Model(&model.NovelWriteSession{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates).Error
}

// DeleteNovelWriteSession 删 session + 级联删 messages（事务）。
func DeleteNovelWriteSession(id, userID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// 先校验归属（避免删到别人的）
		var s model.NovelWriteSession
		if err := tx.Where("id = ? AND user_id = ?", id, userID).First(&s).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&model.NovelWriteMessage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.NovelWriteSession{}).Error
	})
}

// CountNovelWriteSessionsByUser 数用户的 session 数。
func CountNovelWriteSessionsByUser(userID string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.Model(&model.NovelWriteSession{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// TouchNovelWriteSession 更新 updated_at（让 session 浮到列表顶）。
func TouchNovelWriteSession(id, userID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.NovelWriteSession{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("updated_at", nowUTC()).Error
}

// nowUTC 内联 helper：返回 RFC3339 UTC 时间戳（和 model 里其他表风格一致）。
func nowUTC() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// DeleteOldestNovelWriteSession 删用户最早（updated_at 最小）的 session（带级联）。
func DeleteOldestNovelWriteSession(userID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var s model.NovelWriteSession
		if err := tx.Where("user_id = ?", userID).Order("updated_at ASC").First(&s).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", s.ID).Delete(&model.NovelWriteMessage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", s.ID).Delete(&model.NovelWriteSession{}).Error
	})
}

// === Messages ===

// AppendNovelWriteMessage 追加一条消息。
func AppendNovelWriteMessage(m *model.NovelWriteMessage) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Create(m).Error
}

// ListNovelWriteMessagesBySession 列某 session 的全部消息（按 id ASC）。
func ListNovelWriteMessagesBySession(sessionID string) ([]model.NovelWriteMessage, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var rows []model.NovelWriteMessage
	if err := db.Where("session_id = ?", sessionID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// === Exports ===

// CreateNovelWriteExport 插入导出记录。
func CreateNovelWriteExport(e *model.NovelWriteExport) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Create(e).Error
}

// ListNovelWriteExportsByUser 列用户导出历史（按 created_at DESC）。
func ListNovelWriteExportsByUser(userID string, limit int) ([]model.NovelWriteExport, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var rows []model.NovelWriteExport
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
