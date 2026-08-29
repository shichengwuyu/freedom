package service

// novel-workflow v2：5 层节点图定义
//
// 5 层（input / script / asset / shot / post）+ 每层包含的子节点。
// "层"是 UI 概念，节点按 layer 字段归类；具体跑哪些节点是动态的
//（例：用户没选 BGM → bgm-pick 节点跳过；剧集级资产未锁定 → 跨分镜一致性约束不生效）。
//
// 节点 ID 规范：
//   - 前置层（输入/剧本/资产）：全工程只有 1 个，nodeId = 固定字符串
//   - 镜头层节点：nodeId = "shot-{shotId}-{kind}"（per-shot）
//   - 后期层节点：nodeId = "full-{kind}"（整部成片）
//
// 节点依赖（DAG）：
//   - 剧本节点依赖：输入节点
//   - 资产节点依赖：剧本节点
//   - 镜头视频节点依赖：资产节点
//   - 镜头配音节点依赖：镜头视频节点
//   - 镜头字幕节点依赖：镜头视频节点
//   - bgm-pick 节点依赖：剧本节点（不依赖视频，用户配 BGM 不需要等视频）
//   - composition 节点依赖：所有镜头视频 + 所有镜头配音 + 所有镜头字幕 + bgm-pick
//   - export 节点依赖：composition
//
// 节点状态机：未启动 → 排队中 → 进行中 → （成功 / 失败 / 跳过 / 已取消）
// "失败 / 跳过"可被重试回到"排队中"。

// novelNodeLayer 5 个层（UI 横向步骤条按此分组）。
type novelNodeLayer string

const (
	layerInput  novelNodeLayer = "input"
	layerScript novelNodeLayer = "script"
	layerAsset  novelNodeLayer = "asset"
	layerShot   novelNodeLayer = "shot"
	layerPost   novelNodeLayer = "post"
)

// novelNodeKind 节点类型（与 model.NovelWorkflowNode.NodeKind 字段对齐）。
type novelNodeKind string

const (
	kindScript         novelNodeKind = "script"           // 剧本节点
	kindStoryboard     novelNodeKind = "storyboard"       // 分镜剧本节点
	kindCharacter      novelNodeKind = "character"        // 角色资产节点
	kindScene          novelNodeKind = "scene"            // 场景资产节点
	kindProp           novelNodeKind = "prop"             // 道具资产节点
	kindShotVideo      novelNodeKind = "shot-video"       // 镜头视频节点
	kindShotDubbing    novelNodeKind = "shot-dubbing"     // 镜头配音节点
	kindShotSubtitle   novelNodeKind = "shot-subtitle"    // 镜头字幕节点
	kindBgmPick        novelNodeKind = "bgm-pick"         // BGM 选曲节点
	kindComposition    novelNodeKind = "composition"      // 合成节点
	kindExport         novelNodeKind = "export"           // 导出节点
)

// novelNodeStatus 节点状态机（7 态）。
type novelNodeStatus string

const (
	statusNotStarted novelNodeStatus = "未启动"
	statusQueued     novelNodeStatus = "排队中"
	statusRunning    novelNodeStatus = "进行中"
	statusSuccess    novelNodeStatus = "成功"
	statusFailed     novelNodeStatus = "失败"
	statusSkipped    novelNodeStatus = "跳过"
	statusCanceled   novelNodeStatus = "已取消"
)

// novelLayerOrder 层的展示顺序（前端横向步骤条按此排）。
var novelLayerOrder = []novelNodeLayer{
	layerInput, layerScript, layerAsset, layerShot, layerPost,
}

// novelLayerDisplayName 层的 UI 显示名。
var novelLayerDisplayName = map[novelNodeLayer]string{
	layerInput:  "输入",
	layerScript: "剧本",
	layerAsset:  "资产",
	layerShot:   "镜头",
	layerPost:   "后期",
}

// novelKindDisplayName 节点类型的 UI 显示名。
var novelKindDisplayName = map[novelNodeKind]string{
	kindScript:         "剧本",
	kindStoryboard:     "分镜剧本",
	kindCharacter:      "角色",
	kindScene:          "场景",
	kindProp:           "道具",
	kindShotVideo:      "镜头视频",
	kindShotDubbing:    "镜头配音",
	kindShotSubtitle:   "镜头字幕",
	kindBgmPick:        "BGM 选曲",
	kindComposition:    "成片合成",
	kindExport:         "导出",
}

// novelNodeTemplate 节点模板（按 kind 静态定义；运行时按 shot 数生成具体节点）。
type novelNodeTemplate struct {
	Kind         novelNodeKind
	Title        string         // 模板显示名
	PerShot      bool           // true = 每个分镜一个节点，false = 全工程一个节点
	FixedNodeID  string         // 非 per-shot 时的稳定 nodeId（PerShot=false 时使用）
	IDPrefix     string         // per-shot 时的 ID 前缀（PerShot=true 时使用）
	Dependencies []string       // 依赖的 kind 列表（按 shot 隔离时依赖同 shot 的 kind）
	Layer        novelNodeLayer
}

// novelNodeTemplates 11 个节点模板（按依赖顺序）。
// 新增节点时：加模板 + 在 service/novel_workflow.go 派发器里加 handler 即可。
var novelNodeTemplates = []novelNodeTemplate{
	{Kind: kindScript, Title: "剧本", PerShot: false, FixedNodeID: "project-script", Layer: layerScript},
	{Kind: kindStoryboard, Title: "分镜剧本", PerShot: false, FixedNodeID: "project-storyboard", Layer: layerScript,
		Dependencies: []string{"project-script"}},
	{Kind: kindCharacter, Title: "角色资产", PerShot: false, FixedNodeID: "project-characters", Layer: layerAsset,
		Dependencies: []string{"project-storyboard"}},
	{Kind: kindScene, Title: "场景资产", PerShot: false, FixedNodeID: "project-scenes", Layer: layerAsset,
		Dependencies: []string{"project-storyboard"}},
	{Kind: kindProp, Title: "道具资产", PerShot: false, FixedNodeID: "project-props", Layer: layerAsset,
		Dependencies: []string{"project-storyboard"}},
	{Kind: kindShotVideo, Title: "镜头视频", PerShot: true, IDPrefix: "shot", Layer: layerShot,
		Dependencies: []string{"project-characters", "project-scenes", "project-props"}},
	{Kind: kindShotDubbing, Title: "镜头配音", PerShot: true, IDPrefix: "shot", Layer: layerShot,
		Dependencies: []string{"shot-video"}},
	{Kind: kindShotSubtitle, Title: "镜头字幕", PerShot: true, IDPrefix: "shot", Layer: layerShot,
		Dependencies: []string{"shot-video"}},
	{Kind: kindBgmPick, Title: "BGM 选曲", PerShot: false, FixedNodeID: "full-bgm-pick", Layer: layerPost,
		Dependencies: []string{"project-storyboard"}},
	{Kind: kindComposition, Title: "成片合成", PerShot: false, FixedNodeID: "full-composition", Layer: layerPost,
		Dependencies: []string{"shot-video", "shot-dubbing", "shot-subtitle", "full-bgm-pick"}},
	{Kind: kindExport, Title: "导出", PerShot: false, FixedNodeID: "full-export", Layer: layerPost,
		Dependencies: []string{"full-composition"}},
}

// NovelNodeDefinition 一个具体节点定义（per-shot 展开后）。
type NovelNodeDefinition struct {
	NodeID        string
	Kind          novelNodeKind
	Title         string
	Layer         novelNodeLayer
	DependsOn     []string
	PerShotIndex  int  // -1 表示全工程节点；>= 0 表示第几个分镜
}

// perShotSuffix 把 per-shot 节点 kind 映射为 ID 后缀。
//   - shot-video     → "video"
//   - shot-dubbing   → "dubbing"
//   - shot-subtitle  → "subtitle"
func perShotSuffix(k novelNodeKind) string {
	switch k {
	case kindShotVideo:
		return "video"
	case kindShotDubbing:
		return "dubbing"
	case kindShotSubtitle:
		return "subtitle"
	}
	return string(k)
}

// ExpandWorkflowGraph 按"剧本→资产→镜头→后期"展开 11 个节点模板为 N+5 个具体节点。
// shotIDs 是分镜 ID 列表（按剧本顺序）；空表示项目还没出分镜，仅展开前置 5 个节点。
//
// 依赖关系在展开时按 shot 索引对齐：shot 3 的配音依赖 shot 3 的视频（同 shot）。
func ExpandWorkflowGraph(shotIDs []string) []NovelNodeDefinition {
	out := make([]NovelNodeDefinition, 0, len(shotIDs)*3+5)
	for _, tpl := range novelNodeTemplates {
		if !tpl.PerShot {
			// 全工程节点
			out = append(out, NovelNodeDefinition{
				NodeID:       tpl.FixedNodeID,
				Kind:         tpl.Kind,
				Title:        tpl.Title,
				Layer:        tpl.Layer,
				DependsOn:    tpl.Dependencies,
				PerShotIndex: -1,
			})
			continue
		}
		// per-shot 节点
		for i, shotID := range shotIDs {
			base := tpl.IDPrefix + "-" + shotID
			suffix := perShotSuffix(tpl.Kind)
			deps := make([]string, 0, len(tpl.Dependencies))
			for _, dep := range tpl.Dependencies {
				if dep == "shot-video" {
					deps = append(deps, base+"-video")
				} else if dep == "shot-dubbing" {
					deps = append(deps, base+"-dubbing")
				} else if dep == "shot-subtitle" {
					deps = append(deps, base+"-subtitle")
				} else {
					deps = append(deps, dep)
				}
			}
			nodeID := base + "-" + suffix
			out = append(out, NovelNodeDefinition{
				NodeID:       nodeID,
				Kind:         tpl.Kind,
				Title:        tpl.Title + " " + shotID,
				Layer:        tpl.Layer,
				DependsOn:    deps,
				PerShotIndex: i,
			})
		}
	}
	return out
}
