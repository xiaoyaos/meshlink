# MeshLink Windows CLI manager
param (
    [string]$command = "",
    [string]$target = ""
)

$installDir = "C:\Program Files\MeshLink"
$addrFile = "$installDir\data\address.txt"
$stateFile = "$installDir\data\state.json"
$reconnectFile = "$installDir\data\reconnect.request"

function Show-Help {
    Write-Host "MeshLink Windows manager" -ForegroundColor Green
    Write-Host ""
    Write-Host "Usage: meshlink.cmd <command> [args]"
    Write-Host ""
    Write-Host "Commands:"
    Write-Host "  stats           Show node status, virtual IP, and peer list"
    Write-Host "  start           Start MeshLink task"
    Write-Host "  stop            Stop MeshLink task and process"
    Write-Host "  restart         Restart MeshLink task"
    Write-Host "  test <ip>       Ping a virtual IP"
    Write-Host "  reconnect [ip|all]  Ask the core to retry direct P2P dialing"
    Write-Host "  -h, --help      Show help"
}

if ($command -eq "--help" -or $command -eq "-h" -or $command -eq "") {
    Show-Help
    return
}

switch ($command) {
    "stats" {
        Write-Host "=== MeshLink node status ===" -ForegroundColor Blue

        $version = "unknown"
        if (Test-Path $stateFile) {
            $state = Get-Content $stateFile | ConvertFrom-Json
            $version = $state.version
        } elseif (Test-Path $addrFile) {
            $content = Get-Content $addrFile
            $versionLine = $content | Select-Object -First 1
            if ($versionLine) {
                $version = $versionLine.Split(":")[1].Trim()
            }
        }
        Write-Host "Version: $version"

        $proc = Get-Process "p2p-node" -ErrorAction SilentlyContinue
        if ($proc) {
            Write-Host "Core: running (PID: $($proc.Id))" -ForegroundColor Green
        } else {
            Write-Host "Core: stopped" -ForegroundColor Red
        }

        if (Test-Path $stateFile) {
            $state = Get-Content $stateFile | ConvertFrom-Json
            Write-Host ""
            Write-Host "[ Local ]" -ForegroundColor Cyan
            Write-Host "  Virtual IP: $($state.self_vip)"
            Write-Host "  Peer ID   : $($state.self_id)"

            Write-Host ""
            Write-Host "[ Peers ]" -ForegroundColor Yellow
            Write-Host "  Virtual IP        Mode      OS         Hostname           Peer ID"
            foreach ($vip in $state.peers.PSObject.Properties.Name) {
                $p = $state.peers.$vip
                $mode = if ($p.direct) { "direct" } elseif ($p.connected -ne $false) { "relay" } else { "offline" }
                $peerOS = if ($p.os) { $p.os } else { "-" }
                $hostname = if ($p.hostname) { $p.hostname } else { "-" }
                if ($hostname.Length -gt 16) { $hostname = $hostname.Substring(0, 16) }
                Write-Host "  $($vip.PadRight(17)) $($mode.PadRight(9)) $($peerOS.PadRight(10)) $($hostname.PadRight(18)) $($p.id)"
            }
        }

        Write-Host ""
        Write-Host "[ Shorthand addresses ]" -ForegroundColor Magenta
        if (Test-Path $addrFile) {
            $content = Get-Content $addrFile
            foreach ($line in $content) {
                if ($line.Trim() -match "^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:[0-9]+:") {
                    Write-Host "  $($line.Trim())"
                }
            }
        }
    }
    "test" {
        if ([string]::IsNullOrWhiteSpace($target)) {
            Write-Host "Usage: meshlink.cmd test <virtual-ip>"
            return
        }
        Write-Host "[TEST] Pinging $target ..." -ForegroundColor Blue
        ping -n 4 $target
    }
    "reconnect" {
        $value = if ([string]::IsNullOrWhiteSpace($target)) { "all" } else { $target.Trim() }
        if (-not (Test-Path "$installDir\data")) {
            New-Item -ItemType Directory -Path "$installDir\data" | Out-Null
        }
        Set-Content -Path $reconnectFile -Value $value -Encoding UTF8
        Write-Host "MeshLink reconnect requested: $value"
    }
    "start" {
        Start-ScheduledTask -TaskName "MeshLink"
        Write-Host "MeshLink started."
    }
    "stop" {
        Stop-ScheduledTask -TaskName "MeshLink"
        Get-Process "p2p-node" -ErrorAction SilentlyContinue | Stop-Process -Force
        Write-Host "MeshLink stopped."
    }
    "restart" {
        Stop-ScheduledTask -TaskName "MeshLink"
        Get-Process "p2p-node" -ErrorAction SilentlyContinue | Stop-Process -Force
        Start-ScheduledTask -TaskName "MeshLink"
        Write-Host "MeshLink restarted."
    }
    default {
        Show-Help
    }
}
