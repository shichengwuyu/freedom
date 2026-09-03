package model

// === novel-workflow v2: bgm-layer ===
//
// 两个 model：
//   - BgmPreset: 系统预设 BGM（从 manifest.json 读，不入库；提供内存版供前端展示）
//   - BgmCustom:  用户上传的自定义 BGM（per-project，存库 + 对象存储）

// BgmPreset 系统预设 BGM（仅运行时内存用，不入库）。
// 字段对齐 assets/bgm-presets/manifest.json 的 presets[] 元素。
type BgmPreset struct {
	ID              string   `json:"id"`              // "gufeng-1"
	Title           string   `json:"title"`           // "古风·山水"
	Tags            []string `json:"tags"`            // ["古风", "舒缓", "国风"]
	FileName        string   `json:"fileName"`        // "gufeng-1.mp3"
	DurationSeconds int      `json:"durationSeconds"` // 180
	Description     string   `json:"description"`
	// mp3 文件实际可读路径（启动时按 file 拼接 bgm_presets_dir；缺失则为空）
	AvailablePath string `json:"availablePath,omitempty"`
	// 是否 mp3 实际可播放（启动时校验）
	Available bool `json:"available"`
}

// BgmCustom 用户上传的 BGM。
//
// 文件名 = newID("bgm") + 原扩展名；
// 对象存储路径 = novel/bgm/custom/{projectId}/{id}.{ext}；
// 元数据存库（BgmCustom），文件存对象存储。
type BgmCustom struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"index"`
	ProjectID string `json:"projectId" gorm:"index"`

	Title    string `json:"title"`
	TagsJSON string `json:"tagsJson" gorm:"type:text"` // 逗号分隔 / JSON 数组字符串
	FileURL  string `json:"fileUrl" gorm:"type:text"`
	MimeType string `json:"mimeType"`
	SizeBytes int64 `json:"sizeBytes"`
	DurationSeconds int `json:"durationSeconds"` // 用户上传时可从 audio metadata 读（v2 简化存 0）

	CreatedAt string `json:"createdAt" gorm:"index"`
	UpdatedAt string `json:"updatedAt" gorm:"index"`
}
