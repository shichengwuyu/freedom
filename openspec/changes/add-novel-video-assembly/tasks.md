## 1. 运行时与构建基础

- [ ] 1.1 在 Go 服务 Docker 镜像中装 ffmpeg（带 libass），并在运行的容器里执行 `ffmpeg -version` 验证可用
- [ ] 1.2 在 `config/config.go` 和 `.env.example` 增加 `FfmpegBinaryPath`、`AssemblyOutputDir`、`AssemblyWorkerCount`、`TtsProvider`、`ShareRetentionDays`、`EnableNovelWorkflow`
- [ ] 1.3 main.go 启动时打 ffmpeg 版本日志，缺失时 warn（不阻止启动，让 dev 机可以照常开发）

## 2. 工作流编排层（核心底座）

- [ ] 2.1 定义 `model/novel_workflow_run.go`（NovelWorkflowRun）和 `model/novel_workflow_node.go`（NovelWorkflowNode），并加入 `repository/db.go` 的 AutoMigrate
- [ ] 2.2 定义工作流节点图（`service/novel_workflow_graph.go`）：9 个节点 + 依赖关系 + 节点类型 → 4 个能力（脚本 / 分镜 / 资产 / 视频 / 配音 / 字幕 / 合成）映射
- [ ] 2.3 实现 `service/novel_workflow.go` 编排器：状态机（7 态）、操作（开始 / 停止 / 重试）、依赖推进、自动 / 手动模式、worker 池；写单元测试覆盖「上游失败 → 下游不启动」「自动模式下上游成功 → 下游自动启动」
- [ ] 2.4 实现 `handler/novel_workflow.go`：POST 创建 run、GET run 状态、POST 单节点启动 / 停止 / 重试；注册路由
- [ ] 2.5 main.go 启动工作流 worker 池（默认 2 个）；并发起新 run 时自动派发就绪节点
- [ ] 2.6 端到端验证：起一个 3 分镜项目 → 启动 run → 验证 9 个节点状态机正确推进、刷新后状态恢复

## 3. 镜头配音节点

- [ ] 3.1 定义 `service.TTSProvider` 接口（`service/tts.go`）+ 工厂
- [ ] 3.2 实现 `mimoTTSProvider`（HTTP 调 MiMo TTS，返回 mp3 + 时长），加单元测试（mock HTTP 层）
- [ ] 3.3 把 TTS Provider 接入现有 ModelDispatch 扣费 / 退款（BalanceLog）；测试成功扣费 / 失败退款两条路径
- [ ] 3.4 `model/shot_dubbing.go`（ShotDubbing）+ AutoMigrate
- [ ] 3.5 `repository/shot_dubbing.go` CRUD
- [ ] 3.6 `service/novel_dubbing.go`（DispatchForShot 含重试 2 次、GetForShot、ReDispatch、ListForProject）；测试 TTS 失败 2 次后该分镜标空、整体节点不阻塞
- [ ] 3.7 `handler/novel_dubbing.go`（POST dispatch、GET list、POST re-dispatch）；注册路由
- [ ] 3.8 前端 `web/src/services/api/novel_dubbing.ts`（dispatch / list / re-dispatch）
- [ ] 3.9 前端 `web/src/app/(user)/novel/components/novel-dubbing-card.tsx`：单镜头配音卡（▶ 试听 / 重新生成 / 音色下拉 / 语速滑杆）
- [ ] 3.10 `web/src/app/(user)/novel/page.tsx` 把配音卡挂到分镜详情面板；视频节点成功时自动启动配音节点
- [ ] 3.11 端到端验证：起 run → 视频节点成功 → 配音节点自动跑 → 试听 mp3 → 切换音色重新生成

## 4. 镜头字幕节点

- [ ] 4.1 `model/shot_subtitle.go`（ShotSubtitle 含 lines_json / style_json）+ AutoMigrate
- [ ] 4.2 `repository/shot_subtitle.go` CRUD
- [ ] 4.3 `service/novel_subtitle.go`（ComputeTimeline 按字数线性切分，GetStyle / SetStyle）；单元测试覆盖空文本 / 单行 / 多行 / 自定义时长
- [ ] 4.4 `handler/novel_subtitle.go`（GET / PUT 文本+起止 / PUT 全局样式）；注册路由
- [ ] 4.5 前端 `web/src/services/api/novel_subtitle.ts`
- [ ] 4.6 前端 `web/src/app/(user)/novel/components/novel-subtitle-editor.tsx`：单镜头字幕编辑器（行列表 + 文字编辑 + 起止拖动 + 增删）
- [ ] 4.7 全局字幕样式面板（字体 / 颜色 / 描边 / 位置 / 字号）挂在 novel 工作台顶部
- [ ] 4.8 端到端验证：编辑某行字幕文字 → 刷新仍在 → 切换全局样式 → 样式在 UI 上反映

## 5. 成片合成节点

- [ ] 5.1 `model/video_assembly.go`（VideoAssembly：id / project_id / user_id / status / progress_json / output_url / error_log / started_at / finished_at）+ AutoMigrate
- [ ] 5.2 `repository/video_assembly.go`（Create / UpdateProgress / UpdateStatus / GetById / ListByProject）
- [ ] 5.3 `service/ffmpeg_runner.go`（RunFfmpegWithProgress：exec ffmpeg 带 `-progress pipe:1`，解析进度行，发步骤事件到 channel）；单元测试运行已知 ffmpeg 命令并验证步骤事件触发
- [ ] 5.4 `service/novel_assembly.go`（Compose：按分镜顺序构建 concat 列表、构建 ffmpeg 命令（concat + amix + ass filter + xfade）、通过 RunFfmpegWithProgress 跑、输出 mp4 落对象存储）；端到端测试 3 分镜 fixture 项目产出可播放 mp4
- [ ] 5.5 `handler/novel_assembly.go`（POST create / GET status / POST stop / POST retry）；注册路由
- [ ] 5.6 main.go 把合成任务接进工作流 worker 池
- [ ] 5.7 前端 `web/src/services/api/novel_assembly.ts`
- [ ] 5.8 前端 `web/src/app/(user)/novel/components/novel-assembly-panel.tsx`：合成配置（字幕样式 / BGM / 片头片尾）+ 「一键合成成片」按钮 + 5 步进度条 + 停止 / 重试按钮 + 输出播放器
- [ ] 5.9 `web/src/app/(user)/novel/page.tsx` 把合成面板挂到工作流图；持久化合成配置到 NovelProject
- [ ] 5.10 端到端验证：起 run → 跑合成 → 进度按 5 步推进 → 成功 → 播放器加载 mp4

## 6. BGM 库（admin 端 + user 端）

- [ ] 6.1 `model/bgm.go`（BgmTrack：id / title / description / tags_json / file_url / mime_type / size_bytes / created_by / created_at）+ AutoMigrate
- [ ] 6.2 `repository/bgm.go` CRUD + 标签筛选
- [ ] 6.3 `service/bgm.go`（Upload multipart → 对象存储 `novel/bgm/`、Delete 同步清理对象、20MB 大小限制、MIME 校验）；测试 25MB 文件被拒
- [ ] 6.4 `handler/admin_bgm.go`（admin upload / edit / delete / list）；`handler/bgm.go`（user list 按标签筛选 / preview URL）；注册路由
- [ ] 6.5 前端 `web/src/services/api/bgm.ts`
- [ ] 6.6 前端 `web/src/app/(admin)/admin/bgm/page.tsx`：admin BGM 管理页（上传表单 + 列表 + 编辑弹窗）
- [ ] 6.7 前端 `web/src/app/(user)/novel/components/novel-bgm-picker.tsx`：标签筛选 + 列表 + 试听 + 选用 + 音量 / 淡入 / 淡出滑杆
- [ ] 6.8 端到端验证：admin 上传 1 个 mp3 → 用户在前端看到 → 试听 → 选用 → 合成时该 BGM 被混音进 mp4

## 7. 导出与分享

- [ ] 7.1 `model/share_link.go`（ShareLink：id / token / assembly_id / expires_at / revoked_at）+ AutoMigrate
- [ ] 7.2 `repository/share_link.go` + service（生成 16 字节 token、设置过期、写表）
- [ ] 7.3 `handler/share.go`（GET /api/v1/share/:token 服务 mp4 + Content-Type 正确）；curl 测试匿名请求返回 mp4
- [ ] 7.4 `handler/novel_export.go`（POST issue 分享链接 / GET export 历史 / POST revoke）；注册路由
- [ ] 7.5 前端 `web/src/services/api/novel_export.ts`
- [ ] 7.6 前端 `web/src/app/(user)/novel/components/novel-export-modal.tsx`：下载 / 复制分享链接 / 复制平台文案 / 列表 + 撤销
- [ ] 7.7 端到端验证：合成成功 → 生成分享链接 → 粘贴到无痕窗口 → 播放成功 → 撤销后访问 404

## 8. novel 工作流图 UI

- [ ] 8.1 前端 `web/src/app/(user)/novel/components/novel-workflow-graph.tsx`：横向 9 节点 + 状态徽标 + 单步操作（开始 / 停止 / 重试）+ 总体状态徽标
- [ ] 8.2 `web/src/app/(user)/novel/page.tsx` 把 4 段 stepper 升级为 novel-workflow-graph 组件；4 段折叠为前 4 节点（脚本 / 分镜 / 资产 / 视频）默认显示，后 5 节点（配音 / 字幕 / BGM / 合成 / 导出）可点"展开全部"
- [ ] 8.3 自动 / 手动模式开关挂在 novel 工作台顶部
- [ ] 8.4 端到端验证：自动模式下视频节点成功后配音 / 字幕 / 合成自动跑；手动模式下需要逐个点开始

## 9. 系统设置与 admin 菜单

- [ ] 9.1 admin 系统设置页增加 `enableNovelWorkflow`（boolean）、`ttsProvider`（select）、`shareRetentionDays`（number）；保存后立即生效
- [ ] 9.2 admin 侧边栏新增「BGM 库」入口，可从 admin home 跳到 BGM 管理页
- [ ] 9.3 当 `enableNovelWorkflow = false` 时，novel 工作流图上「一键合成成片」按钮隐藏，相关 API 返回 404

## 10. 跨切关注点：清理、扣费、文档

- [ ] 10.1 加每日 cron：清理过期 share_links + 对应对象存储 mp4
- [ ] 10.2 加每日 cron：清理 30 天以上且无活跃 share_link 关联的 video_assemblies + 对应 mp4
- [ ] 10.3 端到端验证余额：TTS 成功扣费、失败退款、BalanceLog 两条都写；通过 `/api/v1/balance-logs` 查到
- [ ] 10.4 更新 `docs/overview/features.md` 加「成片工作流」段
- [ ] 10.5 新增 `docs/canvas/novel-workflow.md` 用户指南（工作流图怎么用 / 配音怎么调 / 字幕怎么改 / BGM 怎么选 / 怎么分享）
- [ ] 10.6 `deploy/` 加 ffmpeg 镜像升级 + 新环境变量说明
- [ ] 10.7 `CHANGELOG.md` Unreleased 加新能力条目 + ffmpeg 运行时要求
