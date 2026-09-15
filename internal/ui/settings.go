//go:build gui

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/config"
)

func (a *App) showSettings() {
	themeSel := widget.NewSelect([]string{"Light", "Dark", "System"}, nil)
	themeSel.SetSelected(titleCase(a.cfg.UI.Theme))
	scale := widget.NewEntry()
	scale.SetText(strconv.FormatFloat(a.cfg.UI.Scale, 'g', -1, 64))
	font := widget.NewEntry()
	font.SetText(strconv.FormatFloat(a.cfg.UI.FontSize, 'g', -1, 64))
	vt := widget.NewSelect(valueTypeOptions(), nil)
	vt.SetSelected(ceValueTypeLabel(a.defaultValueType()))
	writable := widget.NewCheck("Writable only", nil)
	writable.SetChecked(a.cfg.Scan.WritableOnly)
	align := widget.NewEntry()
	align.SetText(strconv.Itoa(a.cfg.Scan.Alignment))
	limit := widget.NewEntry()
	limit.SetText(strconv.Itoa(a.cfg.UI.ResultLimit))
	icons := widget.NewCheck("Show process icons", nil)
	icons.SetChecked(a.cfg.UI.ProcessIcons)

	form := widget.NewForm(
		widget.NewFormItem("Theme", themeSel),
		widget.NewFormItem("UI scale", scale),
		widget.NewFormItem("Font size", font),
		widget.NewFormItem("Default value type", vt),
		widget.NewFormItem("Writable only", writable),
		widget.NewFormItem("Alignment", align),
		widget.NewFormItem("Result limit", limit),
		widget.NewFormItem("Process list", icons),
	)
	d := dialog.NewCustomConfirm("Settings", "Apply", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		a.cfg.UI.Theme = strings.ToLower(themeSel.Selected)
		if f, err := strconv.ParseFloat(strings.TrimSpace(scale.Text), 64); err == nil && f > 0 {
			a.cfg.UI.Scale = f
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(font.Text), 64); err == nil {
			a.cfg.UI.FontSize = f
		}
		a.cfg.Scan.ValueType = parseCEValueType(vt.Selected).String()
		a.cfg.Scan.WritableOnly = writable.Checked
		if n, err := strconv.Atoi(strings.TrimSpace(align.Text)); err == nil && n >= 0 {
			a.cfg.Scan.Alignment = n
		}
		if n, err := strconv.Atoi(strings.TrimSpace(limit.Text)); err == nil && n >= 0 {
			a.cfg.UI.ResultLimit = n
		}
		a.cfg.UI.ProcessIcons = icons.Checked
		a.showIcons = icons.Checked
		a.applyTheme()
		a.updateThemeChecks()
		a.saveConfig()
		if a.procList != nil {
			a.procList.Refresh()
		}
		if a.showIcons {
			go a.loadIcons()
		}
		a.setStatus("settings applied")
	}, a.win)
	d.Resize(fyne.NewSize(460, 520))
	d.Show()
}

// applyTheme rebuilds the theme from the config and refreshes the widgets.
func (a *App) applyTheme() {
	a.th = newTheme(parseScheme(a.cfg.UI.Theme), a.cfg.UI.FontSize)
	a.fapp.Settings().SetTheme(a.th)
	if a.foundList != nil {
		a.foundList.Refresh()
	}
	if a.table != nil {
		a.table.Refresh()
	}
	if a.procList != nil {
		a.procList.Refresh()
	}
	if a.disasmList != nil {
		a.disasmList.Refresh()
	}
	if a.hexList != nil {
		a.hexList.Refresh()
	}
}

func (a *App) saveConfig() {
	if err := a.cfg.Save(config.DefaultPath()); err != nil {
		a.fail(err)
	}
}

func (a *App) showAbout() {
	dialog.ShowInformation("About Firstspark",
		fmt.Sprintf("Firstspark\n\nA Cheat Engine style memory scanner, debugger and code patcher for Linux.\n\n"+
			"Released under the MIT License.\nCopyright (c) 2026 Lucas Cruz dos Reis.\n\n"+
			"Configuration: %s",
			config.DefaultPath()), a.win)
}

func titleCase(s string) string {
	if s == "" {
		return "System"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
