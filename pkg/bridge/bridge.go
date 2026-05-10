package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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
	Version   string              `json:"version"`
	NodeType  string              `json:"node_type"` // "中继节点" 或 "普通客户端"
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
	version   string // 软件版本
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

func New(t *tun.Interface, n *p2p.Node, configDir string, version string) *Bridge {
	b := &Bridge{
		tun:       t,
		node:      n,
		ctx:       n.Ctx,
		configDir: configDir,
		version:   version,
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

	// 当连接彻底断开时，清理路由映射
	n.PeerDisconnectedHandler = func(p peer.ID) {
		if b.node.Relay == nil {
			return
		}

		count := 0
		b.routeCache.Range(func(key, value any) bool {
			if value.(peer.ID) == p {
				vip := key.(string)
				b.routeCache.Delete(vip)
				count++
			}
			return true
		})
		if count > 0 {
			fmt.Printf("[控制] 节点已下线，成功清理 %d 条相关路由: %s\n", count, p)
		}
	}

	return b
}

func (b *Bridge) sendLocalVIP(target peer.ID) error {
	// 增加重试逻辑，确保在连接初期协议协商未完成时不会直接失败
	var lastErr error
	for i := 0; i < 3; i++ {
		s, err := b.newControlStream(target, 10*time.Second)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		// 格式: "HELLO:<IP>:<PeerID>"
		msg := fmt.Sprintf("HELLO:%s:%s", b.tun.IP.String(), b.node.Host.ID().String())
		_, err = s.Write([]byte(msg))
		s.Close()

		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return lastErr
}

func (b *Bridge) syncRegistryToPeer(target peer.ID) {
	// 异步延迟同步，等待链路稳定
	go func() {
		time.Sleep(2 * time.Second)
		b.routeCache.Range(func(key, value any) bool {
			vip := key.(string)
			owner := value.(peer.ID)
			if owner == target {
				return true
			}

			go func(v string, o peer.ID) {
				s, err := b.newControlStream(target, 5*time.Second)
				if err != nil {
					return
				}
				defer s.Close()
				msg := fmt.Sprintf("HELLO:%s:%s", v, o.String())
				_, _ = s.Write([]byte(msg))
			}(vip, owner)
			return true
		})
	}()
}

func (b *Bridge) handleControlStream(s network.Stream) {
	defer s.Close()
	remotePeer := s.Conn().RemotePeer()

	buf := make([]byte, 512)
	n, err := s.Read(buf)
	if err != nil {
		return
	}

	msg := string(buf[:n])
	parts := strings.Split(msg, ":")
	if len(parts) < 2 {
		return
	}

	cmd := parts[0]
	switch cmd {
	case "HELLO":
		// 收到对方宣告: HELLO:VIP:PeerID
		if len(parts) < 3 {
			return
		}
		vip, ownerIDStr := parts[1], parts[2]
		ownerID, err := peer.Decode(ownerIDStr)
		if err != nil {
			return
		}
		b.updateRoute(vip, ownerID)

		// 立即回复自己的信息 (WELCOME)
		resp := fmt.Sprintf("WELCOME:%s:%s", b.tun.IP.String(), b.node.Host.ID().String())
		_, _ = s.Write([]byte(resp))

		// 如果自己是服务器，额外同步其他人的信息
		if b.node.Relay != nil {
			go b.broadcastVIPInfo(vip, ownerID)
		}

	case "WELCOME":
		// 收到回应: WELCOME:VIP:PeerID
		if len(parts) < 3 {
			return
		}
		vip, ownerIDStr := parts[1], parts[2]
		ownerID, err := peer.Decode(ownerIDStr)
		if err != nil {
			return
		}
		b.updateRoute(vip, ownerID)

	case "PING":
		// 内置诊断协议: PING:请求ID
		if len(parts) < 2 {
			return
		}
		reqID := parts[1]
		resp := "PONG:" + reqID
		_, _ = s.Write([]byte(resp))

	case "PONG":
		// 收到 PONG，由 stats/test 命令处理逻辑捕获 (此处仅记录日志)
		if b.shouldLog("control:pong:"+remotePeer.String(), time.Second) {
			fmt.Printf("[诊断] 收到来自 %s 的响应\n", remotePeer)
		}
	}
}

func (b *Bridge) updateRoute(vip string, id peer.ID) {
	if b.shouldLog("route:learned:"+vip, time.Minute) {
		fmt.Printf("[控制] 映射成功: %s -> %s\n", vip, id)
	}
	b.routeCache.Store(vip, id)

	// 如果对方不是服务器，尝试打洞直连
	if b.node.Relay == nil && id != b.node.Host.ID() {
		go b.node.EnsureDirectConn(id)
	}
}

func (b *Bridge) broadcastVIPInfo(vip string, owner peer.ID) {
	peers := b.node.Host.Network().Peers()
	msg := fmt.Sprintf("HELLO:%s:%s", vip, owner.String())
	for _, p := range peers {
		if p == owner || p == b.node.Host.ID() {
			continue
		}
		go func(target peer.ID) {
			s, err := b.newControlStream(target, 5*time.Second)
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
		packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)
		if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
			ip, _ := ipLayer.(*layers.IPv4)
			dstIP := ip.DstIP.String()

			// 情况 A: 目标是本机 -> 写入 TUN
			if dstIP == b.tun.IP.String() {
				n, err := b.tun.Write(data)
				if err != nil {
					if b.shouldLog("tun:write-err", time.Second*5) {
						fmt.Printf("[网卡] 写入失败: %v\n", err)
					}
				} else if b.shouldLog("bridge:inbound", time.Second*10) {
					fmt.Printf("[网桥] 收到入站包: 长度=%d\n", n)
				}
				return
			}

			// 情况 B: 目标是别人 -> 如果自己是服务器，则执行中转
			if b.node.Relay != nil {
				if peerID, ok := b.routeCache.Load(dstIP); ok {
					targetID := peerID.(peer.ID)
					if b.shouldLog("bridge:forward:"+dstIP, time.Second*5) {
						fmt.Printf("[中转] 正在转发: 目标=%s 通过节点=%s\n", dstIP, targetID)
					}
					_ = b.node.SendPacket(targetID, data)
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

	// 5. 定期更新实时状态文件供 CLI 工具查看 (包含僵尸路由清理)
	go b.stateUpdateLoop()
}

func (b *Bridge) stateUpdateLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 1. 自动清理已断开连接但仍在 Cache 中的僵尸路由 (双重保险)
			b.pruneStaleRoutes()
			// 2. 写入状态文件
			b.writeStateFile()
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *Bridge) pruneStaleRoutes() {
	if b.node.Relay == nil {
		return
	}

	// 获取当前所有真实的物理连接
	activePeers := make(map[peer.ID]bool)
	for _, p := range b.node.Host.Network().Peers() {
		if len(b.node.Host.Network().ConnsToPeer(p)) > 0 {
			activePeers[p] = true
		}
	}

	b.routeCache.Range(func(key, value any) bool {
		vip := key.(string)
		ownerID := value.(peer.ID)

		// 如果该 Peer 已经不在连接列表中，且不是自己，则剔除
		if !activePeers[ownerID] && ownerID.String() != b.node.Host.ID().String() {
			if b.shouldLog("prune:"+vip, time.Minute) {
				fmt.Printf("[路由] 检测到离线僵尸路由，正在剔除: %s -> %s\n", vip, ownerID)
			}
			b.routeCache.Delete(vip)
		}
		return true
	})
}

func (b *Bridge) writeStateFile() {
	nodeType := "普通客户端"
	if b.node.Relay != nil {
		nodeType = "引导/中继节点"
	}

	state := GlobalState{
		Version:   b.version,
		NodeType:  nodeType,
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

		// 情况 A: 发给本机的包 (Local Loopback)
		if dstIP == b.tun.IP.String() {
			if b.shouldLog("bridge:loopback", time.Second*10) {
				fmt.Printf("[网桥] 收到发往本机的包，正在回环: %s\n", dstIP)
			}
			_, _ = b.tun.Write(data)
			return
		}

		if !strings.HasPrefix(dstIP, "10.") {
			return
		}
		if isNoisyLocalIPv4(ip.DstIP) {
			return
		}

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
			if cached, ok := b.routeCache.Load(dstIP); ok && cached.(peer.ID) == peerID {
				b.routeCache.Delete(dstIP)
			}
			if b.shouldLog("send:"+dstIP, 10*time.Second) {
				fmt.Printf("[隧道] 发送失败 目标=%s 节点=%s 错误=%v\n", dstIP, peerID, err)
			}
		} else if b.shouldLog("bridge:outbound:"+dstIP, time.Second*5) {
			fmt.Printf("[网桥] 已发包: 目标=%s 长度=%d\n", dstIP, len(data))
		}
	}
}

func isNoisyLocalIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return true
	}
	return ip4[0] == 0 || ip4[0] >= 224 || ip4[3] == 0 || ip4[3] == 255
}

func (b *Bridge) newControlStream(target peer.ID, timeout time.Duration) (network.Stream, error) {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "p2p-vpn control over relay fallback")
	return b.node.Host.NewStream(ctx, target, p2p.ControlProtocolID)
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
