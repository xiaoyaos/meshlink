#!/usr/bin/env bash
# =============================================================================
# MeshLink P2P Node — macOS 安装脚本
# 用法: sudo bash install-macos.sh [选项]
#   --port PORT          监听端口（默认 4001）
#   --relay              开启中继模式
#   --bootstrap ADDR     引导节点 Multiaddr
# =============================================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/meshlink"
PLIST_PATH="/Library/LaunchDaemons/com.meshlink.p2p.plist"
BINARY_NAME="p2p-node"

PORT="4001"
RELAY=false
BOOTSTRAP_ADDR=""

# ── 参数解析 ──────────────────────────────────────────────────────────────────
INTERACTIVE=false
[[ $# -eq 0 ]] && INTERACTIVE=true

while [[ $# -gt 0 ]]; do
  case "$1" in
    --port)       PORT="$2";           shift 2 ;;
    --relay)      RELAY=true;          shift   ;;
    --bootstrap)  BOOTSTRAP_ADDR="$2"; shift 2 ;;
    *) error "未知参数: $1" ;;
  esac
done

# ── 交互式向导 ────────────────────────────────────────────────────────────────
if [[ "$INTERACTIVE" == "true" ]]; then
  echo -e "${BLUE}╔══════════════════════════════════════════╗${NC}"
  echo -e "${BLUE}║      MeshLink macOS 交互式安装向导       ║${NC}"
  echo -e "${BLUE}╚══════════════════════════════════════════╝${NC}"
  echo ""
  
  echo -e "请选择节点类型:"
  echo -e "  1) ${YELLOW}引导/中继节点${NC} (通常不推荐在个人 Mac 上开启)"
  echo -e "  2) ${YELLOW}普通客户端${NC} (加入现有网络)"
  read -p "选择 [1-2]: " NODE_TYPE_CHOICE
  
  if [[ "$NODE_TYPE_CHOICE" == "1" ]]; then
    RELAY=true
  else
    RELAY=false
    echo ""
    echo -e "请输入引导节点地址 (格式 IP:Port:PeerID 或标准 Multiaddr):"
    read -p "> " BOOTSTRAP_ADDR
  fi
  echo ""
fi

if [[ "$RELAY" != "true" && -z "$BOOTSTRAP_ADDR" ]]; then
  error "客户端模式必须指定引导节点: --bootstrap ADDR；如果本机是公网引导节点，请使用 --relay"
fi

[[ $EUID -ne 0 ]] && error "请使用 sudo 运行"

# 架构检测
ARCH=$(uname -m)
case "$ARCH" in
  arm64) ARCH="arm64" ;;
  x86_64) ARCH="amd64" ;;
  *) error "不支持的架构: $ARCH" ;;
esac
BINARY_FILE="${BINARY_NAME}-darwin-${ARCH}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_PATH="${SCRIPT_DIR}/${BINARY_FILE}"

[[ -f "$BINARY_PATH" ]] || error "找不到二进制: ${BINARY_PATH}"

# 1. 安装文件
info "正在安装 MeshLink 到 ${INSTALL_DIR} ..."
mkdir -p "${INSTALL_DIR}"
cp -f "$BINARY_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"

cp -f "${SCRIPT_DIR}/meshlink.sh" "${INSTALL_DIR}/meshlink"
chmod 755 "${INSTALL_DIR}/meshlink"

# 2. 配置目录
mkdir -p "${CONFIG_DIR}/data"
chmod 755 "${CONFIG_DIR}"
chmod 755 "${CONFIG_DIR}/data"

# 3. 环境变量
ENV_FILE="${CONFIG_DIR}/meshlink.env"
cat > "${ENV_FILE}" <<EOF
PORT=${PORT}
CONFIG_DIR=${CONFIG_DIR}/data
RELAY=${RELAY}
BOOTSTRAP_ADDR=${BOOTSTRAP_ADDR}
EOF
chmod 644 "${ENV_FILE}"

# 4. 生成 LaunchDaemon Plist
info "正在创建 LaunchDaemon 服务 ..."
EXEC_CMD="${INSTALL_DIR}/${BINARY_NAME} -port ${PORT} -config ${CONFIG_DIR}/data"
[[ "$RELAY" == "true" ]] && EXEC_CMD="${EXEC_CMD} -relay"
[[ -n "$BOOTSTRAP_ADDR" ]] && EXEC_CMD="${EXEC_CMD} -bootstrap ${BOOTSTRAP_ADDR}"

cat > "${PLIST_PATH}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.meshlink.p2p</string>
    <key>ProgramArguments</key>
    <array>
        $(echo $EXEC_CMD | sed 's/ /<\/string><string>/g' | sed 's/^/<string>/' | sed 's/$/<\/string>/')
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/meshlink.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/meshlink.log</string>
    <key>WorkingDirectory</key>
    <string>${CONFIG_DIR}</string>
</dict>
</plist>
EOF

# 5. 启动服务
info "正在启动服务 ..."
launchctl unload "${PLIST_PATH}" 2>/dev/null || true
launchctl load -w "${PLIST_PATH}"

success "MeshLink 已在 macOS 上成功安装并运行！"
echo -e "使用 ${YELLOW}meshlink stats${NC} 查看实时状态"
echo -e "使用 ${YELLOW}meshlink --help${NC} 查看帮助手册"
