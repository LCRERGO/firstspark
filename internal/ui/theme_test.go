//go:build gui

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

func TestParseFamilyAndVariant(t *testing.T) {
	if parseFamily("nord") != familyNord || parseFamily("tokyo-night") != familyTokyoNight {
		t.Error("parseFamily did not recognise a known family")
	}
	if parseFamily("") != familyCyberpunk || parseFamily("bogus") != familyCyberpunk {
		t.Error("parseFamily should default to cyberpunk")
	}
	if parseVariant("dark") != variantDark || parseVariant("light") != variantLight {
		t.Error("parseVariant did not recognise a known variant")
	}
	if parseVariant("") != variantSystem || parseVariant("bogus") != variantSystem {
		t.Error("parseVariant should default to system")
	}
	for _, f := range families {
		if parseFamily(string(f)) != f {
			t.Errorf("family round trip failed for %q", f)
		}
	}
	for _, v := range []variant{variantLight, variantDark, variantSystem} {
		if parseVariant(v.String()) != v {
			t.Errorf("variant round trip failed for %v", v)
		}
	}
}

// TestEveryColorNameIsMapped checks that each palette answers every colour name
// the application relies on with an opaque colour.
func TestEveryColorNameIsMapped(t *testing.T) {
	names := []fyne.ThemeColorName{
		theme.ColorNameBackground, theme.ColorNameButton, theme.ColorNameDisabledButton,
		theme.ColorNameDisabled, theme.ColorNameError, theme.ColorNameForeground,
		theme.ColorNameForegroundOnError, theme.ColorNameForegroundOnPrimary,
		theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning,
		theme.ColorNameHeaderBackground, theme.ColorNameHover, theme.ColorNameHyperlink,
		theme.ColorNameInnerWindowBorder, theme.ColorNameInnerWindowBorderInactive,
		theme.ColorNameInputBackground, theme.ColorNameInputBorder, theme.ColorNameMenuBackground,
		theme.ColorNameOverlayBackground, theme.ColorNamePlaceHolder, theme.ColorNamePressed,
		theme.ColorNamePrimary, theme.ColorNameScrollBar, theme.ColorNameScrollBarBackground,
		theme.ColorNameSelection, theme.ColorNameSeparator, theme.ColorNameShadow,
		theme.ColorNameSuccess, theme.ColorNameWarning, theme.ColorNameFocus,
	}
	for _, f := range families {
		for _, vr := range []variant{variantLight, variantDark} {
			th := newTheme(f, vr, 14)
			for _, n := range names {
				c := th.Color(n, theme.VariantLight)
				if c == nil {
					t.Errorf("family %s variant %v: %s has no colour", f, vr, n)
					continue
				}
				if _, _, _, a := c.RGBA(); a == 0 {
					t.Errorf("family %s variant %v: %s is transparent", f, vr, n)
				}
			}
		}
	}
}

func TestHyperlinkUsesSecondaryAccent(t *testing.T) {
	for _, f := range families {
		for _, vr := range []variant{variantLight, variantDark} {
			p := familyPalette(f, resolveVariant(vr))
			if p.color(theme.ColorNameHyperlink, theme.VariantLight) != p.secondary {
				t.Errorf("family %s variant %v: hyperlink is not the secondary accent", f, vr)
			}
		}
	}
}

func TestThemeMenuHasSingleSystemOption(t *testing.T) {
	a := newTestApp(t)
	systemLabel := variantLabel(variantSystem)
	systems, checked := 0, 0
	var walk func(items []*fyne.MenuItem)
	walk = func(items []*fyne.MenuItem) {
		for _, it := range items {
			if it.Label == systemLabel {
				systems++
			}
			if it.Checked {
				checked++
			}
			if it.ChildMenu != nil {
				walk(it.ChildMenu.Items)
			}
		}
	}
	for _, it := range a.viewMenu.Items {
		if it.ChildMenu != nil {
			walk(it.ChildMenu.Items)
		}
	}
	if systems != 1 {
		t.Errorf("found %d System entries, want 1", systems)
	}
	if checked != 1 {
		t.Errorf("found %d checked theme entries, want 1", checked)
	}
}

// resolveVariant mirrors cyberTheme.resolvedVariant for tests.
func resolveVariant(vr variant) fyne.ThemeVariant {
	if vr == variantDark {
		return theme.VariantDark
	}
	return theme.VariantLight
}
