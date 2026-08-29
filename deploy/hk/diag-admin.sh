#!/usr/bin/env bash
# 诊断 admin 密码 hash 是否完整写入 MySQL。
set +e
PWD_HASH=$(docker exec freedom-mysql mysql -uroot -poTn0XVy5YacSCPivlIfpW9KQ -N -B -e "USE freedom; SELECT password FROM users WHERE username='admin';" 2>/dev/null)
echo "HASH_START>>>${PWD_HASH}<<<HASH_END"
echo "HASH_LEN=${#PWD_HASH}"
echo "HASH_PREFIX=${PWD_HASH:0:4}"

# 用 python3 校验（若装了 bcrypt）
python3 - "$PWD_HASH" <<'PY' 2>/dev/null || echo "PY_NO_BCRYPT"
import sys
try:
    import bcrypt
    h = sys.argv[1].encode()
    print("BCRYPT_MATCH_FREEDOM =", bcrypt.checkpw(b"freedom", h))
except Exception as e:
    print("PY_ERR", e)
    sys.exit(3)
PY
echo "DIAG_DONE"
