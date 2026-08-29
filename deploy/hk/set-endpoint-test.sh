#!/bin/bash
set -e
# 把 MinIO provider 的 endpoint 改成公网 IP（绕过 SSRF 内网拦截），publicBaseUrl 暂用同地址，实测上传。
BASE="http://127.0.0.1"
ADMIN_USER="admin"; ADMIN_PASS="freedom"
ENDPOINT="http://149.88.78.8:9000"
PUBLIC_BASE="$1"   # 由参数传入 publicBaseUrl

TOKEN=$(curl -s -X POST "$BASE/api/admin/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")

curl -s "$BASE/api/admin/settings" -H "Authorization: Bearer $TOKEN" -o /tmp/settings_full.json

python3 - "$ENDPOINT" "$PUBLIC_BASE" <<'PY'
import json, sys
endpoint, public_base = sys.argv[1], sys.argv[2]
env = json.load(open('/tmp/settings_full.json'))
data = env.get('data', env)
storage = data.setdefault('private', {}).setdefault('storage', {})
provs = storage.get('providers') or []
found = False
for p in provs:
    if p.get('id') == 'minio-local':
        p['endpoint'] = endpoint
        p['publicBaseUrl'] = public_base
        p['secretAccessKey'] = 'Mn7Kd2Xp9Qw4Rt6YbVc3Lf8'  # 新值需带全，避免被当空保留
        found = True
if not found:
    print('ERROR: minio-local provider not found'); sys.exit(1)
storage['providers'] = provs
json.dump(data, open('/tmp/settings_new.json','w'), ensure_ascii=False)
print('endpoint ->', endpoint, '| publicBaseUrl ->', public_base)
PY

curl -s -X POST "$BASE/api/admin/settings" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d @/tmp/settings_new.json -o /dev/null -w "保存 HTTP %{http_code}\n"

echo '--- storage/config ---'
curl -s "$BASE/api/storage/config"; echo

echo '=== 实测上传 ==='
FREEDOM_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' freedom)
UTOKEN=$(curl -s -X POST "http://$FREEDOM_IP:3000/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n\x2d\xb4\x00\x00\x00\x00IEND\xaeB\x60\x82' > /tmp/px.png
RESP=$(curl -s -X POST "http://$FREEDOM_IP:3000/api/v1/files" -H "Authorization: Bearer $UTOKEN" -F "file=@/tmp/px.png;type=image/png")
echo "上传返回: $RESP"

echo '=== 后端日志 ==='
docker logs --since 30s freedom 2>&1 | grep -iE "error|fail|禁止|too long|storage|files|Put " | tail -10 || echo no-errors
echo TEST_DONE
