# MeshLink P2P 虚拟网状网络

MeshLink 是基于 Go 和 libp2p 的跨平台虚拟局域网。每个节点会获得一个稳定的 `10.x.x.x` 虚拟 IP；公网节点负责节点发现、DHT 路由及 NAT 打洞辅助，数据传输优先通过 P2P 直连，并支持中继回退。

## 架构

```text
  [ 用户应用 / 操作系统 ]            [ 用户应用 / 操作系统 ]
          |                              |
  [ TUN 网卡 (MTU 1380) ]        [ TUN 网卡 (MTU 1380) ]
          |                              |
  [ 网桥 (Worker Pool 并发控制) ]  [ 网桥 (Worker Pool 并发控制) ]
          |                              |
          +-------- 持久化长连接流 (Length-Prefixed Framing) --------+
          |           (优先直连打洞，自动回退到 Relay 中继)            |
          v                                                      v
  [ libp2p 节点 ] <------- DHT / 引导 / 打洞控制 -------> [ libp2p 节点 ]
                                 |
                        [ 公网引导节点 (Relay/DHT) ]
```

- **TUN 接口**：统一 MTU 为 1380，预留封装空间，防止 IP 分片。
- **网桥层**：引入工作线程池（Worker Pool）处理 TUN 数据包，防止高负载下协程爆炸。
- **数据面**：采用**持久化长连接流**并使用 4 字节长度前缀进行帧包装，极大提升吞吐量。
- **连接策略**：优先尝试打洞建立直连隧道（Direct）；若 NAT 打洞失败，将自动降级通过 Relay 中继通信，确保连接不中断。

---

## 快速启动

所有平台均已统一为纯命令行（CLI）管理模式，支持后台静默运行。

### 1. 编译发行包

在本地开发机执行：
```bash
make dist VERSION=1.0.3
```
编译完成后，产物位于 `dist/packages/`：
- `meshlink-linux-1.0.3.tar.gz`
- `meshlink-macos-1.0.3.tar.gz`
- `meshlink-windows-1.0.3.zip`

### 2. 公网引导节点 (Linux)

将 Linux 包上传至服务器并执行安装（无参数启动将进入**交互式向导**）：
```bash
sudo bash install.sh
```
*向导会引导您选择“引导节点”模式。安装后运行 `meshlink address` 获取您的地址。*

### 3. 客户端安装 (macOS / Windows)

客户端同样支持**交互式安装**，您只需准备好引导节点的地址。

**地址格式支持：**
- **简写格式 (推荐)**: `服务器IP:4001:12D3KooW...`
- **标准格式**: `/ip4/服务器IP/tcp/4001/p2p/12D3KooW...`

#### macOS
1. 下载并解压 `meshlink-macos-1.0.3.tar.gz`。
2. 执行交互式安装：
   ```bash
   sudo bash install.sh
   ```

#### Windows
1. 下载并解压 `meshlink-windows-1.0.3.zip`。
2. **以管理员身份**打开 PowerShell 并进入解压目录。
3. 执行交互式安装：
   ```powershell
   .\install.ps1
   ```

---

## 常用管理命令

安装后，您可以使用统一的 `meshlink` 命令管理服务：

| 命令 | Linux / macOS | Windows (PowerShell) |
| --- | --- | --- |
| **查看状态** | `meshlink status` | `.\meshlink.ps1 status` |
| **查看地址** | `meshlink address` | `.\meshlink.ps1 address` |
| **实时日志** | `meshlink logs` | (查看 `p2p-node` 进程输出) |
| **重启服务** | `meshlink restart` | `.\meshlink.ps1 restart` |
| **停止服务** | `meshlink stop` | `.\meshlink.ps1 stop` |

---

## 排查

- **连接失败**：检查服务器端口 4001 (TCP/UDP) 是否开放。
- **权限问题**：Windows 下必须使用“管理员模式”终端；macOS 下必须加 `sudo`。
- **冲突清理**：若重装失败，请先运行 `uninstall` 脚本。
- **Windows 驱动**：确保安装包内的 `wintun.dll` 与程序在同一目录。
