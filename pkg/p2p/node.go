package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
)

const (
	ProtocolID = "/p2p-vpn/1.0.0"
)

// Node 封装了 libp2p 主机及其相关服务
type Node struct {
	Host   host.Host
	DHT    *dht.IpfsDHT
	Relay  *relay.Relay // 增加 Relay 字段
	Ctx    context.Context
	Cancel context.CancelFunc

	// PacketHandler 当接收到 P2P 流量时的处理回调（通常写入 TUN）
	PacketHandler func(data []byte)
}

// NewNode 创建并启动一个 libp2p 节点
func NewNode(priv crypto.PrivKey, listenAddr string, enableRelay bool) (*Node, error) {
	ctx, cancel := context.WithCancel(context.Background())

	var idht *dht.IpfsDHT
	connm, err := connmgr.NewConnManager(100, 400, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		cancel()
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", listenAddr),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%s/quic-v1", listenAddr),
		),
		libp2p.DefaultTransports,
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.ConnectionManager(connm),
		libp2p.NATPortMap(), // 尝试 UPnP/PMP
		libp2p.EnableAutoNATv2(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			idht, err = dht.New(ctx, h, dht.Mode(dht.ModeServer))
			return idht, err
		}),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, err
	}

	var r *relay.Relay
	if enableRelay {
		r, err = relay.New(h)
		if err != nil {
			fmt.Printf("failed to start relay service: %v\n", err)
		} else {
			fmt.Println("Relay service enabled.")
		}
	}

	node := &Node{
		Host:   h,
		DHT:    idht,
		Relay:  r,
		Ctx:    ctx,
		Cancel: cancel,
	}

	// 注册协议处理器
	h.SetStreamHandler(ProtocolID, node.handleStream)

	return node, nil
}

// handleStream 处理来自远程节点的 P2P 流（接收到的 IP 数据包）
func (n *Node) handleStream(s network.Stream) {
	defer s.Close()
	buf := make([]byte, 2048) // MTU 足够
	for {
		read, err := s.Read(buf)
		if err != nil {
			return
		}
		if n.PacketHandler != nil {
			n.PacketHandler(buf[:read])
		}
	}
}

// SendPacket 向指定节点发送 IP 数据包
func (n *Node) SendPacket(target peer.ID, data []byte) error {
	s, err := n.Host.NewStream(n.Ctx, target, ProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()

	_, err = s.Write(data)
	return err
}

// Bootstrap 连接到一组引导节点
func (n *Node) Bootstrap(addrs []string) error {
	connected := false
	for _, addrStr := range addrs {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			fmt.Printf("invalid multiaddr %s: %v\n", addrStr, err)
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			fmt.Printf("invalid addr info %s: %v\n", addrStr, err)
			continue
		}
		fmt.Printf("Attempting to connect to bootstrap node: %s\n", info.ID)
		if err := n.Host.Connect(n.Ctx, *info); err != nil {
			fmt.Printf("FAILED to connect to bootstrap node %s: %v\n", info.ID, err)
		} else {
			fmt.Printf("SUCCESSFULLY connected to bootstrap node: %s\n", info.ID)
			connected = true
		}
	}

	if !connected && len(addrs) > 0 {
		return fmt.Errorf("could not connect to any bootstrap nodes")
	}

	if err := n.DHT.Bootstrap(n.Ctx); err != nil {
		return err
	}

	return nil
}

// Close 关闭节点
func (n *Node) Close() error {
	n.Cancel()
	return n.Host.Close()
}
