#!/bin/bash
# 验证：登录拿 admin token -> 调用保存设置接口 -> 检查是否还报 Data too long
FREEDOM_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' freedom)

echo "=== 1) 登录获取 token ==="
TOKEN=$(curl -s -X POST http://$FREEDOM_IP:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"freedom"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
echo "token 前 20 位: ${TOKEN:0:20}..."

echo
echo "=== 2) 读取当前设置 ==="
curl -s http://$FREEDOM_IP:8080/api/admin/settings -H "Authorization: Bearer $TOKEN" -o /tmp/cur_settings.json -w "GET HTTP %{http_code}\n"
echo "返回体大小: $(wc -c < /tmp/cur_settings.json) 字节"

echo
echo "=== 3) 原样回存设置（触发 INSERT settings）==="
# 取出 data 部分作为保存 body
BODY=$(sed -n 's/^{"code":0,"data":\(.*\),"msg":"ok"}$/\1/p' /tmp/cur_settings.json)
if [ -z "$BODY" ]; then BODY=$(cat /tmp/cur_settings.json); fi
echo "$BODY" | curl -s -X POST http://$FREEDOM_IP:8080/api/admin/settings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @- -w "\nPOST HTTP %{http_code}\n"

echo
echo "=== 4) 最近后端日志是否还有 Data too long ==="
docker logs --since 2m freedom 2>&1 | grep -iE "Data too long|1406|Error" | tail -10 || echo "（无 Data too long / Error）"
