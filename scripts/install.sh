#!/usr/bin/env bash
# =============================================================================
# MeshLink P2P Node — Linux 安装脚本
# 支持系统: Ubuntu 18.04+ / Debian 10+ / CentOS 7+ / RHEL 8+ / 任意 systemd 发行版
# 用法: sudo bash install.sh [选项]
#   --port PORT          监听端口（默认 4001）
#   --relay              开启中继模式（引导节点推荐开启）
#   --bootstrap ADDR     引导节点 Multiaddr（客户端模式使用）
#   --arch ARCH          架构: amd64 | arm64（默认自动检测）
# =============================================================================

set -euo pipefail

# ── 颜色输出 ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# ── 默认配置 ──────────────────────────────────────────────────────────────────
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/meshlink"
SERVICE_NAME="meshlink"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
BINARY_NAME="p2p-node"

PORT="4001"
RELAY=false
BOOTSTRAP_ADDR=""
ARCH=""

# ── 参数解析 ──────────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --port)       PORT="$2";           shift 2 ;;
    --relay)      RELAY=true;          shift   ;;
    --bootstrap)  BOOTSTRAP_ADDR="$2"; shift 2 ;;
    --arch)       ARCH="$2";           shift 2 ;;
    --help|-h)
      grep '^#' "$0" | grep -E '^\# ' | sed 's/^# //'
      exit 0 ;;
    *) error "未知参数: $1，使用 --help 查看帮助" ;;
  esac
done

# ── 权限检查 ──────────────────────────────────────────────────────────────────
[[ $EUID -ne 0 ]] && error "请使用 root 权限运行: sudo bash install.sh"

# ── 架构检测 ──────────────────────────────────────────────────────────────────
if [[ -z "$ARCH" ]]; then
  MACHINE=$(uname -m)
  case "$MACHINE" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *) error "不支持的架构: $MACHINE，请使用 --arch 手动指定 amd64 或 arm64" ;;
  esac
fi
BINARY_FILE="${BINARY_NAME}-linux-${ARCH}"

# ── 脚本目录（查找二进制）───────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_PATH="${SCRIPT_DIR}/${BINARY_FILE}"

[[ -f "$BINARY_PATH" ]] || error "找不到二进制文件: ${BINARY_PATH}\n请确保 ${BINARY_FILE} 与本脚本在同一目录"

# ── 开始安装 ──────────────────────────────────────────────────────────────────
echo ""
echo -e "${BLUE}╔══════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║      MeshLink P2P Node — 安装程序        ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════╝${NC}"
echo ""
info "架构: ${ARCH}"
info "端口: ${PORT}"
info "中继模式: ${RELAY}"
[[ -n "$BOOTSTRAP_ADDR" ]] && info "引导节点: ${BOOTSTRAP_ADDR}"
echo ""

# 1. 复制二进制和管理脚本
info "安装二进制到 ${INSTALL_DIR}/${BINARY_NAME} ..."
cp -f "$BINARY_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"

info "安装全局命令 ${INSTALL_DIR}/meshlink ..."
cp -f "${SCRIPT_DIR}/meshlink.sh" "${INSTALL_DIR}/meshlink"
chmod 755 "${INSTALL_DIR}/meshlink"
success "二进制和管理命令安装完成"

# 2. 创建配置目录
info "创建配置目录 ${CONFIG_DIR} ..."
mkdir -p "${CONFIG_DIR}"
chmod 700 "${CONFIG_DIR}"
success "配置目录就绪"

# 3. 写入环境配置文件（供 systemd 读取）
ENV_FILE="${CONFIG_DIR}/meshlink.env"
info "写入配置文件 ${ENV_FILE} ..."
cat > "${ENV_FILE}" <<EOF
# MeshLink 节点配置
# 修改后执行: systemctl restart meshlink

# 监听端口
PORT=${PORT}

# 配置目录（存储节点身份密钥）
CONFIG_DIR=${CONFIG_DIR}/data

# 是否开启中继/引导节点模式 (true/false)
RELAY=${RELAY}

# 引导节点 Multiaddr（留空则为引导节点模式）
# 示例: /ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
BOOTSTRAP_ADDR=${BOOTSTRAP_ADDR}
EOF
chmod 600 "${ENV_FILE}"
success "配置文件写入完成"

# 4. 创建数据目录
mkdir -p "${CONFIG_DIR}/data"
chmod 700 "${CONFIG_DIR}/data"

# 5. 构建 ExecStart 命令
EXEC_START="${INSTALL_DIR}/${BINARY_NAME} -port \${PORT} -config \${CONFIG_DIR}"
EXEC_START_EXPANDED="${INSTALL_DIR}/${BINARY_NAME} -port ${PORT} -config ${CONFIG_DIR}/data"
if [[ "$RELAY" == "true" ]]; then
  EXEC_START="${EXEC_START} -relay"
  EXEC_START_EXPANDED="${EXEC_START_EXPANDED} -relay"
fi
if [[ -n "$BOOTSTRAP_ADDR" ]]; then
  EXEC_START="${EXEC_START} -bootstrap \${BOOTSTRAP_ADDR}"
  EXEC_START_EXPANDED="${EXEC_START_EXPANDED} -bootstrap ${BOOTSTRAP_ADDR}"
fi

# 6. 写入 systemd service
info "写入 systemd 服务 ${SERVICE_FILE} ..."
cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=MeshLink P2P Virtual Mesh Network Node
Documentation=https://github.com/$(basename "$(git -C "${SCRIPT_DIR}" remote get-url origin 2>/dev/null || echo 'meshlink/meshlink')" .git 2>/dev/null || echo 'meshlink/meshlink')
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${CONFIG_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${EXEC_START_EXPANDED}
Restart=always
RestartSec=5
TimeoutStopSec=10

# 日志
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

# 安全加固（可选）
NoNewPrivileges=false
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
chmod 644 "${SERVICE_FILE}"
success "systemd 服务配置完成"

# 7. 启用并启动服务
info "重新加载 systemd 配置 ..."
systemctl daemon-reload

# 关键优化：自动搜索并停止可能冲突的旧版服务
info "扫描并清理冲突的旧版服务 ..."
LEGACY_SERVICES=$(grep -lE "p2p-node|meshlink" /etc/systemd/system/*.service 2>/dev/null || true)
for svc_file in $LEGACY_SERVICES; do
  svc_name=$(basename "$svc_file")
  if [[ "$svc_name" != "${SERVICE_NAME}.service" ]]; then
    warn "发现冲突服务: ${svc_name}，正在停止并禁用..."
    systemctl stop "$svc_name" 2>/dev/null || true
    systemctl disable "$svc_name" 2>/dev/null || true
    # 彻底移除旧服务文件，防止它在后台偷偷重启
    # rm -f "$svc_file" # 暂时不删除物理文件，只禁用，更安全
  fi
done

# 停止当前服务
systemctl stop "${SERVICE_NAME}" 2>/dev/null || true

# 强力清理：不仅清理精准匹配的进程，还要通过端口号和模糊名称清理
info "正在执行深层进程清理..."
# 1. 尝试杀掉所有包含 p2p-node 的进程
pkill -9 -f "p2p-node" 2>/dev/null || true

# 2. 检查并杀掉占用端口的进程（终极手段）
# 重复检查 2 次，防止某些守护进程在被杀后瞬间重启
for i in {1..2}; do
  PORT_PIDS=$(ss -tlnp "sport = :${PORT}" | grep -oP '(?<=pid=)\d+' | sort -u || true)
  if [[ -n "$PORT_PIDS" ]]; then
    warn "检测到端口 ${PORT} 被进程 ${PORT_PIDS} 占用，正在强制终止..."
    echo "$PORT_PIDS" | xargs kill -9 2>/dev/null || true
    sleep 1
  fi
done

# 等待端口完全释放，最多等 5 秒
for i in $(seq 1 5); do
  if ! ss -tlnp "sport = :${PORT}" | grep -q ":${PORT} "; then
    break
  fi
  warn "端口 ${PORT} 仍在等待内核释放... (${i}/5)"
  sleep 1
done

info "设置开机自启 ..."
systemctl enable "${SERVICE_NAME}"

info "启动服务 ..."
systemctl start "${SERVICE_NAME}"
sleep 2

# 8. 状态检查
if systemctl is-active --quiet "${SERVICE_NAME}"; then
  success "服务启动成功！"
  echo ""
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}  MeshLink 节点已成功安装并运行！${NC}"
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo -e "  📋 常用命令（随时随地可用）："
  echo -e "     查看状态: ${YELLOW}meshlink status${NC}"
  echo -e "     查看日志: ${YELLOW}meshlink logs${NC}"
  echo -e "     重启服务: ${YELLOW}meshlink restart${NC}"
  echo -e "     修改配置: ${YELLOW}nano ${ENV_FILE}${NC}"
  echo -e "     卸载节点: ${YELLOW}sudo bash uninstall.sh${NC}"
  echo ""
  echo -e "  📡 节点地址（稍等几秒后输入以下命令获取）："
  echo -e "     ${YELLOW}meshlink address${NC}"
  echo ""
else
  warn "服务启动可能失败，请检查日志："
  echo "  journalctl -u ${SERVICE_NAME} -n 30 --no-pager"
  exit 1
fi
