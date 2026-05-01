package main

import (
	"fmt"
	"net"
	"p2p/pkg/tun"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func main() {
	ip := net.ParseIP("10.1.2.3")
	itf, err := tun.New(ip)
	if err != nil {
		panic(err)
	}
	defer itf.Close()
	fmt.Printf("Created TUN %s\n", itf.Name)

	buf := make([]byte, 2048)
	fmt.Println("Please ping 10.1.2.4 from another terminal on this Mac...")
	
	for {
		n, err := itf.Read(buf)
		if err != nil {
			panic(err)
		}
		data := buf[:n]
		fmt.Printf("Read %d bytes\n", n)
		
		packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)
		if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
			ipv4 := ipLayer.(*layers.IPv4)
			fmt.Printf("Parsed IPv4: Dst=%s\n", ipv4.DstIP)
		} else {
			fmt.Printf("FAILED to parse IPv4! First 4 bytes: %x %x %x %x\n", data[0], data[1], data[2], data[3])
		}
	}
}
