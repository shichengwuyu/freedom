# 打包并部署到香港服务器
$projectRoot = "f:\trae\wifi\infinite-canvas-main"
$serverIP = "149.88.78.8"
$serverUser = "root"
$serverPath = "/root/freedom"
$tarFile = "freedom-src-latest.tar.gz"

Write-Host "=== 开始打包项目 ===" -ForegroundColor Green

# 切换到项目目录
Set-Location $projectRoot

# 打包整个项目（排除构建产物、数据文件、临时文件）
Write-Host "正在打包源码..." -ForegroundColor Yellow
tar --exclude='node_modules' `
    --exclude='.next' `
    --exclude='data' `
    --exclude='*.exe' `
    --exclude='*.log' `
    --exclude='deploy/build' `
    --exclude='.npm-cache' `
    --exclude='tmp' `
    --exclude='.trae' `
    --exclude='.agents' `
    --exclude='.claude' `
    --exclude='.github' `
    -czf $tarFile .

$size = [math]::Round((Get-Item $tarFile).Length / 1MB, 2)
Write-Host "打包完成: $tarFile ($size MB)" -ForegroundColor Green

# 上传到服务器
Write-Host "`n=== 上传到服务器 ===" -ForegroundColor Green
Write-Host "目标: ${serverUser}@${serverIP}:${serverPath}" -ForegroundColor Yellow

scp $tarFile "${serverUser}@${serverIP}:${serverPath}/"

if ($LASTEXITCODE -ne 0) {
    Write-Host "上传失败！" -ForegroundColor Red
    exit 1
}

Write-Host "上传完成" -ForegroundColor Green

# 显示部署命令
Write-Host "`n=== 部署命令 ===" -ForegroundColor Green
Write-Host "请 SSH 登录服务器后执行以下命令：" -ForegroundColor Yellow
Write-Host ""
Write-Host "ssh ${serverUser}@${serverIP}" -ForegroundColor Cyan
Write-Host "cd $serverPath" -ForegroundColor Cyan
Write-Host "tar -xzf $tarFile" -ForegroundColor Cyan
Write-Host "cd deploy/hk" -ForegroundColor Cyan
Write-Host "bash build-deploy.sh" -ForegroundColor Cyan
Write-Host ""
Write-Host "或者一键执行：" -ForegroundColor Yellow
Write-Host "ssh ${serverUser}@${serverIP} 'cd $serverPath && tar -xzf $tarFile && cd deploy/hk && bash build-deploy.sh'" -ForegroundColor Cyan
