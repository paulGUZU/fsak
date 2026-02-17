package ui

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/paulGUZU/fsak/cmd/gui/internal/models"
	"github.com/paulGUZU/fsak/cmd/gui/internal/services"
)

// ProfileManager handles the profile management dialog
type ProfileManager struct {
	window   fyne.Window
	state    *models.GUIState
	svc      *services.ProfileService
	onClose  func()

	// Form fields
	profileSelect *widget.Select
	nameEntry     *widget.Entry
	addresses     *widget.Entry
	host          *widget.Entry
	tls           *widget.Check
	sni           *widget.Entry
	port          *widget.Entry
	proxyPort     *widget.Entry
	secret        *widget.Entry

	// Current profiles cache
	profiles map[string]models.ClientConfig
	selected string
}

// NewProfileManager creates a new profile manager dialog
func NewProfileManager(app fyne.App, parent fyne.Window, state *models.GUIState, svc *services.ProfileService) *ProfileManager {
	pm := &ProfileManager{
		window:   app.NewWindow("Manage Profiles"),
		state:    state,
		svc:      svc,
		profiles: state.Profiles(),
		selected: state.Selected(),
	}

	pm.setupUI()
	return pm
}

// Window returns the profile manager window
func (pm *ProfileManager) Window() fyne.Window {
	return pm.window
}

// Show displays the profile manager
func (pm *ProfileManager) Show() {
	pm.window.Show()
}

// SetOnClose sets the close callback
func (pm *ProfileManager) SetOnClose(fn func()) {
	pm.onClose = fn
	pm.window.SetOnClosed(fn)
}

func (pm *ProfileManager) setupUI() {
	pm.window.Resize(fyne.NewSize(models.ProfileManagerWidth, models.ProfileManagerHeight))
	pm.window.CenterOnScreen()

	// Create form fields
	pm.profileSelect = widget.NewSelect(nil, pm.onProfileSelected)
	pm.profileSelect.PlaceHolder = "Select existing profile..."

	pm.nameEntry = widget.NewEntry()
	pm.nameEntry.SetPlaceHolder("e.g., office-gateway")

	pm.addresses = widget.NewMultiLineEntry()
	pm.addresses.SetPlaceHolder("1.1.1.1\n2.2.2.0/24\n3.3.3.3-4.4.4.4")
	pm.addresses.SetMinRowsVisible(4)

	pm.host = widget.NewEntry()
	pm.host.SetPlaceHolder("cdn.example.com")

	pm.tls = widget.NewCheck("Enable TLS encryption", pm.onTLSChanged)

	pm.sni = widget.NewEntry()
	pm.sni.SetPlaceHolder("cdn.example.com (required if TLS enabled)")
	pm.sni.Disable()

	pm.port = widget.NewEntry()
	pm.port.SetPlaceHolder("80")

	pm.proxyPort = widget.NewEntry()
	pm.proxyPort.SetPlaceHolder("1080")

	pm.secret = widget.NewPasswordEntry()
	pm.secret.SetPlaceHolder("shared secret")

	// Action buttons
	newBtn := widget.NewButtonWithIcon("New", theme.ContentAddIcon(), pm.onNew)
	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), pm.onSave)
	deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), pm.onDelete)
	doneBtn := widget.NewButtonWithIcon("Done", theme.ConfirmIcon(), func() {
		pm.window.Close()
	})

	saveBtn.Importance = widget.HighImportance
	deleteBtn.Importance = widget.DangerImportance
	doneBtn.Importance = widget.HighImportance

	// Button row
	buttonRow := container.NewGridWithColumns(3, newBtn, saveBtn, deleteBtn)

	// Build form with better spacing
	form := container.NewVBox(
		widget.NewLabelWithStyle("Profile Configuration", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		
		// Profile selection
		widget.NewLabelWithStyle("Select Profile", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pm.profileSelect,
		buttonRow,
		widget.NewSeparator(),
		
		// Profile details
		widget.NewLabelWithStyle("Profile Name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pm.nameEntry,
		
		widget.NewLabelWithStyle("Server Addresses", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("One per line or comma-separated"),
		pm.addresses,
		
		widget.NewLabelWithStyle("Host Header", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pm.host,
		
		widget.NewSeparator(),
		pm.tls,
		
		widget.NewLabelWithStyle("SNI (Server Name Indication)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pm.sni,
		
		widget.NewSeparator(),
		
		container.NewGridWithColumns(2,
			container.NewVBox(
				widget.NewLabelWithStyle("Server Port", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				pm.port,
			),
			container.NewVBox(
				widget.NewLabelWithStyle("Local SOCKS5 Port", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				pm.proxyPort,
			),
		),
		
		widget.NewLabelWithStyle("Shared Secret", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		pm.secret,
	)

	// Scrollable content
	scroller := container.NewVScroll(form)
	
	// Footer with done button
	footer := container.NewHBox(layout.NewSpacer(), doneBtn)
	
	// Main layout
	content := container.NewBorder(nil, container.NewPadded(footer), nil, nil, container.NewPadded(scroller))

	pm.window.SetContent(content)

	// Initialize with current profiles
	pm.refreshProfileList()
	if pm.selected != "" {
		if cfg, ok := pm.profiles[pm.selected]; ok {
			pm.fillForm(pm.selected, cfg)
		}
	}
}

func (pm *ProfileManager) onProfileSelected(name string) {
	if name == "" {
		return
	}
	cfg, ok := pm.profiles[name]
	if !ok {
		return
	}
	pm.fillForm(name, cfg)
}

func (pm *ProfileManager) onTLSChanged(checked bool) {
	if checked {
		pm.sni.Enable()
	} else {
		pm.sni.Disable()
		pm.sni.SetText("")
	}
}

func (pm *ProfileManager) onNew() {
	pm.profileSelect.ClearSelected()
	pm.clearForm()
	pm.nameEntry.FocusGained()
}

func (pm *ProfileManager) onSave() {
	name, cfg, err := pm.readForm()
	if err != nil {
		dialog.ShowError(err, pm.window)
		return
	}

	// Update state
	pm.state.SetProfile(name, cfg)

	// Persist
	profiles := pm.state.Profiles()
	selected := pm.state.Selected()
	if err := pm.svc.SaveProfiles(selected, profiles); err != nil {
		dialog.ShowError(err, pm.window)
		return
	}

	// Update local cache
	pm.profiles = profiles
	pm.selected = selected

	// Refresh UI
	pm.refreshProfileList()
	pm.profileSelect.SetSelected(name)
	
	dialog.ShowInformation("Saved", fmt.Sprintf("Profile '%s' saved successfully.", name), pm.window)
}

func (pm *ProfileManager) onDelete() {
	name := pm.profileSelect.Selected
	if name == "" {
		name = pm.nameEntry.Text
	}
	name = models.SanitizeString(name)

	if name == "" {
		dialog.ShowError(errors.New("select a profile to delete"), pm.window)
		return
	}

	dialog.NewConfirm("Delete Profile",
		fmt.Sprintf("Are you sure you want to delete '%s'?", name),
		func(ok bool) {
			if !ok {
				return
			}
			pm.doDelete(name)
		}, pm.window).Show()
}

func (pm *ProfileManager) doDelete(name string) {
	if _, exists := pm.profiles[name]; !exists {
		dialog.ShowError(errors.New("profile not found"), pm.window)
		return
	}

	if !pm.state.DeleteProfile(name) {
		dialog.ShowError(errors.New("failed to delete profile"), pm.window)
		return
	}

	// Persist
	profiles := pm.state.Profiles()
	selected := pm.state.Selected()
	if err := pm.svc.SaveProfiles(selected, profiles); err != nil {
		dialog.ShowError(err, pm.window)
		return
	}

	// Update local cache
	pm.profiles = profiles
	pm.selected = selected

	// Update UI
	pm.refreshProfileList()
	if len(profiles) == 0 {
		pm.clearForm()
		pm.profileSelect.ClearSelected()
	} else {
		if cfg, ok := profiles[selected]; ok {
			pm.fillForm(selected, cfg)
		} else {
			pm.clearForm()
		}
	}
}

func (pm *ProfileManager) fillForm(name string, cfg models.ClientConfig) {
	pm.nameEntry.SetText(name)
	pm.addresses.SetText(models.FormatAddresses(cfg.Addresses))
	pm.host.SetText(cfg.Host)
	pm.tls.SetChecked(cfg.TLS)
	pm.sni.SetText(cfg.SNI)
	pm.port.SetText(fmt.Sprintf("%d", cfg.Port))
	pm.proxyPort.SetText(fmt.Sprintf("%d", cfg.ProxyPort))
	pm.secret.SetText(cfg.Secret)

	pm.onTLSChanged(cfg.TLS)
}

func (pm *ProfileManager) clearForm() {
	pm.nameEntry.SetText("")
	pm.addresses.SetText("")
	pm.host.SetText("")
	pm.tls.SetChecked(false)
	pm.sni.SetText("")
	pm.port.SetText("80")
	pm.proxyPort.SetText("1080")
	pm.secret.SetText("")
	pm.onTLSChanged(false)
}

func (pm *ProfileManager) readForm() (string, models.ClientConfig, error) {
	name := models.SanitizeString(pm.nameEntry.Text)
	if name == "" {
		return "", models.ClientConfig{}, errors.New("profile name is required")
	}

	port, err := models.ParsePort(pm.port.Text)
	if err != nil {
		return "", models.ClientConfig{}, fmt.Errorf("server %w", err)
	}

	proxyPort, err := models.ParsePort(pm.proxyPort.Text)
	if err != nil {
		return "", models.ClientConfig{}, fmt.Errorf("local SOCKS5 %w", err)
	}

	addrs := models.ParseAddresses(pm.addresses.Text)

	cfg := models.ClientConfig{
		Addresses: addrs,
		Host:      models.SanitizeString(pm.host.Text),
		TLS:       pm.tls.Checked,
		SNI:       models.SanitizeString(pm.sni.Text),
		Port:      port,
		ProxyPort: proxyPort,
		Secret:    models.SanitizeString(pm.secret.Text),
	}

	normalized, err := cfg.Normalize()
	if err != nil {
		return "", models.ClientConfig{}, err
	}

	return name, normalized, nil
}

func (pm *ProfileManager) refreshProfileList() {
	names := models.SortedProfileNames(pm.profiles)
	pm.profileSelect.Options = names
	pm.profileSelect.Refresh()
}
