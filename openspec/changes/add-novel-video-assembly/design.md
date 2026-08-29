## 背景

Freedom novel 工作台已具备前半段链路（剧本 → 分镜剧本 → 资产 → 单条视频），缺后半段（配音 / 字幕 / 合成 / 导出）。本次新增后半段，并把它建模为一条「多节点 + 用户可见 + 可单步干预」的工作流（参考小云雀"多 Agent 协同 + 用户可见流水线"范式），保留 Freedom 现有的节点级编辑、3D 导演台、画布协作等差异化能力。

当前项目跑 Go（Gin + GORM，MySQL prod / SQLite dev）后端，Next.js（App Router）前端，Docker 镜像中**未安装 ffmpeg**，对象存储可配（MinIO / S3 / R2）。MiMo TTS 已在 v0.5.2 引入但未接入 novel 流程。

约束：

- C1. 不破坏现有 storyboard_tasks 流水线、画布节点系统、3D 导演台链路
- C2. ffmpeg 装在 Go 服务 Docker 镜像中，合成任务**只在服务端跑**（不走 ffmpeg.wasm）
- C3. 长任务（配音 / 合成）必须遵循现有的 task + 轮询模式（参见 `docs/progress/todo.md` 的 storyboard_tasks）
- C4. 所有媒体资产（配音 mp3 / 字幕 srt / BGM 文件 / 最终 mp4）走现有对象存储
- C5. TTS 调用复用现有 BalanceLog 扣费 / 退款
- C6. 工作流节点是"声明式依赖图"而非"命令式脚本"——编排器读声明即可推理

## 目标 / 非目标

**目标：**

- 把后半段（配音 / 字幕 / BGM / 合成 / 导出）建模为 4 个新增工作流节点，挂到 novel 工作流图上
- 工作流节点遵循统一状态机（7 态）和统一操作（开始 / 停止 / 重试 / 单步编辑）
- 工作流跑起来后用户能切到「分步视图」逐节点看进度，且可以只跑单个节点
- TTS 配音支持可插拔提供方
- 字幕时间轴可手动编辑
- ffmpeg 合成任务可轮询、可停止、可重试
- 分享链接有过期时间，可撤销

**非目标：**

- 不引入新的角色 / 权限模型（复用 user / admin 切分）
- 不接抖音 / 小红书 / 视频号官方开放平台（只生成可复制的发布文案）
- 不做 ASR 字幕重对齐（v1 走字数线性切分 + 手动编辑）
- 不做实时协作编辑字幕 / 配音
- 不把 3D 导演台的相机运动直接 bake 进成片（成片只读"已生成好的 mp4"）
- 不引入新扣费点（配音从余额扣，字幕 / BGM / 合成不单独计费）

## 关键决策

### D1. 工作流节点 = 「声明式四要素」

每个工作流节点是一个数据结构，包含：

- **id**：节点唯一标识
- **kind**：节点类型（脚本 / 分镜 / 资产 / 视频 / 配音 / 字幕 / 合成）
- **dependsOn**：上游节点 ID 列表
- **state**：当前状态（7 态枚举）
- **input / output**：节点产物的引用（mp3 URL / srt URL / mp4 URL 等）

编排器读节点图即可推理：① 哪些节点可启动（上游全成功）② 谁阻塞了谁 ③ 整体状态如何聚合。前端 UI 直接用同一份图渲染（节点图 + 状态徽标 + 操作按钮）。

**为什么**：避免"散落的 if-else 编排逻辑"。新增节点只需声明四要素，不必改编排器。

**备选**：命令式脚本（一个长函数依次调各步骤）——拒绝，无法支持单步运行 / 介入编辑。

### D2. 工作流节点状态机统一 7 态

`未启动 / 排队中 / 进行中 / 成功 / 失败 / 跳过 / 已取消`，加 4 个统一操作（开始 / 停止 / 重试 / 介入编辑）。

**为什么**：和现有 `storyboard_tasks` 的状态机对齐；前端组件可复用；后端 worker 逻辑可复用。

### D3. 工作流任务持久化为「运行 + 节点」两张表

- `novel_workflow_rumm`（id、project_id、user_id、mode=自动|手动、started_at、finished_at、总体状态）
- `novel_workflow_nodes`（id、run_id、node_id、kind、depends_on_json、state、progress_json、output_json、error_log、started_at、finished_at）

每次工作流启动创建一条 run，节点数 = 工作流图节点数 × 1。刷新 / 切设备时直接读这张表恢复。

**为什么**：和现有 storyboard_tasks 范式一致；便于多设备同步；便于 admin 排查。

### D4. TTS Provider 接口抽象

```go
type TTSProvider interface {
    Synthesize(ctx, text string, opts TTSOpts) (audioURL string, durationMs int64, err error)
}
```

`mimoTTSProvider` 是默认实现；可注册 `volcanoTTSProvider` / `openaiTTSProvider` / `elevenLabsProvider` 等。admin 在系统设置里选一个。

**为什么**：v0.5.2 已经引入 MiMo TTS 但未抽接口；本次顺手抽出来，避免后续接入其他 TTS 重复劳动。

### D5. 字幕时间轴按字数线性切分

每个分镜取「对白 / 旁白」文本，按标点切 N 行，按字数比例分配到该分镜视频时长内。

**为什么**：v1 算法；不调 ASR 节省成本和延迟；编辑器兜底修正。

**备选**：调 ASR 强制对齐——拒绝，成本 / 延迟过高，v1 收益不匹配。

### D6. ffmpeg 在 Docker 镜像中、concat demuxer + filter_complex

基础镜像装 ffmpeg（带 libass）；合成命令模板：

```
ffmpeg \
  -f concat -safe 0 -i concat_list.txt \
  -i dubbing_1.mp3 -i dubbing_2.mp3 ... \
  -i bgm.mp3 \
  -filter_complex "[a:dubbing]concat=n=N:v=0:a=1[concat_a];[concat_a][bgm]amix=inputs=2:duration=first:dropout_transition=0[amixed];[0:v]ass=subtitle.ass[v]" \
  -map "[v]" -map "[amixed]" \
  -c:v libx264 -pix_fmt yuv420p -c:a aac \
  output.mp4
```

**为什么**：concat demuxer 处理同 codec 流最简单；filter_complex 覆盖混音 / 烧字幕 / 转场；libx264 + aac 兼容性最好。

**备选**：把所有镜头先归一化为统一中间 mp4——会多花 30% 时间，但能避开 codec 不一致问题。**最终选**：先做轻量归一化（`-c:v libx264 -pix_fmt yuv420p -c:a aac` 5 秒内完成），再走 concat；兼顾速度与稳定。

### D7. ffmpeg 进度解析用 `-progress pipe:1`

合成 worker 启动 ffmpeg 时传 `-progress pipe:1 -nostats`，解析 `out_time_ms` / `progress=continue|end` 输出，把当前步骤（归一化 / 拼接 / 混音 / 烧字幕 / 输出）写到 `video_assemblies.progress_json`，前端 3 秒轮询。

**为什么**：不依赖视频时长已知；不引入 sidecar 文件；前端直接拿到分步骤进度。

### D8. 节点失败 = 整节点跳过，不阻塞工作流

- 配音节点某分镜 TTS 失败 → 该分镜用静音合成
- 字幕节点某分镜无字幕 → 该分镜不烧字幕
- BGM 未选 → 不混 BGM
- 镜头视频缺失 → 该分镜跳过并记 warning

合成任务整体仍标为「成功」（前提是 ffmpeg 进程退出码为 0）。

**为什么**：用户的核心需求是"拿到一段可看的成片"，而不是"每一步必须完美"。

### D9. 分享链接用 16 字节随机 token + 过期时间

```go
token := base64.RawURLEncoding.EncodeToString(randBytes(16))  // 22 字符
expiresAt := time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour)
```

访问 `/api/v1/share/{token}` 时查表，过期 / 撤销返回 404。

**为什么**：用 token 而非 signed S3 URL 是为了支持"撤销立即生效"和"每条导出独立记录"。

### D10. 工作流分自动 / 手动两种模式

- **自动模式**：上游节点成功后下游节点自动启动（默认）
- **手动模式**：用户必须手动点每个节点的「开始」

用户在 novel 工作台顶部可切换。手动模式适合"我只想单独跑一下配音 / 字幕看看效果"的场景。

## 风险 / 取舍

- **R1：ffmpeg 二进制不在开发机** → 缓解：README 写安装说明；CI 跑合成测试只在 Docker 镜像中；启动时 smoke check（`ffmpeg -version`）失败就拒绝启动 worker。
- **R2：libass 未编译进 ffmpeg** → 缓解：基础镜像固定为 `mwader/static-ffmpeg:7.1`（自带 libass）；启动时跑一次 sample srt → sample mp4 烧录测试。
- **R3：长合成任务让用户觉得卡** → 缓解：进度条按 5 个步骤展示（不只显示百分比），3 秒轮询；「停止」按钮可立即 kill ffmpeg。
- **R4：TTS 烧余额** → 缓解：仅在 TTS 返回可用 mp3 后扣费；失败自动退款；流水记 BalanceLog。
- **R5：BGM 大文件浪费存储** → 缓解：admin 上传限制 20MB；超限拒绝。
- **R6：分享链接泄漏** → 缓解：默认 30 天过期；可撤销；token 16 字节随机。
- **R7：字幕编辑器过度工程** → 缓解：v1 只做文字 / 起止 / 增删，不做富文本 / 字体 / 动画。
- **R8：不同模型输出 codec 不一致导致 concat 失败** → 缓解：合成前用轻量归一化把所有镜头视频转 h264 / yuv420p / aac。
- **R9：工作流节点数量多（9 个）使 UI 拥挤** → 缓解：横向步骤条 + 默认折叠前 3 个已有节点（脚本 / 分镜 / 资产），只展示后半段 4 个新节点；可点"展开全部"。

## 部署计划

- **无数据迁移**。本变更只新增表（`novel_workflow_runs` / `novel_workflow_nodes` / `shot_dubbings` / `shot_subtitles` / `bgm_tracks` / `video_assemblies` / `share_links`）和路由，不影响现有 novel 项目、分镜、视频节点。
- **Auto-migrate** 通过 GORM（`repository/db.go` 中追加新 model）。
- **Docker 镜像升级**：基础镜像装 ffmpeg（带 libass），`Dockerfile` 加 `apt-get install ffmpeg`（或换成多阶段构建用 static-ffmpeg 镜像）。
- **回滚**：删新表，删路由，不影响现有功能。
- **特性开关**：新增 `system_settings.enable_novel_workflow`，默认 `true`（新装）/ `false`（老装），让 operator 在确认 ffmpeg 装好后开启。
- **运行时检查**：main.go 启动时执行 `ffmpeg -version`，缺失就在日志里 warn，但**不**阻止启动（这样开发机可以照常 dev，只在跑合成时报错）。

## 待定问题

- OQ1：自动模式默认开还是关？**默认开**（更符合"一键出片"心智），用户可切手动。
- OQ2：BGM 是否允许 per-shot 切换？**v1 不允许**（整部一首），如需要后续加 per-shot 字段。
- OQ3：分享链接默认过期时间？**30 天 admin 可配**。
- OQ4：工作流节点图是否支持用户自定义（拖拽添加 / 删除节点）？**v1 不支持**（9 个节点是硬编码），后续可加。
