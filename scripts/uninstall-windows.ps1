# MeshLink Windows 卸载脚本 (PowerShell)
param (
    [switch]$purge = $false
)

$ErrorActionPreference = "SilentlyContinue"

Write-Host "[INFO] 正在停止 MeshLink 服务..."
Stop-ScheduledTask -TaskName "MeshLink"
Unregister-ScheduledTask -TaskName "MeshLink" -Confirm:$false
Get-Process "p2p-node" | Stop-Process -Force

$installDir = "C:\Program Files\MeshLink"

if ($purge) {
    Write-Host "[WARN] 正在清除所有配置和密钥..."
    Remove-Item -Path $installDir -Recurse -Force
} else {
    Write-Host "[INFO] 正在清理程序文件 (保留配置)..."
    Remove-Item -Path "$installDir\p2p-node.exe" -Force
    Remove-Item -Path "$installDir\wintun.dll" -Force
    Remove-Item -Path "$installDir\meshlink.ps1" -Force
}

Write-Host "✅ MeshLink 已从 Windows 卸载。" -ForegroundColor Green
