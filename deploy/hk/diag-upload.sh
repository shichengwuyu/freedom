#!/bin/bash
# 线上文件上传排查脚本：检查数据目录、files 表、存储配置，并直连后端做一次真实上传。
PW='InGQR7jWqSDbmftPkx9KX8zO'

echo '=== 1) 容器内数据目录 ==='
docker exec freedom sh -c 'ls -la /app/data 2>&1; echo ---uploads---; ls -la /app/data/uploads 2>&1 | head; echo ---df---; df -h /app/data 2>&1'

echo
echo '=== 2) files 表结构与行数 ==='
docker exec freedom-mysql mysql -ufreedom -p"$PW" freedom -e 'SHOW COLUMNS FROM files;' 2>/dev/null
docker exec freedom-mysql mysql -ufreedom -p"$PW" freedom -e 'SELECT COUNT(*) AS n FROM files;' 2>/dev/null

echo
echo '=== 3) 应用环境变量（存储相关）==='
docker exec freedom sh -c 'env | grep -iE "STORAGE|PUBLIC_BASE|API_BASE|DATABASE" || echo (none)'

echo
echo '=== 4) 直连后端做真实上传测试 ==='
FREEDOM_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' freedom)
echo "freedom 容器 IP: $FREEDOM_IP"

TOKEN=$(curl -s -X POST http://$FREEDOM_IP:3000/api/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"freedom"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
echo "token 前 20 位: ${TOKEN:0:20}..."

# 造一个 1x1 png
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n\x2d\xb4\x00\x00\x00\x00IEND\xaeB\x60\x82' > /tmp/px.png

echo '--- 上传到 /api/v1/files ---'
curl -s -X POST "http://$FREEDOM_IP:3000/api/v1/files" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/px.png;type=image/png" \
  -w "\nHTTP %{http_code}\n" | head -c 2000

echo
echo '=== 5) 最近后端错误日志 ==='
docker logs --since 2m freedom 2>&1 | grep -iE "error|fail|panic|too long|denied|no space|permission|1406|1054|files" | tail -30 || echo (no-errors)
