#!/bin/sh
set -eu

# 从 docker-compose 挂载的 /app/.env（即 deploy/hk/.env）加载所有环境变量并导出
# 这样 Next.js standalone (server.js) 才能读到 API_BASE_URL 等配置，
# 否则会落到 route.ts 默认的 http://127.0.0.1:18080，导致 /api/* 全部代理失败。
if [ -f /app/.env ]; then
  set -a
  # shellcheck disable=SC1091
  . /app/.env
  set +a
fi

# 兜底：Next.js 前端代理转发必须指向容器内后端 8080（API_BASE_URL 可能配成宿主机 127.0.0.1 也 OK，容器内共用 net ns）
export API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080}"
# 浏览器直链时使用的公开访问前缀（也注入给前端 Node 进程以防后续使用）
export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"

PORT=8080 /app/server &
API_PID=$!

cd /app/web
PORT=3000 node server.js &
WEB_PID=$!

shutdown() {
  trap - INT TERM
  kill -TERM "$API_PID" "$WEB_PID" 2>/dev/null || true
  wait "$API_PID" 2>/dev/null || true
  wait "$WEB_PID" 2>/dev/null || true
  exit 0
}

trap shutdown INT TERM

while kill -0 "$API_PID" 2>/dev/null && kill -0 "$WEB_PID" 2>/dev/null; do
  sleep 1
done

kill -TERM "$API_PID" "$WEB_PID" 2>/dev/null || true
wait "$API_PID" 2>/dev/null || true
wait "$WEB_PID" 2>/dev/null || true
exit 1
