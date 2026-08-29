#!/usr/bin/env bash
# 排查当前占用 apt/dpkg 锁的进程。
echo "=== 占用锁的进程 ==="
for f in /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock /var/lib/apt/lists/lock; do
  pid=$(fuser "$f" 2>/dev/null)
  if [ -n "$pid" ]; then
    echo "锁文件 $f 被占用，pid:$pid"
    ps -o pid,etime,cmd -p $pid 2>/dev/null
  fi
done
echo "=== unattended-upgrades 状态 ==="
systemctl is-active unattended-upgrades 2>/dev/null || true
echo "=== 相关进程 ==="
ps -eo pid,etime,cmd | grep -Ei 'apt|dpkg|unattended|update' | grep -v grep || echo none
echo "LOCKCHECK_DONE"
