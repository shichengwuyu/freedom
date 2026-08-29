package service

import (
	"sync"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// promptTagsCache 进程内缓存：提示词标签全量列表。
// 标签极少变动，列表页不需要强一致，默认缓存 5 分钟可大幅降低 DB 全表扫描。
var (
	promptTagsCacheMu      sync.RWMutex
	promptTagsCacheValue   []string
	promptTagsCacheExpire  time.Time
	promptTagsCacheTTL     = 5 * time.Minute
	promptCategoryCacheValue  []string
	promptCategoryCacheExpire time.Time
)

// getCachedPromptTags 返回缓存的全量标签；过期或未命中时才查 DB。
func getCachedPromptTags(q model.Query) ([]string, error) {
	// 只有不带 keyword 和 category 的默认查询才走缓存（这是进入页面时的首次请求）
	useCache := q.Keyword == "" && q.Category == "" && (len(q.Tags) == 0 || (len(q.Tags) == 1 && (q.Tags[0] == "" || q.Tags[0] == "全部" || q.Tags[0] == "all")))

	if useCache {
		promptTagsCacheMu.RLock()
		cached := promptTagsCacheValue
		expire := promptTagsCacheExpire
		promptTagsCacheMu.RUnlock()
		if len(cached) > 0 && time.Now().Before(expire) {
			return cached, nil
		}
	}

	tags, err := repository.ListPromptTags(q)
	if err != nil {
		return nil, err
	}

	if useCache && len(tags) > 0 {
		promptTagsCacheMu.Lock()
		promptTagsCacheValue = tags
		promptTagsCacheExpire = time.Now().Add(promptTagsCacheTTL)
		promptTagsCacheMu.Unlock()
	}
	return tags, nil
}

// getCachedPromptCategories 返回缓存的分类列表。
func getCachedPromptCategories() []string {
	promptTagsCacheMu.RLock()
	cached := promptCategoryCacheValue
	expire := promptCategoryCacheExpire
	promptTagsCacheMu.RUnlock()
	if len(cached) > 0 && time.Now().Before(expire) {
		return cached
	}
	codes := promptCategoryCodes(ListPromptCategories())
	if len(codes) > 0 {
		promptTagsCacheMu.Lock()
		promptCategoryCacheValue = codes
		promptCategoryCacheExpire = time.Now().Add(promptTagsCacheTTL)
		promptTagsCacheMu.Unlock()
	}
	return codes
}

// InvalidatePromptTagsCache 写操作后主动清理缓存（新增/删除提示词时调用）。
func InvalidatePromptTagsCache() {
	promptTagsCacheMu.Lock()
	promptTagsCacheValue = nil
	promptTagsCacheExpire = time.Time{}
	promptCategoryCacheValue = nil
	promptCategoryCacheExpire = time.Time{}
	promptTagsCacheMu.Unlock()
}

func ListPrompts(q model.Query) (model.PromptList, error) {
	items, total, err := repository.ListPrompts(q)
	if err != nil {
		return model.PromptList{}, err
	}
	tags, err := getCachedPromptTags(q)
	if err != nil {
		return model.PromptList{}, err
	}
	categories := getCachedPromptCategories()
	return model.PromptList{Items: items, Tags: tags, Categories: categories, Total: int(total)}, nil
}

func ListPromptCategories() []model.PromptCategory {
	categories, _ := repository.ListPromptCategories()
	return categories
}

func SavePrompt(item model.Prompt) (model.Prompt, error) {
	now := time.Now().Format(time.RFC3339)
	if item.Category == "" {
		item.Category = repository.PromptCategories()[0].Category
	}
	if item.ID == "" {
		item.ID = newID(item.Category)
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	category, ok := repository.PromptCategoryByCode(item.Category)
	if !ok {
		category = repository.PromptCategories()[0]
		item.Category = category.Category
	}
	item.GithubURL = ""
	saved, err := repository.SavePrompt(item)
	if err == nil {
		InvalidatePromptTagsCache()
	}
	return saved, err
}

func DeletePrompt(id string) error {
	err := repository.DeletePrompt(id)
	if err == nil {
		InvalidatePromptTagsCache()
	}
	return err
}

func DeletePrompts(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	err := repository.DeletePrompts(ids)
	if err == nil {
		InvalidatePromptTagsCache()
	}
	return err
}

// SubmitPrompt 保存用户提交的提示词，状态为 pending。
func SubmitPrompt(item model.Prompt, userID string) (model.Prompt, error) {
	now := time.Now().Format(time.RFC3339)
	if item.Category == "" {
		item.Category = repository.PromptCategories()[0].Category
	}
	if item.ID == "" {
		item.ID = newID(item.Category)
	}
	item.Status = model.PromptStatusPending
	item.SubmittedByID = userID
	item.CreatedAt = now
	item.UpdatedAt = now
	category, ok := repository.PromptCategoryByCode(item.Category)
	if !ok {
		category = repository.PromptCategories()[0]
		item.Category = category.Category
	}
	item.GithubURL = ""
	saved, err := repository.SavePrompt(item)
	if err == nil {
		InvalidatePromptTagsCache()
	}
	return saved, err
}

// ListPendingPrompts 返回待审核分页列表。
func ListPendingPrompts(page, pageSize int) ([]model.Prompt, int64, error) {
	return repository.ListPendingPrompts(page, pageSize)
}

// ListRejectedPrompts 返回被拒绝分页列表。
func ListRejectedPrompts(page, pageSize int) ([]model.Prompt, int64, error) {
	return repository.ListRejectedPrompts(page, pageSize)
}

// ApprovePrompt 通过审核。
func ApprovePrompt(id string, reviewerID string) error {
	err := repository.ApprovePrompt(id, reviewerID)
	if err == nil {
		InvalidatePromptTagsCache()
	}
	return err
}

// RejectPrompt 拒绝审核。
func RejectPrompt(id string, reviewerID string) error {
	err := repository.RejectPrompt(id, reviewerID)
	if err == nil {
		InvalidatePromptTagsCache()
	}
	return err
}

func promptCategoryCodes(items []model.PromptCategory) []string {
	codes := []string{}
	for _, item := range items {
		if item.Category != "" {
			codes = append(codes, item.Category)
		}
	}
	return codes
}
