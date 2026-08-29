#!/bin/bash
# 部署前探测：确认服务器源码树完整、构建环境可用。
cd /root/freedom || { echo "NO_PROJECT_DIR"; exit 1; }
echo "=== git? ==="
if [ -d .git ]; then echo HAS_GIT; else echo NO_GIT; fi
echo "=== 关键源文件是否存在 ==="
files=(
  "handler/ai.go"
  "web/src/app/(user)/canvas/components/canvas-node.tsx"
  "web/src/app/(user)/canvas/[id]/canvas-client-page.tsx"
  "web/src/app/(user)/novel/page.tsx"
  "web/src/app/(user)/canvas/components/canvas-assistant-panel.tsx"
)
for f in "${files[@]}"; do
  if [ -f "$f" ]; then echo "OK  $f"; else echo "MISSING  $f"; fi
done
echo "=== 磁盘 ==="
df -h / | tail -1
echo "=== docker compose ==="
docker compose version 2>/dev/null | head -1
echo "=== app 镜像 ==="
docker images freedom:local --format '{{.Repository}}:{{.Tag}} {{.Size}} {{.CreatedSince}}'
echo "PROBE_DONE"
