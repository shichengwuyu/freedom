# 剧本转视频 (Storyboard-to-Video) API 端到端测试报告

- **测试日期**：2026-08-27
- **范围**：`/api/v1/storyboard-tasks` 全链路 + `/api/v1/canvas/image-tasks` + `/api/v1/videos` + `/api/admin/settings` 提示词管理
- **后端**：`http://localhost:18080` (运行中，Go 1.x + Gin)
- **测试账号**：admin (balance 295) + 新注册 `tester2026` (balance 0)
- **测试方式**：API 端到端，调用真实 LLM

---

## 0. TL;DR — 严重阻塞缺陷

> **全链路剧本转视频因中文编码 bug 而不可用**。
> 后端接收合法 UTF-8 中文 → 透传到上游 LLM 时被识别为乱码 → LLM 拒绝生成有效分镜。
> 这同时阻塞了 `/api/v1/chat/completions`、`/api/v1/storyboard-tasks`、`/api/v1/canvas/image-tasks`、视频生成提示词编辑等所有依赖文本生成的端点。
> **任何走 LLM 的剧本分镜、角色图片描述、视频描述词生成，目前都无法产出正常结果**。

---

## 1. 已验证 ✅

### 1.1 API 协议层（与编码无关的部分）
| 端点 | 状态 | 备注 |
|---|---|---|
| `POST /api/auth/register` | ✅ | 正常创建用户 |
| `POST /api/auth/login` | ✅ | 返回 JWT |
| `POST /api/admin/login` | ✅ | 返回 admin JWT |
| `GET  /api/v1/storyboard-tasks` | ✅ | 分页正常，仅返回任务元数据（不含 chapters/assets，性能合理） |
| `GET  /api/v1/storyboard-tasks/:id` | ✅ | 元数据 + result/error |
| `POST /api/v1/storyboard-tasks/:id/cancel` | ✅ | 标记 cancelled |
| `DELETE /api/v1/storyboard-tasks/:id` | ⚠️ | 不存在任务也返回 `{deleted:true}`（见 3.1） |
| `POST /api/v1/canvas/image-tasks` | ✅ | 正确 schema：`requestBody` 内嵌真实请求 |
| `GET  /api/v1/canvas/image-tasks` | ✅ | 列表分页 |
| `GET  /api/v1/canvas/image-tasks/:id` | ✅ | 状态轮询 |
| `GET  /api/v1/videos` (list) | ✅ | 列表 |
| `POST /api/v1/videos` | ⚠️ | 上游 NewWow 拒收（无采集样本） |
| `GET  /api/v1/videos/:id` | ✅ | 状态轮询 |
| `GET  /api/admin/settings` | ✅ | 返回 `data.public.modelChannel.systemPrompts` 等 |
| `POST /api/admin/settings` | ✅ | 整对象保存；`storyboardScript` 等字段可读写；`/api/settings` 公开端随之更新 |
| `GET  /api/settings` (public) | ✅ | 不含 apiKey 等敏感字段 |
| `GET  /api/v1/storyboard-tasks` 未鉴权 | ✅ | 401 `未登录或权限不足` |
| 坏 JSON 请求体 | ✅ | 400 `请求参数解析失败` |
| 缺 `chapters` | ⚠️ | 见 3.2 |

### 1.2 状态机
- `queued` → `running` → `completed` / `failed` / `cancelled` 流转正常
- 已完成的历史任务在 `data` 中显示 `progress: 100, doneCount: 2/2` 格式
- `error` 字段在 LLM 失败时会被填充（但因编码 bug 内容是乱码）

### 1.3 任务调度
- 多分章任务能逐章执行（`===SHOT===` 分隔）
- `doneCount` 实时累加
- balance 不足时按预期失败（已验证自动 demo 用户 balance=0 时无 500）

### 1.4 提示词管理
- admin settings 包含完整分镜提示词三件套：
  - `systemPrompts.storyboardScript`（章节→分镜剧本）
  - `systemPrompts.storyboardVideo`（分镜→视频描述词）
  - `systemPrompts.storyboardImage`（角色三视图/场景四宫格/道具标准图）
- 通过 admin 端点更新后，再读回内容正确（marker 验证通过）
- 公开 `/api/settings` 不返回这些提示词（合理，前端不读）

---

## 2. 严重 Bug：中文编码在 AI 代理层被破坏

### 2.1 复现路径

```bash
# 请求 (UTF-8 合法)
curl -X POST http://localhost:18080/api/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{"model":"agnes-2.0-flash","channelId":"channel-0ad1b4fbc3c86b5c",
       "messages":[{"role":"user","content":"用中文回答你喜欢什么季节,15字以内"}]}'
```

**实际响应**（节选）：
```
choices[0].message.content = "抱歉，我无法理解您的乱码问题。"
reasoning_content = "用户的问题是乱码，无法识别实际语言或意图……"
```

**确认链路**：LLM 看到的输入是乱码，所以它认为用户提了乱码问题。

### 2.2 直接复现证据

历史 storyboard 任务 `storyboard-task-1b65d30e-2139-4caa-98f3-91fe5620d316` 状态 `completed`，但 `error` 字段（`"无法识别输入内容..."`）和 `result` 字段都被 LLM 当乱码拒答过：
- result 中 LLM 实际收到 `"25?��??��?"`（替换字符 U+FFFD 的混合）
- 这是**典型的 UTF-8 字节被误按 GBK 解码后重新编码**的症状

### 2.3 排除项
- 后端 HTTP 响应正确输出 UTF-8（`xxd` 检查 list 接口返回 `0xe6b2a1...` 是「没」字的正确 UTF-8 编码）
- MySQL 存储是合法 UTF-8（列 charset utf8mb4）
- Gin 默认 UTF-8 序列化无问题
- **bug 一定在「从 `r.Body` 读出 → 转发到上游 channel」之间**

### 2.4 可疑代码点

| 文件 | 行 | 现象 |
|---|---|---|
| `handler/ai.go` | ~250-260 | `io.ReadAll(r.Body)` 后 `bytes.NewReader(body)` 转发，Content-Type 透传但 charset 不会自动转 |
| `handler/canvas_task.go` | 462 | `io.ReadAll(io.LimitReader(r.Body, ...))` 同上 |
| `service/ai_proxy*.go` | — | 上游请求 URL 用 `BuildModelChannelURL`，但 body 是否在转发时再次被编码不确定 |
| `model/setting.go` 序列化 | — | 已用 `json:"..."` 显式标签，不应触发默认 GBK |

**最可能根因（待进一步验证）**：
1. 早期版本用 `ioutil.ReadAll` 而没有限制 charset；
2. `BuildModelChannelURL` 中上游 baseURL 后追加路径时，可能有一次 `url.Values.Encode()` 调用会按 GBK 编码 query 字符串（这不会影响 body）；
3. **更可疑的是 `proxyAIRequest` 内 `json.Marshal` 重组请求体的位置** — 如果某处对 `body` 用了 `string(body)` 再 `json.Marshal(map[string]string{"messages": ...})`，且该 string 被某中间层 `[]byte(s)` 后台按 GBK 序列化，就会产生"看得到原文但实际是 GBK"的乱码。

### 2.5 建议排查顺序

1. 在 `handler/ai.go` 的 `io.ReadAll(r.Body)` 之后立即打 `hex.Dump(body[:64])` 看中文是否还是 UTF-8 — 如果已坏，问题在 Gin/中间件/CORS；如果还正常，则在转发前被某次序列化破坏。
2. 在 `service/ai_proxy.go` / `proxyAIRequest` 中对比 `body` 进入/出去两次的 hex dump。
3. 检查 Windows locale（GBK 是系统默认）下，标准库 `net/http`、`encoding/json` 的隐式 charset 行为；显式加 `utf8.NewDecoder` 包一层。

---

## 3. 次要 Bug

### 3.1 DELETE 不存在任务返回成功
- **位置**：`handler/storyboard_task.go` `DeleteUserStoryboardTaskHandler`
- **行为**：`DELETE /api/v1/storyboard-tasks/no-such-id` → `{"code":0,"data":{"deleted":true}}`
- **影响**：前端无法判断是否真删了；并发删除会假成功；可触发重复扣费/释放。
- **建议**：handler 内先 `GetUserStoryboardTask(userID, id)` 查在不在，不在返 `code:1, msg:"任务不存在"`。

### 3.2 空 chapters 仍可成功创建任务
- **位置**：`handler/storyboard_task.go` `CreateStoryboardTaskHandler`
- **行为**：`chapters:"[]"` 也通过校验生成 `queued` 任务。
- **影响**：用户漏传分章也能走完整 worker 流程，最终 LLM 拒答失败，浪费余额和算力。
- **建议**：在 `UnmarshalChapters` 后追加 `if len(chapters) == 0 { fail }`；同样校验 `assets` 至少一项。

### 3.3 NewWow 视频模型与可下拉列表不一致
- **位置**：`/api/v1/videos` handler
- **行为**：所有 NewWow 上游视频模型（`kling-*`, `seedance-*` 等）都返回 `errCode=1001 模型不存在`。
- **根因**：用户激活了 NewWow 但未提供任何采集样本；handler 透传到 NewWow 适配器，被 NewWow 拒收。
- **建议**：前端 video 节点在用户使用 NewWow 时应先调用 `GET /api/v1/vendor/samples` 提示用户采集；或 handler 检测无样本时直接 `Fail("NewWow 还没有可用样本，请先采集")`（参考 image-tasks 已有此分支）。

### 3.4 UserChannelID 不查全局渠道
- **位置**：`service/user_data.go:54-90` `SelectUserLocalModelChannelForModel`
- **行为**：传 `userChannelId` 时只查用户的本地 `localChannels`，不查全局渠道。
- **影响**：用户在前端选了一个本地渠道（实际不存在），会报"本地渠道不存在"而非自动回退。
- **建议**：在 `localChannels` miss 时 fallback 到 `SelectModelChannelForModel`。

### 3.5 缺 `chapters` 字段的 POST
- `{"clientTaskId":"...","sourceId":"...","model":"...","channelId":"...","shotDuration":8}` 不传 chapters 时，handler `json.Unmarshal` 拿不到 chapters → `errors.New("分章内容解析失败")`（400），行为正确 ✅（不是 bug，记一下）。

---

## 4. 端点速查表（验证可用）

| 端点 | 状态 | 关键参数/Schema 提示 |
|---|---|---|
| `POST /api/v1/storyboard-tasks` | ✅ | `chapters` 必须是 JSON 字符串（`"[\"...\"]"`，不是 array）；`assets` 同理 |
| `GET  /api/v1/storyboard-tasks?page=&pageSize=` | ✅ | 不返回 chapters/assets |
| `GET  /api/v1/storyboard-tasks/:id` | ✅ | 返回 result/error，result 内含每章 LLM 输出 |
| `POST /api/v1/storyboard-tasks/:id/cancel` | ✅ | 仅对 `queued`/`running` 生效 |
| `DELETE /api/v1/storyboard-tasks/:id` | ⚠️ | 不存在也返 success（见 3.1） |
| `POST /api/v1/canvas/image-tasks` | ✅ | **必须把真实请求体放进 `requestBody` 字段**（不是平铺 `prompt`） |
| `GET  /api/v1/canvas/image-tasks?page=&pageSize=` | ✅ | |
| `GET  /api/v1/canvas/image-tasks/:id` | ✅ | |
| `POST /api/v1/videos` | ⚠️ | NewWow 用户必须先采集样本 |
| `GET  /api/v1/videos/:id` | ✅ | |
| `GET  /api/v1/video-tasks?page=&pageSize=` | ✅ | |
| `GET  /api/admin/settings` | ✅ | `data.public.modelChannel.systemPrompts.storyboard*` |
| `POST /api/admin/settings` | ✅ | 整对象保存 |

---

## 5. 可立即复现的关键命令

```bash
# 1) 编码 bug 最小复现
curl -sX POST http://localhost:18080/api/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{"model":"agnes-2.0-flash","channelId":"channel-0ad1b4fbc3c86b5c",
       "messages":[{"role":"user","content":"用中文回答你喜欢什么季节,15字以内"}]}'

# 2) DELETE 不存在任务假成功
curl -sX DELETE http://localhost:18080/api/v1/storyboard-tasks/no-such -H "Authorization: Bearer $TOKEN"
# {"code":0,"data":{"deleted":true},"msg":"ok"}

# 3) 空 chapters 仍能创建
curl -sX POST http://localhost:18080/api/v1/storyboard-tasks \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"clientTaskId":"empty","sourceId":"t","model":"agnes-2.0-flash",
       "channelId":"channel-0ad1b4fbc3c86b5c","shotDuration":8,
       "scriptPrompt":"","chapters":"[]","assets":"[]"}'
# → 200 + 新 id，状态 queued
```

---

## 6. 修复优先级

1. **P0 — 编码 bug**：阻塞整个剧本转视频核心流程。需在 `handler/ai.go` 的 `io.ReadAll` 路径加 `utf8` 显式保证，并复审所有 `string([]byte)` → `json.Marshal` 的中间转换。
2. **P1 — DELETE 不存在任务返 404**：避免误判；快速修复。
3. **P1 — 空 chapters 校验**：避免无效 worker 任务浪费余额。
4. **P2 — UserChannelID 回退**：提升用户路径容错。
5. **P2 — NewWow 无样本提前 fail**：视频端点友好提示。

---

## 7. 已完成测试用例清单

| # | 用例 | 状态 |
|---|---|---|
| T2.x | storyboard-tasks 创建/列表/详情/取消/删除（5 路径） | ✅ 协议层 OK；运行层受 2.1 阻塞 |
| T3.1 | canvas image-tasks POST schema 校验 | ✅ |
| T3.2 | canvas image-tasks GET list | ✅ |
| T3.3 | canvas image-tasks GET :id 状态轮询 | ✅（生成因 NewWow 无样本失败） |
| T4.1 | videos POST（kling / seedance） | ⚠️ NewWow 拒收 |
| T4.2 | videos GET :id | ✅ |
| T4.3 | video-tasks list | ✅ |
| T5.1 | admin GET settings（含 storyboard 提示词） | ✅ |
| T5.2 | admin POST settings（更新 storyboardScript） | ✅ |
| T5.3 | admin GET settings 验证更新 | ✅ marker 存在 |
| T6.1 | DELETE 不存在任务 | ⚠️ 假成功 |
| T6.2 | POST 空 chapters | ⚠️ 仍创建 |
| T6.3 | GET 无 token | ✅ 401 |
| T6.4 | POST 坏 JSON | ✅ 400 |

—— 报告完 ——
