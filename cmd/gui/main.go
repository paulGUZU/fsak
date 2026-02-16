package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/paulGUZU/fsak/internal/client"
	"github.com/paulGUZU/fsak/pkg/config"
	_ "github.com/xjasonlyu/tun2socks/v2/dns"
	"github.com/xjasonlyu/tun2socks/v2/engine"
)

type ClientProfile struct {
	Name   string       `json:"name"`
	Config ClientConfig `json:"config"`
}

type ClientConfig struct {
	Addresses []string `json:"addresses"`
	Host      string   `json:"host"`
	TLS       bool     `json:"tls"`
	SNI       string   `json:"sni"`
	Port      int      `json:"port"`
	ProxyPort int      `json:"proxy_port"`
	Secret    string   `json:"secret"`
}

type ProfilesStore struct {
	Selected string          `json:"selected"`
	Profiles []ClientProfile `json:"profiles"`
}

type RunningClient struct {
	profileName string
	mode        string
	pool        *client.AddressPool
	socks       *client.SOCKS5Server
	systemProxy client.SystemProxySession
	done        chan error
	startedAt   time.Time
	cleanupMu   sync.Mutex
	cleanedUp   bool
}

type GUIState struct {
	mu       sync.RWMutex
	store    string
	profiles map[string]ClientConfig
	selected string
	runner   *RunningClient
	lastErr  string
}

const (
	startModeProxy = "proxy"
	startModeTUN   = "tun"
	modeLabelProxy = "Proxy (current system)"
	modeLabelTUN   = "TUN (system-wide)"
	tunHelperArg   = "--fsak-tun-helper"
)

type vibrantTheme struct {
	base fyne.Theme
}

func newVibrantTheme() fyne.Theme {
	return &vibrantTheme{base: theme.LightTheme()}
}

func (t *vibrantTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0xF2, G: 0xF7, B: 0xFF, A: 0xFF}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x00, G: 0xB8, B: 0xA9, A: 0xFF}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0xDE, G: 0xFF, B: 0xFA, A: 0xFF}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0xD7, G: 0xF3, B: 0xFF, A: 0xFF}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0x00, G: 0x9C, B: 0x8F, A: 0xFF}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0xFF, G: 0xE5, B: 0x9C, A: 0xFF}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0xFF, G: 0x7A, B: 0x59, A: 0xFF}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0xF8, G: 0xFB, B: 0xFF, A: 0xFF}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xD9, G: 0x2D, B: 0x20, A: 0xFF}
	default:
		return t.base.Color(name, variant)
	}
}

func (t *vibrantTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t *vibrantTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *vibrantTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == tunHelperArg {
		if err := runTunHelper(os.Args[2:]); err != nil {
			log.Fatalf("TUN helper failed: %v", err)
		}
		return
	}

	storePath, err := defaultStorePath()
	if err != nil {
		log.Fatalf("failed to resolve storage path: %v", err)
	}

	state := &GUIState{
		store:    storePath,
		profiles: make(map[string]ClientConfig),
	}
	if err := state.loadProfiles(); err != nil {
		log.Fatalf("failed to load profiles: %v", err)
	}

	ui := newDesktopUI(state)
	ui.run()
}

func defaultStorePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "fsak", "client_profiles.json"), nil
}

func (s *GUIState) loadProfiles() error {
	data, err := os.ReadFile(s.store)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.seedDefaultProfile()
		}
		return err
	}

	var file ProfilesStore
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}

	for _, p := range file.Profiles {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		cfg, err := normalizeConfig(p.Config)
		if err != nil {
			continue
		}
		s.profiles[name] = cfg
	}

	if len(s.profiles) == 0 {
		return s.seedDefaultProfile()
	}

	if _, ok := s.profiles[file.Selected]; ok {
		s.selected = file.Selected
	} else {
		s.selected = sortedProfileNames(s.profiles)[0]
	}

	return nil
}

func (s *GUIState) seedDefaultProfile() error {
	if cfg, err := config.LoadConfig("config.json"); err == nil {
		s.profiles["default"] = fromInternal(*cfg)
		s.selected = "default"
		return s.saveProfilesLocked()
	}

	s.profiles["default"] = ClientConfig{
		Addresses: []string{"127.0.0.1"},
		Host:      "localhost",
		TLS:       false,
		SNI:       "",
		Port:      8080,
		ProxyPort: 1080,
		Secret:    "",
	}
	s.selected = "default"
	return s.saveProfilesLocked()
}

func (s *GUIState) saveProfilesLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.store), 0o755); err != nil {
		return err
	}

	names := sortedProfileNames(s.profiles)
	profiles := make([]ClientProfile, 0, len(names))
	for _, name := range names {
		profiles = append(profiles, ClientProfile{Name: name, Config: s.profiles[name]})
	}

	payload, err := json.MarshalIndent(ProfilesStore{Selected: s.selected, Profiles: profiles}, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.store + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.store)
}

type tunProcessSession struct {
	process *os.Process
	done    chan error
}

type cappedBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.max <= 0 {
		b.max = 4096
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (s *tunProcessSession) Disable() error {
	if s == nil {
		return nil
	}

	if s.process != nil {
		_ = s.process.Signal(syscall.SIGTERM)
	}

	select {
	case <-s.done:
		return nil
	case <-time.After(4 * time.Second):
		if s.process != nil {
			_ = s.process.Kill()
		}
		<-s.done
		return nil
	}
}

func (s *tunProcessSession) Done() <-chan error {
	if s == nil {
		return nil
	}
	return s.done
}

func startTunProcessSession(proxyPort int, bindInterface string, bypassEntries []string) (*tunProcessSession, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("TUN mode currently supports macOS only")
	}

	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve executable path: %w", err)
	}

	args := []string{tunHelperArg, "--proxy-port", strconv.Itoa(proxyPort)}
	if strings.TrimSpace(bindInterface) != "" {
		args = append(args, "--interface", strings.TrimSpace(bindInterface))
	}
	if len(bypassEntries) > 0 {
		args = append(args, "--bypass", strings.Join(bypassEntries, ","))
	}
	cmd := exec.Command(exePath, args...)
	logs := &cappedBuffer{max: 8192}
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start TUN helper: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err == nil {
			msg := strings.TrimSpace(logs.String())
			if msg != "" {
				return nil, fmt.Errorf("TUN helper exited unexpectedly: %s", msg)
			}
			return nil, errors.New("TUN helper exited unexpectedly")
		}
		msg := strings.TrimSpace(logs.String())
		if msg != "" {
			return nil, fmt.Errorf("failed to initialize TUN helper: %v (%s)", err, msg)
		}
		return nil, fmt.Errorf("failed to initialize TUN helper: %w", err)
	case <-time.After(800 * time.Millisecond):
	}

	return &tunProcessSession{
		process: cmd.Process,
		done:    done,
	}, nil
}

func (s *GUIState) startRunner(profileName string, cfg ClientConfig, mode string) error {
	s.mu.Lock()
	if s.runner != nil {
		s.mu.Unlock()
		return errors.New("client is already running")
	}
	s.mu.Unlock()

	if mode != startModeProxy && mode != startModeTUN {
		return fmt.Errorf("unsupported start mode: %s", mode)
	}

	internalCfg := cfg.toInternal()
	pool, err := client.NewAddressPool(internalCfg.Addresses, internalCfg.Port, internalCfg.Host, internalCfg.TLS)
	if err != nil {
		return err
	}

	transport := client.NewTransport(&internalCfg, pool)
	socks := client.NewSOCKS5Server(internalCfg.ProxyPort, transport)
	socksDone := make(chan error, 1)

	go func() {
		socksDone <- socks.ListenAndServe()
	}()

	select {
	case err := <-socksDone:
		pool.Stop()
		if err == nil {
			return errors.New("client stopped unexpectedly")
		}
		return err
	case <-time.After(200 * time.Millisecond):
	}

	var systemProxy client.SystemProxySession
	var systemDone <-chan error
	if mode == startModeTUN {
		if runtime.GOOS != "darwin" {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			_ = socks.Stop(ctx)
			pool.Stop()
			return errors.New("TUN mode currently supports macOS only")
		}

		tunSession, err := startTunProcessSession(internalCfg.ProxyPort, "", internalCfg.Addresses)
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			_ = socks.Stop(ctx)
			pool.Stop()
			return fmt.Errorf("failed to start TUN runtime: %w", err)
		}
		systemProxy = tunSession
		systemDone = tunSession.Done()
	}

	done := make(chan error, 1)
	go func() {
		if systemDone == nil {
			done <- <-socksDone
			return
		}

		select {
		case err := <-socksDone:
			done <- err
		case err := <-systemDone:
			if err == nil {
				err = errors.New("TUN runtime exited unexpectedly")
			}
			done <- err
		}
	}()

	r := &RunningClient{
		profileName: profileName,
		mode:        mode,
		pool:        pool,
		socks:       socks,
		systemProxy: systemProxy,
		done:        done,
		startedAt:   time.Now(),
	}

	s.mu.Lock()
	s.runner = r
	s.lastErr = ""
	s.mu.Unlock()

	return nil
}

func (r *RunningClient) cleanup(timeout time.Duration) error {
	r.cleanupMu.Lock()
	defer r.cleanupMu.Unlock()
	if r.cleanedUp {
		return nil
	}

	var firstErr error
	if r.systemProxy != nil {
		if err := r.systemProxy.Disable(); err != nil {
			firstErr = err
		} else {
			r.systemProxy = nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if r.socks != nil {
		if err := r.socks.Stop(ctx); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			r.socks = nil
		}
	}
	if r.pool != nil {
		r.pool.Stop()
		r.pool = nil
	}
	if firstErr == nil {
		r.cleanedUp = true
	}
	return firstErr
}

func (s *GUIState) stopRunner(timeout time.Duration) error {
	s.mu.RLock()
	r := s.runner
	s.mu.RUnlock()
	if r == nil {
		return nil
	}

	if err := r.cleanup(timeout); err != nil {
		return err
	}

	s.mu.Lock()
	if s.runner == r {
		s.runner = nil
	}
	s.mu.Unlock()
	return nil
}

func (s *GUIState) snapshotProfiles() (selected string, profiles map[string]ClientConfig) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cloned := make(map[string]ClientConfig, len(s.profiles))
	for k, v := range s.profiles {
		cloned[k] = v
	}
	return s.selected, cloned
}

func (s *GUIState) profileListSnapshot() (selected string, names []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names = make([]string, 0, len(s.profiles))
	for name := range s.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return s.selected, names
}

func (s *GUIState) statusSnapshot() (selected string, running bool, active string, mode string, started time.Time, lastErr string, cfg ClientConfig, hasCfg bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	selected = s.selected
	lastErr = s.lastErr
	if s.runner != nil {
		running = true
		active = s.runner.profileName
		mode = s.runner.mode
		started = s.runner.startedAt
		cfg, hasCfg = s.profiles[active]
		return
	}
	cfg, hasCfg = s.profiles[selected]
	return
}

func (s *GUIState) selectedProfileConfig() (string, ClientConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.selected == "" {
		return "", ClientConfig{}, false
	}
	cfg, ok := s.profiles[s.selected]
	if !ok {
		return "", ClientConfig{}, false
	}
	return s.selected, cfg, true
}

func (s *GUIState) runningSnapshot() (running bool, active string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.runner == nil {
		return false, ""
	}
	return true, s.runner.profileName
}

func (s *GUIState) runnerSnapshot() *RunningClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runner
}

func stringSlicesEqual(a, b []string) bool {
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

type desktopUI struct {
	state *GUIState
	app   fyne.App
	win   fyne.Window
	prof  fyne.Window

	suspendProfileChange bool

	profileSelect *widget.Select
	modeSelect    *widget.Select
	manageBtn     *widget.Button

	statusLabel       *widget.Label
	runtimeLabel      *widget.Label
	errorLabel        *widget.Label
	activeProfileStat *widget.Label
	serverStat        *widget.Label
	proxyStat         *widget.Label
	addressStat       *widget.Label
	statusDot         *canvas.Circle
	statusPanel       *canvas.Rectangle

	connectBtn *widget.Button
	refreshBtn *widget.Button
}

func newDesktopUI(state *GUIState) *desktopUI {
	a := app.NewWithID("com.paulguzu.fsak.client.gui")
	a.Settings().SetTheme(newVibrantTheme())

	w := a.NewWindow("FSAK VPN Client")
	w.Resize(fyne.NewSize(418, 1120))

	ui := &desktopUI{
		state: state,
		app:   a,
		win:   w,
	}
	ui.build()
	ui.refreshView()
	ui.bindClose()
	return ui
}

func (ui *desktopUI) run() {
	ui.win.ShowAndRun()
}

func (ui *desktopUI) build() {
	ui.profileSelect = widget.NewSelect(nil, func(name string) {
		if ui.suspendProfileChange {
			return
		}
		if name == "" {
			return
		}
		ui.state.mu.Lock()
		if _, ok := ui.state.profiles[name]; ok {
			ui.state.selected = name
		}
		ui.state.mu.Unlock()
		ui.refreshStatus()
	})
	ui.profileSelect.PlaceHolder = "Choose profile"
	ui.modeSelect = widget.NewSelect([]string{modeLabelProxy, modeLabelTUN}, nil)
	ui.modeSelect.SetSelected(modeLabelProxy)

	ui.statusLabel = widget.NewLabelWithStyle("Disconnected", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	ui.runtimeLabel = widget.NewLabel("Tunnel is offline")
	ui.runtimeLabel.Alignment = fyne.TextAlignCenter
	ui.runtimeLabel.Wrapping = fyne.TextWrapWord

	ui.errorLabel = widget.NewLabel("")
	ui.errorLabel.Wrapping = fyne.TextWrapWord

	ui.activeProfileStat = widget.NewLabel("-")
	ui.serverStat = widget.NewLabel("-")
	ui.proxyStat = widget.NewLabel("-")
	ui.addressStat = widget.NewLabel("-")
	ui.activeProfileStat.TextStyle = fyne.TextStyle{Bold: true}
	ui.serverStat.TextStyle = fyne.TextStyle{Bold: true}
	ui.proxyStat.TextStyle = fyne.TextStyle{Bold: true}
	ui.addressStat.TextStyle = fyne.TextStyle{Bold: true}

	ui.statusDot = canvas.NewCircle(color.NRGBA{R: 0xD9, G: 0x2D, B: 0x20, A: 0xFF})
	ui.statusPanel = canvas.NewRectangle(color.NRGBA{R: 0xFF, G: 0xEE, B: 0xEE, A: 0xFF})

	ui.manageBtn = widget.NewButtonWithIcon("Manage Profiles", theme.SettingsIcon(), ui.openProfileManager)
	ui.manageBtn.Importance = widget.MediumImportance
	ui.connectBtn = widget.NewButtonWithIcon("Connect", theme.MediaPlayIcon(), ui.onConnectToggle)
	ui.connectBtn.Importance = widget.HighImportance
	ui.refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), ui.refreshView)
	ui.refreshBtn.Importance = widget.MediumImportance
	ui.modeSelect.OnChanged = func(string) {
		ui.refreshStatus()
	}

	ui.installMainMenu()
	topCard := widget.NewCard("Active Profile", "Choose profile and start mode", container.NewVBox(
		widget.NewLabel("Profile"),
		ui.profileSelect,
		widget.NewLabel("Start mode"),
		ui.modeSelect,
		ui.manageBtn,
	))

	statusTop := container.NewHBox(
		container.NewGridWrap(fyne.NewSize(16, 16), ui.statusDot),
		widget.NewLabelWithStyle("Tunnel Status", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		ui.statusLabel,
	)
	heroContent := container.NewVBox(
		widget.NewLabelWithStyle("FSAK VPN Dashboard", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		statusTop,
		ui.runtimeLabel,
		container.NewHBox(layout.NewSpacer(), ui.connectBtn, layout.NewSpacer()),
		container.NewHBox(layout.NewSpacer(), ui.refreshBtn),
		ui.errorLabel,
	)
	connectionCard := widget.NewCard("Connection", "One-click tunnel control", container.NewStack(ui.statusPanel, container.NewPadded(heroContent)))

	statsGrid := container.NewGridWithColumns(2,
		ui.statTile("Profile", ui.activeProfileStat, color.NRGBA{R: 0xE8, G: 0xF4, B: 0xFF, A: 0xFF}),
		ui.statTile("Local SOCKS5", ui.proxyStat, color.NRGBA{R: 0xE9, G: 0xFB, B: 0xEF, A: 0xFF}),
		ui.statTile("Server", ui.serverStat, color.NRGBA{R: 0xFF, G: 0xF2, B: 0xE3, A: 0xFF}),
		ui.statTile("Addresses", ui.addressStat, color.NRGBA{R: 0xF5, G: 0xEE, B: 0xFF, A: 0xFF}),
	)

	overviewCard := widget.NewCard("Session Overview", "Current routing context", statsGrid)
	onePage := container.NewVScroll(container.NewVBox(topCard, connectionCard, overviewCard))
	ui.win.SetContent(container.NewPadded(onePage))
}

func (ui *desktopUI) installMainMenu() {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Manage Profiles", ui.openProfileManager),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Refresh", ui.refreshView),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Exit", func() { ui.win.Close() }),
	)

	connectionMenu := fyne.NewMenu("Connection",
		fyne.NewMenuItem("Start Proxy", ui.onStartProxy),
		fyne.NewMenuItem("Start TUN", ui.onStartTUN),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Connect / Disconnect", ui.onConnectToggle),
		fyne.NewMenuItem("Stop", ui.onStop),
	)

	profilesMenu := fyne.NewMenu("Profiles",
		fyne.NewMenuItem("Manage Profiles", ui.openProfileManager),
	)

	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("Quick Tips", func() {
			dialog.ShowInformation("Quick Tips",
				"1. Pick a profile.\n2. Click Connect.\n3. Set your apps to SOCKS5 127.0.0.1:<Local Port>.\n4. Use Disconnect before editing fields.",
				ui.win,
			)
		}),
		fyne.NewMenuItem("About", func() {
			dialog.ShowInformation("About FSAK GUI", "Colorful VPN-style control panel for FSAK client profiles.", ui.win)
		}),
	)

	ui.win.SetMainMenu(fyne.NewMainMenu(fileMenu, connectionMenu, profilesMenu, helpMenu))
}

func (ui *desktopUI) statTile(title string, value *widget.Label, bg color.Color) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	body := container.NewVBox(titleLabel, value)
	panel := canvas.NewRectangle(bg)
	return container.NewStack(panel, container.NewPadded(body))
}

func modeLabel(mode string) string {
	if mode == startModeTUN {
		return modeLabelTUN
	}
	return modeLabelProxy
}

func modeKey(label string) string {
	if label == modeLabelTUN {
		return startModeTUN
	}
	return startModeProxy
}

func (ui *desktopUI) selectedStartMode() string {
	if ui.modeSelect == nil {
		return startModeProxy
	}
	return modeKey(ui.modeSelect.Selected)
}

func (ui *desktopUI) openProfileManager() {
	running, _ := ui.state.runningSnapshot()
	if running {
		dialog.ShowInformation("Disconnect First", "Disconnect before editing profiles to avoid confusion.", ui.win)
		return
	}
	if ui.prof != nil {
		ui.prof.RequestFocus()
		return
	}

	managerWin := ui.app.NewWindow("Manage Profiles")
	managerWin.Resize(fyne.NewSize(500, 820))
	ui.prof = managerWin
	managerWin.SetOnClosed(func() {
		ui.prof = nil
		ui.refreshView()
	})

	selected, profiles := ui.state.snapshotProfiles()
	names := sortedProfileNames(profiles)

	profileSelect := widget.NewSelect(nil, nil)
	profileSelect.PlaceHolder = "Select existing profile"

	profileName := widget.NewEntry()
	profileName.SetPlaceHolder("example: office-gateway")

	addresses := widget.NewMultiLineEntry()
	addresses.SetPlaceHolder("1.1.1.1\n2.2.2.0/24\n3.3.3.3-4.4.4.4")
	addresses.SetMinRowsVisible(6)

	host := widget.NewEntry()
	host.SetPlaceHolder("cdn.example.com")

	tls := widget.NewCheck("Enable TLS", nil)

	sni := widget.NewEntry()
	sni.SetPlaceHolder("cdn.example.com")

	port := widget.NewEntry()
	port.SetPlaceHolder("80")

	proxyPort := widget.NewEntry()
	proxyPort.SetPlaceHolder("1080")

	secret := widget.NewPasswordEntry()
	secret.SetPlaceHolder("shared secret")

	fillForm := func(name string, cfg ClientConfig) {
		profileName.SetText(name)
		addresses.SetText(strings.Join(cfg.Addresses, "\n"))
		host.SetText(cfg.Host)
		tls.SetChecked(cfg.TLS)
		sni.SetText(cfg.SNI)
		port.SetText(strconv.Itoa(cfg.Port))
		proxyPort.SetText(strconv.Itoa(cfg.ProxyPort))
		secret.SetText(cfg.Secret)
	}

	clearForm := func() {
		profileName.SetText("")
		addresses.SetText("")
		host.SetText("")
		tls.SetChecked(false)
		sni.SetText("")
		port.SetText("80")
		proxyPort.SetText("1080")
		secret.SetText("")
	}

	readForm := func() (string, ClientConfig, error) {
		name := strings.TrimSpace(profileName.Text)
		if name == "" {
			return "", ClientConfig{}, errors.New("profile name is required")
		}

		serverPort, err := strconv.Atoi(strings.TrimSpace(port.Text))
		if err != nil {
			return "", ClientConfig{}, errors.New("server port must be a number")
		}
		localProxyPort, err := strconv.Atoi(strings.TrimSpace(proxyPort.Text))
		if err != nil {
			return "", ClientConfig{}, errors.New("local SOCKS5 port must be a number")
		}

		rawAddrs := strings.FieldsFunc(addresses.Text, func(r rune) bool {
			return r == ',' || r == '\n'
		})
		addrs := make([]string, 0, len(rawAddrs))
		for _, addr := range rawAddrs {
			trimmed := strings.TrimSpace(addr)
			if trimmed != "" {
				addrs = append(addrs, trimmed)
			}
		}

		cfg := ClientConfig{
			Addresses: addrs,
			Host:      strings.TrimSpace(host.Text),
			TLS:       tls.Checked,
			SNI:       strings.TrimSpace(sni.Text),
			Port:      serverPort,
			ProxyPort: localProxyPort,
			Secret:    strings.TrimSpace(secret.Text),
		}
		normalized, err := normalizeConfig(cfg)
		if err != nil {
			return "", ClientConfig{}, err
		}
		return name, normalized, nil
	}

	refreshOptions := func(pick string) {
		names = sortedProfileNames(profiles)
		profileSelect.Options = names
		profileSelect.Refresh()
		if pick != "" {
			profileSelect.SetSelected(pick)
		}
	}

	profileSelect.OnChanged = func(name string) {
		if name == "" {
			return
		}
		cfg, ok := profiles[name]
		if !ok {
			return
		}
		fillForm(name, cfg)
	}

	newBtn := widget.NewButtonWithIcon("New", theme.ContentAddIcon(), func() {
		profileSelect.ClearSelected()
		clearForm()
	})
	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		name, cfg, err := readForm()
		if err != nil {
			dialog.ShowError(err, managerWin)
			return
		}
		ui.state.mu.Lock()
		ui.state.profiles[name] = cfg
		ui.state.selected = name
		ui.state.lastErr = ""
		err = ui.state.saveProfilesLocked()
		ui.state.mu.Unlock()
		if err != nil {
			dialog.ShowError(err, managerWin)
			return
		}
		selected, profiles = ui.state.snapshotProfiles()
		refreshOptions(selected)
		ui.refreshView()
	})
	deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		name := strings.TrimSpace(profileSelect.Selected)
		if name == "" {
			name = strings.TrimSpace(profileName.Text)
		}
		if name == "" {
			dialog.ShowError(errors.New("select a profile to delete"), managerWin)
			return
		}
		dialog.NewConfirm("Delete Profile", fmt.Sprintf("Delete profile '%s'?", name), func(ok bool) {
			if !ok {
				return
			}
			ui.state.mu.Lock()
			if _, exists := ui.state.profiles[name]; !exists {
				ui.state.mu.Unlock()
				dialog.ShowError(errors.New("profile not found"), managerWin)
				return
			}
			delete(ui.state.profiles, name)
			if len(ui.state.profiles) == 0 {
				ui.state.selected = ""
			} else if ui.state.selected == name {
				ui.state.selected = sortedProfileNames(ui.state.profiles)[0]
			}
			err := ui.state.saveProfilesLocked()
			ui.state.mu.Unlock()
			if err != nil {
				dialog.ShowError(err, managerWin)
				return
			}
			selected, profiles = ui.state.snapshotProfiles()
			if len(profiles) == 0 {
				clearForm()
				profileSelect.ClearSelected()
			} else {
				refreshOptions(selected)
				if cfg, ok := profiles[selected]; ok {
					fillForm(selected, cfg)
				}
			}
			ui.refreshView()
		}, managerWin).Show()
	})
	saveBtn.Importance = widget.HighImportance
	deleteBtn.Importance = widget.DangerImportance

	form := container.NewVBox(
		widget.NewLabelWithStyle("Profile Manager", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Select existing profile"),
		profileSelect,
		container.NewGridWithColumns(3, newBtn, saveBtn, deleteBtn),
		widget.NewSeparator(),
		widget.NewLabel("Profile name"),
		profileName,
		widget.NewLabel("Server addresses (one per line or comma-separated)"),
		addresses,
		widget.NewLabel("Host Header"),
		host,
		tls,
		widget.NewLabel("SNI (required when TLS is enabled)"),
		sni,
		widget.NewLabel("Server Port"),
		port,
		widget.NewLabel("Local SOCKS5 Port"),
		proxyPort,
		widget.NewLabel("Shared Secret"),
		secret,
	)

	scroller := container.NewVScroll(form)
	doneBtn := widget.NewButton("Done", func() {
		managerWin.Close()
	})
	doneBtn.Importance = widget.HighImportance
	managerWin.SetContent(container.NewPadded(container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), doneBtn), nil, nil, scroller)))
	managerWin.Show()

	refreshOptions(selected)
	if selected != "" {
		if cfg, ok := profiles[selected]; ok {
			fillForm(selected, cfg)
			return
		}
	}
	if len(names) > 0 {
		first := names[0]
		if cfg, ok := profiles[first]; ok {
			refreshOptions(first)
			fillForm(first, cfg)
			return
		}
	}
	clearForm()
}

func (ui *desktopUI) bindClose() {
	ui.win.SetCloseIntercept(func() {
		running, active := ui.state.runningSnapshot()
		if !running {
			ui.state.mu.Lock()
			_ = ui.state.saveProfilesLocked()
			ui.state.mu.Unlock()
			ui.win.Close()
			return
		}
		msg := fmt.Sprintf("Profile '%s' is running. Stop it and exit?", active)
		dialog.NewConfirm("Exit", msg, func(ok bool) {
			if !ok {
				return
			}
			if err := ui.stopRunnerWithRetry(); err != nil {
				dialog.ShowError(err, ui.win)
				return
			}
			ui.state.mu.Lock()
			_ = ui.state.saveProfilesLocked()
			ui.state.mu.Unlock()
			ui.win.Close()
		}, ui.win).Show()
	})
}

func (ui *desktopUI) refreshView() {
	selected, names := ui.state.profileListSnapshot()
	ui.suspendProfileChange = true
	if !stringSlicesEqual(ui.profileSelect.Options, names) {
		ui.profileSelect.Options = append([]string(nil), names...)
		ui.profileSelect.Refresh()
	}

	if selected != "" {
		if ui.profileSelect.Selected != selected {
			ui.profileSelect.SetSelected(selected)
		}
	} else if len(names) > 0 {
		if ui.profileSelect.Selected != names[0] {
			ui.profileSelect.SetSelected(names[0])
		}
		ui.state.mu.Lock()
		ui.state.selected = names[0]
		ui.state.mu.Unlock()
	} else {
		if ui.profileSelect.Selected != "" {
			ui.profileSelect.ClearSelected()
		}
	}
	ui.suspendProfileChange = false

	ui.refreshStatus()
}

func (ui *desktopUI) setConfigEditable(enabled bool) {
	if enabled {
		ui.profileSelect.Enable()
		ui.modeSelect.Enable()
		ui.manageBtn.Enable()
		return
	}

	ui.profileSelect.Disable()
	ui.modeSelect.Disable()
	ui.manageBtn.Disable()
}

func (ui *desktopUI) refreshStatus() {
	selected, running, active, activeMode, started, lastErr, cfg, hasCfg := ui.state.statusSnapshot()
	activeName := selected
	mode := ui.selectedStartMode()
	if running {
		activeName = active
		mode = activeMode
	}

	if running {
		ui.statusLabel.SetText("Connected")
		ui.runtimeLabel.SetText(fmt.Sprintf("Profile: %s\nMode: %s\nStarted: %s", active, modeLabel(mode), started.Format(time.RFC1123)))
		ui.connectBtn.SetText("Disconnect")
		ui.connectBtn.SetIcon(theme.MediaStopIcon())
		ui.connectBtn.Importance = widget.DangerImportance
		ui.statusDot.FillColor = color.NRGBA{R: 0x12, G: 0xB7, B: 0x6A, A: 0xFF}
		ui.statusDot.Refresh()
		ui.statusPanel.FillColor = color.NRGBA{R: 0xE7, G: 0xF9, B: 0xED, A: 0xFF}
		ui.statusPanel.Refresh()
	} else {
		ui.statusLabel.SetText("Disconnected")
		if activeName == "" {
			ui.runtimeLabel.SetText("Tunnel is offline")
		} else {
			ui.runtimeLabel.SetText(fmt.Sprintf("Ready profile: %s\nMode: %s", activeName, modeLabel(mode)))
		}
		if mode == startModeTUN {
			ui.connectBtn.SetText("Start TUN")
		} else {
			ui.connectBtn.SetText("Start Proxy")
		}
		ui.connectBtn.SetIcon(theme.MediaPlayIcon())
		ui.connectBtn.Importance = widget.HighImportance
		ui.statusDot.FillColor = color.NRGBA{R: 0xD9, G: 0x2D, B: 0x20, A: 0xFF}
		ui.statusDot.Refresh()
		ui.statusPanel.FillColor = color.NRGBA{R: 0xFF, G: 0xEE, B: 0xEE, A: 0xFF}
		ui.statusPanel.Refresh()
	}

	if hasCfg {
		ui.activeProfileStat.SetText(activeName)
		ui.serverStat.SetText(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
		ui.proxyStat.SetText(fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort))
		ui.addressStat.SetText(strconv.Itoa(len(cfg.Addresses)))
	} else {
		ui.activeProfileStat.SetText("-")
		ui.serverStat.SetText("-")
		ui.proxyStat.SetText("-")
		ui.addressStat.SetText("-")
	}

	ui.setConfigEditable(!running)
	if strings.TrimSpace(lastErr) == "" {
		ui.errorLabel.SetText("No alerts.")
	} else {
		ui.errorLabel.SetText("Alert: " + lastErr)
	}
}

func (ui *desktopUI) selectedProfileConfig() (string, ClientConfig, error) {
	selected, cfg, ok := ui.state.selectedProfileConfig()
	if selected == "" {
		return "", ClientConfig{}, errors.New("no profile selected, click Manage Profiles to create one")
	}
	if !ok {
		return "", ClientConfig{}, errors.New("selected profile not found")
	}
	return selected, cfg, nil
}

func (ui *desktopUI) onConnectToggle() {
	running, _ := ui.state.runningSnapshot()
	if running {
		ui.onStop()
		return
	}
	ui.onStart()
}

func (ui *desktopUI) onStartProxy() {
	ui.modeSelect.SetSelected(modeLabelProxy)
	ui.onStart()
}

func (ui *desktopUI) onStartTUN() {
	ui.modeSelect.SetSelected(modeLabelTUN)
	ui.onStart()
}

func (ui *desktopUI) onStart() {
	name, cfg, err := ui.selectedProfileConfig()
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}

	mode := ui.selectedStartMode()
	if err := ui.state.startRunner(name, cfg, mode); err != nil {
		ui.state.mu.Lock()
		ui.state.lastErr = err.Error()
		ui.state.mu.Unlock()
		dialog.ShowError(err, ui.win)
		ui.refreshStatus()
		return
	}

	ui.refreshView()
	go ui.watchRunner()
}

func (ui *desktopUI) onStop() {
	if err := ui.stopRunnerWithRetry(); err != nil {
		ui.state.mu.Lock()
		ui.state.lastErr = err.Error()
		ui.state.mu.Unlock()
		dialog.ShowError(err, ui.win)
	}
	ui.refreshStatus()
}

func (ui *desktopUI) stopRunnerWithRetry() error {
	err := ui.state.stopRunner(4 * time.Second)
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ui.state.stopRunner(20 * time.Second)
	}
	return err
}

func (ui *desktopUI) watchRunner() {
	r := ui.state.runnerSnapshot()
	if r == nil {
		return
	}

	err := <-r.done
	_ = r.cleanup(4 * time.Second)

	ui.state.mu.Lock()
	if ui.state.runner == r {
		ui.state.runner = nil
	}
	if err != nil {
		ui.state.lastErr = err.Error()
	}
	ui.state.mu.Unlock()

	ui.refreshStatus()
}

func runTunHelper(args []string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("TUN helper currently supports macOS only")
	}

	var proxyPort int
	var tunDevice string
	var bindInterface string
	var bypassRaw string

	fs := flag.NewFlagSet("fsak-tun-helper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&proxyPort, "proxy-port", 0, "local SOCKS5 port")
	fs.StringVar(&tunDevice, "device", "utun233", "TUN device name")
	fs.StringVar(&bindInterface, "interface", "", "physical egress interface")
	fs.StringVar(&bypassRaw, "bypass", "", "comma separated server IPs/CIDRs to bypass tunnel")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if proxyPort < 1 || proxyPort > 65535 {
		return errors.New("invalid proxy-port for TUN helper")
	}

	defaultIface, defaultGateway, err := detectDefaultRouteDarwin()
	if err != nil {
		return fmt.Errorf("failed to detect default route: %w", err)
	}
	if bindInterface == "" {
		bindInterface = defaultIface
	}
	if strings.TrimSpace(defaultGateway) == "" {
		return errors.New("default gateway not found for TUN setup")
	}

	bypassEntries := splitBypassEntries(bypassRaw)

	key := &engine.Key{
		MTU:       1500,
		Proxy:     fmt.Sprintf("socks5://127.0.0.1:%d", proxyPort),
		Device:    tunDevice,
		Interface: bindInterface,
		LogLevel:  "warn",
	}
	engine.Insert(key)
	engine.Start()
	defer engine.Stop()

	cleanup, err := setupDarwinTunnelRoutes(tunDevice, defaultGateway, bypassEntries)
	if err != nil {
		return fmt.Errorf("failed to configure tunnel routes: %w", err)
	}
	defer func() {
		_ = cleanup()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh
	return nil
}

func splitBypassEntries(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func detectDefaultRouteDarwin() (iface string, gateway string, err error) {
	out, err := runCommand("route", "-n", "get", "default")
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
		if strings.HasPrefix(line, "gateway:") {
			gateway = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		}
	}
	if iface == "" {
		return "", "", errors.New("default interface not found in route output")
	}
	if gateway == "" {
		return "", "", errors.New("default gateway not found in route output")
	}
	return iface, gateway, nil
}

func setupDarwinTunnelRoutes(tunDevice string, defaultGateway string, bypassEntries []string) (func() error, error) {
	if err := runCommandErr("ifconfig", tunDevice, "inet", "198.18.0.1", "198.18.0.1", "up"); err != nil {
		return nil, fmt.Errorf("ifconfig %s up failed (run GUI with elevated privileges): %w", tunDevice, err)
	}

	bypassRoutes := collectBypassRoutes(bypassEntries)
	for _, target := range bypassRoutes {
		_ = runCommandErr("route", "-n", "delete", target.kindFlag, target.value)
		if err := runCommandErr("route", "-n", "add", target.kindFlag, target.value, defaultGateway); err != nil {
			return nil, fmt.Errorf("failed to add bypass route %s %s via %s: %w", target.kindFlag, target.value, defaultGateway, err)
		}
	}

	if err := replaceDarwinSplitRoute("0.0.0.0/1", tunDevice); err != nil {
		return nil, err
	}
	if err := replaceDarwinSplitRoute("128.0.0.0/1", tunDevice); err != nil {
		return nil, err
	}

	return func() error {
		var errs []string
		if err := runCommandErr("route", "-n", "delete", "-net", "0.0.0.0/1", "-interface", tunDevice); err != nil {
			errs = append(errs, err.Error())
		}
		if err := runCommandErr("route", "-n", "delete", "-net", "128.0.0.0/1", "-interface", tunDevice); err != nil {
			errs = append(errs, err.Error())
		}
		for _, target := range bypassRoutes {
			if err := runCommandErr("route", "-n", "delete", target.kindFlag, target.value); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if err := runCommandErr("ifconfig", tunDevice, "down"); err != nil {
			errs = append(errs, err.Error())
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return nil
	}, nil
}

func replaceDarwinSplitRoute(cidr string, tunDevice string) error {
	_ = runCommandErr("route", "-n", "delete", "-net", cidr, "-interface", tunDevice)
	if err := runCommandErr("route", "-n", "add", "-net", cidr, "-interface", tunDevice); err != nil {
		return fmt.Errorf("route add %s via %s failed: %w", cidr, tunDevice, err)
	}
	return nil
}

type bypassRoute struct {
	kindFlag string
	value    string
}

func collectBypassRoutes(entries []string) []bypassRoute {
	seen := make(map[string]struct{})
	routes := make([]bypassRoute, 0, len(entries))

	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "-") {
			// IP range syntax is not mapped to route entries here.
			continue
		}

		if _, ipNet, err := net.ParseCIDR(raw); err == nil {
			key := "-net|" + ipNet.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			routes = append(routes, bypassRoute{kindFlag: "-net", value: ipNet.String()})
			continue
		}

		if ip := net.ParseIP(raw); ip != nil {
			ipStr := ip.String()
			key := "-host|" + ipStr
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			routes = append(routes, bypassRoute{kindFlag: "-host", value: ipStr})
		}
	}

	return routes
}

func runCommand(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed == "" {
			return "", fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), trimmed)
	}
	return trimmed, nil
}

func runCommandErr(name string, args ...string) error {
	_, err := runCommand(name, args...)
	return err
}

func normalizeConfig(cfg ClientConfig) (ClientConfig, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.SNI = strings.TrimSpace(cfg.SNI)
	cfg.Secret = strings.TrimSpace(cfg.Secret)

	addrs := make([]string, 0, len(cfg.Addresses))
	for _, addr := range cfg.Addresses {
		trimmed := strings.TrimSpace(addr)
		if trimmed != "" {
			addrs = append(addrs, trimmed)
		}
	}
	cfg.Addresses = addrs

	if len(cfg.Addresses) == 0 {
		return cfg, errors.New("at least one address is required")
	}
	if cfg.Host == "" {
		return cfg, errors.New("host is required")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, errors.New("port must be between 1 and 65535")
	}
	if cfg.ProxyPort < 1 || cfg.ProxyPort > 65535 {
		return cfg, errors.New("proxy_port must be between 1 and 65535")
	}
	if cfg.Secret == "" {
		return cfg, errors.New("secret is required")
	}
	if cfg.TLS && cfg.SNI == "" {
		return cfg, errors.New("sni is required when tls is enabled")
	}
	return cfg, nil
}

func (c ClientConfig) toInternal() config.Config {
	return config.Config{
		Addresses: c.Addresses,
		Host:      c.Host,
		TLS:       c.TLS,
		SNI:       c.SNI,
		Port:      c.Port,
		ProxyPort: c.ProxyPort,
		Secret:    c.Secret,
	}
}

func fromInternal(c config.Config) ClientConfig {
	return ClientConfig{
		Addresses: c.Addresses,
		Host:      c.Host,
		TLS:       c.TLS,
		SNI:       c.SNI,
		Port:      c.Port,
		ProxyPort: c.ProxyPort,
		Secret:    c.Secret,
	}
}

func sortedProfileNames(m map[string]ClientConfig) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
