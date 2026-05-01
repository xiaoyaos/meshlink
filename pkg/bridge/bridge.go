package bridge

import (
	"context"
	"fmt"
	"strings"
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
			fmt.Printf("[桥接] 读取虚拟网卡失败: %v\n", err)
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

		// 严格过滤：只处理目标 IP 是 10.x.x.x 虚拟子网的包
		// 这可以拦截系统泄露的外部公网 IP、组播（224.x）、广播等
		if !strings.HasPrefix(dstIP, "10.") {
			return
		}

		peerID, err := b.resolvePeerID(dstIP)
		if err != nil {
			fmt.Printf("[寻址失败] 找不到目标 IP (%s) 对应的设备，可能对方已掉线或尚未广播 IP: %v\n", dstIP, err)
			return
		}

		fmt.Printf("[数据流向] 虚拟网卡发出 -> P2P隧道 (目标IP: %s, 大小: %d 字节, 接收节点: %s)\n", dstIP, len(data), peerID)
		if err := b.node.SendPacket(peerID, data); err != nil {
			fmt.Printf("[错误] 通过 P2P 隧道发送数据失败: %v\n", err)
		}
	} else {
		fmt.Printf("[拦截] 拦截到非 IPv4 流量，已自动丢弃 (大小: %d 字节)\n", len(data))
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
	
	// 初始化时如果 DHT 里还没节点，重试间隔设短一点
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		if err := b.node.DHT.Provide(b.ctx, c, true); err != nil {
			fmt.Printf("[网络寻址] ⚠️ 尚未同步路由表，正在尝试全网广播本机 IP (%s)... (将不断重试直到成功)\n", b.tun.IP.String())
			ticker.Reset(time.Second * 5) // 失败则 5 秒后重试
		} else {
			fmt.Printf("[网络寻址] ✅ 成功广播本机 IP (%s)！网络中的其他设备现在可以连接到你了。\n", b.tun.IP.String())
			ticker.Reset(time.Minute * 5) // 成功后，只需每 5 分钟保活一次
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
