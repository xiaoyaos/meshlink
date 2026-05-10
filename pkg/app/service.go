package app

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"p2p/pkg/identity"
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

	cmd        *exec.Cmd
	daemonName string

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
	msg := sanitizeLogMessage(fmt.Sprintf(format, v...))
	if msg == "" {
		return
	}
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

func (s *VPNService) GetInfo() (string, string, error) {
	if s.peerID != "" && s.virtualIP != "" {
		return s.peerID.String(), s.virtualIP, nil
	}

	priv, err := identity.LoadOrGenerateKey(s.configDir)
	if err != nil {
		return "", "", err
	}

	id, ip, err := identity.GetPeerInfo(priv)
	if err != nil {
		return "", "", err
	}

	s.priv = priv
	s.peerID = id
	s.virtualIP = ip.String()
	return s.peerID.String(), s.virtualIP, nil
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

	// 3. 开发环境寻找 ../../dist/bin
	path = filepath.Join(dir, "..", "..", "dist", "bin", binName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// 4. 当前工作目录下的 dist/bin
	cwd, _ := os.Getwd()
	path = filepath.Join(cwd, "dist", "bin", binName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("could not find p2p-node binary (%s) in any expected path", binName)
}

func (s *VPNService) Start(bootstrapAddr string) error {
	s.Lock()
	defer s.Unlock()

	if s.state == StateConnecting || s.state == StateConnected {
		return fmt.Errorf("already running or connecting")
	}
	s.cmd = nil
	s.daemonName = ""

	s.setState(StateConnecting)
	s.logf("正在启动 P2P VPN 服务...")

	if _, _, err := s.GetInfo(); err != nil {
		s.setState(StateError)
		return fmt.Errorf("failed to load node identity: %v", err)
	}

	if err := os.MkdirAll(s.configDir, 0700); err != nil {
		s.setState(StateError)
		return fmt.Errorf("failed to create config dir: %v", err)
	}

	binPath, err := findNodeBinary()
	if err != nil {
		s.setState(StateError)
		return err
	}
	s.daemonName = filepath.Base(binPath)

	args := []string{"-port", s.port, "-config", s.configDir, "-parent-pid", fmt.Sprintf("%d", os.Getpid())}
	if s.enableRelay {
		args = append(args, "-relay")
	}
	if bootstrapAddr != "" {
		args = append(args, "-bootstrap", bootstrapAddr)
	}

	var cmd *exec.Cmd
	logPath := filepath.Join(s.configDir, "daemon.log")
	startupLogPath := filepath.Join(s.configDir, "daemon-startup.log")

	if runtime.GOOS == "windows" {
		// Windows: 需要提权启动 CLI 以创建 TUN 网卡
		// 因为 GUI 目前是非管理员运行，我们通过 PowerShell 的 RunAs 动词来启动 CLI。
		// 为了捕获输出，我们将 CLI 的输出重定向到临时文件，由 GUI 轮询读取。
		if err := resetLogFile(logPath); err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to prepare daemon log: %v", err)
		}
		if err := resetLogFile(startupLogPath); err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to prepare startup log: %v", err)
		}

		// 使用 CLI 新增的 -logfile 参数，这样就不需要复杂的 shell 重定向了
		winArgs := append(args, "-logfile", logPath)
		launcherPath := filepath.Join(s.configDir, "launch-daemon.cmd")
		if err := writeWindowsLauncher(launcherPath, startupLogPath, binPath, winArgs); err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to write daemon launcher: %v", err)
		}

		s.logf("Windows: 正在尝试以管理员权限启动 P2P 核心...")
		powershellCmd := "Start-Process 'cmd.exe' -ArgumentList " + powershellSingleQuote("/c "+windowsBatchQuote(launcherPath)) + " -Verb RunAs -WindowStyle Hidden"
		cmd = exec.Command("powershell", "-Command", powershellCmd)

		if err := cmd.Run(); err != nil {
			s.setState(StateError)
			return fmt.Errorf("无法发起提权请求 (用户可能点击了取消): %v", err)
		}

		// 启动一个虚拟进程用于维持生命周期管理
		s.cmd = exec.Command(binPath)
		go s.tailDaemonLog(logPath)
		go s.tailStartupLog(startupLogPath)
		return nil
	} else if runtime.GOOS == "darwin" {
		if err := resetLogFile(logPath); err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to prepare daemon log: %v", err)
		}
		if err := resetLogFile(startupLogPath); err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to prepare startup log: %v", err)
		}

		daemonArgs := append(args, "-logfile", logPath)
		launcherPath := filepath.Join(s.configDir, "launch-daemon.sh")
		if err := writeUnixLauncher(launcherPath, startupLogPath, logPath, binPath, daemonArgs); err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to write daemon launcher: %v", err)
		}

		cliCmd := "nohup /bin/sh " + shellQuote(launcherPath) + " >/dev/null 2>&1 &"
		script := fmt.Sprintf("do shell script %q with administrator privileges", cliCmd)

		s.logf("macOS: 正在请求管理员权限启动 P2P 核心...")
		cmd = exec.Command("osascript", "-e", script)
		if err := cmd.Run(); err != nil {
			s.setState(StateError)
			return fmt.Errorf("无法发起提权请求 (用户可能点击了取消): %v", err)
		}

		s.cmd = exec.Command(binPath)
		go s.tailDaemonLog(logPath)
		go s.tailStartupLog(startupLogPath)
		return nil
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
		scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			s.handleDaemonLogLine(line)
		}

		cmd.Wait()
		s.Lock()
		s.cmd = nil
		s.daemonName = ""
		s.setState(StateDisconnected)
		s.Unlock()
		s.logf("VPN 进程已退出。")
	}()

	return nil
}

func (s *VPNService) Stop() {
	s.Lock()
	defer s.Unlock()

	daemonName := s.daemonName
	if daemonName == "" && s.cmd != nil {
		daemonName = filepath.Base(s.cmd.Path)
	}

	if daemonName != "" {
		if runtime.GOOS == "windows" {
			// Windows: 因为是通过 Start-Process 提权启动的，s.cmd 只是个占位符。
			exec.Command("taskkill", "/F", "/IM", daemonName).Run()
		} else if runtime.GOOS == "darwin" {
			killCmd := "killall " + shellQuote(daemonName)
			script := fmt.Sprintf("do shell script %q with administrator privileges", killCmd)
			exec.Command("osascript", "-e", script).Run()
		} else {
			exec.Command("sudo", "killall", daemonName).Run()
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	s.cmd = nil
	s.daemonName = ""
	s.setState(StateDisconnected)
	s.logf("VPN 已停止。")
}

// tailDaemonLog 读取提权后的守护进程日志，避免桌面端直接接管长期 stdout。
func (s *VPNService) tailDaemonLog(path string) {
	s.logf("日志文件: %s", path)
	// 等待文件创建
	for i := 0; i < 60; i++ {
		if _, err := os.Stat(path); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	file, err := os.Open(path)
	if err != nil {
		s.logf("无法打开日志文件: %v", err)
		s.setState(StateError)
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			s.handleDaemonLogLine(line)
		}

		if err != nil {
			time.Sleep(500 * time.Millisecond)
			s.Lock()
			running := s.state == StateConnecting || s.state == StateConnected
			s.Unlock()
			if !running {
				break
			}
		}
	}
}

func (s *VPNService) tailStartupLog(path string) {
	s.logf("启动日志: %s", path)
	for i := 0; i < 60; i++ {
		if _, err := os.Stat(path); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	file, err := os.Open(path)
	if err != nil {
		s.logf("无法打开启动日志: %v", err)
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			s.logf("[launcher] %s", line)
			if code, ok := launcherExitCode(line); ok && code != 0 {
				s.Lock()
				running := s.state == StateConnecting || s.state == StateConnected
				s.Unlock()
				if running {
					s.setState(StateError)
					s.logf("P2P 核心提前退出，退出码=%d。", code)
				}
			}
		}

		if err != nil {
			time.Sleep(500 * time.Millisecond)
			s.Lock()
			running := s.state == StateConnecting || s.state == StateConnected
			s.Unlock()
			if !running {
				break
			}
		}
	}
}

func (s *VPNService) handleDaemonLogLine(line string) {
	s.logf("[daemon] %s", line)

	if isDaemonReady(line) {
		if s.state != StateConnected {
			s.setState(StateConnected)
			s.logf("VPN 已连接。")
		}
		return
	}

	if isDaemonFatal(line) {
		s.setState(StateError)
		s.logf("VPN 启动失败，请检查权限、端口和引导节点。")
	}
}

func isDaemonReady(line string) bool {
	return strings.Contains(line, "[ready]") ||
		strings.Contains(line, "[就绪]") ||
		strings.Contains(line, "[STATE] CONNECTED") ||
		strings.Contains(line, "Network is ready")
}

func isDaemonFatal(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(line, "[fatal]") ||
		strings.Contains(line, "[STATE] ERROR") ||
		strings.Contains(line, "致命错误") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "bootstrap failed") ||
		strings.Contains(lower, "failed to load identity") ||
		strings.Contains(lower, "failed to get peer info")
}

func shellCommand(binPath string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(binPath))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func resetLogFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

func writeUnixLauncher(path, startupLogPath, daemonLogPath, binPath string, args []string) error {
	content := "#!/bin/sh\n" +
		"umask 022\n" +
		"{\n" +
		"echo \"[startup] time=$(date '+%Y-%m-%d %H:%M:%S')\"\n" +
		"echo " + shellQuote("[startup] binary="+binPath) + "\n" +
		"echo " + shellQuote("[startup] logfile="+daemonLogPath) + "\n" +
		"if [ ! -x " + shellQuote(binPath) + " ]; then\n" +
		"  echo " + shellQuote("[startup] binary is not executable") + "\n" +
		"  exit 126\n" +
		"fi\n" +
		shellCommand(binPath, args) + "\n" +
		"code=$?\n" +
		"echo \"[startup] exit_code=${code}\"\n" +
		"exit ${code}\n" +
		"} >> " + shellQuote(startupLogPath) + " 2>&1\n"
	return os.WriteFile(path, []byte(content), 0755)
}

func writeWindowsLauncher(path, startupLogPath, binPath string, args []string) error {
	logPath := windowsBatchQuote(startupLogPath)
	content := "@echo off\r\n" +
		"echo [startup] time=%DATE% %TIME%>> " + logPath + "\r\n" +
		"echo [startup] binary=" + binPath + ">> " + logPath + "\r\n" +
		"if not exist " + windowsBatchQuote(binPath) + " (\r\n" +
		"  echo [startup] binary missing>> " + logPath + "\r\n" +
		"  exit /b 126\r\n" +
		")\r\n" +
		windowsBatchQuote(binPath) + " " + windowsArgumentList(args) + " >> " + logPath + " 2>&1\r\n" +
		"set code=%ERRORLEVEL%\r\n" +
		"echo [startup] exit_code=%code%>> " + logPath + "\r\n" +
		"exit /b %code%\r\n"
	return os.WriteFile(path, []byte(content), 0644)
}

func windowsArgumentList(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, windowsArgQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func windowsArgQuote(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func windowsBatchQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func launcherExitCode(line string) (int, bool) {
	const marker = "exit_code="
	idx := strings.LastIndex(line, marker)
	if idx < 0 {
		return 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(line[idx+len(marker):]))
	if err != nil {
		return 0, false
	}
	return code, true
}

func sanitizeLogMessage(msg string) string {
	const maxRunes = 2000

	var b strings.Builder
	b.Grow(len(msg))
	written := 0
	truncated := false

	for _, r := range msg {
		if written >= maxRunes {
			truncated = true
			break
		}

		switch {
		case r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			continue
		default:
			b.WriteRune(r)
		}
		written++
	}

	out := strings.TrimSpace(b.String())
	if truncated {
		out += " ...[truncated]"
	}
	return out
}
