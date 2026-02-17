package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/paulGUZU/fsak/cmd/gui/internal/app"
)

// StatTile is a reusable statistics display widget with theme support
type StatTile struct {
	widget.BaseWidget
	
	title  string
	value  binding.String
	bg     color.Color
	isDark bool
	titleL *widget.Label
	valueL *widget.Label
	panel  *canvas.Rectangle
}

// NewStatTile creates a new stat tile
func NewStatTile(title string, initialValue string, bg color.Color) *StatTile {
	s := &StatTile{
		title: title,
		value: binding.NewString(),
		bg:    bg,
	}
	s.value.Set(initialValue)
	s.ExtendBaseWidget(s)
	return s
}

// Bind connects the tile to a data binding
func (s *StatTile) Bind(data binding.String) {
	s.value = data
	if s.valueL != nil {
		s.valueL.Bind(data)
	}
}

// SetValue updates the tile value
func (s *StatTile) SetValue(v string) {
	s.value.Set(v)
}

// SetTheme updates the tile for current theme
func (s *StatTile) SetTheme(isDark bool, bg color.Color) {
	s.isDark = isDark
	s.bg = bg
	if s.panel != nil {
		s.panel.FillColor = bg
		s.panel.Refresh()
	}
}

// CreateRenderer implements fyne.Widget
func (s *StatTile) CreateRenderer() fyne.WidgetRenderer {
	s.titleL = widget.NewLabelWithStyle(s.title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	s.valueL = widget.NewLabelWithStyle("-", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	s.valueL.Bind(s.value)
	
	s.panel = canvas.NewRectangle(s.bg)
	s.panel.CornerRadius = 8
	
	body := container.NewVBox(
		container.NewPadded(s.titleL),
		container.NewPadded(s.valueL),
	)
	content := container.NewStack(s.panel, body)
	
	return widget.NewSimpleRenderer(content)
}

// StatusPanel displays the connection status with theme-aware colored background
type StatusPanel struct {
	widget.BaseWidget
	
	isDark  bool
	status  binding.Int
	error   binding.String
	bg      *canvas.Rectangle
	content fyne.CanvasObject
}

// NewStatusPanel creates a new status panel
func NewStatusPanel() *StatusPanel {
	s := &StatusPanel{
		isDark: isDarkMode(),
		status: binding.NewInt(),
		error:  binding.NewString(),
	}
	s.ExtendBaseWidget(s)
	return s
}

func isDarkMode() bool {
	// Check system theme
	return theme.DefaultTheme().Color(theme.ColorNameBackground, theme.VariantDark) != 
		theme.DefaultTheme().Color(theme.ColorNameBackground, theme.VariantLight)
}

// SetStatus updates the panel status
func (s *StatusPanel) SetStatus(status int) {
	s.status.Set(status)
	s.updateBackground()
}

// SetError sets the error message
func (s *StatusPanel) SetError(err string) {
	s.error.Set(err)
}

// BindStatus connects to a status binding
func (s *StatusPanel) BindStatus(data binding.Int) {
	s.status = data
	data.AddListener(binding.NewDataListener(func() {
		s.updateBackground()
	}))
}

// SetIsDark updates the theme
func (s *StatusPanel) SetIsDark(isDark bool) {
	s.isDark = isDark
	s.updateBackground()
}

func (s *StatusPanel) updateBackground() {
	status, _ := s.status.Get()
	connected := status == 2 // Connected
	
	s.bg.FillColor = app.PanelBackground(s.isDark, connected)
	s.bg.Refresh()
}

// CreateRenderer implements fyne.Widget
func (s *StatusPanel) CreateRenderer() fyne.WidgetRenderer {
	s.bg = canvas.NewRectangle(app.PanelBackground(s.isDark, false))
	s.bg.CornerRadius = 12
	
	if s.content == nil {
		s.content = widget.NewLabel("Loading...")
	}
	
	content := container.NewStack(s.bg, container.NewPadded(s.content))
	return widget.NewSimpleRenderer(content)
}

// SetContent updates the panel content
func (s *StatusPanel) SetContent(content fyne.CanvasObject) {
	s.content = content
	s.Refresh()
}

// StatusDot is a colored dot indicating connection status
type StatusDot struct {
	widget.BaseWidget
	
	status binding.Int
	isDark bool
	dot    *canvas.Circle
}

// NewStatusDot creates a new status dot
func NewStatusDot() *StatusDot {
	s := &StatusDot{
		isDark: isDarkMode(),
		status: binding.NewInt(),
	}
	s.ExtendBaseWidget(s)
	return s
}

// Bind connects the dot to a status binding
func (s *StatusDot) Bind(data binding.Int) {
	s.status = data
	data.AddListener(binding.NewDataListener(func() {
		s.updateColor()
	}))
}

// SetStatus updates the dot status
func (s *StatusDot) SetStatus(status int) {
	s.status.Set(status)
	s.updateColor()
}

func (s *StatusDot) updateColor() {
	status, _ := s.status.Get()
	colors := app.StatusColors(s.isDark)
	
	switch status {
	case 2: // Connected
		s.dot.FillColor = colors.Connected
	case 1: // Connecting
		s.dot.FillColor = colors.Connecting
	default: // Disconnected
		s.dot.FillColor = colors.Disconnected
	}
	s.dot.Refresh()
}

// CreateRenderer implements fyne.Widget
func (s *StatusDot) CreateRenderer() fyne.WidgetRenderer {
	colors := app.StatusColors(s.isDark)
	s.dot = canvas.NewCircle(colors.Disconnected)
	s.dot.Resize(fyne.NewSize(14, 14))
	return widget.NewSimpleRenderer(container.NewCenter(s.dot))
}

// ConnectionButton is a large prominent button for connect/disconnect
type ConnectionButton struct {
	widget.Button
	
	isConnected  bool
	onConnect    func()
	onDisconnect func()
}

// NewConnectionButton creates a new connection button
func NewConnectionButton() *ConnectionButton {
	b := &ConnectionButton{
		isConnected: false,
	}
	
	b.Button = *widget.NewButtonWithIcon("Connect", theme.MediaPlayIcon(), b.onClick)
	b.Importance = widget.HighImportance
	
	// Make button larger
	b.Resize(fyne.NewSize(180, app.ButtonHeight()))
	
	b.ExtendBaseWidget(b)
	return b
}

// SetConnected sets the connected state directly
func (b *ConnectionButton) SetConnected(connected bool) {
	if b.isConnected != connected {
		b.isConnected = connected
		b.updateAppearance()
	}
}

// SetOnConnect sets the connect handler
func (b *ConnectionButton) SetOnConnect(fn func()) {
	b.onConnect = fn
}

// SetOnDisconnect sets the disconnect handler
func (b *ConnectionButton) SetOnDisconnect(fn func()) {
	b.onDisconnect = fn
}

func (b *ConnectionButton) onClick() {
	if b.isConnected {
		if b.onDisconnect != nil {
			b.onDisconnect()
		}
	} else {
		if b.onConnect != nil {
			b.onConnect()
		}
	}
}

func (b *ConnectionButton) updateAppearance() {
	if b.isConnected {
		b.Button.SetText("Disconnect")
		b.Button.SetIcon(theme.MediaStopIcon())
		b.Button.Importance = widget.DangerImportance
	} else {
		b.Button.SetText("Connect")
		b.Button.SetIcon(theme.MediaPlayIcon())
		b.Button.Importance = widget.HighImportance
	}
	b.Button.Refresh()
}

// MinSize returns minimum size for the button
func (b *ConnectionButton) MinSize() fyne.Size {
	return fyne.NewSize(180, app.ButtonHeight())
}

// LargeButton creates a prominent button with proper sizing
func LargeButton(label string, icon fyne.Resource, fn func()) *widget.Button {
	btn := widget.NewButtonWithIcon(label, icon, fn)
	btn.Importance = widget.HighImportance
	btn.Resize(fyne.NewSize(160, app.ButtonHeight()))
	return btn
}

// SectionCard creates a card with consistent styling
func SectionCard(title string, content fyne.CanvasObject) fyne.CanvasObject {
	card := widget.NewCard(title, "", container.NewPadded(content))
	return card
}

// PaddedContainer wraps content with standard padding
func PaddedContainer(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewPadded(obj)
}

// VSpacer creates vertical spacing
func VSpacer(height float32) fyne.CanvasObject {
	return canvas.NewRectangle(color.Transparent)
}

// Centered creates a centered container
func Centered(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewHBox(layout.NewSpacer(), obj, layout.NewSpacer())
}
