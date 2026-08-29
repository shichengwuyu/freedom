# 多供应商云端切换架构设计文档（官方 / UpDream / LibTV / NewWow）

> 文档版本：v1.0  
> 编写日期：2026-08-16  
> 适用范围：本项目 AI 生成链路（文本 / 图片 / 视频 / 音频）、资产库、模型配置体系

---

## 一、背景与目标

### 1.1 现状

项目当前的模型 / 云端架构为 **「官方管理员配置 ModelChannel」 + 「用户本地自定义渠道（allowCustomChannel 开关）」** 双轨制：

- **官方模式（channelMode = remote）**：用户登录后，所有 AI 请求走后端 `/api/v1/*` 代理，由管理员在后台配置的 `ModelChannel`（含 BaseURL、APIKey、模型列表、权重、超时、扣费）统一调度，并扣用户账户 Credits。
- **本地直连模式（channelMode = local）**：当管理员开启 `allowCustomChannel` 且用户已登录时，用户可在前端填自己的 BaseURL + APIKey，前端直连第三方 OpenAI 兼容接口，不走后端代理，也不扣 Credits。

核心文件：

- 前端：`web/src/stores/use-config-store.ts`（`AiConfig`、`resolveEffectiveConfig()`）
- 前端 UI：`web/src/components/layout/app-config-modal.tsx`
- 后端模型：`model/setting.go`（`PublicModelChannelSetting`、`ModelChannel`、`ModelCost`）
- 后端代理入口：`handler/ai.go`（`proxyAIRequest()`、`selectAIRequestChannel()`）
- 后端调度逻辑：`service/settings.go`（`SelectModelChannelForModel()`）

### 1.2 问题 & 需求

现状对 **「独立部署、用户自备 Key」** 场景够用，但对 **「接入多家外部云端平台（UpDream / LibTV / NewWow 等）」** 有以下缺口：

| 维度 | 现状 ModelChannel | 外部供应商平台实际需要 |
|---|---|---|
| 鉴权方式 | 手动填明文 API Key | OAuth 授权登录（需跳上游、换 Token、刷新） |
| 模型来源 | 管理员配置 + /models 接口 | 供应商账户绑定后调用「我的可用模型」专属接口 |
| 接口格式 | 统一 OpenAI 兼容协议（/chat/completions、/images/generations） | 各家大概率非 OpenAI 兼容，字段、路径、鉴权 Header 均不同 |
| 资产/素材 | 本项目 S3 / WebDAV / IndexedDB | 供应商侧有自己的素材库，需要拉取 / 上传 / 同步 |
| 计费 | 项目自扣 Credits | 走供应商账户余额（按时长 / 按次 / 包套餐） |
| 模型集合 | 所有渠道取并集混合展示 | 切换供应商后，模型列表应**全部**换成该供应商官方的，不可混其他家 |

### 1.3 设计目标

1. **供应商即开即用**：用户在配置弹窗选择某供应商（UpDream / LibTV / NewWow） → 自动跳 OAuth 登录授权 → 登录成功后模型列表、AI 接口、资产库**全部**切换到该供应商。
2. **官方模式零改动**：`activeVendorType = "official"` 时，所有现有代码路径、数据结构、配置项**完全不动**，用户可随时切回，无任何兼容风险。
3. **本地直连保留**：即使切到供应商体系，用户仍可在"官方模式"下使用本地自定义渠道（你说的「本地预留一手」）。
4. **供应商适配可插拔**：新增一家供应商 = 实现一个 `VendorAdapter` 接口 + 注册，无需改动业务层代码。
5. **资产双写兜底**：供应商侧的素材资产在使用时同步到本项目存储，避免供应商下线 / 授权过期导致用户历史项目打不开。

---

## 二、整体架构

### 2.1 分层架构图

```
┌──────────────────────────────────────────────────────────────────────┐
│                            前端 UI 层                                  │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │  app-config-modal.tsx   新增：供应商切换 Tab                     │   │
│  │  ├─ [●] 官方云端（现有行为，进入后可选 remote / local）          │   │
│  │  ├─ [○] UpDream  ── 未绑定 → 跳 OAuth 授权                      │   │
│  │  ├─ [○] LibTV     ── 已绑定 → 显示昵称/余额/解绑按钮             │   │
│  │  └─ [○] NewWow                                                 │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                               ↓ activeVendorType                       │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │  use-config-store.ts   新增：buildVendorEffectiveConfig()      │   │
│  │  official → 走原 resolveEffectiveConfig() 完全不动              │   │
│  │  其他   → 从后端拉该供应商账户的可用模型快照，填入 imageModels 等 │   │
│  └────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────┘
                                      │ HTTP（/api/v1/* 不变）
                                      ▼
┌──────────────────────────────────────────────────────────────────────┐
│                          后端 Handler 层                               │
│  handler/ai.go  proxyAIRequest() 顶部新增分支：                        │
│    查 GetActiveVendorAccount(userID)                                   │
│      ├─ official 或 nil  →  走原 selectAIRequestChannel() 完全不动    │
│      └─ 其他供应商       →  Token 过期自动 Refresh → dispatchBy       │
│                             VendorAdapter() 分发到 GenerateImage 等    │
└──────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    VendorAdapter 层（可插拔）                           │
│  service/vendor_adapter.go  定义统一接口 + 注册中心                    │
│                                                                       │
│  ┌─────────────┐  ┌────────────┐  ┌─────────┐  ┌──────────────┐      │
│  │ UpDream Adpt│  │ LibTV Adpt │  │ NewWow │  │ Official Adpt│      │
│  │ (调 updream │  │ (调 libtv  │  │  Adpt   │  │ (包住现有     │      │
│  │  SDK/API)   │  │  SDK/API)  │  │         │  │ ModelChannel │      │
│  └─────────────┘  └────────────┘  └─────────┘  └──────────────┘      │
└──────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌──────────────────────────────────────────────────────────────────────┐
│                   持久化层（DB + 加密存储）                             │
│  ┌──────────────────────┐   ┌───────────────────────────────┐        │
│  │ vendors 表            │   │ user_vendor_accounts 表        │        │
│  │ (管理员配置的供应商元   │   │ (用户OAuth Token，加密存；      │        │
│  │  信息：ID/Logo/OAuth   │   │  IsActive=true 表示当前激活；  │        │
│  │  URL/是否启用)         │   │  AvailableModelsJSON 存模型快照)│        │
│  └──────────────────────┘   └───────────────────────────────┘        │
└──────────────────────────────────────────────────────────────────────┘
```

### 2.2 请求分发流程（时序摘要）

```
用户点「开始生图」
       │
       ▼
 前端 useEffectiveConfig()
       │
       ├─ activeVendorType === "official"
       │     └─ 走原逻辑（channelMode=remote/local）
       │
       └─ activeVendorType === updream/libtv/newwow
             └─ 模型列表=该账户 AvailableModelsJSON 快照，POST /api/v1/images/generations
                      │
                      ▼
               后端 proxyAIRequest()
                      │
                      ├─ GetActiveVendorAccount(userID) → IsActive=true 的那条
                      │
                      ├─ Token 是否过期（<5min）？
                      │     └─ 过期 → singleflight 锁内调用 adapter.RefreshAccessToken()
                      │
                      └─ adapter.GenerateImage(ctx, account, input)
                              │
                              ├─ UpDream → 调 updream API，字段映射到标准 GenerateMediaOutput
                              ├─ LibTV   → 调 libtv API，字段映射到标准 GenerateMediaOutput
                              └─ ...
                                      │
                                      ▼
                              返回 GenerateMediaOutput（URL/字节）
                                      │
                                      ├─ 如果需要双写 → 调用现有 StorageProvider 存一份到 S3/WebDAV
                                      │
                                      └─ 包装成 OpenAI 兼容格式返回前端（前端代码零改动）
```

---

## 三、数据模型设计（新增 2 张表，不修改现有表）

### 3.1 vendors（系统级供应商元信息）

管理员在后台配置，所有用户共享。**不影响现有 settings 表。**

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string (PK) | 主键，建议 `vt_xxx` 前缀 |
| `type` | string(32) | 供应商类型枚举（**唯一业务标识**）：`official` / `updream` / `libtv` / `newwow` |
| `name` | string | 显示名，如"UpDream 云端创作平台" |
| `logo_url` | string | Logo 图片 URL（下拉框 / 绑定页显示） |
| `oauth_auth_url` | string | OAuth 授权页 URL（official 为空） |
| `oauth_token_url` | string | OAuth Token 换取地址 |
| `oauth_client_id` | string | 本项目在该供应商注册的 App Client ID |
| `oauth_client_secret` | string | Client Secret（后端保存，**永不返回前端**） |
| `oauth_redirect_uri` | string | 本项目回调地址，如 `https://xxx.com/api/vendor/oauth/callback/:type` |
| `api_root_url` | string | 供应商 API 根地址，如 `https://api.updream.com/v1` |
| `enabled` | bool | 是否启用（管理员停用后前端不显示该选项） |
| `sort` | int | 前端下拉顺序（小在前） |
| `extra_config_json` | longtext | 供应商专属配置 JSON（字段名规范、特殊 Header 等），兜底用 |
| `created_at` / `updated_at` | datetime | 常规时间戳 |

**新增 Go Model：** `model/vendor.go` 的 `Vendor` 结构体。

### 3.2 user_vendor_accounts（用户绑定的供应商账户）

每个用户 + 每个供应商最多一条；同一用户同一时刻只有一条 `is_active=true`。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string (PK) | 主键 |
| `user_id` | string (INDEX) | 关联 users 表 |
| `vendor_type` | string(32) | 对应 vendors.type |
| `vendor_id` | string | 冗余关联 vendors.id |
| `display_name` | string | 供应商侧显示昵称（如"UpDream-小明"），前端展示用 |
| `avatar_url` | string | 头像（可选） |
| `access_token` | text (**加密列**) | OAuth Access Token（DB 层 AES 加密，或应用层加密后存，JSON 序列化 `-` 忽略） |
| `refresh_token` | text (加密列) | OAuth Refresh Token（同上） |
| `token_expires_at` | datetime \| null | Access Token 过期时间；刷新时更新 |
| `scope` | string | 授权 scope（如 `images:write assets:read`），透传 |
| `is_active` | bool (INDEX) | **激活标记**：每个 user_id 只有一条为 true |
| `available_models_json` | longtext | 该账户可用模型的快照 JSON（结构见 §3.3），避免每次调供应商 API 拉取；绑定成功 / 后台管理员点「刷新模型」时更新 |
| `balance_info_json` | longtext | 余额 / 套餐快照 JSON（如 `{"credits": 12850, "package": "Pro 年卡", "expire": "2027-08-01"}`） |
| `vendor_user_id` | string | 供应商侧用户唯一 ID（用于去重绑定） |
| `raw_extra_json` | longtext | 兜底字段：供应商返回的其他个性化信息 |
| `bound_at` | datetime | 首次绑定时间 |
| `last_used_at` | datetime | 最近一次用这个账户发起 AI 请求的时间（用于排序、清理不活跃账户） |
| `created_at` / `updated_at` | datetime | 常规时间戳 |

**索引建议**：
- `UNIQUE KEY uk_user_vendor (user_id, vendor_type)`：用户每种供应商只能绑一个
- `KEY idx_active (user_id, is_active)`：高频查询"用户当前激活账户"

**加密策略**（二选一，推荐 A）：
- **A. 应用层 AES-GCM**：服务启动时读环境变量 `VENDOR_TOKEN_AES_KEY`，存/取 `access_token`、`refresh_token` 时自动加解密。GORM 可以用自定义 `Serializer` 实现。
- **B. 数据库列加密**（如 MySQL AES_ENCRYPT）：写 SQL 时用函数，简单但审计 / 迁移麻烦。

**新增 Go Model：** `model/vendor.go` 的 `UserVendorAccount` 结构体。

### 3.3 AvailableModelsJSON 结构（内部约定）

`user_vendor_accounts.available_models_json` 存的 JSON 结构（跟前端 `AiConfig.imageModels` 等对齐，方便前端直接消费）：

```json
{
  "imageModels": [
    { "id": "updream-sd3-turbo", "name": "SD3 Turbo 极速版", "capability": "image",
      "sizes": ["1024x1024", "768x1344"], "extra": {} }
  ],
  "videoModels": [
    { "id": "updream-video-2.0", "name": "UpDream Video 2.0", "capability": "video",
      "maxSeconds": 15, "refVideo": true, "refAudio": false, "genAudio": true, "extra": {} }
  ],
  "textModels": [
    { "id": "updream-gpt-4o", "name": "UpDream 文本大模型", "capability": "text", "extra": {} }
  ],
  "audioModels": [
    { "id": "updream-tts-pro", "name": "UpDream 专业配音", "capability": "audio",
      "voices": ["冰糖", "阿杰"], "extra": {} }
  ],
  "modelLabels": {
    "updream-video-2.0": "阿普视频 2.0"
  },
  "fetchedAt": "2026-08-16T10:00:00Z"
}
```

前端拿到后可以直接拆进 `effectiveConfig.imageModels / videoModels / ...` 数组（只取 `id` 作为模型下拉 value，`name` 或 `modelLabels[id]` 作为显示 label）。

---

## 四、VendorAdapter 统一接口层（核心抽象）

> 文件位置：`service/vendor_adapter.go`（新建）  
> 原则：**所有和供应商平台相关的差异，全部收敛在 Adapter 内部；业务层只看统一入参 / 出参。**

### 4.1 账户 & 鉴权：OAuth + Cookie/AK 双路径（P1 实际采用）

考虑到 UpDream / LibTV / NewWow 官方未必都开放 OAuth 授权，**P1 实际实现采用「浏览器插件自动采集 Cookie / AccessKey」+「手动粘贴兜底」双路径绑定**，把 OAuth 作为后续扩展预留。

#### 4.1.1 双路径总体链路

```
┌────────────────────── 前端 AppConfigModal ────────────────────────┐
│  Segmented 切换供应商卡片：未绑定 → 点击打开绑定子弹窗                 │
│  ├─ Tab1: 🧩 浏览器插件（推荐）                                     │
│  │     · 显示后端地址 + JWT Token（一键复制给插件）                   │
│  │     · 步骤：安装插件 → 填地址+Token → 去官网登录 → 一键提交       │
│  │     · 插件后台服务：chrome.cookies 按域聚合 → POST /api/v1/      │
│  │       vendor/bind-cookie 携带 Authorization: Bearer <JWT>        │
│  └─ Tab2: ✋ 手动粘贴（兜底）                                        │
│        · TextArea 粘贴 Cookie 字符串 / AK/AS/AppKey                  │
│        · 前端直接 fetch POST /api/v1/vendor/bind-cookie             │
└──────────────────────────────────┬─────────────────────────────────┘
                                   ▼
                   ┌───────── 后端 Handler ─────────┐
                   │  POST /api/v1/vendor/bind-cookie│
                   │  1.鉴权 JWT（必须登录）           │
                   │  2.入参: {vendorType,cookie,AK} │
                   │  3.调用 service.BindVendorByCookie │
                   └────────────┬───────────────────┘
                                ▼
                   ┌───── Service.VerifyVendorCookieWithSpec ────────┐
                   │  按 vendorType 分发 → 调用 adapter.VerifyLogin- │
                   │  Credentials（SafeProxyHTTPClient 域名白名单防   │
                   │  SSRF）→ 调供应商 /me /currentUser /account 等   │
                   │  接口：① 200 + 无未登录关键词 → 解析 displayName │
                   │  / vendorUserId / 余额 / expiresAt；② 失败返回   │
                   │  清晰报错。                                       │
                   └───────────────────┬─────────────────────────────┘
                                       ▼
                   ┌───────────── 持久化（加密）────────────────────┐
                   │  Upsert user_vendor_accounts:                  │
                   │  · access_token = AES-GCM( JSON{ cookie, AK } )│
                   │  · is_active = true（首次绑定自动激活）          │
                   │  · displayName / vendorUserId / expires_at     │
                   │  · 异步拉一次模型快照 → available_models_json   │
                   └────────────────────────────────────────────────┘
```

#### 4.1.2 浏览器插件（MV3，项目根：`vendor-browser-extension/`）

| 文件 | 作用 |
|---|---|
| `manifest.json` | MV3：`cookies`, `storage`, `host_permissions` 覆盖 3 家官网；content_scripts 注入气泡；action.default_popup |
| `background.js` | ① 收到"抓取 Cookie"命令 → 按 `VENDOR_SPEC[vendorType].cookieDomains` 聚合 `chrome.cookies.getAll()` → 组装 `k=v; k2=v2` 字符串；② 收到"提交到项目"命令 → 用用户填的 `projectApiBase` + `projectJwt` 直接调用 `POST /api/v1/vendor/bind-cookie`；③ 轮询登录态：定时去供应商 `/me` 接口判断已登录时给 content.js 发消息。 |
| `content.js` | 仅匹配 3 家官网域名；检测到用户已登录 → 页面右上角注入固定气泡"无限画布项目已捕获你的登录态"→ 点"一键提交到项目" → sendMessage 给 background → 调 bind-cookie 成功 → 气泡变绿色✅。 |
| `popup.html` / `popup.js` | ① 顶部输入：Project API Base（默认从页面自动带 `protocol://host`）、Project JWT Token（粘贴）；② 中部 3 张卡片：UpDream / LibTV / NewWow → 显示"未登录 / 已登录 [昵称] / 已绑定到项目"三状态；③ 按钮：「去官网登录」（新标签打开官方首页）→「检测登录态」→「提交到项目」；④ 提交成功后返回 `bound.account.displayName` 和 `vendorType`，并提示回到项目配置页点刷新。 |

> **安全**：Cookie / AK 只在**插件内存 + 后端加密列**两处存在，插件 storage.local 不持久化敏感字段，仅存 `projectApiBase` 和最近一次成功后的 `vendorUserId` 标识用于状态展示。

#### 4.1.3 `POST /api/v1/vendor/bind-cookie` 接口契约

请求（application/json）：
```json
{
  "vendorType":  "updream | libtv | newwow",
  "cookieString": "session=xxx; uid=yyy; ...",   // 和 accessKey 至少填一项
  "accessKey":   "LIBTV_AK_xxx",                // 可选
  "accessSecret":"LIBTV_AS_xxx",                // 可选
  "appKey":      "xxx",                         // 可选
  "vendorUserId":"xxx",                         // 可选，AK 模式必填时由 adapter 校验
  "displayName": "昵称覆盖",                      // 可选，为空则由 VerifyLoginCredentials 解析
  "expiresAt":   "2026-09-16T12:00:00+08:00"     // 可选
}
```

响应（成功 200）：
```json
{
  "vendorType": "updream",
  "account":    {
    "id": "uva_xxx",
    "vendorType": "updream",
    "displayName": "自由画布用户 007",
    "vendorUserId":  "ud_123456",
    "balanceInfo":   { "currency":"CNY", "balance":"88.50", "credits":999 },
    "expiresAt":     "2026-09-16T12:00:00+08:00",
    "hasModels": false,
    "isActive":  true,
    "boundAt":   "2026-08-16T20:00:00+08:00"
  }
}
```

失败 400/401 示例：
```json
{ "code": 401, "message": "UpDream Cookie 已过期或无效，官网返回“请先登录”" }
{ "code": 400, "message": "libtv AccessKey 必须同时提供 vendorUserId" }
```

后端内部关键安全点（`service/vendor.go`）：
1. **SSRF 防护**：所有对供应商的 Verify 请求必须走 `SafeProxyHTTPClient`；Adapter 的 `VerifyLoginCredentials` 只允许命中 `APIHostMatch` 白名单域名 + 公网非内网 IP。
2. **Cookie 有效性双重校验**：HTTP 200 但 body 包含 `未登录 / 登录失效 / invalid token / please login` 关键词也视为失败（`looksLikeUnauthorized()`）。
3. **加密存储**：`access_token` 列在应用层 AES-256-GCM 加密（或透明加密列，按项目现有加密方案），序列化结构：
   ```json
   { "kind":"cookie|ak|mix", "cookie":"k=v;...", "accessKey":"xxx", "accessSecret":"xxx", "appKey":"xxx" }
   ```
4. **并发安全**：同用户+同 vendorType 同时多次 bind 使用 `singleflight` 合并，避免重复请求供应商。

#### 4.1.4 凭证校验接口抽象（Adapter 层）

在 `VendorAdapter` 账户鉴权分组中新增：

```go
// VerifyCredentialsParams 传入的登录凭证（Cookie / AK 组合）
type VerifyCredentialsParams struct {
    CookieString string
    AccessKey    string
    AccessSecret string
    AppKey       string
    VendorUserID string
}

// CredentialVerifyResult 校验通过后得到的基础账户信息
type CredentialVerifyResult struct {
    Valid        bool
    VendorUserID string
    DisplayName  string
    AvatarURL    string
    ExpiresAt    *time.Time
    // 用于 balanceInfoJSON，结构任意，按适配器自行定义
    BalanceInfo  map[string]any
    // 调试信息：供应商原始响应体（日志用）
    TraceID      string
}
```

---

#### 4.1.5 无开放平台供应商的接口学习（capture-sample）

UpDream / NewWow 没有开放平台（不像 LibTV 有 AccessKey/SecretKey 官方开放接口），后端**预先不知道它们的生图内部接口长什么样**。采用「用户在官网真实生成一次 → 插件抓样本 → 后端学习并重放」的路线：

```
用户在 UpDream/NewWow 官网点「生成」
   │  （浏览器里真实发起的 POST 请求）
   ▼
插件 content.js  monkey-patch fetch / XMLHttpRequest
   │  仅捕获目标域 POST/PUT/PATCH，排除 login/auth/password 等敏感接口
   │  记录 url / method / 请求头 / 请求体 / 响应状态 / 响应体（各截断 64KB）
   ▼
插件 background.js  storeSample → chrome.storage.local（按供应商分组、按 url+method+body 去重）
   │
   ▼ 用户回到插件弹窗「生成样本嗅探」→ 点「推送」
插件 background.js  pushSamplesToBackend → 经「无限画布 Web 地址」代理
   │  POST {webBase}/api/v1/vendor/capture-sample（带 Authorization: Bearer <JWT>）
   │  （走 Next.js /api/* 代理转发到 Go 后端，同源、无 CORS）
   ▼
后端 handler.VendorCaptureSample
   │  校验：必须已绑定该供应商账户（否则无 Cookie 可重放）
   │  启发式：isLikelyGeneration（命中 image/generate/prompt…且非登录接口）、endpointGroup（URL 归一化）
   ▼
vendor_api_samples 表落库（user_id + vendor_type + 样本）
```

后续 UpDream / NewWow 的 `VendorAdapter.GenerateImage` 即可读取该用户最新的「生成类样本」，提取其真实请求 URL / 头 / 体模板，把用户的 prompt 填进去，并带上用户自己绑定账户的 Cookie 重放，从而复用供应商内部接口完成生图。

#### 4.1.5.1 样本重放（GenerateImage 消费样本）

实现位置：`service/vendor_replay.go`（通用基类 `replayVendorAdapter`）+ `service/vendor_updream.go` / `service/vendor_newwow.go`（仅注册）。两家的重放逻辑完全一致，只差展示名。

重放算法（`replayVendorAdapter.GenerateImage`）：

1. 取账户凭据：`vendorAccountCredentials()` 拿 `AccessToken`（约定里 Cookie 复用该字段）→ 若为空报错"未绑定 Cookie"。
2. 读样本：`ListVendorApiSamples(userID, vendorType, onlyGeneration=true, limit=1)` 取该用户**最新一条生成类样本**；没有则报错引导用户先去官网采集一次。
3. 解析样本请求：
   - URL：样本可能存的是相对路径（如 `/api/txt2img`），`normalizeReplayURL()` 用捕获请求头的 `Referer` / `Origin` 推导 origin 拼成绝对 URL；已是绝对 URL 则直接用。
   - 头：`parseHeadersJSON()` 解出 map，**保留**非 Cookie 头（含 CSRF Token、自定义签名头、UA、Referer），用账户**当前** Cookie 覆盖 `Cookie` 头，去掉 `Host` / `Content-Length` 等 hop-by-hop 头（由 http 客户端重算）。
   - 体：`buildReplayBody()` 把本次 `prompt` 注入样本请求体——
     - JSON：`injectPromptValue()` 深度优先把第一个命中的 prompt 候选键（`prompt / text / input / content / description / …`）字符串值替换为新 prompt；若有 `negative_prompt` 且输入带负向提示则一并替换；重排成 `application/json`。
     - 表单：`application/x-www-form-urlencoded` 同理按候选键 `Set`。
     - 其他（multipart 等）：明确报错"暂不支持，请重新采集标准 JSON/表单生图请求"。
4. 重放：用 `SafeProxyHTTPClient()`（SSRF 内网屏蔽）发请求；HTTP 非 2xx 透传报错。
5. 提取图片：`extractReplayImages()` 优先按已知图片键（`imageUrl / url / src / resultUrl / picUrl / imageUrls / urls / …`）收集；找不到再全树扫描像图片 URL 的字符串；再不行正则兜底扫描原文；去重后映射成 `GeneratedAssetItem{url}`。
6. 若响应里**一个图片 URL 都没解析到**：报错提示该供应商可能走异步生成（先返回任务 ID 再轮询），当前样本重放仅支持**同步返回图片直链**，引导用户确认采集样本响应包含图片 URL。

> 局限（P1 范围，已在 `docs/progress/pending-test.md` 登记）：仅支持同步返回图片地址、JSON/表单请求体；异步生成与 multipart（图生图上传）暂不支持；重放依赖样本里的非 Cookie 头与 Cookie 一致，**采集应在最新登录后立刻进行**，改绑后需重新采集。

> 说明：本路线不依赖任何第三方 MCP / 自动化登录工具，完全由「用户自己登录 + 插件抓包 + 后端带凭据重放」构成，符合项目"用户自备账号"的定位。

### 4.2 接口定义

```go
package service

import (
    "context"
    "io"
    "time"
    "github.com/tigerowo/freedom/model"
)

// ========== 通用返回 ==========

// GeneratedAssetItem 统一生成输出（图片/视频/音频通用）
type GeneratedAssetItem struct {
    ID          string // 供应商侧任务/资产 ID
    URL         string // 供应商 CDN URL（临时或永久）
    StorageKey  string // 如果触发了双写，这里是本项目 S3/WebDAV 的 key
    Data        []byte // 内联返回的字节（小图/短音频可选，否则留空走 URL 下载）
    Width       int
    Height      int
    DurationMs  int    // 视频 / 音频时长
    Bytes       int
    MimeType    string // 如 image/png, video/mp4
    RawExtra    map[string]any // 供应商透传字段（如水印、种子数等）
}

// GenerateMediaOutput 多结果统一输出
type GenerateMediaOutput struct {
    Items   []GeneratedAssetItem
    RawBody string // 供应商原始响应体（用于日志/排错）
    TraceID string // 供应商请求 ID（对接客服用）
}

// VendorModelInfo 模型信息
type VendorModelInfo struct {
    ID          string            // 模型 ID（请求用）
    Name        string            // 显示名（中文）
    Capability  string            // image / video / text / audio
    DefaultFor  string            // 可选：建议作为 imageModel / videoModel / textModel / audioModel 默认值
    Supports    map[string]bool   // 能力开关：如 { "refVideo":true, "genAudio":true }
    Constraints  map[string]any   // 约束：如 { "maxSeconds":15, "sizes":["1024x1024"] }
    ModelLabels map[string]string // 别名映射（同 id 可覆盖显示）
    Extra       map[string]any   // 其他
}

// VendorModels 分组模型列表
type VendorModels struct {
    ImageModels []VendorModelInfo
    VideoModels []VendorModelInfo
    TextModels  []VendorModelInfo
    AudioModels []VendorModelInfo
}

// VendorAsset 供应商侧素材库条目
type VendorAsset struct {
    ID           string    // 供应商资产 ID
    VendorType   string
    Name         string
    Kind         string    // image / video / audio / project / scene / character
    ThumbnailURL string
    SizeBytes    int64
    Width        int
    Height       int
    DurationMs   int
    MimeType     string
    Tags         []string
    CreatedAt    time.Time
    UpdatedAt    time.Time
    RawExtra     map[string]any
}

// AssetFilter 资产库筛选
type AssetFilter struct {
    Kind       string   // image/video/audio，空=全部
    Keyword    string
    Tags       []string
    Page       int
    PageSize   int
}

// ========== 生成入参 ==========

type GenerateImageInput struct {
    Prompt        string
    Model         string
    Size          string // "1024x1024" / "1:1"（Adapter 内部折算）
    Count         int
    Quality       string // auto/low/medium/high
    NegativePrompt string
    Seed          *int64
    // 参考图：统一传本项目 StorageKey 或 URL，Adapter 内部下载字节再按供应商要求上传
    ReferenceImages []ReferenceImageInput
    Extra           map[string]any // 供应商专属参数（透传，如步骤数、CFG）
}
type ReferenceImageInput struct {
    URL        string // 本项目可访问的 URL（如 /api/files/xxx 或供应商 CDN）
    StorageKey string // 可选，避免重复下载
    Kind       string // init / reference / mask / controlnet
    Weight     *float64
}

type GenerateVideoInput struct {
    Prompt           string
    Model            string
    Seconds          int
    Size             string // "1280x720" 等
    FPS              int
    NegativePrompt   string
    ReferenceImages  []ReferenceImageInput // 首帧参考
    ReferenceVideo   *ReferenceImageInput  // 视频参考（R2V / 续镜）
    ReferenceAudio   *ReferenceImageInput  // 音频参考
    GenerateAudio    bool
    Watermark        bool
    Seed             *int64
    Extra            map[string]any
}

type GenerateTextInput struct {
    Model       string
    SystemPrompt string
    Messages     []ChatMessage // 复用现有结构
    Temperature  *float64
    MaxTokens    *int
    Stream       bool // 是否需要流式（当前项目图片/视频生成场景少，文本用多）
    Extra        map[string]any
}
type ChatMessage struct {
    Role    string // system / user / assistant
    Content string // 纯文本；多模态放在 ReferenceImages（对文本场景一般为空）
}

type GenerateTextOutput struct {
    Text     string   // 非流式直接返回全文
    Chunks   <-chan string // 流式：逐块推送（Adapter 闭包生成）
    Usage    *TokenUsage
    RawBody  string
    TraceID  string
}
type TokenUsage struct {
    PromptTokens   int
    CompletionTokens int
    TotalTokens    int
    CostCredits    int // 供应商侧返回的本项目 Credits 等价额（可选）
}

// ========== 主接口 ==========

// VendorAdapter 每家供应商必须实现的全部能力（没用到的方法返回 ErrNotSupported 即可）
type VendorAdapter interface {
    // ── 账户 & 鉴权 ──
    // BuildOAuthAuthorizeURL 生成跳转授权地址，state 用于防 CSRF
    BuildOAuthAuthorizeURL(ctx context.Context, vendor *model.Vendor, state string) (string, error)
    // ExchangeOAuthCode 回调拿 code 换 token；返回已填好字段的 UserVendorAccount（还未入库）
    ExchangeOAuthCode(ctx context.Context, vendor *model.Vendor, code string, redirectURI string) (*model.UserVendorAccount, error)
    // RefreshAccessToken 用 refresh_token 换新 access_token；**直接修改 account 字段**，调用方负责存库
    RefreshAccessToken(ctx context.Context, account *model.UserVendorAccount) error
    // GetAccountInfo 拉账户资料 + 余额，写入 account.DisplayName / BalanceInfoJSON / VendorUserID
    GetAccountInfo(ctx context.Context, account *model.UserVendorAccount) error
    // VerifyLoginCredentials 使用用户提供的登录凭证（Cookie 字符串 / AccessKey 组合）去供应商鉴权接口校验有效性并解析基础用户信息
    VerifyLoginCredentials(ctx context.Context, params VerifyCredentialsParams) (*CredentialVerifyResult, error)

    // ── 模型 ──
    ListModels(ctx context.Context, account *model.UserVendorAccount) (*VendorModels, error)

    // ── 生成（核心） ──
    GenerateImage(ctx context.Context, account *model.UserVendorAccount, input GenerateImageInput) (*GenerateMediaOutput, error)
    GenerateVideo(ctx context.Context, account *model.UserVendorAccount, input GenerateVideoInput) (*GenerateMediaOutput, error) // 视频一般异步：返回 Items[0].ID=任务ID，状态=queued；另配 GetVideoTaskStatus
    GenerateAudio(ctx context.Context, account *model.UserVendorAccount, input GenerateAudioInput) (*GenerateMediaOutput, error)
    GenerateText(ctx context.Context, account *model.UserVendorAccount, input GenerateTextInput) (*GenerateTextOutput, error)

    // ── 异步任务状态（视频 / 大图生成常用） ──
    GetTaskStatus(ctx context.Context, account *model.UserVendorAccount, taskID string) (*TaskStatus, error)
    CancelTask(ctx context.Context, account *model.UserVendorAccount, taskID string) error

    // ── 资产库 ──
    ListAssets(ctx context.Context, account *model.UserVendorAccount, filter AssetFilter) ([]VendorAsset, int /* total */, error)
    DownloadAsset(ctx context.Context, account *model.UserVendorAccount, assetID string) (reader io.ReadCloser, mimeType string, size int64, err error)
    UploadAsset(ctx context.Context, account *model.UserVendorAccount, name string, kind string, data io.Reader, size int64, mimeType string) (*VendorAsset, error)
    DeleteAsset(ctx context.Context, account *model.UserVendorAccount, assetID string) error
}

// TaskStatus 异步任务统一状态
type TaskStatus struct {
    ID        string
    Status    string   // queued / processing / completed / failed / canceled
    Progress  int      // 0-100
    Message   string   // 失败原因 / 当前阶段
    OutputURL string   // 完成后的 CDN URL
    Output    *GeneratedMediaOutput // 若适配器能直接拼好 Items 就填
    Extra     map[string]any
}

// GenerateAudioInput（简单版，音频字段跟视频类似但少）
type GenerateAudioInput struct {
    Model           string
    Text            string
    Voice           string
    Format          string // mp3 / wav / flac
    Speed           *float64
    Instruction     string // 风格描述
    ReferenceAudio  *ReferenceImageInput // 声音克隆参考
    Extra           map[string]any
}

// ── 注册中心（Go 包 init() 时各自注册） ──

var adapterRegistry = make(map[string]func(vendor *model.Vendor) VendorAdapter)

func RegisterVendorAdapter(vendorType string, factory func(vendor *model.Vendor) VendorAdapter) {
    adapterRegistry[vendorType] = factory
}
func NewVendorAdapter(vendor *model.Vendor) (VendorAdapter, bool) {
    f, ok := adapterRegistry[strings.ToLower(vendor.Type)]
    if !ok { return nil, false }
    return f(vendor), true
}
```

### 4.2 ErrNotSupported 设计

对某个适配器不需要的能力（比如某家供应商只做视频，不提供资产库）：

```go
var ErrNotSupported = errors.New("vendor adapter: operation not supported")

// 示例：LibTVAdapter 不提供资产库上传
func (a *libTVAdapter) UploadAsset(...) (*VendorAsset, error) {
    return nil, ErrNotSupported
}
```

上层调用方判断 `errors.Is(err, ErrNotSupported)` → 给用户弹"该供应商暂不支持素材上传"。

### 4.3 各供应商 Adapter 骨架文件

| 文件 | 做什么 |
|---|---|
| `service/vendor_official.go` | 包住现有 `SelectModelChannelForModel()` + `proxyAIRequest` 逻辑，把官方也套成一个 Adapter（可选，推荐做；不做也行，official 走原路径） |
| `service/vendor_replay.go` | 通用「样本重放」基类 `replayVendorAdapter`：被 UpDream / NewWow 复用，完整实现 VendorAdapter 接口（GenerateImage 读样本重放，其余能力按 P1 范围返 ErrNotSupported） |
| `service/vendor_updream.go` | UpDream 注册：仅 `init()` 调 `newReplayAdapter(...)`，显示名 "UpDream" |
| `service/vendor_libtv.go`   | LibTV 专属（AccessKey/SecretKey 开放平台，标准 HMAC-SHA1 签名） |
| `service/vendor_newwow.go`  | NewWow 注册：仅 `init()` 调 `newReplayAdapter(...)`，显示名 "NewWow" |

#### 4.3.1 LibTV 视频生成（Kling）契约

LibTV 开放平台视频走与图片**同一套 webui 模板体系**（HMAC-SHA1 签名 + `templateUuid` + `generateParams`），仅提交端点不同：

| 类型 | 提交端点 | templateUuid | generateParams 关键字段 |
|---|---|---|---|
| 文生视频 | `POST /api/generate/video/kling/text2video` | `61cd8b60d340404394f2a545eeaf197a` | `prompt` + `model`(`kling-v2-6`) + `aspectRatio`(`16:9/9:16/1:1`) + `duration`(`5/10`) + `mode`(`pro`) + `sound`(`on/off`) |
| 图生视频 | `POST /api/generate/video/kling/img2video` | `180f33c6748041b48593030156d2a71d` | `prompt` + `model` + `duration` + `mode`；kling-v2-6 用 `images:[首帧URL]`，旧版用 `startFrame` |

- 轮询与图片一致：`POST /api/generate/webui/status`（body `{generateUuid}`），`generateStatus==5` 成功，6/7 失败。
- 成功响应视频 URL 在 `data.videos[].videoUrl`（图片在 `data.images[].imageUrl`）。
- 契约来源：社区 SDK（liblib-ai-gen）+ godeps/aigo 引擎库，**待真机验证**。
- 实现状态：`service/vendor_libtv.go` 已实现 `SubmitVideo`（提交返回 generateUuid，供异步任务链路）+ `GenerateVideo`（同步轮询，复用 SubmitVideo）、`ListModels` 返回两个视频模型（文生/图生）、`GetTaskStatus` 支持视频结果提取；模型判定按模板 UUID / 名称含「文生视频/图生视频」优先，图生视频必须带首帧。
- **已接分发**：`handler/video_task.go` 的 `proxyAIVideoTaskRequest` 顶部已加供应商视频分发（`dispatchVendorVideoProxy`，类型断言 `service.VendorVideoSubmitter`），提交拿 `generateUuid` 创建带 `vendor_type` 标记的 `VideoTask`；轮询阶段 `pollVideoTaskFromUpstream` 识别供应商任务改调 `adapter.GetTaskStatus`（`pollVendorVideoTask`）。UpDream / NewWow 未实现 `SubmitVideo`，视频请求会收到"暂不支持视频生成"提示。

每一个文件的 `init()` 里做注册：

```go
// service/vendor_updream.go
func init() {
    RegisterVendorAdapter(model.VendorTypeUpDream, func(v *model.Vendor) VendorAdapter {
        return &upDreamAdapter{vendor: v, client: &http.Client{Timeout: 600 * time.Second}}
    })
}
```

这样加新供应商 = 新写一个文件 + 一个 init()，**完全零侵入现有代码**。

---

## 五、后端 Handler & Service 改造点

### 5.1 新增 HTTP API（router/router.go 追加）

| Method | Path | 说明 |
|---|---|---|
| GET | `/api/vendors` | 前端下拉用：列出所有 `enabled=true` 的供应商（脱敏，不含 client_secret） |
| GET | `/api/vendors/:type/oauth/url` | 返回该供应商 OAuth 跳转 URL（带 state=随机串，state 存在 session/用户配置里） |
| GET | `/api/vendor/oauth/callback/:type` | OAuth 回调地址：拿 code → 换 token → 存 `UserVendorAccount` → 设 is_active=true → 跳转前端配置页 |
| GET | `/api/vendor/account` | 前端查当前用户已绑定的供应商列表（含 is_active、余额、昵称，不含 token） |
| POST | `/api/vendor/account/:type/activate` | 切换激活账户（把其他设 is_active=false，这条设 true），并拉一次模型快照写入 available_models_json |
| POST | `/api/vendor/account/:type/refresh-models` | 手动触发刷新该账户的模型快照（调 adapter.ListModels） |
| POST | `/api/vendor/account/:type/unbind` | 解绑（软删除或直接删都行）；如果当前激活的就是它，自动切回 official |
| GET | `/api/vendor/assets` | 代理 `adapter.ListAssets`（带分页、筛选） |
| GET | `/api/vendor/assets/:type/:assetId` | 代理 `adapter.DownloadAsset`（字节流返回，顺便双写一份到本项目存储） |
| POST | `/api/vendor/assets/:type/upload` | 代理 `adapter.UploadAsset`（字节流 + multipart） |
| **POST** | **`/api/v1/vendor/bind-cookie`** | **【P1】通过 Cookie / AccessKey 绑定供应商账户：插件 & 手动粘贴双路径的统一后端入口** |

### 5.2 现有 AI 代理入口改造（最小改动）

**文件**：`handler/ai.go`

在 `proxyAIRequest(w, r, path)` 函数**最顶部**加供应商拦截（原代码约第 111 行）：

```go
func proxyAIRequest(w http.ResponseWriter, r *http.Request, path string) {
    startedAt := time.Now()
    body, contentType, modelName, err := readAIRequest(r)
    if err != nil { ... }
    user, ok := service.UserFromContext(r.Context())
    if !ok { Fail(w, "未登录或权限不足"); return }

    // ====== 新增：供应商分发 ======
    activeAccount, err := service.GetActiveVendorAccount(user.ID)
    if err == nil && activeAccount != nil && activeAccount.VendorType != model.VendorTypeOfficial {
        vendor, _ := service.GetVendorByType(activeAccount.VendorType)
        if vendor == nil { Fail(w, "供应商已下线"); return }
        adapter, ok := service.NewVendorAdapter(vendor)
        if !ok { Fail(w, "供应商适配器未注册"); return }

        // 自动刷新 Token（带 singleflight 防并发重复刷新）
        if needsRefresh(activeAccount) {
            if sErr := service.SingleflightRefreshToken(r.Context(), activeAccount, adapter); sErr != nil {
                Fail(w, "供应商授权已过期，请重新绑定："+sErr.Error())
                return
            }
        }

        // 分发：根据 path 判是生图 / 文本 / 语音等
        if dispatchByVendorAdapter(w, r, path, adapter, activeAccount, body, contentType, modelName, startedAt) {
            return // dispatch 已写响应，直接结束
        }
        // 如果 dispatch 返回 false（某路径未支持），继续 Fallback 走官方原逻辑（可选）
    }
    // ====== 原有逻辑（official 模式 / Fallback）全部不动 ======
    channel, userChannelID, err := selectAIRequestChannel(user, modelName, ...)
    if err != nil { ... }
    // === 原扣 Credits + 代理 ===
}
```

**关键不变量**：`dispatchByVendorAdapter()` 返回的响应**必须保持 OpenAI 兼容格式**（跟原 proxyAIRequest 返回结构一致），这样前端完全不用改。比如生图响应：

```json
{
  "created": 1718000000,
  "data": [
    { "url": "https://cdn.updream.com/xxx.png", "revised_prompt": "..." }
  ]
}
```

内部实现可以是 `adapter.GenerateImage()` → 把 `GeneratedAssetItem.URL` 映射进 `data[].url`，其他字段补齐即可。

### 5.3 Token Refresh 加 singleflight 防竞态

**场景**：用户同时点"生图 x3"，3 个请求同时发现 Token 过期 → 3 个 Refresh 请求并发。上游 RefreshToken 大多是**一次性**（拿新 refresh_token 后旧的作废），并发会导致最后两个拿"已作废的旧 refresh_token"去换新的 → 失败。

**解决**：用 `golang.org/x/sync/singleflight` 按 `account_id` 做幂等合并。

```go
// service/vendor_token_refresh.go
var tokenRefreshGroup singleflight.Group

// SingleflightRefreshToken 并发安全的刷新入口
func SingleflightRefreshToken(ctx context.Context, account *model.UserVendorAccount, adapter VendorAdapter) error {
    v, err, _ := tokenRefreshGroup.Do(account.ID, func() (any, error) {
        // 双检查：等锁过程中可能已被其他 goroutine 刷新好了
        if !needsRefresh(account) {
            return nil, nil
        }
        if err := adapter.RefreshAccessToken(ctx, account); err != nil {
            return nil, err
        }
        account.LastUsedAt = now()
        // 存库
        if saveErr := repository.SaveUserVendorAccount(account); saveErr != nil {
            return nil, saveErr
        }
        return nil, nil
    })
    if err != nil { return err }
    _ = v
    return nil
}

func needsRefresh(a *model.UserVendorAccount) bool {
    if a.TokenExpiresAt == nil { return true } // 没过期时间默认刷一下
    return time.Now().After(a.TokenExpiresAt.Add(-5 * time.Minute))
}
```

---

## 六、前端改造点

### 6.1 AiConfig 加字段（use-config-store.ts）

在现有 `AiConfig` 类型（[use-config-store.ts#L26-L91](file:///f:/trae/wifi/infinite-canvas-main/web/src/stores/use-config-store.ts#L26-L91)）追加：

```typescript
export type AiConfig = {
    // ===== 原有 100% 保留 =====
    channelMode: "remote" | "local";
    baseUrl: string;
    // ...（imageModel / videoModel / models / ... 全都不动）

    // ===== 新增（默认 "official" = 现有行为）=====
    activeVendorType: "official" | "updream" | "libtv" | "newwow";
};
```

`defaultConfig.activeVendorType = "official"`。

### 6.2 resolveEffectiveConfig 顶部分支

在 `resolveEffectiveConfig()` 最顶部（约第 176 行）加：

```typescript
function resolveEffectiveConfig(config: AiConfig, modelChannel: ..., canUseRemoteChannel: boolean) {
    // 新增分支：非官方供应商 → 从供应商账户快照构建 effectiveConfig
    if (config.activeVendorType && config.activeVendorType !== "official") {
        return buildVendorEffectiveConfig(config);
    }
    // ===== 原有 official 逻辑（从 adminHasConfiguredRemote 开始）全部不动 =====
    const adminHasConfiguredRemote = Boolean(modelChannel && ...);
    // ...
}

/**
 * 根据当前激活的供应商账户快照，构建 effectiveConfig。
 * 模型列表、别名等来自 userVendorAccount.availableModelsJson。
 * 业务层看到的仍是标准 AiConfig 结构，不感知供应商。
 */
function buildVendorEffectiveConfig(config: AiConfig): AiConfig {
    const snapshot = useVendorStore.getState().activeAccountModels; // 见下一节
    if (!snapshot) {
        // 还没拿到模型快照时，用 baseConfig，禁止发起生成
        return { ...config, models: [], imageModels: [], videoModels: [], textModels: [], audioModels: [] };
    }
    const pickIDs = (arr: VendorModelInfo[]) => arr.map(m => m.id);
    const labels: Record<string, string> = { ...snapshot.modelLabels };
    for (const m of [...snapshot.imageModels, ...snapshot.videoModels, ...snapshot.textModels, ...snapshot.audioModels]) {
        if (!labels[m.id] && m.name) labels[m.id] = m.name;
    }
    return {
        ...config,
        channelMode: "remote", // 供应商模式固定走后端代理（Adapter 分发）
        models: [...pickIDs(snapshot.imageModels), ...pickIDs(snapshot.videoModels), ...pickIDs(snapshot.textModels), ...pickIDs(snapshot.audioModels)],
        imageModels: pickIDs(snapshot.imageModels),
        videoModels: pickIDs(snapshot.videoModels),
        textModels:  pickIDs(snapshot.textModels),
        audioModels: pickIDs(snapshot.audioModels),
        modelCostLabels: labels, // 用供应商模型别名显示
        // 若供应商提供了默认模型（DefaultFor）就用上，否则选列表第一个
        imageModel: pickDefault(config.imageModel, snapshot.imageModels, "image"),
        videoModel: pickDefault(config.videoModel, snapshot.videoModels, "video"),
        textModel:  pickDefault(config.textModel,  snapshot.textModels,  "text"),
        audioModel: pickDefault(config.audioModel, snapshot.audioModels, "audio"),
        publicChannels: [], // 不混官方云端渠道
        localChannels:  [], // 供应商模式不显示本地渠道 UI（allowCustomChannel 失效）
    };
}
```

### 6.3 新增 useVendorStore（zustand）

专门管供应商绑定状态，不污染 use-config-store：

```typescript
// web/src/stores/use-vendor-store.ts
import { create } from "zustand";
import { persist } from "zustand/middleware";
import { apiGet, apiPost } from "@/services/api/request";

export type VendorType = "official" | "updream" | "libtv" | "newwow";

export type VendorMeta = {
    type: VendorType;
    name: string;
    logoUrl: string;
    enabled: boolean;
    sort: number;
};

export type BoundAccount = {
    vendorType: VendorType;
    isActive: boolean;
    displayName: string;
    avatarUrl?: string;
    boundAt: string;
    balanceText?: string; // 如 "余额 ¥128.50 / Pro 年卡 362 天"
};

export type ModelsSnapshot = { /* §3.3 JSON 结构的 TS 版本 */ };

type VendorState = {
    vendors: VendorMeta[];
    accounts: BoundAccount[];
    activeAccountModels: ModelsSnapshot | null;
    isLoading: boolean;
    loadVendors: () => Promise<void>;
    loadAccounts: () => Promise<void>;
    activateVendor: (type: VendorType) => Promise<void>;
    unbindVendor: (type: VendorType) => Promise<void>;
    refreshModels: (type: VendorType) => Promise<void>;
};

export const useVendorStore = create<VendorState>()(
    persist(
        // ...具体实现调用 5.1 节的 API
        { name: "freedom:vendor_store", partialize: s => ({ vendors: s.vendors, accounts: s.accounts, activeAccountModels: s.activeAccountModels }) }
    )
);
```

### 6.4 AppConfigModal 新增「供应商」Tab

在 `app-config-modal.tsx` 里新增一段 UI（放在「模型渠道切换（remote/local）」那一段**外面**，不影响原有布局）：

```tsx
{/* ===== 新增：云端供应商切换 ===== */}
<Divider orientation="left">云端供应商</Divider>
<div className="space-y-3">
    <div className="text-sm text-muted-foreground">
        选择官方云端 = 现有行为；选择其他平台 = 用对方账号登录，模型 & 接口 & 素材全部走对方官方。
    </div>
    <Segmented
        value={effectiveConfig.activeVendorType}
        onChange={(v) => {
            const t = v as VendorType;
            if (t === "official") {
                updateConfig("activeVendorType", "official");
                void useVendorStore.getState().activateVendor("official");
                return;
            }
            // 非 official：先看有没有绑定
            const bound = useVendorStore.getState().accounts.find(a => a.vendorType === t);
            if (!bound) {
                // 未绑定：打开对应平台的"粘贴 Token / Cookie 绑定"弹窗（见 6.3 绑定流程）
                openBindModal(t);
                return;
            }
            updateConfig("activeVendorType", t);
            void useVendorStore.getState().activateVendor(t);
        }}
        options={vendors.filter(v => v.enabled).map(v => ({
            label: (
                <div className="flex items-center gap-2 px-2">
                    <img src={v.logoUrl} alt="" className="w-4 h-4 rounded" />
                    <span>{v.name}</span>
                    {accounts.find(a => a.vendorType === v.type)?.isActive && <Badge color="green">当前</Badge>}
                </div>
            ),
            value: v.type,
            disabled: !v.enabled,
        }))}
    />
    {/* 如果绑定了，展示账户卡片 */}
    {currentBoundAccount && currentBoundAccount.vendorType !== "official" && (
        <Card size="small" className="mt-3">
            <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                    <Avatar src={currentBoundAccount.avatarUrl}>
                        {currentBoundAccount.displayName?.[0]}
                    </Avatar>
                    <div>
                        <div className="font-medium">{currentBoundAccount.displayName}</div>
                        <div className="text-xs text-muted-foreground">
                            绑定于 {formatDate(currentBoundAccount.boundAt)}
                            {currentBoundAccount.balanceText && <span className="ml-2">· {currentBoundAccount.balanceText}</span>}
                        </div>
                    </div>
                </div>
                <Space>
                    <Button size="small" onClick={() => refreshModels(currentBoundAccount.vendorType)} loading={loading}>
                        刷新模型
                    </Button>
                    <Popconfirm title="确定解绑？解绑后需要重新授权。" onConfirm={() => unbind(currentBoundAccount.vendorType)}>
                        <Button size="small" danger>解绑</Button>
                    </Popconfirm>
                </Space>
            </div>
        </Card>
    )}
</div>

{/* ===== 原有：本地渠道配置（仅 activeVendorType=official 且 allowCustomChannel 才显示）===== */}
{effectiveConfig.activeVendorType === "official" && allowCustomChannel && (
    <>
        <Divider orientation="left">本地渠道（allowCustomChannel 开启）</Divider>
        {/* 原有 Segmented（remote/local）、localChannels 列表…… 完全不动 */}
    </>
)}
```

---

## 七、现有配置项兼容映射表

> 目标：`activeVendorType = "official"` 时，所有现有开关、行为保持和今天完全一致。

| 现有配置 / 开关 | 作用域变化 | 说明 |
|---|---|---|
| `allowCustomChannel`（管理员） | 仅 official 模式 | 切换到 updream 等供应商时，前端直接隐藏"本地渠道"Tab，因为供应商模式不允许用户自带 Key |
| `allowUserRemoteChannel` | 仅 official 模式 | 同上 |
| `channelMode: remote / local` | 仅 official 模式 | 供应商模式强制 remote（所有请求统一走后端 VendorAdapter） |
| 管理员后台 `ModelChannel[]` 配置 | 仅 official 模式 | 作为 `VendorTypeOfficial` Adapter（或原逻辑）的调度依据 |
| `availableModels` / `modelCosts` / `ModelCost` 扣费 | 仅 official 模式 | 供应商模式走供应商账户余额，不扣本项目 Credits（如需镜像计费，可在 Adapter 里上报） |
| `SystemPrompt` / `SystemPrompts.*` | 仅 official 模式 | 供应商模式下，建议把 systemPrompts.image 等作为 `GenerateImageInput.Extra.systemPrompt` 透传给 Adapter，由各家决定是否支持 |
| 用户 `localChannels[]` | 仅 official 模式 | 供应商模式清空，不生效 |
| 用户 `syncStorageConfig` / `syncWebDAVStorageConfig` | **全部模式保留** | 因为 §7.3 资产双写需要存储提供商，完全沿用 |
| 用户 `imageModel / videoModel / ...` 选择记录 | 按 vendorType 隔离 | 建议 `AiConfig` 里按维度改成对象，如 `imageModelByVendor: Record<VendorType, string>`；**或更简单**：切供应商时如果当前值不在新列表，自动换成新列表第一个 |
| 前端现有 API 路径 `/api/v1/images/generations` 等 | **100% 不变** | 后端在 proxyAIRequest 内部分发，前端零感知 |
| 现有存储 provider（S3/WebDAV/Local/IndexedDB） | **全部保留** | 用于双写资产、生成结果缓存、画布项目文件 |

---

## 八、资产双写 & 同步策略（供应商素材库）

你说的「获取资产也是调用接口」，核心问题是**要不要实时调供应商资产接口**。这里推荐 **懒拉取 + 本地索引 + 使用时双写** 策略：

### 8.1 三种资产场景 & 处理

| 场景 | 推荐做法 |
|---|---|
| **画布节点引用了供应商资产**（如角色卡来自 updream） | 首次使用时触发 `adapter.DownloadAsset()` 下载字节，存到本项目 S3/WebDAV；**之后项目里的引用全走本项目 StorageKey**。即使后来解绑 / 供应商下线，画布照样正常打开。 |
| **资产库页面点「供应商素材」Tab** | 首次进入 Tab → 弹同步向导「是否把 UpDream 资产库索引同步到本地？（只同步元数据 + 缩略图，不同步原图）」→ 后台分页 `adapter.ListAssets()` 批量写 `assets` 表（加 `vendor_type` + `vendor_asset_id` 两列标识来源） |
| **用户本地上传的图要回传到供应商**（供应商那边才能继续编辑） | 在资产详情页加「同步到 [供应商名]」按钮 → 点击调 `adapter.UploadAsset()`；默认**不自动双传**，避免用户隐私泄露到第三方。 |

### 8.2 assets 表加列（不破坏现有数据）

现有 `model/asset.go` 的 `Asset` 追加：

```go
type Asset struct {
    // ===== 原有字段全保留 =====
    ID        string `json:"id"`
    UserID    string `json:"userId"`
    ProjectID string `json:"projectId,omitempty"`
    Name      string `json:"name"`
    Kind      string `json:"kind"` // image/video/audio/character/scene/prop
    StorageKey string `json:"storageKey"`
    URL       string `json:"url"`
    // ...

    // ===== 新增：供应商来源标记（可空，空=本项目原生）=====
    VendorType     string `json:"vendorType,omitempty"`     // updream/libtv/...
    VendorAssetID  string `json:"vendorAssetId,omitempty"`  // 供应商侧资产 ID（解绑重绑可还原关联）
    VendorSyncAt   string `json:"vendorSyncAt,omitempty"`   // 上次从供应商同步的时间
}
```

---

## 九、实施分期（推荐 3 期，风险隔离）

| 期数 | 内容 | 预估工时 | 完成后能验证什么 |
|---|---|---|---|
| **P0 · 数据模型 & 空 UI 骨架** | ①建表 `vendors` + `user_vendor_accounts`（repository / model）<br>② AiConfig 加 `activeVendorType`，默认 official<br>③ AppConfigModal 加供应商切换 UI（空壳，点了只调 state，不接真 API）<br>④ 后端 vendors / vendor/account 两个 GET API（返回空数组） | 0.5~1 天 | 前端下拉能看到 4 个选项，切官方和原行为一致，切其他供应商不报错且不生效 |
| **P1 · 接入一家跑通全链路（建议先 UpDream）** | ① vendor_adapter.go 接口 + UpDream Adapter<br>② OAuth 登录 + 存 UserVendorAccount + RefreshToken（singleflight）<br>③ proxyAIRequest 加分发分支，走 UpDream 生图/文本/视频<br>④ VendorAdapter GenerateMediaOutput → OpenAI 兼容映射<br>⑤ 拉模型快照（ListModels）→ available_models_json → 前端下拉正常显示 | 2~3 天 | 用户切 UpDream → 跳 OAuth → 回跳成功 → 模型列表全是 UpDream 官方的 → 点生图走 UpDream 接口 → 返回图片正常显示 |
| **P2 · 资产库 + 补齐其余两家** | ① assets 表加列 + 资产库供应商 Tab（懒拉取 + 双写）<br>② VendorAdapter List/Download/UploadAsset 实现<br>③ LibTV + NewWow Adapter（每家 1~1.5 天，按他们 SDK 复杂度）<br>④ 余额 / 套餐展示、解绑、模型手动刷新、错误提示<br>⑤ 压力测试：10 并发生图 Token 刷新不报错 | 4~6 天 | 4 家全部可切换；供应商素材在资产库能看能用；解绑 / 重绑 / 切换不串数据 |

**P0 结束后即可随时回归**——因为 official 路径一行没改，哪怕 P1 写崩了，前端切回 official 就是今天的项目。

---

## 十、已知风险 & 规避方案（提前看一眼）

| 风险 | 概率 | 影响 | 规避 |
|---|---|---|---|
| **供应商 OAuth 回调用户关闭页面**导致绑定一半 | 中 | 用户以为绑好了但 DB 没 Account 记录 | OAuth URL 的 state 里带上 `user_id`+`ts`，回调后端无论成功失败都给前端跳 `?vendor_result=success|fail&msg=xxx`，前端弹 Toast 明确反馈。 |
| **供应商 Token Refresh 并发炸了**（singleflight 没做或做错） | 中 | 用户批量生图时集体报错，要重新绑定 | 严格上 §5.3 的 singleflight；**并加单测**：开 10 goroutine 并发刷新 needsRefresh=true 的 account，断言只调用一次 adapter.RefreshAccessToken。 |
| **某供应商 API 字段格式跟预期差别很大**（如视频任务轮询方式） | 高 | Adapter 里堆大量 if/else，维护成本高 | Adapter 接口要留足 RawExtra / GenerateMediaOutput.RawBody；**禁止为某家在业务层加特殊判断**，所有差异封在 Adapter 内部。实在复杂就拆 vendor_xxx_video.go / vendor_xxx_asset.go 分文件。 |
| **模型集合混淆**（切完供应商还能看到上一家的模型） | 低 | 用户困惑"这模型到底是谁家的" | 严格按 §6.2：每次 buildVendorEffectiveConfig 都把 `models / imageModels / videoModels` **整组替换**，绝不和上一家合并。并在下拉框加后缀，如"阿普视频 2.0（UpDream）"。 |
| **供应商侧资产 URL 有效期短**（临时签名 1h 过期） | 高 | 用户第二天开项目图全挂 | 严格 §8.1「使用时双写」：**画布里存的永远是本项目 StorageKey**，供应商 URL 只在资产浏览页的缩略图场景用。 |
| **本项目 Credits 和供应商余额两套体系，用户懵** | 中 | 付费认知冲突 | 顶部余额显示处加 Tab 切换：「本项目 Credits：128」 / 「UpDream 余额：¥88.50」；并在生图按钮上方明确提示「本次使用 UpDream 账户余额，约扣 ¥0.32」（Adapter 里预估或拿实时报价）。 |
| **管理员想**禁用某供应商怎么办 | 低 | 影响全部用户 | `vendors.enabled` 设 false → 前端下拉隐藏；且后端 proxyAIRequest 分发前检查，发现停用直接报错「该供应商已被管理员停用，已切回官方」并自动把该 user 的 is_active 切回 official。 |

---

## 十一、关键不变量（开发过程中反复验证）

1. ✅ **任何时刻，`activeVendorType = "official"` 的行为 = 今天代码的行为（逐字节一致）。**
2. ✅ **前端不接触任何供应商 AccessToken / RefreshToken。** Token 只存在后端加密列，前端拿到的永远只有 displayName / balanceText。
3. ✅ **Canvas 项目文件中的资源引用不直接依赖供应商 URL。** 即使是供应商资产，进入项目节点前必须已双写到本项目存储。
4. ✅ **新增一家供应商 = 新增一个 `vendor_xxx.go` 文件 + 一个 init() 注册**，handler / service 业务层不增加 if/else 分支。
5. ✅ **现有 API（/api/v1/images/generations 等）返回结构对外不变。** 所有 Adapter 差异在后端映射掉。

---

## 十二、与现有文档的关系

本设计文档配套参考：

- 现有系统配置结构：[`docs/backend/system-settings.md`](./system-settings.md)
- 现有数据库结构：[`docs/backend/backend-database.md`](./backend-database.md)（**实施 P0 后要同步更新 vendors / user_vendor_accounts / assets 加列 的 DDL 说明**）
- 现有响应结构约定：[`docs/backend/api-response.md`](./api-response.md)（新增 `/api/vendor/*` 接口须遵循 `{code, data, msg}` 格式）

---

> — 文档完 —
