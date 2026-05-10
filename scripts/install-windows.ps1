# MeshLink Windows installer (PowerShell)
param (
    [string]$port = "4001",
    [string]$bootstrap = "",
    [switch]$relay = $false
)

$ErrorActionPreference = "Stop"

$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "Please run this installer as Administrator."
}

if ($bootstrap -eq "" -and $relay.IsPresent -eq $false) {
    Write-Host "========================================" -ForegroundColor Blue
    Write-Host "       MeshLink Windows Installer        " -ForegroundColor Blue
    Write-Host "========================================" -ForegroundColor Blue
    Write-Host ""
    Write-Host "Select node type:"
    Write-Host "  1) Bootstrap / relay node"
    Write-Host "  2) Client node"
    $choice = Read-Host "Select [1-2]"

    if ($choice -eq "1") {
        $relay = $true
    } else {
        $bootstrap = Read-Host "Enter bootstrap address (IP:Port:PeerID or multiaddr)"
        while ([string]::IsNullOrWhiteSpace($bootstrap)) {
            $bootstrap = Read-Host "Bootstrap address cannot be empty"
        }
    }
    Write-Host ""
}

if ([string]::IsNullOrWhiteSpace($bootstrap) -and $relay.IsPresent -eq $false) {
    Write-Error "Client mode requires bootstrap address. Use -bootstrap <ADDR>, or use -relay for bootstrap/relay node."
}

$installDir = "C:\Program Files\MeshLink"
$binName = "p2p-node.exe"
$dllName = "wintun.dll"

Write-Host "[INFO] Stopping old MeshLink process and task if present..." -ForegroundColor Cyan
Stop-Process -Name "p2p-node" -Force -ErrorAction SilentlyContinue
Get-ScheduledTask -TaskName "MeshLink" -ErrorAction SilentlyContinue | Unregister-ScheduledTask -Confirm:$false -ErrorAction SilentlyContinue

Write-Host "[INFO] Copying files to $installDir ..." -ForegroundColor Cyan
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir | Out-Null
}
Copy-Item "$PSScriptRoot\p2p-node-windows-amd64.exe" "$installDir\$binName" -Force
Copy-Item "$PSScriptRoot\wintun.dll" "$installDir\$dllName" -Force
Copy-Item "$PSScriptRoot\meshlink.ps1" "$installDir\meshlink.ps1" -Force
if (Test-Path "$PSScriptRoot\meshlink.cmd") {
    Copy-Item "$PSScriptRoot\meshlink.cmd" "$installDir\meshlink.cmd" -Force
}

$envFile = "$installDir\meshlink.env"
$envContent = "PORT=$port`nCONFIG_DIR=$installDir\data`nRELAY=$($relay.ToString().ToLower())`nBOOTSTRAP_ADDR=$bootstrap"
Set-Content -Path $envFile -Value $envContent -Encoding ASCII

if (-not (Test-Path "$installDir\data")) {
    New-Item -ItemType Directory -Path "$installDir\data" | Out-Null
}

Write-Host "[INFO] Creating scheduled task..." -ForegroundColor Cyan
$taskArgs = "-port $port -config `"$installDir\data`" -logfile `"$installDir\meshlink.log`""
if ($relay) {
    $taskArgs = "$taskArgs -relay"
}
if (-not [string]::IsNullOrWhiteSpace($bootstrap)) {
    $taskArgs = "$taskArgs -bootstrap `"$bootstrap`""
}
$action = New-ScheduledTaskAction -Execute "$installDir\$binName" -Argument $taskArgs -WorkingDirectory $installDir
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
Register-ScheduledTask -TaskName "MeshLink" -Action $action -Trigger $trigger -Settings $settings -User "SYSTEM" -RunLevel Highest

Write-Host "[INFO] Configuring firewall rules..." -ForegroundColor Cyan
New-NetFirewallRule -DisplayName "MeshLink P2P Traffic TCP" -Direction Inbound -LocalPort $port -Protocol TCP -Action Allow -ErrorAction SilentlyContinue | Out-Null
New-NetFirewallRule -DisplayName "MeshLink P2P Traffic UDP" -Direction Inbound -LocalPort $port -Protocol UDP -Action Allow -ErrorAction SilentlyContinue | Out-Null

Start-ScheduledTask -TaskName "MeshLink"

Write-Host ""
Write-Host "[OK] MeshLink has been installed and started." -ForegroundColor Green
Write-Host "Use: `"$installDir\meshlink.cmd`" stats"
