package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"p2p/pkg/p2p"
	"p2p/pkg/tun"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

// Bridge 连接 TUN 网卡和 P2P 网络
type Bridge struct {
	tun      *tun.Interface
	node     *p2p.Node
	ctx      context.Context
	
	// routeCache 缓存 虚拟IP -> PeerID 的映射
	routeCache sync.Map // map[string]peer.ID
}

func New(t *tun.Interface, n *p2p.Node) *Bridge {
	return &Bridge{
		tun:  t,
		node: n,
		ctx:  n.Ctx,
	}
}

// Start 启动双向转发循环
func (b *Bridge) Start() {
	// 1. 处理从 P2P 接收到的数据包
	b.node.PacketHandler = func(data []byte) {
		b.tun.Write(data)
	}

	// 2. 启动从 TUN 读取的循环
	go b.readTunLoop()

	// 3. 在 DHT 中发布自己的 IP，以便他人发现
	go b.advertiseIPLoop()
}

func (b *Bridge) readTunLoop() {
	buf := make([]byte, 2048)
	for {
		n, err := b.tun.Read(buf)
		if err != nil {
			fmt.Printf("TUN read error: %v\n", err)
			return
		}

		packetData := make([]byte, n)
		copy(packetData, buf[:n])

		go b.handleOutgoingPacket(packetData)
	}
}

func (b *Bridge) handleOutgoingPacket(data []byte) {
	packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ip, _ := ipLayer.(*layers.IPv4)
		dstIP := ip.DstIP.String()

		// 忽略发往本机的包
		if dstIP == b.tun.IP.String() {
			return
		}

		peerID, err := b.resolvePeerID(dstIP)
		if err != nil {
			fmt.Printf("[Bridge] Could not resolve PeerID for IP %s: %v\n", dstIP, err)
			return
		}

		fmt.Printf("[Bridge] Sending packet (%d bytes) to Peer %s\n", len(data), peerID)
		if err := b.node.SendPacket(peerID, data); err != nil {
			fmt.Printf("[Bridge] Failed to send packet: %v\n", err)
		}
	}
}

func (b *Bridge) resolvePeerID(ip string) (peer.ID, error) {
	if v, ok := b.routeCache.Load(ip); ok {
		return v.(peer.ID), nil
	}

	// 从 DHT 查询
	c, _ := ipToCid(ip)
	providers := b.node.DHT.FindProvidersAsync(b.ctx, c, 1)
	
	select {
	case p := <-providers:
		if p.ID != "" {
			b.routeCache.Store(ip, p.ID)
			return p.ID, nil
		}
	case <-time.After(time.Second * 5):
	}

	return "", fmt.Errorf("peer not found for IP %s", ip)
}

func (b *Bridge) advertiseIPLoop() {
	c, _ := ipToCid(b.tun.IP.String())
	
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		if err := b.node.DHT.Provide(b.ctx, c, true); err != nil {
			fmt.Printf("[Bridge] Failed to advertise IP in DHT: %v\n", err)
		} else {
			fmt.Printf("[Bridge] IP %s advertised successfully\n", b.tun.IP.String())
		}
		
		select {
		case <-ticker.C:
		case <-b.ctx.Done():
			return
		}
	}
}

func ipToCid(ip string) (cid.Cid, error) {
	v := []byte("ip:" + ip)
	pref := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}
	return pref.Sum(v)
}
