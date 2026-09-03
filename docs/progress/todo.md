---
title: TODO
description: 当前项目后续值得处理的事项
---

# TODO

本文档用来记录当前项目后续比较值得处理的事项。

## 方向调整：不做限速 / 不做会员等级 / 只做卡密

2026-08-29 用户明确调整方向：
- ❌ **不做 quota 限速**（"有钱就能一直调用"）
- ❌ **不做会员等级 / group 定价**（Sprint 3 阶梯定价保留 —— 因为已经实现；但不做 monthly quota / 升级 / 续费 / 订阅相关）
- ✅ **只做卡密**（LicenseKey 兑换余额）

Sprint 3 UserGroup 字段保留（已经实现且无破坏性）；但后续 Sprint 5+ 不再往 group 维度加 quota / subscription / 自动续费。

### 卡密现状（2026-08-29 摸代码发现已经全部实现）

摸代码发现 Freedom 的**卡密能力早已完整**（todo.md 顶部"已完成：支付系统重构"段落里有提）：

- **数据模型**：`model/license_key.go` 有 `LicenseKey`（id / key / faceValueCents / status / usedBy / usedAt / batchName / createdBy）+ `LicenseRedeemLog`（id / licenseKeyId / keyMasked / userId / userName / faceValueCents / createdAt）
- **admin 接口**（`handler/admin_license_key.go`）：`POST /api/admin/license-keys/import`（批量导入）、`GET /api/admin/license-keys`（列表）、`POST /api/admin/license-keys/generate`（自动生成 + TXT 下载）
- **用户接口**（`router/router.go:53/153/154`）：`GET /api/license/purchase-config`（公开）、`POST /api/v1/license/redeem`（兑换）、`GET /api/v1/license/redeem-logs`（我的兑换记录）
- **service**：`service/license_key.go` 有 `ImportLicenseKeys` / `RedeemLicenseKey` / `MyRedeemLogs` / `AdminListLicenseKeys` 等
- **前端**：`web/src/app/(user)/wallet/page.tsx` 已有"兑换" tab + 卡密兑换表单

**Sprint 5 quota 改造已撤销**（`model/setting.go::PrivateSetting.GroupQuotas` 字段已删；其他 quota 相关文件未创建）。

**没有可做的新增卡密功能**——除非有具体诉求（比如"扫码支付自动发卡"、"卡密有效期"、"卡密绑定用户"等）。等你给具体诉求再单独开 Sprint。

Sprint 4 落地了通用 Task 模型 + 通用 worker 框架，但没有真正接管任何 task 也没注册任何 handler。Sprint 4.2 用最小代价收尾：

- **新建** `service/task_handler_example.go` —— 完整可复制的 `TaskHandler` 实现模板（image_batch 场景），含详细中文注释（"为什么这么写"）。未来 Sprint 5/6 直接 fork 这个文件。新增 helper `RegisterExampleTaskHandler()` 用于显式 opt-in
- **修改** `main.go` `StartTaskWorker()` 启动时加注释说明："新能力接入 = 调 RegisterTaskHandler"
- **文档**：`docs/backend/backend-database.md` 加 `tasks` 表完整结构 + 通用 worker 状态机设计 + handler 注册指南

**为什么不接现有 video poller 到通用 worker**：
- 现有 `service/video_task.go::StartVideoTaskPoller` 已经是 new-api 形态的成熟 poller（信号量并发限制、panic 兜底、清理过期、进度唤醒）
- 改造业务行为完全不变，风险大收益小
- 留 Sprint 4.3（如果做）再抽象

**Why**：
- 零回归风险
- 给未来 Sprint 5（quota 限速）/ Sprint 6（实时进度推送）提供"接入指南"
- Sprint 4 真正收尾

**How to apply（后续 Sprint 怎么用）**：
- Sprint 5 quota 限速：fork example handler → 在 `Submit` 之前查 user 当月用量 → 超限返 `failure` + `error_message="quota exceeded"`
- Sprint 6 实时进度：fork example handler → 在 `Poll` 时通过 SSE 推 `progress` 变化
- 任何新能力：调 `service.RegisterTaskHandler(typeStr, handlerImpl)` → 创建 task → 通用 worker 自动接管

## 误判澄清：Sprint 4 plan 中"UpDream/NewWow 视频 vendor 缺口"和"canvas_image_task 没接 Sprint 2 selector"已不成立

实施 Sprint 4.1 时实际摸代码发现：

1. **UpDream 视频 vendor 三个方法已完整实现**（`service/vendor_updream.go`）：
   - `SubmitVideo` (line 801)：调 `/api/ai/generate-video/async` 提交任务
   - `GetTaskStatus` (line 704)：调 `/api/ai/task/{id}` 轮询
   - `GenerateVideo` (line 994)：同步等待视频生成
2. **NewWow 视频 vendor 三个方法同样已实现**（`service/vendor_newwow.go:553/630`）：canvas→shot→generate-video 三步流程 + `/agent/story-canvas/batch-query-status` 轮询
3. **canvas_image_task 已走 Sprint 2 selector**：路径是 `handler/canvas_task.go::executeCanvasAIRequest` (line 402) 直接调 `proxyAIRequest`，而 `proxyAIRequest` 在 Sprint 2 改造中已包含 `runRemoteChannelWithRetry` —— **canvas_image_task 自动获得了 retry + failover + cooldown 能力**

P0 报告（`[Script-to-Video Vendor Mismatch]`）里 UpDream/NewWow 视频"TODO"实际**已在 Sprint 1.x 期间补完**（之前没仔细看代码就标 TODO 是 plan 误判）。

**Sprint 4.1 不需要做实际改动**。下面"进行中：Sprint 4 通用 Task 模型（部分完成）"状态不变。

## 进行中：Sprint 4 通用 Task 模型（部分完成）

Sprint 4 是工作量较大的 Sprint（涉及 8 个新文件 + 4 个修改）。本 Sprint 已完成"骨头"部分：

- **数据层**：
  - `model/task.go` 新增：通用 `Task` 模型 + 6 个 type 常量 + 5 个 status 常量
  - `repository/task.go` 新增：5 个 CRUD 函数（Save / GetByID / ListPendingTasks / ListUserTasks / UpdateTaskStatus / IncrementTaskAttempts）
  - AutoMigrate 加 `&model.Task{}`
- **后端 worker**：
  - `service/task_worker.go` 新增：`TaskHandler` 接口（Submit / Poll / Cancel）+ `RegisterTaskHandler` 注册 + `StartTaskWorker` 后台循环 + 状态机
  - main.go 启动期调 `StartTaskWorker`
- **API**：
  - `handler/task.go` 新增：`UserTasks` handler
  - `router/router.go` 加 `GET /api/v1/tasks`

### 留到 Sprint 4.1（后续）
- 补 UpDream / NewWow 视频 vendor 适配器（`SubmitVideo` / `GenerateVideo` / `GetTaskStatus`）—— 嗅探契约缺失，先返 `ErrTaskNotSupported` fallback 官方
- canvas_image_task 改用 Sprint 2 selector + retry（当前是单次 select）
- video_task poller 改用通用 worker 框架
- 注册 videoHandler 让通用 worker 真正工作

### 留到 Sprint 4.2（更后续）
- 4 套旧 task 表（video_tasks / canvas_image_tasks / canvas_audio_tasks / storyboard_tasks）数据迁移到通用 tasks 表
- 前端 type 同步升级（业务级破坏；需要前端代码层面配合）
- admin 通用 task 查询（`/api/admin/tasks`）

## 已完成：Sprint 3 UserGroup 阶梯定价

把 Freedom 从"一刀切"定价升级为 new-api 形态的"用户组 + 倍率"商业化基础。

- **数据层**：
  - 新表 `user_groups`（`model/user_group.go`）：内置 4 个 group（default / plus / pro / enterprise），AutoMigrate 自动建表
  - `model/user.go::User` 加 `GroupID string`（db column + AuthUser DTO）
  - `model/setting.go::ModelCost` 加 `GroupPricingJSON`（per-model per-group 倍率覆盖）+ `GetGroupPricingRatio` helper
  - `model/setting.go::PrivateSetting` 加 `GroupRatios map[string]float64`（group 维度统一倍率）
- **后端业务**：
  - `service/user_group.go` 新增：`SeedDefaultUserGroups`（启动期 seed）+ `ListActiveUserGroups`
  - `service/pricing.go` 新增：`GetGroupRatio(groupID)` + `CalcUnitCostCents(model, groupID)`（计费公式：base * groupRatio * modelGroupRatio，向下取整）+ `ListPublicPricing`（公开 pricing 页数据）
  - `handler/pricing.go` 新增：`GetPricing`（公开接口，不需登录）
  - `service/auth.go` 新用户默认 `GroupID = "default"`；admin 改用户时支持透传
  - `handler/auth.go::saveUserRequest` 加 `GroupID` 字段
  - `main.go` 启动期调 `SeedDefaultUserGroups`
  - `router/router.go` 加 `GET /api/pricing`
  - 4 个 handler 计费点（`handler/ai.go` / `handler/video_task.go` / `service/workflow_agent.go`）改用 `CalcUnitCostCents` 算 per-unit cost
- **前端**：
  - `services/api/auth.ts::AuthUser` 加 `groupId`
  - `services/api/admin.ts::AdminUser` 加 `groupId`
  - `services/api/pricing.ts` 新增：`fetchPricing` / `listActiveUserGroups` + 4 个 TypeScript 类型
  - `admin/users/page.tsx` 编辑表单加 `groupId` Select（4 个内置 group）
  - `user/wallet/page.tsx` 加第 5 个 tab "价目表"（`DollarOutlined` 图标）
  - `user/wallet/components/pricing-table.tsx` 新增：价目表组件（group 列高亮 + 折扣 Tag 着色）
- **兼容性**：
  - `user.GroupID = ""` 兼容（空 → 走 default，倍率 1.0）
  - 老配置无 `GroupRatios` 字段时 `GetGroupRatio` 返回 1.0（安全默认）
  - 老配置无 `ModelCost.GroupPricingJSON` 时 `GetGroupPricingRatio` 返回 1.0
  - 现有 vendor 路径（`dispatchVendorProxy`）**不**走 group pricing
  - `ConsumeUserBalanceWithHold` 函数签名**不**变（cents 外面算好传进来）

### 后续可优化（Sprint 3.5 候选）
- admin 完整 UserGroup CRUD UI（当前只 hardcode 4 个内置 group）
- 余额通知（"余额 < 阈值"推送）
- 卡密兑换升级 PLUS（admin 创建卡密时指定 group）
- "升 PLUS"按钮（用户自助购买，扫码支付 + 自动升级 group）
- 用户配额 / 月卡（按 group 限制每月生成次数）
- admin groupRatios 编辑 UI（当前 admin 改 settings.private JSON）

## 已完成：Sprint 2.6 admin 渠道健康度页面

把 Sprint 2 落地的"渠道失败诊断"能力可视化到 admin 页面，让 admin 一眼看到哪些 channel 在抽风 / 哪些在冷却 / 影响哪些模型。

- **后端**：
  - `service/channel_selector.go::LoadAllCooldownsSnapshot` 新增（cooldownMap 快照，过期项顺手清理）
  - `handler/admin_channels_health.go` 新增：`AdminChannelsHealth`（汇总接口）+ `AdminClearCooldowns`（清空冷却接口）
  - 响应结构：`summary` (4 个 KPI) + `channels` (按 failureCount 倒序) + `recentFailures` (最近 100 条) + `now`
  - 路由：`GET /api/admin/channels-health` + `POST /api/admin/channels-health/clear-cooldowns`
- **前端**：
  - `web/src/services/api/admin.ts` 加 `fetchChannelsHealth` / `clearChannelCooldowns` + 4 个 TypeScript 类型
  - `web/src/app/(admin)/admin/layout.tsx` 菜单加 "渠道健康"（用 `ApiOutlined` 图标，放在"AI 日志"之后）
  - `web/src/app/(admin)/admin/channels-health/page.tsx` 新建：4 个 KPI 卡片 + 渠道统计表 + 最近失败表 + [清空冷却] 危险按钮（Popconfirm 二次确认）
- **页面布局**：antd `Row/Col/Statistic/Card/Table/Tag` 组合；status code Tag 着色（0=灰，429=橙，5xx=红，4xx=火山红）；冷却 Tag 着色（绿=正常，橙=冷却中）
- **数据流**：单次 RTT 拿所有页面数据（200KB 内），刷新按钮手动触发（**不做** 5s 自动轮询，避免分散注意力）

### 后续可优化
- 按 model / capability 过滤（Sprint 4 视频 vendor 接入后）
- 5s 自动轮询（如果 admin 真有"实时监控"需求再加）
- channel 健康度趋势图（用 ai_log 表做时序数据，跨 Sprint）
- 失败日志持久化（admin 报表场景，跨 Sprint）

## 已完成：Sprint 2.5 admin 渠道管理 UI 升级

把 Sprint 2 落地的"多 key / 优先级 / 状态码 failover / cooldown"配置能力暴露给 admin 抽屉表单，并修复一个安全漏洞。

- **安全修复（最优先）**：`service/settings.go::hidePrivateAPIKeys` 同步清空 Sprint 2 新加的 `Keys []string` 字段。改前 admin 调 `GET /api/admin/settings` 拿到所有多 key 明文，改后 keys 字段返回 `null`，完整 keys 必须经过 `mergeChannelApiKeys` 的"传回原值"流程
- **前端 type**：`web/src/services/api/admin.ts::AdminModelChannel` 加 6 个新字段（`priority / statusCodeMapping / cooldownSeconds / keys / group / capability`）；`AdminPublicModelChannelInfo` 同步加 `priority / keyCount / 等`
- **抽屉 Drawer**：在 line 1213 后插入「高级选项」`Collapse`（默认折叠），包含 Priority / CooldownSeconds / StatusCodeMapping / Capability Select / Group Input
- **多 Key 录入**：用 `Input.TextArea` + antd `getValueFromEvent` / `getValueProps` 把字符串 ↔ 字符串数组转换；placeholder 一行一个 key；提示"留空则使用上方 API Key"
- **列表 Table**：新增"优先级"列（Tag 着色：默认/高优/低优）和"Keys"列（>1 显示蓝色 Tag）；`channelTableData` memo 里从 `keys.length + (apiKey ? 1 : 0)` 计算 `keyCount`
- **数据兼容**：`normalizeChannel` 加新字段兜底默认值；`mergeChannelApiKeys` 扩展沿用 saved 的 keys（防"编辑时 keys 显示空 → 保存后清空"）

### 后续可优化
- 渠道别名 `modelLabels` 编辑 UI（已有后端支持，UI 复杂留 Sprint 2.6）
- 全局渠道健康度页面（已有 `/api/admin/channel-fail-logs` 接口，可视化留 Sprint 2.6）
- rate limit UI（new-api 完整 ratelimit 配置入口，等 Sprint 3 一起评估）
- 渠道批量启停 / 批量改 tag（admin 渠道数 < 30，单条操作够用；如真要再加）

## 已完成：Sprint 2 渠道选择器（多 key + 优先级 + 状态码 failover）

解决之前 P0 报告（[Script-to-Video Vendor Mismatch]）的根因：上游渠道失败时**没有自动切下一家**的能力。Sprint 2 把 Freedom 的渠道调度从"按 Weight 随机"升级成 new-api 形态的"多 key + 优先级桶 + 状态码 failover + 冷却熔断"。

- **数据**：`ModelChannel`（`model/setting.go`）扩 6 个新字段：`Priority` / `StatusCodeMapping` / `CooldownSeconds` / `Keys []string`（多 key 轮询） / `Group`（Sprint 3 预留） / `Capability`（text/image/video/audio）；老配置 JSON 反序列化时全部为默认值，**行为完全兼容**
- **能力索引**：`model/ability.go` 内存倒排索引 `(group, model, capability) → []ChannelRef`；启动期 `service.BuildAbilityCache()` 重建，`service.SaveSettings` 改 channels 后**异步重建**（admin 改配置立即生效，无需重启）
- **渠道选择器**：`service/channel_selector.go::PickChannelWithRetry` — 按 Priority 升序分桶、桶内 Weight 随机；自动排除 cooldown 中 / 已 retry 过的 channel
- **cooldown 熔断**：`service/channel_selector.go::MarkChannelFail` — 状态码命中 `StatusCodeMapping` 时进 cooldown（默认 60s）；纯内存 `sync.Map`，进程重启清零
- **retry 循环**：`handler/ai.go::runRemoteChannelWithRetry` — 最多 3 次尝试；计费只在 attempt=0 扣（`ConsumeUserBalanceWithHold` 用 requestID 幂等命中）；最终失败统一 `holdCancel`；成功 settle
- **本地渠道分支**：`handler/ai.go::runLocalChannelSingle` — 单次请求，不走 retry（本地重试无意义）
- **诊断字段**：`model/ai_log.go::AICallLog` 加 `AttemptIndex` / `UpstreamStatusCode` / `KeyIndex` / `LastTryAt`；admin `/admin/ai-logs` 一眼看出"卡在第几次 retry"
- **诊断日志**：`service/channel_fail_log.go` 内存 ring buffer（最近 1000 条）+ `GET /api/admin/channel-fail-logs?limit=100` 接口
- **路由**：`router/router.go` 加 `GET /api/admin/channel-fail-logs`
- **vendor 不受影响**：`handler/vendor_proxy.go` 与本选择器**平行**；vendor 失败仍走官方 selector（沿用现有 `dispatchVendorProxy` → fallback 路径）

### 后续可优化
- admin 渠道管理 UI（Sprint 2.5）支持直接编辑 priority / statusCodeMapping / 多 key 录入
- rate limit（new-api 有完整 `model-rate-limit.go`，等 Sprint 2.5 一起做）
- 按 user group 灰度（Sprint 3 引入 UserGroup 后启用 `ModelChannel.Group` 字段）
- cooldown 持久化到 MySQL（当前进程重启清零，符合"重启=恢复"预期；如需持久化再迁）

## 已完成：Sprint 1.5 API Key 管理前端页面

把 Sprint 1.1 落地的 `user_tokens` 后端能力暴露成前端用户能直接用的 UI。

- `web/src/services/api/user_token.ts` —— 5 个 API client（list/create/delete/disable/enable）+ 4 个 TypeScript 类型
- `web/src/app/(user)/wallet/components/api-key-manager.tsx` —— 主组件：列表 + 启停 + 删除 + 创建入口
- `web/src/app/(user)/wallet/components/api-key-create-modal.tsx` —— 创建表单（名称 + 高级：过期时间 / 独立额度 / 不限制）
- `web/src/app/(user)/wallet/components/api-key-reveal-modal.tsx` —— **关键**：明文一次展示弹窗，「我已保存」checkbox 未勾时关闭按钮 disabled，防误关
- `web/src/app/(user)/wallet/page.tsx` —— Tabs.items 末尾追加第 4 个 tab「API Key」

**交互关键点**：
- 创建流程分两步：表单弹窗 → 明文弹窗；raw 仅在第二步展示，关后无法再看
- KeyPrefix 在列表里脱敏为 `sk-fk-...xxxx`，可点击复制
- 用量列区分三种：无限 / 独立额度上限 / 用账户余额
- 启停/删除走 antd Popconfirm 二次确认
- 状态 Tag 颜色：active=绿 / disabled=灰 / exhausted=橙 / expired=红

### 后续可优化
- modelLimits / allowIps 暂未在 UI 暴露配置入口（功能后端已支持，留 Sprint 2 渠道管理一起做）
- 当前不做"创建后立即测试"按钮，避免弹窗堆叠
- 暂不分页（单用户 token 数 < 10 是常态）
- 后续可加"按 group 过滤 token"列（Sprint 3 引入 UserGroup 时再做）

## 已完成：Sprint 1.1 用户自建 API Key（OpenAI 兼容 sk- 鉴权）

把 Freedom 从"画布工具"扩展为"画布 + AI 网关"，让外部 SDK（OpenAI Python/Node SDK、Cursor、Cline、curl）能直接 `Authorization: Bearer sk-fk-xxx` 调 `/v1/chat/completions` `/v1/images/generations` 等端点，扣用户余额。

- **数据**：`user_tokens` 表（model/ai_log.go 同时新增 `token_id` 列）；明文 `sk-fk-` 前缀 + 32 字节随机 base64url（43 字符），存库只存 SHA-256 hash，uniqueIndex 防碰撞。
- **鉴权**：`middleware/admin.go::authUser` 加 `Bearer sk-` 前置分支（不破坏 cookie / JWT 流程），通过 `service.CurrentAuthUserByTokenFull` 校验 status / expired_at / allow_ips（单 IP + CIDR）后同时把 token 注入 ctx（`WithUserToken` / `UserTokenFromContext`）。
- **计费**：`service/auth.go::ConsumeUserBalanceWithHold` 末尾加 `tokenID ...string` 可变参数（不破坏现有 3 个调用点）；`BalanceLog.Extra` 写 tokenId，`AICallLogInput.TokenID` 同步写入。
- **管理**：`/api/v1/user-tokens` 五条路由（POST 创建、GET 列表、DELETE 删除、POST :id/disable、POST :id/enable），全部挂在现有 `middleware.UserAuth` 下，鉴权透明。
- **路由**：现有 `/api/v1/*` 全部沿用 `middleware.UserAuth` —— authUser 内部完成分发，handler/service 用 `UserFromContext` 行为完全不变。

### 后续可优化
- `user_tokens.model_limits` 当前已落库但暂未在 ai handler 强校验（Sprint 2 渠道选择器里实现，weight/priority 之后）。
- 暂未做 rate limit（new-api 有完整的 `model-rate-limit.go`，等 Sprint 2 一起做）。
- token 续期 / 自动 rotate 等运维能力等用户量起来再做。

## 已完成：分镜生成后端任务化

分镜生成（`handleParseStoryboards`）已从纯前端并发改为后端任务化：

- **后端**：新增 `model/storyboard_task.go`、`repository/storyboard_task.go`、`service/storyboard_task.go`、`handler/storyboard_task.go`，实现任务创建、worker 调度、逐章调文本模型、进度落库。
- **前端**：新增 `services/api/storyboard_task.ts` API 客户端；`novel/page.tsx` 的 `handleParseStoryboards` 改为提交后端任务 + 轮询恢复进度，刷新/重开页面后自动恢复轮询；未登录或后端不可用时回退到原前端直连模式（`handleParseStoryboardsLocal`）。
- **路由**：`POST/GET/DELETE /api/v1/storyboard-tasks` 已注册；`main.go` 启动 worker。

### 后续可优化
- 后端 executor 当前逐章串行调用文本模型；如需更快，可改为 worker pool 并发（需加 mutex 保护 onProgress 与 results）。
- `regenerateStoryboard`（单条重新生成）仍走前端直连，后续也可任务化。
- 大剧本分块改写、资产批量生成等长流程可复用本任务化模式。

## 已完成：支付系统重构（路线 2：彻底拆掉积分概念）

原来的"算力点 / 积分"体系被推翻，整套按人民币元（¥）显示。

- **底层单位**：从"1 积分 = 1 cent"改为"1 元 = 100 cents"，所有金额字段统一存整数 cents；后端渲染层无 ×100 隐式换算。
- **后端字段重命名**：`User.Credits → User.BalanceCents`、`CreditLog → BalanceLog`（type 枚举改为 `manual_adjust / generation_consume / generation_refund / manual_recharge`）、`ModelCost.Credits/PerSecondCredits → CostCents/CostCentsPerSecond`、`LicenseKey.Credits → FaceValueCents`、`LicenseRedeemLog.Credits → FaceValueCents`。
- **后端 service / handler / router**：`Consume/Refund/AdjustUserCredits → Consume/Refund/AdjustUserBalance`；admin 路由 `/api/admin/users/:id/credits → .../balance`、`/api/admin/credit-logs → .../balance-logs`；用户流水路由 `/api/v1/credit-logs → .../balance-logs`。
- **前端文案**：账户页改 `/wallet`、admin 流水页改 `/admin/balance-logs`、顶栏徽标组件 `CreditsBadge → BalanceBadge`（Zap → Coins 图标，文案统一"余额 / ¥X.XX"）；常量 `credits.tsx → balance.tsx` 提供 `formatBalanceYuan(cents)` 等辅助函数。
- **卡密体系**：保留 admin 端的"批量生成卡密 + 整批修改面额"作为 manual补发通道，但**移除用户侧兑换入口**（`POST /api/v1/license/redeem` 已下线），新增 type `manual_recharge` 用于 admin 直接补发场景。
- **供应商账本分离不变**：用户激活 UpDream / NewWow / LibTV 时仍走供应商自己计费，不扣本系统余额；UI 显示文案改"供应商账户余额"。

### 后续可优化
- 卡密体系可以等需要"在线扫码支付"时整段替换为真正的订单/支付通道（详见 pending-test 待验证项）。
- 现在模型扣费是单一固定单价（cents / 次 或 cents / 秒），后续可支持阶梯定价。

## 已完成：LibTV 视频供应商分发

- `handler/video_task.go` 的 `proxyAIVideoTaskRequest` 已加供应商分发分支：用户激活了支持异步视频的供应商（LibTV 实现 `service.VendorVideoSubmitter` 可选接口的 `SubmitVideo`）→ 提交拿 `generateUuid` 创建 `VideoTask`（新增 `vendor_type` 列标记供应商任务，不走官方渠道/积分）。
- 轮询阶段 `pollVideoTaskFromUpstream` 识别 `vendor_type` 非空 → 改调 `adapter.GetTaskStatus` 更新状态。
- 同步轮询的 `GenerateVideo` 保留（复用 `SubmitVideo` 提交），按模型模板 UUID / 名称判定文生/图生，图生视频必须带首帧。
- 契约仍未真机验证（提交端点、`mode`/`images` 参数、`data.videos[].videoUrl`），见 pending-test 待验证项。

## 待接：UpDream / NewWow 视频生成

- UpDream / NewWow 视频生成依赖官网真实抓包样本，暂无契约；当前仅支持图片生图（样本重放），点视频生成会提示"暂仅支持图片生图"。

## 已完成：模型下拉/渠道解析按"供应商 vs 官方"规范收敛（待测试）

- 抽全局单一事实来源 `selectableModelChannels(config)`：供应商模式只显示供应商虚拟渠道，官方模式只显示云端 + 真实添加的本地渠道（云端模式不兜底空"自定义渠道"）。
- 修改 `use-config-store.ts`（新增函数 + `normalizeLocalChannels` 加 `allowFallback`）、`model-picker.tsx`、`image/page.tsx`、`video/page.tsx`、`creative-workflow-workspace.tsx`。
- 可测试变更详见 `docs/progress/pending-test.md`。
