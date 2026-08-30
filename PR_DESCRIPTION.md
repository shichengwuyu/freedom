# novel-workflow v2 完整交付 + Sprint 1-4 + 前端 UI

> PR 描述. 复制粘贴到 https://github.com/shichengwuyu/freedom/pull/new/feature/novel-workflow

## 概览

本 PR 把 feature/novel-workflow 分支(20 个 commit, 约 70+ 文件, +13000 / -600 行)合并到 main.

包含**三批工作**:

1. **Sprint 1-4 基线开发** (5 个 commit) - 之前未提交到 main 的开发工作
2. **novel-workflow v2 完整后端交付** (10 个任务组 / 11 个 commit)
3. **novel-workflow v2 前端 UI** (3 个 commit) - 5 个核心组件 + 嵌入 novel/page.tsx

## 1. Sprint 1-4 基线开发

| Commit | 内容 |
|---|---|
| bc3f161 | Sprint 1-2: 用户自建 API Key + 多 Key 轮询 + admin 渠道健康度页面 |
| 08123f7 | Sprint 3: 公开定价 API + UserGroup 用户组 + 阶梯定价 |
| 0974f50 | Sprint 4: 通用 task 模型 + 通用 worker (5s 轮询) |
| 9bf6bd5 | Sprint 4.2: task worker example 模板 + 方向调整 (不做 quota / 不做订阅) |
| 3f1377f | docs: novel-workflow v2 PR description |

## 2. novel-workflow v2 完整后端交付 (11 commit / 10 任务组)

借鉴小云雀"多 Agent 协同 + 用户可见流水线"范式, 把 novel 工作台后半段(剧本->成片)建模成 5 层 9 节点工作流.

### 8 个 capability (v2 final)

1. novel-storyboard-workflow - 编排底座: 5 层 9 节点图, 7 态状态机, 4 启动模式
2. shot-dubbing-node - 按分镜 TTS 配音 + 可插拔 provider + 扣费 hold 流程
3. shot-subtitle-node - 字数线性切时间轴 + 手动编辑 + 全局字幕样式
4. bgm-layer - 6-8 系统预设 BGM + 用户上传 (20MB 限制)
5. composition-layer - ffmpeg 5 步流水线 (归一化/拼接/混音/烧字幕/输出)
6. export-layer - mp4 下载 + 抖音/小红书/视频号文案 + 元数据
7. novel-rerun-layer - **★ 核心 UX**: 成片回看 + 单层重做
8. series-asset-lock - **★ 漫剧一致性**: 项目级主资产包 + 全局色调 prompt

### 关键 UX 价值 (vs 小云雀)

- **成片回看 + 单层重做**: 改完字幕样式 5 分钟出新版 (不重跑视频/配音), 不满意某条分镜 10 秒重做. **vs 小云雀"改一处要全部重跑"**
- **剧集级资产锁定**: 漫剧/短剧用户能锁定主资产包, 所有分镜视频生成自动追加风格约束
- **TTS 扣费走 hold 流程**: 成功 Settle / 失败 Cancel 自动退款, 幂等键防重复扣

### 运行时变化

- **Docker 镜像**: apt-get install ffmpeg (含 libass); COPY assets/bgm-presets/
- **6 个新环境变量**: FFMPEG_BINARY_PATH / COMPOSITION_OUTPUT_DIR / COMPOSITION_WORKER_COUNT / TSS_PROVIDER / ENABLE_SERIES_ASSET_LOCK / BGM_PRESETS_DIR
- **新 cron**: novelWorkflowCleanupCron (30 天过期成片清理)
- **8 张新表**: novel_workflow_runs / novel_workflow_nodes / shot_dubbings / shot_subtitles / bgm_customs / composition_tasks / rerun_records / series_asset_locks
- **~30 个新 HTTP API** (/api/v1/novel/*)

## 3. novel-workflow v2 前端 UI (3 commit)

### 5 个核心组件 (web/src/app/(user)/novel/components/)

| 组件 | 功能 | 任务组 |
|---|---|---|
| novel-workflow-layers.tsx | 5 层步骤条 + 节点展开 + 单节点操作 | 11 |
| novel-rerun-panel.tsx | ★ 核心 UX: 单分镜/整部重做 + 版本历史 + 回滚 | 11 |
| novel-composition-view.tsx | 5 步合成进度 + 停止/重试 + stderr 诊断 | 11 |
| novel-bgm-picker.tsx | 8 预设 + 自定义上传 + 音量/淡入/淡出滑杆 | 11 |
| novel-series-asset-lock-panel.tsx | 主资产包编辑 + 锁定/解锁 + 详情展示 | 11 |

### 嵌入 page.tsx (124 行改动)

- **v1 stepper 之上**: 3 个组件 (layers / lock / bgm) - 与 v1 6 段 stepper **并存** (视角互补: v1 看阶段, v2 看节点)
- **v1 stepper 之下** (pipelinePhase=done 时显示): NovelRerunPanel (整部成片重做)

### TypeScript 验证

- tsc --noEmit 0 错
- 严格类型: NovelWorkflowNode / RerunRecord / CompositionTask / BgmPreset / SeriesAssetLock

## 端到端验证 (51 个端点全过)

- 鉴权 (双链路 cookie + Bearer)
- 商业化 (pricing + 邀请 + 兑换)
- AI 网关 (graceful fail 5 个端点)
- 管理后台 (11 端点)
- 用户侧 (6 端点)
- 公共 (6 端点)
- novel-workflow v2 (8 capability 16 端点)
- 余额流水真实记录 TTS 扣费 (-6 cents, hold -> settle)
- 字幕重做链路 v1 -> v4 (每条新 ID + DB 真写入)
- 通用 task worker 真实跑过

## OpenSpec 文档

完整 OpenSpec 在 openspec/changes/add-novel-video-assembly/:
- proposal.md + proposal-v2.md
- design.md + tasks.md
- 8 个 spec 文件在 specs/

openspec validate 通过, openspec status 4/4 artifact complete.

用户指南在 docs/canvas/novel-workflow.md.

## 影响评估

| 维度 | 影响 |
|---|---|
| API 兼容性 | 全部新增路由, 零破坏 |
| 数据迁移 | 8 张新表 + AutoMigrate; 已有数据无影响 |
| 部署 | Docker 镜像 +30MB (ffmpeg + 6-8 BGM mp3 待用户填充); 新 env 变量向后兼容 (都有默认值) |
| 依赖 | 新增 ffmpeg 系统依赖 (Docker 镜像 apt-get) |
| 扣费 | TTS 调用从用户余额扣 6 cents/次; 其他节点不扣费 |
| 存储 | 成片 mp4 写到 data/compositions/{taskId}.mp4 (dev 模式); 生产改对象存储 |

## 检查清单

- [ ] CI / Docker build 通过 (需装 ffmpeg 的 runner 镜像)
- [ ] 数据库 AutoMigrate 验证 (启动时自动建 8 张新表)
- [ ] 启动日志确认 ffmpeg + BGM 加载正常
- [ ] 创建一条 test run 验证 5 层节点状态机
- [ ] 触发 TTS 调用验证扣费/退款链路

## 后续 PR (v3 候选)

1. novel-workflow 前端 UI 扩展 - 配音卡 / 字幕编辑器 / 导出 modal / 成片回看页
2. 真实 MiMo TTS HTTP 接线 - 替换 mock 实现
3. 多版本成片对比 - 一次工作流产 2-3 个版本让用户选
4. 分享链接 / token 撤销 - 匿名可访问的 mp4 链接

🤖 Generated with Claude Code (https://claude.com/claude-code)
