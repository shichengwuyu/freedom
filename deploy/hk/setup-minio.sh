#!/bin/bash
set -e
# 在服务器上接入 MinIO：追加 .env 变量、拉起 minio + 初始化 bucket。
cd /root/freedom/deploy/hk

echo '=== 1) 追加 MinIO 环境变量到 .env（若尚未存在）==='
if ! grep -q '^MINIO_ROOT_USER=' .env; then
cat >> .env <<'EOF'

# ===== MinIO 对象存储（供 docker-compose.mysql.yml 使用）=====
MINIO_ROOT_USER=freedomminio
MINIO_ROOT_PASSWORD=Mn7Kd2Xp9Qw4Rt6YbVc3Lf8
MINIO_BUCKET=freedom
EOF
echo '.env 已追加 MinIO 变量'
else
echo '.env 已存在 MinIO 变量，跳过'
fi

echo
echo '=== 2) 拉起 minio 与初始化容器 ==='
docker compose -f docker-compose.mysql.yml up -d minio
echo '--- 等待 minio 健康 ---'
for i in $(seq 1 30); do
  st=$(docker inspect -f '{{.State.Health.Status}}' freedom-minio 2>/dev/null || echo none)
  echo "minio health: $st"
  [ "$st" = "healthy" ] && break
  sleep 2
done

echo
echo '=== 3) 运行 bucket 初始化 ==='
docker compose -f docker-compose.mysql.yml up minio-init
echo '--- minio-init 日志 ---'
docker logs freedom-minio-init 2>&1 | tail -20

echo
echo '=== 4) 让 app 加入依赖（重建 app 网络关系，不改镜像）==='
docker compose -f docker-compose.mysql.yml up -d app

echo
echo '=== 5) 状态汇总 ==='
docker ps --format '{{.Names}} | {{.Status}} | {{.Ports}}'
echo 'DEPLOY_MINIO_DONE'
