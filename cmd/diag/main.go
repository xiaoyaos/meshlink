package main

import (
	"context"
	"fmt"
	"log"
	"p2p/pkg/identity"
	"p2p/pkg/p2p"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

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

func main() {
	priv, _ := identity.LoadOrGenerateKey("./diag_config")
	node, err := p2p.NewNode(priv, "4005", false)
	if err != nil {
		log.Fatal(err)
	}
	defer node.Close()

	bootstrap := "/ip4/47.110.232.246/tcp/4001/p2p/12D3KooWSfQy8Z3C9doqYmAyME2EukxVQyKkeU9TcssmnLRHHVq3"
	if err := node.Bootstrap([]string{bootstrap}); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to bootstrap. Resolving 10.136.64.35...")

	targetIP := "10.136.64.35"
	c, _ := ipToCid(targetIP)
	
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	providers := node.DHT.FindProvidersAsync(ctx, c, 1)
	select {
	case p := <-providers:
		if p.ID != "" {
			fmt.Printf("SUCCESS: Resolved %s to PeerID %s\n", targetIP, p.ID)
		} else {
			fmt.Println("FAILED: Got empty provider")
		}
	case <-ctx.Done():
		fmt.Println("FAILED: Resolution timed out")
	}
}
