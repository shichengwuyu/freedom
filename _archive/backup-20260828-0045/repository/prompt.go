package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// PromptCategories 返回内置提示词分类的副本。
func PromptCategories() []model.PromptCategory {
	result := make([]model.PromptCategory, len(promptCategories))
	copy(result, promptCategories)
	return result
}

// PromptCategoryByCode 根据分类编码查找内置提示词分类。
func PromptCategoryByCode(category string) (model.PromptCategory, bool) {
	for _, item := range promptCategories {
		if item.Category == category {
			return item, true
		}
	}
	return model.PromptCategory{}, false
}

// ListPromptCategories 返回内置提示词分类。
func ListPromptCategories() ([]model.PromptCategory, error) {
	return PromptCategories(), nil
}

// ListPrompts 按查询条件返回提示词分页列表。
// 默认只返回 approved 状态的提示词；传入 q.Status="pending" 或 "rejected" 可查对应状态。
func ListPrompts(q model.Query) ([]model.Prompt, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.Prompt{})
	// 非审核管理查询，默认只返回已通过
	if q.Status == "" {
		tx = tx.Where("status = ?", model.PromptStatusApproved)
	} else {
		tx = tx.Where("status = ?", q.Status)
	}
	tx = applyPromptFilters(tx, q)

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []model.Prompt
	if err := tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	categories, _ := ListPromptCategories()
	githubURLs := map[string]string{}
	for _, item := range categories {
		githubURLs[item.Category] = item.GithubURL
	}
	for i := range items {
		items[i].GithubURL = githubURLs[items[i].Category]
	}
	return items, total, nil
}

// ListPendingPrompts 返回待审核提示词分页列表。
func ListPendingPrompts(page, pageSize int) ([]model.Prompt, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := db.Model(&model.Prompt{}).Where("status = ?", model.PromptStatusPending).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.Prompt
	if err := db.Where("status = ?", model.PromptStatusPending).Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListRejectedPrompts 返回被拒绝的提示词分页列表。
func ListRejectedPrompts(page, pageSize int) ([]model.Prompt, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := db.Model(&model.Prompt{}).Where("status = ?", model.PromptStatusRejected).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.Prompt
	if err := db.Where("status = ?", model.PromptStatusRejected).Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// SaveUserPrompt 保存用户提交的提示词，状态为 pending。
func SaveUserPrompt(item model.Prompt) (model.Prompt, error) {
	item.Status = model.PromptStatusPending
	item.SubmittedByID = "" // 由 service 层注入
	return SavePrompt(item)
}

// ApprovePrompt 通过审核。
func ApprovePrompt(id string, reviewerID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.Prompt{}).Where("id = ?", id).Updates(map[string]any{
		"status":      model.PromptStatusApproved,
		"reviewer_id": reviewerID,
		"updated_at":  gorm.Expr("now()"),
	}).Error
}

// RejectPrompt 拒绝审核。
func RejectPrompt(id string, reviewerID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.Prompt{}).Where("id = ?", id).Updates(map[string]any{
		"status":      model.PromptStatusRejected,
		"reviewer_id": reviewerID,
		"updated_at":  gorm.Expr("now()"),
	}).Error
}

// ListPromptTags 返回当前提示词查询条件下的全部标签。
func ListPromptTags(q model.Query) ([]string, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	q.Normalize()
	q.Tags = nil
	tx := applyPromptFilters(db.Model(&model.Prompt{}), q)

	var items []model.Prompt
	if err := tx.Select("tags").Find(&items).Error; err != nil {
		return nil, err
	}
	return promptTagsFromItems(items), nil
}

// SavePrompt 保存提示词，并在更新时保留原创建时间。
func SavePrompt(item model.Prompt) (model.Prompt, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	if saved, ok, err := findPrompt(db, item.ID); err != nil {
		return item, err
	} else if ok && item.CreatedAt == "" {
		item.CreatedAt = saved.CreatedAt
	}
	item.GithubURL = ""
	return item, db.Save(&item).Error
}

// DeletePrompt 删除指定提示词。
func DeletePrompt(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.Prompt{}, "id = ?", id).Error
}

// DeletePrompts 批量删除提示词。
func DeletePrompts(ids []string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.Prompt{}, "id IN ?", ids).Error
}

// ReplacePromptCategory 用远程同步结果替换整个提示词分类。
func ReplacePromptCategory(category model.PromptCategory, items []model.Prompt) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("category = ?", category.Category).Delete(&model.Prompt{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		// 远程同步入库的提示词直接标记为已通过审核，前台公开 API 默认只返回 approved 状态。
		for i := range items {
			items[i].Category = category.Category
			items[i].GithubURL = ""
			items[i].Status = model.PromptStatusApproved
		}
		return tx.Create(&items).Error
	})
}

// applyPromptFilters 应用提示词列表的搜索条件。
func applyPromptFilters(tx *gorm.DB, q model.Query) *gorm.DB {
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("title LIKE ? OR prompt LIKE ?", like, like)
	}
	if isActivePromptOption(q.Category) {
		tx = tx.Where("category = ?", q.Category)
	}
	return applyPromptTagsFilter(tx, q.Tags)
}

// findPrompt 根据 ID 查询提示词。
func findPrompt(db *gorm.DB, id string) (model.Prompt, bool, error) {
	item := model.Prompt{}
	err := db.Where("id = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Prompt{}, false, nil
	}
	return item, err == nil, err
}

// applyPromptTagsFilter 应用 JSON 标签条件。
func applyPromptTagsFilter(tx *gorm.DB, tags []string) *gorm.DB {
	if len(tags) == 0 {
		return tx
	}
	condition := tx.Session(&gorm.Session{NewDB: true})
	for _, tag := range tags {
		condition = condition.Or(promptJSONTagsContains(tx), tag)
	}
	return tx.Where(condition)
}

func promptTagsFromItems(items []model.Prompt) []string {
	seen := map[string]bool{}
	tags := []string{}
	for _, item := range items {
		for _, tag := range item.Tags {
			if tag != "" && !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// promptJSONTagsContains 返回提示词 tags 的 JSON 包含条件。
func promptJSONTagsContains(_ *gorm.DB) string {
	return "JSON_CONTAINS(tags, JSON_QUOTE(?))"
}

// isActivePromptOption 判断提示词筛选项有效状态。
func isActivePromptOption(value string) bool {
	return value != "" && value != "全部" && value != "all"
}
