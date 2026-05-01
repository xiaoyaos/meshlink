package app

import (
	"fmt"
	"log"
	"p2p/pkg/bridge"
	"p2p/pkg/identity"
	"p2p/pkg/p2p"
	"p2p/pkg/tun"
	"sync"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type State int

const (
	StateDisconnected State = iota
	StateConnecting
	StateConnected
	StateError
)

type VPNService struct {
	sync.Mutex
	state       State
	configDir   string
	port        string
	enableRelay bool
	
	priv      crypto.PrivKey
	peerID    peer.ID
	virtualIP string

	node   *p2p.Node
	itf    *tun.Interface
	bridge *bridge.Bridge

	OnLog    func(string)
	OnState  func(State)
}

func NewVPNService(configDir, port string) *VPNService {
	return &VPNService{
		configDir: configDir,
		port:      port,
		state:     StateDisconnected,
	}
}

func (s *VPNService) SetRelay(enable bool) {
	s.enableRelay = enable
}

func (s *VPNService) logf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if s.OnLog != nil {
		s.OnLog(msg)
	}
	log.Println(msg)
}

func (s *VPNService) setState(st State) {
	s.state = st
	if s.OnState != nil {
		s.OnState(st)
	}
}

func (s *VPNService) GetNode() *p2p.Node {
	return s.node
}

func (s *VPNService) GetInfo() (string, string) {
	if s.peerID == "" {
		priv, _ := identity.LoadOrGenerateKey(s.configDir)
		id, ip, _ := identity.GetPeerInfo(priv)
		s.peerID = id
		s.virtualIP = ip.String()
	}
	return s.peerID.String(), s.virtualIP
}

func (s *VPNService) Start(bootstrapAddr string) error {
	s.Lock()
	defer s.Unlock()

	if s.state != StateDisconnected {
		return fmt.Errorf("already running or connecting")
	}

	s.setState(StateConnecting)
	s.logf("Starting P2P VPN service...")

	// 1. Identity
	priv, err := identity.LoadOrGenerateKey(s.configDir)
	if err != nil {
		s.setState(StateError)
		return err
	}
	s.priv = priv
	id, ip, _ := identity.GetPeerInfo(priv)
	s.peerID = id
	s.virtualIP = ip.String()

	// 2. Node
	node, err := p2p.NewNode(priv, s.port, s.enableRelay)
	if err != nil {
		s.setState(StateError)
		return err
	}
	s.node = node

	// 3. Bootstrap
	if bootstrapAddr != "" {
		s.logf("Connecting to bootstrap: %s", bootstrapAddr)
		go func() {
			if err := node.Bootstrap([]string{bootstrapAddr}); err != nil {
				s.logf("Bootstrap warning: %v", err)
			}
		}()
	}

	// 4. TUN
	itf, err := tun.New(ip)
	if err != nil {
		node.Close()
		s.setState(StateError)
		return fmt.Errorf("TUN error (need root?): %v", err)
	}
	s.itf = itf

	// 5. Bridge
	s.bridge = bridge.New(itf, node)
	s.bridge.Start()

	s.setState(StateConnected)
	s.logf("VPN Connected. Virtual IP: %s", s.virtualIP)
	return nil
}

func (s *VPNService) Stop() {
	s.Lock()
	defer s.Unlock()

	if s.node != nil {
		s.node.Close()
	}
	if s.itf != nil {
		s.itf.Close()
	}
	s.setState(StateDisconnected)
	s.logf("VPN Stopped.")
}
