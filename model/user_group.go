package model

import "time"

// UserGroup 用户组（Sprint 3 引入）。
//
// 用于阶梯定价：每个 group 有一个统一倍率（groupRatio），叠加到 ModelCost
// 上做最终扣费。倍率存在 settings.private.groupRatios（前端可配置）。
//
// 内置 4 个 group（main.go 启动期 seed）：
//   - "default"    所有人默认；不可删除（IsDefault=true）
//   - "plus"       PLUS 会员
//   - "pro"        PRO 会员
//   - "enterprise" 企业版
type UserGroup struct {
	ID          string    `json:"id" gorm:"primaryKey;type:varchar(64)"`
	Name        string    `json:"name" gorm:"type:varchar(64);uniqueIndex"`
	DisplayName string    `json:"displayName" gorm:"type:varchar(64)"`
	Sort        int       `json:"sort" gorm:"default:0"`
	IsDefault   bool      `json:"isDefault"`
	IsActive    bool      `json:"isActive" gorm:"default:true"`
	Remark      string    `json:"remark" gorm:"type:varchar(255)"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// 内置 group ID 常量（Sprint 3 seed 用；admin 改 groupRatios 时也用这些 key）。
const (
	UserGroupDefault    = "default"
	UserGroupPlus       = "plus"
	UserGroupPro        = "pro"
	UserGroupEnterprise = "enterprise"
)
