#!/usr/bin/env bash
# MeshLink — macOS desktop portable package

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_BIN="${ROOT_DIR}/dist/bin"
SCRIPTS_DIR="${ROOT_DIR}/scripts"
OUTPUT_DIR="${ROOT_DIR}/dist/desktop/macos"
VERSION="${1:-1.0.0}"
GO_LDFLAGS="${GO_LDFLAGS:--s -w}"

PKG_NAME="meshlink-desktop-macos-universal-${VERSION}"
PKG_DIR="${OUTPUT_DIR}/${PKG_NAME}"
APP_DIR="${PKG_DIR}/MeshLink Desktop.app"
MACOS_DIR="${APP_DIR}/Contents/MacOS"
RES_DIR="${APP_DIR}/Contents/Resources"
BUILD_DIR="${OUTPUT_DIR}/.build-${VERSION}"

mkdir -p "${MACOS_DIR}" "${RES_DIR}" "${BUILD_DIR}"

CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
  go build -buildvcs=false -ldflags="${GO_LDFLAGS} -X 'main.Version=${VERSION}'" \
  -o "${BUILD_DIR}/meshlink-desktop-arm64" "${ROOT_DIR}/cmd/desktop"

CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
  go build -buildvcs=false -ldflags="${GO_LDFLAGS} -X 'main.Version=${VERSION}'" \
  -o "${BUILD_DIR}/meshlink-desktop-amd64" "${ROOT_DIR}/cmd/desktop"

lipo -create \
  "${BUILD_DIR}/meshlink-desktop-arm64" \
  "${BUILD_DIR}/meshlink-desktop-amd64" \
  -output "${MACOS_DIR}/meshlink-desktop"

cp "${DIST_BIN}/p2p-node-darwin-arm64" "${MACOS_DIR}/"
cp "${DIST_BIN}/p2p-node-darwin-amd64" "${MACOS_DIR}/"
cp "${DIST_BIN}/p2p-node-darwin-arm64" "${RES_DIR}/"
cp "${DIST_BIN}/p2p-node-darwin-amd64" "${RES_DIR}/"
cp "${SCRIPTS_DIR}/install-macos.sh" "${RES_DIR}/"
cp "${SCRIPTS_DIR}/uninstall-macos.sh" "${RES_DIR}/"
cp "${SCRIPTS_DIR}/meshlink.sh" "${RES_DIR}/"

cat > "${APP_DIR}/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>zh_CN</string>
  <key>CFBundleExecutable</key>
  <string>meshlink-desktop</string>
  <key>CFBundleIdentifier</key>
  <string>com.meshlink.desktop</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>MeshLink Desktop</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${VERSION}</string>
  <key>CFBundleVersion</key>
  <string>${VERSION}</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.15</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

cat > "${PKG_DIR}/README.txt" <<EOF
MeshLink macOS 桌面便携版
=========================

直接双击 MeshLink Desktop.app 运行。

桌面端会检测系统 MeshLink 服务；未安装时可在桌面内触发安装。
安装后桌面端控制 /usr/local/bin/meshlink，与 install-macos.sh 使用同一套服务和配置。
EOF

TAR_FILE="${OUTPUT_DIR}/${PKG_NAME}.tar.gz"
tar -czf "${TAR_FILE}" -C "${OUTPUT_DIR}" "${PKG_NAME}"
rm -rf "${PKG_DIR}" "${BUILD_DIR}"
echo "✅ macOS desktop package ready: ${TAR_FILE}"
