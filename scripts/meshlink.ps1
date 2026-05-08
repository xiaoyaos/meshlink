# MeshLink Windows CLI 管理工具
param (
    [string]$command = "help",
    [string]$target = ""
)

$installDir = "C:\Program Files\MeshLink"
$addrFile = "$installDir\data\address.txt"
$stateFile = "$installDir\data\state.json"

switch ($command) {
    "stats" {
        Write-Host "=== MeshLink 节点状态报告 ===" -ForegroundColor Blue
        $proc = Get-Process "p2p-node" -ErrorAction SilentlyContinue
        if ($proc) {
            Write-Host "核心状态: 正在运行 (PID: $($proc.Id))" -ForegroundColor Green
        } else {
            Write-Host "核心状态: 已停止" -ForegroundColor Red
        }

        if (Test-Path $stateFile) {
            $state = Get-Content $stateFile | ConvertFrom-Json
            Write-Host ""
            Write-Host "[ 本机信息 ]" -ForegroundColor Cyan
            Write-Host "  虚拟 IP  : $($state.self_vip)"
            Write-Host "  节点 ID  : $($state.self_id)"

            Write-Host ""
            Write-Host "[ 已连接的对等节点 ]" -ForegroundColor Yellow
            Write-Host "  虚拟 IP        连接方式    节点 ID"
            foreach ($vip in $state.peers.PSObject.Properties.Name) {
                $p = $state.peers.$vip
                $mode = if ($p.direct) { "直连" } else { "中继" }
                Write-Host "  $($vip.PadRight(15)) $($mode.PadRight(8)) $($p.id)"
            }
        }

        Write-Host ""
        Write-Host "[ 简写地址 ]" -ForegroundColor Magenta
        if (Test-Path $addrFile) {
            $content = Get-Content $addrFile
            $found = $false
            foreach ($line in $content) {
                if ($line -like "*简写格式*") { $found = $true; continue }
                if ($line -like "*标准*") { $found = $false }
                if ($found -and $line.Trim() -match "^[0-9]") {
                    Write-Host "  🔗 $($line.Trim())"
                }
            }
        }
    }
    "test" {
        if ([string]::IsNullOrWhiteSpace($target)) {
            Write-Host "用法: .\meshlink.ps1 test <目标虚拟IP>"
            return
        }
        Write-Host "[测试] 正在测试到 $target 的 P2P 链路延迟..." -ForegroundColor Blue
        ping -n 4 $target
    }
    "start" {
        Start-ScheduledTask -TaskName "MeshLink"
        Write-Host "MeshLink 已启动。"
    }
    "stop" {
        Stop-ScheduledTask -TaskName "MeshLink"
        Get-Process "p2p-node" -ErrorAction SilentlyContinue | Stop-Process -Force
        Write-Host "MeshLink 已停止。"
    }
    "restart" {
        Stop-ScheduledTask -TaskName "MeshLink"
        Get-Process "p2p-node" -ErrorAction SilentlyContinue | Stop-Process -Force
        Start-ScheduledTask -TaskName "MeshLink"
        Write-Host "MeshLink 已重启。"
    }
    default {
        Write-Host "MeshLink Windows 管理工具"
        Write-Host "用法: powershell ./meshlink.ps1 {stats|start|stop|restart|test <IP>}"
    }
}
