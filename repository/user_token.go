package repository

import (
	"time"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// SaveUserToken 插入或更新 user_token 记录。KeyHash 唯一索引保证不会因 hash 冲突重复落库。
func SaveUserToken(t *model.UserToken) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Save(t).Error
}

// GetUserTokenByID 按主键查。注意：调用方需要自行校验 user_id 归属，避免越权访问。
func GetUserTokenByID(id string) (*model.UserToken, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var t model.UserToken
	if err := db.Where("id = ?", id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// GetUserTokenByHash 按 sha256(明文) 查。这是 sk- token 鉴权的主路径。
func GetUserTokenByHash(hash string) (*model.UserToken, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var t model.UserToken
	if err := db.Where("key_hash = ?", hash).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ListUserTokensByUser 列出某用户的所有 token，按创建时间倒序。KeyHash 已被 GORM tag 屏蔽 json 序列化。
func ListUserTokensByUser(userID string) ([]model.UserToken, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var tokens []model.UserToken
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

// DeleteUserToken 仅允许删除自己名下 token（userID 双条件）。
func DeleteUserToken(id, userID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.UserToken{}).Error
}

// UpdateUserTokenStatus 修改 token 状态（active/disabled/exhausted/expired）。同时刷新 updated_at。
func UpdateUserTokenStatus(id, status string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.UserToken{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "updated_at": time.Now().UTC()}).
		Error
}

// UpdateUserTokenLastUsed 异步更新 last_used_ip / last_used_at，best-effort 调用方忽略错误。
func UpdateUserTokenLastUsed(id, ip string, at time.Time) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.UserToken{}).
		Where("id = ?", id).
		Updates(map[string]any{"last_used_ip": ip, "last_used_at": at, "updated_at": at}).
		Error
}

// IncrementUserTokenUsedCents 累加 token 已用额度（cents）。Sprint 2 渠道选择器后续在 consume 成功后调。
func IncrementUserTokenUsedCents(id string, cents int) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.UserToken{}).
		Where("id = ?", id).
		Update("used_cents", gorm.Expr("used_cents + ?", cents)).
		Error
}
