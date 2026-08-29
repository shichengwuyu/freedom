#!/bin/bash
# 重新构建 app 镜像并重启容器（只重建 app，不动 mysql/minio）。
cd /root/freedom/deploy/hk || { echo "NO_COMPOSE_DIR"; exit 1; }

# 让 host bind mount 出来的 data/ 持久化目录归属与容器内 appuser (uid=999) 对齐，
# 避免 appuser runtime 写日志/上传时 permission denied。Dockerfile 切到 USER appuser:999 后，
# 容器内进程无权写 root:root 0755 子目录，所以必须在 host 侧把它修正成 999:999。
# 已存在的旧文件 chown 一次到位之后，下次 up -d 不再变；新创建的文件自然继承。2>/dev/null 兜底空挂载。
chown -R 999:999 ./data 2>/dev/null || true

# 构建前清理旧的 Build Cache，防止磁盘爆满（每次 build 累积 ~10GB）
echo "=== 构建前清理 Docker Build Cache $(date) ==="
BEFORE=$(df -h / | awk 'NR==2{print $5}')
docker builder prune -f 2>/dev/null
AFTER=$(df -h / | awk 'NR==2{print $5}')
echo "磁盘使用: $BEFORE -> $AFTER"

echo "=== 开始构建 app 镜像 $(date) ==="
docker compose -f docker-compose.mysql.yml build app
echo "=== 构建完成，重启 app $(date) ==="
docker compose -f docker-compose.mysql.yml up -d app

# 构建后再次清理 dangling 镜像和缓存
echo "=== 构建后清理 $(date) ==="
docker image prune -f 2>/dev/null
docker builder prune -f 2>/dev/null

echo "=== 容器状态 ==="
docker ps --format '{{.Names}} | {{.Status}}'
echo "=== 磁盘状态 ==="
df -h /
echo "BUILD_DEPLOY_DONE $(date)"
