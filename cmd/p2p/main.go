package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"p2p/pkg/bridge"
	"p2p/pkg/identity"
	"p2p/pkg/p2p"
	"p2p/pkg/tun"
)

func main() {
	port := flag.String("port", "4001", "libp2p listen port")
	configDir := flag.String("config", "./config", "config directory")
	bootstrapAddr := flag.String("bootstrap", "", "bootstrap node multiaddr")
	enableRelay := flag.Bool("relay", false, "enable libp2p relay service")
	logFile := flag.String("logfile", "", "write logs to file")
	flag.Parse()

	// 如果指定了日志文件，则重定向标准输出和错误输出
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("无法打开日志文件: %v", err)
		}
		// 重定向全局 log 和 fmt 使用的输出
		os.Stdout = f
		os.Stderr = f
		log.SetOutput(f)
		
		// 也要确保后续的 fmt.Printf 能够看到变化
		// 注意：fmt 内部直接引用 os.Stdout 变量
	}

	// 1. 加载身份
	priv, err := identity.LoadOrGenerateKey(*configDir)
	if err != nil {
		log.Fatalf("failed to load identity: %v", err)
	}

	id, virtualIP, err := identity.GetPeerInfo(priv)
	if err != nil {
		log.Fatalf("failed to get peer info: %v", err)
	}

	fmt.Printf("[系统信息] 您的设备专属 ID (PeerID): %s\n", id)
	fmt.Printf("[系统信息] 您的 MeshLink 虚拟局域网 IP: %s\n", virtualIP)

	// 2. 创建 P2P 节点
	node, err := p2p.NewNode(priv, *port, *enableRelay)
	if err != nil {
		log.Fatalf("[致命错误] P2P 核心网络启动失败，请检查端口占用: %v", err)
	}
	defer node.Close()

	fmt.Printf("\n[服务状态] P2P 核心已启动，正在监听以下本机端口：\n")
	var addrList []string
	for _, addr := range node.Host.Addrs() {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, node.Host.ID())
		fmt.Printf("  %s\n", fullAddr)
		addrList = append(addrList, fullAddr)
	}

	// 将地址和虚拟IP写入文件方便查看（无需翻日志）
	addrFile := filepath.Join(*configDir, "address.txt")
	fileContent := fmt.Sprintf("Virtual IP: %s\n\nMultiaddr:\n%s\n", virtualIP, strings.Join(addrList, "\n"))
	if err := os.WriteFile(addrFile, []byte(fileContent), 0644); err == nil {
		fmt.Printf("[系统信息] 本机网络配置已备份至: %s\n\n", addrFile)
	}

	// 3. 连接引导节点
	if *bootstrapAddr != "" {
		if err := node.Bootstrap([]string{*bootstrapAddr}); err != nil {
			fmt.Printf("[致命错误] 引导流程中断，无法接入网络: %v\n", err)
			os.Exit(1)
		} else {
			fmt.Println("[网络状态] 引导流程完成。")
		}
	}

	// 4. 创建 TUN 网卡
	itf, err := tun.New(virtualIP)
	if err != nil {
		log.Fatalf("[致命错误] 无法创建虚拟网卡，权限不足。请在 Linux/macOS 使用 sudo 运行，或在 Windows 使用管理员权限: %v", err)
	}
	defer itf.Close()
	fmt.Printf("[网络状态] 成功创建虚拟网卡 (名称: %s, IP: %s)\n", itf.Name, virtualIP)

	// 5. 启动网桥
	br := bridge.New(itf, node)
	br.Start()

	fmt.Println("🟢 [运行状态] 一切就绪！您的跨地域虚拟局域网已打通。您可以开始 ping 对方的虚拟 IP 了。(按 Ctrl+C 安全退出)")

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n🔴 [运行状态] 收到退出指令，正在安全关闭网络层和虚拟网卡...")
}
