# MeshLink Windows uninstaller (PowerShell)
param (
    [switch]$purge = $false
)

$ErrorActionPreference = "SilentlyContinue"

Write-Host "[INFO] Stopping MeshLink..."
Stop-ScheduledTask -TaskName "MeshLink"
Unregister-ScheduledTask -TaskName "MeshLink" -Confirm:$false
Get-Process "p2p-node" | Stop-Process -Force

$installDir = "C:\Program Files\MeshLink"

if ($purge) {
    Write-Host "[WARN] Removing all files, config, and keys..."
    Remove-Item -Path $installDir -Recurse -Force
} else {
    Write-Host "[INFO] Removing program files and keeping config..."
    Remove-Item -Path "$installDir\p2p-node.exe" -Force
    Remove-Item -Path "$installDir\wintun.dll" -Force
    Remove-Item -Path "$installDir\meshlink.ps1" -Force
    Remove-Item -Path "$installDir\meshlink.cmd" -Force
}

Write-Host "[OK] MeshLink has been uninstalled." -ForegroundColor Green
