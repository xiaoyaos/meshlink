package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	KeyFile = "node.key"
)

// LoadOrGenerateKey 从指定目录加载密钥，如果不存在则生成新密钥
func LoadOrGenerateKey(configDir string) (crypto.PrivKey, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	keyPath := filepath.Join(configDir, KeyFile)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		// 生成新密钥
		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}

		raw, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			return nil, err
		}

		if err := ioutil.WriteFile(keyPath, raw, 0600); err != nil {
			return nil, err
		}
		return priv, nil
	}

	// 加载现有密钥
	raw, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalPrivateKey(raw)
}

// DeriveVirtualIPv4 根据 PeerID (基于公钥) 推导出一个唯一的 10.x.x.x 地址
func DeriveVirtualIPv4(id peer.ID) net.IP {
	hash := sha256.Sum256([]byte(id))
	// 使用哈希的最后 3 个字节来填充 10.x.x.x 的后三位
	// 避免使用 10.0.0.0, 10.0.0.1 等常见网关地址，虽然哈希是随机的，但我们可以加上偏移或直接使用
	ip := net.IPv4(10, hash[0], hash[1], hash[2])
	return ip
}

// GetPeerInfo 返回节点的 PeerID 和对应的虚拟 IP
func GetPeerInfo(priv crypto.PrivKey) (peer.ID, net.IP, error) {
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return "", nil, err
	}
	return id, DeriveVirtualIPv4(id), nil
}
