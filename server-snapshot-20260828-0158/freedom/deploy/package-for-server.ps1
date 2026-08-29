# ============================================================
# 本地打包脚本 - 在 Windows 上运行
# 功能：构建前端 + 交叉编译后端 + 打包部署文件
# 用法：在 PowerShell 中运行 .\deploy\package-for-server.ps1
# ============================================================

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$DeployDir = Join-Path $ProjectRoot "deploy\build"

Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  infinite-canvas 部署打包工具" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""

# ---------- 清理旧构建 ----------
if (Test-Path $DeployDir) {
    Write-Host "[清理] 删除旧构建目录..." -ForegroundColor Yellow
    Remove-Item -Recurse -Force $DeployDir
}
New-Item -ItemType Directory -Path $DeployDir -Force | Out-Null

# ---------- 1. 构建前端 ----------
Write-Host "[1/4] 构建前端 Next.js..." -ForegroundColor Green
Push-Location (Join-Path $ProjectRoot "web")
try {
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "前端构建失败" }
    Write-Host "  ✅ 前端构建完成" -ForegroundColor Green
} finally {
    Pop-Location
}

# 复制 standalone 输出
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

# ---------- 2. 交叉编译后端 (Go linux/amd64) ----------
Write-Host "[2/4] 交叉编译后端 Go (linux/amd64)..." -ForegroundColor Green
Push-Location $ProjectRoot
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o (Join-Path $DeployDir "server") main.go
    if ($LASTEXITCODE -ne 0) { throw "后端编译失败" }
    Write-Host "  ✅ 后端编译完成" -ForegroundColor Green
} finally {
    Pop-Location
    $env:CGO_ENABLED = ""
    $env:GOOS = ""
    $env:GOARCH = ""
}

# ---------- 3. 复制数据目录 ----------
Write-Host "[3/4] 复制数据目录..." -ForegroundColor Green
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
    Write-Host "  ✅ 数据目录复制完成" -ForegroundColor Green
}

# ---------- 4. 生成配置文件 ----------
Write-Host "[4/4] 生成配置文件..." -ForegroundColor Green

# .env 文件
$EnvContent = @"
# 管理员账号
ADMIN_USERNAME=admin
ADMIN_PASSWORD=infinite-canvas

# JWT 密钥（正式部署请修改为随机字符串！）
JWT_SECRET=infinite-canvas-change-me-in-production
JWT_EXPIRE_HOURS=168

# 后端端口
PORT=8080

# 公开访问地址
PUBLIC_BASE_URL=https://xiaoyxiao.xyz

# 前端 API 代理目标
API_BASE_URL=http://127.0.0.1:8080

# 数据库
STORAGE_DRIVER=mysql
DATABASE_DSN=data/infinite-canvas.db

# 卡密购买链接
LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A
"@
Set-Content -Path (Join-Path $DeployDir ".env") -Value $EnvContent -Encoding UTF8

# PM2 ecosystem 配置
$Pm2Config = @"
module.exports = {
  apps: [
    {
      name: 'backend',
      script: './server',
      cwd: '/opt/infinite-canvas',
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: '512M'
    },
    {
      name: 'frontend',
      script: 'node',
      args: 'server.js',
      cwd: '/opt/infinite-canvas/web',
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: '1G',
      env: {
        NODE_ENV: 'production',
        PORT: '3000',
        API_BASE_URL: 'http://127.0.0.1:8080'
      }
    }
  ]
};
"@
Set-Content -Path (Join-Path $DeployDir "ecosystem.config.js") -Value $Pm2Config -Encoding UTF8

# Nginx 配置
$NginxConfig = @"
server {
    listen 80;
    server_name xiaoyxiao.xyz www.xiaoyxiao.xyz;

    location /_next/static/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_cache_valid 200 30d;
        add_header Cache-Control "public, immutable";
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host `$host;
        proxy_set_header X-Real-IP `$remote_addr;
        proxy_set_header X-Forwarded-For `$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto `$scheme;
        client_max_body_size 50m;
    }

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade `$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host `$host;
        proxy_set_header X-Real-IP `$remote_addr;
        proxy_set_header X-Forwarded-For `$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto `$scheme;
        proxy_cache_bypass `$http_upgrade;
        client_max_body_size 50m;
    }
}
"@
Set-Content -Path (Join-Path $DeployDir "nginx-infinite-canvas.conf") -Value $NginxConfig -Encoding UTF8

# ---------- 打包 ----------
Write-Host ""
Write-Host "正在打包..." -ForegroundColor Green
$ArchiveName = "infinite-canvas-deploy.tar.gz"
$ArchivePath = Join-Path $ProjectRoot "deploy\$ArchiveName"

Push-Location $DeployDir
try {
    tar -czf $ArchivePath .
} finally {
    Pop-Location
}

$FileSize = [math]::Round((Get-Item $ArchivePath).Length / 1MB, 2)

Write-Host ""
Write-Host "==========================================" -ForegroundColor Green
Write-Host "  ✅ 打包完成！" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
Write-Host ""
Write-Host "  文件: $ArchivePath" -ForegroundColor White
Write-Host "  大小: ${FileSize} MB" -ForegroundColor White
Write-Host ""
Write-Host "  下一步操作：" -ForegroundColor Yellow
Write-Host "  1. 运行 .\deploy\upload-to-server.ps1 上传到服务器" -ForegroundColor White
Write-Host "  2. SSH 登录服务器执行部署脚本" -ForegroundColor White
Write-Host ""
