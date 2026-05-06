package tun

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"

	"github.com/songgao/water"
	"golang.zx2c4.com/wireguard/tun"
)

// Interface 封装了 TUN 设备及其配置
type Interface struct {
	Name string
	IP   net.IP
	ifce *water.Interface
	wTun tun.Device // Windows 特有
}

// New 创建一个新的 TUN 设备并分配 IP
func New(ip net.IP) (*Interface, error) {
	if runtime.GOOS == "windows" {
		return newWindows(ip)
	}

	config := water.Config{
		DeviceType: water.TUN,
	}

	ifce, err := water.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN interface: %v", err)
	}

	itf := &Interface{
		Name: ifce.Name(),
		IP:   ip,
		ifce: ifce,
	}

	if err := itf.setup(); err != nil {
		ifce.Close()
		return nil, err
	}

	return itf, nil
}

func newWindows(ip net.IP) (*Interface, error) {
	// 在 Windows 上使用 wintun
	dev, err := tun.CreateTUN("p2p-mesh", 1380)
	if err != nil {
		return nil, fmt.Errorf("failed to create Wintun interface (is wintun.dll present?): %v", err)
	}

	name, _ := dev.Name()
	itf := &Interface{
		Name: name,
		IP:   ip,
		wTun: dev,
	}

	if err := itf.setup(); err != nil {
		dev.Close()
		return nil, err
	}

	return itf, nil
}

// setup 执行平台相关的网卡配置命令
func (itf *Interface) setup() error {
	var cmd *exec.Cmd
	ipStr := itf.IP.String()

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("ifconfig", itf.Name, ipStr, ipStr, "mtu", "1380", "up")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to ifconfig on darwin: %v", err)
		}
		cmd = exec.Command("route", "add", "-net", "10.0.0.0/8", "-interface", itf.Name)
	case "linux":
		cmd = exec.Command("ip", "link", "set", "dev", itf.Name, "mtu", "1380")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set MTU on linux: %v", err)
		}
		cmd = exec.Command("ip", "addr", "add", ipStr+"/8", "dev", itf.Name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set IP on linux: %v", err)
		}
		cmd = exec.Command("ip", "link", "set", "dev", itf.Name, "up")
	case "windows":
		// Windows 使用 netsh
		cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface", itf.Name, "mtu=1380", "store=active")
		if err := cmd.Run(); err != nil {
			// 如果失败可能是接口名不匹配或权限问题，继续尝试设置 IP
			fmt.Printf("[tun] warning: failed to set MTU on windows: %v\n", err)
		}
		cmd = exec.Command("netsh", "interface", "ip", "set", "address",
			"name="+itf.Name, "source=static", "addr="+ipStr, "mask=255.0.0.0", "gateway=none")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to setup interface (check root privileges): %v", err)
	}

	return nil
}

// Read 从 TUN 设备读取一个数据包
func (itf *Interface) Read(buf []byte) (int, error) {
	if itf.wTun != nil {
		// Wintun 的 Read 要求传入多维切片，第一个元素是存储包的 buffer，offset 是偏移量
		packets := [][]byte{buf}
		sizes := []int{0}
		n, err := itf.wTun.Read(packets, sizes, 0)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, nil
		}
		return sizes[0], nil
	}
	return itf.ifce.Read(buf)
}

// Write 向 TUN 设备写入一个数据包
func (itf *Interface) Write(buf []byte) (int, error) {
	if itf.wTun != nil {
		packets := [][]byte{buf}
		n, err := itf.wTun.Write(packets, 0)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	return itf.ifce.Write(buf)
}

// Close 关闭设备
func (itf *Interface) Close() error {
	if itf.wTun != nil {
		return itf.wTun.Close()
	}
	return itf.ifce.Close()
}
