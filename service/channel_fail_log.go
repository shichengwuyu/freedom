package service

import (
	"sync"
	"time"

	"github.com/tigerowo/freedom/model"
)

// ChannelFailLog 单条渠道失败诊断记录（Sprint 2 引入）。
//
// 用途：admin 在渠道页 / dashboard 排查"为什么这次请求走了别的渠道"。
// 存储：内存 ring buffer（最近 channelFailLogCapacity 条）；进程重启即清空。
// 不持久化：admin 真要排查就发请求触发；冷启动期数据本来就没意义。
type ChannelFailLog struct {
	ChannelID    string    `json:"channelId"`
	ChannelName  string    `json:"channelName"`
	Model        string    `json:"model"`
	Capability   string    `json:"capability,omitempty"`
	KeyIndex     int       `json:"keyIndex"`
	StatusCode   int       `json:"statusCode"` // 0 表示网络错误
	ErrorMessage string    `json:"errorMessage"`
	At           time.Time `json:"at"`
}

const channelFailLogCapacity = 1000

var (
	failLogMu   sync.Mutex
	failLogRing []ChannelFailLog // ring buffer（最旧在头部，最新在尾部）
)

// RecordChannelFail 写入一条失败记录。Sprint 2 内部由 MarkChannelFail 调用；admin 也能直接调。
func RecordChannelFail(channel *model.ModelChannel, keyIndex, statusCode int, errMsg string) {
	if channel == nil {
		return
	}
	entry := ChannelFailLog{
		ChannelID:    channel.ID,
		ChannelName:  channel.Name,
		Model:        "", // 由调用方通过 SetChannelFailModel 补全；这里 model 不知道
		KeyIndex:     keyIndex,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		At:           time.Now().UTC(),
	}
	failLogMu.Lock()
	defer failLogMu.Unlock()
	failLogRing = append(failLogRing, entry)
	if len(failLogRing) > channelFailLogCapacity {
		// 截断最旧的（保留最新 capacity 条）
		failLogRing = failLogRing[len(failLogRing)-channelFailLogCapacity:]
	}
}

// RecordChannelFailWithContext 与 RecordChannelFail 类似，但带上 model/capability。
// handler 调这个版本，记录更全的诊断信息。
func RecordChannelFailWithContext(channel *model.ModelChannel, keyIndex, statusCode int, errMsg, modelName, capability string) {
	if channel == nil {
		return
	}
	entry := ChannelFailLog{
		ChannelID:    channel.ID,
		ChannelName:  channel.Name,
		Model:        modelName,
		Capability:   capability,
		KeyIndex:     keyIndex,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		At:           time.Now().UTC(),
	}
	failLogMu.Lock()
	defer failLogMu.Unlock()
	failLogRing = append(failLogRing, entry)
	if len(failLogRing) > channelFailLogCapacity {
		failLogRing = failLogRing[len(failLogRing)-channelFailLogCapacity:]
	}
}

// ListChannelFailLogs 返回最近 limit 条（默认 100，最多 1000）。按时间倒序。
func ListChannelFailLogs(limit int) []ChannelFailLog {
	if limit <= 0 {
		limit = 100
	}
	if limit > channelFailLogCapacity {
		limit = channelFailLogCapacity
	}
	failLogMu.Lock()
	defer failLogMu.Unlock()
	n := len(failLogRing)
	if n == 0 {
		return []ChannelFailLog{}
	}
	out := make([]ChannelFailLog, 0, limit)
	// 倒序遍历（最新在前）
	for i := n - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, failLogRing[i])
	}
	return out
}
