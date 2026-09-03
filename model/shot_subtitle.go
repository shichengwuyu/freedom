package model

// ShotSubtitle novel 工作流 - 镜头字幕节点的数据模型。
//
// 每个 shot 最多 1 条 ShotSubtitle（per-project 唯一性由 service 层保证）：
//   - 自动按"对白/旁白"字数线性切分到视频时长
//   - 用户可在合成前手动编辑（改文字 / 拖动起止 / 增删行）
//   - 字幕样式（字体/颜色/描边/位置）在整部成片级别统一，存到 SubtitleStyleJSON（per-project）
//
// 与 ShotDubbing 区别：字幕不调任何 AI 模型，是纯数据 + 编辑器。
// 烧录到 mp4 由 composition-layer 的 ffmpeg ass filter 完成（任务组 6）。
type ShotSubtitle struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"index"`
	ProjectID string `json:"projectId" gorm:"index"`
	ShotID    string `json:"shotId" gorm:"index"`

	// 字幕时间轴：JSON 数组字符串
	// Line = { startMs int, endMs int, text string }
	// 例：[{"startMs":0,"endMs":1500,"text":"第一句"},{"startMs":1500,"endMs":3000,"text":"第二句"}]
	LinesJSON string `json:"linesJson" gorm:"type:longtext"`

	// 状态：空 (未生成) | success (有 lines) | skipped (文本为空)
	Status string `json:"status" gorm:"index"`
	Error  string `json:"error" gorm:"type:text"`

	CreatedAt string `json:"createdAt" gorm:"index"`
	UpdatedAt string `json:"updatedAt" gorm:"index"`
}

// SubtitleStyle 整部成片级别字幕样式（不存表，存在 NovelProject 上；这里只定义类型）。
//
// v2 简化：样式存到 NovelProject JSON 字段；本表只存 per-shot 字幕行。
// 字段：
//   - font: 字体名（"黑体"/"微软雅黑"/"Source Han Sans" 等）
//   - size: 字号（24-72）
//   - color: 颜色 hex（"FFFFFF" 白）
//   - outline: 描边 hex（"000000" 黑）
//   - outlineWidth: 描边宽度（1-5）
//   - background: 背景色 hex（""=无背景；"80000000" 半透明黑底）
//   - position: 位置（"bottom"/"center"/"top"）
//   - marginBottom: 距底距离像素（0-200）
type SubtitleStyle struct {
	Font          string `json:"font"`
	Size          int    `json:"size"`
	Color         string `json:"color"`
	Outline       string `json:"outline"`
	OutlineWidth  int    `json:"outlineWidth"`
	Background    string `json:"background"`
	Position      string `json:"position"`
	MarginBottom  int    `json:"marginBottom"`
}

// DefaultSubtitleStyle 默认字幕样式（白字 + 黑描边 + 底部居中）。
func DefaultSubtitleStyle() SubtitleStyle {
	return SubtitleStyle{
		Font:         "黑体",
		Size:         36,
		Color:        "FFFFFF",
		Outline:      "000000",
		OutlineWidth: 2,
		Background:   "",
		Position:     "bottom",
		MarginBottom: 60,
	}
}

// SubtitleLine 一行字幕。
type SubtitleLine struct {
	StartMs int    `json:"startMs"`
	EndMs   int    `json:"endMs"`
	Text    string `json:"text"`
}
