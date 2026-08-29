# 媒体分发链路审查（S3 直链 / 代理 / nginx 超时）

> 审查时间：2026-08-22
> 范围：图片加载、参考图加载、图片/视频下载、视频播放、大文件/慢网加载中断
> 结论：**前端直链改造基本正确，但「nginx 60s 超时」这一项线上仍未真正解决**，另有 3 处中低风险待补。

---

## 一、已确认正确的部分 ✅

1. **图片直链 CDN（老文件）**
   - `resolveImageUrl` / `resolveMediaUrl` 已优先读 `storage_objects.public_url`，返回非空即用直链，不再走后端 `/api/files/{id}/content`。
   - 后端 `StorageObjectInfo` 在「站点 HTTPS 但 publicUrl 是 HTTP」时清空 publicUrl 回退 content（防混合内容），逻辑正确。
   - 线上 `fix-storage-objects.sql` 把历史 `public_url` 修正为 `:9000/freedom/...`，MinIO 桶已 `anonymous set download` 公开读（`docker-compose.mysql.yml` minio-init），`9000:9000` 已对公网暴露 → 老文件直链在线上是真直链。

2. **参考图加载健壮化**
   - `imageToDataUrl` 加 30s `AbortController` 超时 + 中文错误；`resolveReferenceDataUrls` 单张失败跳过、全失败才抛 → 不再整批 `Failed to fetch`。已实现。

3. **远程媒体下载/上传 415 修复**
   - 新增 `ProxyMedia`（图片+视频，100MB），`downloadRemoteMedia` 改用 `getMediaProxyUrl`；图片路径仍走 `ProxyImage`（仅图片，32MB）。已实现且路由已注册（`router.go:157-158`）。

4. **视频不再被后端整块读内存（本地存储路径）**
   - `FileContent` 优先 `DownloadStorageObjectStream` → `http.ServeFile`（零内存流式）。仅当对象在本地存储时生效。

---

## 二、仍有问题的部分 ⚠️

### ❗1. nginx 60s 超时——线上"慢网老视频加载中断"并未真正解决

- `deploy/hk/nginx.conf` 与 `deploy/hk/nginx-https.conf` **完全没有设置 `proxy_read_timeout` / `proxy_send_timeout` / `send_timeout`**，全部沿用 nginx 默认 **60s**。
- 前端直链改造让**新上传**的视频走 `public_url`（MinIO `:9000` 直链，不经过 nginx 的 `/api/`），这部分确实不受 60s 影响。
- 但**两种老视频仍走 nginx 的 60s 钳制**：
  - (a) 历史数据里 `public_url` 被清空（HTTPS 混合内容保护）的对象 → 前端回退到 `/api/files/{id}/content` → 经 `location /api/` → nginx 60s 超时。
  - (b) 远程 S3 回退路径：`FileContent` 调 `DownloadStorageObject` → `w.Write(download.Data)` **把整个对象读进内存再写回**，视频大文件会撑爆后端内存，且仍被 nginx 60s 卡。
- **修复建议**（改 `nginx-https.conf` 与 `nginx.conf` 的 `/api/` 与 `/` 两个 location）：
  ```nginx
  proxy_read_timeout 300s;
  proxy_send_timeout 300s;
  send_timeout 300s;
  proxy_buffering off;          # 不在 nginx 缓冲整段响应，慢网边收边发
  ```
  同时建议 `FileContent` 远程路径也加 `w.Header().Set("X-Accel-Buffering", "no")` 并改用流式拷贝（`io.Copy(w, resp.Body)`）而非 `io.ReadAll` + `w.Write`。

### ⚠️2. 本地 dev 环境基本不走直链，全部压后端代理

- `config.go` 仅读 `PUBLIC_BASE_URL`（站点地址），**没有 `S3_PUBLIC_BASE_URL` / MinIO 公网基址**相关配置；`objectURL` 的 `provider.PublicBaseURL` 来自管理员配置的存储 provider。
- 本地未配置存储 provider 的 `publicBaseUrl` 时，`objectURL` 返回空串 → 上传返回的 `url = /api/files/{id}/content` → 本地全走后端代理。
- 后端 `FileContent` 本地无对象时回退 `DownloadStorageObject`（远程），本地 MinIO 若未启用则直接 404/失败。
- 影响：你本地开发测试"直链"其实是测不到的，验证需在**线上**（已配 MinIO + publicBaseUrl）进行。这是设计预期，但要在文档里写清，避免误判"本地还是加载不出来=没修好"。

### ⚠️3. `FileContent` 远程 S3 回退仍是整块读内存

- `storage.go:130-142`：`download, err := service.DownloadStorageObject(id)` → `io.ReadAll(io.LimitReader(resp.Body, 100<<20))` → `w.Write(download.Data)`。
- 100MB 上限 + 整块读 + 写，慢网/大视频会占满后端内存并触发 nginx 60s。应改为 `io.Copy(w, resp.Body)` 流式（已对本地路径做流式，远程路径漏了）。

### ⚠️4. 视频播放 src 的真实取值未全程确认直链优先

- `video.ts` 各 `mediaReferenceToFormValue` / 视频结果构建优先用 `publicHttpUrl(resolvedUrl)`（`resolvedUrl` 来自 `resolveMediaUrl` → 优先 `publicUrl`）。逻辑对。
- 但**画布/资产库视频卡片直接 `<video src=...>`** 用的是 `asset.data.url`（`use-asset-store.ts:78/85` 经 `resolveMediaUrl`）。若对象 `publicUrl` 为空（HTTPS 混合内容/未配 provider），则 `url` 回退为 `/api/files/{id}/content` → 仍受 nginx 60s + 后端整块读影响。
- 即：只要 `publicUrl` 缺失，老视频在画布里依旧可能被掐断——与问题 1 同源。

---

## 三、优先级建议与已修复状态

| 优先级 | 项 | 状态 |
|---|---|---|
| P0 | nginx 60s 超时 | ✅ 线上 22:26 已加 300s + buffering（sed）；仓库 `nginx.conf`/`nginx-https.conf` 已同步成一致版本，避免下次部署被旧配置覆盖 |
| P1 | `FileContent` 远程 S3 流式 | ✅ 仓库已落地 `DownloadStorageObjectStreaming` + `X-Accel-Buffering: no`；`go build`/`go vet` 通过。**待下次部署上线**（线上当前仍是旧整块读，但 A 已让老文件走直链，实际压力小） |
| P2 | 文档澄清 | ⏳ 需在 `pending-test.md` 写明：本地 dev 不验证直链，需线上；老对象 publicUrl 缺失时仍走 content |
| P3 | S3 直链 base 配置 | ✅ 线上已配 `publicBaseUrl` 且回填历史 publicUrl，新对象正常直链 |

---

## 四、一句话结论

前 4 个问题的前端侧（直链优先、参考图超时容错、媒体代理 415、本地零内存流式）**已落地且逻辑正确**；
线上"慢网/大文件加载中断（nginx 60s）"**也已通过回填 publicUrl + nginx 300s 超时缓解**，且本次把后端 `/content` 远程路径改为流式（B 项）补进了仓库源码，闭环更稳。
剩一处需你知悉：**本地 dev 环境因未配存储 provider 的 `publicBaseUrl`，全走后端代理，直链效果只能在线上验证**；另 B 项需下次部署才生效。
