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
- **数据面**：采用**持久化长连接流**代替“每包一流”，并使用 4 字节长度前缀进行帧包装，极大提升吞吐量。
- **连接策略**：优先尝试打洞建立直连隧道（Direct）；若 NAT 打洞失败，将自动降级通过 Relay 中继通信，确保连接不中断。

## 需要更新哪些端

这次架构重构（持久化流与帧包装）引入了破坏性协议变更：

- **所有节点必须同步更新**：旧版本（每包一流）无法与新版本（持久化流）互通。
- **引导节点**：建议同步更新，以支持最新的连接协商逻辑。


## 为什么客户端也要监听端口

客户端监听端口是 P2P 直连的必要条件，不代表让公网服务器中继业务流量。

- 另一个客户端要连进来时，本机必须有 socket 在等待连接。
- NAT 打洞需要稳定的本地 TCP/UDP 端口，双方通过公网节点交换观察到的地址后同时尝试连接。
- libp2p 需要把本机可用地址发布到 DHT，其他客户端才能发现并尝试直连。

默认监听：

```text
/ip4/0.0.0.0/tcp/<port>
/ip4/0.0.0.0/udp/<port>/quic-v1
```

同一台机器运行多个客户端时端口不能重复；不同机器可以使用相同端口。公网服务器需要开放对应 TCP/UDP 端口，普通客户端通常不需要手工做端口映射，但本机防火墙不能阻止程序监听和出站连接。

## 快速启动

### 公网引导节点

```bash
sudo ./p2p-node-linux-amd64 -port 4001 -relay -config ./server_config
```

Windows 管理员命令提示符：

```powershell
p2p-node-windows-amd64.exe -port 4001 -relay -config .\server_config
```

启动后记录 `address.txt` 或日志中的公网 Multiaddr：

```text
/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
```

### 客户端

CLI：

```bash
sudo ./p2p-node-linux-amd64 -port 4002 -config ./client_config -bootstrap "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW..."
```

桌面端：

1. 启动 macOS 或 Windows GUI。
2. 填入公网引导节点 Multiaddr。
3. 点击启动网络。
4. 使用界面显示的 `10.x.x.x` 虚拟 IP 互访。

## 日志判断

客户端正常应看到：

```text
[dht] mode=auto
[relay] candidates=1 usage=hole-punch-control
[advertise] ready ip=... direct_addrs=... relay_addrs=...
[route] candidate ip=... direct_addrs=... relay_addrs=...
[tunnel] hole punch start ...
[tunnel] direct ready ...
[route] direct ready ...
```

公网引导节点正常应看到：

```text
[dht] mode=server
[relay] service enabled
```

如果客户端长期 `relay_addrs=0`，说明还没有拿到可用于打洞的 relay 地址；如果有 `relay_addrs>0` 但最终报 `hole punch did not produce a direct tunnel`，说明当前 NAT 组合没有打通 direct 连接。按当前策略，业务流量不会降级走公网节点中继。

## Linux 服务端安装包

生成生产包：

```bash
make package-linux VERSION=1.0.0
```

产物：

```text
dist/packages/meshlink-linux-1.0.0.tar.gz
```

服务器安装：

```bash
scp dist/packages/meshlink-linux-1.0.0.tar.gz root@1.2.3.4:/tmp/
ssh root@1.2.3.4
cd /tmp
tar xzf meshlink-linux-1.0.0.tar.gz
cd meshlink-linux-1.0.0
sudo bash install.sh --relay --port 4001
```

常用命令：

```bash
meshlink status
meshlink logs
meshlink restart
meshlink address
meshlink stop
```

## 生产构建目录规范

生产产物统一输出到根目录 `dist/`，不要放在 `cmd/`、`pkg/` 或源码目录中。

```text
dist/
  bin/          # p2p-node 跨平台 CLI 二进制
  apps/         # 桌面应用或便携目录
  packages/     # 可发布安装包、tar.gz、zip
```

标准构建命令：

```bash
make release-cli              # 生成 dist/bin/*
make release-gui              # macOS 本机构建 dist/apps/p2p-gui-macos.app
make docker-builder           # 首次构建 Windows Docker builder
make release-gui-windows      # 生成 dist/apps/windows-amd64/
make package-linux VERSION=1.0.0
make verify
```

完整构建：

```bash
make dist VERSION=1.0.0
```

说明：

- `dist/bin/p2p-node-*` 是服务端和桌面端内置后台程序的标准来源。
- macOS `.app` 内会复制对应架构的 `p2p-node-darwin-*`。
- Windows 便携目录会包含 `p2p-gui-windows-amd64.exe`、`p2p-node-windows-amd64.exe` 和 `wintun.dll`。
- `release/` 已不再作为生产输出目录。

## 平台说明

| 组件 | macOS | Linux | Windows |
| --- | --- | --- | --- |
| `p2p-node` CLI | arm64 / amd64 | amd64 / arm64 | amd64 |
| 桌面 GUI | Universal | 使用 CLI | amd64 |

Windows：

- Wintun 驱动已嵌入 CLI，运行时释放到可执行文件同级目录。
- 需要管理员权限创建虚拟网卡。

macOS：

- 需要管理员权限创建 TUN 和添加路由。
- 配置默认存储在 `~/Library/Application Support/P2PMesh/`。

## 排查

- 连接公网节点正常但客户端不通：看客户端是否出现 `[tunnel] direct ready`。
- DHT 找不到对端：确认两个客户端填的是同一个公网节点 Multiaddr，并等待首次 `[advertise] ready`。
- 端口冲突：换 `-port`，同机多个实例不能共用端口。
- 物理网段冲突：避免本地真实网络也使用 `10.0.0.0/8`。
