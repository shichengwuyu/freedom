package repository

import (
	"errors"
	"fmt"
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
func GetBalanceHoldByUserAndRequest(userID, requestID string) (*model.BalanceHold, bool, error) {
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

// GetUserByAffCode 根据邀请码查询用户（推广码落地用）。
func GetUserByAffCode(code string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "aff_code = ?", code)
}

// SaveAffCommissionLog 保存邀请返佣流水。
func SaveAffCommissionLog(log model.AffCommissionLog) (model.AffCommissionLog, error) {
	db, err := DB()
	if err != nil {
		return log, err
	}
	return log, db.Save(&log).Error
}

// GetAffCommissionLogByRecharge 按充值 ID 查返佣记录（幂等校验：同一笔充值只结算一次）。
func GetAffCommissionLogByRecharge(rechargeID string) (model.AffCommissionLog, bool, error) {
	db, err := DB()
	if err != nil {
		return model.AffCommissionLog{}, false, err
	}
	var log model.AffCommissionLog
	err = db.Where("recharge_id = ?", rechargeID).First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AffCommissionLog{}, false, nil
	}
	return log, err == nil, err
}

// SumAffCommissionByInviter 聚合某邀请人的累计佣金（分）与笔数。
func SumAffCommissionByInviter(inviterID string) (totalCents int, count int64, err error) {
	db, err := DB()
	if err != nil {
		return 0, 0, err
	}
	return sumAffCommissionByInviterStatus(db, inviterID, model.AffCommissionStatusSettled)
}

// SumAffCommissionPendingByInviter 聚合某邀请人待结算佣金（分）与笔数（status=pending）。
func SumAffCommissionPendingByInviter(inviterID string) (totalCents int, count int64, err error) {
	db, err := DB()
	if err != nil {
		return 0, 0, err
	}
	return sumAffCommissionByInviterStatus(db, inviterID, model.AffCommissionStatusPending)
}

func sumAffCommissionByInviterStatus(db *gorm.DB, inviterID, status string) (int, int64, error) {
	var sum *float64
	if e := db.Model(&model.AffCommissionLog{}).
		Where("inviter_id = ? AND status = ?", inviterID, status).
		Select("COALESCE(SUM(commission_cents), 0)").
		Scan(&sum).Error; e != nil {
		return 0, 0, e
	}
	total := 0
	if sum != nil {
		total = int(*sum)
	}
	var c int64
	if e := db.Model(&model.AffCommissionLog{}).
		Where("inviter_id = ? AND status = ?", inviterID, status).
		Count(&c).Error; e != nil {
		return 0, 0, e
	}
	return total, c, nil
}

// ListPendingAffCommissionInviterIDs 返回当前有待结算佣金（status=pending）的去重邀请人 ID 列表。
// 批结算调度器按此列表逐人聚合入账。limit 建议 ≤ 500 防大批量卡死 DB。
func ListPendingAffCommissionInviterIDs(limit int) ([]string, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 500
	}
	var ids []string
	err = db.Model(&model.AffCommissionLog{}).
		Where("status = ?", model.AffCommissionStatusPending).
		Distinct("inviter_id").
		Limit(limit).
		Pluck("inviter_id", &ids).Error
	return ids, err
}

// ListPendingAffCommissionLogsByInviter 返回某邀请人全部待结算（status=pending）佣金流水。
func ListPendingAffCommissionLogsByInviter(inviterID string) ([]model.AffCommissionLog, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	return listPendingAffCommissionLogsByInviterDB(db, inviterID)
}

// ListPendingAffCommissionLogsByInviterTx 事务内版本，供批结算事务里二次校验（防并发重复入账）。
func ListPendingAffCommissionLogsByInviterTx(tx *gorm.DB, inviterID string) ([]model.AffCommissionLog, error) {
	return listPendingAffCommissionLogsByInviterDB(tx, inviterID)
}

func listPendingAffCommissionLogsByInviterDB(db *gorm.DB, inviterID string) ([]model.AffCommissionLog, error) {
	var logs []model.AffCommissionLog
	err := db.Where("inviter_id = ? AND status = ?", inviterID, model.AffCommissionStatusPending).
		Order("created_at ASC").
		Find(&logs).Error
	return logs, err
}

// SettlePendingAffCommissionsByInviterTx 在事务内对某邀请人的全部 pending 佣金做批结算：
// 聚合佣金 → 邀请人余额入账（平台让利，不从被邀请人处扣）→ 写余额流水 → 标记这些日志 settled。
// 调用方需保证传入的 balanceLog 已填好（ID 等），且 pendingLogs 确实属于该 inviter 且状态为 pending。
// 返回入账总额（分）与已结算的日志数，便于日志与幂等。
func SettlePendingAffCommissionsByInviterTx(tx *gorm.DB, inviterID string, pendingLogs []model.AffCommissionLog, balanceLog model.BalanceLog, settleTime string) (totalCents int, settledCount int, err error) {
	if len(pendingLogs) == 0 {
		return 0, 0, nil
	}
	totalCents = 0
	for _, l := range pendingLogs {
		totalCents += l.CommissionCents
	}
	if totalCents <= 0 {
		// 仍把这些日志标记为 settled，避免空结算反复扫
		return 0, len(pendingLogs), markAffLogsSettledTx(tx, pendingLogs, settleTime)
	}

	inviter, ok, err := GetUserByIDTx(tx, inviterID)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, fmt.Errorf("邀请人 %s 不存在", inviterID)
	}

	// 1. 邀请人余额入账
	if _, ok, err := RefundUserBalanceTx(tx, inviterID, totalCents, settleTime); err != nil || !ok {
		if err != nil {
			return 0, 0, err
		}
		return 0, 0, fmt.Errorf("邀请人 %s 入账失败", inviterID)
	}

	// 2. 余额流水（ID 由调用方生成，避免跨包依赖 service.newID）
	balanceLog.Balance = inviter.BalanceCents + totalCents
	balanceLog.CreatedAt = settleTime
	if err := SaveBalanceLogTx(tx, balanceLog); err != nil {
		return 0, 0, err
	}

	// 3. 标记日志 settled
	if err := markAffLogsSettledTx(tx, pendingLogs, settleTime); err != nil {
		return 0, 0, err
	}
	return totalCents, len(pendingLogs), nil
}

func markAffLogsSettledTx(tx *gorm.DB, logs []model.AffCommissionLog, settleTime string) error {
	ids := make([]string, 0, len(logs))
	for _, l := range logs {
		ids = append(ids, l.ID)
	}
	return tx.Model(&model.AffCommissionLog{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"status":    model.AffCommissionStatusSettled,
			"settled_at": settleTime,
		}).Error
}

// ListAffCommissionLogsByInviter 分页查某邀请人的返佣流水。
func ListAffCommissionLogsByInviter(inviterID string, q model.Query) ([]model.AffCommissionLog, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	var total int64
	if err := db.Model(&model.AffCommissionLog{}).Where("inviter_id = ?", inviterID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.AffCommissionLog
	if err := db.Model(&model.AffCommissionLog{}).
		Where("inviter_id = ?", inviterID).
		Order("created_at desc").
		Offset(q.Offset()).Limit(q.PageSize).
		Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
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
