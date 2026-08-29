---
title: Docker 部署
description: 使用 Docker Compose 部署Freedom
---

# Docker 部署

如果你希望在自己的机器或服务器上运行项目，可以直接使用 Docker Compose。

## 使用发布镜像

```bash
git clone git@github.com:tigerowo/freedom.git
cd freedom
cp .env.example .env
docker compose up -d
```

启动后访问：

```text
http://localhost:3000
```

默认管理员账号：

```text
用户名：admin
密码：.env 中的 ADMIN_PASSWORD
```

## 本地构建镜像

如果需要基于当前源码构建镜像：

```bash
cp .env.example .env
docker compose -f docker-compose.local.yml up -d --build
```

## 数据目录

`docker-compose.yml` 会把本地 `./data` 挂载到容器内 `/app/data`，用于保存提示词数据和上传素材。

MySQL 数据库需外部提供，`DATABASE_DSN` 填写 MySQL 连接串即可。

如果需要让火山方舟拉取本地上传的 Seedance 参考素材，还需要把 `PUBLIC_BASE_URL` 设置为公网可访问的站点地址。

## 生产部署域名与 CORS 检查

部署到公网（如 `https://xiaoyxiao.xyz`）后，务必确认以下两项，否则前端接口会被浏览器跨域拦截、邀请链接也可能异常：

1. **CORS 白名单**：在服务器 `.env` 设置以下任一变量，把前端域名加进允许来源：
   ```text
   PUBLIC_BASE_URL=https://xiaoyxiao.xyz
   # 或
   CORS_ALLOWED_ORIGINS=https://xiaoyxiao.xyz
   ```
   未设置时后端默认仅放行 `localhost` 开发环境，生产域名会被拦。
2. **邀请链接域名**：钱包页「我的邀请」用 `window.location.origin` 动态生成邀请链接（`/register?inviterCode=XXX`），部署后自动为真实域名，无需硬编码。但前提是用户通过域名（经 nginx 反代）访问，且第 1 项的 CORS 已放开。

> 后台「邀请返佣」开关（私有配置）默认关闭，上线后如需开启推广分润，到 `管理员 → 设置 → 私有配置 → 邀请返佣` 打开并设置比例。
