//go:build gui

package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/config"
)

func (a *App) showSettings() {
	famSel := widget.NewSelect([]string{
		familyLabel(familyCyberpunk), familyLabel(familyNord),
		familyLabel(familyDracula), familyLabel(familyTokyoNight),
	}, nil)
	famSel.SetSelected(familyLabel(parseFamily(a.cfg.UI.Theme)))
	varSel := widget.NewSelect([]string{
		variantLabel(variantLight), variantLabel(variantDark), variantLabel(variantSystem),
	}, nil)
	varSel.SetSelected(variantLabel(parseVariant(a.cfg.UI.ThemeVariant)))
	langSel := widget.NewSelect(i18n.Supported(), nil)
	langSel.SetSelected(i18n.Language())
	scale := widget.NewEntry()
	scale.SetText(strconv.FormatFloat(a.cfg.UI.Scale, 'g', -1, 64))
	font := widget.NewEntry()
	font.SetText(strconv.FormatFloat(a.cfg.UI.FontSize, 'g', -1, 64))
	vt := widget.NewSelect(valueTypeOptions(), nil)
	vt.SetSelected(ceValueTypeLabel(a.defaultValueType()))
	writable := widget.NewCheck(i18n.T("settings.writable_only"), nil)
	writable.SetChecked(a.cfg.Scan.WritableOnly)
	align := widget.NewEntry()
	align.SetText(strconv.Itoa(a.cfg.Scan.Alignment))
	limit := widget.NewEntry()
	limit.SetText(strconv.Itoa(a.cfg.UI.ResultLimit))
	icons := widget.NewCheck(i18n.T("settings.show_process_icons"), nil)
	icons.SetChecked(a.cfg.UI.ProcessIcons)

	form := widget.NewForm(
		widget.NewFormItem(i18n.T("settings.theme"), famSel),
		widget.NewFormItem(i18n.T("settings.theme_variant"), varSel),
		widget.NewFormItem(i18n.T("settings.language"), langSel),
		widget.NewFormItem(i18n.T("settings.ui_scale"), scale),
		widget.NewFormItem(i18n.T("settings.font_size"), font),
		widget.NewFormItem(i18n.T("settings.default_value_type"), vt),
		widget.NewFormItem(i18n.T("settings.writable_only"), writable),
		widget.NewFormItem(i18n.T("settings.alignment"), align),
		widget.NewFormItem(i18n.T("settings.result_limit"), limit),
		widget.NewFormItem(i18n.T("settings.process_list"), icons),
	)
	d := dialog.NewCustomConfirm(i18n.T("settings.title"), i18n.T("action.apply"), i18n.T("action.cancel"), form, func(ok bool) {
		if !ok {
			return
		}
		a.cfg.UI.Theme = string(parseFamilyLabel(famSel.Selected))
		a.cfg.UI.ThemeVariant = parseVariantLabel(varSel.Selected).String()
		langChanged := langSel.Selected != "" && langSel.Selected != a.cfg.UI.Language
		if langSel.Selected != "" {
			a.cfg.UI.Language = langSel.Selected
		}
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
		a.setStatusText(i18n.T("status.settings_applied"))
		if langChanged {
			dialog.ShowInformation(i18n.T("settings.title"), i18n.T("settings.language_restart"), a.win)
		}
	}, a.win)
	d.Resize(fyne.NewSize(460, 520))
	d.Show()
}

// applyTheme rebuilds the theme from the config and refreshes the widgets.
func (a *App) applyTheme() {
	a.th = newTheme(parseFamily(a.cfg.UI.Theme), parseVariant(a.cfg.UI.ThemeVariant), a.cfg.UI.FontSize)
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
	dialog.ShowInformation(i18n.T("about.title"), i18n.T("about.body"), a.win)
}
