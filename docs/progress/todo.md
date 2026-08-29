---
title: TODO
description: 当前项目后续值得处理的事项
---

# TODO

本文档用来记录当前项目后续比较值得处理的事项。

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
