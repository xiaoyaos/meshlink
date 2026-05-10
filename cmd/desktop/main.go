package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var Version = "dev"

const (
	modeClient   = "客户端"
	modeRelay    = "引导/中继"
	logTailLines = 200
)

type desktopApp struct {
	window fyne.Window
	ctrl   systemController

	mode      *widget.RadioGroup
	port      *widget.Entry
	bootstrap *widget.Entry

	installButton  *widget.Button
	cancelButton   *widget.Button
	startButton    *widget.Button
	stopButton     *widget.Button
	restartButton  *widget.Button
	refreshButton  *widget.Button
	clearLogButton *widget.Button

	installed    *widget.Label
	status       *widget.Label
	nodeType     *widget.Label
	vip          *widget.Entry
	peerID       *widget.Entry
	config       *widget.Entry
	installConf  *widget.Entry
	logFile      *widget.Entry
	logs         *widget.RichText
	reinstalling bool
	installedNow bool

	logMu              sync.Mutex
	logLines           []string
	serviceLogMu       sync.Mutex
	serviceLogSnapshot []string
	lastServiceLogRead string
}

type meshState struct {
	Version   string              `json:"version"`
	NodeType  string              `json:"node_type"`
	SelfVIP   string              `json:"self_vip"`
	SelfID    string              `json:"self_id"`
	Peers     map[string]peerInfo `json:"peers"`
	UpdatedAt string              `json:"updated_at"`
}

type peerInfo struct {
	VIP      string `json:"vip"`
	ID       string `json:"id"`
	Direct   bool   `json:"direct"`
	LastSeen string `json:"last_seen"`
}

type installStatus struct {
	Installed bool
	Running   bool
	ConfigDir string
	DataDir   string
	StateFile string
	LogFile   string
	Settings  *serviceSettings
	Message   string
	State     *meshState
}

type serviceSettings struct {
	Port      string
	Relay     bool
	Bootstrap string
	Summary   string
}

type systemController interface {
	Name() string
	ConfigDir() string
	DataDir() string
	StateFile() string
	LogFile() string
	IsInstalled() bool
	IsRunning() bool
	Install(port, bootstrap string, relay bool) error
	Start() error
	Stop() error
	Restart() error
	Status() installStatus
	ReadRecentLogs(lines int) string
}

func main() {
	a := app.NewWithID("com.meshlink.desktop")
	w := a.NewWindow("MeshLink Desktop")

	d := &desktopApp{
		window: w,
		ctrl:   newSystemController(),
	}
	d.build()

	w.Resize(fyne.NewSize(960, 660))
	w.SetContent(d.content())
	d.appendLog("MeshLink Desktop %s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH)
	d.refresh()
	d.startLogWatcher()
	w.ShowAndRun()
}

func (d *desktopApp) build() {
	d.mode = widget.NewRadioGroup([]string{modeClient, modeRelay}, func(string) {
		d.updateBootstrapState()
	})
	d.mode.Horizontal = true

	d.port = widget.NewEntry()
	d.port.SetText("4001")

	d.bootstrap = widget.NewEntry()
	d.bootstrap.SetPlaceHolder("IP:端口:PeerID 或 /ip4/.../p2p/...")
	d.mode.SetSelected(modeClient)

	d.installed = widget.NewLabel("检测中")
	d.status = widget.NewLabel("未知")
	d.nodeType = widget.NewLabel("-")
	d.vip = readonlyEntry()
	d.peerID = readonlyEntry()
	d.config = readonlyEntry()
	d.installConf = readonlyEntry()
	d.logFile = readonlyEntry()
	d.logs = widget.NewRichText(logTextSegment(""))
	d.logs.Scroll = fyne.ScrollBoth
	d.logs.Wrapping = fyne.TextWrapWord

	d.installButton = widget.NewButtonWithIcon("安装", theme.DownloadIcon(), func() {
		d.installButtonPressed()
	})
	d.installButton.Importance = widget.HighImportance
	d.cancelButton = widget.NewButtonWithIcon("取消重装", theme.CancelIcon(), func() {
		d.reinstalling = false
		d.refresh()
		d.appendLog("已取消重新安装")
	})
	d.cancelButton.Hide()

	d.startButton = widget.NewButtonWithIcon("启动", theme.MediaPlayIcon(), func() {
		d.runAction("启动", d.ctrl.Start)
	})
	d.stopButton = widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), func() {
		d.runAction("停止", d.ctrl.Stop)
	})
	d.stopButton.Importance = widget.DangerImportance
	d.restartButton = widget.NewButtonWithIcon("重启", theme.ViewRefreshIcon(), func() {
		d.runAction("重启", d.ctrl.Restart)
	})
	d.refreshButton = widget.NewButtonWithIcon("刷新", theme.ViewRefreshIcon(), func() {
		d.refresh()
	})
	d.clearLogButton = widget.NewButtonWithIcon("清空", theme.ContentClearIcon(), func() {
		d.clearLogs()
	})

	d.updateBootstrapState()
}

func (d *desktopApp) content() fyne.CanvasObject {
	installForm := widget.NewForm(
		widget.NewFormItem("模式", d.mode),
		widget.NewFormItem("监听端口", d.port),
		widget.NewFormItem("引导节点", d.bootstrap),
	)
	installControls := container.NewHBox(d.installButton, d.cancelButton, d.startButton, d.stopButton, d.restartButton, d.refreshButton)

	info := widget.NewForm(
		widget.NewFormItem("安装状态", d.installed),
		widget.NewFormItem("运行状态", d.status),
		widget.NewFormItem("节点类型", d.nodeType),
		widget.NewFormItem("虚拟 IP", d.vip),
		widget.NewFormItem("Peer ID", d.peerID),
		widget.NewFormItem("配置目录", d.config),
		widget.NewFormItem("安装配置", d.installConf),
		widget.NewFormItem("日志文件", d.logFile),
	)

	copyPeer := widget.NewButtonWithIcon("复制 Peer ID", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(d.peerID.Text)
	})
	copyVIP := widget.NewButtonWithIcon("复制虚拟 IP", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(d.vip.Text)
	})

	left := container.NewVBox(
		widget.NewLabelWithStyle("安装与服务", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		installForm,
		installControls,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("节点信息", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		info,
		container.NewGridWithColumns(2, copyPeer, copyVIP),
	)

	logHeader := container.NewHBox(
		widget.NewLabelWithStyle("服务日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		d.clearLogButton,
	)

	right := container.NewBorder(
		logHeader,
		nil,
		nil,
		nil,
		d.logs,
	)

	split := container.NewHSplit(container.NewPadded(left), container.NewPadded(right))
	split.SetOffset(0.43)
	return split
}

func (d *desktopApp) installButtonPressed() {
	st := d.ctrl.Status()
	if st.Installed && !d.reinstalling {
		if st.Settings != nil {
			d.applyServiceSettings(st.Settings)
		}
		d.reinstalling = true
		d.refresh()
		d.appendLog("已进入重新安装模式，可以修改监听端口和引导节点")
		return
	}
	d.install()
}

func (d *desktopApp) install() {
	port := strings.TrimSpace(d.port.Text)
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		d.appendLog("监听端口无效: %s", port)
		return
	}

	relay := d.mode.Selected == modeRelay
	bootstrap := strings.TrimSpace(d.bootstrap.Text)
	if relay {
		bootstrap = ""
	}
	if !relay && bootstrap == "" {
		d.appendLog("客户端模式需要填写引导节点")
		return
	}

	d.setBusy(true)
	d.appendLog("开始安装 MeshLink 服务: 模式=%s 端口=%s", d.mode.Selected, port)
	go func() {
		err := d.ctrl.Install(port, bootstrap, relay)
		fyne.Do(func() {
			d.setBusy(false)
			if err != nil {
				d.appendLog("安装失败: %v", err)
			} else {
				d.reinstalling = false
				d.appendLog("安装完成")
			}
			d.refresh()
		})
	}()
}

func (d *desktopApp) runAction(name string, fn func() error) {
	d.setBusy(true)
	d.appendLog("%s MeshLink 服务...", name)
	go func() {
		err := fn()
		fyne.Do(func() {
			d.setBusy(false)
			if err != nil {
				d.appendLog("%s失败: %v", name, err)
			} else {
				d.appendLog("%s完成", name)
			}
			d.refresh()
		})
	}()
}

func (d *desktopApp) refresh() {
	st := d.ctrl.Status()
	d.installedNow = st.Installed
	if !st.Installed {
		d.reinstalling = false
	}
	d.installed.SetText(installedText(st.Installed))
	d.status.SetText(runningText(st.Running))
	d.config.SetText(st.ConfigDir)
	d.logFile.SetText(defaultText(st.LogFile))
	if st.Settings != nil {
		d.installConf.SetText(defaultText(st.Settings.Summary))
		if !d.reinstalling {
			d.applyServiceSettings(st.Settings)
		}
	} else {
		d.installConf.SetText("-")
	}

	if st.State != nil {
		d.nodeType.SetText(defaultText(st.State.NodeType))
		d.vip.SetText(defaultText(st.State.SelfVIP))
		d.peerID.SetText(defaultText(st.State.SelfID))
	} else {
		d.nodeType.SetText("-")
		d.vip.SetText("")
		d.peerID.SetText("")
	}

	d.updateControls(st)

	d.appendNewServiceLogs(d.ctrl.ReadRecentLogs(logTailLines))
	if st.Message != "" {
		d.appendLog("%s", st.Message)
	}
}

func (d *desktopApp) setBusy(busy bool) {
	if busy {
		d.installButton.Disable()
		d.cancelButton.Disable()
		d.startButton.Disable()
		d.stopButton.Disable()
		d.restartButton.Disable()
		d.refreshButton.Disable()
		d.updateInstallInputs(false)
		return
	}
	d.refreshButton.Enable()
}

func (d *desktopApp) updateControls(st installStatus) {
	d.installButton.Enable()
	d.cancelButton.Enable()

	switch {
	case st.Installed && d.reinstalling:
		d.installButton.SetText("确认重装")
		d.cancelButton.Show()
	case st.Installed:
		d.installButton.SetText("重新安装")
		d.cancelButton.Hide()
	default:
		d.installButton.SetText("安装")
		d.cancelButton.Hide()
	}

	if st.Installed && !d.reinstalling {
		d.startButton.Enable()
		d.stopButton.Enable()
		d.restartButton.Enable()
	} else {
		d.startButton.Disable()
		d.stopButton.Disable()
		d.restartButton.Disable()
	}

	d.updateInstallInputs(!st.Installed || d.reinstalling)
}

func (d *desktopApp) applyServiceSettings(settings *serviceSettings) {
	if settings == nil {
		return
	}
	if settings.Relay {
		d.mode.SetSelected(modeRelay)
		d.bootstrap.SetText("")
	} else {
		d.mode.SetSelected(modeClient)
		d.bootstrap.SetText(settings.Bootstrap)
	}
	if strings.TrimSpace(settings.Port) != "" {
		d.port.SetText(settings.Port)
	}
}

func (d *desktopApp) updateBootstrapState() {
	d.updateInstallInputs(!d.installedNow || d.reinstalling)
}

func (d *desktopApp) updateInstallInputs(editable bool) {
	if !editable {
		d.mode.Disable()
		d.port.Disable()
		d.bootstrap.Disable()
		return
	}
	d.mode.Enable()
	d.port.Enable()
	if d.mode.Selected == modeRelay {
		d.bootstrap.SetText("")
		d.bootstrap.Disable()
		return
	}
	d.bootstrap.Enable()
}

func (d *desktopApp) appendLog(format string, args ...interface{}) {
	line := strings.TrimSpace(fmt.Sprintf(format, args...))
	if line == "" {
		return
	}
	d.appendLogLines([]string{fmt.Sprintf("%s  %s", time.Now().Format("15:04:05"), line)})
}

func (d *desktopApp) appendLogLines(lines []string) {
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		clean = append(clean, line)
	}
	if len(clean) == 0 {
		return
	}
	d.logMu.Lock()
	d.logLines = append(d.logLines, clean...)
	if len(d.logLines) > 500 {
		d.logLines = d.logLines[len(d.logLines)-500:]
	}
	text := strings.Join(d.logLines, "\n")
	d.logMu.Unlock()

	d.setLogViewText(text)
}

func (d *desktopApp) clearLogs() {
	d.logMu.Lock()
	d.logLines = nil
	d.logMu.Unlock()

	d.serviceLogMu.Lock()
	snapshotText := d.ctrl.ReadRecentLogs(logTailLines)
	d.lastServiceLogRead = snapshotText
	d.serviceLogSnapshot = normalizeLogLines(snapshotText)
	d.serviceLogMu.Unlock()

	d.setLogViewText("")
}

func (d *desktopApp) appendNewServiceLogs(text string) {
	if text == "" {
		return
	}
	current := normalizeLogLines(text)
	d.serviceLogMu.Lock()
	if text == d.lastServiceLogRead {
		d.serviceLogMu.Unlock()
		return
	}
	added := diffLogLines(d.serviceLogSnapshot, current)
	d.serviceLogSnapshot = current
	d.lastServiceLogRead = text
	d.serviceLogMu.Unlock()
	d.appendLogLines(added)
}

func (d *desktopApp) setLogViewText(text string) {
	if len(d.logs.Segments) == 1 {
		if segment, ok := d.logs.Segments[0].(*widget.TextSegment); ok {
			segment.Text = text
			d.logs.Refresh()
			return
		}
	}
	d.logs.Segments = []widget.RichTextSegment{logTextSegment(text)}
	d.logs.Refresh()
}

func logTextSegment(text string) *widget.TextSegment {
	style := widget.RichTextStyleInline
	style.TextStyle = fyne.TextStyle{Monospace: true}
	return &widget.TextSegment{
		Style: style,
		Text:  text,
	}
}

func (d *desktopApp) startLogWatcher() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			text := d.ctrl.ReadRecentLogs(logTailLines)
			if text == "" {
				continue
			}
			fyne.Do(func() {
				d.appendNewServiceLogs(text)
			})
		}
	}()
}

func readonlyEntry() *widget.Entry {
	e := widget.NewEntry()
	e.Wrapping = fyne.TextWrapOff
	e.Disable()
	return e
}

func installedText(ok bool) string {
	if ok {
		return "已安装"
	}
	return "未安装"
}

func runningText(ok bool) string {
	if ok {
		return "运行中"
	}
	return "已停止"
}

func defaultText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func defaultValue(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func normalizeLogLines(text string) []string {
	raw := strings.Split(strings.TrimSpace(text), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func diffLogLines(previous, current []string) []string {
	if len(current) == 0 {
		return nil
	}
	if len(previous) == 0 {
		return append([]string(nil), current...)
	}

	maxOverlap := len(previous)
	if len(current) < maxOverlap {
		maxOverlap = len(current)
	}
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if equalStringSlices(previous[len(previous)-overlap:], current[:overlap]) {
			return append([]string(nil), current[overlap:]...)
		}
	}
	return append([]string(nil), current...)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newSystemController() systemController {
	if runtime.GOOS == "windows" {
		return windowsController{}
	}
	return unixController{darwin: runtime.GOOS == "darwin"}
}

type unixController struct {
	darwin bool
}

func (c unixController) Name() string {
	if c.darwin {
		return "macOS LaunchDaemon"
	}
	return "systemd"
}

func (c unixController) ConfigDir() string {
	return "/etc/meshlink"
}

func (c unixController) DataDir() string {
	return "/etc/meshlink/data"
}

func (c unixController) StateFile() string {
	return filepath.Join(c.DataDir(), "state.json")
}

func (c unixController) LogFile() string {
	if c.darwin {
		return "/var/log/meshlink.log"
	}
	return ""
}

func (c unixController) IsInstalled() bool {
	if c.darwin {
		return fileExists("/Library/LaunchDaemons/com.meshlink.p2p.plist") && fileExists("/usr/local/bin/meshlink") && fileExists("/usr/local/bin/p2p-node")
	}
	return fileExists("/etc/systemd/system/meshlink.service") && fileExists("/usr/local/bin/meshlink") && fileExists("/usr/local/bin/p2p-node")
}

func (c unixController) IsRunning() bool {
	var cmd *exec.Cmd
	if c.darwin {
		cmd = exec.Command("pgrep", "-x", "p2p-node")
	} else {
		cmd = exec.Command("systemctl", "is-active", "--quiet", "meshlink")
	}
	return cmd.Run() == nil
}

func (c unixController) Install(port, bootstrap string, relay bool) error {
	scriptName := "install.sh"
	if c.darwin {
		scriptName = "install-macos.sh"
	}
	script, err := findBundledFile(scriptName)
	if err != nil {
		return err
	}
	args := []string{"/bin/bash", script, "--port", port}
	if relay {
		args = append(args, "--relay")
	} else if bootstrap != "" {
		args = append(args, "--bootstrap", bootstrap)
	}
	return runPrivilegedShell(args)
}

func (c unixController) Start() error {
	if c.darwin {
		return runPrivilegedShell([]string{"/usr/local/bin/meshlink", "start"})
	}
	return runPrivilegedShell([]string{"/usr/local/bin/meshlink", "start"})
}

func (c unixController) Stop() error {
	return runPrivilegedShell([]string{"/usr/local/bin/meshlink", "stop"})
}

func (c unixController) Restart() error {
	return runPrivilegedShell([]string{"/usr/local/bin/meshlink", "restart"})
}

func (c unixController) Status() installStatus {
	st := installStatus{
		Installed: c.IsInstalled(),
		Running:   c.IsRunning(),
		ConfigDir: c.ConfigDir(),
		DataDir:   c.DataDir(),
		StateFile: c.StateFile(),
		LogFile:   c.LogFile(),
	}
	if st.Installed {
		settings, err := c.readServiceSettings()
		if err == nil {
			st.Settings = settings
		} else {
			st.Message = "已安装，但无法读取安装配置: " + err.Error()
		}
	}
	state, err := readMeshState(c.StateFile())
	if err == nil {
		st.State = state
	}
	return st
}

func (c unixController) ReadRecentLogs(lines int) string {
	if c.darwin {
		return tailFile(c.LogFile(), lines)
	}
	out, err := exec.Command("journalctl", "-u", "meshlink", "-n", strconv.Itoa(lines), "--no-pager").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return string(out)
}

func (c unixController) readServiceSettings() (*serviceSettings, error) {
	envPath := filepath.Join(c.ConfigDir(), "meshlink.env")
	if settings, err := readEnvSettings(envPath); err == nil {
		return settings, nil
	}
	if c.darwin {
		return readLaunchDaemonSettings("/Library/LaunchDaemons/com.meshlink.p2p.plist")
	}
	return readSystemdSettings("/etc/systemd/system/meshlink.service")
}

type windowsController struct{}

func (c windowsController) Name() string {
	return "Windows Scheduled Task"
}

func (c windowsController) ConfigDir() string {
	return `C:\Program Files\MeshLink`
}

func (c windowsController) DataDir() string {
	return filepath.Join(c.ConfigDir(), "data")
}

func (c windowsController) StateFile() string {
	return filepath.Join(c.DataDir(), "state.json")
}

func (c windowsController) LogFile() string {
	return filepath.Join(c.ConfigDir(), "meshlink.log")
}

func (c windowsController) IsInstalled() bool {
	return fileExists(filepath.Join(c.ConfigDir(), "meshlink.ps1")) && fileExists(filepath.Join(c.ConfigDir(), "p2p-node.exe"))
}

func (c windowsController) IsRunning() bool {
	cmd := `if (Get-Process p2p-node -ErrorAction SilentlyContinue) { exit 0 } else { exit 1 }`
	return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmd).Run() == nil
}

func (c windowsController) Install(port, bootstrap string, relay bool) error {
	script, err := findBundledFile("install.ps1")
	if err != nil {
		return err
	}
	args := []string{"-ExecutionPolicy", "Bypass", "-File", script, "-port", port}
	if relay {
		args = append(args, "-relay")
	} else if bootstrap != "" {
		args = append(args, "-bootstrap", bootstrap)
	}
	psArgs := windowsPowerShellArgs(args)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Start-Process powershell -ArgumentList "+psArgs+" -Verb RunAs -Wait")
	return runCmd(cmd)
}

func (c windowsController) Start() error {
	return c.runManager("start")
}

func (c windowsController) Stop() error {
	return c.runManager("stop")
}

func (c windowsController) Restart() error {
	return c.runManager("restart")
}

func (c windowsController) runManager(command string) error {
	script := filepath.Join(c.ConfigDir(), "meshlink.ps1")
	args := windowsPowerShellArgs([]string{"-ExecutionPolicy", "Bypass", "-File", script, "-command", command})
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Start-Process powershell -ArgumentList "+args+" -Verb RunAs -Wait")
	return runCmd(cmd)
}

func (c windowsController) Status() installStatus {
	st := installStatus{
		Installed: c.IsInstalled(),
		Running:   c.IsRunning(),
		ConfigDir: c.ConfigDir(),
		DataDir:   c.DataDir(),
		StateFile: c.StateFile(),
		LogFile:   c.LogFile(),
	}
	if st.Installed {
		settings, err := readEnvSettings(filepath.Join(c.ConfigDir(), "meshlink.env"))
		if err == nil {
			st.Settings = settings
		} else {
			st.Message = "已安装，但无法读取安装配置: " + err.Error()
		}
	}
	state, err := readMeshState(c.StateFile())
	if err == nil {
		st.State = state
	}
	return st
}

func (c windowsController) ReadRecentLogs(lines int) string {
	return tailFile(c.LogFile(), lines)
}

func readEnvSettings(path string) (*serviceSettings, error) {
	values, err := readKeyValueFile(path)
	if err != nil {
		return nil, err
	}
	settings := &serviceSettings{
		Port:      firstNonEmpty(values["PORT"], "4001"),
		Relay:     parseBool(values["RELAY"]),
		Bootstrap: strings.TrimSpace(values["BOOTSTRAP_ADDR"]),
	}
	settings.Summary = settings.summary()
	return settings, nil
}

func readSystemdSettings(path string) (*serviceSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	settings := &serviceSettings{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		settings.applyArgs(strings.Fields(strings.TrimPrefix(line, "ExecStart=")))
		break
	}
	if settings.Port == "" && !settings.Relay && settings.Bootstrap == "" {
		return nil, fmt.Errorf("未在 %s 中找到 MeshLink 启动参数", path)
	}
	settings.Summary = settings.summary()
	return settings, nil
}

type launchdPlist struct {
	ProgramArguments []string `xml:"dict>array>string"`
}

func readLaunchDaemonSettings(path string) (*serviceSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plist launchdPlist
	if err := xml.Unmarshal(data, &plist); err != nil {
		return nil, err
	}
	settings := &serviceSettings{}
	settings.applyArgs(plist.ProgramArguments)
	if settings.Port == "" && !settings.Relay && settings.Bootstrap == "" {
		return nil, fmt.Errorf("未在 %s 中找到 MeshLink 启动参数", path)
	}
	settings.Summary = settings.summary()
	return settings, nil
}

func (s *serviceSettings) applyArgs(args []string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-port", "--port":
			if i+1 < len(args) {
				s.Port = strings.TrimSpace(args[i+1])
				i++
			}
		case "-bootstrap", "--bootstrap":
			if i+1 < len(args) {
				s.Bootstrap = strings.TrimSpace(args[i+1])
				i++
			}
		case "-relay", "--relay":
			s.Relay = true
		}
	}
	if s.Port == "" {
		s.Port = "4001"
	}
}

func (s serviceSettings) summary() string {
	mode := modeClient
	if s.Relay {
		mode = modeRelay
	}
	parts := []string{
		"模式=" + mode,
		"端口=" + defaultValue(s.Port, "4001"),
	}
	if !s.Relay {
		parts = append(parts, "引导节点="+defaultValue(s.Bootstrap, "未配置"))
	}
	return strings.Join(parts, "，")
}

func readKeyValueFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s 没有可读配置项", path)
	}
	return values, nil
}

func readMeshState(path string) (*meshState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st meshState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func runPrivilegedShell(args []string) error {
	if len(args) == 0 {
		return errors.New("empty command")
	}
	command := shellCommand(args[0], args[1:])
	if runtime.GOOS == "darwin" {
		script := fmt.Sprintf("do shell script %q with administrator privileges", command)
		return runCmd(exec.Command("osascript", "-e", script))
	}
	all := append([]string{args[0]}, args[1:]...)
	return runCmd(exec.Command("pkexec", all...))
}

func runCmd(cmd *exec.Cmd) error {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(out.String())
		if text == "" {
			return err
		}
		return fmt.Errorf("%v: %s", err, text)
	}
	return nil
}

func findBundledFile(name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dirs := []string{filepath.Dir(exe)}
	if runtime.GOOS == "darwin" {
		dirs = append(dirs,
			filepath.Join(filepath.Dir(exe), "..", "Resources"),
			filepath.Join(filepath.Dir(exe), "..", "..", ".."),
		)
	}
	cwd, err := os.Getwd()
	if err == nil {
		dirs = append(dirs, cwd, filepath.Join(cwd, "scripts"))
	}
	for _, dir := range dirs {
		path := filepath.Clean(filepath.Join(dir, name))
		if fileExists(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("找不到安装文件: %s", name)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func tailFile(path string, lines int) string {
	if path == "" || lines <= 0 {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	ring := make([]string, lines)
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	for scanner.Scan() {
		ring[count%lines] = scanner.Text()
		count++
	}

	start := 0
	total := count
	if count > lines {
		start = count % lines
		total = lines
	}
	out := make([]string, 0, total)
	for i := 0; i < total; i++ {
		out = append(out, ring[(start+i)%lines])
	}
	return strings.Join(out, "\n")
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

func windowsPowerShellArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, powershellQuote(arg))
	}
	return strings.Join(quoted, ",")
}

func powershellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
