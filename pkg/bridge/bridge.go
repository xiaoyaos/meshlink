package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"p2p/pkg/p2p"
	"p2p/pkg/tun"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	mh "github.com/multiformats/go-multihash"
)

const (
	providerQueryLimit = 8
	providerQueryTime  = 12 * time.Second
	fastAdvertiseCount = 12
)

// PeerInfo 存储单个对等节点的状态
type PeerInfo struct {
	VIP      string `json:"vip"`
	ID       string `json:"id"`
	Direct   bool   `json:"direct"`
	LastSeen string `json:"last_seen"`
}

// GlobalState 存储整个节点的运行状态
type GlobalState struct {
	SelfVIP   string              `json:"self_vip"`
	SelfID    string              `json:"self_id"`
	Peers     map[string]PeerInfo `json:"peers"` // VIP -> PeerInfo
	UpdatedAt string              `json:"updated_at"`
}

// Bridge 连接 TUN 网卡和 P2P 网络
type Bridge struct {
	tun       *tun.Interface
	node      *p2p.Node
	ctx       context.Context
	configDir string // 存储状态文件的路径
	announce  bool
	lastAdvOK bool
	logMu     sync.Mutex
	lastLog   map[string]time.Time

	// routeCache 缓存 虚拟IP -> PeerID 的映射
	routeCache sync.Map // map[string]peer.ID
	resolving  sync.Map // map[string]*routeResolveCall
}

type routeResolveCall struct {
	done chan struct{}
	peer peer.ID
	err  error
}

func New(t *tun.Interface, n *p2p.Node, configDir string) *Bridge {
	b := &Bridge{
		tun:       t,
		node:      n,
		ctx:       n.Ctx,
		configDir: configDir,
		lastLog:   make(map[string]time.Time),
	}

	// 注册控制协议，用于直接交换 VIP
	n.Host.SetStreamHandler(p2p.ControlProtocolID, b.handleControlStream)

	// 当建立连接时，主动发起 VIP 告知
	n.PeerConnectedHandler = func(p peer.ID) {
		if b.shouldLog("control:send:"+p.String(), time.Minute) {
			fmt.Printf("[控制] 正在向节点同步 VIP 信息: %s\n", p)
		}
		// 1. 发送自己的 VIP
		_ = b.sendLocalVIP(p)

		// 2. 如果自己是服务器，则把当前已知的整个路由表发给新来的 Peer
		if b.node.Relay != nil {
			b.syncRegistryToPeer(p)
		}
	}

	return b
}


func (b *Bridge) sendLocalVIP(target peer.ID) error {
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()

	s, err := b.node.Host.NewStream(ctx, target, p2p.ControlProtocolID)
	if err != nil {
		return err
	}
	defer s.Close()

	// 格式: "vip:<IP>:<PeerID>"
	msg := fmt.Sprintf("vip:%s:%s", b.tun.IP.String(), b.node.Host.ID().String())
	_, err = s.Write([]byte(msg))
	return err
}

func (b *Bridge) syncRegistryToPeer(target peer.ID) {
	b.routeCache.Range(func(key, value any) bool {
		vip := key.(string)
		owner := value.(peer.ID)
		if owner == target {
			return true
		}

		go func(v string, o peer.ID) {
			ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
			defer cancel()
			s, err := b.node.Host.NewStream(ctx, target, p2p.ControlProtocolID)
			if err != nil {
				return
			}
			defer s.Close()
			msg := fmt.Sprintf("vip:%s:%s", v, o.String())
			_, _ = s.Write([]byte(msg))
		}(vip, owner)
		return true
	})
}

func (b *Bridge) handleControlStream(s network.Stream) {
	defer s.Close()

	buf := make([]byte, 256)
	n, err := s.Read(buf)
	if err != nil {
		return
	}

	msg := string(buf[:n])
	if strings.HasPrefix(msg, "vip:") {
		parts := strings.Split(msg, ":")
		if len(parts) < 3 {
			return
		}
		vip := parts[1]
		ownerIDStr := parts[2]
		ownerID, err := peer.Decode(ownerIDStr)
		if err != nil {
			return
		}

		if b.shouldLog("control:learned:"+vip, time.Minute) {
			fmt.Printf("[control] learned mapping: %s -> %s\n", vip, ownerID)
		}
		b.routeCache.Store(vip, ownerID)

		// 如果本地是服务器，则广播给其他所有人。
		if b.node.Relay != nil {
			go b.broadcastVIPInfo(vip, ownerID)
		}
	}
}

func (b *Bridge) broadcastVIPInfo(vip string, owner peer.ID) {
	peers := b.node.Host.Network().Peers()
	msg := fmt.Sprintf("vip:%s:%s", vip, owner.String())
	for _, p := range peers {
		if p == owner || p == b.node.Host.ID() {
			continue
		}
		go func(target peer.ID) {
			ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
			defer cancel()
			s, err := b.node.Host.NewStream(ctx, target, p2p.ControlProtocolID)
			if err != nil {
				return
			}
			defer s.Close()
			_, _ = s.Write([]byte(msg))
		}(p)
	}
}

// Start 启动双向转发循环
func (b *Bridge) Start() {
	// 1. 处理从 P2P 接收到的数据包
	b.node.PacketHandler = func(data []byte) {
		// 解析目标 IP
		packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)
		if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
			ip, _ := ipLayer.(*layers.IPv4)
			dstIP := ip.DstIP.String()

			// 情况 A: 发给本机的包 -> 写入 TUN
			if dstIP == b.tun.IP.String() {
				n, err := b.tun.Write(data)
				if err != nil {
					if b.shouldLog("tun:write-err", time.Second*5) {
						fmt.Printf("[网卡] 写入失败: %v\n", err)
					}
				} else {
					if b.shouldLog("bridge:inbound", time.Second*10) {
						fmt.Printf("[网桥] 收到入站数据包: 长度=%d\n", n)
					}
				}
				return
			}

			// 情况 B: 发给别人的包 -> 如果自己是服务器，则执行路由转发
			if b.node.Relay != nil {
				if peerID, ok := b.routeCache.Load(dstIP); ok {
					targetID := peerID.(peer.ID)
					if b.shouldLog("bridge:forward:"+dstIP, time.Second*5) {
						fmt.Printf("[中转] 正在转发数据包: 目标=%s 通过节点=%s\n", dstIP, targetID)
					}
					go func() {
						_ = b.node.SendPacket(targetID, data)
					}()
					return
				}
			}
		}
	}

	// 2. 启动从 TUN 读取的循环
	go b.readTunLoop()

	// 3. 在 DHT 中发布自己的 IP
	go b.advertiseIPLoop()

	// 4. 定期向所有已连接的 Peer 宣告自己的 VIP (保底机制)
	go b.periodicAnnounceLoop()

	// 5. 定期更新实时状态文件供 CLI 工具查看
	go b.stateUpdateLoop()
}

func (b *Bridge) stateUpdateLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.writeStateFile()
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *Bridge) writeStateFile() {
	state := GlobalState{
		SelfVIP:   b.tun.IP.String(),
		SelfID:    b.node.Host.ID().String(),
		Peers:     make(map[string]PeerInfo),
		UpdatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	b.routeCache.Range(func(key, value any) bool {
		vip := key.(string)
		ownerID := value.(peer.ID)

		if vip == state.SelfVIP {
			return true
		}

		// 检查连接类型
		isDirect := false
		for _, conn := range b.node.Host.Network().ConnsToPeer(ownerID) {
			if !conn.Stat().Limited && !strings.Contains(conn.RemoteMultiaddr().String(), "p2p-circuit") {
				isDirect = true
				break
			}
		}

		state.Peers[vip] = PeerInfo{
			VIP:      vip,
			ID:       ownerID.String(),
			Direct:   isDirect,
			LastSeen: time.Now().Format("15:04:05"),
		}
		return true
	})

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}

	statePath := filepath.Join(b.configDir, "state.json")
	_ = os.WriteFile(statePath, data, 0644)
}

func (b *Bridge) periodicAnnounceLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			peers := b.node.Host.Network().Peers()
			for _, p := range peers {
				go b.sendLocalVIP(p)
			}
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *Bridge) readTunLoop() {
	packetChan := make(chan []byte, 2048)

	// 启动更多工作协程，并确保它们不会被阻塞搜索死锁
	for i := 0; i < 16; i++ {
		go func() {
			for {
				select {
				case data, ok := <-packetChan:
					if !ok {
						return
					}
					b.handleOutgoingPacket(data)
				case <-b.ctx.Done():
					return
				}
			}
		}()
	}

	buf := make([]byte, 2048)
	for {
		n, err := b.tun.Read(buf)
		if err != nil {
			fmt.Printf("[tun] read failed: %v\n", err)
			close(packetChan)
			return
		}

		packetData := make([]byte, n)
		copy(packetData, buf[:n])

		select {
		case packetChan <- packetData:
		default:
			if b.shouldLog("bridge:drop", time.Second*5) {
				fmt.Println("[bridge] queue full, packet dropped")
			}
		}
	}
}

func (b *Bridge) handleOutgoingPacket(data []byte) {
	packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ip, _ := ipLayer.(*layers.IPv4)
		dstIP := ip.DstIP.String()

		if dstIP == b.tun.IP.String() {
			return
		}

		if !strings.HasPrefix(dstIP, "10.") {
			return
		}

		// 异步处理路由解析和发送，避免阻塞 worker pool
		go func() {
			if b.shouldLog("bridge:out-start:"+dstIP, time.Second) {
				fmt.Printf("[路由] 正在寻址: %s\n", dstIP)
			}
			peerID, err := b.resolvePeerID(dstIP)
			if err != nil {
				if b.shouldLog("route:"+dstIP, 15*time.Second) {
					fmt.Printf("[路由] 寻址失败 目标=%s 错误=%v\n", dstIP, err)
				}
				return
			}

			if err := b.node.SendPacket(peerID, data); err != nil {
				if b.shouldLog("send:"+dstIP, 10*time.Second) {
					fmt.Printf("[隧道] 发送失败 目标=%s 节点=%s 错误=%v\n", dstIP, peerID, err)
				}
			} else if b.shouldLog("bridge:outbound:"+dstIP, time.Second*5) {
				fmt.Printf("[网桥] 已发包: 目标=%s 长度=%d\n", dstIP, len(data))
			}
		}()
	}
}

func (b *Bridge) resolvePeerID(ip string) (peer.ID, error) {
	if v, ok := b.routeCache.Load(ip); ok {
		return v.(peer.ID), nil
	}

	call := &routeResolveCall{done: make(chan struct{})}
	actual, loaded := b.resolving.LoadOrStore(ip, call)
	if loaded {
		existing := actual.(*routeResolveCall)
		select {
		case <-existing.done:
			return existing.peer, existing.err
		case <-b.ctx.Done():
			return "", b.ctx.Err()
		}
	}
	defer b.resolving.Delete(ip)
	defer close(call.done)

	call.peer, call.err = b.resolvePeerIDUncached(ip)
	return call.peer, call.err
}

func (b *Bridge) resolvePeerIDUncached(ip string) (peer.ID, error) {
	// 从 DHT 查询
	c, _ := ipToCid(ip)
	ctx, cancel := context.WithTimeout(b.ctx, providerQueryTime)
	defer cancel()
	providers := b.node.DHT.FindProvidersAsync(ctx, c, providerQueryLimit)

	var candidates int
	var lastErr error
	seen := make(map[peer.ID]struct{})
	for {
		select {
		case p, ok := <-providers:
			if !ok {
				if candidates == 0 {
					return "", fmt.Errorf("peer not found for IP %s", ip)
				}
				return "", fmt.Errorf("no usable direct tunnel for IP %s after %d candidate(s): %w", ip, candidates, lastErr)
			}
			if p.ID == "" || p.ID == b.node.Host.ID() {
				continue
			}
			if _, ok := seen[p.ID]; ok {
				continue
			}
			seen[p.ID] = struct{}{}
			candidates++

			directAddrs, relayedAddrs := b.node.AddPeerAddrs(p)
			if b.shouldLog("route-candidate:"+ip+":"+p.ID.String(), time.Minute) {
				fmt.Printf("[route] candidate ip=%s peer=%s direct_addrs=%d relay_addrs=%d\n", ip, p.ID, directAddrs, relayedAddrs)
			}
			if err := b.node.EnsureDirectConn(p.ID); err != nil {
				lastErr = fmt.Errorf("peer=%s direct_addrs=%d relay_addrs=%d err=%w", p.ID, directAddrs, relayedAddrs, err)
				if b.shouldLog("route-direct-failed:"+ip+":"+p.ID.String(), 30*time.Second) {
					fmt.Printf("[route] direct failed ip=%s peer=%s err=%v\n", ip, p.ID, err)
				}
				continue
			}

			b.routeCache.Store(ip, p.ID)
			fmt.Printf("[route] direct ready ip=%s peer=%s candidates=%d\n", ip, p.ID, candidates)
			return p.ID, nil
		case <-ctx.Done():
			if candidates == 0 {
				return "", fmt.Errorf("peer not found for IP %s: %w", ip, ctx.Err())
			}
			return "", fmt.Errorf("no usable direct tunnel for IP %s after %d candidate(s): %w", ip, candidates, lastErr)
		}
	}
}

func (b *Bridge) advertiseIPLoop() {
	c, _ := ipToCid(b.tun.IP.String())

	// 初始化时如果 DHT 里还没节点，重试间隔设短一点
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	successes := 0

	for {
		directAddrs, relayedAddrs := b.node.AddrCounts()
		if err := b.node.DHT.Provide(b.ctx, c, true); err != nil {
			if b.lastAdvOK || !b.announce {
				fmt.Printf("[advertise] waiting ip=%s err=%v\n", b.tun.IP.String(), err)
			}
			b.lastAdvOK = false
			b.announce = true
			successes = 0
			ticker.Reset(time.Second * 5) // 失败则 5 秒后重试
		} else {
			if !b.lastAdvOK {
				fmt.Printf("[advertise] ready ip=%s direct_addrs=%d relay_addrs=%d\n", b.tun.IP.String(), directAddrs, relayedAddrs)
			} else if successes < fastAdvertiseCount && b.shouldLog("advertise-fast:"+b.tun.IP.String(), 30*time.Second) {
				fmt.Printf("[advertise] refresh ip=%s direct_addrs=%d relay_addrs=%d\n", b.tun.IP.String(), directAddrs, relayedAddrs)
			}
			b.lastAdvOK = true
			b.announce = true
			successes++
			if successes < fastAdvertiseCount {
				ticker.Reset(time.Second * 10) // 启动早期持续刷新，等待 AutoRelay 地址进入 DHT provider 记录
			} else {
				ticker.Reset(time.Minute * 5)
			}
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

func (b *Bridge) shouldLog(key string, interval time.Duration) bool {
	b.logMu.Lock()
	defer b.logMu.Unlock()

	now := time.Now()
	if last, ok := b.lastLog[key]; ok && now.Sub(last) < interval {
		return false
	}
	b.lastLog[key] = now
	return true
}
