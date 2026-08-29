#!/usr/bin/env bash
# 调整 mirror 顺序（DaoCloud 优先），并逐个预拉构建所需镜像，避免大层卡死。
set +e

cat > /etc/docker/daemon.json <<'EOF'
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://docker.1ms.run",
    "https://docker.xuanyuan.me",
    "https://docker.1panel.live",
    "https://dockerproxy.net"
  ],
  "max-concurrent-downloads": 3
}
EOF

systemctl restart docker
sleep 4

for img in "mysql:8.0" "golang:1.25-alpine" "node:22-bookworm-slim" "oven/bun:1.3.14"; do
  echo "==================== PULL $img ===================="
  # 每个镜像最多重试 3 次，利用断点续传。
  for attempt in 1 2 3; do
    if timeout 300 docker pull "$img"; then
      echo "PULL_OK $img"
      break
    else
      echo "PULL_RETRY $img attempt=$attempt"
      sleep 3
    fi
  done
done

echo "=== 已拉取镜像 ==="
docker images
echo "PREPULL_DONE"
