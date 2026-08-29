#!/bin/bash
set -e

DOMAIN="xiaoyxiao.xyz"
PROJECT_DIR="/opt/infinite-canvas"
ARCHIVE_NAME="infinite-canvas-deploy.tar.gz"

echo "--- 安装依赖 ---"
yum install -y epel-release 2>/dev/null || true
yum install -y curl wget git nginx tar 2>/dev/null || apt-get update -y && apt-get install -y curl wget git nginx tar 2>/dev/null || true

# 安装 Node.js 18
if ! command -v node &>/dev/null || [[ $(node --version 2>/dev/null | cut -d'v' -f2 | cut -d'.' -f1) -lt 18 ]]; then
    curl -fsSL https://rpm.nodesource.com/setup_18.x | bash - 2>/dev/null || \
    curl -fsSL https://deb.nodesource.com/setup_18.x | bash - 2>/dev/null
    yum install -y nodejs 2>/dev/null || apt-get install -y nodejs 2>/dev/null
fi
echo "Node: $(node --version 2>/dev/null)"

# 安装 PM2
npm install -g pm2 2>/dev/null || true

# 解压部署包
echo "--- 解压部署包 ---"
mkdir -p $PROJECT_DIR
rm -rf $PROJECT_DIR/web $PROJECT_DIR/server $PROJECT_DIR/data
tar -xzf /tmp/$ARCHIVE_NAME -C $PROJECT_DIR
chmod +x $PROJECT_DIR/server

# 确保 .env 存在
if [ ! -f "$PROJECT_DIR/.env" ]; then
    cat > $PROJECT_DIR/.env << 'ENVEOF'
ADMIN_USERNAME=admin
ADMIN_PASSWORD=infinite-canvas
JWT_SECRET=infinite-canvas-change-me-in-production
JWT_EXPIRE_HOURS=168
PORT=8080
PUBLIC_BASE_URL=https://xiaoyxiao.xyz
API_BASE_URL=http://127.0.0.1:8080
STORAGE_DRIVER=mysql
DATABASE_DSN=data/infinite-canvas.db
LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A
ENVEOF
fi

# 停止旧服务
pm2 delete all 2>/dev/null || true

# 启动后端
echo "--- 启动后端 ---"
cd $PROJECT_DIR
pm2 start ./server --name backend --cwd $PROJECT_DIR --max-memory-restart 512M
sleep 2

# 启动前端
echo "--- 启动前端 ---"
pm2 start node --name frontend --cwd $PROJECT_DIR/web -- server.js
sleep 2

# 保存 PM2 进程列表（开机自启）
pm2 save
pm2 startup systemd -u root --hp /root 2>/dev/null || true

# 配置 Nginx
echo "--- 配置 Nginx ---"
cat > /etc/nginx/conf.d/infinite-canvas.conf << NGINXEOF
server {
    listen 80;
    server_name $DOMAIN www.$DOMAIN;

    location /_next/static/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_cache_valid 200 30d;
        add_header Cache-Control "public, immutable";
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        client_max_body_size 50m;
    }

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        client_max_body_size 50m;
    }
}
NGINXEOF

rm -f /etc/nginx/conf.d/default.conf 2>/dev/null || true
nginx -t && systemctl restart nginx && systemctl enable nginx

firewall-cmd --permanent --add-service=http 2>/dev/null || true
firewall-cmd --permanent --add-service=https 2>/dev/null || true
firewall-cmd --reload 2>/dev/null || true

echo ""
echo "=========================================="
echo "  部署完成！"
echo "=========================================="
echo "  访问: http://$DOMAIN"
echo "  管理员: admin / infinite-canvas"
echo ""
pm2 status
