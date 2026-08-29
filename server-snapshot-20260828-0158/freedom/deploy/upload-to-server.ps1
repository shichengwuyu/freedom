# ============================================================
# 上传脚本 - 将打包好的文件上传到服务器
# 用法：在 PowerShell 中运行 .\deploy\upload-to-server.ps1
# ============================================================

$ServerIP = "43.248.3.138"
$ServerPort = "10233"
$ServerUser = "root"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$ArchivePath = Join-Path $ProjectRoot "deploy\infinite-canvas-deploy.tar.gz"

Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  上传部署包到服务器" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""

# 检查打包文件是否存在
if (!(Test-Path $ArchivePath)) {
    Write-Host "❌ 未找到打包文件: $ArchivePath" -ForegroundColor Red
    Write-Host "   请先运行 .\deploy\package-for-server.ps1" -ForegroundColor Yellow
    exit 1
}

$FileSize = [math]::Round((Get-Item $ArchivePath).Length / 1MB, 2)
Write-Host "  文件大小: ${FileSize} MB" -ForegroundColor White
Write-Host ""

Write-Host "正在上传到 ${ServerUser}@${ServerIP}:${ServerPort} ..." -ForegroundColor Green
Write-Host "(首次连接需要输入密码: hu0aDCULerjL)" -ForegroundColor Yellow
Write-Host ""

# 使用 scp 上传
scp -P $ServerPort -o StrictHostKeyChecking=accept-new $ArchivePath "${ServerUser}@${ServerIP}:/tmp/infinite-canvas-deploy.tar.gz"

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "✅ 上传成功！" -ForegroundColor Green
    Write-Host ""
    Write-Host "下一步：SSH 登录服务器执行部署" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  ssh -p ${ServerPort} ${ServerUser}@${ServerIP}" -ForegroundColor White
    Write-Host ""
    Write-Host "登录后执行：" -ForegroundColor Yellow
    Write-Host "  cd /tmp && tar -xzf infinite-canvas-deploy.tar.gz -C /opt/infinite-canvas --strip-components=1" -ForegroundColor White
    Write-Host "  cd /opt/infinite-canvas && chmod +x server deploy.sh" -ForegroundColor White
    Write-Host "  ./deploy.sh" -ForegroundColor White
    Write-Host ""
} else {
    Write-Host ""
    Write-Host "❌ 上传失败，请检查网络连接和密码" -ForegroundColor Red
}
