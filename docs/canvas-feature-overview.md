# Freedom 画布功能 — 实现介绍

> 目的：让任何人问"画布功能是怎么做的"时，能用 10-15 分钟讲清楚。  
> 范围：基于代码事实（2026-08-28 服务器 snapshot 之后）。  
> 不覆盖：UI 视觉细节、dialog/popover 的具体交互（只列组件名）。

## 一、画布是什么

**画布** = 一个**有向节点图编辑器**，用户在上面拖放图片/视频/音频/文字/配置节点，连线成"工作流"，点击节点触发 AI 生成，结果回到画布上形成新节点。

类比：
- **Miro / FigJam** 的画布操作（平移/缩放/拖拽）
- **ComfyUI** 的节点图（每个节点是一个 AI 操作）
- **Stable Diffusion WebUI** 的 img2img 链（节点 = 操作，连线 = 数据流）

**核心特色**：
- "小说→分镜→视频" 全自动 Agent 流水线（10 阶段状态机）
- 用户既可手动拼节点，也能"一句话让 Agent 全自动跑完"

---

## 二、画布的层级结构

```
/canvas                   画布库（项目列表）
  └─ /canvas/[id]         某个画布（节点图编辑器）
       ├─ 节点层          Image / Video / Audio / Text / Config / Director / Group / Panorama
       ├─ 连接层          from-to 边
       ├─ 视口层          平移、缩放、网格
       ├─ 侧边栏          节点配置面板
       ├─ AI 助手面板      对话 + Agent 控制
       └─ 工具栏          撤销/重做、新建、保存
```

---

## 三、画布引擎（自研，无 Konva/React Flow）

文件：`web/src/app/(user)/canvas/components/infinite-canvas.tsx`

**世界坐标**：
- 节点用绝对坐标 `position: {x, y}`，不靠 DOM 层级布局
- **视口**用 3 元组 `{x, y, k}`：x/y 是视口原点平移，k 是缩放
- 屏幕坐标 = 世界坐标 × k + 视口平移 (`transform: translate(x, y) scale(k)`)

**交互**：
- **缩放**：滚轮（`Math.pow(1.1, deltaY/100)`，k 限制 0.05~5）
- **平移**：空格+拖拽 / 中键 / Ctrl+拖拽
- **双击空白**：新建节点
- **拖入**：从侧边栏拖资产到画布（HTML5 drag-drop）

**性能**：
- pan 状态用 `useRef`（不触发 re-render）
- 视口变化走 `requestAnimationFrame` 节流

**为什么不用 Konva/React Flow**：
- 节点数少（典型 10-50），简单自研更灵活
- 不需要 WebGL 性能
- 跟 React 状态系统无缝集成（节点就是普通组件）

---

## 四、节点系统

**8 种节点** (`types.ts:12-21`)：

| 类型 | 作用 | AI 触发 |
|---|---|---|
| `image` | 图片生成结果 | 后端 `/api/v1/images/generations` |
| `video` | 视频生成结果 | 后端 `/api/v1/videos` |
| `audio` | 音频/TTS 结果 | 后端 `/api/v1/audio/speech` |
| `text` | 文字/提示词 | 不调 AI（用户输入） |
| `config` | 配置（喂给其他节点用） | 不调 AI（共享参数） |
| `director` | 导演/分镜编排 | Agent 驱动 |
| `group` | 视觉分组 | 纯前端 |
| `panorama` | 全景图 | 后端图片生成特殊参数 |

**节点 metadata**（`types.ts:35-100`）：~60 个字段，覆盖 `model / size / count / seconds / negativePrompt / generateAudio / references[] / cameraControl / progress / imageTaskId / batchRootId` 等。

**连线**（`types.ts:125-129`）：
```ts
type CanvasConnection = { id; fromNodeId; toNodeId }
```
- **简化设计**：没有"端口"概念，连接只表示"依赖关系"
- 上游节点的输出会作为下游节点的输入（如图片节点 → 视频节点的 first_frame 参考）

**节点生成**（`canvas-node-generation.ts`，293 行）：
- 收集 `references`（上游节点的输出）
- 拼 OpenAI 兼容的 prompt/messages
- 调后端 API，拿到 task ID
- 轮询状态（图片每 1.5s 查，3 分钟超时；视频异步等回调）

---

## 五、AI 助手 + Agent（最复杂的部分）

这是画布**和别家最不一样**的地方。

### 5.1 AI 助手（简单对话）

- 侧边栏一个聊天窗口（`canvas-assistant-panel.tsx`，833 行）
- 用户问"帮我做个 3 分钟的玄幻短片"
- LLM 调工具（tool_calls）→ 创建节点、配置参数、触发生成

### 5.2 Agent（全自动流水线）

文件：`agent/canvas-agent-runtime.ts`（331 行）+ 16 个 `agent/skills/*.ts`

**10 阶段状态机** (`types.ts:167-177`)：
```
intake → concept → script → breakdown → references
       → storyboard → video → audio → review → complete
```

每阶段对应一个 skill：
- `skills/script.ts` — 写剧本
- `skills/image-storyboard.ts` — 分镜图片
- `skills/image-character-sheet.ts` — 角色设定图
- `skills/video-single-shot.ts` — 单镜头视频
- `skills/video-multi-shot.ts` — 多镜头拼接
- `skills/audio.ts` — 配音/BGM
- `skills/organize.ts` — 在画布上摆节点、连线
- ... 等

**协议**：用 OpenAI function_calling 协议（`CanvasAgentProtocolMessage`），LLM 选 skill → 执行 → 返回结果 → 下一阶段。

**自动模式** (`types.ts:193`)：
- `off` — 用户完全手动
- `full` — Agent 自主跑到底
- `checkpoint` — 关键阶段问用户（如"剧本确认"再继续）

**Pet 角色** (`agent/pet-characters.ts`)：Agent 有虚拟形象（猫/狗），对话时有动画。

---

## 六、存储 + 同步

**前端**：
- `useCanvasStore` (Zustand + localForage/IndexedDB) — 离线可用
- 单个项目包含：节点 + 连接 + 视口 + AI 对话 + Agent 配置
- 改动走 **400ms 防抖** → 调后端 `saveCanvasProject`

**后端**：
- `canvas_projects` 表（`model/canvas_project.go`）：
  - 主键 `(userId, id)` 复合
  - `projectData` 字段是完整 JSON（节点+连接+对话）
  - `deletedAt` 软删除
  - 复合索引 `(userId, deletedAt, updatedAt)` 给"我的画布列表"用
- `canvas_image_tasks` / `canvas_audio_tasks` 表：
  - `clientTaskId` 唯一索引（幂等键，HTTP 重试不双扣）
  - `(userId, source, sourceId, nodeId)` 复合索引（按节点查任务）

**保存触发**：
- 改节点 → 400ms 防抖 → saveCanvasProject
- 退出/切页面 → `flushCanvasPersistence()` 立即落盘
- 自动 5s cooldown syncWithRemote（拉远端更新）

**删除**：
- 软删除（`deletedAt` 非空）
- `canvas_project_deletion_scheduler.go` 后台定期真删 + 清理关联任务

---

## 七、任务生成链路

**图片**（`canvas-node-generation.ts` + `handler/canvas_task.go`）：

```
节点 hover 工具栏 → "生成"
  ↓
buildNodeGenerationInputs(metadata, references)
  - 收集上游节点输出
  - 拼 prompt（节点 prompt + config 节点 + 摄像机控制）
  ↓
POST /api/v1/canvas/image-tasks
  - 后端: clientTaskId = nanoid() (前端生成)
  - 后端: ConsumeUserBalanceWithHold (扣费)
  - 后端: 调上游 AI
  - 后端: 返回 task_id
  ↓
前端轮询 GET /api/v1/canvas/image-tasks/{id}
  - 1.5s 间隔，3 分钟超时
  - status=success → 把图片存到 storageKey → 节点显示图片
  - status=failed → 节点显示错误 + RefundFailedVideoTask 退余额
```

**视频**：同图片，但走 `createVideoGenerationTask`（预付费：先 settle hold，失败走 RefundFailedVideoTask → CancelBalanceHold 退款）

**音频**：`createCanvasAudioTask`，逻辑同图片

---

## 八、扣费（跟画布绑定的关键链路）

`canvas-client-page.tsx` 在用户点"生成"时**不直接扣费**——它只调后端。后端在 `service/canvas_image_task.go` 里统一扣费：

```
ConsumeUserBalanceWithHold(
  userID, modelName, cents, "canvas/image-tasks", clientTaskID
)
```

- `cents` = `ModelCost[model].CostCents × count`（per_second 模式则乘秒数）
- `clientTaskID` 做幂等键（网络重试不双扣）
- 业务成功 → `SettleBalanceHold(holdID)`
- 业务失败 → `CancelBalanceHold(holdID)` 退款 + 写退款流水

**关键修复（2026-08-28）**：之前 `ModelCost` 在价格表里找不到模型时静默返回 `CostCents:0`，**0 元白嫖漏洞**。已修——返回 error，handler 直接返回 400。

---

## 九、UI 组件清单

按 `components/` 目录顺序（38 个）：

**画布层**：
- `infinite-canvas.tsx` — 画布引擎（自研 transform）
- `canvas-node.tsx` — 节点渲染（878 行，复杂）
- `canvas-connections.tsx` — 连接 SVG 路径绘制
- `canvas-mini-map.tsx` — 缩略图导航
- `canvas-toolbar.tsx` — 顶部工具栏（撤销/重做/新建/保存）
- `canvas-side-panel.tsx` — 左侧节点/资产面板
- `canvas-context-menu.tsx` — 右键菜单
- `canvas-zoom-controls.tsx` — 缩放按钮

**节点交互**：
- `canvas-node-prompt-panel.tsx` — 节点详情/配置面板
- `canvas-config-node-panel.tsx` — 配置节点编辑
- `canvas-config-composer.tsx` — 配置组合
- `canvas-image-settings-popover.tsx` — 图片生成参数弹窗
- `canvas-video-settings-popover.tsx` — 视频参数
- `canvas-audio-settings-popover.tsx` — 音频参数
- `canvas-node-hover-toolbar.tsx` — 节点 hover 浮按钮
- `canvas-node-angle-dialog.tsx` — 旋转
- `canvas-node-crop-dialog.tsx` — 裁剪
- `canvas-node-split-dialog.tsx` — 拆分
- `canvas-node-upscale-dialog.tsx` — 放大
- `canvas-node-mask-edit-dialog.tsx` — 蒙版编辑

**AI 助手 + Agent**：
- `canvas-assistant-panel.tsx` — 主对话面板（833 行）
- `canvas-assistant-composer.tsx` — 对话输入框
- `canvas-director.tsx` — 导演视图
- `canvas-director-node-panel.tsx` — 导演节点面板
- `canvas-agent-character.tsx` — Agent 角色
- `canvas-agent-pet.tsx` — Agent 宠物动画
- `canvas-agent-settings.tsx` — Agent 配置

**全景图**：
- `canvas-panorama-viewer.tsx` — 全景图查看器
- `canvas-camera-control.tsx` — 摄像机控制

**资源/资产**：
- `asset-picker-modal.tsx` — 资产选择弹窗
- `canvas-prompt-library.tsx` — 提示词库
- `canvas-prompt-chip-input.tsx` — 提示词标签输入
- `canvas-resource-mention-textarea.tsx` — @提及资源

**项目列表**：
- `canvas-project-card.tsx` — 画布卡片
- `canvas-delete-projects-dialog.tsx` — 批量删除

---

## 十、关键技术决策

1. **画布引擎自研**：节点少、需 React 集成、不需 WebGL → 自研比 Konva/React Flow 简单
2. **连接无端口**：只表示依赖，不限制数据流方向 → 简化
3. **配置节点**（`config`）作为独立类型：让"一组参数"成为可复用节点
4. **本地优先 + 后端防抖同步**：离线可用、网络重试不双扣（clientTaskID 幂等）
5. **Agent 10 阶段状态机**：每个阶段一个 skill → 易于扩展/调试
6. **预付费视频**：任务创建立刻 settle hold，失败走 RefundFailedVideoTask → 杜绝双退
7. **0 元白嫖堵漏**（2026-08-28）：`ModelCost` 找不到价格时返回 error，handler 拒绝 → 防止 admin 误配免费用

---

## 十一、画布模块文件总览

- 前端：`web/src/app/(user)/canvas/` — 72 个文件
  - `page.tsx` (115 行) — 画布库
  - `[id]/canvas-client-page.tsx` (5176 行) — 单画布主页面 ⚠️ 该拆分
  - `components/` — 38 个
  - `agent/` — 18 个（runtime + 16 skills）
  - `stores/` — 2 个
  - `utils/` — 工具
  - `types.ts` — 类型定义
- 后端：
  - `handler/canvas_project.go` / `handler/canvas_task.go` — HTTP
  - `service/canvas_*.go` — 业务逻辑
  - `model/canvas_*.go` — 数据模型
  - `repository/canvas_*.go` — DB
  - `service/canvas_project_deletion_scheduler.go` — 后台清理

---

## 十二、可能的改进（可选讨论）

- `canvas-client-page.tsx` 5176 行 → 按职责拆（节点/连接/拖拽/历史/Agent UI）
- 节点撤销/重做（`CanvasHistoryEntry` 已定义但未确认是否完整实现）— 工具栏有 Undo/Redo 图标
- 画布协作（多人同时编辑）— 当前没有
- 节点历史版本（节点内可看历史生成）
