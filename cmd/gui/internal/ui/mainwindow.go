package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/paulGUZU/fsak/cmd/gui/internal/app"
	"github.com/paulGUZU/fsak/cmd/gui/internal/models"
	"github.com/paulGUZU/fsak/cmd/gui/internal/services"
)

// MainWindow represents the main application window
type MainWindow struct {
	window fyne.Window
	app    fyne.App
	state  *models.GUIState
	svc    struct {
		profile *services.ProfileService
		runner  *services.RunnerService
	}

	// UI Components
	profileSelect *widget.Select
	modeSelect    *widget.Select
	connectBtn    *ConnectionButton
	refreshBtn    *widget.Button
	manageBtn     *widget.Button

	statusDot    *StatusDot
	statusLabel  *widget.Label
	statusPanel  *StatusPanel
	runtimeLabel *widget.Label
	errorLabel   *widget.Label

	statTiles struct {
		profile *StatTile
		proxy   *StatTile
		server  *StatTile
		address *StatTile
	}

	profileManager fyne.Window
}

// NewMainWindow creates a new main window
func NewMainWindow(a fyne.App, state *models.GUIState, profileSvc *services.ProfileService, runnerSvc *services.RunnerService) *MainWindow {
	w := a.NewWindow(models.AppName)
	w.SetMaster()

	mw := &MainWindow{
		window: w,
		app:    a,
		state:  state,
	}
	mw.svc.profile = profileSvc
	mw.svc.runner = runnerSvc

	mw.setupUI()
	mw.setupBindings()
	mw.setupMenu()
	mw.setupCloseHandler()

	return mw
}

// Show displays the window
func (mw *MainWindow) Show() {
	mw.refreshProfiles()
	mw.window.Show()
}

// ShowAndRun runs the application
func (mw *MainWindow) ShowAndRun() {
	mw.Show()
	mw.window.ShowAndRun()
}

func (mw *MainWindow) setupUI() {
	// Profile selector with better styling
	mw.profileSelect = widget.NewSelect([]string{}, func(name string) {
		if name != "" {
			mw.state.SetSelected(name)
			mw.updateStats()
		}
	})
	mw.profileSelect.PlaceHolder = "Select a profile..."

	// Mode selector
	mw.modeSelect = widget.NewSelect([]string{models.ModeLabelProxy, models.ModeLabelTUN}, nil)
	mw.modeSelect.SetSelected(models.ModeLabelProxy)

	// Manage Profiles button - full width
	mw.manageBtn = widget.NewButtonWithIcon("Manage Profiles", theme.SettingsIcon(), mw.openProfileManager)
	mw.manageBtn.Importance = widget.MediumImportance

	// Refresh button - compact
	mw.refreshBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		mw.refreshProfiles()
	})
	mw.refreshBtn.Importance = widget.LowImportance

	// Connect button - large and prominent
	mw.connectBtn = NewConnectionButton()
	mw.connectBtn.SetOnConnect(mw.onConnect)
	mw.connectBtn.SetOnDisconnect(mw.onDisconnect)
	mw.connectBtn.Importance = widget.HighImportance

	// Status components
	mw.statusDot = NewStatusDot()
	mw.statusLabel = widget.NewLabelWithStyle("Disconnected", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	mw.runtimeLabel = widget.NewLabelWithStyle("Select a profile and click Connect", fyne.TextAlignCenter, fyne.TextStyle{})
	mw.runtimeLabel.Wrapping = fyne.TextWrapOff

	mw.errorLabel = widget.NewLabelWithStyle("No alerts.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	mw.errorLabel.Wrapping = fyne.TextWrapOff

	mw.statusPanel = NewStatusPanel()

	// Stat tiles - get appropriate colors for current theme
	isDark := mw.isDarkMode()
	tileColors := app.TileColors(isDark)
	mw.statTiles.profile = NewStatTile("Profile", "-", tileColors.Profile)
	mw.statTiles.proxy = NewStatTile("Local SOCKS5", "-", tileColors.Proxy)
	mw.statTiles.server = NewStatTile("Server", "-", tileColors.Server)
	mw.statTiles.address = NewStatTile("Addresses", "-", tileColors.Address)

	// Build layout
	mw.buildLayout()
}

func (mw *MainWindow) isDarkMode() bool {
	// Check if the app is using dark theme
	return mw.app.Settings().Theme().Color(theme.ColorNameBackground, theme.VariantDark) !=
		mw.app.Settings().Theme().Color(theme.ColorNameBackground, theme.VariantLight)
}

func (mw *MainWindow) buildLayout() {
	// ===== Profile Selection Section =====
	profileForm := container.NewVBox(
		container.NewHBox(
			widget.NewLabelWithStyle("Profile", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
		),
		mw.profileSelect,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabelWithStyle("Mode", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
		),
		mw.modeSelect,
		widget.NewSeparator(),
		container.NewPadded(mw.manageBtn),
	)
	profileCard := widget.NewCard("", "", profileForm)

	// ===== Connection Status Section =====
	// Status header with dot
	statusHeader := container.NewHBox(
		mw.statusDot,
		widget.NewLabelWithStyle("Status", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		mw.statusLabel,
	)

	// Connection button - centered and large
	buttonContainer := container.NewHBox(
		layout.NewSpacer(),
		container.NewPadded(mw.connectBtn),
		layout.NewSpacer(),
	)

	// Status content
	statusContent := container.NewVBox(
		container.NewPadded(
			container.NewVBox(
				widget.NewLabelWithStyle("FSAK VPN Dashboard", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewSeparator(),
				statusHeader,
				container.NewHBox(layout.NewSpacer(), mw.runtimeLabel, layout.NewSpacer()),
				buttonContainer,
				container.NewHBox(layout.NewSpacer(), mw.errorLabel, layout.NewSpacer()),
			),
		),
	)
	mw.statusPanel.SetContent(statusContent)

	connectionCard := widget.NewCard("Connection", "", mw.statusPanel)

	// ===== Stats Section =====
	statsGrid := container.NewGridWithColumns(2,
		container.NewPadded(mw.statTiles.profile),
		container.NewPadded(mw.statTiles.proxy),
		container.NewPadded(mw.statTiles.server),
		container.NewPadded(mw.statTiles.address),
	)
	statsCard := widget.NewCard("Session Overview", "", statsGrid)

	// ===== Main Layout =====
	// Create scrollable content with proper spacing
	content := container.NewVBox(
		profileCard,
		widget.NewSeparator(),
		connectionCard,
		widget.NewSeparator(),
		statsCard,
	)

	// Add padding around the whole content
	paddedContent := container.NewPadded(content)
	
	// Create scroll container
	scroll := container.NewVScroll(paddedContent)
	scroll.SetMinSize(fyne.NewSize(models.DefaultWindowWidth-24, models.DefaultWindowHeight-50))

	// Set window content
	mw.window.SetContent(scroll)
	mw.window.Resize(fyne.NewSize(models.DefaultWindowWidth, models.DefaultWindowHeight))
	mw.window.SetFixedSize(false)
	mw.window.CenterOnScreen()
}

func (mw *MainWindow) setupBindings() {
	// Bind state to UI
	mw.state.SetProfileChangedCallback(func(name string) {
		mw.updateStats()
	})

	mw.state.SetStateChangedCallback(func(status models.ConnectionStatus) {
		mw.onConnectionStateChanged(status)
	})

	// Bind status dot
	mw.statusDot.Bind(mw.state.ConnectionState)

	// Bind connection button - directly sync with state
	mw.state.ConnectionState.AddListener(binding.NewDataListener(func() {
		status, _ := mw.state.ConnectionState.Get()
		mw.connectBtn.SetConnected(status == int(models.StatusConnected))
		mw.updateStatusUI(status)
	}))
	// Initial update
	mw.connectBtn.SetConnected(mw.state.IsRunning())

	// Bind error display
	mw.state.LastError.AddListener(binding.NewDataListener(func() {
		err, _ := mw.state.LastError.Get()
		if err == "" {
			mw.errorLabel.SetText("No alerts.")
			mw.errorLabel.Importance = widget.LowImportance
		} else {
			mw.errorLabel.SetText("⚠️  " + err)
			mw.errorLabel.Importance = widget.WarningImportance
		}
		mw.errorLabel.Refresh()
	}))
}

func (mw *MainWindow) setupMenu() {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Manage Profiles", mw.openProfileManager),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Refresh", mw.refreshProfiles),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Exit", func() { mw.window.Close() }),
	)

	connectionMenu := fyne.NewMenu("Connection",
		fyne.NewMenuItem("Start Proxy", mw.onStartProxy),
		fyne.NewMenuItem("Start TUN", mw.onStartTUN),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Connect / Disconnect", mw.onToggle),
		fyne.NewMenuItem("Stop", mw.onDisconnect),
	)

	profilesMenu := fyne.NewMenu("Profiles",
		fyne.NewMenuItem("Manage Profiles", mw.openProfileManager),
	)

	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("Quick Tips", func() {
			dialog.ShowInformation("Quick Tips",
				"1. Select a profile from the dropdown\n"+
					"2. Choose Proxy or TUN mode\n"+
					"3. Click Connect to start\n"+
					"4. Use Disconnect before switching profiles",
				mw.window,
			)
		}),
		fyne.NewMenuItem("About", func() {
			dialog.ShowInformation("About FSAK",
				"FSAK VPN Client v"+models.AppVersion+"\n\n"+
					"A high-performance SOCKS5 proxy client\n"+
					"for secure internet access.",
				mw.window)
		}),
	)

	mw.window.SetMainMenu(fyne.NewMainMenu(fileMenu, connectionMenu, profilesMenu, helpMenu))
}

func (mw *MainWindow) setupCloseHandler() {
	mw.window.SetCloseIntercept(func() {
		if mw.state.IsRunning() {
			profile := mw.state.RunningProfile()
			dialog.NewConfirm("Exit",
				fmt.Sprintf("Profile '%s' is running. Stop it and exit?", profile),
				func(ok bool) {
					if !ok {
						return
					}
					mw.onDisconnect()
					mw.saveAndExit()
				}, mw.window).Show()
		} else {
			mw.saveAndExit()
		}
	})
}

func (mw *MainWindow) saveAndExit() {
	profiles := mw.state.Profiles()
	selected := mw.state.Selected()
	if err := mw.svc.profile.SaveProfiles(selected, profiles); err != nil {
		fmt.Printf("Failed to save profiles: %v\n", err)
	}
	mw.window.Close()
}

func (mw *MainWindow) refreshProfiles() {
	selected, names := mw.state.Selected(), mw.state.ProfileNames()

	mw.profileSelect.Options = names
	mw.profileSelect.Refresh()

	if selected != "" {
		mw.profileSelect.SetSelected(selected)
	} else if len(names) > 0 {
		mw.profileSelect.SetSelected(names[0])
	}

	mw.updateStats()
}

func (mw *MainWindow) updateStats() {
	name, cfg, ok := mw.state.SelectedConfig()
	if !ok {
		mw.statTiles.profile.SetValue("—")
		mw.statTiles.proxy.SetValue("—")
		mw.statTiles.server.SetValue("—")
		mw.statTiles.address.SetValue("—")
		return
	}

	mw.statTiles.profile.SetValue(name)
	mw.statTiles.server.SetValue(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	mw.statTiles.proxy.SetValue(fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort))
	mw.statTiles.address.SetValue(fmt.Sprintf("%d", len(cfg.Addresses)))
}

func (mw *MainWindow) updateStatusUI(status int) {
	isDark := mw.isDarkMode()
	mw.statusPanel.SetIsDark(isDark)
	mw.statusPanel.SetStatus(status)

	// Update tile colors based on theme
	tileColors := app.TileColors(isDark)
	mw.statTiles.profile.SetTheme(isDark, tileColors.Profile)
	mw.statTiles.proxy.SetTheme(isDark, tileColors.Proxy)
	mw.statTiles.server.SetTheme(isDark, tileColors.Server)
	mw.statTiles.address.SetTheme(isDark, tileColors.Address)

	switch models.ConnectionStatus(status) {
	case models.StatusConnected:
		mw.statusLabel.SetText("Connected")
		mw.statusLabel.Importance = widget.SuccessImportance
		runner := mw.state.Runner()
		if runner != nil {
			mw.runtimeLabel.SetText(fmt.Sprintf("Profile: %s\nMode: %s\nStarted: %s",
				runner.ProfileName,
				runner.Mode,
				runner.StartedAt.Format("15:04:05")))
		}
		mw.setControlsEnabled(false)

	case models.StatusConnecting:
		mw.statusLabel.SetText("Connecting...")
		mw.statusLabel.Importance = widget.WarningImportance
		mw.runtimeLabel.SetText("Please wait...")
		mw.setControlsEnabled(false)

	case models.StatusDisconnecting:
		mw.statusLabel.SetText("Disconnecting...")
		mw.statusLabel.Importance = widget.WarningImportance
		mw.runtimeLabel.SetText("Cleaning up...")
		mw.setControlsEnabled(false)

	default:
		mw.statusLabel.SetText("Disconnected")
		mw.statusLabel.Importance = widget.MediumImportance
		name, _, _ := mw.state.SelectedConfig()
		if name == "" {
			mw.runtimeLabel.SetText("Select a profile and click Connect")
		} else {
			mode := mw.selectedMode()
			mw.runtimeLabel.SetText(fmt.Sprintf("Ready: %s (%s)", name, modeLabel(mode)))
		}
		mw.setControlsEnabled(true)
	}
	mw.statusLabel.Refresh()
}

func (mw *MainWindow) setControlsEnabled(enabled bool) {
	if enabled {
		mw.profileSelect.Enable()
		mw.modeSelect.Enable()
		mw.manageBtn.Enable()
	} else {
		mw.profileSelect.Disable()
		mw.modeSelect.Disable()
		mw.manageBtn.Disable()
	}
}

func (mw *MainWindow) onConnectionStateChanged(status models.ConnectionStatus) {
	// Triggered by state callback
}

func (mw *MainWindow) onConnect() {
	name, cfg, err := mw.getSelectedProfile()
	if err != nil {
		dialog.ShowError(err, mw.window)
		return
	}

	mode := mw.selectedMode()

	if mode == models.ModeTUN {
		if cfg.TLS && cfg.SNI == "" {
			dialog.ShowError(fmt.Errorf("SNI is required when TLS is enabled for TUN mode"), mw.window)
			return
		}
	}

	if err := mw.svc.runner.Start(services.StartOptions{
		ProfileName: name,
		Config:      cfg,
		Mode:        mode,
	}); err != nil {
		mw.state.SetError(err.Error())
		dialog.ShowError(err, mw.window)
		return
	}

	mw.svc.runner.Watch(func(err error) {
		if err != nil {
			mw.state.SetError(err.Error())
		}
	})
}

func (mw *MainWindow) onDisconnect() {
	if err := mw.svc.runner.Stop(); err != nil {
		mw.state.SetError(err.Error())
		dialog.ShowError(err, mw.window)
	}
}

func (mw *MainWindow) onToggle() {
	if mw.state.IsRunning() {
		mw.onDisconnect()
	} else {
		mw.onConnect()
	}
}

func (mw *MainWindow) onStartProxy() {
	mw.modeSelect.SetSelected(models.ModeLabelProxy)
	mw.onConnect()
}

func (mw *MainWindow) onStartTUN() {
	mw.modeSelect.SetSelected(models.ModeLabelTUN)
	mw.onConnect()
}

func (mw *MainWindow) getSelectedProfile() (string, models.ClientConfig, error) {
	name, cfg, ok := mw.state.SelectedConfig()
	if !ok {
		return "", models.ClientConfig{}, fmt.Errorf("no profile selected")
	}
	return name, cfg, nil
}

func (mw *MainWindow) selectedMode() models.ConnectionMode {
	if mw.modeSelect.Selected == models.ModeLabelTUN {
		return models.ModeTUN
	}
	return models.ModeProxy
}

func modeLabel(mode models.ConnectionMode) string {
	if mode == models.ModeTUN {
		return models.ModeLabelTUN
	}
	return models.ModeLabelProxy
}

func (mw *MainWindow) openProfileManager() {
	if mw.state.IsRunning() {
		dialog.ShowInformation("Disconnect First",
			"Please disconnect before editing profiles.",
			mw.window)
		return
	}

	if mw.profileManager != nil {
		mw.profileManager.RequestFocus()
		return
	}

	pm := NewProfileManager(mw.app, mw.window, mw.state, mw.svc.profile)
	mw.profileManager = pm.Window()
	pm.Window().SetOnClosed(func() {
		mw.profileManager = nil
		mw.refreshProfiles()
	})
	pm.Show()
}
