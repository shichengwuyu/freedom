package model

import "sync"

// ChannelRef 渠道引用（ability 索引中的轻量引用，避免在索引里持有 ModelChannel 大对象）。
//
// Sprint 2 简化：
//   - Group 字段暂不强校验（所有 enabled channel 都在 default group）
//   - 后续 Sprint 3 引入 UserGroup 时，把 Group 字段加到索引 key 里
type ChannelRef struct {
	ChannelID string
	KeyIndex  int // 0-based；多 key 模式下当前用第几个 key
	Priority  int // 数字小=优先
	Weight    int // 同 priority 桶内按 weight 随机
}

var (
	abilityMap  map[string][]ChannelRef // key = "group|model|capability"
	abilityLock sync.RWMutex
)

// AbilityKey 构造 abilities 索引的 key。空值回退到 "default"。
func AbilityKey(group, modelName, capability string) string {
	if group == "" {
		group = "default"
	}
	if capability == "" {
		capability = "default"
	}
	return group + "|" + modelName + "|" + capability
}

// GetAbilitiesByKey 读索引（RLock）。返回的切片是只读副本或共享只读，调用方不要修改。
func GetAbilitiesByKey(key string) []ChannelRef {
	abilityLock.RLock()
	defer abilityLock.RUnlock()
	return abilityMap[key]
}

// SnapshotAbilities 返回索引的浅拷贝（用于调试/admin，不阻塞主路径）。
func SnapshotAbilities() map[string][]ChannelRef {
	abilityLock.RLock()
	defer abilityLock.RUnlock()
	out := make(map[string][]ChannelRef, len(abilityMap))
	for k, v := range abilityMap {
		out[k] = v
	}
	return out
}

// ClearAbilityCache 清空索引。下一次 GetAbilitiesByKey 不会自动重建——
// 调度由调用方（BuildAbilityCache 启动期 / SaveSettings 失效后）显式触发。
func ClearAbilityCache() {
	abilityLock.Lock()
	abilityMap = nil
	abilityLock.Unlock()
}

// SetAbilityMap 由 service.BuildAbilityCache 调用。整张 map 替换（非合并）。
// 调用方负责：1) 拿 lock；2) 不在主路径并发写。
func SetAbilityMap(m map[string][]ChannelRef) {
	abilityLock.Lock()
	abilityMap = m
	abilityLock.Unlock()
}
