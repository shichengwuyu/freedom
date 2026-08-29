package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: export-layer ===
//
// 合成任务成功后, 用户能：
//   - 下载 mp4
//   - 复制平台文案（抖音 / 小红书 / 视频号）
//   - 查看成片元数据
//
// v2 不做分享链接 / token（v3 再做）。

// ExportMetadata 成片元数据（前端展示用）。
type ExportMetadata struct {
	ProjectID         string `json:"projectId"`
	ProjectTitle      string `json:"projectTitle"`
	CompositionID     string `json:"compositionId"`
	OutputURL         string `json:"outputUrl"`
	OutputSize        int64  `json:"outputSize"`
	OutputMime        string `json:"outputMime"`
	DurationSeconds   int    `json:"durationSeconds"`
	CreatedAt         string `json:"createdAt"`
	BGMName           string `json:"bgmName"`
	SubtitleStyle     string `json:"subtitleStyle"`
	ShotCount         int    `json:"shotCount"`
	TotalNodes        int    `json:"totalNodes"`
}

// GetExportMetadata 读成片元数据。
//   - composition 必填
//   - project（可选, 用于拿 project title）
func GetExportMetadata(compositionID, userID string) (*ExportMetadata, error) {
	task, err := repository.GetCompositionTask(compositionID, userID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, errors.New("成片任务不存在")
	}
	if task.Status != string(statusSuccess) {
		return nil, errors.New("成片任务未成功, 当前状态: " + task.Status)
	}

	meta := &ExportMetadata{
		ProjectID:     task.ProjectID,
		CompositionID: task.ID,
		OutputURL:     task.OutputURL,
		OutputSize:    task.OutputSize,
		OutputMime:    task.OutputMime,
		CreatedAt:     task.CompletedAt,
	}

	// 文件大小 fallback
	if meta.OutputSize == 0 && task.OutputURL != "" {
		if info, err := os.Stat(task.OutputURL); err == nil {
			meta.OutputSize = info.Size()
		}
	}

	// BGM 名称（从 inputJson 解析 bgmSource）
	var input CompositionInput
	if task.InputJSON != "" {
		_ = input // 仅占位, 真实解析在下面
	}
	// v2 简化: 不深入解析 inputJson 找 BGM, 留 v3
	meta.BGMName = "(待接入)"

	// SubtitleStyle 简化
	meta.SubtitleStyle = "白字+黑描边+底部居中"

	// Shot 数量（input.shotVideos 数量）
	// v2 简化: 不解析, 留 v3
	meta.ShotCount = 0

	return meta, nil
}

// GeneratePlatformCaption 生成抖音 / 小红书 / 视频号文案。
//
// 入参：
//   - platform: "douyin" / "xiaohongshu" / "shipinhao"
//   - projectTitle: 项目标题
//   - description: 一句话描述（可空, 默认生成"由 Freedom 生成的短剧/漫剧"）
//
// 返回：文案字符串（带 #标签 + 短描述 + 占位链接）。
//
// v2 简化: 真实分享链接 / token 留 v3（export-layer spec 6.1 写了 v2 不做分享链接）。
// 这里文案用占位 [分享链接] 让用户自己粘贴生成的 mp4 链接。
func GeneratePlatformCaption(platform, projectTitle, description string) string {
	title := strings.TrimSpace(projectTitle)
	if title == "" {
		title = "未命名作品"
	}
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = "由 Freedom AI 创作平台生成的短剧 / 漫剧作品"
	}
	linkPlaceholder := "[分享链接]"

	switch strings.ToLower(platform) {
	case "douyin":
		return fmt.Sprintf(`%s

%s

#%s #AI短剧 #漫剧 #Freedom`, title, desc, title)
	case "xiaohongshu":
		return fmt.Sprintf(`【%s】

%s

🎬 AI 创作 | Freedom
#%s #AI创作 #短剧推荐`, title, desc, title)
	case "shipinhao":
		return fmt.Sprintf(`%s

%s

由 Freedom AI 创作

%s`, title, desc, linkPlaceholder)
	default:
		return fmt.Sprintf(`%s

%s

由 Freedom AI 创作`, title, desc)
	}
}

// ListExportHistory 列项目所有成功成片（按时间倒序）—— v2 简化从 composition_tasks 表直接查。
func ListExportHistory(projectID, userID string) ([]model.CompositionTask, error) {
	if projectID == "" {
		return nil, errors.New("projectID required")
	}
	tasks, _, err := repository.ListCompositionTasksByProject(projectID, 50, 0)
	if err != nil {
		return nil, err
	}
	filtered := make([]model.CompositionTask, 0)
	for _, t := range tasks {
		if t.UserID == userID && t.Status == string(statusSuccess) {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// DownloadFilename 拼下载文件名：{projectTitle}_{timestamp}.mp4。
func DownloadFilename(projectTitle, completedAt string) string {
	title := strings.TrimSpace(projectTitle)
	if title == "" {
		title = "Freedom"
	}
	// 文件名安全字符
	title = safeFilename(title)
	ts := completedAt
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", completedAt); err == nil {
		ts = t.Format("20060102-150405")
	}
	return title + "_" + ts + ".mp4"
}

func safeFilename(s string) string {
	// 替换 / \ : * ? " < > | 等不安全字符
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_")
	return r.Replace(s)
}
