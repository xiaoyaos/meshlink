package main

import (
	"fmt"
	"time"

	"p2p/pkg/identity"
	"p2p/pkg/p2p"

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
	fmt.Println("Starting Node A (Bootstrap)...")
	privA, _ := identity.LoadOrGenerateKey("./configA")
	idA, ipA, _ := identity.GetPeerInfo(privA)
	nodeA, err := p2p.NewNode(privA, "5001", false)
	if err != nil {
		panic(err)
	}
	defer nodeA.Close()

	fmt.Printf("Node A: ID=%s IP=%s\n", idA, ipA)
	addrA := fmt.Sprintf("%s/p2p/%s", "/ip4/127.0.0.1/tcp/5001", idA)

	fmt.Println("Starting Node B (Client)...")
	privB, _ := identity.LoadOrGenerateKey("./configB")
	idB, ipB, _ := identity.GetPeerInfo(privB)
	nodeB, err := p2p.NewNode(privB, "5002", false)
	if err != nil {
		panic(err)
	}
	defer nodeB.Close()

	fmt.Printf("Node B: ID=%s IP=%s\n", idB, ipB)

	// Connect Node B to Node A
	fmt.Println("Node B connecting to Node A...")
	if err := nodeB.Bootstrap([]string{addrA}); err != nil {
		panic(err)
	}

	// Node A advertises itself
	cA, _ := ipToCid(ipA.String())
	go func() {
		for {
			if err := nodeA.DHT.Provide(nodeA.Ctx, cA, true); err != nil {
				fmt.Printf("Node A failed to provide: %v\n", err)
				time.Sleep(2 * time.Second)
			} else {
				fmt.Println("Node A provided IP successfully!")
				break
			}
		}
	}()

	// Node B advertises itself
	cB, _ := ipToCid(ipB.String())
	go func() {
		for {
			if err := nodeB.DHT.Provide(nodeB.Ctx, cB, true); err != nil {
				fmt.Printf("Node B failed to provide: %v\n", err)
				time.Sleep(2 * time.Second)
			} else {
				fmt.Println("Node B provided IP successfully!")
				break
			}
		}
	}()

	time.Sleep(5 * time.Second)

	// Node B tries to resolve Node A's IP
	fmt.Printf("Node B resolving Node A's IP (%s)...\n", ipA)
	providers := nodeB.DHT.FindProvidersAsync(nodeB.Ctx, cA, 1)
	select {
	case p := <-providers:
		fmt.Printf("Node B resolved Node A: %s\n", p.ID)
	case <-time.After(5 * time.Second):
		fmt.Println("Node B failed to resolve Node A!")
	}

	// Node A tries to resolve Node B's IP
	fmt.Printf("Node A resolving Node B's IP (%s)...\n", ipB)
	providersA := nodeA.DHT.FindProvidersAsync(nodeA.Ctx, cB, 1)
	select {
	case p := <-providersA:
		fmt.Printf("Node A resolved Node B: %s\n", p.ID)
	case <-time.After(5 * time.Second):
		fmt.Println("Node A failed to resolve Node B!")
	}
}
