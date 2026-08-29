---
title: 待测试
description: 当前版本已实现但仍需人工验证的变更项
---

# 待测试

## Sprint 2.6：admin 渠道健康度页面

把 Sprint 2 渠道失败诊断 ring buffer 可视化为 admin 看板。

### 改动一览
- **后端**：`service/channel_selector.go::LoadAllCooldownsSnapshot`；`handler/admin_channels_health.go` 新增（汇总 + 清空冷却）；路由 `GET /api/admin/channels-health` + `POST /api/admin/channels-health/clear-cooldowns`
- **前端**：`admin.ts` 加 4 个 TS 类型 + 2 个 API client；`layout.tsx` 菜单加 "渠道健康"；`channels-health/page.tsx` 新建

### 需人工验证

1. **页面可访问**：admin 登录 → 左侧菜单看到「渠道健康」→ 点击 → 页面加载
2. **空状态**：刚启动没有失败记录 → 4 个 KPI 全部 0 → 渠道表 / 最近失败表均显示 "暂无失败记录"
3. **触发失败**：
   - 配一个 channel mock 返 429（Sprint 2 验证已就绪）
   - 调 3 次
   - 页面看到：
     - KPI：失败总数=3，失败渠道数=1，最长冷却=60
     - 渠道表：1 行，failureCount=3，isInCooldown=true，cooldownRemaining=60
     - 最近失败：3 行，channelName=mock，model=xxx，statusCode=429
4. **冷却倒计时**：mock 429 后等 10s → 点 [刷新] → cooldownRemaining=50
5. **冷却过期**：等 60s → 刷新 → isInCooldown=false，cooldownRemaining=0
6. **清空冷却**：
   - 再触发 1 次失败 → 看到冷却 60s
   - 点 [清空冷却] → 弹 Popconfirm → 确认 → toast "已清空 1 个" → 页面刷新 → isInCooldown=false
7. **多 channel 排序**：mock 2 个 channel 各 3 / 5 次失败 → 渠道表按 failureCount 倒序（5 在前）
8. **AffectedModels**：同 channel 调 3 个不同模型 → 渠道表 `affectedModels: ["m1","m2","m3"]`
9. **进程重启清空**：服务重启 → 刷新页面 → 全部为 0（ring buffer 行为）
10. **手动刷新**：点 [刷新] 按钮 → loading 状态可见 → 重新加载
11. **权限**：非 admin 访问 `/admin/channels-health` → 跳登录页（沿用现有 layout 鉴权）
12. **回归**：Sprint 1.1 token 鉴权、Sprint 1.5 API Key UI、Sprint 2 渠道选择器、Sprint 2.5 admin 渠道配置全部不变

## Sprint 2.5：admin 渠道管理 UI 升级

把 Sprint 2 的能力（多 key / 优先级 / 状态码 failover / cooldown）暴露给 admin 抽屉表单 + 修复多 key 明文泄露漏洞。

### 改动一览
- **后端**：`service/settings.go::hidePrivateAPIKeys` 同步清空 `Keys []string`（**安全修复**）
- **前端 type**：`AdminModelChannel` + `AdminPublicModelChannelInfo` 加 6 个新字段
- **抽屉 Drawer**：插入「高级选项」`Collapse`（默认折叠）+ 多 Key 轮询 Textarea
- **列表 Table**：新增「优先级」列 +「Keys」列
- **数据兼容**：`normalizeChannel` / `mergeChannelApiKeys` 扩展

### 需人工验证

1. **安全漏洞修复（最优先）**：
   - admin 调 `GET /api/admin/settings`（浏览器开 DevTools Network 面板看响应体）
   - channels 数组里 `keys` 字段为 `null`（不是明文！）
   - **改前是泄露的**，改后看不到完整 key

2. **基础兼容（向后兼容老用户）**：
   - 现有只配 `apiKey` 的渠道 → 编辑 Drawer 打开 → 「多 Key 轮询」文本框**为空**（不是后端默认填的 `[apiKey]`）→ 取消不改 → 保存 → 后端自动把单 `apiKey` 转为 `keys: [apiKey]`
   - 验证方法：再次编辑同一渠道，「多 Key 轮询」里能看到 1 个 key（自动迁移）

3. **新增多 key 渠道**：
   - admin 新增渠道 → 填 name / baseUrl / 「多 Key 轮询」填 3 行（如 `k1\nk2\nk3`）→ 提交
   - `POST /api/admin/settings` 看到 `keys: ["k1","k2","k3"]`、`apiKey: ""`（后端优先用 keys）
   - 调一次模型 → ai_log `keyIndex: 0`，再调 `keyIndex: 1`

4. **Priority 优先级**：
   - admin 把 channel A `priority: 0`、channel B `priority: 5`
   - 调 10 次 → ai_log 应该**全部在 A**（除非 A 失败）
   - 列表里 A 显示「默认」、B 显示「高优 5」

5. **StatusCodeMapping**：
   - admin 把 channel A `statusCodeMapping: "429,403"`
   - mock 返 429 → 切下一家
   - mock 返 400 → **不切**（400 不在映射里）

6. **cooldownSeconds**：
   - admin 把 channel A `cooldownSeconds: 30`
   - mock 429 → 30s 内被跳过

7. **Capability 能力**：
   - admin 把 channel A `capability: "image"`
   - 调图片生成 → 走 A
   - 调文本 → 走其他 channel（能力不匹配）

8. **Group 字段**：
   - 填「用户组」`vip` → 保存 → 下次调用（**Sprint 3 启用筛选前**）所有用户都能用（不报错）
   - Sprint 3 引入 UserGroup 后这里会变成灰度筛选字段

9. **列表展示**：
   - admin 渠道列表看到「优先级」列（Tag 着色）+「Keys」列
   - 多 key 渠道 Keys 列显示蓝色 Tag `3`
   - 单 key 渠道 Keys 列显示 `-`

10. **折叠面板**：
    - 默认折叠，点「高级选项」展开看到所有新字段

11. **保存校验**：
    - 空 `keys` + 空 `apiKey` 仍允许（admin 可能配本地 channel）
    - `priority: 0` 接受（默认优先级）

12. **回归**：
    - Sprint 1.1 token 鉴权不变
    - Sprint 1.5 API Key UI 行为不变
    - Sprint 2 多 key 轮询 + 状态码 failover 行为不变
    - admin 设置的非渠道部分（prompt 同步、license 等）未改动

## Sprint 2：渠道选择器端到端验证

P0 报告（[Script-to-Video Vendor Mismatch]）的根治：上游失败时**自动切下一家**。

### 后端改动一览
- `model/setting.go` `ModelChannel` 加 `Priority` / `StatusCodeMapping` / `CooldownSeconds` / `Keys` / `Group` / `Capability`；`PublicModelChannelInfo` 同步加 `Priority` / `KeyCount` / 等
- `model/ability.go` 新增：能力倒排索引 + `GetAbilitiesByKey` / `SetAbilityMap` / `ClearAbilityCache`
- `model/ai_log.go` `AICallLog` 加 `AttemptIndex` / `UpstreamStatusCode` / `KeyIndex` / `LastTryAt`
- `service/ability_cache` 在 `main.go` 启动期 + `SaveSettings` 后异步重建
- `service/channel_selector.go` 新增：`PickChannelWithRetry` / `MarkChannelFail` / `BuildAbilityCache` / `cooldownMap` / `shouldTriggerCooldown`
- `service/channel_fail_log.go` 新增：内存 ring buffer + `RecordChannelFailWithContext` / `ListChannelFailLogs`
- `service/settings.go` `SaveSettings` 调 `BuildAbilityCache`（异步）+ `publicChannelInfos` 透传新字段
- `service/ai_log.go` `AICallLogInput` 加同名字段 + 写入
- `handler/ai.go` 拆 `proxyAIRequest` → `runLocalChannelSingle`（本地渠道） + `runRemoteChannelWithRetry`（云端 retry loop）+ `capabilityOf` / `normalizeRemoteImageBody` / `doProbeRequest` / `writeProbeResponse`
- `handler/admin_channel_fail_log.go` 新增：失败日志接口
- `router/router.go` 加 `GET /api/admin/channel-fail-logs`

### 需人工验证

**启动期 abilities 构建**：
- `go run .` 启动后看日志 `[ability] built N abilities from M channels`（Sprint 2 启动 log 由 BuildAbilityCache 内部 print）

**多 key 轮询**：
- admin 在 `settings.private.channels` 把 channel A 的 `keys` 字段改为 `["key1","key2","key3"]`（JSON 数组）
- 调一次 → ai_log `keyIndex: 0`
- 再调一次 → ai_log `keyIndex: 1`
- 再调一次 → ai_log `keyIndex: 2`
- 再调一次 → ai_log `keyIndex: 0`（轮询）

**优先级 + 权重**：
- channel A `priority: 0, weight: 1`
- channel B `priority: 10, weight: 100`
- 调 10 次 → ai_log 应该**全部在 A**（除非 A 失败）；同 priority 内按 weight 随机

**状态码 failover**：
- mock 一个 channel 固定返 429
- 调一次 → 第一次 ai_log 显示 status=429（attempt 0），自动切下一家 channel；最终返回成功响应
- admin 看 `GET /api/admin/channel-fail-logs` 应能看到 mock channel 的失败记录

**cooldown 熔断**：
- mock 429 后立即再调 → mock channel 在 60s 冷却内被跳过，**直接走下一家**
- 60s 后再调 → mock channel 重新可被选

**StatusCodeMapping 自定义**：
- mock channel 配 `statusCodeMapping: "403,404"`
- mock 返 403 → 切下一家
- mock 返 400 → **不切**（400 不在映射里），直接报错给用户

**admin 改 channels 后缓存失效**：
- admin `POST /api/admin/settings` 改一个 channel 的 weight
- 下次请求立即按新 weight 走（无需重启服务）

**vendor 路径不受影响**：
- UpDream 用户用 sk-token 调 `/v1/images/generations` → 仍走 `dispatchVendorProxy`，**不**走新 selector
- vendor 失败 fallback 仍走 `SelectModelChannelForModel`（老路径，本 Sprint 保留）

**ai_log 新字段**：
- admin `/admin/ai-logs` 看 `attemptIndex / upstreamStatusCode / keyIndex / lastTryAt` 字段全部填充

**cooldown 持久化（手动验证）**：
- 触发一个 channel 失败 → `service/cooldownMap[channelID] = now+60s`
- 重启服务 → cooldown 清零（符合"重启=恢复"预期）

### 回归（确保没破坏）
- 现有 `curl -H "Authorization: Bearer sk-fk-..." /v1/chat/completions` 正常
- 现有 cookie 登录 + 画布 + 工作台 全部不变
- 现有 LinuxDo OAuth 登录不变
- Sprint 1.5 API Key UI 行为不变
- 现有本地渠道（userChannelID != ""）单次请求行为不变
- UpDream / LibTV / NewWow vendor 路径完全不变

## Sprint 1.5：API Key 管理前端页面端到端验证

Sprint 1.1 后端的 user_token 能力在 `/wallet` 加了第 4 个 tab「API Key」。

### 前端改动一览
- `web/src/services/api/user_token.ts` 新增：5 个 API client
- `web/src/app/(user)/wallet/components/api-key-manager.tsx` 新增：主组件
- `web/src/app/(user)/wallet/components/api-key-create-modal.tsx` 新增：创建表单
- `web/src/app/(user)/wallet/components/api-key-reveal-modal.tsx` 新增：**关键**明文展示弹窗
- `web/src/app/(user)/wallet/page.tsx` 修改：Tabs.items 末尾追加第 4 个 tab

### 需人工验证

1. **登录后进 `/wallet`** → 默认显示"余额流水"tab → 切到第 4 个"API Key"tab → 看到空状态"还没有 API Key，点击创建"。
2. **点 [+ 创建 API Key]** → 弹创建表单 → 填 name=「test-1」→ 点创建 → 表单弹窗关闭 + 弹出"⚠️ 请立即保存" 弹窗，**显示完整明文 `sk-fk-...`**。
3. **点 [📋 复制 Key]** → toast "已复制 Key，请妥善保存" → 粘贴到任意地方能拿到完整 key。
4. **关闭按钮 disabled** → 「我已保存」未勾选时点关闭按钮无效 → 勾选后关闭按钮启用 → 点关闭弹窗。
5. **关后无法再看** → 再点 [+ 创建 API Key] 是新流程，**旧 key 永远只能看到 `sk-fk-...1234` 脱敏**。
6. **列表显示**：`keyPrefix` 脱敏为 `sk-fk-...xxxx`；状态 Tag 绿色 active；用量显示 `¥0.00（用账户余额）`；最后使用 = "从未使用"；操作列有「禁用」「删除」。
7. **拿 key 跑 curl**（按 Sprint 1.1 pending-test 已就绪）：
   ```bash
   curl -H "Authorization: Bearer sk-fk-..." http://localhost:3000/v1/chat/completions \
     -H 'Content-Type: application/json' \
     -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
   ```
   → 正常响应。
8. **回列表看** → `lastUsedAt` 刷新到刚刚，`lastUsedIp` 填上，`usedCents` 数字更新。
9. **点 [禁用]** → Popconfirm 二次确认 "禁用后该 Key 立即失效" → 状态变 `disabled`、Tag 灰色 → 拿该 key 再 curl → 401。
10. **点 [启用]** → 状态恢复 `active` → curl 恢复。
11. **点 [删除]** → Popconfirm 二次确认 danger 红色 → 该行消失 → curl → 401。
12. **高级选项**：创一个 `expiredAt = 2024-01-01` 的 → 列表显示 → 调 → 401（Status 自动转 `expired`，按钮变禁用）。
13. **高级选项**：创一个 `balanceCapCents = 100`（1 元）的 → 调一次扣几分 → 列表 `usedCents` 显示对应值，"额度用量"列显示 `¥0.04 / ¥1.00`。
14. **未登录访问 `/wallet` API Key tab**：跳登录页（沿用现有 wallet 鉴权逻辑，不变）。

**回归**（确保没破坏现有 3 个 tab）：余额流水 / 卡密兑换 / 邀请 tab 显示和行为完全不变。

## Sprint 1.1：用户自建 API Key（sk- token）端到端验证

新增 `user_tokens` 表与 `Authorization: Bearer sk-fk-...` 鉴权路径，让外部 SDK 直接对接 Freedom。

### 后端改动一览
- `model/user_token.go` 新增 `UserToken` struct + 4 个状态常量
- `model/ai_log.go` `AICallLog` 加 `TokenID string` indexed 字段
- `repository/user_token.go` 7 个 CRUD（Save / GetByID / GetByHash / ListByUser / Delete / UpdateStatus / UpdateLastUsed / IncrementUsedCents）
- `repository/db.go` AutoMigrate 追加 `&model.UserToken{}`
- `service/user_token.go` CreateUserToken / ListUserTokens / DeleteUserToken / SetUserTokenStatus / CurrentAuthUserByTokenFull + `hashToken` `randomURLSafe` `ipAllowed` helpers
- `service/context.go` 加 `WithUserToken` / `UserTokenFromContext`
- `service/auth.go::ConsumeUserBalanceWithHold` 末尾加 `tokenID ...string` 可变参数
- `service/ai_log.go` `AICallLogInput` 加 `TokenID` 字段
- `service/workflow_agent.go` 调 `ConsumeUserBalanceWithHold` 时把 ctx 里的 token id 透传
- `middleware/admin.go::authUser` 加 `Bearer sk-` 前置分支
- `handler/ai.go` 所有 `ConsumeUserBalanceWithHold` 调用点传 `tokenIDFromContext(r.Context())`；`aiLogContext` + `saveAIProxyLog` 写入 `TokenID`；新加 `tokenIDFromContext` helper
- `handler/video_task.go` 同上
- `handler/user_token.go` 4 个 HTTP handler（Create / List / Delete / SetStatus）
- `router/router.go` 5 个 `/api/v1/user-tokens` 路由

### 需人工验证

**建表**：
- `go run .` 启动后看 MySQL 是否自动建出 `user_tokens` 表（列名 / 索引 / 类型对照 `docs/backend/backend-database.md` §user_tokens）。
- `ai_call_logs` 是否自动加 `token_id` 列（已有 indexed）。

**创建 token（明文一次性返回）**：
```bash
curl -c cookie.txt -X POST http://localhost:3000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"<你的账号>","password":"<密码>"}'
curl -b cookie.txt -X POST http://localhost:3000/api/v1/user-tokens \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-curl-token"}'
# 响应里 data.raw = "sk-fk-..."（仅此一次，刷新页面后无法再看到完整 key）
# data.token.keyPrefix = "sk-fk-xxx..."（脱敏后）
# data.token.keyHash 永远不返回
```

**Bearer 鉴权调 chat（无 cookie）**：
```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-fk-..." \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
# → 走 OpenAI 格式响应，扣用户余额
```

**Bearer 鉴权调生图**：
```bash
curl -X POST http://localhost:3000/v1/images/generations \
  -H "Authorization: Bearer sk-fk-..." \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-image-1","prompt":"a cat","n":1}'
# → 走 OpenAI 格式响应，扣用户余额
```

**错误 token 401**：
```bash
curl -H "Authorization: Bearer sk-fk-FAKE-FAKE" http://localhost:3000/v1/chat/completions -d '{}' -H 'Content-Type: application/json'
# → 401
```

**过期 token 自动失效**：
```bash
# 建一个 expiredAt = "2024-01-01" 的 token
curl -b cookie.txt -X POST http://localhost:3000/api/v1/user-tokens \
  -H 'Content-Type: application/json' \
  -d '{"name":"expired-test","expiredAt":"2024-01-01T00:00:00Z"}'
# → 401；查 user_tokens 表确认 status = "expired"
```

**IP 白名单拒绝**：
```bash
curl -b cookie.txt -X POST http://localhost:3000/api/v1/user-tokens \
  -H 'Content-Type: application/json' \
  -d '{"name":"ip-restricted","allowIps":["127.0.0.1"]}'
# 用该 token 从其他 IP 调 → 403 "IP 不在允许列表"
```

**列表 + 删除**：
```bash
curl -b cookie.txt http://localhost:3000/api/v1/user-tokens
# → items[*].keyHash 全空，keyPrefix 已脱敏为 "sk-fk-...1234"
curl -b cookie.txt -X DELETE http://localhost:3000/api/v1/user-tokens/<id>
# → 删除后该 token 立即 401
```

**禁用 / 启用**：
```bash
curl -b cookie.txt -X POST http://localhost:3000/api/v1/user-tokens/<id>/disable
curl -b cookie.txt -X POST http://localhost:3000/api/v1/user-tokens/<id>/enable
# 期间用该 token 调 → 401
```

**ai_log 关联**：
- admin 后台看 `/admin/ai-logs`，sk-token 鉴权的最新一条记录 `tokenId` 字段应有值；cookie 鉴权的为 `null`。
- 查 `balance_logs.extra`（MySQL JSON 字段），应能看到 `"tokenId":"utok-..."`。

**回归（确保没破坏）**：
- 现有 `curl -b cookie.txt http://localhost:3000/api/v1/canvas/projects` 行为不变。
- 现有画布/工作台前端无任何改动，所有路由仍能正常使用。
- 现有 LinuxDo OAuth 登录流程不变。
- UpDream / LibTV / NewWow 视频 vendor 走 `vendor_proxy.go` 不受 token 影响（token 仅做"是谁"识别）。

## 远程媒体下载 / 上传 415 修复（新增 proxy-media 媒体代理）

用户反馈「上传和下载都显示媒体下载失败415」。根因：`downloadRemoteMedia`（下载远程媒体；上传远程媒体 URL 也复用它）把**所有**媒体 URL 走后端 `/api/proxy-image` 图片代理，而该代理只放行 `image/*`，`video/mp4` 等一律返回 415「仅支持图片代理」→ 前端包成「媒体下载失败：415」。

改动：

- `handler/storage.go`：把原 `ProxyImage` 逻辑抽成内部函数 `proxyRemoteContent(w, r, allowVideo, maxBytes)`；新增 `ProxyMedia`（图片**和视频**都放行，上限 100MB，拒绝时文案「仅支持图片或视频代理」）；`ProxyImage` 行为保持不变（仅图片、32MB）。
- `router/router.go`：`/api` 组新增 `GET /api/proxy-media`（与 `/proxy-image` 同级）。
- `web/src/services/image-storage.ts`：新增 `getMediaProxyUrl`（与 `getProxyUrl` 相同的本地/同源绕过逻辑，但指向 `/api/proxy-media`）。
- `web/src/services/file-storage.ts`：`downloadRemoteMedia` 改用 `getMediaProxyUrl`；移除不再使用的 `getProxyUrl` 导入。

### 需人工验证

- 视频工作台下载某条历史视频（远程 URL）→ 不再报「媒体下载失败415」，正常拿到 mp4 保存。
- novel 工作台 / 画布上传远程视频 URL 到服务端（`uploadRemoteMediaToServer`）→ 不再 415。
- 图片相关路径（参考图 / `uploadImage` / `imageToDataUrl`）仍走 `/api/proxy-image`，行为不变。
- 超过 32MB 的远程图片代理仍被拒（保持原行为）；媒体代理单次上限 100MB。
- 非 http(s) 或同源/内网地址仍直连不走代理（`getMediaProxyUrl` 绕过逻辑与 `getProxyUrl` 一致）。

## 生图参考图加载健壮化（修复线上 "Failed to fetch"）

线上生图（带参考图）全部报 `Error: Failed to fetch`。根因：画布节点里 gpt-image-2（rolldek）生成的图存的是厂商 CDN URL（`download2.bnq777.xyz`），该 CDN 现不可达（本机/服务器均 502、DNS 异常）；拿它当参考图时，`imageToDataUrl` 拉取失败 → 整批生图在发请求前中止（DB 无任务记录）。

改动：

- `web/src/services/image-storage.ts`：`imageToDataUrl` 拉取参考图加 30s 超时（AbortController）；失败抛清晰中文错误（`参考图加载失败（域名 不可访问）` / `参考图加载超时`），不再透出原生 `Failed to fetch`；新增 `resolveReferenceDataUrls` 批量 helper——单张参考图失败**跳过**（不 abort 整批，console.warn 提示），全部失败才抛错。
- `web/src/services/api/image.ts`：`requestImages`（:896）与 `createCanvasImageTaskRequest`（:994）改用 `resolveReferenceDataUrls`。
- `web/src/services/api/video.ts`：视频参考图（grok2api :230 / seedance :253）同样改用 `resolveReferenceDataUrls`（同一类问题）。
- `web/src/app/(user)/canvas/components/canvas-node-generation.ts`：`hydrateNodeGenerationContext` 批量参考图改 `Promise.allSettled` 跳过坏图，首/尾帧失败降级为 null。

### 需人工验证

- 画布/生图工作台：拿一张「图片地址已失效」（如旧的 gpt-image-2 图）当参考图生图 → 不再整批报 `Failed to fetch`；单张坏图被跳过，其余参考图正常参与生成；多张都坏时提示「参考图加载失败（xxx 不可访问）」。
- 视频生成带参考图同样不因单张坏图中断。
- 正常参考图（本地/服务器存储/可达 CDN）行为不变。

## 自动定价：上游定价 × 加价率写入 modelCosts（视频 +50% / 图片 +20%）

新增 `service/model_pricing_scheduler.go`：每天（0 点）拉取一次 `PRICING_URL`（默认 `https://rolldek.com/api/pricing`）的模型定价（人民币元），按「视频 +50%、图片 +20%」加价率换算成人民币分，合并写回 `settings.public.modelChannel.modelCosts`；pricing 里出现的模型（rolldek 图片/视频）总是覆盖为自动价，其它渠道（如 agnes 文本）定价原样保留。`main.go` 已启动调度器，进程启动即执行一次。

- 加价公式：分 = round(上游人民币元 × 100 × 加价率)；视频加价率 1.5、图片 1.2
- 分类：per_second 计费 → 视频；模型名含 seedance/kling/sd- → 视频；含 image/gpt-image → 图片；其余跳过（如文本模型）
- 示例：gpt-image-2 = 0.04 × 100 × 1.2 ≈ 5 分/次（¥0.05/次）；kling-3.0-omni-720p = 0.04 × 100 × 1.5 = 6 分/秒（¥0.06/秒）

### 需人工验证

- 重启 backend 后：模型下拉框价格自动显示，无需再手工设置 modelCosts
- admin 系统设置 → 公开模型 → modelCosts 里能看到 rolldek 模型被自动填充价格
- 用任意 rolldek 图片/视频模型生成一次，余额按自动价扣费（如 gpt-image-2 一次约 ¥0.05）
- 后台手工改过的其它渠道模型定价不被自动定价覆盖

## 图片工作台：载入历史任务后尺寸/质量按当前模型自动归一化

修复：底部布局使用原生 `<select>`，当 `config.size` 不在当前模型合法选项（如 Gemini 不支持 `21:9`）时，浏览器会默认显示第一个 `<option>`（如 `1:1`），但底层 state 仍是 `21:9`，导致用户看到 `1:1` 实际却发出 `21:9`，上游返回 `unsupported image size`。已在三条路径强制按当前图片模型能力白名单吸附 size / quality：

- `previewGenerationLog`（点击历史记录「载入」时）
- 两个 `ModelPicker` 的 `onChange`（切换图片模型时，含侧边/底部布局）
- `buildRequestSnapshot`（组装最终生成请求时兜底）

改动文件：`web/src/app/(user)/image/page.tsx`。

### 需人工验证

- 底部布局：点击一条 `21:9` 的历史失败记录「载入」→ 底部「尺寸」下拉应同步变为当前模型合法值（Gemini 为 `1:1`），再次生成不再报 `unsupported image size`。
- 侧边布局：同样验证「载入」后尺寸与模型匹配。
- 切换图片模型（如从 seedream 切到 Gemini）时，当前 `21:9` 自动吸附为 `1:1`；从 Gemini 切到 seedream 时 `1:1` 保持合法。
- 重试旧失败记录时，请求 size / quality 也按当前模型重新归一化。

## 前端所有「积分 / 扣点」残留清零（画布 + 工作台 + admin 日志）

用户原话：「**前端也有问题你不早改**」「**这句 1 元 = 100 点的逻辑代码中不能存在**」——前一轮 admin settings 清了，但画布 / image / video / novel 三个工作台 + admin ai-logs 还有"积分"文案和「cents 整数直接渲染」的展示残留（用户截图里画布节点「开始生成」按钮显示的 `⚡ 6 个` 就是这条 bug 的最显眼表现：6 cents = 0.06 元，应显示 `¥0.06`）。

**改动 13 处**（cents 整数展示 → 转元 + 加 ¥；"积分"文案 → "余额 / 扣费"）：

| # | 文件 | 改动 |
|---|---|---|
| 1 | `web/src/app/(user)/canvas/components/canvas-node-prompt-panel.tsx` | import 加 `formatBalanceYuan`；line 132 `{cents.toLocaleString()}` → `{formatBalanceYuan(cents)}` |
| 2-4 | `web/src/app/(user)/image/page.tsx` 三处 | `<span>{costInfo.costCents.toLocaleString()} {costInfo.unit}</span>` → `<span>¥{((costInfo.costCents \|\| 0) / 100).toFixed(2)} /张</span>` |
| 5-7 | `web/src/app/(user)/video/page.tsx` 三处 | 同样改：`<span>¥{...} {costInfo.isPerSecond ? "/秒" : "/次"}</span>`（按 isPerSecond 区分单位） |
| 8 | `web/src/app/(user)/novel/page.tsx:53` import | 加 `formatBalanceYuan` |
| 9 | `web/src/app/(user)/novel/page.tsx:5385` Tooltip | `shotCredit.costCents.toLocaleString()` → `formatBalanceYuan(shotCredit.costCents)` |
| 10 | `web/src/app/(user)/novel/page.tsx:5386` 单镜约耗 | 同样改 `formatBalanceYuan` |
| 11 | `web/src/app/(user)/novel/page.tsx:5402` 余额 | `userBalanceCents.toLocaleString()` → `formatBalanceYuan(userBalanceCents)` |
| 12 | `web/src/app/(user)/novel/page.tsx:454` Modal 标题 | `"积分/余额不足"` → `"余额不足"` |
| 13 | `web/src/app/(admin)/admin/ai-logs/page.tsx:185` 列定义 | title `"扣点"` → `"扣费（元）"`；加 `render: (value) => `¥${((value \|\| 0) / 100).toFixed(2)}` |

注释里"积分 / 扣点 / 1元=100"字样也清完（balance.tsx / wallet/page.tsx / vendor.ts / video+image page.tsx / canvas-config-node-panel.tsx / canvas agent skills core.ts 共 8 处）。保留 `costCents` 字段名、参数名、`formatBalanceYuan(cents)` 内部 `cents/100` 公式——这些是数据层 / 函数签名 / API 契约，不是用户字面反对的"1元=100点"逻辑本身。

全项目 `grep "积分\|扣点\|×100\s*转\|100\s*点\|算力点\|每点\|1元=100"` 结果 0 hit。

### 需人工验证

- 画布节点下方"开始生成"按钮：6 cents 整数 → 显示 `¥0.06`（不再是 `⚡ 6 个`）
- 图片工作台"剩余免费次数"卡片：6 cents → `¥0.06 /张`
- 视频工作台"剩余免费次数"卡片：6 cents → `¥0.06 /次`（或 `¥0.06 /秒` 如果该模型 unit 是 per_second）
- novel 工作台配置区"单镜约耗"和"余额"chip：6 cents → `¥0.06`
- novel 工作台点"开始"弹"余额不足"Modal：标题 = `余额不足`（不再是 `积分/余额不足`）
- admin/ai-logs 列表「扣费（元）」列：cents=6 → `¥0.06`（不再是整数 6）
- 全项目搜"积分 / 扣点 / 1元=100 / 100点"：0 hit

## 系统设置「1元=100点」逻辑清零（admin 模型扣费区）

用户要求「**这句 1 元 = 100 点的逻辑代码中不能存在**」「单次额度的单位是元，可以是小数」。把 admin settings / 相关代码注释里"×100 转 cents 存储（1 元 = 100 cents）"字样**全部删除**；底层 `costCents` 字段名 + `/100` 换算（数据契约与整数存储）保留——它们是技术实现，不是用户反对的"1元=100点"逻辑本身。

**改动 5 处**：

1. `web/src/app/(admin)/admin/settings/page.tsx:723` 顶部说明：删 `，保存时自动 ×100 转 cents 存储（1 元 = 100 cents）`，新文案：`扣费按元填写（如 0.08 = 8 分钱）。图片按「元/张 × 生成数量」扣费；视频可选「按次（per_call）」或「按秒 × 视频秒数（per_second）」；文本/音频按「元/次」扣费。`
2. `web/src/app/(admin)/admin/settings/page.tsx:788` "单次额度"列 Tooltip：删 `，保存时 ×100 转 cents 存储`，新文案：`按元填写（如 0.07 = 7 分钱）。视频切到「按视频秒数（per_second）」时此栏不生效。`
3. `web/src/app/(admin)/admin/settings/page.tsx:817` "每秒额度"列 Tooltip：删 `，保存时 ×100 转 cents 存储`，新文案：`按元/秒填写（如 0.10 = 一角钱一秒）。仅在「扣费单位」选 per_second 时生效。`
4. `web/src/services/api/admin.ts:230` 注释：`// 单次扣费（分，1 元 = 100 cents）` → `// 单次扣费，整数存储避免浮点误差`
5. `web/src/constant/balance.tsx:70-71` 注释：`把分（cents）格式化为 "¥X.XX" 显示。/ * 1 元 = 100 cents。` → `把内部整数余额格式化为 "¥X.XX" 显示。`

全项目 `grep "1\s*元\s*=\s*100|×100\s*转|1元=100|100\s*点"` 结果 0 hit。

### 需人工验证

- 打开 admin settings → "模型余额" 区块顶部说明文案**只剩**「扣费按元填写（如 0.08 = 8 分钱）。图片按「元/张 × 生成数量」扣费；...」，**不再出现**「1元=100」「×100 转 cents 存储」字样。
- "单次额度"列：图片模型后缀是「元/张」、视频切到 per_second 时是「元/次（不生效）」、其他是「元/次」；hover Tooltip 是「按元填写（如 0.07 = 7 分钱）...」，没有"保存时 ×100 转 cents 存储"。
- "每秒额度"列：后缀是「元/秒」；hover Tooltip 是「按元/秒填写（如 0.10 = 一角钱一秒）...」。
- "单次额度"输入框：默认 `step={0.01} precision={4}`，**可以直接键入 0.06、0.07 这种小数**（不再只能输整数 6），存进 DB 的是 `Math.round(0.06*100)=6` cents。
- 旧 DB 值（`costCents=600`，即 6 元）打开后 input 自动按 `600/100=6` 显示为「6 元」——管理员能直观看到是 6 元不是 6 分。
- 切 per_call ↔ per_second 时「单次额度」/「每秒额度」两列仍按既定的对称禁用规则（per_call 时"每秒额度"置灰且无 placeholder、per_second 时反之）。
- 顶部说明 / 两个 Tooltip / 代码注释里**任何位置**都搜不到"1元=100"、"100点"、"×100 转 cents"字样。

## 支付系统重构（路线 2：去"算力点"概念）

把原来"积分"为单位的钱袋系统推翻，改为直接按人民币元（¥）扣费。后端字段全表重命名，路由同步改，文案统一"余额 / ¥X.XX"，卡密从"用户兑换"改为"admin 手动补发"通道。详见 `docs/progress/todo.md` 与 `CHANGELOG.md` Unreleased。

### 需人工验证

- **未登录**访问任何页面 → 顶栏不再显示"积分 / Credits"徽标，新组件 `BalanceBadge` 在未登录态隐藏或弱化为 ¥0.00 + 登录引导。
- 登录后顶栏 → "当前账户余额 ¥X.XX" + 小图标，鼠标 hover 显示「点击查看充值说明」，跳转 `/wallet`。
- 用户 `/wallet` 页面：标题"账户余额"，余额以 ¥X.XX 显示（约可生成 X 张图片按 ¥0.04/张估算仅参考），下方"余额流水"表列出：类型（后台调整 / 模型消费 / 失败返还 / 卡密充值）、变动金额（+¥X.XX / -¥X.XX）、变动后余额、备注、关联 ID。**不再有「兑换卡密」表单或"充值记录" Tab**（卡密前台兑换已下线）。
- 用户发起 AI 生成（图片/视频/文本）：成功扣费 `¥X.XX`，前端 toast 显余额变化；余额不足时返回错误"余额不足"，前端引导用户去 `/wallet`。
- **admin** 用户管理页 → 用户列表 + "余额调整"动作：表单字段名 `balanceCents`，保存后该用户 `users.balance_cents` 写入整数分；前端列「余额」显示 ¥X.XX。
- **admin** 卡密管理页 → 批量生成卡密 + 整批修改面额：面额以"分（cents）"录入，导入/编辑/展示文案统一 `¥X.XX`，不再出现"0.04 Credits / 张"等历史文案。
- **admin** 余额日志页 → 路由从 `/admin/credit-logs` 改为 `/admin/balance-logs`，菜单项 / 浏览器地址都同步；流水编辑表单的"变动金额 / 变动后余额"输入以分（cents）整数录入，列表显示 `+¥X.XX` / `-¥X.XX`。
- 三家供应商侧账户（UpDream / NewWow / LibTV）仍走自己的计费，本系统不参与；前端顶栏显示"供应商余额"文案不变（来自 `activeVendorAccount.balanceText`），官方模式下才显示本系统余额徽标。
- 供应商异步视频 / 生图：上游接口返回的"预估金额"在新代码里也按"分（cents）"解析；模型返回 `data.power` 或 `estimated_credits` 都视同 cents，画布节点预估 chip 显示 ¥X.XX 而非 "1234 Credits"。
- **数据迁移说明**：项目尚未上线（AGENTS.md 明确），按用户确认不走迁移脚本；线上若已有历史 Credits 余额，新代码读 `users.balance_cents` 字段取不到值会按 0 计算。如有真实付费用户需先备份数据再上线。
- admin 在用户管理页对某用户执行"余额调整"，保存后该用户在 `/wallet` 立即看到 `¥X.XX` 并新增一行 type=`manual_adjust` 的流水条目。

## 分镜前自动生成分镜图片：改从分镜剧本中取名（替代"按原文自由提取"）

`web/src/app/(user)/novel/page.tsx` 重写了"分镜前自动生成分镜图片"开关的资产生成逻辑：从原来的 `generateAssetsFromNovel`（直接读章节原文让文本模型"自由发挥"提取 `{name, description}` JSON，经常冒出新的角色名/写出错的描述、与已定义出厂角色脱节）改成 `generateAssetsFromStoryboards`（先等分镜剧本产出，再从每个剧本的「出场角色 / 场景」行 + 全文 regex 抓 `@` 道具名 → 用 `extractStoryboardHeader` 与新 regex 拿到**确定性**名字 → 再让文本模型按剧本上下文给这些名字生成描述 → 走 `buildAssetPrompt + requestGeneration`）。

调用点迁移：从 `handleParseStoryboards` / `handleParseStoryboardsLocal` 顶部挪到分镜产出后——后端任务版挪到 `pollStoryboardTask` 的 `task.status === "completed"` 分支末尾（仅 completed 触发），前端直连版挪到 `Promise.all(workers)` 完成且 `!controller.signal.aborted` 分支末尾；两次调用都用各自新建的 `AbortController` 透传给 `setAbortController(assetController)`，与"停止"按钮接通。

主开关的 Tooltip 文案同步改为「分镜剧本产出后，自动从剧本里提取角色/场景/道具并生成分镜图片（已有同名资产复用，不重复生成）」。

新增文本模型入口 `extractDescriptionsForAssets(items, scriptContext, controller)`：给定 `[{name, type, contextSnippet}]` + 全量剧本上下文，调 `callTextModel` 让模型按上下文为每个名字写出 30-80 字 `description`，结果按 items 顺序对齐（漏给时用 name 占位），与 `generateAssetsFromStoryboards` 配合实现"名字确定、描述走文模"。

修前的根因：分镜剧本提示词已经把"角色外观必须严格参考【角色/场景/道具参考文档】"写进去了，分镜剧本的「出场角色」行里其实已经是受控的名字（沈一言/齐川/赵星辰），但旧流程在分镜剧本产出前就让文本模型按原文重新提取一遍，绕开这个约束、产出陈年/陈义 之类的副本。

### 需人工验证
- 项目里已有 ≥1 个角色资产（沈一言/齐川/赵星辰 等出厂角色），勾选章节 → 开启「分镜前自动生成分镜图片」→ 点开始分镜 → UI 文案是「分镜剧本产出后，自动从剧本里提取角色/场景/道具并生成分镜图片（已有同名资产复用，不重复生成）」；进入流程：先出分镜剧本 → 顶部进度条走完后立即出现「正在从分镜剧本中提取并生成分镜图片...」→ 完成后左下"导入图片"区出现以剧本里出场角色命名的资产卡（沈一言 等），不再冒出原文里出现过的别名（陈年 等）。
- 「人物/场景/道具」三个 tab 资产会自动按剧本 regex 出的类型归类（"出场角色" → character，"场景" → scene，正文 regex 抓到的 `@道具名` → prop）；任意类型取消勾选 → 该类型不再触发生成、不再出现在失败/成功列表。
- 已有 alias 严格匹配复用：剧本里有"沈一言"且素材库已有"沈一言" → 不会再生成一张；剧本里冒出"沈一寻"（错别字/别名）→ 被当成新名字生成，不再像改前那样被静默跳过而留下"上半身不等于下半身"的奇怪三视图。
- 全空项目（没有任何出厂角色）→ 整套流程同改前可用，文本模型从无到有生成新描述。
- 「停止」按钮：在分镜产出后的资产生成阶段按 → 立即触发 `assetController.abort()` → 进度条停、生成本轮后续 item 不再继续，`pendingAssets` 状态被清掉，已生成但未 commit 的图片丢弃（不写进 `assets`）。
- 任务化版（登录用户）：后端任务完成后 UI 才进入「正在从分镜剧本中提取并生成分镜图片...」阶段，轮询中不会触发；任务 `failed` 状态不会触发资产生成（仅 `completed` 触发），失败章节的占位分镜（`⚠ ...`）也不会参与提取。
- 工具提示文案同步：左上的"图片"按钮 hover 显示的是新文案「分镜剧本产出后，自动从剧本里提取角色/场景/道具并生成分镜图片（已有同名资产复用，不重复生成）」，不再是「分镜前自动生成分镜图片」。

## 分镜图片三个子模板（角色/场景/道具）差异化

`service/settings.go:182-184`（后端默认值 `DefaultSystemPrompts().StoryboardImage`）和 `web/src/app/(user)/novel/page.tsx:230-232`（前端 `DEFAULT_ASSET_PROMPT`）原内容完全一致，三个子模板贴了同一段 40 字长的画风后缀 `3D古风动漫风格，3D风格，国漫仙逆风格，精致感，高质量，柔焦，细节刻画，超高清，32K，大师级光影，明暗对比，杰出`，导致【场景四宫格】和【道具标准图】看起来几乎"只是改了行首标签"，【角色三视图】因为角色细节多相对好一点。

按"先讲自己这一类资产独有的诉求，再贴一行统一画风后缀"重写：

- 【角色三视图】聚焦角色本身：五视图身材/五官/肤色严格一致，肌肤纹理、五官对称、发丝/瞳孔/睫毛层次，服装版型/缝线/配饰在每个视角保持统一，重心稳定、手指数正确、无畸变；保留角色专属词「高级CG建模 / 伦勃朗光与柔光补光 / 梦幻感与胶片质感融合 / 商用/OC通用」。
- 【场景四宫格】聚焦环境一致性：四面板建筑结构/材质/植被/远景天际线一致，统一时段光线（日光/暮色/夜景任一）不混光，地面纹理/天空色调/季节氛围统一，空间纵深强、远中近景层次清晰；显式「剔除人物与前景道具干扰」；画风后缀换为「电影级场景氛围光 / 柔焦全焦远近清晰 / HDRI环境光 / 大师级纵深」。
- 【道具标准图】聚焦材质细节：金属高光/织物纹理/木纹年轮/玉石通透/玻璃折射/皮革毛孔等 6 类材质清晰可辨，磨损/做旧/符文纹路/装饰刻线等工艺特征强化，色彩还原真实无夸张偏色；「剔除人物与其他道具干扰」，画面对称、留白适中；画风后缀换为「产品级三维渲染 / 影棚三点布光 / 软阴影边缘 / PBR材质精细」。

统一画风后缀精简为 `3D古风X / 国漫仙逆风格 / 32K超清 / 大师级X / 杰出品质` 这一行（不再重复 `精致感/高质量/柔焦/明暗对比` 这种通用词），保留项目画风锚点的同时让三个模板的差异化诉求凸显。`{description}` 占位符、`parseAssetTemplates` 解析逻辑、`buildAssetPrompt` 拼接顺序、模板优先级（管理员后台 `storyboardImage` > `DEFAULT_ASSET_PROMPT`）均未改动。

### 需人工验证
- 「提示词配置」Modal 切到「分镜图片提示词」tab（未配置管理员后台值或未登录时），应看到三段新模板：`【角色三视图】...高级CG建模...商用/OC通用。` / `【场景四宫格】四宫格布局：{description}，同一地点四个固定视角——...剔除人物与前景道具干扰...HDRI环境光...` / `【道具标准图】纯白/纯灰渐变背景...PBR材质精细...杰出品质。`。
- 三段后缀都已精简到一行，不再有大段重复的 `3D古风动漫风格，3D风格，国漫仙逆风格，精致感，高质量，柔焦...`。
- 管理员后台 `systemPrompts.storyboardImage` 自定义值 → 前端 `assetPrompt` state → `parseAssetTemplates(assetPrompt)` 解析链路不变；切换管理员值后弹窗里立即看到新内容（与之前一样）。
- 提取资产后点卡片预览 / 「复制完整提示词」：发往图片模型的完整 prompt 是「管理员值/默认值 模板前缀 + 描述 + 画风后缀」，模板前缀部分应该反映出本轮差异（角色突出五视图一致 + 高级CG建模；场景突出四面板一致 + 时段光线统一 + HDRI；道具突出 6 类材质细节 + 产品级三维渲染）。
- 文本模型（自动提取画风）追加的画风后缀会接在模板前缀后面，与之前一样不受影响。
- 角色 / 场景 / 道具三类各跑一遍实际生成：角色图应保持五视图一致（无服装/发色/脸型跑偏）；场景四宫格应同一时段光线统一（不再出现左上面板是白天右下面板突然是傍晚）；道具图应是纯白/纯灰背景 + 主物居中 60%、不再混入其他道具/人物。
- 后端默认值（管理员后台未设置时）走 `DefaultSystemPrompts().StoryboardImage` 返回值与前端 `DEFAULT_ASSET_PROMPT` 字字一致；后端启动日志无报错。

## 剧本转视频配置弹窗：删除冗余的「并发数」

`web/src/app/(user)/novel/page.tsx` 创作配置 Modal 里 `视频并发数` 与 `生成模式 = 并行生成` 下的 `并发数` 实际作用重叠（前者是全局闸门 `videoGateRef`，后者是 `runConcurrent` 的 worker 数；并行批量模式时两层闸门互相限制，自动化模式下 worker 数又完全不生效）。

- 删掉 `maxParallel` state（`useState(2)`）。
- 删掉 Modal 里 `generateInParallel && (<div>...并发数...</div>)` 这块 UI；`生成模式` Select 保留 `顺序/并行` 二选一。
- `runConcurrent` 内 `workers.length >= maxParallel` 改为 `>= videoConcurrency`，使"批量并行时实际并行度 = 视频并发数"，单一闸门。
- 自动化模式（`autoPilot`）下视频任务依旧走 `videoGateRef` 排队，行为由 `视频并发数` 单一控制。

### 需人工验证
- 打开创作配置弹窗，`视频并发数` 仍可设（1–20，默认 5），再无 `并发数` 行；`生成模式` 仅剩 `顺序生成 / 并行生成` 二选一。
- `生成模式 = 顺序生成` 跑一键创作：行为不变（每次 `await generateShot`，排队过 `videoGateRef`，实际并行度受 `视频并发数` 限制）。
- `生成模式 = 并行生成` 跑一键创作：同时跑的 worker 数 ≤ `视频并发数`（之前受 maxParallel=2 卡死，调高 `视频并发数` 现在能真正生效）。
- 开启自动化模式 → 跑一次小说：每个新产出分镜走 `videoGateRef`，队列上限 = `视频并发数`；UI 没有任何 `并发数` 控件，调它不再改变任何东西。
- 状态不持久化（已删），刷新页面回到默认 `视频并发数=5`、`生成模式=顺序生成`。

## 分镜视频：参考图上传范围做成可配置开关

`web/src/app/(user)/novel/page.tsx` 把"分镜视频只带角色参考图"做成可配置项，方便切换到不支持场景/道具参考图的视频模型时手动收紧。

- 新增 state `videoReferenceCharactersOnly`，默认 `false`（沿用现有的"能带全带"行为，不破坏当前依赖场景/道具参考的用户）。
- `generateVideoFromStoryboard` 抽 header 时：开关 on → `headerMentions` 只取 `header.characters`，且 `assets.find` 加 `a.type === "character"` 过滤；off 时与现状一致（同时收 `characters` 和 `scenes`）。
- `handleAutoMatch`（@ 自动匹配）同步：开关 on → 跳过非 character 资产（跳过时仍记一笔到 `unlinked` 列表提示用户排查）。
- `buildVideoInput`：shot 有 `storyboardId` 且开关 on 才收紧到 `character`；`firstFrame / lastFrame` 是用户手动选的，不强制收紧，但选中资产若非 character 一并跳过（保持和 references 一致语义）。
- 设置 Modal 在「分辨率」和「衔接模式」之间新增「视频参考图范围」行，Switch `checkedChildren="仅角色" / unCheckedChildren="全部"`，下方一行说明文字解释切换理由。
- 状态不持久化（和 `videoConcurrency`、`generateInParallel` 等同页面其他视频开关一致，刷新重置）。

### 需人工验证
- 默认状态（开关 off）打开 novel 工作台 → 生成一条分镜剧本视频，请求 `extra_body.image` 应同时包含场景和角色两类参考图（与改前一致）。
- 打开「视频参考图范围」开关到「仅角色」→ 重新生成同一条分镜视频，请求 `extra_body.image` 仅含 `type === "character"` 的角色图，场景/道具不再出现。
- 分镜剧本头部同时含 `出场角色：A` 和 `场景：教室` 时：开关 off → 关联到 A + 教室两张；开关 on → 仅 A 进 `linkedIds`，教室静默跳过（不弹警告，与角色未匹配警告语义一致）。
- 一次性重新点「自动匹配所有」按钮：开关 on → 跳过的场景/道具别名出现在 antMessage 警告里，方便用户排查。
- 已生成的 shot（`referencedAssetIds` 已存）点「重新生成」视频：开关 on → 视频请求里不会带场景/道具（即使 `referencedAssetIds` 里存着它们的 id），因为 `buildVideoInput` 现在二次过滤。
- 手动给某个 shot 指定 firstFrame 为某张场景图，开关 on：firstFrame 也不进请求体（与 references 一致），用户选 character firstFrame 时照常进入。
- 切换开关不影响 `videoConcurrency` / `generateInParallel` / `enableConsecutiveFrames` 现有行为。

## 分镜图片「已提取资产」改为卡片模式

`web/src/app/(user)/novel/page.tsx`（剧本转视频 → 分镜图片 Library → 中列上半「生成分镜图片」区）原本每条提取出的角色/场景/道具都是 inline 渲染（类型徽章 + 名称 input + 描述 textarea + 删除 X），列表很长时占空间、视觉杂乱；现在改为：

- **卡片模式**：每条资产显示为一张小卡片（圆角 + hover 浅色高亮），卡片只展示「类型 + 名称 + 描述首行摘要」一行预览（参考右侧分镜剧本的卡片模式）。
- **点击卡片预览区**打开弹窗：弹窗可编辑名称 / 切换类型（角色/场景/道具）/ 编辑完整描述；底部「完整提示词」只读预览区实时显示「系统模板前缀 + 描述 + 画风后缀」拼接后的最终 prompt（即提交图片模型时原样发送的内容），可一键「复制完整提示词」导出。
- **删除**：卡片右侧 X 按钮；弹窗底部也有「删除」按钮。
- **手动新增**：列表底部「+新增角色/场景/道具」按钮，按当前 tab 类型新增一条空白资产并直接进入编辑弹窗。
- **批量继续下一步**：底部「② 生成图片（N 个）」按钮仍按现有 `buildAssetPrompt` + `requestGeneration` 链路逐条生成，无需手动干预。
- `extractedAssets` state 类型从 `{ name; type; description }` 扩展为 `{ id: nanoid(); name; type; description }`，`extractAssetsStep` 在写入时分配稳定 id；删除与弹窗定位改用 id 而非数组 index。

### 需人工验证
- 提取一批资产后，列表默认折叠（按现有 `extractedListExpanded` 行为）；点「已提取 N 个」展开，每条都是卡片预览（一行摘要），不再是 inline input + textarea 占满多行。
- 点某张卡片 → 弹窗打开，名称 / 类型 Select / 描述可编辑，修改后底部「完整提示词」预览实时反映拼接结果（系统模板前缀来自管理员后台 `storyboardImage` / 代码内 `DEFAULT_ASSET_PROMPT`，描述来自当前 draft，画风后缀来自「画风提示词」折叠区）。
- 弹窗「复制完整提示词」点一下 → 剪贴板内容 = 系统模板 + 描述 + 画风后缀，与「生成图片」实际发送给模型的 prompt 一致。
- 弹窗「保存」→ 关闭弹窗、列表卡片预览同步更新（名称 / 描述首行）；「删除」→ 关闭弹窗、该卡片从列表消失（不影响其它卡片与底部 N 个计数）。
- 列表底部「+新增角色/场景/道具」按当前 tab 类型新增一条空白资产并自动打开弹窗；编辑保存后出现在列表并计入「生成图片（N 个）」的 N。
- 切到「人物/场景/道具」不同 tab，列表只显示对应类型资产；空 tab 显示「此类型无资产」。
- 「重新提取」→ 旧资产被覆盖为新提取（带新 id），`viewAssetId` 指向被删除的 id 时弹窗自动不显示（`viewAsset === null`）。
- 点「生成图片（N 个）」跑完全部资产后，已生成的资产进入下方网格，列表卡片预览仍然保留，不影响二次编辑。

## LibTV 视频供应商分发（视频任务链路接入）

- `handler/video_task.go`：`proxyAIVideoTaskRequest` 顶部新增供应商视频分发分支（`dispatchVendorVideoProxy`）——用户激活非官方供应商且适配器实现 `service.VendorVideoSubmitter`（LibTV 的 `SubmitVideo`）时，提交视频任务拿 `generateUuid`，创建带 `vendor_type=libtv` 标记的 `VideoTask`（不扣官方积分）；未实现该接口的供应商（UpDream/NewWow）返回"暂不支持视频生成"。
- 轮询：`pollVideoTaskFromUpstream` 识别 `vendor_type` 非空 → `pollVendorVideoTask` 调 `adapter.GetTaskStatus` 更新状态/进度/视频 URL。
- `service/vendor_libtv.go`：`GenerateVideo` 拆为 `SubmitVideo`（提交返回 generateUuid）+ 同步轮询；模型判定改为「模板 UUID / 名称含文生/图生视频」优先，图生视频必须带首帧（缺首帧给明确中文提示）；`GetTaskStatus` 进度兼容 JSON float64；`GenerateImage` 补 prompt 空校验。
- `model/video_task.go` 新增 `vendor_type` 列（AutoMigrate 自动加列，无需手工迁移）。

### 需人工验证
- 用 LibTV 开放平台 AK/SK 绑定账户并激活后，视频工作台选「可灵文生视频」模型 → 提交生成 → 立即返回任务（`vendorType=libtv`），轮询直到 completed 且拿到 `data.videos[].videoUrl` 对应的播放地址。
- 「可灵图生视频」模型带首帧参考图 → 走 img2video；不带首帧 → 明确提示"图生视频需要提供首帧参考图"。
- 文生视频缺参考图 → 走 text2video；`video_generate_audio` 开关能透传到 sound 参数。
- 切换回官方渠道后，视频任务照旧走官方 `/api/v1/videos` 链路（分发不误伤）。
- 未绑定 LibTV 账户/适配器不支持视频的供应商 → 返回清晰中文提示，不回退成官方扣积分请求。
- 真机验证 LibTV 视频契约：提交端点 `/api/generate/video/kling/text2video` / `img2video`、`mode=pro`、kling-v2-6 图生视频用 `images` 数组、`data.videos[].videoUrl` 提取。

## UpDream / NewWow 样本重放增强（多样本回退）

- `service/vendor_replay.go` `GenerateImage` 改为取最近 **5 条**生成样本逐个尝试重放：单条失败（HTTP 错误/响应无图片/无法注入 prompt）自动回退下一条；全部失败汇总"最近 N 条样本均重放失败 + 最后错误"。
- 移除 `GenerateImage` 里仅改内存的 `account.LastUsedAt` 赋值（实际落库由 `handler/vendor_proxy.go` 统一处理）。

### 需人工验证
- 采集多条生成样本（含一条非标准接口的误采样本）后生图：最新样本失败时能自动用更早的标准样本成功出图。
- 所有样本都失败时，错误信息应包含"最近 N 条生成样本均重放失败"和最后一条失败原因。
- 未采集样本 / 未绑定 Cookie 的提示行为与之前一致。

## NewWow 真实接口接入：余额拉取 + 模型列表（不再依赖"auto"占位）

Playwright 嗅探 `neowow.cn` 后确认鉴权走 `accesstoken` HTTP header（不是 cookie），公开响应 envelope `{success, errCode, errMessage, data}`，可用端点：

- `GET /user/profile` → 完整账户 + 积分 + 头像 + 昵称
- `GET /user/points-history/v2` → 最近积分流水（每条带 `imageGenerationParam.model`）
- `GET /agent/membership/current` → 当前会员套餐（如未购买两个字段都 null）
- `GET /agent/user/video/templates` → 11 条视频模板（含 `modelName` / `modelProvider`）
- `GET /agent/image/style/list` → 图片风格列表（每条带 `recommendModel`）

实现要点（`service/vendor_newwow.go`）：

- 新增 `newWowAdapter` **嵌入** `*replayVendorAdapter`（生图仍走样本重放，不依赖真实 modelName），只覆盖 `ListModels` / `GetAccountInfo` 两个方法。
- `ListModels` 调上面两个端点去重整理成 `ImageModels` + `VideoModels`；任何一步失败降级到 `replayVendorAdapter.ListModels` 的"auto"占位，避免偶发不可用时前端下拉空白。
- `fetchNewWowBalanceInto` 调 `/user/profile` + `/user/points-history/v2` + `/agent/membership/current` 三个端点，组装 `BalanceInfoJSON`（含 `credits`/`package`/`recentModel`/`balanceText`，与 `renderBalanceText` 兼容），同一路径同步刷新昵称/头像/vendorUserID，把最近一次 raw 快照存到 `RawExtraJSON.newwow_last_balance` 供排查。
- 同时把 `service/vendor_libtv_task.go` 里的 `fetchNewWowBalanceInto` no-op 占位删掉（实现已迁移到 `vendor_newwow.go`）。

源码 smoke test（用本地后端 18080 跑通）：

- 接口 `POST /api/v1/vendor/bind-cookie` 传 `vendorType=newwow` + `authHeaderName=accesstoken` + `authHeaderValue=<token>` → 200，返回 `displayName="用户0370"`，`balanceText="NewWow 积分 0 · 最近 Seedream 4.0"`，`hasModels=true`。
- `availableModelsJson` 含 4 个真实模型：`gemini-3.1-flash-image-preview`（图片推荐风格）、`MiniMax-Hailuo-02`（MINIMAX）、`doubao-seedance-1-0-pro-250528`(DOUBAO)、`veo3`(FAL)。
- `POST /api/v1/vendor/refresh-balance` 和 `POST /api/v1/vendor/refresh-models` 均正常返回更新后的账户。

### 需人工验证
- 浏览器登录后切到 NewWow 绑定：粘贴 accesstoken 后「我的供应商」卡片显示 `NewWow 积分 X · 最近 Seedream 4.0`（用户实际当前为 0 积分；从积分历史推出来的最近消费模型）。
- 切换手动刷新余额 / 刷新模型按钮：均能在 1–2 秒内拿到最新数据（界面 balanceText 更新、模型下拉非 "auto" 占位）。
- 在生成工作台下拉模型：能看到「gemini-3.1-flash-image-preview · 推荐风格」「MiniMax-Hailuo-02 · MINIMAX」「doubao-seedance-1-0-pro-250528 · DOUBAO」「veo3 · FAL」四个真实模型 ID。
- accesstoken 过期/无效：绑定本身仍能成功（best-effort），但账户卡 `balanceText` 为空、`RawExtraJSON.newwow_last_balance_error` 能在管理员后台查到；下次手动刷新余额时给明确错误提示。
- 管理员后台 `vendors` 表里给 NewWow 这一行配置 `api_root_url=https://neowow.cn`（或保持默认）→ 校验生效（覆盖代码内 `newWowDefaultAPIRoot`）。



## 生图 / 视频工作台 @ 分组弹层（选择参考素材）

- `web/src/app/(user)/components/mention-textarea.tsx` 新增扩展能力：可传入 `promptItems`（预设提示词）/ `recentGenerated`（最近生成图）/ `assetGroups`（素材库分组）；新增 `showAtButton` 在输入框左侧渲染独立 `@` 按钮，点击后在按钮旁打开分组弹层（选择预设提示词 → 最近生成 → 素材库 人物/场景/物品/风格/其它 二级分组，点击分组展开缩略图网格，可返回）。
- 原「输入 `@` 触发缩略图网格」交互与 `HighlightText` 标签高亮完全保留；未传扩展数据的场景（如画布节点）行为不变。
- 新增 `web/src/lib/asset-groups.ts`：按素材标题/标签/来源推断归属人物/场景/物品/风格/其它，`groupAssetsByCategory` 只保留非文本资产。
- 生图工作台（`image/page.tsx`，侧边 + 底部布局）：接入 `@` 按钮分组弹层，素材库/最近生成图选中后落库为参考图并插入「图片N」标签；预设提示词选中直接替换输入（数据源 `use-prompt-list` 前 6 条）。
- 视频创作台（`video/page.tsx`，侧边 + 底部布局）：同上；素材库图片/视频/音频分别落库为参考图/参考视频/参考音频并插入对应「图片N/视频N/音频N」标签（最近生成区暂不展示，视频无封面）。
- `MentionCandidate` 扩展可选字段 `asset` / `storageKey` / `mimeType`；`onSelectAsset` 回调返回真实标签，返回 `null` 取消插入。

### 需人工验证
- 生图工作台：点击输入框左侧 `@` → 弹层显示「选择预设提示词 / 最近生成 / 素材库（5 分组）」；点分组进入缩略图网格，点图后参考图出现在底部参考图条、输入框光标处插入「图片N」。
- 选中素材后输入框左侧 `@` 保持可用；再次点击弹层正常开关；点空白处/切走页面弹层关闭。
- 生图工作台预设提示词：点击后输入框内容被替换为该提示词。
- 视频创作台：素材库图片/视频/音频点击后分别加入参考图/参考视频/参考音频，输入框插入对应标签；生成后请求包含对应参考。
- 视频/音频素材在生图工作台点击 → 提示「视频/音频素材不能作为生图参考图」，不插入标签。
- 未上传任何素材、素材库为空时，`@` 按钮仍可打开弹层（有预设提示词）；三者皆空时不显示 `@` 按钮。
- 浅色/深色主题下弹层样式正常（跟随现有 stone 主题，未硬编码主题色）。
- 在输入框中输入 `@`：弹出的缩略图网格应**紧贴光标右侧**（在 `@` 字符后面），而不是跑到下方重叠参数行 / 卡片底部。

## @ 弹层位置修复（视频/生图工作台 textarea + 画布节点）

`web/src/app/(user)/components/mention-textarea.tsx` 和 `web/src/app/(user)/canvas/components/canvas-resource-mention-textarea.tsx`：用于计算 textarea 光标坐标的镜像 div（`mirrorRef`）原本渲染在各自容器内部，依赖 `position: fixed` / `absolute` + 视口坐标对齐。视频工作台底部浮窗卡片有 `backdrop-blur-2xl`，按 CSS 规范 `backdrop-filter` 会成为 `position: fixed` 元素的 containing block，镜像的位置 = `card.left + viewportX`，导致 `caretCoords` 整体偏移，`MentionMenu` 跟着跑到模型/清晰度/尺寸行附近；画布节点那边 `position: absolute` 同样会被祖先 `transform` / `position: relative` 干扰。修复：把两处镜像 div 都用 `createPortal` 挂到 `document.body` 上，定位回到视口，弹层回到光标右下方 / `@` 字符右侧。

### 需人工验证
- 视频工作台底部布局（浮窗卡片有 backdrop-blur）：输入框敲 `@` → 弹层紧贴光标右侧（紧跟 `@` 字符后），与上方输入框对齐、不再压到模型/清晰度/尺寸行。
- 生图工作台（侧边 + 底部布局）输入框敲 `@`：同样紧贴光标右侧。
- 画布里打开文本节点，输入 `@` 选择参考素材：弹层紧贴 `@` 字符、不再被画布变换/缩放影响跑位。
- 多次敲不同位置的 `@`（首行/换行后）弹层都能跟随到当前光标位置。
- 弹层上方空间不足时仍能正确翻到光标上方显示（不再卡在卡片底部）。
- 未登录/未传 `promptItems`/`assetGroups` 等场景，输入 `@` 触发的缩略图网格位置行为不受影响。

## 分镜剧本与分镜视频批量删除

- `novel/page.tsx` 新增 `batchDeleteStoryboards`（按 `selectedStoryboardIds` 批量删分镜剧本，带确认框）、`batchDeleteShots`（按 `selected` 批量删分镜视频，带确认框）、`deleteShot`（单个删分镜视频，复用到详情弹窗）。
- 分镜剧本顶部工具栏「全选」旁新增🗑批量删除按钮（未勾选时禁用）。
- 分镜视频顶部工具栏「生成选中」旁新增🗑批量删除按钮（未勾选时禁用）。
- `VideoShotCard` 新增 `onDelete` prop：紧凑模式左下角 hover 显示删除按钮（生成中隐藏避免与进度角标重叠）；标准模式底部信息行新增删除按钮。

### 需人工验证
- 分镜剧本勾选多条 → 点🗑 → 确认后批量删除，查看弹窗自动关闭（若查看的是被删分镜）。
- 分镜剧本未勾选时🗑按钮置灰不可点。
- 分镜视频勾选多个 → 点🗑 → 确认后批量删除，详情弹窗若打开的是被删视频则自动关闭。
- 分镜视频卡片 hover 显示单个删除按钮（紧凑/标准两种模式），点击后单独删除。
- 生成中的视频卡片不显示单个删除按钮（避免误删）。

## 分镜生成后端任务化（刷新恢复进度）

- 后端新增 `storyboard_tasks` 表与 worker，逐章调文本模型生成分镜剧本；前端 `handleParseStoryboards` 改为提交后端任务 + 3 秒轮询。
- 前端 `novel/page.tsx` 新增 `pollStoryboardTask`、`handleParseStoryboardsLocal`（回退）、刷新恢复 effect（据 `storyboardTaskId` 自动恢复轮询）。
- 新增 `services/api/storyboard_task.ts` API 客户端。
- `NovelProject` 新增 `storyboardTaskId`、`storyboardGroupMap` 字段（localforage 持久化）。

### 需人工验证
- 登录后导入小说 → 勾选部分章节 → 点「开始分镜」→ 右侧逐章出现分镜剧本，进度条实时更新。

## 分镜刷新恢复：4 段 stepper + 底部状态文案与真实进度对齐

`web/src/app/(user)/novel/page.tsx` 修复"页面切换回来后，4 段流水线 stepper 一直停在「章节解析」active，底部文案一直挂着「正在恢复分镜任务进度…」"的问题——根因是刷新恢复 effect 只设了 `pipelineStatus` 静态文案 + `parsingStoryboard=true`，没把 `pipelinePhase` 切到 `storyboard`，`pollStoryboardTask` 内部也从不更新状态条文案。

**改动 2 处**：

- 恢复 effect（line 2616-2636）：进入恢复分支时同步 `setPipelinePhase("storyboard")` + `setPipelineProgress({ current: 0, total: groupMap.length })`，让 4 段 stepper 一开始就高亮「分镜剧本」步骤并显示初始 `0/N` 计数。
- `pollStoryboardTask` 每次成功拉取后（line 1466-1474）：用真实进度覆写状态条——`status="queued"` 时显示「分镜任务已提交，后端排队中（共 N 章）…」、`running/completed` 时显示「后端正在生成分镜剧本（共 N 章，已完成 X 章）…」，3 秒一次与轮询同步推进，底部状态条不再"卡死在恢复文案"。

### 需人工验证
- 登录用户开一个分镜任务 → 切到「生图创作台」(`/canvas`) 几秒后切回 novel 页：
  - 4 段 stepper 高亮的应是「分镜剧本」步骤（带 ring + 进度数字 `已完成/总数`），而不是「章节解析」。
  - 底部状态条与顶部「剧本转视频」右侧的 spinner 文案 3 秒一刷，进度数字与后端实际 `doneCount/totalCount` 一致（不再是"正在恢复分镜任务进度…"）。
  - 章节解析、资产生成、视频生成三个步骤仍显示「待启动」（虚线边框 + 浅灰字），不动。
- 任务还在 `queued` 阶段时切走再回来 → 底部文案是「分镜任务已提交，后端排队中（共 N 章）…」，不是「后端正在生成…」。
- 任务完成后切回 → stepper 自动收起（4 段都不再 active），底部状态条清空，与既有「completed/canceled/failed」终态分支行为一致。
- 「停止」按钮照旧能 abort 轮询（恢复 effect 的 cleanup 仍调 `controller.abort()`，未改）。

## 供应商模型列表统一过滤（去掉音频 + 编辑/后处理工具）

用户确认：项目不需要音频模型；三家供应商（libtv / updream / newwow）下拉都只保留「可生成」的模型，去掉超分/去字幕/抠像/运镜控制/导演类等非生成工具。

改动：
- `service/vendor_adapter.go` 新增统一过滤：`vendorModelHiddenPatterns` + `filterVendorModels` / `filterHiddenModels` / `isVendorModelHidden`。
  - `AudioModels` 整体置 nil（项目不需要音频）。
  - 对 image/video/text 每个模型，按 `id + " " + name` 小写子串匹配隐藏规则：`upscaler` `upscaling` `subtitle-eraser` `portrait-matting` `matting` `motion-control` `image-editor` `motion-3` `scene-2` `omnihuman` `midjourney-video` `kling-multi-shot`。
- `service/vendor_dispatch.go` 的 `FetchAndStoreVendorModels` 在 `adapter.ListModels` 返回后、写 `AvailableModelsJSON` 前调 `filterVendorModels(models)`——这是唯一落库 chokepoint，三家供应商全生效（含手动「刷新模型」走的同路径）。
- `service/vendor_libtv_task.go` 的 `libtvHardcodedModels()` 兜底里把无效 key `seedream_hd4k`（live 无此 key）改为 `seedream-4`（合法 live key）。

实测（用用户 token 直连 liblib.tv 主页 RSC + benefit/list 模拟过滤）：
- LibTV 过滤前 image 35 / video 40 / text 10 / audio 8 = 93；过滤后 image 31 / video 24 / text 10 / audio 0 = 65。
- 被过滤：image 4（topaz-image-upscaler、hd-upscaling、image-editor、image-editor-pro）、video 16（kling-multi-shot、*-video-upscaler×2、subtitle-eraser、portrait-matting、motion-control×2、motion-3.*×4、scene-2*、omnihuman-1.5、midjourney-video）、audio 8。
- 注意：`kling-video-o1/o3/enhance`、`kling-v3-omni`、`qwen-edit`、`multiple-angles`、`seed-evolving` 等未列入隐藏规则，仍保留在列表——若后续确认是编辑/非生成类可再加规则。

### 需人工验证
- 绑定/刷新 libtv、updream、newwow 任一供应商后，前端模型下拉不应出现任何音频模型，也不应出现超分/去字幕/抠像/运镜控制类工具。
- libtv 视频下拉只剩真实生成模型（kling-v3-turbo、viduq3-pro、MiniMax-Hailuo-H3、pixverse、seedance 等）。
- updream 下拉去掉 audio/music 两组（原来 `/api/ai/audio-models` `/api/ai/music-models` 拉到的全被过滤）。
- newwow 本来只有 image/video，不受影响（过滤规则在其上零命中，除保险外无变化）。
- 分镜生成中刷新页面 → 自动恢复轮询，已产出分镜仍在，剩余章节继续生成。
- 分镜生成中点「停止」→ 轮询停止，状态清空。
- 未登录用户点「开始分镜」→ 自动回退到前端直连模式（提示「登录后可使用后端分镜任务」），分镜正常产出（无刷新恢复）。
- 后端任务提交失败（如后端未启动）→ 自动回退到前端直连模式。
- 开启 autoPilot → 分镜逐章产出后自动触发视频生成。
- 任务失败（如文本模型不可用）→ 显示失败信息，失败章节标 ⚠ 占位。
- 任务完成后 → 项目中 `storyboardTaskId` 清除，可再次提交新任务。

## 工作台参考图/输入跨页面持久化

- 新增 `web/src/stores/use-workbench-store.ts`：用 Zustand `persist` + `localForageStorage` 保存生图/视频工作台的当前输入状态。
- 生图工作台的 `prompt`、`references`（参考图/人物）改为从工作台 store 读写（`web/src/app/(user)/image/page.tsx`）。
- 视频创作台的 `prompt`、`negativePrompt`、`references`、首帧/尾帧、参考视频、参考音频改为从工作台 store 读写（`web/src/app/(user)/video/page.tsx`）。
- 「新建会话」清空逻辑同步清空持久化 store，行为保持一致。

### 需人工验证
- 在生图工作台添加参考图（人物）→ 切换到视频创作台 → 返回生图工作台，参考图与已输入的提示词仍在。
- 在视频创作台设置参考图/首帧/尾帧/参考视频/参考音频与提示词 → 切到生图工作台再返回，内容仍在。
- 刷新页面后上述工作台输入仍能恢复。
- 点击「新建会话」后工作台输入被正确清空。

## 用户可自定义模型渠道（文本 / 图片 / 视频）

- 后端不再强制关闭 `allowCustomChannel`：`normalizePublicSettingWithChannels` 中该字段为 nil 时默认 `true`，`SaveSettings` 不再二次写死 false，管理员可在后台真正开关（`service/settings.go`）。
- 管理后台设置页「是否允许用户自定义渠道」表单初始值改为 `true`（`web/src/app/(admin)/admin/settings/page.tsx`）。
- 配置弹窗放开：`allowCustomChannel === true` 且用户已登录时，显示「渠道模式」切换、本地渠道（新增渠道、Base URL、API Key、拉取模型）、模型列表；用户可为文本/图片/视频分别选自己渠道的模型（`web/src/components/layout/app-config-modal.tsx`）。
- 游客（未登录）不显示自定义渠道 UI，仅使用云端默认模型；系统提示词输入框仍仅管理员可见。

### 需人工验证
- 管理后台关闭「允许用户自定义渠道」后，普通用户配置弹窗应看不到本地渠道 UI，被强制走云端渠道。
- 管理后台开启该开关后，登录的普通用户可在弹窗填写 Base URL / API Key、拉取模型、选文本/图片/视频模型，并能实际生成。
- 未登录游客始终只能用云端默认模型，看不到自定义渠道入口。

## 画布创作 Agent 收起对话不再中断任务

- 收起对话面板不再卸载 `CanvasAssistantPanel`，改为保持挂载并用收起动画隐藏（`web/src/app/(user)/canvas/[id]/canvas-client-page.tsx`）。
- 面板新增 `collapsed` prop，收起动画由父级 `agentPanel.open` 驱动，移除内部一次性 `closing` 状态，重新展开可恢复（`web/src/app/(user)/canvas/components/canvas-assistant-panel.tsx`）。
- `onCollapse` 只切换 `agentPanel.open`，不再 `setAssistantMounted(false)`，避免组件卸载触发 AbortController abort。
- 组件卸载时 abort 未完成任务的逻辑保留，现在仅在真正离开画布页面时触发。

### 需人工验证
- 发起一个 Agent 任务（如"创建一个图片节点"）并在执行中点击「收起对话」，任务应在后台继续执行，不再出现"已停止继续执行"。
- 任务执行中收起再展开，能看到进度延续并最终得到结果。
- 直接离开画布页面时，未完成的 Agent 任务应被正常中止。

## 浏览器插件嗅探 UpDream / NewWow 生成样本（capture-sample）

- 插件 `vendor-browser-extension/` 升级到 v1.1.0：`content.js` 在 UpDream / NewWow 官网 monkey-patch `fetch` / `XMLHttpRequest`，捕获 POST/PUT/PATCH 的生成类请求（排除 login/auth/password 等敏感接口），经 `background.js` 存 `chrome.storage.local` 并按 url+method+body 去重。
- 插件弹窗新增「连接设置」（填写无限画布 Web 地址 + 登录 Token）与「生成样本嗅探」（各供应商样本数、推送/清空按钮、推送结果展示）。
- `background.js` 新增 `captureSample` / `getSamples` / `clearSamples` / `pushSamplesToBackend` / `submitToProject`（顺带修复气泡「绑定」按钮直接提交 Cookie）/`getSettings` / `setSettings`。
- 后端新增 `vendor_api_samples` 表与 `POST /api/v1/vendor/capture-sample`、`GET /api/v1/vendor/samples`、`POST /api/v1/vendor/clear-samples`；样本落库要求用户已绑定对应供应商账户。

### 需人工验证
- 在 Chrome 加载插件（开发者模式 → 加载已解压的扩展，指向 `vendor-browser-extension/`）。
- 插件弹窗「连接设置」填无限画布 Web 地址（与前端打开地址一致，如 http://localhost:3000）和登录 Token（从无限画布设置页复制）。
- 在 UpDream / NewWow 官网登录后生成一张图片；回到插件弹窗「生成样本嗅探」应能看到该供应商出现样本数（生成类计数 > 0）。
- 点「推送」→ 结果提示成功推送 N 条；后端 `GET /api/v1/vendor/samples?vendorType=updream` 能查到样本，且 `isLikelyGeneration=true`、`endpointGroup` 已归一化。
- 「清空」后样本数归零、后端也查不到。
- 在官网点插件气泡「绑定到我的无限画布」能直接把 Cookie 提交绑定（替代手动复制粘贴）。

## UpDream / NewWow 样本重放生图（GenerateImage 消费样本）

- 新增通用基类 `service/vendor_replay.go`（`replayVendorAdapter`，完整实现 VendorAdapter 接口），`service/vendor_updream.go` / `service/vendor_newwow.go` 各自 `init()` 注册（仅传不同显示名）。
- `GenerateImage` 读取该用户最新一条生成类样本（`ListVendorApiSamples(userID, vendorType, onlyGeneration=true, 1)`），把本次 `prompt` 注入样本请求体（JSON 深度优先替换 prompt 候选键 / 表单按候选键 Set），用绑定账户的 Cookie 覆盖请求头 `Cookie`，经 `SafeProxyHTTPClient()`（SSRF 屏蔽）重放。
- 相对 URL 用捕获请求头的 `Referer` / `Origin` 推导成绝对 URL；响应按已知图片键 / 全树扫描 / 正则兜底提取图片直链，映射成 OpenAI 兼容 `{created, data:[{url}]}`。
- 前端分发链路复用 `handler/vendor_proxy.go` 的 `dispatchVendorProxy`（已是 OpenAI 兼容输出）。

### 需人工验证
- 在 UpDream / NewWow 官网登录后，先用插件采集一次真实生图请求并推送到后端（见上一项），确认 `GET /api/v1/vendor/samples?vendorType=updream&onlyGeneration=true` 至少 1 条。
- 无限画布里把该供应商切为「激活」（app-config-modal 供应商 Tab），用插件粘贴的 Cookie 绑定账户。
- 切到对应生图工作台、输入 prompt、点生成 → 后端应带该用户 Cookie 重放官网样本请求，返回图片并显示在画布节点。
- 未采集样本就点生成 → 应提示「还没有可用的生成样本，请先采集一次」。
- 未绑定 Cookie（或 Cookie 失效）就点生成 → 应提示「未绑定 Cookie / Cookie 校验失败」。
- 样本请求体是 JSON 且包含 `prompt` 字段时能正确替换；表单类型同理。
- 输入与样本负向提示键（`negative_prompt`）都存在时能一并替换。
- 该供应商视频/音频生成点不动 → 应提示「暂仅支持图片生图（样本重放）」。
- 已知局限：仅支持同步返回图片直链、JSON/表单请求体；异步生成（先返任务 ID）与 multipart（图生图上传）暂不支持，会给出明确中文提示。

## LibTV 视频生成适配器（GenerateVideo / 视频模型）

- `service/vendor_libtv.go` 已实现 `GenerateVideo`（Kling 文生/图生视频，提交 + 轮询 + 提取 `data.videos[].videoUrl`）、`ListModels` 新增两个视频模型（「可灵文生视频」「可灵图生视频」）、`GetTaskStatus` 支持视频结果提取。
- 视频契约（提交端点 `/api/generate/video/kling/text2video` / `img2video`、`mode` 必填 `pro`、kling-v2-6 图生视频用 `images` 数组）来自社区 SDK 与 godeps/aigo 引擎库，**未经真机验证**。
- 修复 `service/vendor.go` 里 LibTV Cookie 校验地址从旧 `www.liblib.tv/api/user/profile`（已 404）同步为 `api2.liblib.art/api/www/activity/userInfo`（POST），并给 `vendorCookieVerifySpec` 增加 `Method` 字段。

### 需人工验证
- 用 LibTV 开放平台 AK/SK 绑定账户后，`GET /api/vendor/.../models`（或绑定返回的模型快照）应能看到两个视频模型。
- （待接分发）当前视频生成仍走官方渠道 `/api/v1/videos`，尚未接入供应商分发——本轮仅完成适配器层能力，端到端视频生成需下一步接入 `proxyAIVideoTaskRequest` 分发后再验。
- 若直接调 `adapter.GenerateVideo`（AK/SK 有效时）应能提交 Kling 视频任务并轮询拿到 `data.videos[].videoUrl`；文生视频缺参考图走 text2video，带首帧参考图走 img2video。

## Agnes 图片模型请求路由修复（避开上游 /responses float(''））

- `web/src/services/api/image.ts` 的 `requestImages` 重构为统一 `useSimpleEndpoint` 判断：（供应商非官方模式）**或** `isAgnesImageModel(config.model)` 时走 `requestImageGenerationSingle`（已含 `applyAgnesImageSize` 处理 size/ratio 映射），其他情况继续走 `requestResponsesSingle`。
- 触发原因：今日 `data/logs/ai-calls/ai-calls-2026-08-16.log` 12:36–12:37 连发 10+ 个角色/场景生图全部 500，上游错误 `could not convert string to float: ''`，根因是 Agnes 模型在 `/responses` 端点上游 Python 服务解析失败。原先注释「统一走 Responses API 格式：不再区分 apiMode / Agnes 专属分支」在该端点不再适用。

### 需人工验证
- 剧本转小说页登录后激活 `agnes-image-2.0-flash`（或同系列 2.0/2.1）模型，生成一张角色参考图：HTTP 请求应指向 `/images/generations` 而非 `/responses`；请求体里 `size=3840x2160` 直接透传（2.0）或 `size=4K & ratio=16:9` 转换（2.1），不再带 `tools` / `tool_choice` 字段。
- 同提示词生成多张（n>1）仍然并发单发，不出现 `500 / could not convert string to float`。
- 非 Agnes 模型（如 Gemini/Nano Banana/GPT-image 系列）默认 `/responses` 行为不变。
- 切回供应商模式行为不变：仍走 `/images/generations`，由后端 vendor dispatch 接管。

## 分镜剧本视频生成：警告收敛 + 顺序模式强制衔接

- `web/src/app/(user)/novel/page.tsx` `generateVideoFromStoryboard`：未匹配提醒改为只针对"出场角色"行（`extractStoryboardHeader` 拆出的 `characters`），场景/道具未匹配一律静默（依旧进 `referencedAssetIds` 但不出现在警告里）；提示文案同步收紧到「请在左侧「人物」上传」。
- `buildVideoInput` 衔接逻辑改为：`useConsecutive = enableConsecutiveFrames || !generateInParallel`。
  - 顺序生成（`!generateInParallel`）：**仅**取紧邻的上一条 shot 作为尾帧参考；上一条若 `status !== "success"`，**直接跳过、不再回退找更早成功的**（命中"上一条失败 → 不要尾帧"的需求）。
  - 并行生成：保留原行为（取最近一条已成功的视频）。
- 配置弹窗 UI：「衔接模式」Select 在顺序生成时置灰（`disabled={!generateInParallel}`）、值强制为「自动衔接」；顺序模式下额外显示提示「顺序生成强制衔接：上一条视频的尾帧作为下一条参考图（若上一条失败则跳过）」。

### 需人工验证
- 分镜剧本头部含 `场景：医院源头；除仪馆` 但素材库无对应别名时，触发视频生成不再弹出 `@医院源头 / @除仪馆 未在素材库找到` 的警告；仅当 `出场角色` 行未匹配（如 `@沈一心`）才弹警告，且文案里提示改为「请在左侧「人物」上传」。
- 分镜剧本头部同时含 `出场角色：沈一心`（已匹配）与 `场景：医院源头`（未匹配）时，警告只列人物，场景依旧进 `referencedAssetIds` 静默通过。
- 配置选择「顺序生成」时，「衔接模式」控件置灰、值恒为「自动衔接」；下方出现顺序生成强制衔接提示文案。
- 顺序生成时，若紧邻上一条 shot 生成失败（status=failed），当前 shot 不再以更早的成功 shot 作为尾帧，仅按 `referencedAssetIds` / 自定义首尾帧参考图提交。
- 顺序生成时，紧邻上一条 shot 已成功（status=success 且有 videoUrl），下一条会把它作为尾帧传入；如模型不支持首尾帧（`supportsFrameRefs=false`），降级为参考图方式（与原行为一致）。
- 切到「并行生成」后，衔接模式 Select 可再次编辑；并行模式沿用原"取最近一条成功"的尾帧回退行为。

## 画布桌宠可见性 / 位置持久化

- `web/src/stores/use-pet-settings.ts` 类型与状态加 `side`、`yRatio`（归一化 0–1）字段，新增 `setSide` / `setYRatio`，加入 `partialize` 持久化；`resetPosition` 同时把 `side="right"`、`yRatio=0.6` 写回 store。
- `web/src/app/(user)/canvas/components/canvas-agent-pet.tsx`：
  - 桌宠 z-index 从 `z-40` 提高到 `z-[150]`，浮在顶部 / 底部工具栏（z-50 / z-[100]）之上。
  - `EDGE_PADDING` 由 20 调到 32，离屏幕边更远，不再贴边。
  - 引入 `DEFAULT_Y_RATIO=0.6` 与 `computeY` helper：默认纵向落在可视高度的 60%，窗口尺寸变化时按比例自适应。
  - 删除本地 `side` / `y` state 和 `onResize` 校准；位置直接读 `petSettings.side` / `petSettings.yRatio`，拖动期间用本地 `dragY`，`onPointerUp` 时统一调用 `setSide` + `setYRatio` 写回 store。
  - 删掉文件尾部的 `initialY` 函数（已被 `computeY` 替代）。

### 需人工验证
- 进入画布 → 桌宠默认出现在视口垂直方向约 60% 处（屏幕中下偏中），不再贴右下角。
- 拖到屏幕左侧 / 右侧 → 松手后吸附到对应侧，刷新或重开画布后位置仍保留。
- 上下拖到非默认位置 → 刷新后位置保留，且不会因为窗口高度变了跑到屏幕外。
- 调整浏览器窗口大小（拉高 / 缩短）→ 桌宠按比例保持在视口 60% 位置附近（不再贴底）。
- 点击桌宠仍能打开助手面板；拖到顶部 / 底部工具栏附近不再被压在工具栏之下，气泡提示仍正常。
- 点桌宠设置里的「重置位置」→ 桌宠回到右侧、视口 60% 处。

## 剧本转视频：一键出片开关合并 + 技术开关下沉

`web/src/app/(user)/novel/page.tsx` 把"开几个开关才能跑一键生成"的 UX 收敛成「**唯一开关 = 一键出片**」：

- 删 `autoGenerateAssets` state（与「图片(开)」按钮同步删除），对应顶部「图片(开)」按钮整块删掉。`autoGenerateAssets` 在 `pollStoryboardTask` completed 分支（后端任务版）与 `Promise.all(workers)` 末尾（前端直连版）两处条件改成读 `autoPilot`，所以一键出片 on 时一条龙就是「分镜剧本 → 自动抽角色/场景/道具并生图 → 自动出片」；off 时只出分镜剧本。
- 删 `enableConsecutiveFrames` 与 `videoReferenceCharactersOnly` 两个 state，及对应配置弹窗「衔接模式」「视频参考图范围」两行 UI。`buildVideoInput` 的 `useConsecutive` 直接由 `!generateInParallel` 推导；`generateVideoFromStoryboard` / `handleAutoMatch` / `buildVideoInput` 里相关的 `restrictToCharacters` / `if (videoReferenceCharactersOnly)` 分支全部去掉，恢复「能带的全带上」单一行为（原本就是默认行为）。配置弹窗里合并后的两条警告提示保留：「顺序生成强制衔接」+「顺序生成且当前视频模型不支持首尾帧时，衔接仅作为参考图使用」。
- 底部 footer 「自动化」Switch 改名为「一键出片」，`checkedChildren/unCheckedChildren` 从「自动/手动」改为「开/关」，Tooltip 文案同步描述「一键出片 / 手动」语义。
- 顶部「开始分镜」按钮文字在 `autoPilot=true` 时由 `开始分镜(自动出片)` 改为 `一键出片`，Tooltip 同步描述合并后的行为。

### 需人工验证
- 顶部「原始小说」标题栏右侧只剩 [导入] [开始分镜 · 3回] / [一键出片 · 3回] 两个按钮，没有「图片(开)」按钮。
- 一键出片 = 关：点「开始分镜」只产出分镜剧本到右侧列表（生图与出片需手动）。
- 一键出片 = 开：点「一键出片」→ 分镜剧本逐章产出 → 完成后立即出现「正在从分镜剧本中提取并生成分镜图片...」进度 → 完成后立即自动为每条分镜跑视频生成，右侧分镜剧本卡片状态依次进入「生成中 → 已出片」，全程不需要手动操作。
- 顶部「开始分镜」按钮文字随一键出片状态切换：开 = 「一键出片 · N组」、关 = 「开始分镜 · N组」，Tooltip 文案也对应变化。
- 底部 footer：只剩一个 Switch，标签「一键出片」，开/关子文案显示「开/关」；状态与顶部按钮文字一致（同一 state 双向绑定）。
- 配置弹窗：「分镜并发数 / 视频并发数 / 每段时长 / 画面比例 / 分辨率 / 生成模式」六行还在；「视频参考图范围」「衔接模式」两行已移除。
- 配置弹窗里两条警告文案仍在：选「顺序生成」时显示「顺序生成强制衔接：上一条视频的尾帧作为下一条参考图（若上一条失败则跳过）」；选「顺序生成」且当前视频模型不支持首尾帧时显示「顺序生成强制衔接，但当前视频模型不支持首尾帧参考，衔接仅作为参考图使用」。
- 视频请求 `extra_body.image`：分镜剧本头部「出场角色：沈一言；场景：教室」时 → 沈一言 + 教室两张都进（不再有 `restrictToCharacters` 过滤逻辑），未匹配的角色仍按既有逻辑弹警告、场景未匹配静默通过。
- 顺序生成：紧邻上一条 shot 失败（status=failed）→ 当前 shot 不带尾帧参考；上一条成功 → 把它作为尾帧传入。
- 并行生成：shot 之间不再尝试「最近一条成功」尾帧衔接，每条独立生成（去掉原并行分支 `for (let i = ...) prev = p` 找最近一条成功的循环）。
- 一键出片 + 全空项目（没有任何出厂角色）：分镜剧本产出后仍能从分镜里抽出新角色并生图，行为与改前一致。
- 任务化版（登录用户）→ 后端任务 completed 后才进入「自动抽角色/场景/道具并生图」阶段；任务 failed 不触发。

## 画布桌宠 sprite 渲染缩放（修「半个头」）

用户上一轮改完桌宠位置后反馈"只看到半个头、一个手指"。读 `web/src/app/(user)/canvas/components/canvas-agent-character.tsx` 找到根因：`background-size` 直接写 `${sheet.width}px ${sheet.height}px`（3072×2080），CSS 按绝对像素 1:1 渲染，容器只有 66×72，等于只显示 cell（192×208）左上角一小片。用 sharp 解 frame 0 bounding box = `(43,16)–(149,194)`，角色居中绘制，所以视觉上只能看到 `(43,16)–(66,72)` 这一小块——"半个头 + 一截肩膀"。

修复：引入 `cellScale = size / grid.cellH`，把 `background-size` 和 `background-position` 都按这个比例缩放（3082→1063、2080→720），让单个 cell 等比铺满容器。

### 需人工验证
- 进入画布，桌宠默认显示完整的小怪兽角色（不再是半个头）。
- 切换状态（待机 / 睡眠 / 贴边右 / 拖拽 / 点击）：每一帧 sprite 都完整显示。
- 在桌宠设置里调大小滑杆（56–120）：整个角色仍完整可见，不被 cell 边界截掉。
- 桌宠设置弹窗里的预览区（也是这个组件）现在能看到完整角色。

## 画布桌宠：气泡提示「全局」撤回（仍仅在画布显示）

用户上一轮短暂启用过桌宠全站常驻（layout 挂载、`bubbleScope` 持久化字段、`pendingOpenAssistant` 跨页面 flag、`pet:open-assistant` window event）让画布外的页面也能看到桌宠和冒泡。本轮主动回退——节点完成 / 任务播报 / 生成陪伴事件触发源都在画布页 useEffect，layout 上的桌宠虽然订阅同一个 store，但**离开画布后 trigger 都不跑**，只剩"画布外能看到但不会冒泡"的半成品体验，对用户没价值。

**撤回改动**（全部回滚到本轮第一阶段的位置）：
- `web/src/stores/use-pet-settings.ts`：移除 `bubbleScope` / `setBubbleScope` / `pendingOpenAssistant` / `setPendingOpenAssistant`，`partialize` 不再包含 `bubbleScope`。
- 删除 `web/src/app/(user)/components/global-desktop-pet.tsx`。
- `web/src/app/(user)/layout.tsx`：移除末尾的 `<GlobalDesktopPet />` 与对应 import。
- `web/src/app/(user)/canvas/[id]/canvas-client-page.tsx`：恢复 `<CanvasAgentPet>` 渲染与对应 import；删除之前加的 `pendingOpenAssistant` / window event 桥接 effect；onOpenAssistant 回调直接调 `setAgentPanel({open:true})`。
- `web/src/app/(user)/canvas/components/canvas-agent-settings.tsx`：「气泡提示」Segmented 恢复为 `value="canvas"` + global 项 `disabled: true` + 副标题「目前只支持画布中」。

**当前状态**：桌宠只挂在画布页面（行为与第一阶段完全一致）。

### 未来若要重新做"全局"必须解决的依赖
- 节点完成 / 任务 / 陪伴事件触发点要从画布 useEffect 提到全局 store 或长期轮询，否则画布外的桌宠只是装饰。
- 想做"画布外点击桌宠跳进画布并开助手"，要么把"最近一个项目"存到全局（`useCanvasStore` 已经跨页面持久），要么用专门的"最近活跃项目" store。
- 全局桌宠的 `onOpenAssistant` 与画布页助手面板 state 解耦方式（window event vs store flag）已经验证可行，重做时复用。

## 用户自定义桌宠（上传自己的图片）

`bubbleScope` 全局撤回后，桌宠只挂在画布页面；这一段把"Neowow 一个内置角色扩成"Neowow + 用户上传自定义图片"。存储走项目已有 `uploadAssetMediaFile` → `POST /api/v1/files` 上传到 MinIO / 登录用户同步到账号（未登录 fallback localforage）。

**改动**：
- `web/src/stores/use-pet-settings.ts`：
  - 加 `UploadedPetMeta { id, name, url, storageKey, bytes, mimeType, width, height, createdAt }`
  - 加 `uploadedPets: UploadedPetMeta[]` 持久化字段、`addUploadedPet(meta)` / `removeUploadedPet(id)`
  - `character` 类型从 `"neowow"` 扩到 `"neowow" | `uploaded-${string}``，删除时如果当前选的是被删角色，自动回退 `"neowow"`
  - `removeUploadedPet` 内部 `import("@/services/file-storage")` 动态加载调 `deleteStoredMedia([storageKey])`，断网 / 后端 404 也清本地引用保证 UI 同步
- `web/src/app/(user)/canvas/agent/pet-characters.ts`：加 `makeStaticPetCharacter(id, name, url, w, h)` helper——把单张图包成 `1×1` grid + 单帧"待机"的 `PetCharacter`，复用现有 `CanvasAgentCharacter` 渲染器（cellScale=1 整张图填满容器，不用动 sprite 渲染代码；所有交互状态都映射到同一帧静态图）
- `web/src/app/(user)/canvas/components/canvas-agent-settings.tsx`：
  - Select 选项合并 Neowow + `uploaded-*`（标 📎）；预览区按当前选中渲染
  - Row「桌宠」下加 antd `Upload accept="image/*" maxCount=1 beforeUpload` 按钮"上传桌宠图片"
  - 上传流程：`uploadAssetMediaFile` → `new Image()` 测 `naturalWidth/Height`（失败回退 64×64）→ `addUploadedPet` → 自动 `setCharacter(uploaded-<id>)`
  - 下半区显示「我的桌宠（N）」列表，每项点击切到该角色，右侧 × 调 `Modal.confirm` 二次确认 → `removeUploadedPet`
- `web/src/app/(user)/canvas/components/canvas-agent-pet.tsx`：桌宠本体查 character 时新增 `uploaded-<id>` 分支，用 `petSettings.uploadedPets.find(...)` + `makeStaticPetCharacter` 实时构造。

### 需人工验证
- 进桌宠设置 → 「桌宠」Row 右侧 Select 显示「Neowow」一项，下方「上传桌宠图片」按钮。
- 选一张 png/jpg/WebP 上传 → toast "已添加桌宠「xxx」"、Select 自动切到刚上传的条目、预览区立刻显示这张图、保存到 store。
- 列表区「我的桌宠（1）」多一项，文件名（去扩展名）作为默认名；点击项切到该角色，右侧 × 弹确认框 → 确认 → 后端 file 删除 + store 同步清空。
- 删除当前选中的自定义角色 → 自动回退到 Neowow，预览/画布里立即变回 Neowow。
- 桌面本体（画布里右下角）跟着 Select 切换显示不同角色；静态图无动画但点击仍打开助手面板（事件交互沿用现有的桌宠状态机）。
- 刷新页面 → 上传的角色仍在 Select 列表里，画布渲染保留。
- 登录用户上传：换设备登录后看得到自己上传的角色（通过 MinIO `/api/v1/files`）；未登录用户上传：换浏览器 / 清缓存后丢失（localforage），行为与项目其它图片上传一致。
- 上传极宽图（>3:1）或极窄图（<1:3）：MVP 不做 resize，按原图渲染会出现裁切或溢出，但不影响"切角色 + 静态显示"的核心能力；如果用户报再补一个 resize 步骤。

## 助手面板空状态：加欢迎语 + 能力说明

`web/src/app/(user)/canvas/components/canvas-assistant-panel.tsx` 之前 `messages.length === 0 && view === "chat"` 时中间空白什么都不显示（line 437 直接 `null`），只剩顶部「创作 Agent」标题和底部三个 suggestion 按钮，中间一大段空白让用户不知道面板能干嘛。

新增 `AssistantEmptyState` 组件，渲染内容：
- 居中的 Bot 图标徽章（圆角 12×12，theme.node.fill 背景）
- 标题：「Hi，我是画布助手」
- 描述：「告诉我你想做什么，我能帮你创建节点、连接素材、写脚本、做分镜、调整配置。」
- 副标：「选中画布节点会在输入框里以参考图加入」（opacity 40 弱化）

接入点：line 437 `messages.length === 0` 时改渲染 `<AssistantEmptyState />` 而不是 `null`。

### 需人工验证
- 打开画布 → 点开桌宠（召唤助手面板）→ 中间不再空白：出现 Bot 徽章 + 三行文字，垂直居中。
- 顶部「创作 Agent」标题、底部三个 suggestion 按钮、composer 输入框都仍在；中间文字不和它们重叠。
- 切到历史记录（点击顶部时间按钮）→ EmptyState 不显示，只显示历史列表。
- 切换会话 / 新对话 → 仍然没有消息时 EmptyState 出现；发了第一条消息后 EmptyState 消失让位给消息列表。
- 切换浅色 / 深色 / 网格主题 → 徽章背景、文字 opacity 都跟随主题 token，没有硬编码色。

## 用户自定义桌宠：上传 sprite sheet + 填参数（动态版）

上一轮把"上传"做成"单张静态图"——上传后桌宠永远显示这一张图，不会有任何动画反馈；用户指出"用户上传的肯定也要是动态的"。本轮把上传流程改造成"上传 sprite sheet + 填网格 / 帧段"，让上传的角色也有待机动画 + 可选的点击动画。

**改动**：
- `web/src/stores/use-pet-settings.ts`：`UploadedPetMeta` 扩展为 sprite 配置：`cols` / `rows` / `cellW` / `cellH` / `idleStart` / `idleEnd` / `idleFps` / 可选 `clickStart` / `clickEnd` / `clickFps`。`width` / `height` 表示 sprite sheet 总尺寸（= cols×cellW / rows×cellH）。
- `web/src/app/(user)/canvas/agent/pet-characters.ts`：把上轮的 `makeStaticPetCharacter` 替换为 `makeUploadedSpritePetCharacter(meta)`——根据 sprite 配置构造 PetCharacter，必有「待机」状态（loop），可选「点击」状态（once）；hover/press/dragLeft/.../dblclick 等其他状态映射到「待机」或「点击」（点击/双击用点击段，其他全部待机）。
- `web/src/app/(user)/canvas/components/canvas-agent-settings.tsx`：
  - 上传流程改为「上传文件 → 拿到 url + 尺寸 → 弹 SpriteConfigModal 让用户填参数 → 提交入 store」。
  - `SpriteConfigModal` 是 antd Modal：左边一栏表单（桌宠名字 / cols / rows / cellW / cellH / 待机 起/止/fps / 点击 Switch + 起/止/fps），右边一栏 88px 实时预览（用 `makeUploadedSpritePetCharacter` + `CanvasAgentCharacter` 复用现有 sprite 渲染管线）。
  - 表单校验：起 ≤ 止、起止在 `0..(cols×rows-1)` 内、点击段在 Switch 启用时才生效；不合法时禁用确认按钮并红字提示。
  - 提交时构造 `UploadedPetMeta` → `addUploadedPet` + `setCharacter(uploaded-<id>)` 自动切到新角色；预览立刻显示动画。
- `web/src/app/(user)/canvas/components/canvas-agent-pet.tsx`：桌宠本体查 character 时 `uploaded-<id>` 分支改用新 helper。

### 需人工验证
- 上传一张 4×4 cell、512×512 的 sprite sheet → 配置 Modal 弹出，默认值合理（cols=4 rows=4 cellW=128 cellH=128 待机 0-13 fps=7），名字预填文件名（去扩展名）。
- 改 cols/rows → 右侧预览缩放按比例；改 cellW/cellH → 预览变大变小。
- 改待机起止 / fps → 预览区域立刻按新帧段循环播放动画（loop）。
- 切「点击动画」启用 → 出现 起始 / 结束 / fps 三个字段；填好后点桌宠预览/画布本体 → 点击一下播放点击动画（once 播放一次后回待机）。
- 不启用「点击动画」→ 点击 / 双击桌宠仍按待机动画。
- 帧段非法（end < start 或超出 0..cols×rows-1）→ 确认按钮置灰 + 红字提示；不能提交。
- 提交成功 → 列表「我的桌宠（N）」多一项；自动切到新角色；预览立刻显示。
- 画布里桌宠本体：点击 / 双击 / hover 都能看到对应动画差异（不全是同一帧静态图）。
- 刷新 / 重新打开画布 → 上传的 sprite 角色仍在 Select，动画行为保持。
- 删除上传的角色 → 后端 file 同步删；store 清空；若删除的是当前选中的，自动回退 Neowow。
- 旧版本（只有 width/height，没有 cols/rows/cellW 等字段）的 localStorage 数据迁移：当前 store partialize 直接覆盖——升级后旧上传记录会因字段缺失导致 `makeUploadedSpritePetCharacter` 计算错误；老数据建议让用户在桌宠设置里删掉重新上传。

## UpDream 画布节点实时额度估算

- 后端新增 `service.VendorCostEstimator` 可选接口 + `service/vendor_updream.go:EstimateCost`，调真实接口 `GET /api/estimate?model_name=...&service_type=IMAGE&quantity=...&quality=...&num_ref_images=...&has_sound=...&has_ref_video=...`；解析 `estimated_credits` 返回。
- 后端新增 `POST /api/v1/vendor/estimate-cost`（handler + router），失败时返回 `source="fallback"`，不阻断前端。
- 前端 `canvas-config-node-panel.tsx`：激活供应商为 UpDream 时，模型/质量/尺寸/张数/参考图变化后 300ms 防抖调用估算；按钮左侧显示黄色闪电图标 + 估算额度，字体与「开始生成」一致；失败/非供应商模式回退到原来的 `requestCreditCost` 静态估算。

### 需人工验证
- 绑定并激活 UpDream 账户后，在画布创建图片节点，选择模型（如 `cheap-b-2` / `flux2.0pro` / `seedream-5.0-pro`）、质量、尺寸、张数 → 「开始生成」按钮左侧闪电旁的数字应接近官网同参数 `estimated_credits`（截图中 `cheap-b-2` + IMAGE + quantity=1 + quality=1K + 无参考 = 5.0）。
- 切换不同质量（自动/高/中/低）或尺寸（1:1/16:9-4k 等）→ 数字应随之变化；切换模型 → 数字变化。
- 连接参考图节点 → `num_ref_images` 增加，额度应变化（若模型支持参考图）。
- 非 UpDream 供应商 / 官方模式 / 未登录 → 闪电旁显示原来的静态成本或 0，不报错。
- 估算接口失败（如 Cookie 过期、代理未配）→ 前端自动 fallback 到静态成本，按钮仍可点。

## LibTV 画布节点实时额度估算（power/calculator）

- 后端在 `service/vendor_libtv_task.go` 给 `libtvTaskAdapter` 实现 `VendorCostEstimator` 接口（`EstimateCost`），调独立的算力预检端点 `POST https://api.liblib.tv/api/task/generation/power/calculator`。
- 请求体结构与 `/api/task/generation/create` 一致（`model` / `taskType` / `provider` / `params{count,quality,ratio}`），但**不会真正创建任务、也不扣费**，仅返回 `data.power`（本次将消耗的算力点，等价于 UpDream 的 `estimated_credits`）。
- 响应解析 `data.power`（参考 devtools 实测的 MJ 例子：`mj-v8.1` + count=4 → `power=15`）。`code!=0` / `power<=0` / 网络异常一律返回 error → 上层 `EstimateVendorCost` 降级到前端静态 `requestCreditCost`。
- LibTV 现只有 `libtvTaskAdapter`（Token header 创作站路径）这一个适配器实现，`EstimateCost` 由它实现；原开放平台 `libTVAdapter`（AK/SK / HMAC，未真机验证、默认路径下从不生效）已删除，不再有"走 fallback 不接 AK/SK 计费"的并存分支（2026-08-17 一致性重构）。
- 前端无需改动：`canvas-config-node-panel.tsx` 对非官方供应商（含 `libtv`）已有 300ms 防抖调用 `estimateVendorCost`，`source="estimate"` 时显示黄色闪电 + 真实额度，否则回退静态。

### 需人工验证
- 绑定并激活 LibTV 账户（Token header 模式）后，画布创建图片节点，选 `mj-v8.1`、count=4、比例 16:9 → 「开始生成」按钮左侧闪电旁数字应≈15（与用户给的官方响应 `power:15` 对齐）。
- 切换模型（seedream-4.5 / mj-v8.1 等）/ 张数（1→4）/ 比例 → 数字随之变化。
- 切到视频节点选视频模型 → 数字也应反映该模型的单次算力（失败则回退静态）。
- 非 LibTV 供应商 / 官方模式 / 未登录 → 闪电旁显示原来的静态成本或 0，不报错。
- 估算接口失败（Token 失效 / 该 calculator 端点字段对不上）→ 前端自动 fallback 到静态成本，按钮仍可点（若 calculator schema 与假定不一致，需回来校准 `EstimateCost` 的请求体字段）。

## 模型下拉/渠道解析：供应商模式只显示供应商模型，官方模式隐藏空"自定义渠道"

新增全局单一事实来源 `selectableModelChannels(config)`（`web/src/stores/use-config-store.ts`）：
- 供应商模式（`activeVendorType !== "official"`）：只返回供应商虚拟渠道（`publicChannels`，如 `vendor:updream`），完全隐藏本地/自定义渠道。
- 官方模式：云端渠道 + 用户**真实添加**的本地渠道；仅本地直连模式（`channelMode==="local"`）保留 `normalizeLocalChannels` 的兜底默认"自定义渠道"（兼容旧本地配置），云端模式下不兜底——用户未添加本地渠道时下拉不再出现"自定义渠道"分组。

`normalizeLocalChannels` 新增 `allowFallback = true` 参数（默认保持旧行为，路由/配置逻辑不变）。以下展示与"模型→渠道"解析消费点统一改用 `selectableModelChannels`，不再各自合并 `publicChannels` + 兜底本地渠道：
- `web/src/components/model-picker.tsx`（所有模型下拉：画布 ModelBar、image/video 工作台、novel、canvas 节点等）
- `web/src/app/(user)/image/page.tsx` 的 `resolveImageChannelId`
- `web/src/app/(user)/video/page.tsx` 的 `resolveVideoChannelId` / `videoChannelText`
- `web/src/components/workflows/creative-workflow-workspace.tsx` 的 `resolveWorkflowImageChannelId`

### 需人工验证
- 激活 UpDream / NewWow / LibTV 任一供应商后，画布顶部 / image / video 工作台模型下拉**只显示该供应商模型**，不再出现 `(自定义渠道)` 分组（之前会混入 `config.models` 里的旧模型如 flux2.0.pro / grok-image1.0）。
- 官方云端模式：未添加任何本地模型渠道时，下拉里不出现"自定义渠道"分组；添加本地渠道后，其模型正常出现在分组中。
- 本地直连模式（channelMode=local）：保留兜底，`config.models` 仍可作为"自定义渠道"使用（旧本地配置不受影响）。
- 各工作台生图/生视频后，生成记录对应的渠道归属（`channelId`）仍正确解析，不会因隐藏本地渠道而错配到 vendor / local-default。


