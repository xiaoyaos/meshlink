#!/usr/bin/env bash
# =============================================================================
# MeshLink — 打包 Linux 发行包脚本
# 生成: dist/packages/meshlink-linux-<version>.tar.gz
# 包含: p2p-node-linux-amd64, p2p-node-linux-arm64, install.sh, uninstall.sh, README
# =============================================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# 脚本所在目录 = meshlink 项目根目录
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_BIN="${ROOT_DIR}/dist/bin"
SCRIPTS_DIR="${ROOT_DIR}/scripts"
OUTPUT_DIR="${ROOT_DIR}/dist/packages"
VERSION="${1:-$(date +%Y%m%d)}"
PKG_NAME="meshlink-linux-${VERSION}"
PKG_DIR="${OUTPUT_DIR}/${PKG_NAME}"

echo ""
echo -e "${BLUE}╔══════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║    MeshLink — 打包 Linux 发行包          ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════╝${NC}"
echo ""

# 检查 CLI 二进制是否存在
for arch in amd64 arm64; do
  bin="${DIST_BIN}/p2p-node-linux-${arch}"
  [[ -f "$bin" ]] || error "找不到 ${bin}，请先运行: make release-cli"
done

# 创建临时目录
info "创建打包目录 ${PKG_DIR} ..."
rm -rf "${PKG_DIR}"
mkdir -p "${PKG_DIR}"

# 复制二进制
info "复制 Linux 二进制 ..."
cp "${DIST_BIN}/p2p-node-linux-amd64" "${PKG_DIR}/"
cp "${DIST_BIN}/p2p-node-linux-arm64" "${PKG_DIR}/"
chmod 755 "${PKG_DIR}"/p2p-node-linux-*

# 复制脚本
info "复制安装/卸载/管理脚本 ..."
cp "${SCRIPTS_DIR}/install.sh"   "${PKG_DIR}/"
cp "${SCRIPTS_DIR}/uninstall.sh" "${PKG_DIR}/"
cp "${SCRIPTS_DIR}/meshlink.sh"  "${PKG_DIR}/"
chmod 755 "${PKG_DIR}"/*.sh

# 生成包内 README
info "生成快速说明 ..."
cat > "${PKG_DIR}/README.txt" <<'READMEOF'
MeshLink P2P Node — Linux 安装包
=================================

【架构说明】
  p2p-node-linux-amd64  →  x86_64 服务器（大多数 VPS）
  p2p-node-linux-arm64  →  ARM64 服务器（树莓派、AWS Graviton 等）

【快速安装（引导/中继节点）】

  # 一键安装，开启中继模式，端口 4001
  sudo bash install.sh --relay

  # 自定义端口
  sudo bash install.sh --relay --port 5001

  # 作为客户端节点（指定引导节点地址）
  sudo bash install.sh --bootstrap "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW..."

【安装后常用命令】
  系统已自动注册全局 meshlink 命令：

  meshlink status          # 查看服务状态
  meshlink logs            # 实时查看日志
  meshlink restart         # 重启服务
  meshlink address         # 查看节点地址
  meshlink start/stop      # 启动/停止服务

  nano /etc/meshlink/meshlink.env    # 修改配置

【卸载】

  sudo bash uninstall.sh             # 卸载服务（保留配置/密钥）
  sudo bash uninstall.sh --purge     # 彻底清除（包括身份密钥）

【防火墙说明】

  # ufw
  sudo ufw allow 4001/tcp
  sudo ufw allow 4001/udp

  # iptables
  iptables -A INPUT -p tcp --dport 4001 -j ACCEPT
  iptables -A INPUT -p udp --dport 4001 -j ACCEPT

【更多信息】
  GitHub: https://github.com/xiaoyaos/meshlink
READMEOF

# 打包
info "打包为 tar.gz ..."
mkdir -p "${OUTPUT_DIR}"
TAR_FILE="${OUTPUT_DIR}/${PKG_NAME}.tar.gz"
tar -czf "${TAR_FILE}" -C "${OUTPUT_DIR}" "${PKG_NAME}"
rm -rf "${PKG_DIR}"

# 完成
SIZE=$(du -sh "${TAR_FILE}" | cut -f1)
success "打包完成！"
echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  📦 输出文件: ${YELLOW}${TAR_FILE}${NC}"
echo -e "  📏 文件大小: ${YELLOW}${SIZE}${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  传输到服务器:"
echo -e "    ${YELLOW}scp ${TAR_FILE} root@your-server:/tmp/${NC}"
echo ""
echo -e "  服务器上安装:"
echo -e "    ${YELLOW}cd /tmp && tar xzf ${PKG_NAME}.tar.gz && cd ${PKG_NAME}${NC}"
echo -e "    ${YELLOW}sudo bash install.sh --relay${NC}"
echo ""
