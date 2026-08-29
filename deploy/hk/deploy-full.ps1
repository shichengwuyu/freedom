# 完整部署脚本：打包整个项目并上传到香港服务器
$projectRoot = 'f:\trae\wifi\infinite-canvas-main'
$serverIP = '149.88.78.8'
$serverUser = 'root'
$serverPath = '/root/freedom'
$tarFile = 'freedom-src-latest.tar.gz'

Write-Host "=== 开始打包项目 ===" -ForegroundColor Green

# 切换到项目目录
Set-Location $projectRoot

# 创建排除列表文件
$excludeFile = 'C:\Users\liuxaio\AppData\Local\Temp\tar-exclude.txt'
@"
node_modules
.next
.git
.trae
.agents
.claude
.github
data
tmp
*.exe
*.log
*.db
deploy/build
deploy/.npm-cache
deploy/infinite-canvas-deploy.tar.gz
# ===== 服务器特有的部署配置，禁止被本地覆盖 =====
deploy/hk/.env
deploy/hk/.env.*
deploy/hk/freedom-src-latest.tar.gz
"@ | Set-Content $excludeFile

# 打包源码（排除构建产物和数据文件）
Write-Host "正在打包源码..." -ForegroundColor Yellow
tar -czf $tarFile --exclude-from=$excludeFile .

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

# SSH 到服务器执行部署
Write-Host ""
Write-Host "=== 在服务器上解压并部署 ===" -ForegroundColor Green
Write-Host "请在 PowerShell 中执行以下命令：" -ForegroundColor Yellow
Write-Host ""
Write-Host ("ssh " + $serverUser + "@" + $serverIP) -ForegroundColor Cyan
Write-Host ("cd " + $serverPath) -ForegroundColor Cyan
Write-Host ("tar -xzf " + $tarFile) -ForegroundColor Cyan
Write-Host "cd deploy/hk" -ForegroundColor Cyan
Write-Host "bash build-deploy.sh" -ForegroundColor Cyan
Write-Host ""
Write-Host "或者一键执行：" -ForegroundColor Yellow
$oneStep = "ssh " + $serverUser + "@" + $serverIP + " 'cd " + $serverPath + " && tar -xzf " + $tarFile + " && cd deploy/hk && bash build-deploy.sh'"
Write-Host $oneStep -ForegroundColor Cyan

Write-Host ""
Write-Host "=== 部署说明 ===" -ForegroundColor Green
Write-Host "1. 上述命令会自动重建 app 镜像并重启容器" -ForegroundColor White
Write-Host "2. MySQL 和 MinIO 容器不会受影响" -ForegroundColor White
Write-Host "3. 部署完成后可通过 http://xiaoyxiao.xyz 访问" -ForegroundColor White
$logCmd = "4. 查看日志: ssh " + $serverUser + "@" + $serverIP + " 'docker logs -f freedom'"
Write-Host $logCmd -ForegroundColor White
