package repository

import (
	"errors"
	"strings"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// ListUsers 分页查询用户。
func ListUsers(q model.Query) ([]model.User, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.User{})
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("username LIKE ? OR display_name LIKE ? OR email LIKE ? OR linux_do_id LIKE ?", like, like, like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []model.User
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&users).Error
	return users, total, err
}

// CountUsers 返回用户总数。
func CountUsers() (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	return total, db.Model(&model.User{}).Count(&total).Error
}

// HasAdmin 判断系统中是否存在管理员。
func HasAdmin() (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	var total int64
	err = db.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&total).Error
	return total > 0, err
}

// GetUserByID 根据 ID 查询用户。
func GetUserByID(id string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "id = ?", id)
}

// GetUserByUsername 根据用户名查询用户。
func GetUserByUsername(username string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "username = ?", username)
}

// SaveUser 保存用户信息。
func SaveUser(user model.User) (model.User, error) {
	db, err := DB()
	if err != nil {
		return user, err
	}
	return user, db.Save(&user).Error
}

func ConsumeUserBalance(id string, cents int, now string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	if cents <= 0 {
		user, ok, err := GetUserByID(id)
		return user, ok, err
	}
	tx := db.Model(&model.User{}).Where("id = ? AND balance_cents >= ?", id, cents).Updates(map[string]any{
		"balance_cents":    gorm.Expr("balance_cents - ?", cents),
		"updated_at": now,
	})
	if tx.Error != nil {
		return model.User{}, false, tx.Error
	}
	user, ok, err := GetUserByID(id)
	return user, ok && tx.RowsAffected > 0, err
}

func RefundUserBalance(id string, cents int, now string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	if cents <= 0 {
		user, ok, err := GetUserByID(id)
		return user, ok, err
	}
	tx := db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{
		"balance_cents":    gorm.Expr("balance_cents + ?", cents),
		"updated_at": now,
	})
	if tx.Error != nil {
		return model.User{}, false, tx.Error
	}
	user, ok, err := GetUserByID(id)
	return user, ok && tx.RowsAffected > 0, err
}

// SaveBalanceLog 保存余额变更流水。
func SaveBalanceLog(log model.BalanceLog) (model.BalanceLog, error) {
	db, err := DB()
	if err != nil {
		return log, err
	}
	return log, db.Save(&log).Error
}

func ListBalanceLogs(q model.Query) ([]model.BalanceLog, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.BalanceLog{})
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("user_id LIKE ? OR type LIKE ? OR remark LIKE ? OR related_id LIKE ?", like, like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.BalanceLog
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&logs).Error
	return logs, total, err
}

// ListUserBalanceLogs 用户侧查询自己的余额流水（PR-9）：
// 1) 强制 WHERE user_id = ?，绝不把 keyword 跨列模糊到他人行；
// 2) keyword 允许模糊匹配的列只限 type / remark / related_id（都是非用户标识字段）。
// 后台 ListBalanceLogs 仍保留跨列能力（管理员用）。
func ListUserBalanceLogs(userID string, q model.Query) ([]model.BalanceLog, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.BalanceLog{}).Where("user_id = ?", strings.TrimSpace(userID))
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		// 注意：绝不允许把 userID 放回 keyword 命中别的行（admin 侧 ListBalanceLogs 的行为）。
		tx = tx.Where("type LIKE ? OR remark LIKE ? OR related_id LIKE ?", like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.BalanceLog
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&logs).Error
	return logs, total, err
}

func DeleteBalanceLog(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.BalanceLog{}, "id = ?", id).Error
}

// RefundUserBalanceTx 事务内给用户加余额（兑换/退款 通用）。
func RefundUserBalanceTx(tx *gorm.DB, id string, cents int, now string) (model.User, bool, error) {
	if cents <= 0 {
		return GetUserByIDTx(tx, id)
	}
	res := tx.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{
		"balance_cents":    gorm.Expr("balance_cents + ?", cents),
		"updated_at": now,
	})
	if res.Error != nil {
		return model.User{}, false, res.Error
	}
	user, ok, err := GetUserByIDTx(tx, id)
	return user, ok && res.RowsAffected > 0, err
}

// SaveBalanceLogTx 事务内写流水。
func SaveBalanceLogTx(tx *gorm.DB, log model.BalanceLog) error {
	return tx.Create(&log).Error
}

// GetUserByIDTx 事务内查用户。
func GetUserByIDTx(tx *gorm.DB, id string) (model.User, bool, error) {
	user := model.User{}
	err := tx.Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, false, nil
	}
	return user, err == nil, err
}

// DeleteUser 删除指定用户。
func DeleteUser(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.User{}, "id = ?", id).Error
}

// GetBalanceHoldByUserAndRequest 幂等键查询：同一 user 下重复 Consume 相同 requestID 时复用 hold。
// 返回 (hold, found, err)：found=false 表示无记录（err==nil），由 service 层决定新建。
// 注意：只返回 status=held 的记录——cancelled/settled 行不算"可复用 hold"，避免
// 把已结算/已退款的 hold 当成"待结算的同 requestID 余额占用"复用。
func GetBalanceHoldByUserAndRequest(userID, requestID string) (*model.BalanceHold, bool, error) {
	db, err := DB()
	if err != nil {
		return nil, false, err
	}
	var hold model.BalanceHold
	err = db.Where("user_id = ? AND request_id = ? AND status = ?", userID, requestID, model.BalanceHoldHeld).First(&hold).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &hold, true, nil
}

// FindBalanceHoldByUserAndRequest 跨状态查询（不限 held），供事务内幂等回退用。
// 当 unique 冲突导致 Create 失败时，按 (user_id, request_id) 重新读最新行，
// 由 service 层判断是否真的可以复用。
func FindBalanceHoldByUserAndRequest(userID, requestID string) (*model.BalanceHold, bool, error) {
	db, err := DB()
	if err != nil {
		return nil, false, err
	}
	var hold model.BalanceHold
	err = db.Where("user_id = ? AND request_id = ?", userID, requestID).First(&hold).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &hold, true, nil
}

// GetBalanceHoldByID 查询单条 hold；未找到返回 (nil, nil)。
func GetBalanceHoldByID(id string) (*model.BalanceHold, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var hold model.BalanceHold
	err = db.Where("id = ?", id).First(&hold).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &hold, err
}

// SaveBalanceHoldTx 在事务里保存 hold（新建或更新 hold.Status/SettledAt/CancelledAt）。
func SaveBalanceHoldTx(tx *gorm.DB, hold *model.BalanceHold) error {
	return tx.Save(hold).Error
}

// ListStuckBalanceHolds 列出超过指定时间仍停留在 held 状态的余额占用记录（2026-08-17 引入）。
//
// 用于兜底扫描：handler/ai.go 和 handler/video_task.go 的 holdSettle/holdCancel 仅 log 失败，
// 一旦 settle/cancel 自身 DB 抽风，hold 会卡在 held → 余额被扣但永不结算/不退。
// 周期性扫描器会按 held + created_at < beforeTime 找到这些 hold，逐个调 CancelBalanceHold 退款。
//
// beforeTime 必须用 time.RFC3339Nano 格式（项目时间字段约定）；limit 建议 ≤ 500 防止大批量卡死 DB。
func ListStuckBalanceHolds(beforeTime string, limit int) ([]model.BalanceHold, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	var holds []model.BalanceHold
	err = db.Where("status = ? AND created_at < ?", model.BalanceHoldHeld, beforeTime).
		Order("created_at ASC").
		Limit(limit).
		Find(&holds).Error
	return holds, err
}

// GetUserByLinuxDoID 根据 Linux.do ID 查询用户。
func GetUserByLinuxDoID(id string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "linux_do_id = ?", id)
}

// findUser 查询单个用户，并将未命中转换为 ok=false。
func findUser(db *gorm.DB, query string, args ...any) (model.User, bool, error) {
	user := model.User{}
	err := db.Where(query, args...).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, false, nil
	}
	return user, err == nil, err
}
