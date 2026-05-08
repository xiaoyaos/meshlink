# MeshLink Windows 安装脚本 (PowerShell)
param (
    [string]$port = "4001",
    [string]$bootstrap = "",
    [switch]$relay = $false
)

$ErrorActionPreference = "Stop"

# 1. 检查管理员权限
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "请以管理员权限运行此脚本"
}

# 1.5 交互式向导 (如果未提供关键参数)
if ($bootstrap -eq "" -and $relay.IsPresent -eq $false) {
    Write-Host "╔══════════════════════════════════════════╗" -ForegroundColor Blue
    Write-Host "║      MeshLink Windows 交互式安装向导     ║" -ForegroundColor Blue
    Write-Host "╚══════════════════════════════════════════╝" -ForegroundColor Blue
    Write-Host ""
    
    Write-Host "请选择节点类型:"
    Write-Host "  1) 引导/中继节点 (通常用于公网服务器)"
    Write-Host "  2) 普通客户端 (加入现有网络)"
    $choice = Read-Host "选择 [1-2]"
    
    if ($choice -eq "1") {
        $relay = $true
    } else {
        $bootstrap = Read-Host "请输入引导节点地址 (格式 IP:Port:PeerID 或标准 Multiaddr)"
        while ([string]::IsNullOrWhiteSpace($bootstrap)) {
            $bootstrap = Read-Host "地址不能为空，请重新输入"
        }
    }
    Write-Host ""
}

$installDir = "C:\Program Files\MeshLink"
$binName = "p2p-node.exe"
$dllName = "wintun.dll"

# 2. 停止旧进程
Write-Host "[信息] 正在停止旧进程 (如有)..." -ForegroundColor Cyan
Stop-Process -Name "p2p-node" -Force -ErrorAction SilentlyContinue
Get-ScheduledTask -TaskName "MeshLink" -ErrorAction SilentlyContinue | Unregister-ScheduledTask -Confirm:$false -ErrorAction SilentlyContinue

# 3. 复制文件
Write-Host "[信息] 正在复制文件到 $installDir ..." -ForegroundColor Cyan
if (-not (Test-Path $installDir)) { New-Item -ItemType Directory -Path $installDir | Out-Null }
Copy-Item "$PSScriptRoot\p2p-node-windows-amd64.exe" "$installDir\$binName" -Force
Copy-Item "$PSScriptRoot\wintun.dll" "$installDir\$dllName" -Force
Copy-Item "$PSScriptRoot\meshlink.ps1" "$installDir\meshlink.ps1" -Force

# 4. 写入配置
$envFile = "$installDir\meshlink.env"
$envContent = "PORT=$port`nCONFIG_DIR=$installDir\data`nRELAY=$($relay.ToString().ToLower())`nBOOTSTRAP_ADDR=$bootstrap"
Set-Content -Path $envFile -Value $envContent

if (-not (Test-Path "$installDir\data")) { New-Item -ItemType Directory -Path "$installDir\data" | Out-Null }

# 5. 创建计划任务 (实现开机后台静默运行)
Write-Host "[信息] 正在创建后台服务 (计划任务)..." -ForegroundColor Cyan
$action = New-ScheduledTaskAction -Execute "$installDir\$binName" `
    -Argument "-port $port -config `"$installDir\data`" $(if($relay){"-relay"}) $(if($bootstrap){"-bootstrap $bootstrap"})" `
    -WorkingDirectory $installDir
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
Register-ScheduledTask -TaskName "MeshLink" -Action $action -Trigger $trigger -Settings $settings -User "SYSTEM" -RunLevel Highest

# 6. 启动任务
Start-ScheduledTask -TaskName "MeshLink"

Write-Host ""
Write-Host "✅ MeshLink 已在 Windows 上成功安装并作为后台服务运行！" -ForegroundColor Green
Write-Host "常用命令: powershell `"$installDir\meshlink.ps1`" status"
