---
title: 数据库说明
description: 当前后端主要数据表与字段说明
---

# 数据库说明

本文档只记录后端当前已经使用的主要数据表。

## 时间字段规则（2026-08-17 确立）

- **首选**：`time.Time`（GORM 标准字段，自动建 `DATETIME(3)` 列；JSON 序列化 RFC3339 Nano，前端 `new Date()` 兼容）
- **历史遗留**：`string`（项目早期风格，存 `time.Now().Format(time.RFC3339Nano)` 字符串；新表 / 新字段 **不要用**，老字段标注 TODO 逐步迁移）
- **现状例外**：`vendors` / `user_vendor_accounts` / `vendor_api_samples` / `balance_holds` 已统一用 `time.Time`；其他老表（users / prompts / video_tasks / canvas_*_tasks / storyboard_tasks / generation_logs / settings / announcements / storage_objects / canvas_projects / workflows / license_keys / user_configs）仍用 `string`，后续按改动清单逐表迁
- **service 层 `now()` 返回值**：跟当前 model 的字段类型对齐，不要混用；新代码统一 `time.Now().UTC()`

## 数据库

后端使用 GORM 管理数据库连接和表结构迁移。

当前使用 MySQL 作为存储驱动：

- `mysql`

当前启动时执行 `AutoMigrate`，自动维护以下表：

- `users`
- `credit_logs`
- `prompts`
- `assets`
- `settings`
- `video_tasks`
- `storyboard_tasks`
- `video_generation_logs`
- `image_generation_logs`
- `canvas_image_tasks`
- `canvas_audio_tasks`
- `canvas_projects`
- `user_configs`
- `storage_objects`
- `license_keys`
- `license_redeem_logs`
- `aff_commission_logs`（邀请返佣流水，详见下文 §返佣）
- `announcements`
- `vendors`（供应商系统级元信息，详见《多供应商云端切换架构设计文档》§3.1）
- `user_vendor_accounts`（用户绑定供应商账户，详见同文档 §3.2）
- `vendor_api_samples`（浏览器插件嗅探的供应商内部接口样本，UpDream/NewWow 无开放平台时用于后端学习接口形状）

后续新增表时再同步补充本文档，未实际使用的规划表不提前写入。

### users

系统用户表。用户基础信息、角色、账户余额（分 cents）和第三方登录标识放在该表中。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键 |
| `username` | string | 用户名，唯一索引 |
| `password` | string | 密码哈希 |
| `email` | string | 邮箱 |
| `display_name` | string | 昵称 |
| `avatar_url` | string | 头像地址 |
| `role` | string | 角色：`user`、`admin` |
| `balance_cents` | number | 账户余额（单位：分，1 元 = 100 cents）。UI 显示为 `¥X.XX` |
| `aff_code` | string | 用户自己的邀请码，唯一索引 |
| `aff_count` | number | 已邀请用户数量，冗余统计字段 |
| `inviter_id` | string | 邀请人用户 ID |
| `github_id` | string | GitHub 用户 ID |
| `linux_do_id` | string | Linux.do 用户 ID |
| `wechat_id` | string | 微信用户 ID |
| `status` | string | 用户状态：`active`、`ban` |
| `last_login_at` | string | 最近登录时间 |
| `extra` | json | 扩展信息，第三方资料按平台命名空间保存，如 `linuxDo` |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

### user_configs

用户级配置和同步数据表。每个用户一行，模型配置、用户存储配置及其他同步数据继续保存在原有 text 字段中。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `user_id` | string | 用户 ID，主键 |
| `model_config` | text | 模型与偏好配置 JSON；S3/R2 和 WebDAV 的自动同步开关分别为 `syncStorageConfig`、`syncWebDAVStorageConfig` |
| `storage_provider` | text | 用户存储配置 JSON，内部结构为 `{ "s3": {...}, "webdav": {...} }`，两类配置可保留但不能同时启用 |
| `image_history` | text | 用户图片历史同步数据 |
| `asset_data` | text | 用户素材同步数据 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

`storage_provider.s3` 保存 Endpoint、Region、Bucket、Access Key、Secret、公开域名和路径前缀；`storage_provider.webdav` 保存 WebDAV 地址、远程目录、用户名和密码/应用密码。自动同步开关不重复写入 Provider；后端下载和删除旧媒体时仍会读取已保存但已停用的 Provider。

### storage_objects

S3/R2 与 WebDAV 共用的媒体文件索引表，不保存画布、素材列表或生成记录。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 文件 ID，前端存储 key 使用 `server:<id>` |
| `provider_id` | string | 创建文件时使用的 S3/R2 或 WebDAV Provider ID |
| `bucket` | string | S3/R2 Bucket；WebDAV 为空 |
| `object_key` | string | Provider 内相对对象路径，唯一索引 |
| `public_url` | string | S3/R2 可选公开地址；WebDAV 为空并通过 `/api/files/:id/content` 读取 |
| `mime_type` | string | 媒体 MIME 类型 |
| `bytes` | number | 文件字节数 |
| `width` | number | 预留字段，当前上传链路未写入，默认 `0` |
| `height` | number | 预留字段，当前上传链路未写入，默认 `0` |
| `sha256` | string | 文件内容摘要 |
| `created_by` | string | 创建用户 ID |
| `created_at` | string | 创建时间 |
| `deleted_at` | string | 预留字段；当前删除链路直接删除索引记录 |

### prompts

提示词表。用于保存公开提示词、内置 GitHub 系统提示词、分类和预览内容。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键 |
| `title` | string | 标题 |
| `cover_url` | string | 封面图 |
| `prompt` | string | 提示词内容 |
| `tags` | json | 标签列表 |
| `category` | string | 分类标识 |
| `preview` | text | Markdown 展示内容，可包含文本、图片、视频链接等 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

`github_url` 仅用于接口返回，不写入数据库。

### assets

素材表。当前用于后台素材库。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键 |
| `title` | string | 标题 |
| `type` | string | 素材类型：`text`、`image`、`video` 等 |
| `cover_url` | string | 封面图 |
| `tags` | json | 标签列表 |
| `category` | string | 分类标识 |
| `description` | string | 描述 |
| `content` | text | 文本或 Markdown 内容 |
| `url` | string | 图片、视频等媒体地址 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

### video_tasks

视频生成任务表。后端创建视频任务后写入该表，后台轮询器每 5 秒统一查询未完成任务并更新进度、完成地址或失败详情；前端刷新、切换页面或关闭浏览器不会影响后端继续轮询。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键，本地任务 ID，优先使用上游 task ID |
| `user_id` | string | 用户 ID |
| `user_display_name` | string | 用户显示名 |
| `model` | string | 模型名称 |
| `channel_id` | string | 模型渠道 ID |
| `channel_name` | string | 模型渠道名称 |
| `source` | string | 任务来源：`video-workbench`、`canvas` |
| `source_id` | string | 来源内 ID，画布任务记录画布节点 ID，视频创作台为空 |
| `upstream_task_id` | string | 上游任务 ID |
| `upstream_video_id` | string | 上游视频 ID，例如 Agnes 的 `video_...` |
| `status` | string | 状态：`queued`、`processing`、`completed`、`failed` |
| `progress` | number | 生成进度，0-100 |
| `seconds` | string | 视频秒数 |
| `size` | string | 视频尺寸 |
| `video_url` | text | 完成后的视频临时 URL |
| `error` | text | 失败摘要 |
| `error_detail` | text | 失败详情或最近一次轮询错误详情 |
| `request_body` | text | 创建任务时的请求摘要 |
| `response_body` | text | 创建任务时的响应摘要 |
| `last_response` | text | 最近一次状态响应摘要 |
| `cost_cents` | number | 创建任务时预扣的金额（分） |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `started_at` | string | 上游开始时间 |
| `completed_at` | string | 完成时间 |
| `last_polled_at` | string | 最近轮询时间 |

后台轮询器按 `status + created_at` 查询未完成任务；旧数据库中如果残留废弃列，不再参与代码查询。

### storyboard_tasks

分镜生成任务表。后端 worker 循环调文本模型，把小说章节逐章整合为分镜剧本；与 `video_tasks`（后端轮询上游视频 API）不同，本表是后端自己调文本模型生成。前端提交任务后轮询拿回进度与已产出分镜，刷新/重开页面后可据项目中的 `storyboardTaskId` 恢复轮询，不再丢失进度。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键，前端可传 `clientTaskId` 幂等 |
| `user_id` | string | 用户 ID |
| `user_display_name` | string | 用户显示名 |
| `model` | string | 文本模型名称 |
| `channel_id` | string | 云端模型渠道 ID |
| `user_channel_id` | string | 用户本地渠道 ID |
| `channel_name` | string | 渠道名称 |
| `source` | string | 固定 `novel-workbench` |
| `source_id` | string | 前端 NovelProject.id，便于关联 |
| `status` | string | 状态：`queued`、`running`、`completed`、`failed` |
| `progress` | number | 生成进度，0-100 |
| `done_count` | number | 已完成章节数 |
| `total_count` | number | 总章节数 |
| `shot_duration` | number | 单条分镜目标时长（秒），注入提示词约束总时长 |
| `script_prompt` | text | 系统提示词（小说→分镜改写风格） |
| `chapters` | longtext | 输入：JSON `[{title, content}]` |
| `assets` | longtext | 输入：JSON `[{alias, type, description, name}]`，可为空 |
| `result` | longtext | 输出：JSON `[{groupIndex, content}]`，逐章追加 |
| `error` | text | 失败摘要 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `started_at` | string | 开始时间 |
| `completed_at` | string | 完成时间 |

后台 worker 按 `status IN (queued, running) + created_at ASC` 拉取待执行任务；已完成/失败超过 30 分钟的任务自动清理。

### video_generation_logs

视频创作台成果历史表。该表保存用户视频生成成果卡片的完整 JSON，并用独立字段做多设备去重、软删除和查询；它不是运行态轮询表，运行态仍由 `video_tasks` 负责。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键，对应前端生成记录 ID |
| `user_id` | string | 用户 ID，多用户数据隔离 |
| `task_id` | string | 后端或上游视频任务 ID |
| `video_id` | string | 上游视频 ID 或生成结果 ID |
| `status` | string | 记录状态：`生成中`、`成功`、`失败` |
| `payload_json` | text | 完整成果卡片 JSON。删除记录会清空该字段 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `deleted_at` | string | 软删除时间，空字符串表示未删除 |

删除成果记录时只软删除当前用户对应记录，并清空该行 `payload_json`；软删除记录保留 7 天用于阻止旧浏览器缓存把已删除记录恢复回来。

### image_generation_logs

生图工作台成果历史表。当前先提供后端表和接口，前端生图工作台后续再接入；字段设计和软删除策略与 `video_generation_logs` 一致。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键，对应前端生成记录 ID |
| `user_id` | string | 用户 ID，多用户数据隔离 |
| `task_id` | string | 图片任务 ID，可为空 |
| `image_id` | string | 图片结果 ID、存储 key 或 URL |
| `status` | string | 记录状态 |
| `payload_json` | text | 完整成果卡片 JSON。删除记录会清空该字段 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `deleted_at` | string | 软删除时间，空字符串表示未删除 |

### canvas_image_tasks

画布图片生成任务表。只用于画布节点生成恢复，不影响生图工作台原接口。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键，本地任务 ID |
| `user_id` | string | 用户 ID |
| `source` | string | 固定为 `canvas` |
| `source_id` | string | 画布来源 ID |
| `node_id` | string | 画布节点 ID |
| `model` | string | 模型名称 |
| `channel_id` | string | 模型渠道 ID |
| `status` | string | 状态：`queued`、`processing`、`completed`、`failed` |
| `progress` | number | 生成进度 |
| `prompt` | text | 提示词 |
| `generation_type` | string | `generation` 或 `edit` |
| `image_url` | text | 完成后图片 URL或第一张图片 URL |
| `image_urls` | JSON | 完成后全部图片 URL，第一项与 `image_url` 一致 |
| `storage_key` | string | 存储对象 key |
| `error` | text | 失败摘要 |
| `error_detail` | text | 失败详情 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `started_at` | string | 开始时间 |
| `completed_at` | string | 完成时间 |

索引：`idx_canvas_image_tasks_user_source_node (user_id, source, source_id, node_id)`

### canvas_audio_tasks

画布音频生成任务表。只用于画布节点生成恢复，不影响原 `/audio/speech` 接口。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键，本地任务 ID |
| `user_id` | string | 用户 ID |
| `source` | string | 固定为 `canvas` |
| `source_id` | string | 画布来源 ID |
| `node_id` | string | 画布节点 ID |
| `model` | string | 模型名称 |
| `channel_id` | string | 模型渠道 ID |
| `status` | string | 状态：`queued`、`processing`、`completed`、`failed` |
| `progress` | number | 生成进度 |
| `prompt` | text | 提示词 |
| `audio_url` | text | 完成后音频 URL |
| `storage_key` | string | 存储对象 key |
| `error` | text | 失败摘要 |
| `error_detail` | text | 失败详情 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `started_at` | string | 开始时间 |
| `completed_at` | string | 完成时间 |

索引：`idx_canvas_audio_tasks_user_source_node (user_id, source, source_id, node_id)`

### canvas_projects

画布项目表。一条画布项目对应一行，完整项目 JSON 保存在 `project_data`，包含节点、连线、聊天会话、画布设置和视口；不拆节点表。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `user_id` | string | 所属用户，与 `id` 组成主键 |
| `id` | string | 画布项目 ID |
| `project_data` | text | 完整 `CanvasProject` JSON |
| `created_at` | string | 项目创建时间 |
| `updated_at` | string | 项目更新时间 |
| `deleted_at` | string | 软删除时间，空字符串表示未删除；超过 7 天由启动时和每天定时任务物理清理 |

索引：`idx_canvas_projects_user_deleted_updated (user_id, deleted_at, updated_at)`、`idx_canvas_projects_deleted_at (deleted_at)`


### vendor_api_samples

浏览器插件嗅探样本表。用户在 UpDream / NewWow 官网浏览器里真实发起一次生成请求，插件把这次请求的 URL / 方法 / 头 / 体 + 响应状态码 / 体抓下来，经 Web 代理 POST 到 `POST /api/v1/vendor/capture-sample` 落库；后端后续据此构造可重放的请求（带该用户自己的 Cookie），从而在无开放平台的供应商上实现云端生图。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string (PK) | 主键，`vs_` 前缀 |
| `user_id` | string (INDEX) | 关联用户，每个用户样本独立（重放需用自己的 Cookie） |
| `vendor_type` | string(32) (INDEX) | 供应商类型：`updream` / `newwow`（LibTV 走开放平台无需嗅探） |
| `url` | longtext | 被捕获请求的完整 URL |
| `method` | string | 请求方法，通常 POST |
| `request_headers_json` | longtext | 请求头 JSON（map） |
| `request_body` | longtext | 请求体（截断 64KB） |
| `response_status` | int | 响应状态码 |
| `response_headers_json` | longtext | 响应头 JSON（map） |
| `response_body` | longtext | 响应体（截断 64KB） |
| `content_type` | string | 响应 Content-Type |
| `is_likely_generation` | bool | 后端启发式判定：是否为生成类接口（命中 image/generate/prompt 等关键词且非登录/鉴权接口） |
| `endpoint_group` | string(255) | 归一化接口分组（URL 中 ID/数字段替换为 `:param`），便于适配器按分组找生成接口 |
| `created_at` | datetime | 创建时间 |

相关接口：`POST /api/v1/vendor/capture-sample`（落库）、`GET /api/v1/vendor/samples`（列出，调试用）、`POST /api/v1/vendor/clear-samples`（清空）。

### settings

系统配置表，只保存两行数据：`public` 放前端可读取的公开配置，`private` 放仅后端和管理员可读取的私有配置，配置值都用 JSON。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `key` | string | 主键：`public`、`private` |
| `value` | json | 配置内容 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

`public.value` 常放前端展示和可公开读取的配置，例如模型列表、登录开关等。  
`private.value` 常放渠道密钥、登录密钥、后台内部开关等。

当前系统设置接口会按后端结构体序列化和反序列化已知字段；数据库 JSON 中额外存在的旧字段会被忽略。

`public.value` 当前字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `modelChannel` | object | 模型渠道公开配置组 |
| `auth` | object | 公开登录配置 |

`modelChannel` 当前字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `availableModels` | string[] | 系统可用模型列表 |
| `modelCosts` | object[] | 模型扣费配置，单位：分（cents） |
| `defaultModel` | string | 默认模型 |
| `defaultImageModel` | string | 默认图片模型 |
| `defaultVideoModel` | string | 默认视频模型 |
| `defaultTextModel` | string | 默认文本模型 |
| `systemPrompt` | string | 系统提示词 |
| `allowCustomChannel` | bool | 是否允许用户自定义渠道，默认允许，关闭后前端只提供走后端渠道的模式 |

`modelCosts` 每项字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 模型名称 |
| `costCents` | number | 每次后端模型接口调用前预扣的金额（分，1 元 = 100 cents），未配置默认不扣除。视频模型可改用 `costCentsPerSecond` 按秒计费 |

`auth.linuxDo` 当前字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | bool | 是否开启 Linux.do 登录 |

`private.value` 当前字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `channels` | object[] | 模型渠道配置列表 |
| `promptSync` | object | GitHub 远程提示词定时同步配置 |
| `auth` | object | 私有登录配置 |

`channels` 每项字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `protocol` | string | 协议，当前支持 `openai` |
| `name` | string | 渠道名称 |
| `baseUrl` | string | 渠道接口地址 |
| `apiKey` | string | 渠道密钥 |
| `models` | string[] | 渠道可用模型列表 |
| `weight` | number | 渠道权重，同一模型命中多个渠道时按权重随机 |
| `enabled` | bool | 是否启用 |
| `remark` | string | 备注 |

`promptSync` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | bool | 是否开启定时同步，默认开启 |
| `cron` | string | Cron 表达式，默认每天 0 点 |

`auth.linuxDo` 当前字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `clientId` | string | Linux.do OAuth App Client ID |
| `clientSecret` | string | Linux.do OAuth App Client Secret，后台返回时隐藏 |

后端请求模型时，先按模型名筛选启用且包含该模型的渠道，再按 `weight` 加权随机选择一个渠道。

### balance_logs

用户余额变更流水表（单位：分 cents）。当前记录后台手动调整、AI 模型调用预扣、调用失败返还和人工补发卡密入账。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键 |
| `user_id` | string | 关联用户 ID |
| `type` | string | 类型：`manual_adjust`、`generation_consume`、`generation_refund`、`manual_recharge` |
| `amount` | number | 本次变动数量（分 cents），增加为正，扣减为负 |
| `balance` | number | 变动后的用户余额（分 cents） |
| `related_id` | string | 关联业务 ID，可为空 |
| `remark` | string | 备注 |
| `extra` | json | 扩展信息 |
| `created_at` | string | 创建时间 |

`type` 当前取值：

| 值 | 说明 |
| --- | --- |
| `manual_adjust` | 后台手动调整 |
| `generation_consume` | 调用后端模型接口消费 |
| `generation_refund` | 后端模型接口调用失败返还 |
| `manual_recharge` | 管理员通过「卡密管理」人工补发到指定用户余额 |

### announcements

系统公告表。首页弹窗按 `created_at` 倒序取最新 10 条；每条公告自动带发布时间。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键 |
| `content` | text | 公告正文，支持多行文本 |
| `created_at` | string | 创建（发布）时间，RFC3339 格式 |
| `updated_at` | string | 最近更新时间，RFC3339 格式 |

### aff_commission_logs

邀请返佣流水表（一级直推返佣，2026-08-23 引入）。用户通过邀请码注册、并在官方托管版产生消费后，按消费额阶梯给直接邀请人记返佣。**采用 T+1 每日批结算**：消费时只写 `pending`，每日 00:10 由 `service/affiliate_settlement_scheduler.go` 聚合入账并改 `settled`。单位均为分（cents）。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 主键 |
| `inviter_id` | string | 拿佣金的邀请人 ID，索引 `idx_aff_inviter` |
| `invitee_id` | string | 被邀请（消费）用户 ID |
| `recharge_id` | string | 关联消费来源 ID（消费占用 `balance_holds.id`），唯一索引，保证同一笔消费只记一次 |
| `recharge_cents` | number | 本次消费金额（分） |
| `rate` | string | 分成比例快照，如 `0.1000` |
| `commission_cents` | number | 实发佣金（分）= recharge_cents × rate |
| `status` | string | `pending`（已记待结算）/ `settled`（已日结入账）/ `cancelled`（已取消）；消费时写 `pending`，批结算后改 `settled` |
| `settled_at` | string | 结算时间（日结入账时刻） |
| `created_at` | string | 创建时间 |

触发逻辑：被邀请人每次模型消费扣费成功后，`service/affiliate.go` 的 `SettleCommissionOnConsume` 写 `pending` 流水（比例读 `settings.private.affiliate` 的阶梯配置，默认不开启）；`users.inviter_id` 在 `service/auth.go` 的 `Register` 与 `LoginWithLinuxDo` 注册分支落地，`aff_count` 同步 +1。每日批结算由 `service.StartAffiliateSettlementScheduler()` 在 `main.go` 启动。
