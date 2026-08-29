package handler

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tigerowo/freedom/service"
)

// channelHealthSummary 渠道健康度汇总（KPI 卡片用）
type channelHealthSummary struct {
	TotalFailures            int `json:"totalFailures"`
	UniqueChannels           int `json:"uniqueChannels"`
	UniqueModels             int `json:"uniqueModels"`
	LongestCooldownRemaining int `json:"longestCooldownRemaining"` // 秒；0=无冷却
}

// channelHealthItem 单渠道聚合（按 failureCount 倒序）
type channelHealthItem struct {
	ChannelID         string    `json:"channelId"`
	ChannelName       string    `json:"channelName"`
	FailureCount      int       `json:"failureCount"`
	LastFailureAt     time.Time `json:"lastFailureAt"`
	LastStatusCode    int       `json:"lastStatusCode"`
	IsInCooldown      bool      `json:"isInCooldown"`
	CooldownRemaining int       `json:"cooldownRemaining"` // 秒；0=未冷却
	AffectedModels    []string  `json:"affectedModels"`
}

type channelsHealthResponse struct {
	Summary        channelHealthSummary     `json:"summary"`
	Channels       []channelHealthItem      `json:"channels"`
	RecentFailures []service.ChannelFailLog `json:"recentFailures"`
	Now            time.Time                `json:"now"`
}

// AdminChannelsHealth 一次性返回 admin 渠道健康度页面所需的所有数据（Sprint 2.6 引入）。
//
// 数据来源：
//   - service.ListChannelFailLogs(1000)：ring buffer 全量（1000 条上限，admin 监控"最近状态"）
//   - service.LoadAllCooldownsSnapshot：sync.Map 快照（key=channelID，value=cooldownUntil）
//   - service.AdminSettings：查 channel 名字（cooldown 里只有 ID）
func AdminChannelsHealth(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	fails := service.ListChannelFailLogs(1000)
	cooldowns := service.LoadAllCooldownsSnapshot()
	channelNames := loadChannelNames()

	// 按 channelID 聚合
	type aggregate struct {
		count         int
		lastAt        time.Time
		lastCode      int
		affectedSet   map[string]struct{}
		cooldownUntil time.Time
		hasCooldown   bool
	}
	aggByID := make(map[string]*aggregate)
	uniqueModels := make(map[string]struct{})

	for _, f := range fails {
		if _, ok := aggByID[f.ChannelID]; !ok {
			aggByID[f.ChannelID] = &aggregate{affectedSet: make(map[string]struct{})}
		}
		a := aggByID[f.ChannelID]
		a.count++
		if f.At.After(a.lastAt) {
			a.lastAt = f.At
			a.lastCode = f.StatusCode
		}
		if strings.TrimSpace(f.Model) != "" {
			a.affectedSet[f.Model] = struct{}{}
			uniqueModels[f.Model] = struct{}{}
		}
	}

	// 合并 cooldown 状态
	for channelID, until := range cooldowns {
		if _, ok := aggByID[channelID]; !ok {
			aggByID[channelID] = &aggregate{affectedSet: make(map[string]struct{})}
		}
		aggByID[channelID].cooldownUntil = until
		aggByID[channelID].hasCooldown = true
	}

	// 转结构 + 排序
	items := make([]channelHealthItem, 0, len(aggByID))
	longestCooldown := 0
	for id, a := range aggByID {
		isInCooldown := false
		remaining := 0
		if a.hasCooldown {
			r := int(a.cooldownUntil.Sub(now).Seconds())
			if r > 0 {
				isInCooldown = true
				remaining = r
				if r > longestCooldown {
					longestCooldown = r
				}
			}
		}
		models := make([]string, 0, len(a.affectedSet))
		for m := range a.affectedSet {
			models = append(models, m)
		}
		sort.Strings(models)

		name := channelNames[id]
		if name == "" {
			name = "Unknown"
		}

		items = append(items, channelHealthItem{
			ChannelID:         id,
			ChannelName:       name,
			FailureCount:      a.count,
			LastFailureAt:     a.lastAt,
			LastStatusCode:    a.lastCode,
			IsInCooldown:      isInCooldown,
			CooldownRemaining: remaining,
			AffectedModels:    models,
		})
	}
	// 按 FailureCount 倒序（同 count 按 LastFailureAt 倒序）
	sort.Slice(items, func(i, j int) bool {
		if items[i].FailureCount != items[j].FailureCount {
			return items[i].FailureCount > items[j].FailureCount
		}
		return items[i].LastFailureAt.After(items[j].LastFailureAt)
	})

	summary := channelHealthSummary{
		TotalFailures:            len(fails),
		UniqueChannels:           len(items),
		UniqueModels:             len(uniqueModels),
		LongestCooldownRemaining: longestCooldown,
	}

	// 最近失败列表（取最近 100 条；ListChannelFailLogs 已按时间倒序）
	recent := fails
	if len(recent) > 100 {
		recent = recent[:100]
	}

	OK(w, channelsHealthResponse{
		Summary:        summary,
		Channels:       items,
		RecentFailures: recent,
		Now:            now,
	})
}

// loadChannelNames 从 settings 读 channel ID → name 映射。Sprint 2.6 内部 helper。
func loadChannelNames() map[string]string {
	settings, err := service.AdminSettings()
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(settings.Private.Channels))
	for _, ch := range settings.Private.Channels {
		if strings.TrimSpace(ch.ID) == "" {
			continue
		}
		out[ch.ID] = ch.Name
	}
	return out
}

// AdminClearCooldowns 一键清空所有渠道的冷却状态（Sprint 2.6 引入）。
// 返回清空数量。
func AdminClearCooldowns(w http.ResponseWriter, r *http.Request) {
	cleared := service.ClearAllCooldowns()
	OK(w, map[string]any{"cleared": cleared})
}
