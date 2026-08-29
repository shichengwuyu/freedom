## 背景

Freedom novel 工作台已具备前半段链路（剧本 → 分镜剧本 → 资产 → 单条视频），缺后半段（配音 / 字幕 / 合成 / 导出）。本次新增后半段，并把它建模为一条「多层 + 用户可见 + 可单步干预」的工作流（参考小云雀"多 Agent 协同 + 用户可见流水线"范式），保留 Freedom 现有的节点级编辑、3D 导演台、画布协作等差异化能力。

约束：

- C1. 不破坏现有 storyboard_tasks 流水线、画布节点系统、3D 导演台链路
- C2. ffmpeg 装在 Go 服务 Docker 镜像中，合成任务**只在服务端跑**
- C3. 长任务（配音 / 合成）必须遵循现有的 task + 轮询模式
- C4. 所有媒体资产（配音 mp3 / 字幕 srt / BGM 文件 / 最终 mp4）走现有对象存储
- C5. TTS 调用复用现有 BalanceLog 扣费 / 退款
- C6. 工作流是"层"不是"节点图"——5 个层（输入 / 剧本 / 资产 / 镜头 / 后期），每层有自己的状态机
- C7. **单层重做是一等公民**——v2 的核心 UX 补漏
- C8. 6-8 个系统预设 BGM + 用户上传，**不**做 admin BGM 库

## 目标 / 非目标

**目标：**

- 把后半段（配音 / 字幕 / BGM / 合成 / 导出）建模为 7 个新增 capability，挂到 novel 工作流图上
- 工作流节点遵循统一状态机（7 态）和统一操作（开始 / 停止 / 重试 / 单步编辑）
- **单层重做**支持：成片视图 + 单分镜单层按钮 + 整部成片重烧
- TTS 配音支持可插拔提供方
- 字幕时间轴可手动编辑
- ffmpeg 合成任务可轮询、可停止、可重试
- 剧集级资产锁定（漫剧一致性）
- 快速 / 自定义双模式启动

**非目标：**

- 不引入新的角色 / 权限模型
- **不**做分享链接 / token / 撤销（v3 再说）
- **不**做 admin BGM 库管理
- 不接抖音 / 小红书 / 视频号官方开放平台
- 不做 ASR 字幕重对齐
- 不做对话驱动 Agent（小云雀的 C 端范式不适合 Freedom）
- 不做实时协作编辑

## 8 个 capability 拆解

```
novel-workflow 拆分
├── novel-storyboard-workflow  （编排底座：层视图 + 双模式 + 状态机）
├── shot-dubbing-node          （配音）
├── shot-subtitle-node         （字幕）
├── bgm-layer                  （BGM：系统预设 + 用户上传，无 admin 库）
├── composition-layer          （ffmpeg 合成：5 步进度）
├── export-layer               （下载 + 平台文案；无分享链接）
├── novel-rerun-layer          （★ 核心 UX：成片回看 + 单层重做）
└── series-asset-lock          （漫剧级资产锁定）
```

## 关键决策

### D1. 工作流是"层"不是"节点图"

5 个层（输入 / 剧本 / 资产 / 镜头 / 后期），每层有自己的状态机，层内节点折叠隐藏。**比 v1 的 9 节点步骤条更紧凑**。

**为什么**：9 个节点步骤条在小屏幕 / 移动端难以展示；5 层视图更符合"用户认知模型"——前期 / 中期 / 后期。

### D2. 单层重做 = 一等公民

v2 的核心 UX 补漏。成片视图（novel-rerun-layer）支持：
- **单分镜单层重做**：4 个按钮（重做视频 / 配音 / 字幕 / 风格）—— 点哪个只跑哪个
- **整部成片单层重做**：3 个按钮（重烧字幕 / 重新合成 / 重新配音）—— 改完全局样式后只跑受影响步骤
- **每层版本号独立**（v1 / v2 / v3），可回滚

**为什么**：小云雀的"一键出片"最大痛点是"改一处要全部重跑"。v2 用单层重做解决。

### D3. 6-8 个系统预设 BGM，不做 admin 库

BGM 是 C 端娱乐逻辑（拍同款模板市场）。Freedom 是创作者工具，6-8 个预设（古风 / 都市 / 紧张 / 温馨 / 伤感 / 史诗 / 欢快 / 悬疑）足够。

**为什么砍 admin BGM 库**：
- admin 不会运营 BGM 库（不是 C 端产品）
- 库越大越难维护
- 用户上传自己的 BGM 已经够灵活

**实施**：预设 mp3 文件打包进 Docker 镜像（约 30MB 增量）。

### D4. 剧集级资产锁定 = 漫剧一致性

漫剧 / 短剧用户最痛的是"角色变形 / 场景不一致"。v2 引入 series-asset-lock：
- 项目级别锁定一组主资产（角色 + 场景 + 道具 + 全局色调 prompt）
- 视频生成强制从主资产包取参考图
- 跨分镜共享风格 reference

**为什么不默认开启**：自由度降低；只有做漫剧 / 短剧的用户才需要锁定；其他用户继续用 v1 的灵活行为。

### D5. 快速 / 自定义双模式启动

- **快速模式**：用所有节点默认参数，一次跑完（v1 的"一键出片"）
- **自定义模式**：先打开配置弹窗（BGM 风格 / 字幕样式 / 目标时长 / 是否烧字幕 / 配音音色），保存后再启动

**为什么**：80% 用户会想先调参数再启动；纯"一键"对创作者不友好。

### D6. 字幕时间轴按字数线性切分

每个分镜取「对白 / 旁白」文本，按标点切 N 行，按字数比例分配到该分镜视频时长内。

**为什么 v1 算法**：不调 ASR 节省成本和延迟；编辑器兜底修正。

### D7. ffmpeg 在 Docker 镜像中、concat demuxer + filter_complex

基础镜像装 ffmpeg（带 libass）；合成命令模板：

```
ffmpeg \
  -f concat -safe 0 -i concat_list.txt \
  -i dubbing_1.mp3 ... \
  -i bgm.mp3 \
  -filter_complex "[a:dubbing]concat=n=N:v=0:a=1[concat_a];[concat_a][bgm]amix=inputs=2:duration=first:dropout_transition=0[amixed];[0:v]ass=subtitle.ass[v]" \
  -map "[v]" -map "[amixed]" \
  -c:v libx264 -pix_fmt yuv420p -c:a aac \
  output.mp4
```

**为什么**：concat demuxer 处理同 codec 流最简单；filter_complex 覆盖混音 / 烧字幕 / 转场。

### D8. ffmpeg 进度解析用 `-progress pipe:1`

合成 worker 启动 ffmpeg 时传 `-progress pipe:1 -nostats`，解析 `out_time_ms` / `progress=continue|end` 输出，把当前步骤（归一化 / 拼接 / 混音 / 烧字幕 / 输出）写到 `composition_tasks.progress_json`，前端 3 秒轮询。

**单层重做特别重要**：重烧字幕只跑步骤 ④，进度条只高亮 ④。

### D9. 节点失败 = 整节点跳过，不阻塞工作流

- 配音节点某分镜 TTS 失败 → 该分镜用静音合成
- 字幕节点某分镜无字幕 → 该分镜不烧字幕
- BGM 未选 → 不混 BGM
- 镜头视频缺失 → 该分镜跳过并记 warning

合成任务整体仍标为「成功」（前提是 ffmpeg 进程退出码为 0）。

### D10. 分享链接 / token / 撤销 = v3 范围

v2 不做"分享链接"——只做"下载 + 平台文案"。理由：
- 分享链接需要 token + 过期 + 撤销 + 公开路由，全套基础设施
- v2 范围已经够大（8 capability）；加分享链接会变成 9-10 个
- 创作者向工具里，"下载 + 复制文案自己发"已经覆盖 80% 用例

## 风险 / 取舍

- **R1：ffmpeg 二进制不在开发机** → 启动 smoke check（`ffmpeg -version`）
- **R2：libass 未编译进 ffmpeg** → 基础镜像固定 `mwader/static-ffmpeg:7.1`
- **R3：长合成任务让用户觉得卡** → 进度条按 5 个步骤展示 + lastMessage + 停止按钮
- **R4：TTS 烧余额** → 仅成功后扣费，失败退款
- **R5：BGM 预设文件 30MB 镜像增量** → 接受；预设是核心价值
- **R6：剧集级资产锁定的复杂度** → v1 默认关闭，用户主动锁定才生效
- **R7：单层重做的状态机一致性** → 所有重做走 novel-workflow 节点状态机，不绕开
- **R8：不同模型输出 codec 不一致导致 concat 失败** → 合成前归一化

## 部署计划

- **无数据迁移**。本变更只新增表（`novel_workflow_runs` / `novel_workflow_nodes` / `shot_dubbings` / `shot_subtitles` / `bgm_presets` / `bgm_custom` / `composition_tasks` / `composition_versions` / `series_asset_locks` / `rerun_records`）。
- **Auto-migrate** 通过 GORM。
- **Docker 镜像升级**：基础镜像装 ffmpeg（带 libass）+ 6-8 个 BGM 预设文件（约 30MB 增量）。
- **回滚**：删新表，删路由，不影响现有功能。

## 待定问题

- OQ1：单层重做是同步还是异步？**异步**—— 复用现有 task + 轮询模式，不阻塞 UI
- OQ2：系统预设 BGM 的 6-8 首具体选哪些？**交给 v2 实现时定**，先确定架构，曲目选 V1 古风 1 + 都市 1 + 紧张 1 即可
- OQ3：剧集级资产锁定 v1 是否默认开启？**默认关闭**——v1 保持 v1 行为（分镜自由选资产），用户主动锁定才生效
- OQ4：快速 / 自定义模式的 UI 位置？**顶部两个并排按钮**——「快速出片」「自定义出片」
- OQ5：成片多版本对比是必须的吗？v2 不做（v1 单版本），v3 再加 `composition-versions` capability
