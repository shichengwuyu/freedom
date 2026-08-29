## 1. 运行时与构建基础

- [ ] 1.1 在 Go 服务 Docker 镜像中装 ffmpeg（带 libass），并在运行的容器里执行 `ffmpeg -version` 验证可用
- [ ] 1.2 准备 6-8 个系统预设 BGM mp3 文件，放到 `assets/bgm-presets/`，并在 Docker 镜像中 COPY
- [ ] 1.3 在 `config/config.go` 和 `.env.example` 增加 `FfmpegBinaryPath`、`CompositionOutputDir`、`CompositionWorkerCount`、`TtsProvider`、`EnableSeriesAssetLock`
- [ ] 1.4 main.go 启动时打 ffmpeg 版本 + BGM 预设数日志，缺失时 warn

## 2. novel-storyboard-workflow（编排底座）

- [ ] 2.1 定义 `model/novel_workflow_run.go`（NovelWorkflowRun）和 `model/novel_workflow_node.go`（NovelWorkflowNode），并加入 `repository/db.go` 的 AutoMigrate
- [ ] 2.2 定义 5 层节点图（`service/novel_workflow_graph.go`）：输入 / 剧本 / 资产 / 镜头 / 后期；每层包含哪些子节点
- [ ] 2.3 实现 `service/novel_workflow.go` 编排器：7 态状态机、开始 / 停止 / 重试、自动 / 手动模式、快速 / 自定义双模式、worker 池；写单元测试覆盖「上游失败 → 下游不启动」「自动模式下上游成功 → 下游自动启动」
- [ ] 2.4 实现 `handler/novel_workflow.go`：POST 创建 run、GET run 状态、POST 单节点启动 / 停止 / 重试；注册路由
- [ ] 2.5 main.go 启动工作流 worker 池（默认 2 个）；并发起新 run 时自动派发就绪节点
- [ ] 2.6 前端 `web/src/services/api/novel_workflow.ts`
- [ ] 2.7 前端 `web/src/app/(user)/novel/components/novel-workflow-layers.tsx`：5 层横向步骤条 + 每层可展开子节点 + 单步操作 + 总体状态徽标
- [ ] 2.8 `web/src/app/(user)/novel/page.tsx` 把 4 段 stepper 升级为 novel-workflow-layers；新增「快速出片」+「自定义出片」两个按钮
- [ ] 2.9 端到端验证：起一个 3 分镜项目 → 启动 run → 验证 5 层状态机正确推进、刷新后状态恢复

## 3. shot-dubbing-node

- [ ] 3.1 定义 `service.TTSProvider` 接口（`service/tts.go`）+ 工厂
- [ ] 3.2 实现 `mimoTTSProvider`（HTTP 调 MiMo TTS，返回 mp3 + 时长），加单元测试
- [ ] 3.3 把 TTS Provider 接入现有扣费 / 退款（BalanceLog）；测试成功扣费 / 失败退款两条路径
- [ ] 3.4 `model/shot_dubbing.go`（ShotDubbing）+ AutoMigrate
- [ ] 3.5 `repository/shot_dubbing.go` CRUD
- [ ] 3.6 `service/novel_dubbing.go`（DispatchForShot 含重试 2 次、GetForShot、ReDispatch、ListForProject）；测试 TTS 失败 2 次后该分镜标空
- [ ] 3.7 `handler/novel_dubbing.go`（POST dispatch、GET list、POST re-dispatch）；注册路由
- [ ] 3.8 前端 `web/src/services/api/novel_dubbing.ts`
- [ ] 3.9 前端 `web/src/app/(user)/novel/components/novel-dubbing-card.tsx`：单镜头配音卡（▶ 试听 / 重新生成 / 音色下拉 / 语速滑杆）
- [ ] 3.10 `web/src/app/(user)/novel/page.tsx` 把配音卡挂到分镜详情面板
- [ ] 3.11 端到端验证：起 run → 视频节点成功 → 配音节点自动跑 → 试听 mp3 → 切换音色重新生成

## 4. shot-subtitle-node

- [ ] 4.1 `model/shot_subtitle.go`（ShotSubtitle 含 lines_json / style_json）+ AutoMigrate
- [ ] 4.2 `repository/shot_subtitle.go` CRUD
- [ ] 4.3 `service/novel_subtitle.go`（ComputeTimeline 按字数线性切分，GetStyle / SetStyle）；单元测试
- [ ] 4.4 `handler/novel_subtitle.go`（GET / PUT 文本+起止 / PUT 全局样式）；注册路由
- [ ] 4.5 前端 `web/src/services/api/novel_subtitle.ts`
- [ ] 4.6 前端 `web/src/app/(user)/novel/components/novel-subtitle-editor.tsx`：单镜头字幕编辑器
- [ ] 4.7 全局字幕样式面板挂在 novel 工作台顶部
- [ ] 4.8 端到端验证：编辑某行字幕文字 → 刷新仍在 → 切换全局样式 → 样式在 UI 上反映

## 5. bgm-layer

- [ ] 5.1 准备 6-8 个 BGM 预设文件 mp3 + 元数据 JSON（风格 / 时长 / 文件名），落到 `assets/bgm-presets/`
- [ ] 5.2 `model/bgm_preset.go`（BgmPreset 内置）+ `model/bgm_custom.go`（用户上传，自定义）+ AutoMigrate
- [ ] 5.3 `repository/bgm_preset.go` ListPresets / GetByTag；`repository/bgm_custom.go` CRUD
- [ ] 5.4 `service/bgm_preset.go`（ListPresets、UploadCustom、DeleteCustom with 对象存储 cleanup、20MB 大小限制、MIME 校验）
- [ ] 5.5 `handler/bgm.go`（user-only list 预设 / 上传自定义 / 删除自定义）；**不做 admin BGM 库管理**
- [ ] 5.6 前端 `web/src/services/api/bgm.ts`
- [ ] 5.7 前端 `web/src/app/(user)/novel/components/novel-bgm-preset-picker.tsx`：标签筛选 + 列表 + 试听 + 选用 + 音量 / 淡入 / 淡出滑杆
- [ ] 5.8 端到端验证：选预设 BGM → 试听 → 选用 → 合成时该 BGM 被混音进 mp4

## 6. composition-layer

- [ ] 6.1 `model/composition_task.go`（CompositionTask：id / project_id / user_id / status / progress_json / output_url / error_log / started_at / finished_at）+ AutoMigrate
- [ ] 6.2 `repository/composition_task.go`（Create / UpdateProgress / UpdateStatus / GetById / ListByProject）
- [ ] 6.3 `service/ffmpeg_runner.go`（RunFfmpegWithProgress：exec ffmpeg 带 `-progress pipe:1`，解析进度行，发步骤事件到 channel）
- [ ] 6.4 `service/novel_composition.go`（ComposeFull 5 步流水线、ComposeSubtitleOnly 仅跑步骤 ④ 烧字幕）；端到端测试 3 分镜 fixture 项目产出可播放 mp4
- [ ] 6.5 `handler/novel_composition.go`（POST create / GET status / POST stop / POST retry）；注册路由
- [ ] 6.6 main.go 把合成任务接进工作流 worker 池
- [ ] 6.7 前端 `web/src/services/api/novel_composition.ts`
- [ ] 6.8 端到端验证：起 run → 跑合成 → 进度按 5 步推进 → 成功

## 7. export-layer

- [ ] 7.1 `service/novel_export.go`（GeneratePlatformCaption 根据项目元数据生成抖音 / 小红书 / 视频号文案）
- [ ] 7.2 `handler/novel_export.go`（GET 成片元数据、POST 生成文案）；**不做分享链接**
- [ ] 7.3 前端 `web/src/services/api/novel_export.ts`
- [ ] 7.4 前端 `web/src/app/(user)/novel/components/novel-export-panel.tsx`：下载按钮 + 平台文案（可编辑）+ 元数据展示
- [ ] 7.5 端到端验证：合成成功 → 点下载 → 浏览器下载 mp4 + 复制平台文案

## 8. novel-rerun-layer（核心 UX）

- [ ] 8.1 `model/rerun_record.go`（RerunRecord：id / project_id / scope=分镜|整部 / layer=video|dubbing|subtitle|composition / version / payload_json / created_at）+ AutoMigrate
- [ ] 8.2 `repository/rerun_record.go` ListByScope / SaveRecord / GetLatestVersion
- [ ] 8.3 `service/novel_rerun.go`（RerunShotLayer、ReRunFullLayer、ReRollSubtitleOnly、RollbackToVersion）；所有重做走 novel-workflow 节点状态机
- [ ] 8.4 `handler/novel_rerun.go`（POST /rerun/shot/:shotId/layer/:layer、POST /rerun/full/layer/:layer、GET /rerun/shot/:shotId/versions）；注册路由
- [ ] 8.5 前端 `web/src/services/api/novel_rerun.ts`
- [ ] 8.6 前端 `web/src/app/(user)/novel/components/novel-composition-view.tsx`：左播放器 + 右分镜列表 + 顶版本 tab
- [ ] 8.7 前端 `web/src/app/(user)/novel/components/novel-rerun-panel.tsx`：单分镜 4 个重做按钮 + 整部成片 3 个按钮
- [ ] 8.8 端到端验证：合成成功 → 成片视图 → 改全局字幕样式 → 重烧字幕 → 新成片更新；单分镜重做配音 → 仅该分镜变化

## 9. series-asset-lock（漫剧一致性）

- [ ] 9.1 `model/series_asset_lock.go`（SeriesAssetLock：id / project_id / character_ids / scene_ids / prop_ids / global_style_prompt / is_locked / created_at）+ AutoMigrate
- [ ] 9.2 `repository/series_asset_lock.go` CRUD
- [ ] 9.3 `service/series_asset_lock.go`（Lock、Unlock、GetForProject、IsLocked）；未锁定时视频生成按现有行为
- [ ] 9.4 视频生成（`service/novel_video.go` 或 `handler/video_task.go`）改造：锁定时强制从主资产包取参考图；锁定时资产选择器只显示主资产包
- [ ] 9.5 `handler/series_asset_lock.go`（POST lock / POST unlock / GET / PUT）；注册路由
- [ ] 9.6 前端 `web/src/services/api/series_asset_lock.ts`
- [ ] 9.7 前端 `web/src/app/(user)/novel/components/novel-series-asset-lock-panel.tsx`：拖入资产 + 设全局色调 prompt + 锁定 / 解锁按钮
- [ ] 9.8 端到端验证：锁定主资产包（含 2 角色 2 场景）→ 跑视频生成 → 参考图只来自主资产包；解锁后恢复自由选资产

## 10. 跨切关注点：清理、扣费、文档

- [ ] 10.1 加每日 cron：清理 30 天以上且无活跃引用的 composition_tasks + 对应 mp4
- [ ] 10.2 加每日 cron：清理 30 天以上的 rerun_records
- [ ] 10.3 端到端验证余额：TTS 成功扣费、失败退款、BalanceLog 两条都写
- [ ] 10.4 更新 `docs/overview/features.md` 加「成片工作流」段
- [ ] 10.5 新增 `docs/canvas/novel-workflow.md` 用户指南
- [ ] 10.6 `deploy/` 加 ffmpeg 镜像升级 + BGM 预设 + 新环境变量说明
- [ ] 10.7 `CHANGELOG.md` Unreleased 加新能力条目
