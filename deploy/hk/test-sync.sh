#!/bin/bash
# 登录获取 token 并触发同步
BASE="http://localhost:3000"

echo "=== 1. 登录 ==="
TOKEN=$(curl -s -X POST "$BASE/api/admin/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"freedom"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
  echo "登录失败"
  exit 1
fi
echo "登录成功，token: ${TOKEN:0:20}..."

echo ""
echo "=== 2. 触发全部分类同步 ==="
SYNC_RESULT=$(curl -s -X POST "$BASE/api/admin/prompt-categories/sync-all" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json")
echo "同步响应: $SYNC_RESULT"

echo ""
echo "=== 3. 检查公开 API ==="
curl -s "$BASE/api/prompts?pageSize=3"

echo ""
echo ""
echo "=== 4. 检查数据库 ==="
docker exec -i freedom-mysql mysql -uroot -poTn0XVy5YacSCPivlIfpW9KQ -e "USE freedom; SELECT category, status, COUNT(0) AS cnt FROM prompts GROUP BY category, status;" 2>/dev/null
