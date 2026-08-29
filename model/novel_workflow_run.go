package model

// NovelWorkflowRun novel 工作流的一次完整运行（novel-workflow v2 引入）。
//
// 与 StoryboardTask / VideoTask 的关系：StoryboardTask / VideoTask 是单步任务
// （分镜生成 / 视频生成），由 handler 各自有专用 worker；本表是"工作流层"的
// 编排单元——一次 run 跑通 5 层（输入/剧本/资产/镜头/后期）所有节点的串联。
//
// 工作流跑通后 Run 状态由"进行中"变为"已完成/部分失败/已停止"；
// 每个节点状态机独立（见 NovelWorkflowNode.Status）。
type NovelWorkflowRun struct {
	ID             string `json:"id" gorm:"primaryKey"`
	UserID         string `json:"userId" gorm:"index"`
	UserGroupCode  string `json:"userGroupCode" gorm:"index"` // Sprint 3 用户组，限流 / 配额维度
	ProjectID      string `json:"projectId" gorm:"index"`    // 关联 NovelProject（前端 sourceId）

	// 启动模式：auto | manual | quick | custom
	//   auto   — 上游成功后下游自动跑（v1 行为）
	//   manual — 全部手动单步跑
	//   quick  — 快速模式（默认参数 + 自动）
	//   custom — 自定义模式（先调参数再跑）
	Mode string `json:"mode" gorm:"index"`

	// 总体状态：未启动 | 进行中 | 已完成 | 部分失败 | 已停止
	// 聚合所有节点状态得出：所有节点都未启动 → 未启动；有进行中 → 进行中；全部成功/跳过 → 已完成；有失败但其他成功 → 部分失败；有已取消 → 已停止
	OverallStatus string `json:"overallStatus" gorm:"index"`

	// 启动时间配置（custom 模式用）
	ConfigJSON string `json:"configJson" gorm:"type:longtext"` // {bgmPresetId, bgmVolume, bgmFadeIn, bgmFadeOut, subtitleStyleJson, shotDurationSec, enableSubtitle, voiceId, ttsProvider, ...}

	// 状态聚合
	TotalNodes     int `json:"totalNodes"`
	SuccessNodes   int `json:"successNodes"`
	FailedNodes    int `json:"failedNodes"`
	SkippedNodes   int `json:"skippedNodes"`
	CanceledNodes  int `json:"canceledNodes"`
	PendingNodes   int `json:"pendingNodes"` // 还在排队的节点数（未启动 + 排队中 + 进行中）

	// 终态输出（最近一次成功合成的 mp4 URL）
	LastOutputURL  string `json:"lastOutputUrl" gorm:"type:text"`
	LastOutputAt   string `json:"lastOutputAt" gorm:"index"`

	Error  string `json:"error" gorm:"type:text"`
	Result string `json:"result" gorm:"type:longtext"` // 预留：{compositionTaskId, version, ...}

	CreatedAt   string `json:"createdAt" gorm:"index"`
	UpdatedAt   string `json:"updatedAt" gorm:"index"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
}
