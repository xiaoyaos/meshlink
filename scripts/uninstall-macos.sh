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

if [[ "$1" == "--purge" ]]; then
    echo "[WARN] 正在彻底清除配置和身份密钥 ..."
    rm -rf "/etc/meshlink"
    rm -f "/var/log/meshlink.log"
else
    echo "[INFO] 已保留配置文件和密钥。若要彻底删除，请运行: sudo bash uninstall-macos.sh --purge"
fi

echo "[OK] MeshLink 已成功从 macOS 卸载。"
