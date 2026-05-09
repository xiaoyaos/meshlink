#!/usr/bin/env bash
# =============================================================================
# MeshLink P2P Node — Linux 卸载脚本
# 用法: sudo bash uninstall.sh [--purge]
#   --purge    同时删除配置目录和节点身份密钥（不可恢复！）
# =============================================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/meshlink"
SERVICE_NAME="meshlink"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
BINARY_NAME="p2p-node"
PURGE=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --purge) PURGE=true; shift ;;
    --help|-h)
      grep '^#' "$0" | grep -E '^\# ' | sed 's/^# //'
      exit 0 ;;
    *) error "未知参数: $1" ;;
  esac
done

[[ $EUID -ne 0 ]] && error "请使用 root 权限运行: sudo bash uninstall.sh"

echo ""
echo -e "${RED}╔══════════════════════════════════════════╗${NC}"
echo -e "${RED}║      MeshLink P2P Node — 卸载程序        ║${NC}"
echo -e "${RED}╚══════════════════════════════════════════╝${NC}"
echo ""

if [[ "$PURGE" == "true" ]]; then
  warn "⚠️  --purge 模式：将同时删除配置目录和节点身份密钥！"
  read -r -p "确认删除所有数据? [y/N] " confirm
  [[ "$confirm" =~ ^[Yy]$ ]] || { info "取消卸载。"; exit 0; }
fi

# 1. 停止并禁用服务
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
  info "停止服务 ${SERVICE_NAME} ..."
  systemctl stop "${SERVICE_NAME}"
  success "服务已停止"
fi

# 额外的安全措施：清理可能残留的进程（如手动启动或 GUI 留下的）
if pgrep -x "${BINARY_NAME}" > /dev/null; then
  info "检测到残留的核心进程，正在清理..."
  pkill -9 -x "${BINARY_NAME}" || true
  success "残留进程已清理"
fi

if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
  info "禁用开机自启 ..."
  systemctl disable "${SERVICE_NAME}"
  success "开机自启已禁用"
fi

# 2. 删除 service 文件
if [[ -f "${SERVICE_FILE}" ]]; then
  info "删除 systemd 服务文件 ..."
  rm -f "${SERVICE_FILE}"
  systemctl daemon-reload
  systemctl reset-failed 2>/dev/null || true
  success "systemd 服务已清除"
fi

# 3. 删除二进制和管理脚本
if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
  info "删除二进制 ${INSTALL_DIR}/${BINARY_NAME} ..."
  rm -f "${INSTALL_DIR}/${BINARY_NAME}"
  success "二进制已删除"
fi

if [[ -f "${INSTALL_DIR}/meshlink" ]]; then
  info "删除管理脚本 ${INSTALL_DIR}/meshlink ..."
  rm -f "${INSTALL_DIR}/meshlink"
  success "管理脚本已删除"
fi

# 4. 彻底清理配置目录和日志
info "正在清除所有配置目录 ${CONFIG_DIR} ..."
rm -rf "${CONFIG_DIR}"
rm -f "/var/log/meshlink.log" 2>/dev/null || true
success "所有配置、密钥及日志痕迹已清除"

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  MeshLink 节点已彻底卸载！${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
