package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	flag.Parse()

	// 1. 加载身份
	priv, err := identity.LoadOrGenerateKey(*configDir)
	if err != nil {
		log.Fatalf("failed to load identity: %v", err)
	}

	id, virtualIP, err := identity.GetPeerInfo(priv)
	if err != nil {
		log.Fatalf("failed to get peer info: %v", err)
	}

	fmt.Printf("My PeerID: %s\n", id)
	fmt.Printf("My Virtual IP: %s\n", virtualIP)

	// 2. 创建 P2P 节点
	node, err := p2p.NewNode(priv, *port, *enableRelay)
	if err != nil {
		log.Fatalf("failed to start p2p node: %v", err)
	}
	defer node.Close()

	fmt.Printf("P2P Node started. Listening on:\n")
	for _, addr := range node.Host.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", addr, node.Host.ID())
	}

	// 3. 连接引导节点
	if *bootstrapAddr != "" {
		if err := node.Bootstrap([]string{*bootstrapAddr}); err != nil {
			log.Printf("bootstrap failed: %v", err)
		} else {
			fmt.Println("Connected to bootstrap node.")
		}
	}

	// 4. 创建 TUN 网卡
	itf, err := tun.New(virtualIP)
	if err != nil {
		log.Fatalf("failed to create TUN interface (try running with sudo): %v", err)
	}
	defer itf.Close()
	fmt.Printf("TUN interface %s created with IP %s\n", itf.Name, virtualIP)

	// 5. 启动网桥
	br := bridge.New(itf, node)
	br.Start()

	fmt.Println("Network is ready. Press Ctrl+C to exit.")

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
}
