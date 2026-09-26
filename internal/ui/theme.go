//go:build gui

package ui

import (
	_ "embed"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"

	"github.com/LCRERGO/firstspark/internal/i18n"
)

//go:embed assets/fonts/Rajdhani-SemiBold.ttf
var rajdhaniTTF []byte

// displayFont is the embedded OFL display face used for headings and titles.
var displayFont = fyne.NewStaticResource("Rajdhani-SemiBold.ttf", rajdhaniTTF)

// family selects the palette pair a theme renders.
type family string

const (
	familyCyberpunk  family = "cyberpunk"
	familyNord       family = "nord"
	familyDracula    family = "dracula"
	familyTokyoNight family = "tokyo-night"
)

// families is the display order of the palette families.
var families = []family{familyCyberpunk, familyNord, familyDracula, familyTokyoNight}

// parseFamily maps a config value to a family, defaulting to cyberpunk.
func parseFamily(s string) family {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "nord":
		return familyNord
	case "dracula":
		return familyDracula
	case "tokyo-night", "tokyonight", "tokyo_night":
		return familyTokyoNight
	default:
		return familyCyberpunk
	}
}

// familyLabel returns the translated family name.
func familyLabel(f family) string {
	switch f {
	case familyNord:
		return i18n.T("theme.family.nord")
	case familyDracula:
		return i18n.T("theme.family.dracula")
	case familyTokyoNight:
		return i18n.T("theme.family.tokyo_night")
	default:
		return i18n.T("theme.family.cyberpunk")
	}
}

// parseFamilyLabel maps a translated family name back to a family.
func parseFamilyLabel(s string) family {
	switch s {
	case i18n.T("theme.family.nord"):
		return familyNord
	case i18n.T("theme.family.dracula"):
		return familyDracula
	case i18n.T("theme.family.tokyo_night"):
		return familyTokyoNight
	default:
		return familyCyberpunk
	}
}

// variant selects light, dark or the operating system preference.
type variant int

const (
	variantSystem variant = iota
	variantLight
	variantDark
)

// parseVariant maps a config value to a variant, defaulting to system.
func parseVariant(s string) variant {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "light":
		return variantLight
	case "dark":
		return variantDark
	default:
		return variantSystem
	}
}

func (v variant) String() string {
	switch v {
	case variantLight:
		return "light"
	case variantDark:
		return "dark"
	default:
		return "system"
	}
}

// variantLabel returns the translated variant name.
func variantLabel(v variant) string {
	switch v {
	case variantLight:
		return i18n.T("menu.view.light")
	case variantDark:
		return i18n.T("menu.view.dark")
	default:
		return i18n.T("menu.view.system")
	}
}

// parseVariantLabel maps a translated variant name back to a variant.
func parseVariantLabel(s string) variant {
	switch s {
	case i18n.T("menu.view.light"):
		return variantLight
	case i18n.T("menu.view.dark"):
		return variantDark
	default:
		return variantSystem
	}
}

// palette is a flat colour set. Body colours keep a high contrast ratio;
// accents are reserved for primary controls, borders, separators and headings.
type palette struct {
	background, surface, dialogSurface, input, inputBorder color.Color
	text, subtle, innerBorder, separator                   color.Color
	primary, onPrimary, secondary                          color.Color
	selection, focus, header, hover, scrim                 color.Color
	warning, errColor, success                             color.Color
	onWarning, onError, onSuccess                          color.Color
}

func cyberpunkDark() palette {
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
		scrim:         color.NRGBA{A: 0x99},
		warning:       color.NRGBA{R: 0xFF, G: 0xE6, B: 0x00, A: 0xFF},
		errColor:      color.NRGBA{R: 0xFF, G: 0x4D, B: 0x6D, A: 0xFF},
		success:       color.NRGBA{R: 0x39, G: 0xD9, B: 0x8A, A: 0xFF},
		onWarning:     color.NRGBA{A: 0xFF},
		onError:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
	}
}

func cyberpunkLight() palette {
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
		scrim:         color.NRGBA{A: 0x55},
		warning:       color.NRGBA{R: 0xB5, G: 0x89, B: 0x00, A: 0xFF},
		errColor:      color.NRGBA{R: 0xD6, G: 0x33, B: 0x6C, A: 0xFF},
		success:       color.NRGBA{R: 0x12, G: 0xA1, B: 0x50, A: 0xFF},
		onWarning:     color.NRGBA{A: 0xFF},
		onError:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
	}
}

// nordDark uses the Nord Polar Night and Frost colours.
func nordDark() palette {
	return palette{
		background:    color.NRGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xFF},
		surface:       color.NRGBA{R: 0x3B, G: 0x42, B: 0x52, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0x3B, G: 0x42, B: 0x52, A: 0xFF},
		input:         color.NRGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x4C, G: 0x56, B: 0x6A, A: 0xFF},
		text:          color.NRGBA{R: 0xEC, G: 0xEF, B: 0xF4, A: 0xFF},
		subtle:        color.NRGBA{R: 0xD8, G: 0xDE, B: 0xE9, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0x4C, G: 0x56, B: 0x6A, A: 0xFF},
		separator:     color.NRGBA{R: 0x4C, G: 0x56, B: 0x6A, A: 0xFF},
		primary:       color.NRGBA{R: 0x88, G: 0xC0, B: 0xD0, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xFF},
		secondary:     color.NRGBA{R: 0xB4, G: 0x8E, B: 0xAD, A: 0xFF},
		selection:     color.NRGBA{R: 0x3B, G: 0x51, B: 0x62, A: 0xFF},
		focus:         color.NRGBA{R: 0x88, G: 0xC0, B: 0xD0, A: 0xFF},
		header:        color.NRGBA{R: 0x3B, G: 0x42, B: 0x52, A: 0xFF},
		hover:         color.NRGBA{R: 0x43, G: 0x4C, B: 0x5E, A: 0xFF},
		scrim:         color.NRGBA{A: 0x99},
		warning:       color.NRGBA{R: 0xEB, G: 0xCB, B: 0x8B, A: 0xFF},
		errColor:      color.NRGBA{R: 0xBF, G: 0x61, B: 0x6A, A: 0xFF},
		success:       color.NRGBA{R: 0xA3, G: 0xBE, B: 0x8C, A: 0xFF},
		onWarning:     color.NRGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xFF},
		onError:       color.NRGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xFF},
	}
}

// nordLight uses the Nord Snow Storm colours.
func nordLight() palette {
	return palette{
		background:    color.NRGBA{R: 0xEC, G: 0xEF, B: 0xF4, A: 0xFF},
		surface:       color.NRGBA{R: 0xE5, G: 0xE9, B: 0xF0, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0xEC, G: 0xEF, B: 0xF4, A: 0xFF},
		input:         color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x81, G: 0xA1, B: 0xC1, A: 0xFF},
		text:          color.NRGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xFF},
		subtle:        color.NRGBA{R: 0x4C, G: 0x56, B: 0x6A, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0xD8, G: 0xDE, B: 0xE9, A: 0xFF},
		separator:     color.NRGBA{R: 0xD8, G: 0xDE, B: 0xE9, A: 0xFF},
		primary:       color.NRGBA{R: 0x5E, G: 0x81, B: 0xAC, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0xEC, G: 0xEF, B: 0xF4, A: 0xFF},
		secondary:     color.NRGBA{R: 0xB4, G: 0x8E, B: 0xAD, A: 0xFF},
		selection:     color.NRGBA{R: 0xD8, G: 0xDE, B: 0xE9, A: 0xFF},
		focus:         color.NRGBA{R: 0x5E, G: 0x81, B: 0xAC, A: 0xFF},
		header:        color.NRGBA{R: 0xE5, G: 0xE9, B: 0xF0, A: 0xFF},
		hover:         color.NRGBA{R: 0xD8, G: 0xDE, B: 0xE9, A: 0xFF},
		scrim:         color.NRGBA{A: 0x55},
		warning:       color.NRGBA{R: 0xA5, G: 0x6A, B: 0x00, A: 0xFF},
		errColor:      color.NRGBA{R: 0xBF, G: 0x61, B: 0x6A, A: 0xFF},
		success:       color.NRGBA{R: 0x4F, G: 0x8A, B: 0x4C, A: 0xFF},
		onWarning:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		onError:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
	}
}

// draculaDark uses the Dracula colours.
func draculaDark() palette {
	return palette{
		background:    color.NRGBA{R: 0x28, G: 0x2A, B: 0x36, A: 0xFF},
		surface:       color.NRGBA{R: 0x44, G: 0x47, B: 0x5A, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0x34, G: 0x37, B: 0x46, A: 0xFF},
		input:         color.NRGBA{R: 0x21, G: 0x22, B: 0x2C, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x62, G: 0x72, B: 0xA4, A: 0xFF},
		text:          color.NRGBA{R: 0xF8, G: 0xF8, B: 0xF2, A: 0xFF},
		subtle:        color.NRGBA{R: 0x62, G: 0x72, B: 0xA4, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0x44, G: 0x47, B: 0x5A, A: 0xFF},
		separator:     color.NRGBA{R: 0x44, G: 0x47, B: 0x5A, A: 0xFF},
		primary:       color.NRGBA{R: 0xBD, G: 0x93, B: 0xF9, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0x28, G: 0x2A, B: 0x36, A: 0xFF},
		secondary:     color.NRGBA{R: 0xFF, G: 0x79, B: 0xC6, A: 0xFF},
		selection:     color.NRGBA{R: 0x44, G: 0x47, B: 0x5A, A: 0xFF},
		focus:         color.NRGBA{R: 0xBD, G: 0x93, B: 0xF9, A: 0xFF},
		header:        color.NRGBA{R: 0x34, G: 0x37, B: 0x46, A: 0xFF},
		hover:         color.NRGBA{R: 0x44, G: 0x47, B: 0x5A, A: 0xFF},
		scrim:         color.NRGBA{A: 0x99},
		warning:       color.NRGBA{R: 0xF1, G: 0xFA, B: 0x8C, A: 0xFF},
		errColor:      color.NRGBA{R: 0xFF, G: 0x55, B: 0x55, A: 0xFF},
		success:       color.NRGBA{R: 0x50, G: 0xFA, B: 0x7B, A: 0xFF},
		onWarning:     color.NRGBA{R: 0x28, G: 0x2A, B: 0x36, A: 0xFF},
		onError:       color.NRGBA{R: 0x28, G: 0x2A, B: 0x36, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0x28, G: 0x2A, B: 0x36, A: 0xFF},
	}
}

// draculaLight uses Dracula's Alucard light colours.
func draculaLight() palette {
	return palette{
		background:    color.NRGBA{R: 0xFF, G: 0xFB, B: 0xEB, A: 0xFF},
		surface:       color.NRGBA{R: 0xF0, G: 0xEA, B: 0xD2, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0xFF, G: 0xFB, B: 0xEB, A: 0xFF},
		input:         color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x6C, G: 0x66, B: 0x4B, A: 0xFF},
		text:          color.NRGBA{R: 0x1F, G: 0x1F, B: 0x1F, A: 0xFF},
		subtle:        color.NRGBA{R: 0x6C, G: 0x66, B: 0x4B, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0xDD, G: 0xD6, B: 0xB8, A: 0xFF},
		separator:     color.NRGBA{R: 0xDD, G: 0xD6, B: 0xB8, A: 0xFF},
		primary:       color.NRGBA{R: 0x64, G: 0x4A, B: 0xC9, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0xFF, G: 0xFB, B: 0xEB, A: 0xFF},
		secondary:     color.NRGBA{R: 0xA3, G: 0x14, B: 0x4D, A: 0xFF},
		selection:     color.NRGBA{R: 0xE8, G: 0xE0, B: 0xC4, A: 0xFF},
		focus:         color.NRGBA{R: 0x64, G: 0x4A, B: 0xC9, A: 0xFF},
		header:        color.NRGBA{R: 0xF0, G: 0xEA, B: 0xD2, A: 0xFF},
		hover:         color.NRGBA{R: 0xF0, G: 0xEA, B: 0xD2, A: 0xFF},
		scrim:         color.NRGBA{A: 0x55},
		warning:       color.NRGBA{R: 0x84, G: 0x6E, B: 0x15, A: 0xFF},
		errColor:      color.NRGBA{R: 0xCB, G: 0x3A, B: 0x2A, A: 0xFF},
		success:       color.NRGBA{R: 0x14, G: 0x71, B: 0x0A, A: 0xFF},
		onWarning:     color.NRGBA{R: 0xFF, G: 0xFB, B: 0xEB, A: 0xFF},
		onError:       color.NRGBA{R: 0xFF, G: 0xFB, B: 0xEB, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0xFF, G: 0xFB, B: 0xEB, A: 0xFF},
	}
}

// tokyoNightDark uses Tokyo Night's night colours.
func tokyoNightDark() palette {
	return palette{
		background:    color.NRGBA{R: 0x1A, G: 0x1B, B: 0x26, A: 0xFF},
		surface:       color.NRGBA{R: 0x29, G: 0x2E, B: 0x42, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0x1F, G: 0x23, B: 0x35, A: 0xFF},
		input:         color.NRGBA{R: 0x16, G: 0x16, B: 0x1E, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x41, G: 0x48, B: 0x68, A: 0xFF},
		text:          color.NRGBA{R: 0xC0, G: 0xCA, B: 0xF5, A: 0xFF},
		subtle:        color.NRGBA{R: 0x56, G: 0x5F, B: 0x89, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0x29, G: 0x2E, B: 0x42, A: 0xFF},
		separator:     color.NRGBA{R: 0x29, G: 0x2E, B: 0x42, A: 0xFF},
		primary:       color.NRGBA{R: 0x7A, G: 0xA2, B: 0xF7, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0x1A, G: 0x1B, B: 0x26, A: 0xFF},
		secondary:     color.NRGBA{R: 0xBB, G: 0x9A, B: 0xF7, A: 0xFF},
		selection:     color.NRGBA{R: 0x24, G: 0x30, B: 0x4D, A: 0xFF},
		focus:         color.NRGBA{R: 0x7A, G: 0xA2, B: 0xF7, A: 0xFF},
		header:        color.NRGBA{R: 0x1F, G: 0x23, B: 0x35, A: 0xFF},
		hover:         color.NRGBA{R: 0x29, G: 0x2E, B: 0x42, A: 0xFF},
		scrim:         color.NRGBA{A: 0x99},
		warning:       color.NRGBA{R: 0xE0, G: 0xAF, B: 0x68, A: 0xFF},
		errColor:      color.NRGBA{R: 0xF7, G: 0x76, B: 0x8E, A: 0xFF},
		success:       color.NRGBA{R: 0x9E, G: 0xCE, B: 0x6A, A: 0xFF},
		onWarning:     color.NRGBA{R: 0x1A, G: 0x1B, B: 0x26, A: 0xFF},
		onError:       color.NRGBA{R: 0x1A, G: 0x1B, B: 0x26, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0x1A, G: 0x1B, B: 0x26, A: 0xFF},
	}
}

// tokyoNightLight uses Tokyo Night's day colours.
func tokyoNightLight() palette {
	return palette{
		background:    color.NRGBA{R: 0xE1, G: 0xE2, B: 0xE7, A: 0xFF},
		surface:       color.NRGBA{R: 0xD0, G: 0xD5, B: 0xE3, A: 0xFF},
		dialogSurface: color.NRGBA{R: 0xE1, G: 0xE2, B: 0xE7, A: 0xFF},
		input:         color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		inputBorder:   color.NRGBA{R: 0x84, G: 0x8C, B: 0xB5, A: 0xFF},
		text:          color.NRGBA{R: 0x34, G: 0x3B, B: 0x58, A: 0xFF},
		subtle:        color.NRGBA{R: 0x84, G: 0x8C, B: 0xB5, A: 0xFF},
		innerBorder:   color.NRGBA{R: 0xC4, G: 0xC8, B: 0xDA, A: 0xFF},
		separator:     color.NRGBA{R: 0xC4, G: 0xC8, B: 0xDA, A: 0xFF},
		primary:       color.NRGBA{R: 0x2E, G: 0x7D, B: 0xE9, A: 0xFF},
		onPrimary:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		secondary:     color.NRGBA{R: 0x98, G: 0x54, B: 0xF1, A: 0xFF},
		selection:     color.NRGBA{R: 0xC4, G: 0xC8, B: 0xDA, A: 0xFF},
		focus:         color.NRGBA{R: 0x2E, G: 0x7D, B: 0xE9, A: 0xFF},
		header:        color.NRGBA{R: 0xD0, G: 0xD5, B: 0xE3, A: 0xFF},
		hover:         color.NRGBA{R: 0xC4, G: 0xC8, B: 0xDA, A: 0xFF},
		scrim:         color.NRGBA{A: 0x55},
		warning:       color.NRGBA{R: 0x8C, G: 0x6C, B: 0x3E, A: 0xFF},
		errColor:      color.NRGBA{R: 0xF5, G: 0x2A, B: 0x65, A: 0xFF},
		success:       color.NRGBA{R: 0x58, G: 0x75, B: 0x39, A: 0xFF},
		onWarning:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		onError:       color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		onSuccess:     color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
	}
}

// familyPalette returns the palette of a family for a resolved variant.
func familyPalette(f family, v fyne.ThemeVariant) palette {
	dark := v == theme.VariantDark
	switch f {
	case familyNord:
		if dark {
			return nordDark()
		}
		return nordLight()
	case familyDracula:
		if dark {
			return draculaDark()
		}
		return draculaLight()
	case familyTokyoNight:
		if dark {
			return tokyoNightDark()
		}
		return tokyoNightLight()
	default:
		if dark {
			return cyberpunkDark()
		}
		return cyberpunkLight()
	}
}

// colorFields maps each Fyne colour name to the palette field that supplies it.
var colorFields = map[fyne.ThemeColorName]func(palette) color.Color{
	theme.ColorNameBackground:                func(p palette) color.Color { return p.background },
	theme.ColorNameButton:                    func(p palette) color.Color { return p.surface },
	theme.ColorNameDisabledButton:            func(p palette) color.Color { return p.separator },
	theme.ColorNameDisabled:                  func(p palette) color.Color { return p.subtle },
	theme.ColorNameError:                     func(p palette) color.Color { return p.errColor },
	theme.ColorNameForeground:                func(p palette) color.Color { return p.text },
	theme.ColorNameForegroundOnError:         func(p palette) color.Color { return p.onError },
	theme.ColorNameForegroundOnPrimary:       func(p palette) color.Color { return p.onPrimary },
	theme.ColorNameForegroundOnSuccess:       func(p palette) color.Color { return p.onSuccess },
	theme.ColorNameForegroundOnWarning:       func(p palette) color.Color { return p.onWarning },
	theme.ColorNameHeaderBackground:          func(p palette) color.Color { return p.header },
	theme.ColorNameHover:                     func(p palette) color.Color { return p.hover },
	theme.ColorNameHyperlink:                 func(p palette) color.Color { return p.secondary },
	theme.ColorNameInnerWindowBorder:         func(p palette) color.Color { return p.innerBorder },
	theme.ColorNameInnerWindowBorderInactive: func(p palette) color.Color { return p.innerBorder },
	theme.ColorNameInputBackground:           func(p palette) color.Color { return p.input },
	theme.ColorNameInputBorder:               func(p palette) color.Color { return p.inputBorder },
	theme.ColorNameMenuBackground:            func(p palette) color.Color { return p.surface },
	theme.ColorNameOverlayBackground:         func(p palette) color.Color { return p.dialogSurface },
	theme.ColorNamePlaceHolder:               func(p palette) color.Color { return p.subtle },
	theme.ColorNamePressed:                   func(p palette) color.Color { return p.primary },
	theme.ColorNamePrimary:                   func(p palette) color.Color { return p.primary },
	theme.ColorNameScrollBar:                 func(p palette) color.Color { return p.subtle },
	theme.ColorNameScrollBarBackground:       func(p palette) color.Color { return p.surface },
	theme.ColorNameSelection:                 func(p palette) color.Color { return p.selection },
	theme.ColorNameSeparator:                 func(p palette) color.Color { return p.separator },
	theme.ColorNameShadow:                    func(p palette) color.Color { return p.scrim },
	theme.ColorNameSuccess:                   func(p palette) color.Color { return p.success },
	theme.ColorNameWarning:                   func(p palette) color.Color { return p.warning },
	theme.ColorNameFocus:                     func(p palette) color.Color { return p.focus },
}

func (p palette) color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if f, ok := colorFields[n]; ok {
		return f(p)
	}
	return theme.DefaultTheme().Color(n, v)
}

// cyberTheme is a flat theme with a selectable palette family and variant.
type cyberTheme struct {
	fam     family
	variant variant
	size    float32
	mono    float32
}

var _ fyne.Theme = (*cyberTheme)(nil)

func newTheme(fam family, vr variant, fontSize float64) *cyberTheme {
	if fontSize < 8 || fontSize > 40 {
		fontSize = 14
	}
	return &cyberTheme{fam: fam, variant: vr, size: float32(fontSize), mono: float32(fontSize) - 2}
}

// resolvedVariant applies the configured variant to the one Fyne supplies.
func (t *cyberTheme) resolvedVariant(v fyne.ThemeVariant) fyne.ThemeVariant {
	switch t.variant {
	case variantLight:
		return theme.VariantLight
	case variantDark:
		return theme.VariantDark
	default:
		return v
	}
}

func (t *cyberTheme) pal(v fyne.ThemeVariant) palette {
	return familyPalette(t.fam, t.resolvedVariant(v))
}

func (t *cyberTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	rv := t.resolvedVariant(v)
	return familyPalette(t.fam, rv).color(n, rv)
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
