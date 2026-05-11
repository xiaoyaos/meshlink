package bridge

import (
	"net"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestIsNoisyLocalIPv4(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "valid virtual peer", ip: "10.224.155.55", want: false},
		{name: "subnet broadcast", ip: "10.255.255.255", want: true},
		{name: "network address", ip: "10.0.0.0", want: true},
		{name: "multicast", ip: "224.0.0.1", want: true},
		{name: "unspecified", ip: "0.0.0.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNoisyLocalIPv4(net.ParseIP(tt.ip)); got != tt.want {
				t.Fatalf("isNoisyLocalIPv4(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestParsePeerMetaAcceptsLegacyHello(t *testing.T) {
	id, err := peer.Decode("12D3KooWLA8iTGSSJoEoUyPexsLPWvanhcSVq6qW6pdmMaxF3tzh")
	if err != nil {
		t.Fatal(err)
	}

	var b Bridge
	meta, ok := b.parsePeerMeta([]string{"HELLO", "10.90.214.210", id.String()})
	if !ok {
		t.Fatal("parsePeerMeta returned !ok")
	}
	if meta.VIP != "10.90.214.210" || meta.ID != id || meta.OS != "" || meta.Hostname != "" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}

func TestParsePeerMetaAcceptsExtendedHello(t *testing.T) {
	id, err := peer.Decode("12D3KooWLA8iTGSSJoEoUyPexsLPWvanhcSVq6qW6pdmMaxF3tzh")
	if err != nil {
		t.Fatal(err)
	}

	var b Bridge
	meta, ok := b.parsePeerMeta([]string{"HELLO", "10.90.214.210", id.String(), "windows", "office-pc", "1.1.9"})
	if !ok {
		t.Fatal("parsePeerMeta returned !ok")
	}
	if meta.VIP != "10.90.214.210" || meta.ID != id || meta.OS != "windows" || meta.Hostname != "office-pc" || meta.Version != "1.1.9" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}
