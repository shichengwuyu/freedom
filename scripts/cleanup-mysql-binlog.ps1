<#
.SYNOPSIS
    安全清理 MySQL 8.0 二进制日志 (binlog)

.DESCRIPTION
    - 通过 mysql.exe 连接数据库
    - 列出当前所有 binlog 和大小
    - 交互式确认后执行 PURGE BINARY LOGS
    - 默认保留 7 天，可通过参数调整
    - 同时清理 .err 错误日志（可选）

.PARAMETER MySqlExe
    mysql.exe 路径，默认 "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe"

.PARAMETER User
    MySQL 用户名，默认 root

.PARAMETER DaysToKeep
    保留最近 N 天的 binlog，默认 7

.PARAMETER DryRun
    仅列出 binlog，不实际删除

.PARAMETER ClearErrorLog
    是否同时清理 .err 错误日志（默认 true）

.EXAMPLE
    .\cleanup-mysql-binlog.ps1
    .\cleanup-mysql-binlog.ps1 -DaysToKeep 14
    .\cleanup-mysql-binlog.ps1 -DryRun
#>

[CmdletBinding()]
param(
    [string]$MySqlExe = "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe",
    [string]$User = "root",
    [int]$DaysToKeep = 7,
    [switch]$DryRun,
    [bool]$ClearErrorLog = $true
)

# ============= 颜色辅助 =============
function Write-Step($msg)  { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }
function Write-Ok($msg)    { Write-Host "[OK] $msg" -ForegroundColor Green }
function Write-Warn($msg)  { Write-Host "[!] $msg" -ForegroundColor Yellow }
function Write-Err($msg)   { Write-Host "[X] $msg" -ForegroundColor Red }

# ============= 0. 预检 =============
Write-Step "预检环境"

if (-not (Test-Path $MySqlExe)) {
    Write-Err "找不到 mysql.exe: $MySqlExe"
    Write-Host "请用 -MySqlExe 参数指定正确路径" -ForegroundColor Yellow
    exit 1
}
Write-Ok "mysql.exe 已找到: $MySqlExe"

# 检查 mysqld 是否运行
$mysqld = Get-Process mysqld -ErrorAction SilentlyContinue
if (-not $mysqld) {
    Write-Warn "未检测到 mysqld 进程在运行"
    Write-Host "  仍然继续（如果服务是以 Network Service 运行可能检测不到）" -ForegroundColor Gray
} else {
    Write-Ok "mysqld 正在运行 (PID $($mysqld.Id))"
}

# ============= 1. 询问密码 =============
Write-Step "连接 MySQL"
$securePwd = Read-Host "请输入 MySQL [$User] 的密码" -AsSecureString
$pwdPtr = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePwd)
$plainPwd = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto($pwdPtr)
[System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pwdPtr)

# 构造 mysql 参数（用 --defaults-file 之类的容易泄露，这里用环境变量 + 短连接）
$env:MYSQL_PWD = $plainPwd
$plainPwd = $null
[System.GC]::Collect()

function Invoke-MySQL($sql) {
    & $MySqlExe -u $User --skip-column-names -B -e $sql 2>&1
}

# 连接测试
$test = Invoke-MySQL "SELECT VERSION();"
if ($LASTEXITCODE -ne 0) {
    Write-Err "连接 MySQL 失败"
    Write-Host $test
    exit 1
}
Write-Ok "连接成功: $($test[0])"

# ============= 2. 列出 binlog =============
Write-Step "当前 binlog 列表"
$logs = Invoke-MySQL "SHOW BINARY LOGS;"
if (-not $logs -or $logs.Count -eq 0) {
    Write-Warn "未获取到 binlog 列表（可能未启用 binlog）"
} else {
    $totalMB = 0
    $cutoffTime = (Get-Date).AddDays(-$DaysToKeep)
    Write-Host ("{0,-30} {1,15} {2,-20}" -f "Log_name", "File_size(B)", "Created(本地时间)")
    Write-Host ("-" * 70)
    foreach ($line in $logs) {
        $parts = $line -split "`t"
        $name = $parts[0]
        $size = if ($parts[1]) { [int64]$parts[1] } else { 0 }
        $totalMB += $size / 1MB
        Write-Host ("{0,-30} {1,15:N0} {2,-20}" -f $name, $size, "-")
    }
    Write-Host ("-" * 70)
    Write-Host ("总计 {0} 个文件，约 {1:N2} MB" -f $logs.Count, $totalMB) -ForegroundColor Cyan
}

# ============= 3. Dry-Run 模式 =============
if ($DryRun) {
    Write-Step "Dry-Run 模式 - 仅展示，不删除"
    Write-Host "若要实际执行，请去掉 -DryRun 参数" -ForegroundColor Yellow
    $env:MYSQL_PWD = $null
    exit 0
}

# ============= 4. 计算要清理的截止时间 =============
$cutoff = (Get-Date).AddDays(-$DaysToKeep).ToString("yyyy-MM-dd HH:mm:ss")
Write-Step "清理策略"
Write-Host "保留最近 $DaysToKeep 天的 binlog"
Write-Host "将清理 $cutoff 之前的所有 binlog"

# ============= 5. 确认 =============
Write-Host ""
$confirm = Read-Host "确认执行清理吗？(yes/no)"
if ($confirm -ne "yes") {
    Write-Warn "已取消"
    $env:MYSQL_PWD = $null
    exit 0
}

# ============= 6. 执行 PURGE =============
Write-Step "执行 PURGE BINARY LOGS"
$purgeSql = "PURGE BINARY LOGS BEFORE '$cutoff';"
Write-Host "SQL: $purgeSql"
$out = Invoke-MySQL $purgeSql
if ($LASTEXITCODE -ne 0) {
    Write-Err "PURGE 失败"
    Write-Host $out
    $env:MYSQL_PWD = $null
    exit 1
}
Write-Ok "PURGE 执行成功"

# ============= 7. 清理错误日志（可选） =============
if ($ClearErrorLog) {
    Write-Step "检查错误日志"
    $errLogs = Get-ChildItem "C:\ProgramData\MySQL\MySQL Server 8.0\Data\*.err" -ErrorAction SilentlyContinue
    foreach ($err in $errLogs) {
        Write-Host "找到: $($err.Name) ($([math]::Round($err.Length/1KB, 2)) KB)"
        $ec = Read-Host "  是否清空此错误日志？(yes/no)"
        if ($ec -eq "yes") {
            # 用 FLUSH ERROR LOGS + FLUSH LOGS 让 MySQL 重新打开日志文件，然后清空
            Invoke-MySQL "FLUSH ERROR LOGS;" | Out-Null
            # 用 truncate 方式（> ""）而非删除，更安全
            $truncated = $false
            try {
                # PowerShell 5.1 兼容：写入空内容
                $stream = [System.IO.File]::Open($err.FullName, 'Open', 'Write', 'None')
                $stream.SetLength(0)
                $stream.Close()
                $truncated = $true
            } catch {
                Write-Warn "  truncate 失败: $_"
            }
            if ($truncated) {
                Write-Ok "  已清空 $($err.Name)"
            }
        }
    }
}

# ============= 8. 收尾 =============
Write-Step "清理后的 binlog 列表"
$afterLogs = Invoke-MySQL "SHOW BINARY LOGS;"
if ($afterLogs) {
    $afterTotal = 0
    foreach ($line in $afterLogs) {
        $parts = $line -split "`t"
        $size = if ($parts[1]) { [int64]$parts[1] } else { 0 }
        $afterTotal += $size / 1MB
    }
    Write-Host ("剩余 {0} 个文件，约 {1:N2} MB" -f $afterLogs.Count, $afterTotal) -ForegroundColor Green
}

$env:MYSQL_PWD = $null
Write-Ok "完成"
