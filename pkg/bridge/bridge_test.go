package bridge

import (
	"net"
	"testing"
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
