# P2P 虚拟网状网络 (P2P Virtual Mesh Network)

基于 Go 语言和 libp2p 构建的跨地域、跨内网纯点对点虚拟局域网。无需公网中转服务器，客户端加入网络后即可获得唯一的 `10.x.x.x` 虚拟 IP 并直接互访。

---

## 🏗 架构说明

```
[内网 Mac 客户端]  <──── 打洞/中继 ────>  [内网 Windows 客户端]
         │                                        │
         └──────────────── DHT 发现 ──────────────┘
                                │
                   [公网引导/中继节点（任意平台）]
```

- **引导节点**（`p2p-node`）：运行在具有公网 IP 的服务器上，帮助客户端互相发现。支持 Linux / macOS / Windows，**任意平台均可作为引导节点，客户端行为完全一致**。
- **客户端**（`p2p-gui`）：Mac 或 Windows 桌面端，双击运行，自动获得虚拟 IP。

---

## 🚀 快速开始

### 第一步：在公网服务器部署引导节点

下载对应平台的 `p2p-node` 二进制，在服务器上执行：

```bash
# Linux（推荐）
sudo ./p2p-node-linux-amd64 -port 4001 -relay -config ./server_config

# macOS
sudo ./p2p-node-darwin-arm64 -port 4001 -relay -config ./server_config

# Windows（管理员命令提示符）
p2p-node-windows-amd64.exe -port 4001 -relay -config .\server_config
```

启动后，记录日志中输出的 **Multiaddr 地址**，格式如：
```
/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
```

### 第二步：客户端加入网络

#### macOS 客户端
1. 双击打开 `p2p-gui-macos.app`
2. 系统会弹出**密码框**，输入开机密码授予管理员权限
3. 在 "引导节点地址" 输入框中填入第一步的 Multiaddr 地址
4. 点击 **启动网络**，等待状态变为 **已连接**
5. 您的虚拟 IP（`10.x.x.x`）会显示在界面上

#### Windows 客户端
1. 运行 `p2p-gui-windows-setup.exe` 进行安装（**wintun 驱动已内置，无需单独安装**）
2. 程序启动时会弹出 **UAC 管理员权限请求**，点击"是"
3. 在 "引导节点地址" 输入框中填入第一步的 Multiaddr 地址
4. 点击 **启动网络**，等待状态变为 **已连接**

> 两台不同内网的客户端，连接同一个引导节点后，即可通过 `ping 10.x.x.x` 互相通信。

---

## 💻 平台支持

| 组件 | macOS | Linux | Windows |
|------|-------|-------|---------|
| 引导/中继节点（`p2p-node`）| ✅ arm64 / amd64 | ✅ amd64 / arm64 | ✅ amd64 |
| 桌面客户端（`p2p-gui`）| ✅ Universal | ❌（使用 CLI） | ✅ amd64（含 Wintun 驱动）|

### Windows 特别说明
- **Wintun 驱动已内置**：无需访问 wintun.net 下载，程序首次运行时会自动释放驱动文件。
- **UAC 自动弹出**：程序检测到非管理员权限时，会自动触发 Windows 的 UAC 弹窗，点击"是"即可，无需手动"以管理员身份运行"。
- **系统要求**：Windows 10 1903+ / Windows 11。

### macOS 特别说明
- 启动时会弹出系统密码框，输入开机密码即可。
- 程序会自动添加 `10.0.0.0/8` 路由到虚拟网卡。
- 配置文件存储在 `~/Library/Application Support/P2PMesh/`。

---

## 🛠 进阶：Linux 服务器一键部署

如果你需要在公网 Linux 服务器上长期运行**引导节点**，我们提供了一键安装和打包脚本。

### 1. 获取安装包

你可以从 [Releases](../../releases) 页面下载 `meshlink-linux-<version>.tar.gz`，或者自己编译打包：

```bash
# 在含有 Makefile 的源码目录运行
make package-linux VERSION=1.0.0
```
产物在 `dist/meshlink-linux-1.0.0.tar.gz`。

### 2. 上传并解压

```bash
# 传到服务器（将 1.2.3.4 替换为你的服务器 IP）
scp dist/meshlink-linux-1.0.0.tar.gz root@1.2.3.4:/tmp/

# 在服务器上解压
cd /tmp && tar xzf meshlink-linux-1.0.0.tar.gz && cd meshlink-linux-1.0.0
```

### 3. 一键安装并运行

```bash
# 一键安装，开启中继模式，端口 4001
sudo bash install.sh --relay

# 或者自定义端口
sudo bash install.sh --relay --port 5001
```

安装脚本会自动将节点注册为 `systemd` 服务，并设置开机自启。

### 4. 获取节点地址

节点启动后，它的 `Multiaddr`（供客户端连接时填入）会自动保存在配置文件中：

```bash
cat /etc/meshlink/data/address.txt
```
输出示例：
```text
Virtual IP: 10.1.179.163

Multiaddr:
/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
/ip4/127.0.0.1/tcp/4001/p2p/12D3KooW...
```

### 常用管理命令

安装成功后，系统已自动注册全局 `meshlink` 命令，随时随地可用：

```bash
meshlink status          # 查看服务运行状态
meshlink logs            # 实时查看节点日志
meshlink restart         # 重启节点服务
meshlink address         # 直接打印本节点的 Multiaddr 地址
meshlink start           # 启动服务
meshlink stop            # 停止服务

nano /etc/meshlink/meshlink.env    # 修改端口或配置参数（修改后需 restart）
```

### 卸载节点

```bash
sudo bash uninstall.sh             # 卸载服务（保留配置和密钥）
sudo bash uninstall.sh --purge     # 彻底清除（包含身份密钥，不可恢复）
```

---

## 🔍 常见问题排查

### Q1: 连接成功但 Ping 不通对方？
- **防火墙**：确保两端没有拦截 ICMP 协议。如果是自建的 Linux 节点，确保开放了对应的 TCP/UDP 端口（如：`sudo ufw allow 4001`）。
- **路由冲突**：确保物理网段不是 `10.x.x.x`。
- **DHT 延迟**：首次连接后，DHT 寻址可能需要 10-30 秒，请稍等。

### Q2: 报错 "failed to find any peer in table"？
- 正常现象，说明当前只有您一个节点。待其他节点加入后自动消失。

### Q3: Windows 弹出 UAC 后立即关闭？
- 确保点击了"是"而非"否"。如果多次弹出，说明程序在循环提权——可以尝试右键 → 以管理员身份运行。

### Q4: 端口冲突？
- 使用 `-port` 参数更改，例如 `-port 5001`，并确保防火墙开放对应端口的 TCP 和 UDP 流量。

---

## 📦 构建说明（开发者）

```bash
# 编译全平台 CLI 引导节点（可在 macOS 上交叉编译）
make release-cli

# 编译 macOS GUI（在 macOS 上运行）
make release-gui

# 编译 Windows GUI（通过 Docker 交叉编译，需先运行 make docker-builder）
make release-gui-windows

# 打包 Linux 服务端发行包
make package-linux VERSION=1.0.0

# 一键编译所有产物
make dist
```

产物位置：`release/` 和 `dist/` 目录。
