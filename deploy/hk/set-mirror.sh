#!/usr/bin/env bash
# 配置 Docker registry 镜像加速，解决香港服务器直连 Docker Hub 超时问题。
set -e

mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'EOF'
{
  "registry-mirrors": [
    "https://docker.1ms.run",
    "https://docker.xuanyuan.me",
    "https://docker.m.daocloud.io",
    "https://docker.1panel.live",
    "https://dockerproxy.net"
  ]
}
EOF

systemctl daemon-reload
systemctl restart docker
sleep 3
echo "=== docker info mirrors ==="
docker info 2>/dev/null | grep -A6 "Registry Mirrors" || true
echo "=== 试拉 hello-world ==="
docker pull hello-world && echo PULL_OK || echo PULL_FAIL
echo "MIRROR_CONFIG_DONE"
