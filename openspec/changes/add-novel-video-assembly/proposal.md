## 为什么

当前 Freedom novel 工作台已经具备「小说 → 分镜剧本 → 资产（角色/场景/道具）→ 单条视频」前半段链路，但缺后半段「单条视频 → 配音 → 字幕 → 合成成片 → 导出」，导致用户跑完分镜视频后还要去剪映 / PR 手动收尾——**链路在最后一段断掉**。

字节火山引擎「小云雀 AI」的核心竞争力是它把后半段做成了一个**对用户透明的多 Agent 协同工作流**：

- 每一步都是一个**可观察、可暂停、可重试**的节点
- 每一步用户都能**介入修改**（改提示词、改文本、改 BGM 选曲）
- 任何一步失败，**后续节点不会盲目接着跑**
- 最终输出一段**可发布的成片**，而不是一堆散点视频

本提案不抄小云雀的某个具体 API，而是**学它的工作流范式**——把后半段链路重写成一条「多节点 + 用户可见 + 可单步干预」的流水线，并保留 Freedom 现有的节点级编辑、3D 导演台、画布协作等差异化能力（不让项目退化成另一个"黑盒智能体"）。

## 改什么

**整体上**——把 novel 工作台后半段从一个"散点视频集合"重构成一条"用户可见的成片工作流"，新增 / 修改以下能力：

### 1. 工作流编排层（核心新增）

- **新增**：把"剧本 → 成片"建模成一条**多节点的流水线**（工作流），包含以下节点：
  - ① 剧本节点（已有）
  - ② 分镜剧本节点（已有）
  - ③ 资产节点（已有：角色 / 场景 / 道具）
  - ④ 镜头级视频节点（已有，但产出的视频节点升级为"可挂载配音 / 字幕"的复合节点）
  - ⑤ **镜头级配音节点**（新增）
  - ⑥ **镜头级字幕节点**（新增）
  - ⑦ **BGM 选曲**（新增，作为合成节点的前置子步骤）
  - ⑧ **成片合成节点**（新增）
  - ⑨ **导出分享**（新增，作为合成节点的后置子步骤）
- **新增**：每个节点都有统一的「状态机」：`未启动 / 排队中 / 进行中 / 成功 / 失败 / 跳过 / 已取消`；用户能从 UI 上看到每个节点当前状态
- **新增**：每个节点都暴露统一的「操作」：`开始 / 暂停 / 恢复 / 重试 / 单步编辑`；用户可以**只跑单个节点**，不必一次跑完整条流水线
- **新增**：节点之间的依赖关系是声明式的——字幕节点依赖视频节点的成功，配音节点依赖视频节点的成功，合成节点依赖前面所有节点的成功；任何一个上游节点失败，下游节点自动进入"等待重试上游"态而不是盲目接着跑
- **修改**：novel 工作台现有的"一键出片"按钮升级为"启动工作流"，工作流跑起来后用户能切到"分步视图"，逐节点看进度

### 2. 镜头级配音节点

- **新增**：每个镜头（shot）支持挂载一个 AI 配音（mp3）；配音来源是该分镜的「对白 / 旁白」字段
- **新增**：TTS 配音支持可插拔的服务（默认 MiMo TTS，可换 OpenAI TTS / 火山 TTS / ElevenLabs）
- **新增**：配音音色 / 语速可在合成前调整，调整后该镜头配音会重新生成
- **新增**：配音支持单镜头「试听 / 重新生成」，不依赖其他镜头
- **新增**：配音失败 → 该镜头降级为静音合成，不阻塞整条流水线

### 3. 镜头级字幕节点

- **新增**：每个镜头支持挂载一段字幕时间轴（srt 格式），字幕来源是该分镜的「对白 / 旁白」字段
- **新增**：字幕时间轴按"字数线性切分"到镜头视频时长（v1 算法；后续可升级为 ASR 重对齐）
- **新增**：字幕支持在合成前手动编辑（改文字、拖动起止时间、新增 / 删除行）
- **新增**：字幕样式（字体 / 颜色 / 描边 / 位置 / 字号）在整部成片级别统一设置

### 4. BGM 选曲

- **新增**：admin 后台维护一个 BGM 库（上传音频文件 + 标签：古风 / 都市 / 紧张 / 温馨 / 伤感 / 史诗等）
- **新增**：novel 工作台 BGM 选择器按标签筛选，支持试听
- **新增**：BGM 在合成时按"全局插入"模式（整部成片用同一首），可调音量 / 淡入 / 淡出

### 5. 成片合成节点

- **新增**：novel 工作台新增"一键合成成片"按钮，触发后端 ffmpeg 流水线
- **新增**：ffmpeg 按"分镜顺序"拼接所有镜头视频 + 配音 + 字幕 + BGM + 可选片头 / 片尾，输出最终 mp4
- **新增**：合成任务有独立的工作流节点状态（排队中 / 进行中 / 成功 / 失败 / 已取消），可轮询、可停止、可重试
- **新增**：合成进度按"步骤"展示（归一化所有镜头视频 → 拼接 → 混音 → 烧字幕 → 输出），不显示单一百分比
- **新增**：合成输出落对象存储（MinIO / S3 / R2），落库到 `video_assemblies` 表

### 6. 导出分享

- **新增**：合成完成后用户可下载 mp4 / 复制分享链接 / 复制平台发布文案（抖音 / 小红书 / 视频号）
- **新增**：分享链接默认 30 天有效（admin 可配），支持「撤销分享」立即失效

### 7. 保持不变

- **不动**：现有"分镜剧本 → 资产 → 镜头视频"前三步节点（已经在跑）
- **不动**：3D 导演台、全景图、画布节点系统、Seedance 转译链路、画布助手
- **不动**：现有提示词、素材库、卡密、扣费、admin 后台大部分功能
- **不动**：novel/page.tsx 已有的 4 段流水线 stepper 升级为 6 段（或 9 段，含新增节点），但状态机逻辑复用

## 新增能力

下面是本提案在 OpenSpec 中拆分出的 capability（每个对应 OpenSpec 的一份 spec），按"工作流节点"组织而非"技术模块"组织：

- `novel-storyboard-workflow`: 小说剧本到成片的工作流编排层（核心）——把整条流水线建模成可观察的多节点状态机，节点之间是声明式依赖，支持单步运行 / 暂停 / 恢复 / 重试 / 介入编辑
- `shot-dubbing-node`: 镜头级配音节点——每个分镜可挂载 TTS 配音（可插拔 TTS 提供方），支持试听 / 重新生成 / 调音色 / 调语速
- `shot-subtitle-node`: 镜头级字幕节点——每个分镜可挂载字幕时间轴（自动按字数切分），支持合成前手动编辑 + 全局样式
- `final-assembly-node`: 成片合成节点——BGM 选曲 + ffmpeg 多镜头拼接 + 配音混音 + 字幕烧录 + 片头 / 片尾 + mp4 输出 + 导出分享；合成任务有独立的状态机（排队 / 进行中 / 成功 / 失败 / 已取消）和可轮询进度

## 修改能力

无现有 capability 受 spec 级行为变更影响。novel 工作台现有功能（novel/page.tsx、storyboard_tasks 后端任务化、视频节点、画布节点、导演台、Asset 模板、Seedance 转译链路）均保持原样；新工作流是叠加在现有产出之上的"后半段"，不修改它们的契约。

## 影响

### 后端新增

- `model/novel_workflow_run.go`、`model/novel_workflow_node.go`：工作流任务 + 节点状态表
- `model/shot_dubbing.go`、`model/shot_subtitle.go`、`model/bgm.go`、`model/video_assembly.go`、`model/share_link.go`
- `repository/*.go`：对应 CRUD
- `service/novel_workflow.go`：工作流编排器（节点依赖图、状态机、worker 池、停止 / 恢复 / 重试 / 单步运行）
- `service/tts.go`：TTS Provider 接口 + MiMo TTS 实现 + 工厂
- `service/novel_subtitle.go`：字幕时间轴计算（字数线性切分）
- `service/ffmpeg_runner.go`：ffmpeg 子进程 + 进度解析（`-progress pipe:1`）
- `service/novel_assembly.go`：ffmpeg 命令编排（concat + amix + ass filter + xfade）
- `service/bgm.go`：admin BGM CRUD + 标签筛选
- `handler/novel_workflow.go`、`handler/novel_dubbing.go`、`handler/novel_subtitle.go`、`handler/novel_assembly.go`、`handler/bgm.go`、`handler/share.go`、`handler/novel_export.go`
- `router/router.go`：追加路由
- `main.go`：启动工作流 worker 池

### 后端修改

- `config/config.go` + `.env.example`：增加 `FfmpegBinaryPath`、`AssemblyOutputDir`、`AssemblyWorkerCount`、`TtsProvider`、`ShareRetentionDays`、`EnableNovelWorkflow`
- `repository/db.go`：AutoMigrate 追加新表
- `service/model_dispatch.go`（如需要）：接入 TTS Provider 调度，复用现有扣费 / 退款流程

### 前端新增

- `web/src/services/api/novel_workflow.ts`、`novel_dubbing.ts`、`novel_subtitle.ts`、`novel_assembly.ts`、`bgm.ts`、`novel_export.ts`
- `web/src/app/(user)/novel/components/novel-workflow-graph.tsx`：工作流节点图（横向步骤条 + 每个节点状态 + 单步操作）
- `web/src/app/(user)/novel/components/novel-dubbing-card.tsx`：单镜头配音卡（▶ 试听 / 重新生成 / 音色下拉 / 语速滑杆）
- `web/src/app/(user)/novel/components/novel-subtitle-editor.tsx`：单镜头字幕编辑器
- `web/src/app/(user)/novel/components/novel-bgm-picker.tsx`：BGM 选曲器
- `web/src/app/(user)/novel/components/novel-assembly-panel.tsx`：合成配置 + 进度 + 输出
- `web/src/app/(user)/novel/components/novel-export-modal.tsx`：导出弹窗
- `web/src/app/(admin)/admin/bgm/page.tsx`：admin BGM 管理页

### 前端修改

- `web/src/app/(user)/novel/page.tsx`：4 段 stepper 升级为 6 段（含配音 / 字幕 / 合成）；「一键出片」按钮升级为「启动工作流」；新增「分步视图」入口
- `web/src/app/(admin)/admin/page.tsx`：菜单新增「BGM 库」

### Docker 镜像

- 基础镜像安装 `ffmpeg`（含 libass）

### 外部依赖

- ffmpeg 二进制（系统级）
- MiMo TTS（已有 v0.5.2）
- 对象存储（MinIO / S3 / R2，已有）

## 关键设计决策

1. **工作流是声明式节点图，不是命令式脚本**：每个节点暴露「输入依赖 / 输出 / 状态机 / 操作」四要素；编排器读声明即可推理出下一步能跑哪个节点。
2. **节点状态机是统一的**（7 态），便于 UI 复用和后端 worker 复用。
3. **TTS / 字幕 / BGM 都是"节点级可降级"**：任何一个失败，合成仍可完成（用静音 / 跳过字幕 / 不用 BGM），不阻塞用户得到成片。
4. **不接抖音 / 小红书官方开放平台**：v1 只生成可复制的发布文案 + 分享链接，用户自己粘贴。
5. **不引入 ASR 字幕重对齐**：v1 按字数线性切，编辑器兜底修正。
6. **不引入新角色权限**：复用 user / admin 切分。
7. **不单独收配音 / 字幕 / 合成费用**：TTS 调用从用户余额扣（与现有文本模型扣费一致），字幕 / BGM / 合成不单独计费。
