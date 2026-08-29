# ============================================================
# 一键部署脚本 - 从 Windows 自动部署到 CentOS 服务器
# 用法：在 PowerShell 中运行 .\deploy\deploy-all.ps1
# 服务器：43.248.3.138:10233 (root)
# 域名：xiaoyxiao.xyz
# ============================================================

$ErrorActionPreference = "Stop"

# ==================== 配置 ====================
$ServerIP = "43.248.3.138"
$ServerPort = "10233"
$ServerUser = "root"
$ServerPassword = "hu0aDCULerjL"
$Domain = "xiaoyxiao.xyz"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$DeployDir = Join-Path $ProjectRoot "deploy\build"
$ArchiveName = "infinite-canvas-deploy.tar.gz"
$ArchivePath = Join-Path $ProjectRoot "deploy\$ArchiveName"
$RemoteProjectDir = "/opt/infinite-canvas"

function Write-Step($msg) {
    Write-Host ""
    Write-Host ">>> $msg" -ForegroundColor Cyan
}

function Write-Ok($msg) {
    Write-Host "  OK $msg" -ForegroundColor Green
}

function Write-Err($msg) {
    Write-Host "  FAIL $msg" -ForegroundColor Red
}

# ==================== 步骤 1: 构建前端 ====================
Write-Step "1/6 构建前端 Next.js..."
Push-Location (Join-Path $ProjectRoot "web")
try {
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "前端构建失败" }
    Write-Ok "前端构建完成"
} finally {
    Pop-Location
}

# ==================== 步骤 2: 交叉编译后端 ====================
Write-Step "2/6 交叉编译后端 Go (linux/amd64)..."
Push-Location $ProjectRoot
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $DeployDir "server") main.go 2>&1 | ForEach-Object { Write-Host $_ }
    if ($LASTEXITCODE -ne 0) { throw "后端编译失败" }
    Write-Ok "后端编译完成"
} finally {
    Pop-Location
    $env:CGO_ENABLED = ""
    $env:GOOS = ""
    $env:GOARCH = ""
}

# ==================== 步骤 3: 组装部署包 ====================
Write-Step "3/6 组装部署文件..."

# 清理并创建部署目录
if (Test-Path $DeployDir) { Remove-Item -Recurse -Force $DeployDir }
New-Item -ItemType Directory -Path $DeployDir -Force | Out-Null

# 复制前端 standalone
$StandaloneDir = Join-Path $ProjectRoot "web\.next\standalone"
$StaticDir = Join-Path $ProjectRoot "web\.next\static"
$PublicDir = Join-Path $ProjectRoot "web\public"

New-Item -ItemType Directory -Path (Join-Path $DeployDir "web") -Force | Out-Null
Copy-Item -Recurse -Force "$StandaloneDir\*" (Join-Path $DeployDir "web\")
New-Item -ItemType Directory -Path (Join-Path $DeployDir "web\.next\static") -Force | Out-Null
Copy-Item -Recurse -Force "$StaticDir\*" (Join-Path $DeployDir "web\.next\static\")
if (Test-Path $PublicDir) {
    Copy-Item -Recurse -Force $PublicDir (Join-Path $DeployDir "web\public")
}
Write-Ok "前端文件已复制"

# 重新编译 server 到 deploy 目录（前面可能因目录清理丢失）
Push-Location $ProjectRoot
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $DeployDir "server") main.go
} finally {
    Pop-Location
    $env:CGO_ENABLED = ""
    $env:GOOS = ""
    $env:GOARCH = ""
}
Write-Ok "后端二进制已复制"

# 复制数据目录
New-Item -ItemType Directory -Path (Join-Path $DeployDir "data") -Force | Out-Null
$DataDir = Join-Path $ProjectRoot "data"
if (Test-Path $DataDir) {
    Get-ChildItem -Path $DataDir -Recurse | ForEach-Object {
        $rel = $_.FullName.Substring($DataDir.Length)
        $dest = Join-Path (Join-Path $DeployDir "data") $rel
        if ($_.PSIsContainer) {
            New-Item -ItemType Directory -Path $dest -Force | Out-Null
        } else {
            $destDir = Split-Path $dest -Parent
            if (!(Test-Path $destDir)) { New-Item -ItemType Directory -Path $destDir -Force | Out-Null }
            Copy-Item $_.FullName $dest -Force
        }
    }
}
Write-Ok "数据目录已复制"

# 生成 .env
$EnvContent = @"
ADMIN_USERNAME=admin
ADMIN_PASSWORD=infinite-canvas
JWT_SECRET=infinite-canvas-change-me-in-production
JWT_EXPIRE_HOURS=168
PORT=8080
PUBLIC_BASE_URL=https://$Domain
API_BASE_URL=http://127.0.0.1:8080
STORAGE_DRIVER=mysql
DATABASE_DSN=data/infinite-canvas.db
LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A
"@
Set-Content -Path (Join-Path $DeployDir ".env") -Value $EnvContent -Encoding UTF8
Write-Ok ".env 已生成"

# ==================== 步骤 4: 打包 ====================
Write-Step "4/6 打包压缩..."
Push-Location $DeployDir
try {
    tar -czf $ArchivePath .
} finally {
    Pop-Location
}
$FileSize = [math]::Round((Get-Item $ArchivePath).Length / 1MB, 2)
Write-Ok "打包完成 (${FileSize} MB)"

# ==================== 步骤 5: 上传到服务器 ====================
Write-Step "5/6 上传到服务器 ${ServerUser}@${ServerIP}:${ServerPort}..."
Write-Host "  (首次连接需要输入密码: $ServerPassword)" -ForegroundColor Yellow

scp -P $ServerPort -o StrictHostKeyChecking=accept-new $ArchivePath "${ServerUser}@${ServerIP}:/tmp/${ArchiveName}"
if ($LASTEXITCODE -ne 0) {
    Write-Err "上传失败"
    exit 1
}
Write-Ok "上传完成"

# ==================== 步骤 6: 远程执行部署 ====================
Write-Step "6/6 远程部署到服务器..."

$RemoteScript = @"
set -e
DOMAIN="$Domain"
PROJECT_DIR="$RemoteProjectDir"

echo "--- 安装依赖 ---"
yum install -y epel-release 2>/dev/null
yum install -y curl wget git nginx tar 2>/dev/null

# 安装 Node.js 18
if ! command -v node &>/dev/null || [[ \$(node --version 2>/dev/null | cut -d'v' -f2 | cut -d'.' -f1) -lt 18 ]]; then
    curl -fsSL https://rpm.nodesource.com/setup_18.x | bash - 2>/dev/null
    yum install -y nodejs 2>/dev/null
fi
echo "Node: \$(node --version)"

# 安装 PM2
npm install -g pm2 2>/dev/null || true

# 解压部署包
echo "--- 解压部署包 ---"
mkdir -p \$PROJECT_DIR
rm -rf \$PROJECT_DIR/web \$PROJECT_DIR/server \$PROJECT_DIR/data
tar -xzf /tmp/$ArchiveName -C \$PROJECT_DIR
chmod +x \$PROJECT_DIR/server

# 确保 .env 存在
if [ ! -f "\$PROJECT_DIR/.env" ]; then
    cat > \$PROJECT_DIR/.env << 'ENVEOF'
ADMIN_USERNAME=admin
ADMIN_PASSWORD=infinite-canvas
JWT_SECRET=infinite-canvas-change-me-in-production
JWT_EXPIRE_HOURS=168
PORT=8080
PUBLIC_BASE_URL=https://$Domain
API_BASE_URL=http://127.0.0.1:8080
STORAGE_DRIVER=mysql
DATABASE_DSN=data/infinite-canvas.db
LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A
ENVEOF
fi

# 停止旧服务
pm2 delete all 2>/dev/null || true

# 启动后端 (cwd 必须是项目根目录，才能读取 .env)
echo "--- 启动后端 ---"
cd \$PROJECT_DIR
pm2 start ./server --name backend --cwd \$PROJECT_DIR --max-memory-restart 512M
sleep 2

# 启动前端
echo "--- 启动前端 ---"
pm2 start node --name frontend --cwd \$PROJECT_DIR/web -- server.js
sleep 2

# 保存 PM2 进程列表（开机自启）
pm2 save
pm2 startup systemd -u root --hp /root 2>/dev/null || true

# 配置 Nginx
echo "--- 配置 Nginx ---"
cat > /etc/nginx/conf.d/infinite-canvas.conf << NGINXEOF
server {
    listen 80;
    server_name $Domain www.$Domain;

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

# 移除默认 nginx 配置（避免冲突）
rm -f /etc/nginx/conf.d/default.conf 2>/dev/null

nginx -t && systemctl restart nginx && systemctl enable nginx

# 开放防火墙端口
firewall-cmd --permanent --add-service=http 2>/dev/null || true
firewall-cmd --permanent --add-service=https 2>/dev/null || true
firewall-cmd --reload 2>/dev/null || true

echo ""
echo "=========================================="
echo "  部署完成！"
echo "=========================================="
echo "  访问: http://$Domain"
echo "  管理员: admin / infinite-canvas"
echo ""
echo "  PM2 状态:"
pm2 status
echo ""
echo "  后端日志: pm2 logs backend"
echo "  前端日志: pm2 logs frontend"
"@

# 将脚本写入服务器并执行
$TempScript = "/tmp/deploy-infinite-canvas.sh"
ssh -P $ServerPort -o StrictHostKeyChecking=accept-new "${ServerUser}@${ServerIP}" "cat > $TempScript << 'SCRIPT_EOF'
$RemoteScript
SCRIPT_EOF
chmod +x $TempScript && bash $TempScript"

if ($LASTEXITCODE -eq 0) {
    Write-Ok "远程部署完成"
} else {
    Write-Err "远程部署可能有问题，请检查输出"
}

# ==================== 完成 ====================
Write-Host ""
Write-Host "==========================================" -ForegroundColor Green
Write-Host "  部署流程结束！" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
Write-Host ""
Write-Host "  访问地址: http://${Domain}" -ForegroundColor White
Write-Host "  管理员账号: admin / infinite-canvas" -ForegroundColor White
Write-Host ""
Write-Host "  后续操作：" -ForegroundColor Yellow
Write-Host "  1. 在阿里云域名控制台添加 A 记录: ${Domain} -> ${ServerIP}" -ForegroundColor White
Write-Host "  2. 建议配置 HTTPS (certbot)" -ForegroundColor White
Write-Host "  3. 建议修改 JWT_SECRET 为随机字符串" -ForegroundColor White
Write-Host ""
