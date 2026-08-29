package service

import (
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// SeedDefaultUserGroups 启动期 seed 4 个内置 group（Sprint 3 引入）。
//
// 设计要点：
//   - 已有 group 的 displayName / sort 会被刷新（运营改 displayName 后代码改值能自动同步）
//   - 已删的 group 不会被重建（避免覆盖 admin 的删除）
//   - 失败不阻塞启动（启动期只 log warning）
func SeedDefaultUserGroups() error {
	defaults := []model.UserGroup{
		{ID: model.UserGroupDefault, Name: "default", DisplayName: "默认", Sort: 0, IsDefault: true, IsActive: true, Remark: "所有新用户默认归属"},
		{ID: model.UserGroupPlus, Name: "plus", DisplayName: "PLUS", Sort: 10, IsDefault: false, IsActive: true, Remark: "PLUS 会员，统一 8 折"},
		{ID: model.UserGroupPro, Name: "pro", DisplayName: "PRO", Sort: 20, IsDefault: false, IsActive: true, Remark: "PRO 会员，统一 6 折"},
		{ID: model.UserGroupEnterprise, Name: "enterprise", DisplayName: "Enterprise", Sort: 30, IsDefault: false, IsActive: true, Remark: "企业版，统一 4 折"},
	}
	for i := range defaults {
		want := defaults[i]
		existing, err := repository.GetUserGroupByID(want.ID)
		if err != nil {
			return err
		}
		if existing == nil {
			// 新建
			if err := repository.SaveUserGroup(&want); err != nil {
				return err
			}
		} else if existing.DisplayName != want.DisplayName || existing.Sort != want.Sort || existing.Remark != want.Remark {
			// 字段被代码改过 → 同步（不覆盖 admin 改过的 isActive=false 等"软管理"状态）
			existing.DisplayName = want.DisplayName
			existing.Sort = want.Sort
			existing.Remark = want.Remark
			_ = repository.SaveUserGroup(existing)
		}
	}
	return nil
}

// ListActiveUserGroups 公开 pricing API 用。
func ListActiveUserGroups() ([]model.UserGroup, error) {
	return repository.ListActiveUserGroups()
}
