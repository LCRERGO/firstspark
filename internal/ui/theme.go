//go:build gui

package ui

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

//go:embed assets/fonts/Rajdhani-SemiBold.ttf
var rajdhaniTTF []byte

// displayFont is the embedded OFL display face used for headings and titles.
var displayFont = fyne.NewStaticResource("Rajdhani-SemiBold.ttf", rajdhaniTTF)

// scheme selects which colour variant the theme renders.
type scheme int

const (
	schemeSystem scheme = iota
	schemeLight
	schemeDark
)

// parseScheme maps a config value to a scheme, defaulting to system.
func parseScheme(s string) scheme {
	switch s {
	case "light":
		return schemeLight
	case "dark":
		return schemeDark
	default:
		return schemeSystem
	}
}

func (s scheme) String() string {
	switch s {
	case schemeLight:
		return "light"
	case schemeDark:
		return "dark"
	default:
		return "system"
	}
}

// palette is a flat cyberpunk colour set. Body colours keep a high contrast
// ratio; neon is reserved for accents, borders, separators and headings.
type palette struct {
	background, surface, dialogSurface, input, inputBorder color.Color
	text, subtle, innerBorder, separator                   color.Color
	primary, onPrimary, secondary                          color.Color
	selection, focus, header, hover, scrim                 color.Color
	warning, errColor, success                             color.Color
}

func darkPalette() palette {
	return palette{
		background:    color.NRGBA{R: 0x0A, G: 0x0B, B: 0x12, A: 0xFF},
		surface:       color.NRGBA{R: 0x14, G: 0x16, B: 0x1F, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0x17, G: 0x1A, B: 0x26, A: 0xFF},
		input:         color.NRGBA{R: 0x10, G: 0x13, B: 0x20, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x45, G: 0x4E, B: 0x6B, A: 0xFF},
		text:          color.NRGBA{R: 0xE6, G: 0xF1, B: 0xFF, A: 0xFF},
		subtle:        color.NRGBA{R: 0x8A, G: 0x93, B: 0xA6, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0x2E, G: 0x36, B: 0x50, A: 0xFF},
		separator:     color.NRGBA{R: 0x2A, G: 0x31, B: 0x45, A: 0xFF},
		primary:       color.NRGBA{R: 0x00, G: 0xE5, B: 0xFF, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0x00, G: 0x10, B: 0x18, A: 0xFF},
		secondary:     color.NRGBA{R: 0xFF, G: 0x2D, B: 0x95, A: 0xFF},
		selection:     color.NRGBA{R: 0x12, G: 0x3A, B: 0x44, A: 0xFF},
		focus:         color.NRGBA{R: 0x00, G: 0xE5, B: 0xFF, A: 0xFF},
		header:        color.NRGBA{R: 0x14, G: 0x16, B: 0x1F, A: 0xFF},
		hover:         color.NRGBA{R: 0x1B, G: 0x21, B: 0x30, A: 0xFF},
		scrim:         color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x99},
		warning:       color.NRGBA{R: 0xFF, G: 0xE6, B: 0x00, A: 0xFF},
		errColor:      color.NRGBA{R: 0xFF, G: 0x4D, B: 0x6D, A: 0xFF},
		success:       color.NRGBA{R: 0x39, G: 0xD9, B: 0x8A, A: 0xFF},
	}
}

func lightPalette() palette {
	return palette{
		background:    color.NRGBA{R: 0xF4, G: 0xF6, B: 0xFB, A: 0xFF},
		surface:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		input:         color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x8A, G: 0x93, B: 0xA6, A: 0xFF},
		text:          color.NRGBA{R: 0x0B, G: 0x0F, B: 0x1A, A: 0xFF},
		subtle:        color.NRGBA{R: 0x5A, G: 0x63, B: 0x76, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0xC7, G: 0xCE, B: 0xDD, A: 0xFF},
		separator:     color.NRGBA{R: 0xCB, G: 0xD2, B: 0xE0, A: 0xFF},
		primary:       color.NRGBA{R: 0x00, G: 0x89, B: 0xA8, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		secondary:     color.NRGBA{R: 0xD8, G: 0x1B, B: 0x8C, A: 0xFF},
		selection:     color.NRGBA{R: 0xB3, G: 0xE9, B: 0xF2, A: 0xFF},
		focus:         color.NRGBA{R: 0x00, G: 0x89, B: 0xA8, A: 0xFF},
		header:        color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		hover:         color.NRGBA{R: 0xE8, G: 0xED, B: 0xF7, A: 0xFF},
		scrim:         color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x55},
		warning:       color.NRGBA{R: 0xB5, G: 0x89, B: 0x00, A: 0xFF},
		errColor:      color.NRGBA{R: 0xD6, G: 0x33, B: 0x6C, A: 0xFF},
		success:       color.NRGBA{R: 0x12, G: 0xA1, B: 0x50, A: 0xFF},
	}
}

func (p palette) color(n fyne.ThemeColorName) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return p.background
	case theme.ColorNameButton:
		return p.surface
	case theme.ColorNameDisabledButton:
		return p.separator
	case theme.ColorNameDisabled:
		return p.subtle
	case theme.ColorNameError:
		return p.errColor
	case theme.ColorNameForeground:
		return p.text
	case theme.ColorNameForegroundOnError:
		return color.White
	case theme.ColorNameForegroundOnPrimary:
		return p.onPrimary
	case theme.ColorNameForegroundOnSuccess:
		return color.White
	case theme.ColorNameForegroundOnWarning:
		return color.Black
	case theme.ColorNameHeaderBackground:
		return p.header
	case theme.ColorNameHover:
		return p.hover
	case theme.ColorNameHyperlink:
		return p.primary
	case theme.ColorNameInnerWindowBorder, theme.ColorNameInnerWindowBorderInactive:
		return p.innerBorder
	case theme.ColorNameInputBackground:
		return p.input
	case theme.ColorNameInputBorder:
		return p.inputBorder
	case theme.ColorNameMenuBackground:
		return p.surface
	case theme.ColorNameOverlayBackground:
		return p.dialogSurface
	case theme.ColorNamePlaceHolder:
		return p.subtle
	case theme.ColorNamePressed:
		return p.primary
	case theme.ColorNamePrimary:
		return p.primary
	case theme.ColorNameScrollBar:
		return p.subtle
	case theme.ColorNameScrollBarBackground:
		return p.surface
	case theme.ColorNameSelection:
		return p.selection
	case theme.ColorNameSeparator:
		return p.separator
	case theme.ColorNameShadow:
		return p.scrim
	case theme.ColorNameSuccess:
		return p.success
	case theme.ColorNameWarning:
		return p.warning
	case theme.ColorNameFocus:
		return p.focus
	default:
		return theme.DefaultTheme().Color(n, theme.VariantDark)
	}
}

// cyberTheme is a flat theme with a cyan/magenta palette.
type cyberTheme struct {
	mode scheme
	size float32
	mono float32
}

func newTheme(mode scheme, fontSize float64) *cyberTheme {
	if fontSize < 8 || fontSize > 40 {
		fontSize = 14
	}
	return &cyberTheme{mode: mode, size: float32(fontSize), mono: float32(fontSize) - 2}
}

func (t *cyberTheme) setMode(m scheme) { t.mode = m }

func (t *cyberTheme) pal(v fyne.ThemeVariant) palette {
	switch t.mode {
	case schemeLight:
		v = theme.VariantLight
	case schemeDark:
		v = theme.VariantDark
	}
	if v == theme.VariantDark {
		return darkPalette()
	}
	return lightPalette()
}

func (t *cyberTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return t.pal(v).color(n)
}

func (t *cyberTheme) Font(s fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(s)
}

func (t *cyberTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (t *cyberTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return t.size
	case theme.SizeNameHeadingText:
		return t.size + 8
	case theme.SizeNameSubHeadingText:
		return t.size + 3
	case theme.SizeNameCaptionText:
		if t.size > 6 {
			return t.size - 2
		}
		return t.size
	}
	return theme.DefaultTheme().Size(n)
}

// heading renders text in the embedded display face.
func (t *cyberTheme) heading(text string, size float32, col color.Color) *canvas.Text {
	obj := canvas.NewText(text, col)
	obj.FontSource = displayFont
	obj.TextStyle = fyne.TextStyle{Bold: true}
	obj.TextSize = size
	return obj
}

// monoText renders monospaced data text.
func (t *cyberTheme) monoText(text string, col color.Color) *canvas.Text {
	obj := canvas.NewText(text, col)
	obj.TextStyle = fyne.TextStyle{Monospace: true}
	obj.TextSize = t.mono
	return obj
}
