#!/bin/bash
# 抓取线上后端最近日志，聚焦错误
echo "=== 容器状态 ==="
docker ps --format '{{.Names}} | {{.Status}}'
echo
echo "=== freedom 最近 5 分钟内的 错误/异常 行 ==="
docker logs --since 30m freedom 2>&1 | grep -iE "error|fail|panic|1406|1170|1064|record not found|refused|timeout|denied|invalid|exception|too long" | tail -40
echo
echo "=== freedom 最近 50 行原始日志 ==="
docker logs --tail 50 freedom 2>&1
