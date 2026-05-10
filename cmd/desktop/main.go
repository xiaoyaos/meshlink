package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
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
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var Version = "dev"

const (
	modeClient        = "客户端"
	modeRelay         = "引导/中继"
	logTailLines      = 200
	defaultWindowWide = 1100
	defaultWindowHigh = 760
	logPanelHigh      = 112
)

var (
	logThemeDefault fyne.ThemeColorName = "meshlinkLogDefault"
	logThemeNetwork fyne.ThemeColorName = "meshlinkLogNetwork"
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

	installed       *widget.Label
	status          *widget.Label
	nodeType        *widget.Label
	deckInstalled   *widget.Label
	deckStatus      *widget.Label
	deckNodeType    *widget.Label
	controller      *widget.Label
	peerCount       *widget.Label
	updatedAt       *widget.Label
	vip             *widget.Entry
	peerID          *widget.Entry
	config          *widget.Entry
	installConf     *widget.Entry
	logFile         *widget.Entry
	logs            *widget.RichText
	logScroller     *container.Scroll
	coreStatus      *canvas.Text
	coreMode        *canvas.Text
	coreVIP         *canvas.Text
	coreRing        *canvas.Circle
	coreGlow        *canvas.Circle
	installLamp     *canvas.Circle
	runningLamp     *canvas.Circle
	roleLamp        *canvas.Circle
	deckInstallLamp *canvas.Circle
	deckRunningLamp *canvas.Circle
	deckRoleLamp    *canvas.Circle
	reinstalling    bool
	installedNow    bool

	logMu              sync.Mutex
	logLines           []string
	lastLogLine        string
	lastLogRepeat      int
	logRepeatIndex     map[string]int
	logRepeatCount     map[string]int
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

type meshTheme struct{}

func (meshTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x03, G: 0x08, B: 0x11, A: 0xff}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x0b, G: 0x25, B: 0x34, A: 0xff}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0x13, G: 0x20, B: 0x28, A: 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xde, G: 0xfb, B: 0xff, A: 0xff}
	case logThemeDefault:
		return color.NRGBA{R: 0xc8, G: 0xf8, B: 0xff, A: 0xff}
	case logThemeNetwork:
		return color.NRGBA{R: 0x8e, G: 0xef, B: 0xff, A: 0xff}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0x6b, G: 0x7f, B: 0x88, A: 0xff}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x6c, G: 0x8f, B: 0x9c, A: 0xff}
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0xff}
	case theme.ColorNameForegroundOnPrimary:
		return color.NRGBA{R: 0x01, G: 0x0a, B: 0x10, A: 0xff}
	case theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0x05, G: 0x15, B: 0x22, A: 0xf5}
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return color.NRGBA{R: 0x1a, G: 0xe8, B: 0xff, A: 0x78}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0x24}
	case theme.ColorNameFocus, theme.ColorNameSelection:
		return color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0x40}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0x2e}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 0x07, G: 0x1d, B: 0x2c, A: 0xff}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x99}
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0x80}
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 0x06, G: 0x12, B: 0x20, A: 0x80}
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x39, G: 0xf7, B: 0x91, A: 0xff}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xff, G: 0x5f, B: 0x7a, A: 0xff}
	case theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning, theme.ColorNameForegroundOnError:
		return color.NRGBA{R: 0x03, G: 0x08, B: 0x11, A: 0xff}
	}
	return theme.DarkTheme().Color(name, theme.VariantDark)
}

func (meshTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DarkTheme().Font(style)
}

func (meshTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DarkTheme().Icon(name)
}

func (meshTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding, theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameText:
		return 13
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameHeadingText:
		return 19
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius, theme.SizeNameWindowButtonRadius:
		return 4
	case theme.SizeNameScrollBar:
		return 12
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DarkTheme().Size(name)
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
	a.Settings().SetTheme(meshTheme{})
	w := a.NewWindow("MeshLink Desktop")

	d := &desktopApp{
		window:         w,
		ctrl:           newSystemController(),
		logRepeatIndex: make(map[string]int),
		logRepeatCount: make(map[string]int),
	}
	d.build()

	w.SetContent(d.content())
	w.Resize(fyne.NewSize(defaultWindowWide, defaultWindowHigh))
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
	d.deckInstalled = widget.NewLabel("检测中")
	d.deckStatus = widget.NewLabel("未知")
	d.deckNodeType = widget.NewLabel("-")
	d.controller = widget.NewLabel("-")
	d.peerCount = widget.NewLabel("-")
	d.updatedAt = widget.NewLabel("-")
	d.vip = readonlyEntry()
	d.peerID = readonlyEntry()
	d.config = readonlyEntry()
	d.installConf = readonlyEntry()
	d.logFile = readonlyEntry()
	d.logs = widget.NewRichText()
	d.logs.Wrapping = fyne.TextWrapOff
	d.coreStatus = canvas.NewText("STANDBY", color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff})
	d.coreStatus.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	d.coreStatus.TextSize = 22
	d.coreMode = canvas.NewText("-", color.NRGBA{R: 0x8e, G: 0xef, B: 0xff, A: 0xff})
	d.coreMode.TextStyle = fyne.TextStyle{Monospace: true}
	d.coreMode.TextSize = 13
	d.coreVIP = canvas.NewText("-", color.NRGBA{R: 0xde, G: 0xfb, B: 0xff, A: 0xff})
	d.coreVIP.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	d.coreVIP.TextSize = 14
	d.coreRing = canvas.NewCircle(color.Transparent)
	d.coreRing.StrokeColor = color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0xcc}
	d.coreRing.StrokeWidth = 3
	d.coreGlow = canvas.NewCircle(color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0x20})
	d.installLamp = statusLamp(color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff})
	d.runningLamp = statusLamp(color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff})
	d.roleLamp = statusLamp(color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0xff})
	d.deckInstallLamp = statusLamp(color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff})
	d.deckRunningLamp = statusLamp(color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff})
	d.deckRoleLamp = statusLamp(color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0xff})

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
	d.startButton.Importance = widget.SuccessImportance
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
	d.refreshButton.Importance = widget.LowImportance
	d.clearLogButton = widget.NewButtonWithIcon("清空", theme.ContentClearIcon(), func() {
		d.clearLogs()
	})
	d.clearLogButton.Importance = widget.LowImportance

	d.updateBootstrapState()
}

func (d *desktopApp) content() fyne.CanvasObject {
	left := d.commandPanel()
	mid := d.situationPanel()
	right := d.installPanel()

	rightSplit := container.NewHSplit(mid, right)
	rightSplit.SetOffset(0.55)
	center := container.NewHSplit(left, rightSplit)
	center.SetOffset(0.24)

	main := container.NewBorder(
		d.headerPanel(),
		d.logPanel(),
		nil,
		nil,
		center,
	)

	bg := canvas.NewLinearGradient(
		color.NRGBA{R: 0x04, G: 0x0b, B: 0x14, A: 0xff},
		color.NRGBA{R: 0x08, G: 0x1d, B: 0x2c, A: 0xff},
		28,
	)
	return container.NewStack(bg, container.NewPadded(main))
}

func (d *desktopApp) headerPanel() fyne.CanvasObject {
	title := canvas.NewText("MESHLINK COMMAND", color.NRGBA{R: 0xd8, G: 0xfb, B: 0xff, A: 0xff})
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 24

	subtitle := canvas.NewText(fmt.Sprintf("P2P TUNNEL OPERATIONS  v%s  %s/%s", Version, runtime.GOOS, runtime.GOARCH), color.NRGBA{R: 0x67, G: 0xc7, B: 0xde, A: 0xff})
	subtitle.TextSize = 12
	subtitle.TextStyle = fyne.TextStyle{Monospace: true}

	brand := container.NewVBox(title, subtitle)
	status := container.NewGridWithColumns(3,
		statusTile("INSTALL", d.installed, d.installLamp),
		statusTile("SERVICE", d.status, d.runningLamp),
		statusTile("ROLE", d.nodeType, d.roleLamp),
	)

	headerBody := container.NewBorder(nil, nil, brand, nil, status)
	return hudPanel("", "", headerBody)
}

func (d *desktopApp) commandPanel() fyne.CanvasObject {
	serviceControls := container.NewGridWithColumns(2,
		d.startButton,
		d.stopButton,
		d.restartButton,
		d.refreshButton,
	)
	installControls := container.NewVBox(d.installButton, d.cancelButton)
	content := container.NewVBox(
		serviceControls,
		hudDivider(),
		installControls,
	)
	return hudPanel("COMMAND DECK", "primary controls", content)
}

func (d *desktopApp) situationPanel() fyne.CanvasObject {
	copyPeer := widget.NewButtonWithIcon("复制 Peer ID", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(d.peerID.Text)
	})
	copyVIP := widget.NewButtonWithIcon("复制虚拟 IP", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(d.vip.Text)
	})

	metrics := container.NewGridWithColumns(3,
		statusTile("CTRL", d.controller, nil),
		statusTile("PEERS", d.peerCount, nil),
		statusTile("SYNC", d.updatedAt, nil),
	)

	content := container.NewVBox(
		d.corePanel(),
		hudDivider(),
		metrics,
		hudDivider(),
		valueRow("虚拟 IP", d.vip),
		valueRow("Peer ID", d.peerID),
		container.NewGridWithColumns(2, copyVIP, copyPeer),
	)
	return hudPanel("SITUATION CORE", "live node telemetry", content)
}

func (d *desktopApp) corePanel() fyne.CanvasObject {
	holder := canvas.NewRectangle(color.Transparent)
	holder.SetMinSize(fyne.NewSize(190, 128))
	d.coreGlow.Resize(fyne.NewSize(112, 112))
	d.coreRing.Resize(fyne.NewSize(96, 96))
	d.coreRing.Move(fyne.NewPos(8, 8))

	dialBg := canvas.NewRadialGradient(
		color.NRGBA{R: 0x14, G: 0x4d, B: 0x68, A: 0xee},
		color.NRGBA{R: 0x03, G: 0x08, B: 0x11, A: 0x00},
	)
	dialBg.SetMinSize(fyne.NewSize(150, 128))
	dial := container.NewStack(
		holder,
		dialBg,
		container.NewCenter(d.coreGlow),
		container.NewCenter(d.coreRing),
		container.NewCenter(container.NewVBox(
			container.NewCenter(d.coreStatus),
			container.NewCenter(d.coreMode),
			container.NewCenter(d.coreVIP),
		)),
	)
	return container.NewCenter(dial)
}

func (d *desktopApp) statusStrip(label string, value *widget.Label, lamp *canvas.Circle) fyne.CanvasObject {
	caption := canvas.NewText(strings.ToUpper(label), color.NRGBA{R: 0x58, G: 0xc8, B: 0xe2, A: 0xff})
	caption.TextStyle = fyne.TextStyle{Monospace: true}
	caption.TextSize = 10
	body := container.NewBorder(nil, nil, fixedLamp(lamp), nil, container.NewVBox(caption, value))
	return miniPanel(body)
}

func (d *desktopApp) installPanel() fyne.CanvasObject {
	installForm := widget.NewForm(
		widget.NewFormItem("模式", d.mode),
		widget.NewFormItem("监听端口", d.port),
		widget.NewFormItem("引导节点", d.bootstrap),
	)

	content := container.NewVBox(
		installForm,
		hudDivider(),
		valueRow("当前配置", d.installConf),
		valueRow("配置目录", d.config),
		valueRow("日志文件", d.logFile),
	)
	return hudPanel("INSTALL MATRIX", "service profile", content)
}

func (d *desktopApp) logPanel() fyne.CanvasObject {
	d.logScroller = container.NewScroll(d.logs)
	d.logScroller.SetMinSize(fyne.NewSize(1, logPanelHigh))

	logBg := canvas.NewRectangle(color.NRGBA{R: 0x02, G: 0x0a, B: 0x11, A: 0xd8})
	logBg.StrokeColor = color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0x28}
	logBg.StrokeWidth = 1
	logBg.CornerRadius = 4
	floor := canvas.NewRectangle(color.Transparent)
	floor.SetMinSize(fyne.NewSize(1, logPanelHigh))
	logs := container.NewStack(floor, logBg, container.NewPadded(d.logScroller))
	return hudPanelWithAction("EVENT STREAM", "live service trace", d.clearLogButton, logs)
}

func hudPanel(title, subtitle string, body fyne.CanvasObject) fyne.CanvasObject {
	return hudPanelWithAction(title, subtitle, nil, body)
}

func hudPanelWithAction(title, subtitle string, action fyne.CanvasObject, body fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.NRGBA{R: 0x06, G: 0x12, B: 0x20, A: 0xee})
	bg.StrokeColor = color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0x70}
	bg.StrokeWidth = 1
	bg.CornerRadius = 8

	glow := canvas.NewRectangle(color.NRGBA{R: 0x1d, G: 0xf4, B: 0xff, A: 0x28})
	glow.CornerRadius = 8

	var content fyne.CanvasObject
	if title == "" {
		content = body
	} else {
		content = container.NewBorder(panelTitle(title, subtitle, action), nil, nil, nil, body)
	}
	return container.NewStack(glow, bg, container.NewPadded(content))
}

func panelTitle(title, subtitle string, action fyne.CanvasObject) fyne.CanvasObject {
	main := canvas.NewText(title, color.NRGBA{R: 0x9c, G: 0xf7, B: 0xff, A: 0xff})
	main.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	main.TextSize = 13
	var titleLine fyne.CanvasObject = main
	if strings.TrimSpace(subtitle) != "" {
		sub := canvas.NewText(strings.ToUpper(subtitle), color.NRGBA{R: 0x57, G: 0x9e, B: 0xb3, A: 0xff})
		sub.TextStyle = fyne.TextStyle{Monospace: true}
		sub.TextSize = 10
		titleLine = container.NewHBox(main, sub)
	}
	filler := canvas.NewRectangle(color.Transparent)
	filler.SetMinSize(fyne.NewSize(1, 1))
	header := container.NewBorder(nil, nil, titleLine, action, filler)
	return container.NewBorder(nil, hudDivider(), nil, nil, header)
}

func sectionTitle(title, subtitle string) fyne.CanvasObject {
	main := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	if strings.TrimSpace(subtitle) == "" {
		return main
	}
	sub := widget.NewLabel(subtitle)
	return container.NewVBox(main, sub)
}

func statusTile(label string, value *widget.Label, lamp *canvas.Circle) fyne.CanvasObject {
	caption := canvas.NewText(strings.ToUpper(label), color.NRGBA{R: 0x58, G: 0xc8, B: 0xe2, A: 0xff})
	caption.TextStyle = fyne.TextStyle{Monospace: true}
	caption.TextSize = 10
	content := container.NewVBox(caption, value)
	if lamp == nil {
		return miniPanel(content)
	}
	return miniPanel(container.NewBorder(nil, nil, fixedLamp(lamp), nil, content))
}

func valueRow(label string, value *widget.Entry) fyne.CanvasObject {
	caption := widget.NewLabelWithStyle(label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewVBox(caption, value)
}

func miniPanel(body fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.NRGBA{R: 0x03, G: 0x11, B: 0x1b, A: 0xbf})
	bg.StrokeColor = color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0x38}
	bg.StrokeWidth = 1
	bg.CornerRadius = 6
	return container.NewStack(bg, container.NewPadded(body))
}

func statusLamp(c color.Color) *canvas.Circle {
	lamp := canvas.NewCircle(c)
	lamp.StrokeColor = color.NRGBA{R: 0xde, G: 0xfb, B: 0xff, A: 0xaa}
	lamp.StrokeWidth = 1
	return lamp
}

func fixedLamp(lamp *canvas.Circle) fyne.CanvasObject {
	if lamp == nil {
		lamp = statusLamp(color.NRGBA{R: 0x6b, G: 0x7f, B: 0x88, A: 0xff})
	}
	lamp.Resize(fyne.NewSize(12, 12))
	holder := canvas.NewRectangle(color.Transparent)
	holder.SetMinSize(fyne.NewSize(18, 18))
	return container.NewCenter(container.NewStack(holder, container.NewCenter(lamp)))
}

func lampColor(ok bool) color.Color {
	if ok {
		return color.NRGBA{R: 0x39, G: 0xf7, B: 0x91, A: 0xff}
	}
	return color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff}
}

func roleColor(role string) color.Color {
	switch strings.TrimSpace(role) {
	case modeRelay:
		return color.NRGBA{R: 0xb0, G: 0x7a, B: 0xff, A: 0xff}
	case modeClient:
		return color.NRGBA{R: 0x16, G: 0xd9, B: 0xff, A: 0xff}
	default:
		return color.NRGBA{R: 0x6b, G: 0x7f, B: 0x88, A: 0xff}
	}
}

func hudDivider() fyne.CanvasObject {
	line := canvas.NewRectangle(color.NRGBA{R: 0x1a, G: 0xe8, B: 0xff, A: 0x55})
	line.SetMinSize(fyne.NewSize(1, 1))
	return line
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
	d.deckInstalled.SetText(d.installed.Text)
	d.deckStatus.SetText(d.status.Text)
	d.controller.SetText(d.ctrl.Name())
	installColor := lampColor(st.Installed)
	runningColor := lampColor(st.Running)
	d.installLamp.FillColor = installColor
	d.runningLamp.FillColor = runningColor
	d.deckInstallLamp.FillColor = installColor
	d.deckRunningLamp.FillColor = runningColor
	d.installLamp.Refresh()
	d.runningLamp.Refresh()
	d.deckInstallLamp.Refresh()
	d.deckRunningLamp.Refresh()
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
		d.deckNodeType.SetText(d.nodeType.Text)
		d.peerCount.SetText(strconv.Itoa(len(st.State.Peers)))
		d.updatedAt.SetText(defaultText(compactTime(st.State.UpdatedAt)))
		d.vip.SetText(defaultText(st.State.SelfVIP))
		d.peerID.SetText(defaultText(st.State.SelfID))
		d.coreMode.Text = defaultText(st.State.NodeType)
		d.coreVIP.Text = defaultText(st.State.SelfVIP)
	} else {
		d.nodeType.SetText("-")
		d.deckNodeType.SetText("-")
		d.peerCount.SetText("-")
		d.updatedAt.SetText("-")
		d.vip.SetText("")
		d.peerID.SetText("")
		d.coreMode.Text = "-"
		d.coreVIP.Text = "-"
	}
	roleLampColor := roleColor(d.nodeType.Text)
	d.roleLamp.FillColor = roleLampColor
	d.deckRoleLamp.FillColor = roleLampColor
	d.roleLamp.Refresh()
	d.deckRoleLamp.Refresh()
	d.refreshCoreState(st.Running)

	d.updateControls(st)

	d.appendNewServiceLogs(d.ctrl.ReadRecentLogs(logTailLines))
	if st.Message != "" {
		d.appendLog("%s", st.Message)
	}
}

func (d *desktopApp) refreshCoreState(running bool) {
	if running {
		d.coreStatus.Text = "ONLINE"
		d.coreStatus.Color = color.NRGBA{R: 0x39, G: 0xf7, B: 0x91, A: 0xff}
		d.coreRing.StrokeColor = color.NRGBA{R: 0x39, G: 0xf7, B: 0x91, A: 0xdd}
		d.coreGlow.FillColor = color.NRGBA{R: 0x39, G: 0xf7, B: 0x91, A: 0x24}
	} else {
		d.coreStatus.Text = "STANDBY"
		d.coreStatus.Color = color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xff}
		d.coreRing.StrokeColor = color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0xcc}
		d.coreGlow.FillColor = color.NRGBA{R: 0xff, G: 0xc8, B: 0x57, A: 0x20}
	}
	d.coreStatus.Refresh()
	d.coreMode.Refresh()
	d.coreVIP.Refresh()
	d.coreRing.Refresh()
	d.coreGlow.Refresh()
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
	d.updateControls(d.ctrl.Status())
}

func (d *desktopApp) updateControls(st installStatus) {
	d.installButton.Enable()
	d.refreshButton.Enable()

	switch {
	case st.Installed && d.reinstalling:
		d.installButton.SetText("确认重装")
		d.cancelButton.Show()
		d.cancelButton.Enable()
	case st.Installed:
		d.installButton.SetText("重新安装")
		d.cancelButton.Hide()
		d.cancelButton.Disable()
	default:
		d.installButton.SetText("安装")
		d.cancelButton.Hide()
		d.cancelButton.Disable()
	}

	switch {
	case !st.Installed || d.reinstalling:
		d.startButton.Disable()
		d.stopButton.Disable()
		d.restartButton.Disable()
	case st.Running:
		d.startButton.Disable()
		d.stopButton.Enable()
		d.restartButton.Enable()
	default:
		d.startButton.Enable()
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
	for _, line := range clean {
		d.pushLogLineLocked(line)
	}
	if len(d.logLines) > 500 {
		d.logLines = d.logLines[len(d.logLines)-500:]
		d.rebuildLogRepeatIndexLocked()
	}
	text := strings.Join(d.logLines, "\n")
	d.logMu.Unlock()

	d.setLogViewText(text)
}

func (d *desktopApp) clearLogs() {
	d.logMu.Lock()
	d.logLines = nil
	d.lastLogLine = ""
	d.lastLogRepeat = 0
	d.logRepeatIndex = make(map[string]int)
	d.logRepeatCount = make(map[string]int)
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
	d.logs.Segments = logSegments(text)
	d.logs.Refresh()
	if d.logScroller != nil {
		d.logScroller.ScrollToBottom()
	}
}

func (d *desktopApp) pushLogLineLocked(line string) {
	normalized := normalizeRepeatingLogLine(line)
	if normalized != "" && shouldAggregateLogLine(normalized) {
		if idx, ok := d.logRepeatIndex[normalized]; ok && idx >= 0 && idx < len(d.logLines) {
			d.logRepeatCount[normalized]++
			d.logLines[idx] = formatAggregatedLogLine(line, d.logRepeatCount[normalized])
			d.lastLogLine = normalized
			d.lastLogRepeat = d.logRepeatCount[normalized]
			return
		}
		d.logRepeatIndex[normalized] = len(d.logLines)
		d.logRepeatCount[normalized] = 1
		d.lastLogLine = normalized
		d.lastLogRepeat = 1
		d.logLines = append(d.logLines, line)
		return
	}
	if normalized != "" && normalized == d.lastLogLine {
		d.lastLogRepeat++
		d.replaceRepeatLineLocked()
		return
	}
	d.lastLogLine = normalized
	d.lastLogRepeat = 1
	d.logLines = append(d.logLines, line)
}

func (d *desktopApp) replaceRepeatLineLocked() {
	if len(d.logLines) == 0 {
		return
	}
	repeat := fmt.Sprintf("    ↳ 上一条重复 %d 次", d.lastLogRepeat)
	last := d.logLines[len(d.logLines)-1]
	if strings.Contains(last, "↳ 上一条重复") && len(d.logLines) >= 2 {
		d.logLines[len(d.logLines)-1] = repeat
		return
	}
	d.logLines = append(d.logLines, repeat)
}

func (d *desktopApp) rebuildLogRepeatIndexLocked() {
	d.logRepeatIndex = make(map[string]int)
	d.logRepeatCount = make(map[string]int)
	for idx, line := range d.logLines {
		normalized := normalizeRepeatingLogLine(line)
		if normalized == "" || !shouldAggregateLogLine(normalized) {
			continue
		}
		d.logRepeatIndex[normalized] = idx
		d.logRepeatCount[normalized]++
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

func compactTime(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= len("2006-01-02 15:04:05") {
		return s[11:19]
	}
	return s
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

func normalizeRepeatingLogLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "↳ 上一条重复") {
		return ""
	}
	if idx := strings.LastIndex(line, "  |  共 "); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		if _, err := time.Parse("15:04:05", fields[0]); err == nil {
			line = strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		}
	}
	return line
}

func shouldAggregateLogLine(line string) bool {
	return strings.Contains(line, "[控制] 映射成功:") || strings.Contains(line, "[路由] 寻址失败")
}

func formatAggregatedLogLine(line string, count int) string {
	if count <= 1 {
		return line
	}
	base := line
	if idx := strings.LastIndex(base, "  |  共 "); idx >= 0 {
		base = strings.TrimSpace(base[:idx])
	}
	return fmt.Sprintf("%s  |  共 %d 次", base, count)
}

func logSegments(text string) []widget.RichTextSegment {
	if strings.TrimSpace(text) == "" {
		return []widget.RichTextSegment{}
	}
	lines := strings.Split(text, "\n")
	segments := make([]widget.RichTextSegment, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		segments = append(segments, &widget.TextSegment{
			Style: logRichTextStyle(line),
			Text:  line,
		})
	}
	return segments
}

func logRichTextStyle(line string) widget.RichTextStyle {
	lower := strings.ToLower(line)
	style := widget.RichTextStyleCodeBlock
	switch {
	case strings.Contains(line, "重复") || strings.Contains(line, "共 "):
		style.ColorName = theme.ColorNameDisabled
		style.TextStyle = fyne.TextStyle{Italic: true}
	case strings.Contains(line, "失败") || strings.Contains(line, "错误") || strings.Contains(line, "致命") || strings.Contains(lower, "error") || strings.Contains(lower, "failed"):
		style.ColorName = theme.ColorNameError
	case strings.Contains(line, "warning") || strings.Contains(line, "warn") || strings.Contains(line, "等待") || strings.Contains(line, "正在"):
		style.ColorName = theme.ColorNameWarning
	case strings.Contains(line, "成功") || strings.Contains(line, "完成") || strings.Contains(line, "已启动") || strings.Contains(line, "已建立") || strings.Contains(line, "ONLINE"):
		style.ColorName = theme.ColorNameSuccess
	case strings.Contains(line, "[网络]") || strings.Contains(line, "[网桥]") || strings.Contains(line, "[隧道]") || strings.Contains(line, "[tunnel]") || strings.Contains(line, "[relay]") || strings.Contains(line, "[bootstrap]") || strings.Contains(line, "[route]"):
		style.ColorName = logThemeNetwork
	default:
		style.ColorName = logThemeDefault
	}
	return style
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
