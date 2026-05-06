package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
)

const (
	ProtocolID        = "/p2p-vpn/1.0.0"
	ControlProtocolID = "/p2p-vpn-control/1.0.0"
)

// Node 封装了 libp2p 主机及其相关服务
type Node struct {
	Host   host.Host
	DHT    *dht.IpfsDHT
	Relay  *relay.Relay // 增加 Relay 字段
	Ctx    context.Context
	Cancel context.CancelFunc
	direct sync.Map
	punch  sync.Map

	streams sync.Map // map[peer.ID]network.Stream

	// PacketHandler 当接收到 P2P 流量时的处理回调（通常写入 TUN）
	PacketHandler func(data []byte)

	// PeerConnectedHandler 当建立新连接时的回调（用于交换 VIP）
	PeerConnectedHandler func(p peer.ID)
}

// NewNode 创建并启动一个 libp2p 节点
func NewNode(priv crypto.PrivKey, listenAddr string, enableRelay bool) (*Node, error) {
	return NewNodeWithBootstrap(priv, listenAddr, enableRelay, nil)
}

// NewNodeWithBootstrap 创建节点，并把引导节点配置为静态 AutoRelay 候选。
func NewNodeWithBootstrap(priv crypto.PrivKey, listenAddr string, enableRelay bool, bootstrapAddrs []string) (*Node, error) {
	ctx, cancel := context.WithCancel(context.Background())

	var idht *dht.IpfsDHT
	connm, err := connmgr.NewConnManager(100, 400, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		cancel()
		return nil, err
	}

	dhtMode := dht.ModeAuto
	dhtModeName := "auto"
	if enableRelay {
		dhtMode = dht.ModeServer
		dhtModeName = "server"
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
			idht, err = dht.New(ctx, h, dht.Mode(dhtMode))
			return idht, err
		}),
	}

	staticRelays := parseRelayCandidates(bootstrapAddrs)
	if len(staticRelays) > 0 && !enableRelay {
		opts = append(opts,
			libp2p.EnableAutoRelayWithStaticRelays(staticRelays,
				autorelay.WithBootDelay(time.Second),
				autorelay.WithNumRelays(len(staticRelays)),
			),
			libp2p.ForceReachabilityPrivate(),
		)
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, err
	}
	fmt.Printf("[dht] mode=%s\n", dhtModeName)

	var r *relay.Relay
	if enableRelay {
		r, err = relay.New(h)
		if err != nil {
			fmt.Printf("[relay] failed to enable service: %v\n", err)
		} else {
			fmt.Println("[relay] service enabled")
		}
	} else if len(staticRelays) > 0 {
		fmt.Printf("[relay] candidates=%d usage=hole-punch-control\n", len(staticRelays))
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

	// 监听连接事件，以便主动发起 VIP 交换
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(net network.Network, conn network.Conn) {
			remotePeer := conn.RemotePeer()
			fmt.Printf(">>> [NETWORK] CONNECTED: peer=%s addr=%s\n", remotePeer, conn.RemoteMultiaddr())
			if node.PeerConnectedHandler != nil {
				node.PeerConnectedHandler(remotePeer)
			}
		},
		DisconnectedF: func(net network.Network, conn network.Conn) {
			fmt.Printf("<<< [NETWORK] DISCONNECTED: peer=%s\n", conn.RemotePeer())
		},
	})

	return node, nil
}

// handleStream 处理来自远程节点的 P2P 流（接收到的 IP 数据包）
func (n *Node) handleStream(s network.Stream) {
	defer s.Close()
	remotePeer := s.Conn().RemotePeer()

	if !isDirectConn(s.Conn()) {
		fmt.Printf("[tunnel] inbound: relayed stream peer=%s\n", remotePeer)
	} else {
		fmt.Printf("[tunnel] inbound: direct stream peer=%s\n", remotePeer)
	}

	header := make([]byte, 4)
	for {
		_, err := io.ReadFull(s, header)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("[tunnel] stream read error peer=%s: %v\n", remotePeer, err)
			}
			break
		}

		length := binary.BigEndian.Uint32(header)
		if length > 2000 {
			fmt.Printf("[tunnel] packet too large: %d\n", length)
			break
		}

		buf := make([]byte, length)
		_, err = io.ReadFull(s, buf)
		if err != nil {
			fmt.Printf("[tunnel] stream body read error peer=%s: %v\n", remotePeer, err)
			break
		}

		if n.PacketHandler != nil {
			n.PacketHandler(buf)
		}
	}

	n.streams.Delete(remotePeer)
	fmt.Printf("[tunnel] stream closed peer=%s\n", remotePeer)
}

// AddPeerAddrs 将发现到的地址加入 peerstore，返回直连地址和中继地址数量。
func (n *Node) AddPeerAddrs(info peer.AddrInfo) (int, int) {
	if len(info.Addrs) == 0 {
		return 0, 0
	}
	directAddrs, relayedAddrs := splitAddrs(info.Addrs)
	n.Host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.TempAddrTTL)
	return len(directAddrs), len(relayedAddrs)
}

// AddrCounts 返回本节点正在对外发布的直连地址和 relay 控制地址数量。
func (n *Node) AddrCounts() (int, int) {
	directAddrs, relayedAddrs := splitAddrs(n.Host.Addrs())
	return len(directAddrs), len(relayedAddrs)
}

// EnsureDirectConn 尝试与目标节点建立连接，优先尝试直连。
func (n *Node) EnsureDirectConn(target peer.ID) error {
	if c := n.bestConn(target); c != nil {
		return nil
	}

	info := n.Host.Peerstore().PeerInfo(target)
	if len(info.Addrs) == 0 && n.DHT != nil {
		ctx, cancel := context.WithTimeout(n.Ctx, 15*time.Second)
		found, err := n.DHT.FindPeer(ctx, target)
		cancel()
		if err != nil {
			return fmt.Errorf("find peer addresses: %w", err)
		}
		info = found
	}

	if len(info.Addrs) == 0 {
		return fmt.Errorf("no addresses for peer %s", target)
	}

	n.Host.Peerstore().AddAddrs(target, info.Addrs, peerstore.TempAddrTTL)
	directAddrs, relayedAddrs := splitAddrs(info.Addrs)
	var directErr error

	if len(directAddrs) > 0 {
		ctx, cancel := context.WithTimeout(n.Ctx, 15*time.Second)
		ctx = network.WithForceDirectDial(ctx, "p2p-vpn prefers direct client tunnel")
		err := n.Host.Connect(ctx, peer.AddrInfo{ID: target, Addrs: directAddrs})
		cancel()
		if err == nil {
			if c := n.directConn(target); c != nil {
				n.logDirectReady(target, c, "direct")
				return nil
			}
			directErr = fmt.Errorf("direct dial completed without a direct connection")
		} else {
			directErr = err
		}
	}

	if c := n.bestConn(target); c != nil {
		return nil
	}

	if len(relayedAddrs) == 0 {
		if directErr != nil {
			return fmt.Errorf("direct dial failed and no relay address available: %w", directErr)
		}
		return fmt.Errorf("no tunnel to peer %s", target)
	}

	if _, loaded := n.punch.LoadOrStore(target, struct{}{}); !loaded {
		fmt.Printf("[tunnel] establishing connection (with relay) peer=%s direct_addrs=%d relay_addrs=%d\n", target, len(directAddrs), len(relayedAddrs))
	}
	ctx, cancel := context.WithTimeout(n.Ctx, 20*time.Second)
	err := n.Host.Connect(ctx, peer.AddrInfo{ID: target, Addrs: info.Addrs})
	cancel()
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	if c := n.bestConn(target); c != nil {
		if isDirectConn(c) {
			n.logDirectReady(target, c, "hole-punch")
		}
		return nil
	}

	return fmt.Errorf("failed to establish any connection to peer %s", target)
}

func (n *Node) getStream(target peer.ID) (network.Stream, error) {
	if v, ok := n.streams.Load(target); ok {
		return v.(network.Stream), nil
	}

	if err := n.EnsureDirectConn(target); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(n.Ctx, 15*time.Second)
	defer cancel()
	s, err := n.Host.NewStream(ctx, target, ProtocolID)
	if err != nil {
		return nil, err
	}

	if !isDirectConn(s.Conn()) {
		fmt.Printf("[tunnel] outbound: relayed stream peer=%s\n", target)
	} else {
		fmt.Printf("[tunnel] outbound: direct stream peer=%s\n", target)
	}

	actual, loaded := n.streams.LoadOrStore(target, s)
	if loaded {
		_ = s.Reset()
		return actual.(network.Stream), nil
	}
	return s, nil
}

// SendPacket 向指定节点发送 IP 数据包。
func (n *Node) SendPacket(target peer.ID, data []byte) error {
	s, err := n.getStream(target)
	if err != nil {
		return err
	}

	// 使用长度前缀进行帧包装
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))

	if _, err := s.Write(header); err != nil {
		n.streams.Delete(target)
		_ = s.Reset()
		return fmt.Errorf("send length prefix: %w", err)
	}

	if _, err := s.Write(data); err != nil {
		n.streams.Delete(target)
		_ = s.Reset()
		return fmt.Errorf("send packet data: %w", err)
	}

	return nil
}

// Bootstrap 连接到一组引导节点
func (n *Node) Bootstrap(addrs []string) error {
	connected := false
	for _, addrStr := range addrs {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			fmt.Printf("[bootstrap] invalid addr: %v\n", err)
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			fmt.Printf("[bootstrap] invalid peer addr: %v\n", err)
			continue
		}
		if err := n.Host.Connect(n.Ctx, *info); err != nil {
			fmt.Printf("[bootstrap] connect failed peer=%s err=%v\n", info.ID, err)
		} else {
			fmt.Printf("[bootstrap] connected peer=%s\n", info.ID)
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

func parseRelayCandidates(addrs []string) []peer.AddrInfo {
	relays := make([]peer.AddrInfo, 0, len(addrs))
	seen := make(map[peer.ID]struct{})
	for _, addrStr := range addrs {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			fmt.Printf("[relay] ignore invalid bootstrap addr: %v\n", err)
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			fmt.Printf("[relay] ignore invalid bootstrap peer addr: %v\n", err)
			continue
		}
		if _, ok := seen[info.ID]; ok {
			continue
		}
		seen[info.ID] = struct{}{}
		relays = append(relays, *info)
	}
	return relays
}

func (n *Node) bestConn(target peer.ID) network.Conn {
	conns := n.Host.Network().ConnsToPeer(target)
	if len(conns) == 0 {
		return nil
	}
	// 优先返回直连
	for _, c := range conns {
		if isDirectConn(c) {
			return c
		}
	}
	// 兜底返回第一个（可能是 relay）
	return conns[0]
}

func (n *Node) directConn(target peer.ID) network.Conn {
	for _, c := range n.Host.Network().ConnsToPeer(target) {
		if isDirectConn(c) {
			return c
		}
	}
	return nil
}

func (n *Node) logDirectReady(target peer.ID, c network.Conn, via string) {
	if _, loaded := n.direct.LoadOrStore(target, struct{}{}); loaded {
		return
	}
	fmt.Printf("[tunnel] direct ready peer=%s via=%s remote=%s\n", target, via, c.RemoteMultiaddr())
}

func isDirectConn(c network.Conn) bool {
	if c == nil || c.Stat().Limited {
		return false
	}
	return !hasProtocol(c.RemoteMultiaddr(), multiaddr.P_CIRCUIT)
}

func splitAddrs(addrs []multiaddr.Multiaddr) ([]multiaddr.Multiaddr, []multiaddr.Multiaddr) {
	direct := make([]multiaddr.Multiaddr, 0, len(addrs))
	relayed := make([]multiaddr.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		if hasProtocol(addr, multiaddr.P_CIRCUIT) {
			relayed = append(relayed, addr)
		} else {
			direct = append(direct, addr)
		}
	}
	return direct, relayed
}

func hasProtocol(addr multiaddr.Multiaddr, code int) bool {
	_, err := addr.ValueForProtocol(code)
	return err == nil
}
