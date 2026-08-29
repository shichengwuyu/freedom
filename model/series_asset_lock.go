package model

// SeriesAssetLock novel-workflow v2 - series-asset-lock (漫剧一致性) 的项目级主资产包。
//
// 概念：在项目级别锁定一组"主资产包"：1-N 个角色、1-N 个场景、1-N 个道具、
// 一段全局色调 prompt。视频生成强制从主资产包取参考图；
// 跨分镜共享风格 reference；保证漫剧 / 短剧的世界观一致性。
//
// 锁定后：
//   - 分镜级资产选择器只显示主资产包内的资产
//   - 所有分镜视频生成的 prompt 自动追加全局色调描述
//   - 同场景分镜共享同一张场景图
type SeriesAssetLock struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"index"`
	ProjectID string `json:"projectId" gorm:"index"`

	// 主资产包内容
	CharacterIDsJSON string `json:"characterIdsJson" gorm:"type:text"`
	SceneIDsJSON     string `json:"sceneIdsJson" gorm:"type:text"`
	PropIDsJSON      string `json:"propIdsJson" gorm:"type:text"`

	// 全局色调 prompt
	GlobalStylePrompt string `json:"globalStylePrompt" gorm:"type:text"`

	// 是否锁定
	IsLocked bool `json:"isLocked"`

	// 锁定时间
	LockedAt   string `json:"lockedAt"`
	UnlockedAt string `json:"unlockedAt"`

	CreatedAt string `json:"createdAt" gorm:"index"`
	UpdatedAt string `json:"updatedAt" gorm:"index"`
}
