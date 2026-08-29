#!/usr/bin/env bash
# 测试常见 Docker registry mirror 在本机的可达性。
set +e
mirrors=(
  "https://docker.1ms.run"
  "https://docker.xuanyuan.me"
  "https://dockerproxy.net"
  "https://docker.m.daocloud.io"
  "https://hub.rat.dev"
  "https://docker.1panel.live"
  "https://registry-1.docker.io"
)
for m in "${mirrors[@]}"; do
  code=$(timeout 10 curl -fsS -o /dev/null -w "%{http_code}" "$m/v2/" 2>/dev/null)
  rc=$?
  echo "$m -> http=$code rc=$rc"
done
echo "MIRROR_TEST_DONE"
