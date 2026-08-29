#!/usr/bin/env bash
# 直接用 apt 安装 Docker（Docker 官方源已由上一步配置好）。
set -x

# 确保锁已释放
for i in $(seq 1 60); do
  if fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1; then
    echo "wait apt lock $i"; sleep 5
  else
    break
  fi
done

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin docker-buildx-plugin
RC=$?
echo "APT_INSTALL_RC=$RC"

systemctl enable --now docker
docker --version || echo NO_DOCKER
docker compose version || echo NO_COMPOSE
echo "DONE_INSTALL"
