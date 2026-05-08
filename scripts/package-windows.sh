#!/usr/bin/env bash
# MeshLink — Windows 打包脚本

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_BIN="${ROOT_DIR}/dist/bin"
SCRIPTS_DIR="${ROOT_DIR}/scripts"
OUTPUT_DIR="${ROOT_DIR}/dist/packages"
VERSION="${1:-1.0.0}"
PKG_NAME="meshlink-windows-${VERSION}"
PKG_DIR="${OUTPUT_DIR}/${PKG_NAME}"

mkdir -p "${PKG_DIR}"
cp "${DIST_BIN}/p2p-node-windows-amd64.exe" "${PKG_DIR}/"
cp "${ROOT_DIR}/pkg/tun/wintun.dll" "${PKG_DIR}/"
cp "${SCRIPTS_DIR}/install-windows.ps1" "${PKG_DIR}/install.ps1"
cp "${SCRIPTS_DIR}/uninstall-windows.ps1" "${PKG_DIR}/uninstall.ps1"
cp "${SCRIPTS_DIR}/meshlink.ps1" "${PKG_DIR}/"

cat > "${PKG_DIR}/README.txt" <<EOF
MeshLink Windows 安装包
=======================

【安装说明】
  1. 右键以管理员权限运行 PowerShell。
  2. 进入此目录。
  3. 执行安装脚本:
     .\install.ps1 -bootstrap "/ip4/.../p2p/..."

【管理命令】
  在管理员 PowerShell 中进入 C:\Program Files\MeshLink:
  .\meshlink.ps1 status
  .\meshlink.ps1 stop
  .\meshlink.ps1 start
EOF

ZIP_FILE="${OUTPUT_DIR}/${PKG_NAME}.zip"
cd "${OUTPUT_DIR}" && zip -r "${PKG_NAME}.zip" "${PKG_NAME}"
rm -rf "${PKG_DIR}"
echo "✅ Windows package ready: ${ZIP_FILE}"
