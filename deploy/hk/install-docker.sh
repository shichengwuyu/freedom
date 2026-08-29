#!/usr/bin/env bash
# 在香港服务器上安装 Docker，先等待可能占用 apt 锁的进程结束。
set -e

echo "=== 等待 apt/dpkg 锁释放 ==="
# 最多等待 900 秒，避免首次开机的 unattended-upgrades 占锁导致安装失败。
for i in $(seq 1 180); do
  if fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 \
     || fuser /var/lib/dpkg/lock >/dev/null 2>&1 \
     || fuser /var/lib/apt/lists/lock >/dev/null 2>&1; then
    echo "apt 锁被占用，等待中... ($i)"
    sleep 5
  else
    break
  fi
done

echo "=== 检查是否已安装 Docker ==="
if command -v docker >/dev/null 2>&1; then
  echo "Docker 已安装：$(docker --version)"
else
  echo "=== 安装 Docker（官方脚本）==="
  curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
  sh /tmp/get-docker.sh
fi

echo "=== 启用并启动 Docker ==="
systemctl enable --now docker

echo "=== 版本信息 ==="
docker --version
docker compose version

echo "DOCKER_INSTALLED_OK"
