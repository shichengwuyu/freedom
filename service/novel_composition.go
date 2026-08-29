package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: composition-layer: 5 步 ffmpeg 合成 ===
//
// 5 步：
//   1. 归一化所有镜头视频 → h264/aac/yuv420p
//   2. 按分镜顺序拼接（concat demuxer）
//   3. 混音（配音 + BGM；amix filter）
//   4. 烧字幕（ass filter；libass）
//   5. 输出最终 mp4
//
// v2 阶段：real ffmpeg 调用已写好（RunFfmpegWithProgress），但每个步骤的具体参数
//   仍依赖 ffmpeg 二进制在镜像中存在。开发机可独立 ffmpeg 测试。
//
// 简化：
//   - 输入数据通过 CompositionInput JSON 传入
//   - 视频文件路径：v2 用本地路径（dev 模式）；生产应改为对象存储 URL + 下载到临时目录

// CompositionInput 合成输入快照。
type CompositionInput struct {
	ShotVideos    []CompositionShotVideo    `json:"shotVideos"`
	ShotDubbings  []CompositionShotDubbing  `json:"shotDubbings"`
	ShotSubtitles []CompositionShotSubtitle `json:"shotSubtitles"`
	BGMSource     CompositionBGMSource      `json:"bgmSource"`
	BGMVolume     float64                   `json:"bgmVolume"`
	BGMFadeInMs   int                       `json:"bgmFadeInMs"`
	BGMFadeOutMs  int                       `json:"bgmFadeOutMs"`
	SubtitleStyle SubtitleStyleJSON         `json:"subtitleStyle"`
}

// CompositionShotVideo 单条镜头视频输入。
type CompositionShotVideo struct {
	ShotID     string `json:"shotId"`
	URL        string `json:"url"`
	DurationMs int    `json:"durationMs"`
}

// CompositionShotDubbing 单条配音输入。
type CompositionShotDubbing struct {
	ShotID     string `json:"shotId"`
	URL        string `json:"url"`
	DurationMs int64  `json:"durationMs"`
}

// CompositionShotSubtitle 单条字幕输入。
type CompositionShotSubtitle struct {
	ShotID string             `json:"shotId"`
	Lines  []model.SubtitleLine `json:"lines"`
}

// CompositionBGMSource BGM 来源（预设或自定义）。
type CompositionBGMSource struct {
	PresetID string `json:"presetId,omitempty"`
	CustomID string `json:"customId,omitempty"`
}

// SubtitleStyleJSON 字幕样式（ass filter 友好的字段）。
type SubtitleStyleJSON struct {
	Font         string `json:"font"`
	Size         int    `json:"size"`
	Color        string `json:"color"`
	Outline      string `json:"outline"`
	OutlineWidth int    `json:"outlineWidth"`
	Background   string `json:"background"`
	Position     string `json:"position"`
	MarginBottom int    `json:"marginBottom"`
}

// compositionContext 合成过程临时状态。
type compositionContext struct {
	taskID        string
	userID        string
	projectID     string
	workDir       string
	normalizedDir string
	concatList    string
	assFile       string
	outputPath    string
	input         CompositionInput
}

// ComposeFull 跑完整的 5 步合成。
//
// v2 简化：
//   - 同步执行（v3 接通用 task worker 派发）
//   - 不下载远程 URL（v2 假设 input.URL 都是本地路径或已下载到本地）
//   - 输出到本地 CompositionOutputDir（生产应改对象存储）
func ComposeFull(ctx context.Context, task *model.CompositionTask) error {
	if task == nil {
		return errors.New("task required")
	}
	if task.ID == "" {
		return errors.New("task.ID required")
	}
	var input CompositionInput
	if err := json.Unmarshal([]byte(task.InputJSON), &input); err != nil {
		return fmt.Errorf("parse input: %w", err)
	}
	if len(input.ShotVideos) == 0 {
		return errors.New("ShotVideos 不能为空")
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	task.Status = string(statusRunning)
	task.StartedAt = now
	task.UpdatedAt = now
	_ = repository.UpdateCompositionTask(task)

	outputDir := strings.TrimSpace(config.Cfg.CompositionOutputDir)
	if outputDir == "" {
		outputDir = "data/compositions"
	}
	workDir := filepath.Join(outputDir, task.ID)
	normalizedDir := filepath.Join(workDir, "normalized")
	if err := os.MkdirAll(normalizedDir, 0o755); err != nil {
		return err
	}
	cc := &compositionContext{
		taskID:        task.ID,
		userID:        task.UserID,
		projectID:     task.ProjectID,
		workDir:       workDir,
		normalizedDir: normalizedDir,
		concatList:    filepath.Join(workDir, "concat.txt"),
		assFile:       filepath.Join(workDir, "subtitle.ass"),
		outputPath:    filepath.Join(outputDir, task.ID+".mp4"),
		input:         input,
	}

	// 1) 归一化
	if err := cc.step1Normalize(ctx, "归一化镜头视频"); err != nil {
		return cc.fail("step1 normalize", err)
	}
	// 2) 拼接
	if err := cc.step2Concat(ctx, "拼接镜头视频"); err != nil {
		return cc.fail("step2 concat", err)
	}
	// 3) 混音
	if err := cc.step3MixAudio(ctx, "混音（配音+BGM）"); err != nil {
		return cc.fail("step3 mix", err)
	}
	// 4) 烧字幕
	if err := cc.step4BurnSubtitle(ctx, "烧字幕"); err != nil {
		return cc.fail("step4 burn", err)
	}
	// 5) 输出
	if err := cc.step5Output(ctx, "输出最终 mp4"); err != nil {
		return cc.fail("step5 output", err)
	}

	// 成功
	now = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	task.Status = string(statusSuccess)
	task.OutputURL = cc.outputPath
	task.UpdatedAt = now
	task.CompletedAt = now
	if info, err := os.Stat(cc.outputPath); err == nil {
		task.OutputSize = info.Size()
	}
	task.OutputMime = "video/mp4"
	return repository.UpdateCompositionTask(task)
}

// ComposeSubtitleOnly 仅跑步骤 4 烧字幕（novel-rerun-layer 用）。
func ComposeSubtitleOnly(ctx context.Context, task *model.CompositionTask) error {
	if task == nil {
		return errors.New("task required")
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	task.Status = string(statusRunning)
	task.StartedAt = now
	task.UpdatedAt = now
	_ = repository.UpdateCompositionTask(task)

	var input CompositionInput
	if err := json.Unmarshal([]byte(task.InputJSON), &input); err != nil {
		return fmt.Errorf("parse input: %w", err)
	}

	outputDir := strings.TrimSpace(config.Cfg.CompositionOutputDir)
	if outputDir == "" {
		outputDir = "data/compositions"
	}
	workDir := filepath.Join(outputDir, task.ID)
	cc := &compositionContext{
		taskID:        task.ID,
		userID:        task.UserID,
		projectID:     task.ProjectID,
		workDir:       workDir,
		normalizedDir: filepath.Join(workDir, "normalized"),
		concatList:    filepath.Join(workDir, "concat.txt"),
		assFile:       filepath.Join(workDir, "subtitle.ass"),
		outputPath:    filepath.Join(outputDir, task.ID+".mp4"),
		input:         input,
	}

	if err := cc.step4BurnSubtitle(ctx, "重烧字幕"); err != nil {
		return cc.fail("rerun burn subtitle", err)
	}
	if err := cc.step5Output(ctx, "重输出 mp4"); err != nil {
		return cc.fail("rerun output", err)
	}

	now = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	task.Status = string(statusSuccess)
	task.OutputURL = cc.outputPath
	task.UpdatedAt = now
	task.CompletedAt = now
	if info, err := os.Stat(cc.outputPath); err == nil {
		task.OutputSize = info.Size()
	}
	task.OutputMime = "video/mp4"
	return repository.UpdateCompositionTask(task)
}

// fail 错误时回写 task 状态。
func (cc *compositionContext) fail(stepName string, err error) error {
	log.Printf("composition task=%s step=%s err=%v", cc.taskID, stepName, err)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_ = now
	return repository.UpdateCompositionTaskProgress(
		cc.taskID, string(statusFailed),
		fmt.Sprintf(`{"currentStep":0,"lastMessage":"%s 失败: %v"}`, stepName, err),
		fmt.Sprintf("%s 失败: %v", stepName, err), "",
	)
}

// step1Normalize 归一化所有镜头视频到 h264/aac/yuv420p。
func (cc *compositionContext) step1Normalize(ctx context.Context, msg string) error {
	for i, sv := range cc.input.ShotVideos {
		out := filepath.Join(cc.normalizedDir, fmt.Sprintf("shot-%d.mp4", i))
		args := []string{
			"-y",
			"-i", sv.URL,
			"-c:v", "libx264", "-pix_fmt", "yuv420p",
			"-c:a", "aac",
			"-ar", "44100",
			out,
		}
		_, stderr, err := RunFfmpegWithProgress(ctx, ffmpegBin(), args, 1, 5, msg, nil)
		if err != nil {
			return fmt.Errorf("normalize shot %d: %w (stderr: %s)", i, err, stderr)
		}
	}
	return nil
}

// step2Concat 按分镜顺序拼接归一化后的视频。
func (cc *compositionContext) step2Concat(ctx context.Context, msg string) error {
	var sb strings.Builder
	for i := range cc.input.ShotVideos {
		fmt.Fprintf(&sb, "file '%s'\n", filepath.Join(cc.normalizedDir, fmt.Sprintf("shot-%d.mp4", i)))
	}
	if err := os.WriteFile(cc.concatList, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	concatOut := filepath.Join(cc.workDir, "concat.mp4")
	args := []string{
		"-y",
		"-f", "concat", "-safe", "0",
		"-i", cc.concatList,
		"-c", "copy",
		concatOut,
	}
	_, stderr, err := RunFfmpegWithProgress(ctx, ffmpegBin(), args, 2, 5, msg, nil)
	if err != nil {
		return fmt.Errorf("concat: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// step3MixAudio 混音（配音 + BGM）。v2 简化：仅复制原声；完整 mix 留 v3。
func (cc *compositionContext) step3MixAudio(ctx context.Context, msg string) error {
	concatOut := filepath.Join(cc.workDir, "concat.mp4")
	audioOut := filepath.Join(cc.workDir, "audio.mp4")
	_ = cc.dubbingByShot() // v3 真实使用
	args := []string{
		"-y",
		"-i", concatOut,
		"-c", "copy",
		audioOut,
	}
	_, stderr, err := RunFfmpegWithProgress(ctx, ffmpegBin(), args, 3, 5, msg, nil)
	if err != nil {
		return fmt.Errorf("mix: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// dubbingByShot 索引：shotId → CompositionShotDubbing。
func (cc *compositionContext) dubbingByShot() map[string]CompositionShotDubbing {
	out := map[string]CompositionShotDubbing{}
	for _, d := range cc.input.ShotDubbings {
		out[d.ShotID] = d
	}
	return out
}

// step4BurnSubtitle 烧字幕（ass filter）。
func (cc *compositionContext) step4BurnSubtitle(ctx context.Context, msg string) error {
	audioOut := filepath.Join(cc.workDir, "audio.mp4")
	if err := writeAssFile(cc.assFile, cc.input.SubtitleStyle, cc.assLines()); err != nil {
		return err
	}
	args := []string{
		"-y",
		"-i", audioOut,
		"-vf", "ass=" + cc.assFile,
		"-c:a", "copy",
		cc.outputPath,
	}
	_, stderr, err := RunFfmpegWithProgress(ctx, ffmpegBin(), args, 4, 5, msg, nil)
	if err != nil {
		return fmt.Errorf("burn subtitle: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// step5Output 输出最终 mp4（v2 与 step4 合并到 outputPath）。
func (cc *compositionContext) step5Output(ctx context.Context, msg string) error {
	if _, err := os.Stat(cc.outputPath); err != nil {
		return fmt.Errorf("output mp4 not found: %w", err)
	}
	log.Printf("composition task=%s output: %s", cc.taskID, cc.outputPath)
	return nil
}

// assLines 把所有 shot 字幕拼成 ass 格式。
func (cc *compositionContext) assLines() []assLine {
	var out []assLine
	for _, ss := range cc.input.ShotSubtitles {
		for _, l := range ss.Lines {
			out = append(out, assLine{Start: l.StartMs, End: l.EndMs, Text: l.Text})
		}
	}
	return out
}

type assLine struct {
	Start, End int
	Text       string
}

// writeAssFile 写 ass 字幕文件。
func writeAssFile(path string, style SubtitleStyleJSON, lines []assLine) error {
	var sb strings.Builder
	sb.WriteString("[Script Info]\n")
	sb.WriteString("ScriptType: v4.00+\n")
	sb.WriteString("PlayResX: 1920\n")
	sb.WriteString("PlayResY: 1080\n\n")

	sb.WriteString("[V4+ Styles]\n")
	sb.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, BackgroundColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	font := style.Font
	if font == "" {
		font = "黑体"
	}
	size := style.Size
	if size == 0 {
		size = 36
	}
	color := style.Color
	if color == "" {
		color = "&H00FFFFFF" // ass 格式 ABGR
	}
	outline := style.Outline
	if outline == "" {
		outline = "&H00000000"
	}
	outlineW := style.OutlineWidth
	if outlineW == 0 {
		outlineW = 2
	}
	marginV := style.MarginBottom
	if marginV == 0 {
		marginV = 60
	}
	fmt.Fprintf(&sb, "Style: Default,%s,%d,%s,&H000000FF,%s,&H80000000,-1,0,0,0,100,100,0,0,1,%d,1,2,10,10,%d,1\n\n",
		font, size, color, outline, outlineW, marginV)

	sb.WriteString("[Events]\n")
	sb.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, l := range lines {
		fmt.Fprintf(&sb, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			formatAssTime(l.Start), formatAssTime(l.End), escapeAssText(l.Text))
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// formatAssTime 毫秒 → ASS 时间格式（h:mm:ss.cc）。
func formatAssTime(ms int) string {
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	cs := (ms % 1000) / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

func escapeAssText(s string) string {
	return strings.ReplaceAll(s, "\n", "\\N")
}

func ffmpegBin() string {
	bin := strings.TrimSpace(config.Cfg.FfmpegBinaryPath)
	if bin == "" {
		return "ffmpeg"
	}
	return bin
}
