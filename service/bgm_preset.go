package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: bgm-layer ===
//
// 系统预设 BGM：启动时从 config.Cfg.BgmPresetsDir/manifest.json 读 + 校验 mp3 存在。
// 用户上传 BGM：multipart 上传 → 校验大小 / MIME → 落对象存储 → 写 BgmCustom 元数据。
//
// v2 简化：
//   - 用户上传文件落地到 BgmPresetsDir/novel/bgm/custom/{projectId}/{id}.{ext}（不接对象存储；
//     真实部署可改 StorageObject 上传）
//   - 20MB 大小限制
//   - 仅允许 audio/mpeg (mp3) / audio/wav

// BGM 上传限制
const (
	bgmMaxFileSize = 20 * 1024 * 1024 // 20MB
)

// bgmManifest 系统预设 BGM manifest 结构。
type bgmManifest struct {
	Version int              `json:"version"`
	Presets []model.BgmPreset `json:"presets"`
}

var (
	bgmPresetsCache = map[string]model.BgmPreset{} // id → preset
	bgmPresetsOnce  bool
)

// LoadBgmPresets 启动时调一次：读 manifest.json + 校验 mp3 文件存在。
//
// 缺失 mp3 的预设 available=false；前端不展示。
func LoadBgmPresets() error {
	if bgmPresetsOnce {
		return nil
	}
	dir := strings.TrimSpace(config.Cfg.BgmPresetsDir)
	if dir == "" {
		// dev 默认：源码目录
		dir = "assets/bgm-presets"
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	f, err := os.Open(manifestPath)
	if err != nil {
		log.Printf("bgm: open manifest %s err=%v (skip preset loading)", manifestPath, err)
		bgmPresetsOnce = true
		return nil
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	var m bgmManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	for _, p := range m.Presets {
		p.Available = false
		p.AvailablePath = ""
		mp3Path := filepath.Join(dir, p.FileName)
		if info, err := os.Stat(mp3Path); err == nil && !info.IsDir() {
			p.Available = true
			p.AvailablePath = mp3Path
		}
		bgmPresetsCache[p.ID] = p
	}
	bgmPresetsOnce = true
	log.Printf("bgm: loaded %d presets (%d available)", len(m.Presets), countAvailable(bgmPresetsCache))
	return nil
}

func countAvailable(m map[string]model.BgmPreset) int {
	n := 0
	for _, p := range m {
		if p.Available {
			n++
		}
	}
	return n
}

// ListPresets 列出所有预设（可用 + 不可用都列；前端按 available 字段筛选展示）。
// 可选按 tag 过滤。
func ListPresets(tag string) []model.BgmPreset {
	out := make([]model.BgmPreset, 0, len(bgmPresetsCache))
	for _, p := range bgmPresetsCache {
		if tag != "" && !hasTag(p.Tags, tag) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func hasTag(tags []string, t string) bool {
	for _, tag := range tags {
		if tag == t {
			return true
		}
	}
	return false
}

// GetPreset 按 id 取预设。
func GetPreset(id string) (model.BgmPreset, bool) {
	p, ok := bgmPresetsCache[id]
	return p, ok
}

// UploadCustom 上传用户自定义 BGM。
//
//   - userID / projectID：归属
//   - title：用户起的名字（必填）
//   - tags：逗号分隔字符串（v2 简化）
//   - file：multipart.FileHeader
//
// 校验：
//   - 20MB 大小限制
//   - audio/mpeg / audio/wav MIME
//
// 落库：BgmCustom 元数据。
// 落盘：BgmPresetsDir/novel/bgm/custom/{projectId}/{id}.{ext}（v2 简化；生产应接对象存储）
func UploadCustom(userID, projectID, title, tags string, file *multipart.FileHeader) (*model.BgmCustom, error) {
	if userID == "" || projectID == "" {
		return nil, errors.New("userID/projectID required")
	}
	if title == "" {
		return nil, errors.New("title 不能为空")
	}
	if file == nil {
		return nil, errors.New("file 不能为空")
	}
	if file.Size > bgmMaxFileSize {
		return nil, fmt.Errorf("BGM 文件超过 20MB（实际 %d bytes）", file.Size)
	}
	mime := file.Header.Get("Content-Type")
	if !isAudioMIME(mime) {
		return nil, fmt.Errorf("不支持的文件类型: %s（仅支持 mp3 / wav）", mime)
	}

	// 创建 BgmCustom 记录
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	id := newID("bgm")
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		if mime == "audio/mpeg" || mime == "audio/mp3" {
			ext = ".mp3"
		} else if mime == "audio/wav" || mime == "audio/x-wav" {
			ext = ".wav"
		} else {
			ext = ".audio"
		}
	}
	bc := &model.BgmCustom{
		ID:        id,
		UserID:    userID,
		ProjectID: projectID,
		Title:     title,
		TagsJSON:  tags,
		FileURL:   "novel/bgm/custom/" + projectID + "/" + id + ext,
		MimeType:  mime,
		SizeBytes: file.Size,
		CreatedAt: nowStr,
		UpdatedAt: nowStr,
	}

	// 落盘（v2 简化；生产应改用对象存储）
	destDir := filepath.Join(strings.TrimSpace(config.Cfg.BgmPresetsDir), "novel", "bgm", "custom", projectID)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(destDir, id+ext))
	if err != nil {
		return nil, err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return nil, err
	}

	// 落库
	if err := repository.CreateBgmCustom(bc); err != nil {
		// 落盘失败时清理
		_ = os.Remove(filepath.Join(destDir, id+ext))
		return nil, err
	}
	return bc, nil
}

// DeleteCustom 删用户上传 BGM（同时删对象存储文件）。
func DeleteCustom(userID, id string) error {
	bc, err := repository.GetBgmCustom(id, userID)
	if err != nil {
		return err
	}
	if bc == nil {
		return errors.New("BGM 不存在")
	}
	// 删文件
	fullPath := filepath.Join(strings.TrimSpace(config.Cfg.BgmPresetsDir), bc.FileURL)
	_ = os.Remove(fullPath)
	return repository.DeleteBgmCustom(id, userID)
}

// ListCustomForProject 列项目自定义 BGM。
func ListCustomForProject(projectID string) ([]model.BgmCustom, error) {
	return repository.ListBgmCustomsByProject(projectID)
}

func isAudioMIME(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	switch m {
	case "audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav", "audio/wave":
		return true
	}
	return false
}
