#!/bin/bash
# 探测后端容器能否出网访问公网 MinIO 端口。
echo '=== 容器内可用工具 ==='
docker exec freedom sh -c 'for t in curl wget nc busybox getent; do command -v $t >/dev/null 2>&1 && echo "has $t" || echo "no $t"; done'

echo
echo '=== 容器内解析公网 IP（不应被当作内网）==='
docker exec freedom sh -c 'getent hosts 149.88.78.8 || echo "no-getent"'

echo
echo '=== 若有 curl：容器内访问公网 MinIO 健康检查 ==='
docker exec freedom sh -c 'command -v curl >/dev/null 2>&1 && curl -s --max-time 8 http://149.88.78.8:9000/minio/health/live -o /dev/null -w "pub HTTP %{http_code}\n" || echo "no-curl-in-container"'

echo
echo '=== 用后端自身的 /api/proxy-image 走 SafeProxyHTTPClient 探测公网 MinIO 是否被 SSRF 拦 ==='
# proxy-image 用的是同一个带 SSRF 拦截的客户端，可借它验证公网 IP 是否放行
FREEDOM_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' freedom)
curl -s "http://$FREEDOM_IP:3000/api/proxy-image?url=http%3A%2F%2F149.88.78.8%3A9000%2Fminio%2Fhealth%2Flive" -w "\nproxy HTTP %{http_code}\n" | head -c 300
echo
echo DONE
