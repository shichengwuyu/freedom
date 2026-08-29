#!/bin/bash
# 定位当前运行中的 compose 项目目录与配置文件，为接入 MinIO 做准备。
echo '=== freedom 容器的 compose labels ==='
docker inspect freedom --format '{{json .Config.Labels}}' | tr ',' '\n' | grep -i compose

echo
echo '=== 可能的部署目录 ==='
for d in /root/freedom /root/freedom/hk /root/hk; do
  echo "-- $d --"
  ls -la "$d" 2>/dev/null
done

echo
echo '=== .env 文件位置 ==='
find /root -maxdepth 4 -name '.env' 2>/dev/null

echo
echo '=== 当前 hk_default 网络里的容器 ==='
docker network inspect hk_default --format '{{range .Containers}}{{.Name}} {{end}}'
