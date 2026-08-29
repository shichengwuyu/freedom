#!/bin/bash
# 检查并修正 settings.value 列类型为 longtext
PW='InGQR7jWqSDbmftPkx9KX8zO'
echo "=== 当前 settings 列类型 ==="
docker exec freedom-mysql mysql -ufreedom -p"$PW" freedom -e "SHOW COLUMNS FROM settings;" 2>/dev/null

echo
echo "=== 强制把 value 改成 LONGTEXT（兜底，AutoMigrate 可能不改列） ==="
docker exec freedom-mysql mysql -ufreedom -p"$PW" freedom -e "ALTER TABLE settings MODIFY value LONGTEXT;" 2>/dev/null && echo "ALTER_OK"

echo
echo "=== 修正后列类型 ==="
docker exec freedom-mysql mysql -ufreedom -p"$PW" freedom -e "SHOW COLUMNS FROM settings;" 2>/dev/null
