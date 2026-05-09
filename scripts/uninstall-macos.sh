#!/usr/bin/env bash
# MeshLink macOS 卸载脚本

[[ $EUID -ne 0 ]] && echo "请使用 sudo 运行" && exit 1

PLIST_PATH="/Library/LaunchDaemons/com.meshlink.p2p.plist"
BINARY_NAME="p2p-node"

echo "[INFO] 正在停止并卸载服务 ..."
launchctl unload -w "${PLIST_PATH}" 2>/dev/null || true
rm -f "${PLIST_PATH}"

echo "[INFO] 正在清理二进制文件 ..."
rm -f "/usr/local/bin/${BINARY_NAME}"
rm -f "/usr/local/bin/meshlink"

echo "[INFO] 正在彻底清除所有配置、身份密钥和日志痕迹 ..."
rm -rf "/etc/meshlink"
rm -f "/var/log/meshlink.log"

echo "[OK] MeshLink 已彻底从 macOS 卸载，未残留任何痕迹。"
