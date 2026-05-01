package identity

import (
	"os"
	"testing"
)

func TestLoadOrGenerateKey(t *testing.T) {
	tmpDir := "test_config"
	defer os.RemoveAll(tmpDir)

	priv, err := LoadOrGenerateKey(tmpDir)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	if priv == nil {
		t.Fatal("private key is nil")
	}

	// 再次加载
	priv2, err := LoadOrGenerateKey(tmpDir)
	if err != nil {
		t.Fatalf("failed to load key: %v", err)
	}

	raw1, _ := priv.Raw()
	raw2, _ := priv2.Raw()

	if string(raw1) != string(raw2) {
		t.Error("loaded key does not match generated key")
	}
}

func TestDeriveVirtualIPv4(t *testing.T) {
	tmpDir := "test_config_ip"
	defer os.RemoveAll(tmpDir)

	priv, _ := LoadOrGenerateKey(tmpDir)
	id, ip, err := GetPeerInfo(priv)
	if err != nil {
		t.Fatalf("failed to get peer info: %v", err)
	}

	t.Logf("PeerID: %s", id)
	t.Logf("Virtual IP: %s", ip)

	if ip.To4()[0] != 10 {
		t.Errorf("expected IP in 10.x.x.x range, got %s", ip)
	}
}
