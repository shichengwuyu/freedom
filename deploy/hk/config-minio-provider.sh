#!/bin/bash
set -e
# 通过后台 API 把 MinIO 配置为 S3 存储 provider 并启用，然后实测上传。
BASE="http://127.0.0.1"
ADMIN_USER="admin"
ADMIN_PASS="freedom"

# MinIO 连接参数（与 .env / compose 一致）
MINIO_ENDPOINT_INTERNAL="http://minio:9000"        # 后端容器内访问
MINIO_PUBLIC_BASE="http://149.88.78.8:9000/freedom" # 浏览器直链
MINIO_BUCKET="freedom"
MINIO_AK="freedomminio"
MINIO_SK="Mn7Kd2Xp9Qw4Rt6YbVc3Lf8"

echo '=== 1) 管理员登录拿 token ==='
TOKEN=$(curl -s -X POST "$BASE/api/admin/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
echo "token 前 20: ${TOKEN:0:20}..."

echo
echo '=== 2) 读取当前完整设置 ==='
curl -s "$BASE/api/admin/settings" -H "Authorization: Bearer $TOKEN" -o /tmp/settings_full.json
echo "返回体大小: $(wc -c < /tmp/settings_full.json) 字节"

echo
echo '=== 3) 注入 MinIO provider（Python 处理 JSON）==='
python3 - "$MINIO_ENDPOINT_INTERNAL" "$MINIO_PUBLIC_BASE" "$MINIO_BUCKET" "$MINIO_AK" "$MINIO_SK" <<'PY'
import json, sys
endpoint, public_base, bucket, ak, sk = sys.argv[1:6]
with open('/tmp/settings_full.json') as f:
    env = json.load(f)
data = env.get('data', env)
priv = data.setdefault('private', {})
storage = priv.setdefault('storage', {})
providers = storage.get('providers') or []
minio = {
    "id": "minio-local",
    "name": "MinIO 本地对象存储",
    "type": "s3",
    "endpoint": endpoint,
    "region": "us-east-1",
    "bucket": bucket,
    "accessKeyId": ak,
    "secretAccessKey": sk,
    "publicBaseUrl": public_base,
    "pathPrefix": "uploads",
    "weight": 1,
    "enabled": True,
}
# 覆盖同 id 的旧配置，否则追加
providers = [p for p in providers if p.get('id') != 'minio-local']
providers.append(minio)
storage['providers'] = providers
storage['mode'] = 'server_sqlite_s3'  # 仅装饰，后端会自行重算
with open('/tmp/settings_new.json', 'w') as f:
    json.dump(data, f, ensure_ascii=False)
print('注入完成，provider 数量:', len(providers))
PY

echo
echo '=== 4) 回存设置 ==='
curl -s -X POST "$BASE/api/admin/settings" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d @/tmp/settings_new.json \
  -w "\nPOST HTTP %{http_code}\n" -o /tmp/settings_saved.json
echo "保存返回体大小: $(wc -c < /tmp/settings_saved.json) 字节"

echo
echo '=== 5) 校验 mode ==='
curl -s "$BASE/api/storage/config"
echo

echo
echo '=== 6) 实测上传（后端直连）==='
FREEDOM_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' freedom)
UTOKEN=$(curl -s -X POST "http://$FREEDOM_IP:3000/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n\x2d\xb4\x00\x00\x00\x00IEND\xaeB\x60\x82' > /tmp/px.png
echo '--- 上传结果 ---'
curl -s -X POST "http://$FREEDOM_IP:3000/api/v1/files" \
  -H "Authorization: Bearer $UTOKEN" \
  -F "file=@/tmp/px.png;type=image/png" -w "\nHTTP %{http_code}\n"

echo
echo '=== 7) 最近后端日志 ==='
docker logs --since 1m freedom 2>&1 | grep -iE "error|fail|too long|denied|files|storage" | tail -15 || echo "no-error-lines"
echo 'CONFIG_MINIO_DONE'
