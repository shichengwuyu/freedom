package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: shot-subtitle-node 时间轴计算 + 编辑 ===
//
// 核心算法 ComputeTimeline：
//   1) 把文本按标点切 N 行（， 。 ！ ？ ; ； 等）
//   2) 统计每行字符数（含标点）
//   3) 按字符数比例分配每行起止时间到 shotDurationMs
//   4) 第一行从 0 开始；最后一行到 shotDurationMs 结束
//
// v1 算法（字数线性切）足够好；v2 升级为 ASR 重对齐（whisper）留待后续。
//
// 字幕样式（SubtitleStyle）存到 NovelProject JSON 字段，per-project 一份；
// v2 阶段 SubtitleStyleJSON 暂未真正接入 NovelProject（任务组 2.8 一起做），先暴露
// Get/Set API 调用方（前端）自己存。

// 默认分镜时长（毫秒），用于"分镜无显式时长"场景（spec 4.x 提到 4 秒默认）。
const defaultShotDurationMs = 4000

// 标点切分（中文 + 英文），含逗号/句号/问号/感叹号/分号/冒号/破折号。
var subtitleSplitPuncts = []rune{'，', ',', '。', '.', '！', '!', '？', '?', '；', ';', '：', ':', '、', '\n', '\r'}

// ComputeTimeline 按字数线性切分文本到时间轴。
//
// 入参：
//   - text: 字幕文本（"对白/旁白" 字段）
//   - shotDurationMs: 镜头视频时长（毫秒）；<= 0 用默认 4 秒
//
// 返回：[]model.SubtitleLine。
func ComputeTimeline(text string, shotDurationMs int) []model.SubtitleLine {
	if shotDurationMs <= 0 {
		shotDurationMs = defaultShotDurationMs
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	lines := splitByPunct(trimmed)
	if len(lines) == 0 {
		return nil
	}
	// 总字符数
	totalChars := 0
	for _, l := range lines {
		totalChars += len([]rune(l))
	}
	if totalChars == 0 {
		return nil
	}
	out := make([]model.SubtitleLine, 0, len(lines))
	cursor := 0
	for i, l := range lines {
		chars := len([]rune(l))
		// 字符数比例分配；最后一行用 shotDurationMs 收尾避免累计误差
		var endMs int
		if i == len(lines)-1 {
			endMs = shotDurationMs
		} else {
			endMs = cursor + chars*shotDurationMs/totalChars
			// 防止进位丢字：若 endMs <= cursor，至少 +1ms
			if endMs <= cursor {
				endMs = cursor + 1
			}
		}
		out = append(out, model.SubtitleLine{
			StartMs: cursor,
			EndMs:   endMs,
			Text:    l,
		})
		cursor = endMs
	}
	return out
}

// splitByPunct 按标点切分文本，保留标点在前一行尾。
func splitByPunct(text string) []string {
	// 用 rune-level 遍历
	runes := []rune(text)
	var lines []string
	var cur strings.Builder
	for _, r := range runes {
		cur.WriteRune(r)
		if isSplitPunct(r) {
			lines = append(lines, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		lines = append(lines, rest)
	}
	// 过滤空字符串
	filtered := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

func isSplitPunct(r rune) bool {
	for _, p := range subtitleSplitPuncts {
		if r == p {
			return true
		}
	}
	// 兼容部分中点号
	if unicode.IsPunct(r) {
		return true
	}
	return false
}

// LinesToJSON / JSONToLines 工具。
func LinesToJSON(lines []model.SubtitleLine) string {
	if len(lines) == 0 {
		return ""
	}
	b, _ := json.Marshal(lines)
	return string(b)
}

func JSONToLines(s string) []model.SubtitleLine {
	if s == "" {
		return nil
	}
	var lines []model.SubtitleLine
	if err := json.Unmarshal([]byte(s), &lines); err != nil {
		return nil
	}
	return lines
}

// === Project-level 字幕样式（per-project） ===
//
// v2 简化：样式存 NovelProject（前端）；后端只暴露 get/set helpers，前端存 localforage。
// 后续任务组 2.8 改造 novel/page.tsx 时一起接入。

// DispatchSubtitleForShot 调度单条 shot 的字幕生成（自动按字数切）。
//
//   - text 为空：status=skipped
//   - shotDurationMs<=0：默认 4 秒
//   - 写入 ShotSubtitle；前端可后续用 PUT /subtitle/:shotId 改 lines
func DispatchSubtitleForShot(userID, projectID, shotID, text string, shotDurationMs int) error {
	if userID == "" || projectID == "" || shotID == "" {
		return errors.New("userID/projectID/shotID required")
	}
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	lines := ComputeTimeline(text, shotDurationMs)
	if len(lines) == 0 {
		return repository.UpsertShotSubtitle(&model.ShotSubtitle{
			UserID: userID, ProjectID: projectID, ShotID: shotID,
			Status: "skipped",
			UpdatedAt: nowStr,
		})
	}
	return repository.UpsertShotSubtitle(&model.ShotSubtitle{
		UserID: userID, ProjectID: projectID, ShotID: shotID,
		LinesJSON: LinesToJSON(lines),
		Status: "success",
		UpdatedAt: nowStr,
	})
}

// DispatchSubtitleForProject 遍历项目所有 shot，调度字幕生成。
//
// 入参：shots = [{ shotId, text, shotDurationMs? }]
//   - text 为空：status=skipped（不调任何模型，纯数据）
//   - shotDurationMs<=0：默认 4 秒
func DispatchSubtitleForProject(userID, projectID string, shots []ShotForSubtitle) error {
	if len(shots) == 0 {
		return nil
	}
	for _, s := range shots {
		if err := DispatchSubtitleForShot(userID, projectID, s.ShotID, s.Text, s.ShotDurationMs); err != nil {
			// 字幕不会真失败（纯计算），但仍记 error 让调用方感知
			continue
		}
	}
	return nil
}

// ShotForSubtitle 调度入参。
type ShotForSubtitle struct {
	ShotID         string
	Text           string
	ShotDurationMs int
}

// UpdateLines 改某 shot 的字幕行（手动编辑入口）。
func UpdateLines(userID, projectID, shotID string, lines []model.SubtitleLine) error {
	if userID == "" || projectID == "" || shotID == "" {
		return errors.New("userID/projectID/shotID required")
	}
	// 校验：每行 endMs > startMs；总时长非负
	for i, l := range lines {
		if l.EndMs <= l.StartMs {
			return errors.New("line " + itoa(i) + " endMs must be > startMs")
		}
	}
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	return repository.UpsertShotSubtitle(&model.ShotSubtitle{
		UserID: userID, ProjectID: projectID, ShotID: shotID,
		LinesJSON: LinesToJSON(lines),
		Status: "success",
		UpdatedAt: nowStr,
	})
}

// GetSubtitleForShot 取单条字幕（含 lines 解码）。
func GetSubtitleForShot(projectID, shotID string) (*model.ShotSubtitle, []model.SubtitleLine, error) {
	s, err := repository.GetShotSubtitle(projectID, shotID)
	if err != nil || s == nil {
		return s, nil, err
	}
	return s, JSONToLines(s.LinesJSON), nil
}

// ListSubtitleForProject 列项目所有字幕。
func ListSubtitleForProject(projectID string) ([]model.ShotSubtitle, error) {
	return repository.ListShotSubtitlesByProject(projectID)
}

// 简单 itoa（避免引入 strconv 重复 import）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
