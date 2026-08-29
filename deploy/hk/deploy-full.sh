#!/bin/bash
# 完整部署脚本 - 打包整个项目并上传到香港服务器
set -e

PROJECT_ROOT="f:/trae/wifi/infinite-canvas-main"
SERVER_IP="149.88.78.8"
SERVER_USER="root"
SERVER_PATH="/root/freedom"
TAR_FILE="freedom-src-full.tar.gz"

echo "=== 开始打包项目 ==="
cd "$PROJECT_ROOT"

# 打包整个项目(排除构建产物、数据文件、临时文件)
tar --exclude='node_modules' \
    --exclude='.next' \
    --exclude='data' \
    --exclude='*.exe' \
    --exclude='*.log' \
    --exclude='deploy/build' \
    --exclude='.npm-cache' \
    --exclude='tmp' \
    --exclude='.trae' \
    --exclude='.agents' \
    --exclude='.claude' \
    --exclude='.github' \
    --exclude='freedom-src*.tar.gz' \
    -czf "$TAR_FILE" .

SIZE=$(du -h "$TAR_FILE" | cut -f1)
echo "打包完成: $TAR_FILE ($SIZE)"

echo ""
echo "=== 上传到服务器 ==="
echo "目标: ${SERVER_USER}@${SERVER_IP}:${SERVER_PATH}"

scp "$TAR_FILE" "${SERVER_USER}@${SERVER_IP}:${SERVER_PATH}/"

if [ $? -ne 0 ]; then
    echo "上传失败!"
    exit 1
fi

echo "上传完成"

echo ""
echo "=== 在服务器上解压并重建 ==="
ssh "${SERVER_USER}@${SERVER_IP}" << 'ENDSSH'
cd /root/freedom
echo "解压源码..."
tar -xzf freedom-src-full.tar.gz
echo "重建 Docker 镜像..."
cd deploy/hk
bash build-deploy.sh
echo "部署完成!"
ENDSSH

echo ""
echo "=== 部署完成 ==="
echo "访问地址: http://xiaoyxiao.xyz"
echo "查看日志: ssh root@149.88.78.8 'docker logs -f freedom'"
