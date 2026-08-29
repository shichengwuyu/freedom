#!/bin/bash
# ============================================================
# xiaoyxiao.xyz HTTPS 一键部署脚本
# 服务器：149.88.78.8 (root)
# 方案：Certbot + Let's Encrypt + Nginx
# 用法：chmod +x setup-https.sh && ./setup-https.sh
# ============================================================

set -e

# ---------- 配置 ----------
DOMAIN="xiaoyxiao.xyz"
EMAIL="3035947885@qq.com"
DEPLOY_DIR="/root/freedom/deploy/hk"
NGINX_CONF="/etc/nginx/conf.d/${DOMAIN}.conf"
CERT_DIR="/etc/letsencrypt/live/${DOMAIN}"
WEBROOT_DIR="/var/www/certbot"

echo "=========================================="
echo "  xiaoyxiao.xyz HTTPS 部署脚本"
echo "  时间: $(date)"
echo "  邮箱: ${EMAIL}"
echo "=========================================="

# ---------- 1. 检查 root 权限 ----------
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 root 用户运行此脚本 (sudo ./setup-https.sh)"
    exit 1
fi

# ---------- 2. 安装 certbot ----------
echo ""
echo "[1/6] 安装 Certbot..."
if command -v apt-get &>/dev/null; then
    apt-get update -y
    apt-get install -y certbot python3-certbot-nginx
elif command -v yum &>/dev/null; then
    yum install -y epel-release
    yum install -y certbot python3-certbot-nginx
else
    echo "❌ 未识别的包管理器，请手动安装 certbot"
    exit 1
fi
echo "✅ Certbot 已安装: $(certbot --version 2>&1)"

# ---------- 3. 确保 Nginx 已安装并运行 ----------
echo ""
echo "[2/6] 检查 Nginx..."
if ! command -v nginx &>/dev/null; then
    echo "  Nginx 未安装，正在安装..."
    apt-get install -y nginx || yum install -y nginx
fi
systemctl enable nginx
systemctl restart nginx
echo "✅ Nginx 已运行: $(nginx -v 2>&1)"

# ---------- 4. 申请证书 ----------
echo ""
echo "[3/6] 申请 Let's Encrypt 证书..."
echo "  域名: ${DOMAIN} www.${DOMAIN}"
echo "  邮箱: ${EMAIL}"
echo ""

# 先停掉 Nginx 释放 80 端口（如果用 standalone）或者保留 Nginx 用 webroot
# 这里用 --nginx 插件，让 certbot 自动管理 Nginx 配置
certbot --nginx \
    -d "${DOMAIN}" \
    -d "www.${DOMAIN}" \
    --non-interactive \
    --agree-tos \
    --email "${EMAIL}" \
    --redirect \
    --no-eff-email \
    || {
        echo ""
        echo "❌ certbot --nginx 申请失败，尝试 webroot 模式..."

        # 创建 webroot 目录
        mkdir -p "${WEBROOT_DIR}"

        # 先备份当前 Nginx 配置
        [ -f "${NGINX_CONF}" ] && cp "${NGINX_CONF}" "${NGINX_CONF}.bak.$(date +%s)"

        # 写入临时 HTTP 配置（用于 webroot 验证）
        cat > "${NGINX_CONF}" << TEMPEOF
server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN};
    location /.well-known/acme-challenge/ {
        root ${WEBROOT_DIR};
    }
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
TEMPEOF
        nginx -t && systemctl reload nginx

        # 用 webroot 方式申请
        certbot certonly \
            --webroot \
            -w "${WEBROOT_DIR}" \
            -d "${DOMAIN}" \
            -d "www.${DOMAIN}" \
            --non-interactive \
            --agree-tos \
            --email "${EMAIL}" \
            --no-eff-email
    }

echo ""
echo "✅ 证书已生成: ${CERT_DIR}"
ls -la "${CERT_DIR}/" 2>/dev/null || true

# ---------- 5. 部署 HTTPS Nginx 配置 ----------
echo ""
echo "[4/6] 部署 HTTPS Nginx 配置..."

# 备份旧配置
[ -f "${NGINX_CONF}" ] && cp "${NGINX_CONF}" "${NGINX_CONF}.http.bak.$(date +%s)"

# 优先使用本仓库内的 nginx-https.conf
if [ -f "${DEPLOY_DIR}/nginx-https.conf" ]; then
    echo "  使用 ${DEPLOY_DIR}/nginx-https.conf"
    cp "${DEPLOY_DIR}/nginx-https.conf" "${NGINX_CONF}"
else
    echo "  生成内置 HTTPS 配置..."
    cat > "${NGINX_CONF}" << 'NGINXEOF'
server {
    listen 80;
    server_name xiaoyxiao.xyz www.xiaoyxiao.xyz;
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }
    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl http2;
    server_name xiaoyxiao.xyz www.xiaoyxiao.xyz;

    ssl_certificate     /etc/letsencrypt/live/xiaoyxiao.xyz/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/xiaoyxiao.xyz/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options SAMEORIGIN always;

    client_max_body_size 100m;

    location /api/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 100m;
    }

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

    location /_next/static/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_cache_valid 200 30d;
        add_header Cache-Control "public, immutable";
    }

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
        client_max_body_size 100m;
    }
}
NGINXEOF
fi

# ---------- 6. 测试并重载 Nginx ----------
echo ""
echo "[5/6] 测试 Nginx 配置..."
if nginx -t; then
    echo "✅ Nginx 配置语法正确"
    systemctl reload nginx
    echo "✅ Nginx 已重载"
else
    echo "❌ Nginx 配置语法错误，请检查 ${NGINX_CONF}"
    echo "  备份文件: ${NGINX_CONF}.http.bak.*"
    exit 1
fi

# ---------- 7. 配置自动续期 ----------
echo ""
echo "[6/6] 配置证书自动续期..."
# 添加 cron 任务每天检查续期（certbot 默认每 12 小时尝试续期，但只有在剩 30 天时才真正续）
CRON_LINE="0 3 * * * /usr/bin/certbot renew --quiet --deploy-hook 'systemctl reload nginx'"
if ! crontab -l 2>/dev/null | grep -q "certbot renew"; then
    (crontab -l 2>/dev/null; echo "$CRON_LINE") | crontab -
    echo "✅ 已添加 cron 任务: 每天凌晨 3 点检查证书续期"
else
    echo "✅ certbot 续期任务已存在"
fi

# 测试续期（dry-run）
echo ""
echo "--- 测试证书续期（dry-run）---"
certbot renew --dry-run 2>&1 | tail -5 || true

# ---------- 完成 ----------
echo ""
echo "=========================================="
echo "  ✅ HTTPS 部署完成！"
echo "=========================================="
echo ""
echo "  HTTPS 访问地址:"
echo "    https://${DOMAIN}"
echo "    https://www.${DOMAIN}"
echo ""
echo "  HTTP 已自动跳转 HTTPS"
echo ""
echo "  证书路径:"
echo "    ${CERT_DIR}/fullchain.pem"
echo "    ${CERT_DIR}/privkey.pem"
echo ""
echo "  常用命令:"
echo "    查看 Nginx 状态:    systemctl status nginx"
echo "    重载 Nginx 配置:   nginx -s reload"
echo "    查看证书信息:       certbot certificates"
echo "    手动续期:           certbot renew"
echo "    测试续期:           certbot renew --dry-run"
echo "    查看 Nginx 错误日志: tail -f /var/log/nginx/error.log"
echo ""
echo "  证书有效期 90 天，自动续期已配置"
echo "  续期时会自动 reload Nginx"
echo ""
