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
			fmt.Println("[shutdown] parent process died, exiting")
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

	fmt.Printf("[node] peer=%s vip=%s\n", id, virtualIP)

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

	var addrList []string
	for _, addr := range node.Host.Addrs() {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, node.Host.ID())
		addrList = append(addrList, fullAddr)
	}
	fmt.Printf("[listen] addrs=%d port=%s relay=%v\n", len(addrList), *port, *enableRelay)

	// 将地址和虚拟IP写入文件方便查看（无需翻日志）
	addrFile := filepath.Join(*configDir, "address.txt")
	fileContent := fmt.Sprintf("Virtual IP: %s\n\nMultiaddr:\n%s\n", virtualIP, strings.Join(addrList, "\n"))
	if err := os.WriteFile(addrFile, []byte(fileContent), 0644); err == nil {
		fmt.Printf("[config] address_file=%s\n", addrFile)
	}

	// 4. 创建 TUN 网卡
	itf, err := tun.New(virtualIP)
	if err != nil {
		log.Fatalf("[致命错误] 无法创建虚拟网卡，权限不足。请在 Linux/macOS 使用 sudo 运行，或在 Windows 使用管理员权限: %v", err)
	}
	defer itf.Close()
	fmt.Printf("[tun] name=%s ip=%s\n", itf.Name, virtualIP)

	// 5. 启动网桥 (必须在 Bootstrap 之前，以确保捕捉到初始连接事件)
	br := bridge.New(itf, node)
	br.Start()

	// 6. 连接引导节点
	if *bootstrapAddr != "" {
		if err := node.Bootstrap([]string{*bootstrapAddr}); err != nil {
			fmt.Printf("[fatal] bootstrap failed: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("[ready] meshlink is running")

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("[shutdown] stopping")
}
