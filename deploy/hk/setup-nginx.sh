#!/bin/bash
# ============================================================
# 香港服务器 Nginx + 域名绑定一键配置脚本
# 域名：xiaoyxiao.xyz
# 服务器：149.88.78.8
# 用法：chmod +x setup-nginx.sh && ./setup-nginx.sh
# ============================================================

set -e

DOMAIN="xiaoyxiao.xyz"
DEPLOY_DIR="/root/freedom/deploy/hk"
NGINX_CONF="/etc/nginx/conf.d/${DOMAIN}.conf"

echo "=========================================="
echo "  xiaoyxiao.xyz Nginx 配置脚本"
echo "  时间: $(date)"
echo "=========================================="

# ---------- 1. 检查 root 权限 ----------
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 root 用户运行此脚本 (sudo ./setup-nginx.sh)"
    exit 1
fi

# ---------- 2. 安装 Nginx ----------
echo ""
echo "[1/4] 安装 Nginx..."
if command -v apt-get &>/dev/null; then
    apt-get update
    apt-get install -y nginx
elif command -v yum &>/dev/null; then
    yum install -y epel-release
    yum install -y nginx
else
    echo " 未识别的包管理器，请手动安装 Nginx"
    exit 1
fi
echo "✅ Nginx $(nginx -v 2>&1) 已安装"

# ---------- 3. 写入 Nginx 配置 ----------
echo ""
echo "[2/4] 写入 Nginx 配置..."
cat > "$NGINX_CONF" << 'NGINXEOF'
# xiaoyxiao.xyz Nginx 反向代理配置

server {
    listen 80;
    server_name xiaoyxiao.xyz www.xiaoyxiao.xyz;

    # API 请求转发到 Go 后端
    location /api/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 50m;
    }

    # WebSocket 支持
    location /ws/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 前端静态资源缓存
    location /_next/static/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_cache_valid 200 30d;
        add_header Cache-Control "public, immutable";
    }

    # 其他请求转发到 Next.js 前端
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
        client_max_body_size 50m;
    }
}
NGINXEOF
echo "✅ Nginx 配置已写入 $NGINX_CONF"

# ---------- 4. 更新 docker-compose 端口映射 ----------
echo ""
echo "[3/4] 更新 Docker 端口映射（80 → 3000）..."
if [ -f "$DEPLOY_DIR/docker-compose.mysql.yml" ]; then
    # 如果端口还是 80:3000，改为 3000:3000
    if grep -q '"80:3000"' "$DEPLOY_DIR/docker-compose.mysql.yml"; then
        sed -i 's/"80:3000"/"3000:3000"/' "$DEPLOY_DIR/docker-compose.mysql.yml"
        echo "✅ 端口已从 80:3000 改为 3000:3000"
    else
        echo "✅ 端口已经是 3000:3000，无需修改"
    fi
else
    echo "⚠️  docker-compose.mysql.yml 未找到，跳过端口修改"
fi

# 重启 app 容器使端口生效
if [ -f "$DEPLOY_DIR/docker-compose.mysql.yml" ]; then
    echo "--- 重启 app 容器 ---"
    cd "$DEPLOY_DIR"
    docker compose -f docker-compose.mysql.yml up -d app
    echo "✅ app 容器已重启"
fi

# ---------- 5. 测试并启动 Nginx ----------
echo ""
echo "[4/4] 测试并启动 Nginx..."
nginx -t
systemctl enable nginx
systemctl restart nginx
echo "✅ Nginx 已启动"

# ---------- 完成 ----------
echo ""
echo "=========================================="
echo "  ✅ 配置完成！"
echo "=========================================="
echo ""
echo "  访问地址: http://${DOMAIN}"
echo "  访问地址: http://www.${DOMAIN}"
echo ""
echo "  常用命令:"
echo "  查看 Nginx 状态:  systemctl status nginx"
echo "  查看容器状态:     docker ps"
echo "  重载 Nginx 配置:  nginx -s reload"
echo "  查看 Nginx 日志:  tail -f /var/log/nginx/error.log"
echo ""
echo "⚠️  提醒:"
echo "  1. 确保阿里云 DNS 解析已指向 149.88.78.8"
echo "  2. 确保服务器安全组已开放 80 端口"
echo "  3. 后续可配置 HTTPS（certbot 免费证书）"
echo ""
