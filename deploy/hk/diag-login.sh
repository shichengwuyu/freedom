#!/bin/bash
# 从宿主机对比：前端代理 vs 后端直连

FREEDOM_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' freedom)
echo "freedom 容器 IP = $FREEDOM_IP"
echo

echo "=== 1) 后端直连登录 (宿主机 -> 容器IP:8080) ==="
curl -s -X POST http://$FREEDOM_IP:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"freedom"}' \
  -w "\nHTTP %{http_code}\n"
echo

echo "=== 2) 前端代理登录 (宿主机 -> 80 -> 3000 -> 8080) ==="
curl -s -X POST http://127.0.0.1/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"freedom"}' \
  -w "\nHTTP %{http_code}\n"
echo

echo "=== 3) freedom 容器最近日志 (含登录请求) ==="
docker logs --tail 40 freedom 2>&1 | grep -iE "login|auth|8080|proxy|error|ECONNREFUSED" | tail -20
