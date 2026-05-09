package p2p

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func TestParseRelayCandidatesAcceptsShorthand(t *testing.T) {
	pid, err := peer.Decode("12D3KooWSfQy8Z3C9doqYmAyME2EukxVQyKkeU9TcssmnLRHHVq3")
	if err != nil {
		t.Fatalf("decode peer id: %v", err)
	}
	relays := parseRelayCandidates([]string{"47.110.232.246:4001:" + pid.String()})

	if len(relays) != 1 {
		t.Fatalf("expected 1 relay candidate, got %d", len(relays))
	}
	if relays[0].ID != pid {
		t.Fatalf("expected peer %s, got %s", pid, relays[0].ID)
	}
	if got := relays[0].Addrs[0].String(); got != "/ip4/47.110.232.246/tcp/4001" {
		t.Fatalf("unexpected relay addr: %s", got)
	}
}

func TestParseRelayCandidatesDeduplicatesPeer(t *testing.T) {
	pid := "12D3KooWSfQy8Z3C9doqYmAyME2EukxVQyKkeU9TcssmnLRHHVq3"
	relays := parseRelayCandidates([]string{
		"47.110.232.246:4001:" + pid,
		"/ip4/47.110.232.246/tcp/4001/p2p/" + pid,
	})

	if len(relays) != 1 {
		t.Fatalf("expected duplicate peer to be collapsed, got %d candidates", len(relays))
	}
}

func TestParsePeerAddrAcceptsMultiaddr(t *testing.T) {
	want := "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWSfQy8Z3C9doqYmAyME2EukxVQyKkeU9TcssmnLRHHVq3"
	got, err := parsePeerAddr(want)
	if err != nil {
		t.Fatalf("parsePeerAddr returned error: %v", err)
	}
	if got.String() != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestSplitAddrsClassifiesCircuitAddrs(t *testing.T) {
	directAddr := multiaddr.StringCast("/ip4/127.0.0.1/tcp/4001")
	relayAddr := multiaddr.StringCast("/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWSfQy8Z3C9doqYmAyME2EukxVQyKkeU9TcssmnLRHHVq3/p2p-circuit")

	direct, relayed := splitAddrs([]multiaddr.Multiaddr{directAddr, relayAddr})

	if len(direct) != 1 || direct[0].String() != directAddr.String() {
		t.Fatalf("unexpected direct addrs: %v", direct)
	}
	if len(relayed) != 1 || relayed[0].String() != relayAddr.String() {
		t.Fatalf("unexpected relayed addrs: %v", relayed)
	}
}
