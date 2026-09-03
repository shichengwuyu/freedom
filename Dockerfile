# 构建 Next.js 前端产物。
FROM oven/bun:1.3.14 AS web-build

WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile --cache-dir=/root/.bun/install/cache
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY web ./
RUN bun run build

# 构建 Go 后端入口。
FROM golang:1.25-alpine AS api-build

WORKDIR /app
# 使用国内 Go 代理，避免连接 proxy.golang.org 超时
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
COPY config ./config
COPY handler ./handler
COPY middleware ./middleware
COPY model ./model
COPY repository ./repository
COPY router ./router
COPY service ./service
COPY main.go ./
RUN go build -o /server .

# 运行镜像：Next.js 对外监听 3000，Go 只在容器内部监听 8080。
FROM node:22-bookworm-slim

WORKDIR /app
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY --from=api-build /server /app/server
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
COPY --from=web-build /app/web/public /app/web/public
COPY --from=web-build /app/web/.next/standalone /app/web
COPY --from=web-build /app/web/.next/static /app/web/.next/static
# novel-workflow v2：系统预设 BGM 库（8 首选曲，mp3 缺失不阻塞启动）
COPY assets/bgm-presets /app/assets/bgm-presets
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
ENV PROMPT_DATA_DIR=/app/data/prompts
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget ffmpeg && rm -rf /var/lib/apt/lists/* && ffmpeg -version | head -1
# 运行时数据目录的预先建好 — mount 进来时由 deploy/hk/build-deploy.sh 把 host 端目录 chown 到容器内
# appuser (uid=999) 以避免容器内进程写日志/上传时 permission denied（容器内 app 进程以非 root 跑）。
RUN mkdir -p /app/data/prompts /app/data/logs/ai-calls /app/data/uploads /app/data/reference-media

# 创建非 root 用户运行应用，减少容器逃逸风险
RUN groupadd -r appuser && useradd -r -g appuser -d /app -s /sbin/nologin appuser && chown -R appuser:appuser /app
USER appuser

EXPOSE 3000
# 健康检查：每 30 秒检查一次后端 /api/health
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:8080/api/health || exit 1
# 先启动内部 Go API，再由 Next.js 提供页面并代理 /api/*。
CMD ["/app/docker-entrypoint.sh"]
