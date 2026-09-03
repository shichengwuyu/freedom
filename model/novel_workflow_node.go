package model

// NovelWorkflowNode novel 工作流一次 Run 下的单个节点状态（novel-workflow v2 引入）。
//
// 每条记录对应该 Run 的一个具体节点（例：shot 3 的配音节点、shot 7 的字幕节点、
// 整部成片合成节点等）。"层"（layer）本身不在表里，层是 UI 概念，
// 由 service/novel_workflow_graph.go 静态定义；节点按 layer 字段归类。
//
// 单层重做（novel-rerun-layer capability）走通用 task worker：
// 用户点"重做某分镜的配音"时，本表插入一条新记录（或复用现有记录更新状态为"进行中"），
// 同时通过 service/novel_workflow.go 派发到通用 task worker（model/task.go）。
type NovelWorkflowNode struct {
	ID           string `json:"id" gorm:"primaryKey"`
	RunID        string `json:"runId" gorm:"index"`
	UserID       string `json:"userId" gorm:"index"`
	ProjectID    string `json:"projectId" gorm:"index"`

	// 节点身份
	NodeID    string `json:"nodeId" gorm:"index"`    // 工作流图内的稳定 ID，例 "shot-3-dubbing"、"full-composition"、"full-export"
	Layer     string `json:"layer" gorm:"index"`      // input | script | asset | shot | post
	NodeKind  string `json:"nodeKind" gorm:"index"`   // script | storyboard | character | scene | prop | shot-video | shot-dubbing | shot-subtitle | bgm-pick | composition | export
	NodeTitle string `json:"nodeTitle"`               // UI 显示名
	ShotID    string `json:"shotId" gorm:"index"`     // per-shot 节点关联分镜 ID；全工程节点为空
	ShotIndex int    `json:"shotIndex"`               // per-shot 节点在项目里的索引（-1 表示全工程节点）

	// 依赖关系：JSON 数组字符串，例 ["shot-3-video", "shot-3-dubbing"]
	DependsOnJSON string `json:"dependsOnJson" gorm:"type:text"`

	// 状态机：未启动 | 排队中 | 进行中 | 成功 | 失败 | 跳过 | 已取消
	Status string `json:"status" gorm:"index"`

	// 进度（0-100）+ 步骤名（如"归一化镜头 3 / 8"）
	Progress    int    `json:"progress"`
	StepMessage string `json:"stepMessage" gorm:"type:text"`

	// 节点产出 URL（多视频/多配音/多字幕节点都填各自的）
	// 例：shot-dubbing 节点 = mp3 URL；shot-video 节点 = mp4 URL；composition 节点 = 最终 mp4 URL
	OutputURL string `json:"outputUrl" gorm:"type:text"`

	// 关联的通用 task（novel-rerun-layer 通过 model/task.go 派发到通用 worker）
	GenericTaskID string `json:"genericTaskId" gorm:"index"`

	// 错误信息
	Error string `json:"error" gorm:"type:text"`

	// 时间戳
	CreatedAt   string `json:"createdAt" gorm:"index"`
	UpdatedAt   string `json:"updatedAt" gorm:"index"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
}
