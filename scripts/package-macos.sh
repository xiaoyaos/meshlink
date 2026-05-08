#!/usr/bin/env bash
# MeshLink — macOS 打包脚本

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_BIN="${ROOT_DIR}/dist/bin"
SCRIPTS_DIR="${ROOT_DIR}/scripts"
OUTPUT_DIR="${ROOT_DIR}/dist/packages"
VERSION="${1:-1.0.0}"
PKG_NAME="meshlink-macos-${VERSION}"
PKG_DIR="${OUTPUT_DIR}/${PKG_NAME}"

mkdir -p "${PKG_DIR}"
cp "${DIST_BIN}/p2p-node-darwin-arm64" "${PKG_DIR}/"
cp "${DIST_BIN}/p2p-node-darwin-amd64" "${PKG_DIR}/"
cp "${SCRIPTS_DIR}/install-macos.sh" "${PKG_DIR}/install.sh"
cp "${SCRIPTS_DIR}/uninstall-macos.sh" "${PKG_DIR}/uninstall.sh"
cp "${SCRIPTS_DIR}/meshlink.sh" "${PKG_DIR}/"

cat > "${PKG_DIR}/README.txt" <<EOF
MeshLink macOS 安装包
=====================

【快速安装】
  sudo bash install.sh --bootstrap "/ip4/.../p2p/..."

【管理命令】
  meshlink status
  meshlink logs
  meshlink address
  meshlink stop/start
EOF

TAR_FILE="${OUTPUT_DIR}/${PKG_NAME}.tar.gz"
tar -czf "${TAR_FILE}" -C "${OUTPUT_DIR}" "${PKG_NAME}"
rm -rf "${PKG_DIR}"
echo "✅ macOS package ready: ${TAR_FILE}"
