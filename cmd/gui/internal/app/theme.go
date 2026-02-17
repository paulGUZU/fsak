package app

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// vibrantTheme is a custom colorful theme that properly supports dark mode
type vibrantTheme struct {
	base fyne.Theme
}

// NewVibrantTheme creates a new vibrant theme
func NewVibrantTheme() fyne.Theme {
	return &vibrantTheme{base: theme.DefaultTheme()}
}

func (t *vibrantTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	isDark := variant == theme.VariantDark

	switch name {
	// Background colors
	case theme.ColorNameBackground:
		if isDark {
			return color.NRGBA{R: 0x1E, G: 0x1E, B: 0x2E, A: 0xFF} // Deep dark blue-gray
		}
		return color.NRGBA{R: 0xF2, G: 0xF7, B: 0xFF, A: 0xFF}

	case theme.ColorNameInputBackground:
		if isDark {
			return color.NRGBA{R: 0x2A, G: 0x2A, B: 0x3E, A: 0xFF} // Slightly lighter for inputs
		}
		return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	case theme.ColorNameOverlayBackground:
		if isDark {
			return color.NRGBA{R: 0x25, G: 0x25, B: 0x38, A: 0xFF}
		}
		return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	// Primary brand color - Teal
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x00, G: 0xD4, B: 0xC4, A: 0xFF}

	case theme.ColorNameButton:
		if isDark {
			return color.NRGBA{R: 0x00, G: 0xB8, B: 0xA9, A: 0xFF}
		}
		return color.NRGBA{R: 0x00, G: 0xD4, B: 0xC4, A: 0xFF}

	case theme.ColorNameHover:
		if isDark {
			return color.NRGBA{R: 0x00, G: 0xC8, B: 0xB8, A: 0xFF}
		}
		return color.NRGBA{R: 0x00, G: 0xE0, B: 0xD0, A: 0xFF}

	case theme.ColorNamePressed:
		return color.NRGBA{R: 0x00, G: 0xA8, B: 0x99, A: 0xFF}

	// Text colors
	case theme.ColorNameForeground:
		if isDark {
			return color.NRGBA{R: 0xF0, G: 0xF0, B: 0xF5, A: 0xFF} // Almost white
		}
		return color.NRGBA{R: 0x1A, G: 0x1A, B: 0x2E, A: 0xFF}

	case theme.ColorNameDisabled:
		if isDark {
			return color.NRGBA{R: 0x66, G: 0x66, B: 0x80, A: 0xFF}
		}
		return color.NRGBA{R: 0x99, G: 0x99, B: 0xAA, A: 0xFF}

	case theme.ColorNamePlaceHolder:
		if isDark {
			return color.NRGBA{R: 0x88, G: 0x88, B: 0x99, A: 0xFF}
		}
		return color.NRGBA{R: 0x88, G: 0x88, B: 0x99, A: 0xFF}

	// Accent colors
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0xFF, G: 0x7A, B: 0x59, A: 0xFF} // Orange accent

	case theme.ColorNameSelection:
		if isDark {
			return color.NRGBA{R: 0x00, G: 0xD4, B: 0xC4, A: 0x40}
		}
		return color.NRGBA{R: 0x00, G: 0xD4, B: 0xC4, A: 0x30}

	// Error color
	case theme.ColorNameError:
		if isDark {
			return color.NRGBA{R: 0xFF, G: 0x5A, B: 0x52, A: 0xFF}
		}
		return color.NRGBA{R: 0xD9, G: 0x2D, B: 0x20, A: 0xFF}

	// Shadow
	case theme.ColorNameShadow:
		if isDark {
			return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x60}
		}
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x20}

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
	switch name {
	case theme.SizeNamePadding:
		return 12
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameScrollBar:
		return 8
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInputBorder:
		return 2
	default:
		return t.base.Size(name)
	}
}

// Status colors that work in both light and dark modes
func StatusColors(isDark bool) struct {
	Connected    color.Color
	Disconnected color.Color
	Connecting   color.Color
	Error        color.Color
	Warning      color.Color
} {
	if isDark {
		return struct {
			Connected    color.Color
			Disconnected color.Color
			Connecting   color.Color
			Error        color.Color
			Warning      color.Color
		}{
			Connected:    color.NRGBA{R: 0x4C, G: 0xD9, B: 0x96, A: 0xFF}, // Green
			Disconnected: color.NRGBA{R: 0xFF, G: 0x5A, B: 0x52, A: 0xFF}, // Red
			Connecting:   color.NRGBA{R: 0xFF, G: 0xB8, B: 0x4D, A: 0xFF}, // Orange
			Error:        color.NRGBA{R: 0xFF, G: 0x5A, B: 0x52, A: 0xFF},
			Warning:      color.NRGBA{R: 0xFF, G: 0xB8, B: 0x4D, A: 0xFF},
		}
	}
	return struct {
		Connected    color.Color
		Disconnected color.Color
		Connecting   color.Color
		Error        color.Color
		Warning      color.Color
	}{
		Connected:    color.NRGBA{R: 0x12, G: 0xB7, B: 0x6A, A: 0xFF},
		Disconnected: color.NRGBA{R: 0xD9, G: 0x2D, B: 0x20, A: 0xFF},
		Connecting:   color.NRGBA{R: 0xFF, G: 0xA5, B: 0x00, A: 0xFF},
		Error:        color.NRGBA{R: 0xD9, G: 0x2D, B: 0x20, A: 0xFF},
		Warning:      color.NRGBA{R: 0xFF, G: 0xA5, B: 0x00, A: 0xFF},
	}
}

// CardBackground returns appropriate card background for current theme
func CardBackground(isDark bool) color.Color {
	if isDark {
		return color.NRGBA{R: 0x25, G: 0x25, B: 0x38, A: 0xFF}
	}
	return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
}

// PanelBackground returns status panel background
func PanelBackground(isDark bool, connected bool) color.Color {
	if connected {
		if isDark {
			return color.NRGBA{R: 0x1A, G: 0x3D, B: 0x2E, A: 0xFF} // Dark green tint
		}
		return color.NRGBA{R: 0xE7, G: 0xF9, B: 0xED, A: 0xFF}
	}
	if isDark {
		return color.NRGBA{R: 0x3D, G: 0x1A, B: 0x1A, A: 0xFF} // Dark red tint
	}
	return color.NRGBA{R: 0xFF, G: 0xEE, B: 0xEE, A: 0xFF}
}

// TileColors returns colors for stat tiles that work in both themes
func TileColors(isDark bool) struct {
	Profile color.Color
	Proxy   color.Color
	Server  color.Color
	Address color.Color
} {
	if isDark {
		return struct {
			Profile color.Color
			Proxy   color.Color
			Server  color.Color
			Address color.Color
		}{
			Profile: color.NRGBA{R: 0x2D, G: 0x3A, B: 0x4A, A: 0xFF}, // Blue-gray
			Proxy:   color.NRGBA{R: 0x2D, G: 0x4A, B: 0x3A, A: 0xFF}, // Green-gray
			Server:  color.NRGBA{R: 0x4A, G: 0x3D, B: 0x2D, A: 0xFF}, // Orange-gray
			Address: color.NRGBA{R: 0x3D, G: 0x2D, B: 0x4A, A: 0xFF}, // Purple-gray
		}
	}
	return struct {
		Profile color.Color
		Proxy   color.Color
		Server  color.Color
		Address color.Color
	}{
		Profile: color.NRGBA{R: 0xE8, G: 0xF4, B: 0xFF, A: 0xFF},
		Proxy:   color.NRGBA{R: 0xE9, G: 0xFB, B: 0xEF, A: 0xFF},
		Server:  color.NRGBA{R: 0xFF, G: 0xF2, B: 0xE3, A: 0xFF},
		Address: color.NRGBA{R: 0xF5, G: 0xEE, B: 0xFF, A: 0xFF},
	}
}

// ButtonHeight returns standard button height
func ButtonHeight() float32 {
	return 44
}

// ButtonPadding returns standard button padding
func ButtonPadding() fyne.Size {
	return fyne.NewSize(32, 12)
}
