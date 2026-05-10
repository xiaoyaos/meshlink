#!/usr/bin/env bash
# MeshLink — Windows desktop portable package

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_BIN="${ROOT_DIR}/dist/bin"
SCRIPTS_DIR="${ROOT_DIR}/scripts"
OUTPUT_DIR="${ROOT_DIR}/dist/desktop/windows"
VERSION="${1:-1.0.0}"
GO_LDFLAGS="${GO_LDFLAGS:--s -w}"
PKG_NAME="meshlink-desktop-windows-amd64-${VERSION}"
PKG_DIR="${OUTPUT_DIR}/${PKG_NAME}"

if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
  cat >&2 <<EOF
Missing Windows CGO toolchain: x86_64-w64-mingw32-gcc

Install MinGW-w64 before building the Windows desktop package.
macOS example:
  brew install mingw-w64

Linux example:
  sudo apt-get install gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64
EOF
  exit 1
fi

mkdir -p "${PKG_DIR}"

CC=x86_64-w64-mingw32-gcc \
CXX=x86_64-w64-mingw32-g++ \
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
  go build -buildvcs=false -ldflags="${GO_LDFLAGS} -H windowsgui -linkmode external -extldflags '-static' -X 'main.Version=${VERSION}'" \
  -o "${PKG_DIR}/meshlink-desktop.exe" "${ROOT_DIR}/cmd/desktop"

cp "${DIST_BIN}/p2p-node-windows-amd64.exe" "${PKG_DIR}/"
cp "${ROOT_DIR}/pkg/tun/wintun.dll" "${PKG_DIR}/"
cp "${SCRIPTS_DIR}/install-windows.ps1" "${PKG_DIR}/install.ps1"
cp "${SCRIPTS_DIR}/uninstall-windows.ps1" "${PKG_DIR}/uninstall.ps1"
cp "${SCRIPTS_DIR}/meshlink.ps1" "${PKG_DIR}/"
cp "${SCRIPTS_DIR}/meshlink.cmd" "${PKG_DIR}/"

cat > "${PKG_DIR}/README.txt" <<EOF
MeshLink Windows 桌面便携版
===========================

直接双击 meshlink-desktop.exe 运行，无需安装。

桌面端会检测系统 MeshLink 服务；未安装时可在桌面内触发安装。
安装后桌面端控制 C:\Program Files\MeshLink，与 install.ps1 使用同一套计划任务和配置。
EOF

ZIP_FILE="${OUTPUT_DIR}/${PKG_NAME}.zip"
cd "${OUTPUT_DIR}" && zip -r "${PKG_NAME}.zip" "${PKG_NAME}"
rm -rf "${PKG_DIR}"
echo "✅ Windows desktop package ready: ${ZIP_FILE}"
