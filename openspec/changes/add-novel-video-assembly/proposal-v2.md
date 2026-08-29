# Proposal v2 — novel-workflow（精简增量版）

> v1 proposal 在 `proposal.md`（初次提交）。**v2 只列变化点**，原 spec / design / tasks 不动。v2 落地后会更新对应章节。

## 为什么要改 v2

重新对比小云雀「一键成片」和我之前的提案，发现三处关键差距：

1. **小云雀是"对话 + Agent 驱动"**，我做成了"传统 DAG（Airflow 风格）"——范式不对
2. **缺"成片回看 + 单层重做" UX**——用户跑出成片后想改字幕样式，应该只重跑字幕烧录，不重跑视频
3. **缺"剧集级资产锁定"和"成片版本"概念**——漫剧级一致性和多版本对比是创作者向工具的核心

## 改什么

### 1. 重新组织能力（capability 重排）

| 之前 (v1) | 现在 (v2) | 理由 |
|---|---|---|
| `novel-storyboard-workflow` | **保留**（编排底座不动） | 仍是核心 |
| `shot-dubbing-node` | **保留** | 配音节点不变 |
| `shot-subtitle-node` | **保留** | 字幕节点不变 |
| `final-assembly-node` | **拆成 3 个层** | BGM / 合成 / 导出 是 3 个独立阶段 |
| — | **新增 `novel-rerun-layer`** | 核心 UX 缺漏 |
| — | **新增 `series-asset-lock`** | 漫剧级一致性 |

### 2. 拆 `final-assembly-node` → 3 个 capability

```
final-assembly-node (v1)
  ├── BGM 选曲
  ├── ffmpeg 合成
  └── 导出分享

        ↓ 拆成 3 个 layer

bgm-layer (v2 新)
  - 6-8 个系统预设 BGM（古风/都市/紧张/温馨/伤感/史诗/欢快/悬疑）
  - 用户可上传自定义 BGM（不进 admin 库）
  - 试听 / 音量 / 淡入 / 淡出
  - **不引入 admin BGM 库管理**（v1 那个过度工程）

composition-layer (v2 新)
  - ffmpeg 流水线（归一化 → 拼接 → 混音 → 烧字幕 → 输出）
  - 字幕样式（字体/颜色/描边/位置）
  - 5 步进度展示
  - 状态机：未启动 / 排队中 / 进行中 / 成功 / 失败 / 已取消

export-layer (v2 新)
  - 下载 mp4
  - 复制平台文案（抖音 / 小红书 / 视频号）
  - **v2 暂不做分享链接**（token / 撤销）—— 放 v3
```

### 3. 新增 `novel-rerun-layer`（**核心 UX 缺漏**）

**为什么这是最关键的补漏**：

小云雀跑出成片后，用户不满意某处，能直接**只改一处、重跑一处**。我之前的 proposal 只在 spec 里写了 requirement，**没把它做成一等公民**。

**新增 capability：**

- **新增"成片预览页"**：合成成功后，novel 工作台切换到"成片视图"——左边播放器，右边分镜列表，点击某个分镜显示该分镜的视频片段 + 当前配音 + 当前字幕 + 当前 BGM
- **新增"层重做"按钮**：每个分镜旁边有 4 个按钮「重做视频 / 重做配音 / 重做字幕 / 重做风格」—— 点哪个只重跑该层、不重跑其他层
- **整部成片级别"重做字幕烧录"**：成片页顶部一个「重烧字幕」按钮—— 改完全局字幕样式后点一下，**只重跑 ffmpeg 烧字幕步骤**（5 分钟内出新版），视频片段和配音不动
- **状态保留**：每层有独立版本号（如"字幕 v1 / v2 / v3"），用户可对比选一个

### 4. 新增 `series-asset-lock`（**漫剧级一致性**）

**为什么重要**：

Freedom 是**创作者向**工具，目标用户包括"做漫剧 / 短剧"的人。一部漫剧 20+ 集、每集 30+ 镜头，**所有镜头必须看起来是同一个世界**——同一套角色、同一套场景、同一套色调。

**新增 capability：**

- **新增"剧集级资产锁定"**：在 novel 项目级别（不是分镜级别）选定一组资产作为"主资产包"—— 包含 1-N 个角色、1-N 个场景、1-N 个道具、1 套全局色调 prompt
- **视频生成时强制使用**：所有分镜视频生成必须从"主资产包"里取参考图，**不能临时换资产**
- **跨分镜一致性约束**：相邻分镜之间（如同一场景的两个镜头）必须保持色调、镜头语言、角色朝向一致—— 通过 shared style reference 实现
- **持久化**：主资产包存到 novel 项目级别，跨设备同步
- **UI**：novel 工作台顶部新增"主资产包"配置区，可上传 / 拖入资产、设置主色调 prompt

### 5. 加"成片版本"（version）概念

**为什么**：

小云雀的"拍同款"会**同时产出多个相似风格的结果**让用户挑。Freedom 也应该支持——一次工作流产 2-3 个版本的成片（不同 BGM / 不同字幕样式 / 不同风格预设），用户对比选一个。

**新增**：

- **工作流支持多版本输出**：合成节点完成后产出 N 个版本（默认 N=2，可配），每个版本是一组完整参数（视频/配音/字幕/BGM）合成出的 mp4
- **版本对比页**：成片视图新增"版本对比" tab，2-3 个播放器并排显示，用户点"采用此版本"才保存到主输出
- **存储**：每个版本独立存对象存储，主输出是用户选中的那个

### 6. 改"一键出片"为"快速 / 自定义"双模式

**为什么**：

v1 把"一键出片"硬塞为"按默认参数一次跑完"。**实际场景**：用户 80% 的时候会想先调 BGM 风格、字幕样式、目标时长，再启动——纯"一键"对创作者不友好。

**修改**：

- **快速模式**（v1 现有的"一键出片"按钮）：用默认参数，一次跑完
- **自定义模式**（新）：点开"配置"按钮，先调参数（背景音乐风格 / 字幕样式 / 目标时长 / 是否烧字幕 / 配音音色），保存后再启动
- **UI**：novel 工作台顶部两个按钮「快速出片」「自定义出片」

### 7. 砍 v1 的过度工程

| 砍掉 | 理由 |
|---|---|
| **admin BGM 库管理** | 改为 6-8 个系统预设 + 用户上传，admin 不用管 |
| **分享链接 token / 撤销** | v2 只做"下载 mp4 + 复制平台文案"，分享链接 v3 再做 |
| **"global novel workflow graph UI"（9 节点步骤条）** | 改为"层视图"——5 个层横向排列，层内节点折叠，**更紧凑** |
| **"自动 / 手动模式"切换** | 简化为"快速 / 自定义"——前者 = 自动跑完，后者 = 用户先配再跑 |

## 新增 capability（合计 7 个）

| Capability | 状态 | 简述 |
|---|---|---|
| `novel-storyboard-workflow` | 已有 | 工作流编排底座（声明式 7 态状态机）|
| `shot-dubbing-node` | 已有 | 镜头级 TTS 配音 |
| `shot-subtitle-node` | 已有 | 镜头级字幕时间轴 + 编辑 |
| `bgm-layer` | **新增** | 系统预设 BGM + 用户上传（无 admin 库）|
| `composition-layer` | **新增** | ffmpeg 合成（5 步进度）|
| `export-layer` | **新增** | mp4 下载 + 平台文案（v2 无分享）|
| `novel-rerun-layer` | **新增** | 成片回看 + 单层重做（**核心 UX**）|
| `series-asset-lock` | **新增** | 剧集级资产锁定（漫剧一致性）|

**合计 8 个 capability**（v1 是 4 个）。

## 修改 capability

无现有 capability 受 spec 级行为变更影响。**保留**：
- 剧本节点（已有）
- 分镜剧本节点（已有）
- 资产节点（已有）
- 镜头视频节点（已有）

## 影响

### 新增后端文件

- `model/bgm_preset.go`、`model/composition_task.go`、`model/export_task.go`
- `model/rerun_record.go`（记录每次"重做"的历史）
- `model/series_asset_lock.go`（剧集级主资产包）
- `model/composition_version.go`（成片多版本）
- `repository/*.go`：对应 CRUD
- `service/bgm_preset.go`（6-8 个系统预设 + 用户上传）
- `service/composition.go`（ffmpeg 合成，单层重做支持）
- `service/rerun.go`（单层重做逻辑——能只跑视频 / 只跑配音 / 只跑字幕烧录）
- `service/series_asset_lock.go`（资产包锁定 + 跨分镜一致性）
- `service/composition_version.go`（多版本输出）
- `handler/bgm_preset.go`、`handler/composition.go`、`handler/rerun.go`、`handler/series_asset_lock.go`、`handler/composition_version.go`
- 现有 `handler/novel_dubbing.go` 等保留

### 新增前端文件

- `web/src/app/(user)/novel/components/novel-composition-view.tsx`（**成片回看页**：左播放器 / 右分镜列表 / 顶版本切换 tab）
- `web/src/app/(user)/novel/components/novel-rerun-panel.tsx`（**单层重做面板**——每个分镜 4 个按钮 + 整部成片"重烧字幕"按钮）
- `web/src/app/(user)/novel/components/novel-bgm-preset-picker.tsx`（替代 v1 的 BGM 库选择器）
- `web/src/app/(user)/novel/components/novel-series-asset-lock-panel.tsx`（剧集级主资产包配置区）
- `web/src/app/(user)/novel/components/novel-composition-version-compare.tsx`（多版本并排对比）
- 现有 novel-workflow-graph 改为"层视图"组件 `novel-workflow-layers.tsx`

### 删除 / 砍掉

- ~~`web/src/app/(admin)/admin/bgm/page.tsx`~~（v2 不做）
- ~~`web/src/app/(user)/novel/components/novel-export-modal.tsx` 里的分享链接部分~~（v3 再做）
- ~~admin 系统设置里的 `enableNovelWorkflow`、`shareRetentionDays`~~（v2 暂不需要）

### Docker / 依赖

- 基础镜像装 ffmpeg（带 libass）
- 6-8 个系统预设 BGM 文件（mp3）作为**默认资源**打包进 image（约 30MB 增量）

## 关键设计决策（v2 新增）

1. **工作流是"层"不是"节点图"**—— 5 个层（输入 / 剧本 / 资产 / 镜头 / 后期），每层有自己的状态机，层内可折叠
2. **单层重做是一等公民**—— 成片回看页是整个 UX 的核心，配音/字幕/BGM 都可独立重做，不重跑视频
3. **多版本输出默认 N=2**—— 一次工作流产 2 个成片版本让用户选，节省后续"我想要另一种风格"的成本
4. **剧集级资产锁定优先级 > 单分镜资产**—— 当主资产包锁定后，分镜级资产只能从主资产包里选，**不能临时加新角色**
5. **不引入 admin BGM 库**—— BGM 是 C 端娱乐逻辑（模板市场），Freedom 是创作者工具，6-8 个系统预设 + 用户上传就够了
6. **v2 不做分享链接**—— 先把"下载 + 平台文案"做好，**分享链接 + token 撤销**放 v3（避免 v2 范围爆炸）
7. **不引入"对话驱动 Agent"**—— 保留 Freedom 节点级编辑 + 3D 导演台的差异化优势，**不学小云雀的"对话框"**—— 那是 C 端用户向的，Freedom 是创作者向

## v2 vs v1 capability 数对比

```
v1: novel-storyboard-workflow + shot-dubbing-node + shot-subtitle-node + final-assembly-node
    = 4 个

v2: novel-storyboard-workflow + shot-dubbing-node + shot-subtitle-node
    + bgm-layer + composition-layer + export-layer
    + novel-rerun-layer + series-asset-lock
    = 8 个
```

**v2 工作量是 v1 的 ~1.8 倍**（4 个 → 8 个 capability），但**UX 质量显著提升**（多版本对比 + 单层重做 + 剧集级一致性 + 6 个系统预设 BGM）。

## 推荐下一步

1. 确认 v2 proposal 认可 → 我**更新 tasks.md**（按 8 个 capability 重排）
2. 不认可的话告诉我砍掉哪个 / 加哪个
3. 全部认可后 → **开始 apply**（按 tasks.md 顺序实现）
</content>
</invoke>