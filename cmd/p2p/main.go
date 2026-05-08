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
	"time"

	"p2p/pkg/bridge"
	"p2p/pkg/identity"
	"p2p/pkg/p2p"
	"p2p/pkg/tun"
	"p2p/pkg/utils"
)

func main() {
	port := flag.String("port", "4001", "libp2p listen port")
	configDir := flag.String("config", "./config", "config directory")
	bootstrapAddr := flag.String("bootstrap", "", "bootstrap node multiaddr")
	enableRelay := flag.Bool("relay", false, "enable libp2p relay service")
	advertiseIP := flag.String("advertise-ip", "", "public IP to advertise in shorthand address")
	logFile := flag.String("logfile", "", "write logs to file")
	parentPID := flag.Int("parent-pid", 0, "parent process ID to monitor")
	flag.Parse()

	// 如果指定了父进程 PID，则启动监控协程，父进程退出时本进程也退出
	if *parentPID > 0 {
		go func() {
			for {
				process, err := os.FindProcess(*parentPID)
				if err != nil {
					break
				}
				// 在 Unix 上 Signal(0) 可以检查进程是否存在
				// 在 Windows 上行为略有不同，但 FindProcess + Signal(0) 是通用做法
				err = process.Signal(syscall.Signal(0))
				if err != nil {
					break
				}
				time.Sleep(2 * time.Second)
			}
			fmt.Println("[退出] 父进程已关闭，正在退出程序")
			os.Exit(0)
		}()
	}

	// Windows 环境下确保 Wintun 驱动就绪
	if err := tun.EnsureWintun(); err != nil {
		log.Fatalf("[致命错误] 无法初始化 Wintun 驱动: %v", err)
	}

	// 检查管理员权限
	if !utils.IsAdmin() {
		fmt.Println("检测到当前未以管理员/Root 权限运行。正在尝试提权...")
		if err := utils.SelfElevate(); err != nil {
			log.Fatalf("[致命错误] 权限不足且自动提权失败。请手动使用 sudo (Linux/macOS) 或管理员身份 (Windows) 运行: %v", err)
		}
		// SelfElevate 在某些平台会退出进程并启动新进程
		return
	}

	// 如果指定了日志文件，则重定向标准输出和错误输出
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("无法打开日志文件: %v", err)
		}
		// 重定向全局 log 和 fmt 使用标的输出
		os.Stdout = f
		os.Stderr = f
		log.SetOutput(f)
	}

	// 1. 加载身份
	priv, err := identity.LoadOrGenerateKey(*configDir)
	if err != nil {
		log.Fatalf("[错误] 无法加载节点身份: %v", err)
	}

	id, virtualIP, err := identity.GetPeerInfo(priv)
	if err != nil {
		log.Fatalf("[错误] 无法获取节点信息: %v", err)
	}

	fmt.Printf("[节点] ID=%s 虚拟IP=%s\n", id, virtualIP)

	var bootstrapAddrs []string
	if *bootstrapAddr != "" {
		bootstrapAddrs = []string{*bootstrapAddr}
	}

	// 2. 创建 P2P 节点
	node, err := p2p.NewNodeWithBootstrap(priv, *port, *enableRelay, bootstrapAddrs)
	if err != nil {
		log.Fatalf("[致命错误] P2P 核心网络启动失败，请检查端口占用: %v", err)
	}
	defer node.Close()

	// 获取外部可达地址
	var addrList []string
	var shorthandList []string

	// 如果显式指定了公网 IP，优先将其加入简写列表
	if *advertiseIP != "" {
		shorthandList = append(shorthandList, fmt.Sprintf("%s:%s:%s", *advertiseIP, *port, node.Host.ID()))
	}

	for _, addr := range node.Host.Addrs() {
		// 过滤本地回环和不安全地址
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") || strings.Contains(addrStr, "::1") {
			continue
		}
		fullAddr := fmt.Sprintf("%s/p2p/%s", addrStr, node.Host.ID())
		addrList = append(addrList, fullAddr)

		// 构造简写格式 (仅限 IPv4 TCP)
		if strings.HasPrefix(addrStr, "/ip4/") && strings.Contains(addrStr, "/tcp/") {
			parts := strings.Split(addrStr, "/")
			if len(parts) >= 5 {
				ip := parts[2]
				port := parts[4]
				// 避免重复（如果 advertiseIP 已经加过了）
				if ip != *advertiseIP {
					shorthandList = append(shorthandList, fmt.Sprintf("%s:%s:%s", ip, port, node.Host.ID()))
				}
			}
		}
	}
	fmt.Printf("[网络] 监听地址=%d 端口=%s 中继模式=%v\n", len(addrList), *port, *enableRelay)

	// 将地址和虚拟IP写入文件方便查看（无需翻日志）
	addrFile := filepath.Join(*configDir, "address.txt")
	fileContent := fmt.Sprintf("虚拟IP: %s\n\n简写格式 (推荐):\n%s\n\n标准 Multiaddr:\n%s\n",
		virtualIP, strings.Join(shorthandList, "\n"), strings.Join(addrList, "\n"))
	if err := os.WriteFile(addrFile, []byte(fileContent), 0644); err == nil {
		// 确保即便父目录是 700，文件本身也是可读的（如果上级目录允许的话）
		os.Chmod(addrFile, 0644)
		fmt.Printf("[配置] 地址信息已保存到: %s\n", addrFile)
	}

	// 4. 创建 TUN 网卡
	itf, err := tun.New(virtualIP)
	if err != nil {
		log.Fatalf("[致命错误] 无法创建虚拟网卡，权限不足: %v", err)
	}
	defer itf.Close()
	fmt.Printf("[网卡] 名称=%s IP=%s\n", itf.Name, virtualIP)

	// 5. 启动网桥 (必须在 Bootstrap 之前，以确保捕捉到初始连接事件)
	br := bridge.New(itf, node, *configDir)
	br.Start()

	// 6. 连接引导节点
	if *bootstrapAddr != "" {
		if err := node.Bootstrap([]string{*bootstrapAddr}); err != nil {
			fmt.Printf("[引导] 连接失败: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("[就绪] MeshLink 已启动并在后台运行")

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("[退出] 正在停止服务")
}
