package app

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"p2p/pkg/identity"
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

	cmd *exec.Cmd

	OnLog   func(string)
	OnState func(State)
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

func (s *VPNService) GetInfo() (string, string) {
	if s.peerID == "" {
		priv, _ := identity.LoadOrGenerateKey(s.configDir)
		id, ip, _ := identity.GetPeerInfo(priv)
		s.peerID = id
		s.virtualIP = ip.String()
	}
	return s.peerID.String(), s.virtualIP
}

// findNodeBinary locates the p2p-node CLI executable
func findNodeBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)

	var binName string
	if runtime.GOOS == "windows" {
		binName = "p2p-node-windows-amd64.exe"
	} else if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			binName = "p2p-node-darwin-arm64"
		} else {
			binName = "p2p-node-darwin-amd64"
		}
	} else {
		binName = "p2p-node-linux-" + runtime.GOARCH
	}

	// 1. 同级目录寻找
	path := filepath.Join(dir, binName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// 2. macOS 寻找 Contents/MacOS
	if runtime.GOOS == "darwin" {
		// e.g. Contents/MacOS/p2p-desktop -> path is the same dir, handled above
		// Or if we bundle it as just p2p-node
		path = filepath.Join(dir, "p2p-node")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// 3. 开发环境寻找 ../../release/cli
	path = filepath.Join(dir, "..", "..", "release", "cli", binName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// 4. 当前工作目录下的 release/cli
	cwd, _ := os.Getwd()
	path = filepath.Join(cwd, "release", "cli", binName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("could not find p2p-node binary (%s) in any expected path", binName)
}

func (s *VPNService) Start(bootstrapAddr string) error {
	s.Lock()
	defer s.Unlock()

	if s.state != StateDisconnected {
		return fmt.Errorf("already running or connecting")
	}

	s.setState(StateConnecting)
	s.logf("Starting P2P VPN service daemon...")

	s.GetInfo() // Ensure config and keys exist

	binPath, err := findNodeBinary()
	if err != nil {
		s.setState(StateError)
		return err
	}

	args := []string{"-port", s.port, "-config", s.configDir}
	if bootstrapAddr != "" {
		args = append(args, "-bootstrap", bootstrapAddr)
	}

	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		// Windows: 需要提权启动 CLI 以创建 TUN 网卡
		// 因为 GUI 目前是非管理员运行，我们通过 PowerShell 的 RunAs 动词来启动 CLI。
		// 为了捕获输出，我们将 CLI 的输出重定向到临时文件，由 GUI 轮询读取。
		logPath := filepath.Join(s.configDir, "daemon.log")
		os.Remove(logPath) // 清理旧日志

		// 使用 CLI 新增的 -logfile 参数，这样就不需要复杂的 shell 重定向了
		winArgs := append(args, "-logfile", logPath)
		cliArgs := strings.Join(winArgs, " ")
		
		s.logf("Windows: 正在尝试以管理员权限启动 P2P 核心...")
		// 使用单引号包裹路径以处理空格
		powershellCmd := fmt.Sprintf("Start-Process '%s' -ArgumentList '%s' -Verb RunAs -WindowStyle Hidden", binPath, cliArgs)
		cmd = exec.Command("powershell", "-Command", powershellCmd)
		
		if err := cmd.Run(); err != nil {
			s.setState(StateError)
			return fmt.Errorf("无法发起提权请求 (用户可能点击了取消): %v", err)
		}

		// 启动一个虚拟进程用于维持生命周期管理
		s.cmd = exec.Command("cmd", "/c", "echo elevated") 
		go s.tailWindowsLog(logPath)
		return nil
	} else if runtime.GOOS == "darwin" {
		// macOS: use osascript to elevate ONLY the CLI and capture stdout
		cliCmd := fmt.Sprintf("'%s' %s", binPath, strings.Join(args, " "))
		script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, cliCmd)
		cmd = exec.Command("osascript", "-e", script)
	} else {
		// Linux: use pkexec
		cmd = exec.Command("pkexec", append([]string{binPath}, args...)...)
	}

	// Capture output
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		s.setState(StateError)
		return fmt.Errorf("failed to start daemon: %v", err)
	}

	s.cmd = cmd

	// Async output reader and state manager
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			s.logf("[Daemon] %s", line)

			if strings.Contains(line, "[STATE] CONNECTED") || strings.Contains(line, "Network is ready") {
				if s.state != StateConnected {
					s.setState(StateConnected)
					s.logf("VPN Connected successfully.")
				}
			} else if strings.Contains(line, "[STATE] ERROR") || strings.Contains(line, "FAILED to connect") || strings.Contains(line, "bootstrap failed") {
				s.setState(StateError)
				s.logf("VPN Connection Error!")
			}
		}

		cmd.Wait()
		s.Lock()
		s.cmd = nil
		s.setState(StateDisconnected)
		s.Unlock()
		s.logf("Daemon process exited.")
	}()

	return nil
}

func (s *VPNService) Stop() {
	s.Lock()
	defer s.Unlock()

	if s.cmd != nil {
		if runtime.GOOS == "windows" {
			// Windows: 因为是通过 Start-Process 提权启动的，s.cmd 只是个占位符
			// 我们需要按名称杀掉真正的进程
			exec.Command("taskkill", "/F", "/IM", "p2p-node-windows-amd64.exe").Run()
			s.cmd.Process.Kill()
		} else if runtime.GOOS == "darwin" {
			// osascript runs child as root, so killing osascript might not kill the child
			// We kill by binary name for safety
			exec.Command("sudo", "killall", filepath.Base(s.cmd.Path)).Run()
			s.cmd.Process.Kill()
		} else {
			exec.Command("sudo", "killall", filepath.Base(s.cmd.Path)).Run()
			s.cmd.Process.Kill()
		}
		s.cmd = nil
	}
	s.setState(StateDisconnected)
	s.logf("VPN Stopped.")
}

// tailWindowsLog 专门用于 Windows 下读取重定向到文件的日志
func (s *VPNService) tailWindowsLog(path string) {
	s.logf("开始轮询读取日志文件: %s", path)
	// 等待文件创建
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(path); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	file, err := os.Open(path)
	if err != nil {
		s.logf("无法打开日志文件: %v", err)
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// 如果读到末尾，稍等一下继续读
			time.Sleep(500 * time.Millisecond)
			s.Lock()
			running := s.state != StateDisconnected
			s.Unlock()
			if !running {
				break
			}
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		s.logf("[Daemon] %s", line)

		if strings.Contains(line, "[STATE] CONNECTED") || strings.Contains(line, "Network is ready") {
			if s.state != StateConnected {
				s.setState(StateConnected)
				s.logf("VPN 连接成功 (Windows 提权模式).")
			}
		} else if strings.Contains(line, "[STATE] ERROR") || strings.Contains(line, "FAILED") {
			s.setState(StateError)
		}
	}
}
