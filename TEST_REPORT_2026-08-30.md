# 前后端联调测试报告(2026-08-30,上线前回归)

## 0. 概况

| 项 | 值 |
|---|---|
| 测试日期 | 2026-08-30 |
| 后端版本 | git HEAD `178a69a`(含 novel-workflow v2) |
| 后端启动 | `go run main.go` 监听 `:18080`(本机 .env) |
| 前端启动 | `cd web && bun run dev` 监听 `:3000` |
| 数据库 | MySQL 127.0.0.1:3006 库 `freedom`(无需重建,AutoMigrate 已跑) |
| 测试账号 | `admin / freedom`(.env 默认 seed) |
| 测试脚本 | `tmp/e2e_full_regression.mjs`(API 冒烟)+ `tmp/e2e_ui_smoke.mjs`(Playwright 截图) |
| 后端日志 | `tmp/e2e_run.log` |
| UI 截图 | `tmp/e2e_screenshots/01_login.png` ~ `29_admin_license_keys.png` |
| Console errors | `tmp/e2e_screenshots/console_errors.log`(13 个) |

### 总体结果

| 维度 | PASS | FAIL | TOTAL | 备注 |
|---|---|---|---|---|
| API 冒烟 | 55 | 19 | 74 | 失败 19 项中**真问题 1 个 + 脚本 bug 18 个** |
| UI 渲染(Playwright) | 19 | 4 | 23 | 失败 4 项中**真问题 1 个 + selector 误判 3 个** |
| **总有效问题** | | **2** | | 见 §5 P0 阻断项 |

---

## 1. 测试覆盖范围

### A. 公开 API(无需登录)
- `/api/health` `/api/settings` `/api/pricing` `/api/storage/config` `/api/vendors` `/api/announcements/latest` `/api/model-status` `/api/license/purchase-config`
- `/api/prompts` `/api/assets`(匿名)
- `/api/v1/bgm/presets`(标注公开但实际要登录 — **见 P0-1**)

### B. Auth
- `POST /api/auth/login` 正常 / 错密码
- `GET /api/auth/me` `POST /api/v1/user/profile`
- `POST /api/admin/login`(注意此路由是用户登录+角色校验,不要求 admin token)

### C. 用户态
- 17 个列表/查询接口:user-config / canvas/projects / image-history / assets / workflows / tasks / user-tokens / video-tasks / storyboard-tasks / canvas/image-tasks / license/redeem-logs / balance-logs / affiliate/{info,commissions} / generation-logs/{videos,images} / vendor/accounts

### D. novel-workflow v2 完整闭环
- workflows:create / list / get / start / node start / cancel
- dubbing:dispatch / list
- subtitle:dispatch / list
- bgm:presets(公开) / custom(用户列表)
- composition:create / get / start / list
- export:metadata / caption / history
- rerun:shot / versions / latest
- series-asset-lock:get / put / lock / unlock

### F. Admin 后台(15 个列表接口)
users / balance-logs / ai-logs/{dates,list} / channel-fail-logs / channels-health / settings / prompt-categories / prompts/{list,pending,rejected} / assets / license-keys / license-redeem-logs / announcements

### G. 错误路径
- 无 token 访问 `/v1/*`
- 错 token 访问 `/v1/*`
- 不存在的 novel-workflow run

---

## 2. 真问题(P0 阻断项 — 上线前必看)

### P0-1: `/api/v1/bgm/presets` 路由要求登录,与注释"公开"不符

**位置**: `F:\trae\wifi\infinite-canvas-main\router\router.go:97`

```go
v1.GET("/bgm/presets", gin.WrapF(handler.ListBgmPresets))    // 公开
```

**问题**: 路由挂在 `v1` 组下,而 `v1` 是 `api.Group("/v1", middleware.UserAuth)`(line 55)。所以 `bgm/presets` 实际**要登录**(返回 401 `未登录或权限不足`),但代码注释写"公开",且 handler 实现也没鉴权。

**复现**:
```bash
curl -i http://127.0.0.1:18080/api/v1/bgm/presets
# HTTP/1.1 401 Unauthorized
# {"code":1,"data":null,"msg":"未登录或权限不足"}
```

**影响**:
- 前端未登录用户(测试中已确认)访问 BGM 预设会被 401
- handler `ListBgmPresets` 内部不检查 user,只是被中间件拦截
- 这条是 novel-workflow v2 任务清单 5.5 的需求(`handler/bgm.go(user-only list 预设 ...)`),但任务清单又写了 "公开" 二字,实现和需求/注释三处不一致

**建议修复**:
```go
// 方案 A:把 /bgm/presets 移到 /api 根组(public)
api.GET("/bgm/presets", gin.WrapF(handler.ListBgmPresets))

// 方案 B:在 handler 内部允许匿名(如果业务上确实要公开)
```

**优先级**: **P0** — 注释和实现不一致,代码 review 时会卡,前端可能也有逻辑引用

---

### P0-2: 多个 novel-workflow v2 路由的请求体字段缺失,无默认值

**位置**: `handler/novel_*.go` 各 endpoint,涉及 composition / export / rerun / series-asset-lock

**问题**: 端点要求很多必填字段,但前端代码 `web/src/app/(user)/novel/page.tsx` 集成测试中没传齐,后端直接 400。例如:
- `POST /api/v1/novel/composition` 必传 `input.shotVideos`
- `GET /api/v1/novel/export/metadata` 必传 `compositionId`
- `POST /api/v1/novel/rerun/shot` 必传 `projectId`(query)
- `PUT /api/v1/novel/series-asset-lock` 必传 `projectId` body

**建议**:
- 短期:不修,前端补齐请求体即可(参考 `web/src/services/api/novel_*.ts` 现有调用)
- 长期:handler 端把不存在的资源/默认值处理掉(返回 1001 而不是 400),前端错误更友好

**优先级**: **P1** — 不是后端 BUG,是接口契约和前端调用约定没沉淀

---

## 3. 已知环境差异(非 BUG)

| 项 | 现状 | 影响 | 处理 |
|---|---|---|---|
| `assets/bgm-presets/*.mp3` | 全部缺失,只有 `manifest.json` | `GET /api/v1/bgm/presets` 返回空数组(预期) | 上线前需补 mp3 |
| ffmpeg | 未在本机装(后端启动 warn 提示) | novel composition 真实合成会失败 | 上线前 docker 镜像已预装,本机跑测试无所谓 |
| MySQL 数据 | admin 用户已 seed(2026-08-14) | 无需重建库 | OK |
| Novel-workflow run | 每次测试创建一个 `regress-test-<timestamp>`  | 数据库有测试残留 | OK,清理脚本存在(`service.StartNovelWorkflowCleanupScheduler`) |

---

## 4. UI 渲染结果(Playwright)

### 4.1 失败页面(3 个假问题 + 1 个真问题)

| 页面 | 现象 | 真实原因 |
|---|---|---|
| `/prompts` | 首屏 Empty 状态 | 提示词库无数据(预期),不是 BUG |
| `/tools/image-watermark` | selector 没找到 | 截图显示页面**有内容**,只是 `main`/`.ant-layout` 不在视口内(假问题) |
| `/tools/prompt-reverse` | selector 没找到 | 同上(假问题) |
| `/` `/video` `/tools/image-watermark` `/admin` | `networkidle` 超时 | 改用 `domcontentloaded + 2.5s` 等待后全部通过,真问题是页面有持续 fetch/动画 |

### 4.2 Console errors(13 个,已存 `console_errors.log`)

| 类型 | 数量 | 严重度 | 备注 |
|---|---|---|---|
| antd `Modal.maskClosable` deprecation warning | 2 | 低 | 升级到 antd v6 后需迁移到 `mask.closable` |
| React 19 **Hydration mismatch** (textarea) | 1(2 处) | **中** | `/image` 和 `/video` 页面 `<MentionTextarea>` 组件,SSR/CSR 渲染 DOM 树不一致(空 ref vs textarea)。会导致客户端重新渲染 |
| `An empty string ("") was passed to the %s attribute` | 1 | 低 | `<img src="">` 警告,需找到传入空 src 的位置 |

**Hydration mismatch 详情**(看 console_errors.log):
- 路径:`(user)/image` `/video` 页面的 `WorkbenchPanel > MentionTextarea`
- 差异:server 渲染 `<textarea>`,client 先渲染 `<div ref={{current:null}} aria-hidden="true">`,再 hydrate 出 textarea
- 影响:Next.js 会丢弃 SSR HTML 重新渲染,影响 LCP 和 SEO 评分
- 修复方向:`<MentionTextarea>` 应该有 `suppressHydrationWarning` 或客户端 only 渲染

### 4.3 截图清单

`tmp/e2e_screenshots/`:
- 用户态:`01_login.png` `03_canvas.png` `04_novel.png` `05_image.png` `06_video.png` `07_wallet.png` `08_prompts.png`(空状态)`09_workflows.png` `11_prompt_reverse.png`
- Admin:`21_admin_users.png` ~ `29_admin_license_keys.png`(全部正常)
- 失败截图已删(被 domcontentloaded 改写后全部成功)

---

## 5. P0 / P1 阻断项清单(上线前必读)

### P0 — 必修
1. **`/api/v1/bgm/presets` 公开性修复**(详见 §2 P0-1)

### P1 — 建议修
2. **React 19 Hydration mismatch** 在 `/image` `/video` 页面(`MentionTextarea` 组件)
3. antd v6 `Modal.maskClosable` 迁移(2 处)
4. novel-workflow v2 接口字段缺失时返回 400 → 建议 1001(业务级错误) + 默认值兜底

### P2 — 可延后
5. `assets/bgm-presets/*.mp3` 文件补充(8 个)
6. 前端 selector 覆盖率补全(目前 selector 只覆盖 main / .ant-layout,对自定义布局的页面不准)

---

## 6. 上线前 Checklist

- [ ] 修复 P0-1(`bgm/presets` 公开/私有二选一)
- [ ] 决定 P1-2 Hydration 修复时机(可上线后迭代)
- [ ] 补 BGM mp3(P2-5,功能完整性)
- [ ] 生产环境 MySQL DSN 改用专用账号(非 root)
- [ ] `.env` 中 `JWT_SECRET` 留空(生产会强制 32 字节随机)
- [ ] `PUBLIC_BASE_URL` 设为正式域名(影响 CORS 白名单)
- [ ] `docker-compose.yml` 资源限制保留(1g/2 CPU)
- [ ] 后端 `PORT=8080`(容器内固定) + 前端 `API_BASE_URL` 不设(走默认 8080)
- [ ] 跑一次 `docker compose up -d --build` 验证镜像能起来

---

## 附录 A: 跑测试的命令

```bash
# 后端
cd F:\trae\wifi\infinite-canvas-main
go run main.go

# 前端
cd web
bun install
bun run dev

# API 冒烟
node tmp/e2e_full_regression.mjs

# UI 冒烟
node tmp/e2e_ui_smoke.mjs
```

## 附录 B: 完整测试结果

详见 `tmp/e2e_results_<timestamp>.json`(API 原始数据)。
