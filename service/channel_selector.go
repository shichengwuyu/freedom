package service

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

var (
	// ErrNoChannel 渠道选择器找不到可用渠道（capability 不匹配 / 全在 cooldown / 全被 exclude）。
	ErrNoChannel = errors.New("没有可用渠道")
	// ErrAllRetriesFailed retry 循环用完所有次数仍未成功。
	ErrAllRetriesFailed = errors.New("所有渠道均失败")

	// cooldownMap channelID → cooldownUntil（sync.Map 保护）
	cooldownMap sync.Map
)

// 渠道选择请求。RetryIndex=0 表示首次；失败时递增。
type ChannelSelectorRequest struct {
	Group             string
	Model             string
	Capability        string
	RetryIndex        int
	ExcludeChannelIDs map[string]bool
}

// 渠道选择结果。
type ChannelSelectorResult struct {
	Channel  *model.ModelChannel
	APIKey   string         // 实际用的 key（多 key 轮询结果）
	KeyIndex int            // 0-based
	Ref      model.ChannelRef
}

// PickChannelWithRetry 选一个可用渠道（含 cooldown 排除、exclude 排除）。
//
// 选择策略：
//  1) 从 abilityMap[group|model|capability] 取候选
//  2) 过滤：exclude / 冷却 / (channel 不存在)
//  3) 按 Priority 升序：找 "前 N 个不同 priority" 中的第 RetryIndex 个桶
//     （new-api 形态：每个 priority 用完所有同 priority 渠道后才升到下个 priority）
//  4) 同 priority 桶内按 Weight 随机
func PickChannelWithRetry(req ChannelSelectorRequest) (*ChannelSelectorResult, error) {
	refs := lookupAbilities(req)
	if len(refs) == 0 {
		return nil, ErrNoChannel
	}

	// 过滤
	candidates := make([]model.ChannelRef, 0, len(refs))
	now := time.Now()
	for _, r := range refs {
		if req.ExcludeChannelIDs[r.ChannelID] {
			continue
		}
		if until, ok := cooldownMap.Load(r.ChannelID); ok {
			if now.Before(until.(time.Time)) {
				continue
			}
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		return nil, ErrNoChannel
	}

	// 按 priority 升序排序（new-api 形态：第 RetryIndex 个不同 priority 桶）
	bucket := pickPriorityBucket(candidates, req.RetryIndex)
	if len(bucket) == 0 {
		return nil, ErrNoChannel
	}

	// 同桶内按 weight 随机
	totalWeight := 0
	for _, r := range bucket {
		totalWeight += r.Weight + 1 // +1 防 weight=0
	}
	if totalWeight <= 0 {
		totalWeight = len(bucket)
	}
	hit := rand.Intn(totalWeight)
	for _, r := range bucket {
		hit -= r.Weight + 1
		if hit < 0 {
			return resolveRef(r)
		}
	}
	// fallback（理论上不会到这里）
	return resolveRef(bucket[0])
}

// lookupAbilities 查索引；找不到 capability 时退化到 "default" 再查一次。
func lookupAbilities(req ChannelSelectorRequest) []model.ChannelRef {
	key := model.AbilityKey(req.Group, req.Model, req.Capability)
	refs := model.GetAbilitiesByKey(key)
	if len(refs) > 0 {
		return refs
	}
	if req.Capability != "" {
		refs = model.GetAbilitiesByKey(model.AbilityKey(req.Group, req.Model, "default"))
	}
	return refs
}

// pickPriorityBucket 把 candidates 按 priority 升序，选用第 RetryIndex 个桶。
//
// 桶划分规则（new-api `getPriority` 形态）：
//   - 不同 priority 视为不同桶
//   - 桶内可能有多个 channel（weight 不同）
//   - retry=N 用第 N 个桶；超出桶数时落到最后一个（最小 priority 桶）
//
// 简化：把 candidates 按 priority 升序分组，返回 RetryIndex 桶。
func pickPriorityBucket(candidates []model.ChannelRef, retryIndex int) []model.ChannelRef {
	if len(candidates) == 0 {
		return nil
	}
	// 按 priority 升序
	sorted := make([]model.ChannelRef, len(candidates))
	copy(sorted, candidates)
	// 简单插入排序（候选数一般 < 10）
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Priority < sorted[j-1].Priority; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	// 找不同 priority 的桶边界
	buckets := make([][]model.ChannelRef, 0)
	for _, r := range sorted {
		if len(buckets) == 0 || buckets[len(buckets)-1][0].Priority != r.Priority {
			buckets = append(buckets, []model.ChannelRef{r})
		} else {
			buckets[len(buckets)-1] = append(buckets[len(buckets)-1], r)
		}
	}
	idx := retryIndex
	if idx >= len(buckets) {
		idx = len(buckets) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return buckets[idx]
}

// resolveRef 把 ChannelRef 解析成完整 ModelChannel + 实际 key。
func resolveRef(r model.ChannelRef) (*ChannelSelectorResult, error) {
	channels, err := loadAllChannels()
	if err != nil {
		return nil, fmt.Errorf("load channels: %w", err)
	}
	for i := range channels {
		if channels[i].ID == r.ChannelID {
			keys := channels[i].ChannelKeys()
			if r.KeyIndex < 0 || r.KeyIndex >= len(keys) {
				r.KeyIndex = 0
			}
			return &ChannelSelectorResult{
				Channel:  &channels[i],
				APIKey:   keys[r.KeyIndex],
				KeyIndex: r.KeyIndex,
				Ref:      r,
			}, nil
		}
	}
	return nil, fmt.Errorf("channel not found: %s", r.ChannelID)
}

// loadAllChannels 从 settings 读全量渠道（每次选都重读 → admin 改 channels 后立即生效）。
//
// 优化方向：Sprint 2.5 可以做内存缓存 + SaveSettings 失效；当前数据量小（< 30）够用。
func loadAllChannels() ([]model.ModelChannel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	channels := normalizePrivateSetting(settings.Private).Channels
	return channels, nil
}

// MarkChannelFail 标记 channel 失败（按 StatusCodeMapping 决定是否进 cooldown）。
//
// 触发冷却后写一条诊断日志（带 model/capability 由 handler 在调 MarkChannelFail 之前补全）。
func MarkChannelFail(channel *model.ModelChannel, keyIndex, statusCode int) {
	if channel == nil {
		return
	}
	if !shouldTriggerCooldown(channel, statusCode) {
		return
	}
	cooldown := channel.CooldownSeconds
	if cooldown <= 0 {
		cooldown = 60
	}
	cooldownMap.Store(channel.ID, time.Now().Add(time.Duration(cooldown)*time.Second))
}

// shouldTriggerCooldown 状态码命中 StatusCodeMapping 时才冷却。
//
// 默认行为：StatusCodeMapping 为空时，触发条件 = "网络错（statusCode==0）或 HTTP 429 或 5xx"。
// 自定义行为：admin 配 "429,500,502" 时只命中这三个；其他 4xx（401/403/404/400）不冷却。
func shouldTriggerCooldown(channel *model.ModelChannel, statusCode int) bool {
	if statusCode == 0 {
		return true
	}
	if channel == nil || strings.TrimSpace(channel.StatusCodeMapping) == "" {
		return statusCode == 429 || (statusCode >= 500 && statusCode <= 599)
	}
	for _, codeStr := range strings.Split(channel.StatusCodeMapping, ",") {
		codeStr = strings.TrimSpace(codeStr)
		if codeStr == "" {
			continue
		}
		if n, err := strconv.Atoi(codeStr); err == nil && n == statusCode {
			return true
		}
	}
	return false
}

// ClearAllCooldowns 清空所有 channel 的冷却（admin 调试用）。
func ClearAllCooldowns() int {
	count := 0
	cooldownMap.Range(func(k, v any) bool {
		cooldownMap.Delete(k)
		count++
		return true
	})
	return count
}

// LoadAllCooldownsSnapshot 返回 cooldownMap 的快照（channelID → cooldownUntil）。
// 用途：Sprint 2.6 渠道健康度页面展示"当前哪些 channel 在冷却"。
// 返回 map 已过期值过滤；admin 看到的 remaining 秒数才是真实的。
func LoadAllCooldownsSnapshot() map[string]time.Time {
	out := make(map[string]time.Time)
	now := time.Now()
	cooldownMap.Range(func(k, v any) bool {
		until, ok := v.(time.Time)
		if !ok {
			return true
		}
		if now.Before(until) {
			out[k.(string)] = until
		} else {
			// 顺手清理过期项（防止 cooldownMap 无限增长）
			cooldownMap.Delete(k)
		}
		return true
	})
	return out
}

// BuildAbilityCache 从 settings.private.channels 重建 abilities 倒排索引。
//
// 调用时机：main.go 启动期；service.SaveSettings 改 channels 后。
// 内部用 model.SetAbilityMap（自身持锁）；不阻塞主路径（启动期调用前不发请求）。
func BuildAbilityCache() error {
	settings, err := repository.GetSettings()
	if err != nil {
		return err
	}
	channels := normalizePrivateSetting(settings.Private).Channels

	newMap := make(map[string][]model.ChannelRef)
	for i := range channels {
		ch := &channels[i]
		if !ch.Enabled {
			continue
		}
		keys := ch.ChannelKeys()
		for _, modelName := range ch.Models {
			capabilities := []string{ch.Capability}
			if strings.TrimSpace(ch.Capability) == "" {
				capabilities = []string{"default"}
			}
			for _, cap := range capabilities {
				for ki := range keys {
					k := model.AbilityKey("", modelName, cap)
					newMap[k] = append(newMap[k], model.ChannelRef{
						ChannelID: ch.ID,
						KeyIndex:  ki,
						Priority:  ch.Priority,
						Weight:    ch.Weight,
					})
				}
			}
		}
	}

	model.SetAbilityMap(newMap)
	return nil
}
