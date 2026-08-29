package repository

import (
	"errors"
	"strings"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func BatchInsertLicenseKeys(keys []model.LicenseKey) (int, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	tx := db.Session(&gorm.Session{SkipDefaultTransaction: true}).
		Clauses(clause.OnConflict{DoNothing: true})
	chunk := 100
	inserted := 0
	for i := 0; i < len(keys); i += chunk {
		end := i + chunk
		if end > len(keys) {
			end = len(keys)
		}
		res := tx.Create(keys[i:end])
		if res.Error != nil {
			return inserted, res.Error
		}
		inserted += int(res.RowsAffected)
	}
	return inserted, nil
}

func GetLicenseKeyByKey(tx *gorm.DB, key string, forUpdate bool) (model.LicenseKey, bool, error) {
	if tx == nil {
		db, err := DB()
		if err != nil {
			return model.LicenseKey{}, false, err
		}
		tx = db
	}
	k := model.LicenseKey{}
	q := tx.Where("`key` = ?", key)
	if forUpdate {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.LicenseKey{}, false, nil
	}
	return k, err == nil, err
}

func MarkLicenseKeyUsed(tx *gorm.DB, id, userID, usedAt string) error {
	res := tx.Model(&model.LicenseKey{}).
		Where("id = ? AND status = ?", id, model.LicenseKeyStatusUnused).
		Updates(map[string]any{
			"status":     model.LicenseKeyStatusUsed,
			"used_by":    userID,
			"used_at":    usedAt,
			"updated_at": usedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.New("该卡密已被使用")
	}
	return nil
}

func ListLicenseKeys(q model.Query, status, batchName, keyword string) ([]model.LicenseKey, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.LicenseKey{})
	if strings.TrimSpace(status) != "" {
		tx = tx.Where("status = ?", status)
	}
	if strings.TrimSpace(batchName) != "" {
		tx = tx.Where("batch_name = ?", batchName)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		tx = tx.Where("`key` LIKE ? OR batch_name LIKE ? OR used_by LIKE ?", "%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.LicenseKey
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func SaveLicenseRedeemLog(tx *gorm.DB, log model.LicenseRedeemLog) error {
	if tx == nil {
		db, err := DB()
		if err != nil {
			return err
		}
		tx = db
	}
	return tx.Create(&log).Error
}

func ListLicenseRedeemLogs(q model.Query, userID, userKeyword string) ([]model.LicenseRedeemLog, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.LicenseRedeemLog{})
	if strings.TrimSpace(userID) != "" {
		tx = tx.Where("user_id = ?", userID)
	}
	if strings.TrimSpace(userKeyword) != "" {
		like := "%" + userKeyword + "%"
		tx = tx.Where("user_name LIKE ? OR key_masked LIKE ?", like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.LicenseRedeemLog
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func CountLicenseKeysByBatchName(batchName string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.LicenseKey{}).
		Where("batch_name = ?", batchName).
		Count(&total).Error
	return total, err
}

func ExistsUsedKeyInBatch(batchName string) (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	var total int64
	err = db.Model(&model.LicenseKey{}).
		Where("batch_name = ? AND status = ?", batchName, model.LicenseKeyStatusUsed).
		Count(&total).Error
	return total > 0, err
}

// ExistingLicenseKeySet 分块 IN 查询，返回已存在于库中的 key 集合（用于生成时去重）。
func ExistingLicenseKeySet(keys []string) (map[string]bool, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	if len(keys) == 0 {
		return set, nil
	}
	chunk := 500
	for i := 0; i < len(keys); i += chunk {
		end := i + chunk
		if end > len(keys) {
			end = len(keys)
		}
		var found []string
		if err := db.Model(&model.LicenseKey{}).
			Where("`key` IN ?", keys[i:end]).
			Pluck("`key`", &found).Error; err != nil {
			return nil, err
		}
		for _, k := range found {
			set[k] = true
		}
	}
	return set, nil
}

// ListLicenseKeyKeysByBatch 按批次查出所有卡密字符串（一行一个导出用）。
func ListLicenseKeyKeysByBatch(batchName string) ([]string, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var keys []string
	if err := db.Model(&model.LicenseKey{}).
		Where("batch_name = ?", batchName).
		Order("created_at asc").
		Pluck("`key`", &keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

func UpdateBatchUnusedFaceValueCents(batchName string, cents int, now string) (int64, error) {
	used, err := ExistsUsedKeyInBatch(batchName)
	if err != nil {
		return 0, err
	}
	if used {
		return 0, errors.New("批次已开兑，不可修改面额")
	}
	db, err := DB()
	if err != nil {
		return 0, err
	}
	res := db.Model(&model.LicenseKey{}).
		Where("batch_name = ? AND status = ?", batchName, model.LicenseKeyStatusUnused).
		Updates(map[string]any{
			"face_value_cents": cents,
			"updated_at": now,
		})
	return res.RowsAffected, res.Error
}
