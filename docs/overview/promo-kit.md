# Freedom 推广物料包（中文为主）

> 配合 `docs/backend/affiliate-plan.md`（推广码+返佣技术方案）使用。
> 头图已生成：`outputs/promo/A_modern__clean_promotional_he_2026-08-23T00-45-47.png`

---

## 一、项目一句话定位（所有渠道统一用）

**Freedom —— 把画布、AI 生图/视频/音频和提示词库装进同一个工作台的的开源创作神器。**

备选短句（15 字内）：
- 无限画布上的 AI 创作工作台
- 一张画布，搞定图文音视频
- 开源 · 自部署 · AI 创作中枢

---

## 二、推广视频：60 秒演示分镜脚本

目标：让没用过的人 60 秒内看懂「它是什么、爽在哪」。语气轻快、节奏快。

| 秒 | 画面 | 旁白/字幕 |
| --- | --- | --- |
| 0-5 | 头图淡入 + 标题「Freedom 开源 AI 创作工作台」 | 「做图、做视频、做音频，还要来回切工具？」 |
| 5-12 | 展示无限画布：拖出一个节点、连线、小地图一闪 | 「在 Freedom，所有创作都在同一张无限画布上」 |
| 12-22 | 生图节点：输入提示词 → 点生成 → 出图回到画布节点 | 「文生图、图生图、参考图编辑，结果直接回到画布」 |
| 22-32 | 视频节点：选模型、传首帧 → 生成视频卡片 | 「视频也一样，首尾帧、参数全在节点里调」 |
| 32-42 | 3D 导演台：摆角色、设机位、截图自动回画布 | 「还有 3D 导演台，机位画面一键发回画布」 |
| 42-50 | 提示词库 + 画布助手对话生图回插 | 「内置数百提示词，画布助手围着节点对话生图」 |
| 50-58 | 多画布缩略图 + Docker 一键部署命令闪过 | 「多画布项目、本地/云端同步，Docker 一条命令自部署」 |
| 58-60 | 结尾：GitHub 星星 + 二维码/域名 | 「GitHub 搜 Freedom，免费自部署，也能用官方托管版」 |

技术提示：
- 真实录屏最佳（你本地跑起来用 OBS/ShareX 录）。
- 若暂时没法录，我可生成一段「概念演示动画」视频当占位，但那不是真实操作录屏。
- 建议出两个尺寸：竖版 9:16（小红书/B站短视频）、横版 16:9（GitHub/YouTube）。

---

## 三、各平台文案

### 1. LinuxDO 发帖（主阵地，你 README 已引流到这里）

标题：**我做了一个开源 AI 创作工作台 Freedom：一张无限画布，把生图/视频/音频/3D 导演台全连起来了**

正文：

> 大家好，最近把我自己用的一个工具整理开源了，叫 **Freedom**。
>
> 痛点大家都懂：生图去一个站、剪视频去另一个、3D 又要开 Blender，素材散落各处。Freedom 想做的是把这些都放进**同一张无限画布**：
> - 🎨 文生图 / 图生图 / 参考图编辑，结果直接回到画布节点
> - 🎬 视频创作台：首尾帧、参数都在节点里，生成卡片可回画布
> - 🎥 3D 导演台：摆角色、设机位，截图一键发回画布
> - 📚 内置数百条 GitHub 开源提示词，画布助手围着节点对话生图
> - 🐳 Docker 一键自部署，也能用我托管的官方版
>
> 技术栈 Go + Gin + Next.js，MIT 协议。
> GitHub：https://github.com/tigerowo/freedom
> 欢迎 Star、提 Issue，也欢迎用官方版的邀请码互相帮衬～
>
> 有任何想加的功能或槽点，评论区聊。

### 2. Product Hunt 文案（英文，面向海外独立开发者）

**Tagline:** The open-source AI creation canvas — image, video, audio & 3D director in one infinite board.

**First comment:**
> Hi Product Hunt! 👋
> Freedom is an open-source workstation that puts AI image / video / audio generation, a 3D director stage, prompt library and a chat assistant onto a single infinite canvas.
> - Generate images & videos right inside canvas nodes
> - 3D director stage with camera shots sent back to the board
> - Hundreds of curated prompts, self-host with one `docker compose up`
> - MIT licensed, Go + Next.js
> We'd love your feedback and feature ideas!

### 3. B站 视频标题 + 简介

标题：**开源神器！一张画布搞定 AI 生图/视频/3D，Freedom 创作工作台演示**

简介：
> 本期演示 Freedom —— 一个把 AI 生图、视频、音频和 3D 导演台整合到同一张无限画布的开源工作台。
> 时间轴：
> 00:00 开场
> 00:12 无限画布与生图节点
> 00:22 视频创作台
> 00:32 3D 导演台
> 00:42 提示词库与画布助手
> 00:50 自部署
> GitHub：https://github.com/tigerowo/freedom
> 觉得有用记得三连，评论区告诉我你还想看什么功能。

### 4. 小红书 / 微博 短文案（竖图+头图）

> 做 AI 图/视频还在多个工具间反复横跳？🥲
> 试试 **Freedom**：一张无限画布，把生图、视频、音频、3D 导演台全连起来，MIT 开源还能 Docker 自部署。
> 内置数百提示词 + 画布 AI 助手，结果直接回画布。
> 搜 GitHub「tigerowo/freedom」就能白嫖～
> #AI创作 #开源工具 #StableDiffusion #效率工具

---

## 四、发布检查清单（发版当天）

- [ ] 本地 `git` 提交当前代码
- [ ] 打 tag：`git tag v0.5.3 && git push --tags`
- [ ] 更新 CHANGELOG `Unreleased` → `## v0.5.3`
- [ ] README 版本徽章 `v0.5.2` → `v0.5.3`，banner 图更新到当前版本
- [ ] 替换头图到 `web/public/` 或 README 顶部
- [ ] GitHub Releases 写一段中文发布说明（贴上面定位语 + 亮点 + 链接）
- [ ] 按渠道顺序发：LinuxDO → Product Hunt → B站 → 小红书/微博
- [ ] 各渠道统一带 GitHub 链接 + 邀请码入口（官方版）

---

## 五、下一步你可让我直接做的

1. 改 README（修版本号、换头图、补演示视频位）
2. 生成「概念演示动画」视频（AI 生成，占位用）
3. 落地推广码返佣代码（按 `affiliate-plan.md` 实现）
4. 做轻量落地页（挂 GitHub Pages，统一对外入口）
