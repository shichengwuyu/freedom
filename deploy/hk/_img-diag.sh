#!/usr/bin/env bash
set +e
FIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' freedom)
MYSQL_ROOT_PW=$(grep -E '^MYSQL_ROOT_PASSWORD=' /root/freedom/deploy/hk/.env | cut -d= -f2 | tr -d '\r')

echo '========== 1. storage_objects by provider_id count =========='
docker exec freedom-mysql mysql -uroot -p"$MYSQL_ROOT_PW" freedom -e "SELECT provider_id, COUNT(*) AS c, MAX(created_at) AS last_time, SUM(bytes) AS total_bytes FROM storage_objects GROUP BY provider_id ORDER BY c DESC;" 2>/dev/null
echo

echo '========== 2. last 3 rows of each provider =========='
docker exec freedom-mysql mysql -uroot -p"$MYSQL_ROOT_PW" freedom -e "SELECT id, provider_id, bucket, LEFT(object_key, 90) AS object_key, LEFT(public_url, 100) AS public_url, created_at FROM storage_objects ORDER BY provider_id, created_at DESC;" 2>/dev/null | head -50
echo

echo '========== 3. 测试 /api/files/{id} FileInfo 两个旧/new provider_id =========='
# 挑一个旧 id（如果存在）和一个新 id
OLD_ID=$(docker exec freedom-mysql mysql -uroot -p"$MYSQL_ROOT_PW" freedom -N -B -e "SELECT id FROM storage_objects WHERE provider_id!='minio-local' ORDER BY created_at DESC LIMIT 1;" 2>/dev/null)
NEW_ID=$(docker exec freedom-mysql mysql -uroot -p"$MYSQL_ROOT_PW" freedom -N -B -e "SELECT id FROM storage_objects WHERE provider_id='minio-local' ORDER BY created_at DESC LIMIT 1;" 2>/dev/null)
echo "OLD_ID=$OLD_ID"
echo "NEW_ID=$NEW_ID"
echo
if [ -n "$OLD_ID" ]; then
  echo "--- FileInfo OLD_ID=$OLD_ID ---"
  curl -s "http://$FIP:3000/api/files/$OLD_ID"; echo; echo
  echo "--- Content OLD_ID=$OLD_ID (head+headers) ---"
  curl -sI "http://$FIP:3000/api/files/$OLD_ID/content" | head -12
  echo
fi
echo "--- FileInfo NEW_ID=$NEW_ID ---"
curl -s "http://$FIP:3000/api/files/$NEW_ID"; echo; echo
echo "--- Content NEW_ID=$NEW_ID (headers) ---"
curl -sI "http://$FIP:3000/api/files/$NEW_ID/content" | head -12
echo

echo '========== 4. 公共 URL + proxy-image 测试 =========='
PUBLIC_URL=$(docker exec freedom-mysql mysql -uroot -p"$MYSQL_ROOT_PW" freedom -N -B -e "SELECT public_url FROM storage_objects WHERE id=\"$NEW_ID\";" 2>/dev/null)
echo "PUBLIC_URL=$PUBLIC_URL"
echo -n "direct curl: "
curl -sI "$PUBLIC_URL" -o /dev/null -w 'HTTP %{http_code} CT=%{content_type} BYTES=%{size_download}\n'
echo
echo -n "/api/proxy-image: "
PROXY_URL="http://$FIP:3000/api/proxy-image?url=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$PUBLIC_URL")"
curl -sI "$PROXY_URL" -o /dev/null -w 'HTTP %{http_code} CT=%{content_type} BYTES=%{size_download}\n'
echo

echo '========== 5. 查看 settings.private storage.providers 有哪些 id =========='
docker exec freedom-mysql mysql -uroot -p"$MYSQL_ROOT_PW" freedom -N -B -e "SELECT value FROM settings WHERE \`key\`='private';" 2>/dev/null | python3 -c "
import json,sys
s = json.loads(sys.stdin.read().strip() or '{}')
storage = (s.get('private') or s).get('storage') if 'private' in s else (s.get('storage') or {})
# 兼容：如果读的是 value 而不是 private key 里的（即 settings.private 直接是 priv JSON）
if 'providers' not in storage:
    storage = s.get('storage') or {}
providers = storage.get('providers') or []
print('providers ids=', [p.get('id') for p in providers])
"

echo
echo 'PROBE_DONE'
