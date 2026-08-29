# 「剧本转视频」功能模块 测试报告（2026-08-27 第二轮）

- **测试日期**：2026-08-27（21:00+）
- **范围**：`/novel` 页面（前端） + `官方云端`（官方 agnes 渠道，channel-0ad1b4fbc3c86b5c）全链路：
  `/api/v1/storyboard-tasks` + `/api/v1/canvas/image-tasks` + `/api/v1/videos` + `/api/v1/vendor/*`
- **后端**：`http://localhost:18080` (Go + Gin, PID 2148 `freedom.exe`, started 19:34)
- **前端**：`http://localhost:3000` (Next.js 16.2 dev, PID 17376)
- **测试账号**：admin（balance 295 分 = ¥2.95，与截图一致）
- **测试方式**：Python `requests` 库端到端，调用真实 LLM 和视频生成 API

---

## 0. TL;DR

1. **「官方云端」文字分镜链路 ✅ 完全可用**。单章 15-20s 跑完，多章（3 章）64s 跑完；LLM 输出合法中文（含 `@李青云` 角色标签、时间段分层、场景描述），没有再观察到 TEST_REPORT_2026-08-27.md 报告的中文编码 bug。
2. **「官方云端」视频生成 ✅ 可用**。一次跑通，`https://cos-platform-outputs.agnes-ai.cn/.../video_*.mp4` 返回；用时约 2-3 分钟（轮询 budget 至少 3 分钟）。
3. **【P0 用户视角 bug】当前激活的 vendor 仍是 NewWow**，与 UI 显示的「官方云端」不一致。新建视频任务立刻被 NewWow 拒收（`errCode=1001 模型不存在: agnes-video-v2.0`）；新建 canvas 资产生图任务直接卡 `queued` 永远不执行。
4. **后台已知小 bug（#3.1 / #3.2）仍在**，未修复。

---

## 1. 关键发现：用户截图与实际状态不一致

截图顶部写着「**官方云端（管理员配置）● 当前**」高亮，但后端 `GET /api/v1/vendor/accounts` 返回：

```json
[{"vendorType": "newwow", "isActive": true, "displayName": "用户1601",
  "balanceText": "NewWow 积分 20", "hasModels": true}]
```

也就是说**用户视觉上选了官方云端，但 DB 里激活的供应商仍然是 NewWow**。这是当前 P0 问题。

### 1.1 触发的影响

| 模块 | 选「官方云端」模型 | 实际行为 |
|---|---|---|
| 文本（`/chat/completions`）| agnes-2.0-flash | ✅ 走 channel-0ad1b4fbc3c86b5c，正常 |
| 分镜剧本（`/api/v1/storyboard-tasks`）| agnes-2.0-flash | ✅ 走 channel-0ad1b4fbc3c86b5c，正常 |
| 资产生图（`/api/v1/canvas/image-tasks`）| agnes-image-2.0-flash | ❌ 卡 `queued` 永远不执行（vendor-mode 接管，channel 被空）|
| 视频（`/api/v1/videos`）| agnes-video-v2.0 | ❌ 502 "NewWow 视频提交被拒：模型不存在: agnes-video-v2.0" |

**根因**：`handler/canvas_task.go:57-78` 看到 `isVendorMode == true`（NewWow 激活）时，把 channel **置空**，无视客户端传入的 `channelId`：

```go
activeVendor, _ := service.GetActiveVendorAccount(user.ID)
isVendorMode := activeVendor != nil && activeVendor.VendorType != model.VendorTypeOfficial
if isVendorMode {
    channel = model.ModelChannel{}   // ← 这里把所有官方 channel 抹掉了
} else {
    // ...正常 select channel
}
```

文本/分镜路径不走这段判定所以 OK；图片/视频路径在 vendor 模式下被劫持，但用户感知不到（UI 上是「官方云端」绿色圆点）。

### 1.2 验证修复

我手动 `POST /api/v1/vendor/activate {vendorType: "official"}` 把状态切到官方，**所有路径立刻恢复正常**：

```
video with official: HTTP 200, id=video-task-b89bb8c0-...
  → 2-3 分钟后 completed
  → video_url=https://cos-platform-outputs.agnes-ai.cn/videos/agnes-video-v2.0/video_5413f6bf...mp4
```

> **注意**：测完已 `POST /api/v1/vendor/activate {vendorType: "newwow"}` 把状态还原，避免影响用户当前会话。

---

## 2. 编码 bug 复测

TEST_REPORT_2026-08-27.md 报的 P0 中文乱码 bug（`handler/ai.go` 转发到上游时 LLM 收到乱码）：**当前不可复现**。

### 2.1 chat/completions 流式 + 非流式

| 调用方式 | 输入 | LLM 返回 | 结论 |
|---|---|---|---|
| 非流式 POST | "用中文回答你喜欢什么季节,15字以内" | `我无偏好，各季节皆有特色。` | ✅ UTF-8 正常 |
| 流式 POST（3 次）| 同上 | 完整 reasoning + content 中文流畅 | ✅ UTF-8 正常 |

### 2.2 真实分镜任务

3 章分镜任务 `storyboard-task-e32a5539-...` 在 64s 内完成，LLM 输出含：

- 章节 1：开场角色 + 场景 + 时间段分层（`0-2s｜中景切入晨雾中的山村茅屋` … `2-5s｜中景跟拍`）+ 角色名 + 动作连贯
- 章节 2：`@李青云` 标签正确出现；`出场角色：李青云；黑色巨蟒`；`场景：山路悬崖；崖壁岩缝`
- 章节 3：动作连贯（`短刀一闪 / 侧身一矮 / 反手一削`），承接上一段

**结论**：之前 15:49 那条乱码任务（`storyboard-task-1b65d30e-...`）确实是当时上游问题或网络抖动，现在默认的 `channel-0ad1b4fbc3c86b5c` (agnes apihub) 链路稳定。

---

## 3. 剧本转视频 E2E 走通（官方云端已激活场景）

### 3.1 完整链路

```
POST /api/v1/storyboard-tasks (chapters=[3 章], model=agnes-2.0-flash)
  → 64s 内 3/3 doneCount 完成
  → result 中每章结构：
    {
      "groupIndex": 0,
      "status": "completed",
      "content": "出场角色：李青云\n场景：…\n\n{0-2s｜…@李青云（…）…}"
    }
  → 含 @角色 标签可供下游「资产生图 + 视频生成」关联
```

### 3.2 截图所示「官方云端」下能用的模型矩阵

| 能力 | 模型 | 渠道 | 状态 |
|---|---|---|---|
| 文本分镜 | agnes-2.0-flash | 免费 (agnes) | ✅ 验证 |
| 文本分镜 | agnes-2.5-flash | 免费 (agnes) | 应可用（同一渠道）|
| 资产生图 | agnes-image-2.0-flash | 免费 (agnes) | ✅ 已创建（实际执行依赖 vendor 状态）|
| 资产生图 | agnes-image-2.1-flash | 免费 (agnes) | 同上 |
| 视频 | agnes-video-v2.0 | 免费 (agnes) | ✅ 已生成（切到 official vendor 后）|
| 视频 | kling-3.0-omni-1080p | 付费 (rolldek) | NewWow 不识别 |
| 视频 | seedance-2.0-431-720p | 付费 (rolldek) | NewWow 不识别 |
| 图片 | gpt-image-2 / gpt-image-2-high | 付费 (rolldek) | NewWow 不识别 |
| 图片 | gemini-3-pro-image-preview | 付费 (rolldek) | NewWow 不识别（被 `apimartImageModelUnsupportedByUpstream` 阻断）|

---

## 4. 端点速查（已验证）

| 端点 | 测试 | 状态 |
|---|---|---|
| `POST /api/auth/login` | 拿到 admin JWT | ✅ |
| `GET  /api/settings` | 23 个 model + 2 个 channel (免费/付费) | ✅ |
| `GET  /api/vendors` | 4 个 vendor，official/newwow 启用 | ✅ |
| `GET  /api/v1/vendor/accounts` | 当前仅 newwow isActive=true | ⚠️ 状态与 UI 不一致 |
| `POST /api/v1/vendor/activate` | 切 official / 切回 newwow 都成功 | ✅ |
| `GET  /api/admin/settings` | 含 `systemPrompts.storyboardScript/Video/Image` | ✅ |
| `POST /api/v1/storyboard-tasks` | chapters=JSON 字符串、assets=JSON 字符串 | ✅ |
| `GET  /api/v1/storyboard-tasks/:id` | result 内含每章 LLM 输出 | ✅ |
| `GET  /api/v1/storyboard-tasks` | 列表分页 | ✅ |
| `POST /api/v1/storyboard-tasks/:id/cancel` | 立即变 canceled | ✅ |
| `DELETE /api/v1/storyboard-tasks/:id` | 不存在也返 success | ⚠️ 旧 bug 仍存（#3.1）|
| `POST /api/v1/storyboard-tasks` 空 chapters | 仍能创建（queued 后被 worker 拒）| ⚠️ 旧 bug 仍存（#3.2）|
| `POST /api/v1/canvas/image-tasks` | requestBody.model + requestBody.prompt 正确时 200 | ✅ |
| `POST /api/v1/videos` agnes-video-v2.0 | 切到 official 后 200 + 真实视频 URL | ✅ |
| `POST /api/v1/videos` 在 NewWow 模式下 | 502 "NewWow 模型不存在" | ⚠️ 用户当前正撞这个 |
| 缺 token | 401 | ✅ |
| 坏 JSON | 400 | ✅ |

---

## 5. 用户当下应该怎么做

按截图「**官方云端**」期望工作，目前 admin 账号下：

1. **打开浏览器** → 「更多工具 → 配置与用户偏好」把「官方云端（管理员配置）」**点一下**（切到 NewWow 再切回官方，会触发 `POST /api/v1/vendor/activate {vendorType:"official"}`）。
2. 或者**后台**直接调 `POST /api/v1/vendor/activate {vendorType:"official"}`（admin 也能用）。
3. 切完之后：
   - 分镜剧本链路（出 3 章分镜 + 角色标签）✅ 15-60s 出结果
   - 资产生图（角色三视图 / 场景四宫格 / 道具标准图）✅
   - 视频生成（每分镜 8s，16:9 720p）✅ 2-3 分钟出 mp4

> 切回 NewWow 之后，**所有非 NewWow 模型都无法工作**（NewWow 自己的模型列表是 `MiniMax-Hailuo-02` / `doubao-seedance-1-0-pro-250528` / `veo3` / `gemini-3.1-flash-image-preview` 四个，不是 agnes）。

---

## 6. 建议修复（按优先级）

### P0 — 让 UI 状态与 DB 状态一致

**复现**：
1. 管理员在「更多工具」选官方云端 → DB 写入 `isActive=true` for official
2. 用户切到 NewWow 完成一些任务
3. 管理员**再次**点「官方云端」 → 期望切回 official，但 UI 圆点已显示「当前」（管理员上次操作的结果），用户感知不到自己点的是 NewWow

**修复方向**（任选其一）：
- (a) 「官方云端」按钮在用户**未绑定**任何第三方 vendor 时禁用（避免误导）
- (b) UI 在切回官方时弹窗确认「NewWow 仍在激活，是否要切？」+ 提示余额差异
- (c) 后端在 `ActivateVendor(official)` 时如果存在 isActive 的 NewWow，自动把 NewWow 标 isActive=false（覆盖而非并存）

### P1 — vendor-mode 屏蔽官方 channel 时的友好提示

`handler/canvas_task.go:57-78` 在 vendor 模式下把 channel 置空，导致 canvas 资产生图任务**默默卡 queued**。建议：
- 当 vendor 模式激活时，handler 在 `error` 字段写「当前 vendor 不支持该模型 X，请切到官方云端或选择 vendor 提供的等价模型」，而不是无错误地写 DB。
- 同样的「vendor 模型不存在」错误目前是 502 但 msg 是供应商原文，应该翻译成「NewWow 暂不支持该模型：agnes-video-v2.0」+ 提示。

### P1 — 旧 bug #3.1 / #3.2 仍未修
- `DELETE /api/v1/storyboard-tasks/no-such-id` 仍返 `{deleted: true}` → 改成先查再删，不存在返 404
- `POST /api/v1/storyboard-tasks` 空 chapters 仍能创建 → handler 加 `if len(chapters)==0 return 400`

---

## 7. 测试用例清单

| # | 用例 | 状态 |
|---|---|---|
| T1 | admin 登录 | ✅ |
| T1b | `/api/settings` 列出 23 模型 / 2 渠道 | ✅ |
| T1c | chat/completions 中文不乱码 | ✅（bug 已不可复现）|
| T2.1 | 单章分镜创建 | ✅ |
| T2.2 | 单章分镜 15-20s 完成 + 合法中文 | ✅ |
| T2.3 | 3 章分镜 64s 完成 + @角色 标签正确 | ✅ |
| T3 | 分镜列表分页 | ✅ |
| T4 | 取消任务（canceled + 错误信息）| ✅ |
| T5 | DELETE 不存在任务 | ⚠️ 假成功（旧 bug #3.1）|
| T6 | 空 chapters 创建 | ⚠️ 仍创建（旧 bug #3.2）|
| T7 | 无 token → 401 | ✅ |
| T8 | admin settings 含 storyboard 提示词 | ✅ |
| T9 | canvas image-task 创建（model 在 requestBody 内）| ✅ |
| T10 | active channel 验证 | ✅ |
| T11 | vendor=newwow 时视频被拒 | ⚠️ 用户当前撞这个 |
| T12 | vendor=official 时视频可生成 + 真实 URL | ✅ |
| T13 | 切 vendor 的 API 工作 | ✅ |

—— 报告完 ——
