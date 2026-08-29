package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// ensureVendorsTableReady 取 DB() 确保表已由 AutoMigrate 创建，并返回 *gorm.DB；统一封装避免每处重复写 db,err。
func ensureVendorsTableReady() (*gorm.DB, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	return db, nil
}

// newVendorID 生成 Vendor 主键（P0 简单用 UUIDv4；后续可替换为稳定哈希）
func newVendorID() string {
	return "vt_" + uuid.NewString()[:16]
}

// newUserVendorAccountID 生成用户供应商账户主键
func newUserVendorAccountID() string {
	return "va_" + uuid.NewString()[:16]
}

// ================ Vendor（系统级供应商）查询 ================

// ListAllVendors 查询所有供应商（按 Sort 正序、Type 再排，管理员/前端共用）
func ListAllVendors() ([]model.Vendor, error) {
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	var vendors []model.Vendor
	if e := db.Order("sort asc, type asc").Find(&vendors).Error; e != nil {
		return nil, e
	}
	return vendors, nil
}

// ListEnabledVendors 仅返回启用状态的供应商（前端下拉用）
func ListEnabledVendors() ([]model.Vendor, error) {
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	// Limit(model.MaxPageSize)兜底：当前内置 4 家，不会超；但万一用户后台加了一堆自定义 vendor 也保护前端。
	var vendors []model.Vendor
	if e := db.Where("enabled = ?", true).Order("sort asc, type asc").Limit(model.MaxPageSize).Find(&vendors).Error; e != nil {
		return nil, e
	}
	return vendors, nil
}

// GetVendorByType 按类型查询（业务依赖的唯一标识；同类型理论上只有一条）
func GetVendorByType(t string) (*model.Vendor, error) {
	if !model.ValidVendorType(t) {
		return nil, errors.New("供应商类型不合法")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	var v model.Vendor
	if e := db.Where("type = ?", t).First(&v).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, e
	}
	return &v, nil
}

// GetVendorByID 按主键查供应商
func GetVendorByID(id string) (*model.Vendor, error) {
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	var v model.Vendor
	if e := db.Where("id = ?", id).First(&v).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, e
	}
	return &v, nil
}

// SaveVendor 保存供应商（新增或更新全字段）
// Save 时自动填充 ID（新增）和时间戳
func SaveVendor(v model.Vendor) (model.Vendor, error) {
	db, err := ensureVendorsTableReady()
	if err != nil {
		return v, err
	}
	now := time.Now()
	if v.ID == "" {
		v.ID = newVendorID()
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if e := db.Save(&v).Error; e != nil {
		return v, e
	}
	return v, nil
}

// ================ UserVendorAccount（用户绑定账户）查询 ================

// ListUserVendorAccounts 查询某用户的全部绑定账户（按 LastUsedAt 倒序，激活的放最前）
func ListUserVendorAccounts(userID string) ([]model.UserVendorAccount, error) {
	if userID == "" {
		return nil, errors.New("userID 不能为空")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	var accounts []model.UserVendorAccount
	// 先按 IsActive 倒序（true 在前），再按 LastUsedAt 倒序，最后绑定时间倒序
	if e := db.Where("user_id = ?", userID).
		Order("is_active desc, last_used_at desc, bound_at desc").
		Find(&accounts).Error; e != nil {
		return nil, e
	}
	return accounts, nil
}

// GetActiveVendorAccount 获取用户当前激活的供应商账户（唯一一条 IsActive=true；若不存在则返回 nil,nil）
func GetActiveVendorAccount(userID string) (*model.UserVendorAccount, error) {
	if userID == "" {
		return nil, errors.New("userID 不能为空")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	var account model.UserVendorAccount
	if e := db.Where("user_id = ? AND is_active = ?", userID, true).
		First(&account).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, e
	}
	return &account, nil
}

// GetUserVendorAccountByType 查询用户绑定的某一类型账户（存在则返回，不存在 nil）
func GetUserVendorAccountByType(userID string, vendorType string) (*model.UserVendorAccount, error) {
	if userID == "" || !model.ValidVendorType(vendorType) {
		return nil, errors.New("参数不合法")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	var account model.UserVendorAccount
	if e := db.Where("user_id = ? AND vendor_type = ?", userID, vendorType).
		First(&account).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, e
	}
	return &account, nil
}

// SaveUserVendorAccount 保存账户；新增时填充 ID 和时间戳
func SaveUserVendorAccount(a model.UserVendorAccount) (model.UserVendorAccount, error) {
	db, err := ensureVendorsTableReady()
	if err != nil {
		return a, err
	}
	now := time.Now()
	if a.ID == "" {
		a.ID = newUserVendorAccountID()
		a.CreatedAt = now
		if a.BoundAt.IsZero() {
			a.BoundAt = now
		}
	}
	a.UpdatedAt = now
	if a.LastUsedAt.IsZero() {
		a.LastUsedAt = now
	}
	if e := db.Save(&a).Error; e != nil {
		return a, e
	}
	return a, nil
}

// ActivateUserVendorAccount 将用户某供应商账户设为激活，同时其他账户全部置为非激活（事务保证一致性）
// 若传入 accountID 为空，表示"切回官方模式"，此时只把所有账户置 inactive。
//
// 提交后立刻回读校验（防御性修复）：用户实践发现 200 OK 返回但 DB is_active 没生效的 silent fail。
// 校验失败 → 返回 error 让 caller surface，事务会被回滚。
func ActivateUserVendorAccount(userID string, accountID string) error {
	if userID == "" {
		return errors.New("userID 不能为空")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return err
	}
	// accountID 空 = 切回官方模式，不需要写激活，但仍然把全部账户置 inactive，并校验至少一条改动。
	if accountID == "" {
		return db.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&model.UserVendorAccount{}).
				Where("user_id = ?", userID).
				Update("is_active", false)
			if res.Error != nil {
				return res.Error
			}
			return nil
		})
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		// 1. 先把该用户所有账户统一置 inactive
		if e := tx.Model(&model.UserVendorAccount{}).
			Where("user_id = ?", userID).
			Update("is_active", false).Error; e != nil {
			return e
		}
		// 2. 把指定 accountID 标为 active，并校验归属
		res := tx.Model(&model.UserVendorAccount{}).
			Where("id = ? AND user_id = ?", accountID, userID).
			Update("is_active", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("未找到对应供应商账户或无权限")
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 3. 提交后立刻回读校验，避免 silent fail（用户原话："activate 返回 200 但 DB 没生效"）。
	//    用一个全新查询绕过可能的语句缓存/事务可见性问题。
	var activeCount int64
	if e := db.Model(&model.UserVendorAccount{}).
		Where("id = ? AND user_id = ? AND is_active = ?", accountID, userID, true).
		Count(&activeCount).Error; e != nil {
		return fmt.Errorf("激活提交成功但回读校验失败: %w", e)
	}
	if activeCount == 0 {
		return errors.New("激活事务已提交但回读不到 is_active=true，疑似 silent fail，请重试或检查 MySQL 事务隔离级别")
	}
	return nil
}

// TouchUserVendorAccountLastUsed 更新"最近使用时间"为 now（每次成功调用该供应商 API 时打一下）
func TouchUserVendorAccountLastUsed(accountID string) error {
	if accountID == "" {
		return nil
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return err
	}
	return db.Model(&model.UserVendorAccount{}).
		Where("id = ?", accountID).
		Update("last_used_at", time.Now()).Error
}

// DeleteUserVendorAccountByID 解绑（真删一条账户记录）；若删的恰好是 active 那条，其他保持 inactive，前端会自然回落到 official
func DeleteUserVendorAccountByID(userID string, accountID string) error {
	if userID == "" || accountID == "" {
		return errors.New("参数缺失")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return err
	}
	res := db.Where("id = ? AND user_id = ?", accountID, userID).
		Delete(&model.UserVendorAccount{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("未找到对应供应商账户")
	}
	return nil
}
