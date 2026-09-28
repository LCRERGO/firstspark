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

// settingsWidgets holds the Settings dialog controls.
type settingsWidgets struct {
	famSel, varSel, langSel    *widget.Select
	vt, logLevel, backend      *widget.Select
	scale, font                *widget.Entry
	align, limit, refresh      *widget.Entry
	snapshot, epsilon          *widget.Entry
	gdbPath, autoAttach        *widget.Entry
	writable, icons, autoRegex *widget.Check
}

// newSettingsWidgets builds and seeds the Settings controls.
func (a *App) newSettingsWidgets() *settingsWidgets {
	s := &settingsWidgets{
		famSel: widget.NewSelect([]string{
			familyLabel(familyCyberpunk), familyLabel(familyNord),
			familyLabel(familyDracula), familyLabel(familyTokyoNight),
		}, nil),
		varSel: widget.NewSelect([]string{
			variantLabel(variantLight), variantLabel(variantDark), variantLabel(variantSystem),
		}, nil),
		langSel:    widget.NewSelect(i18n.Supported(), nil),
		scale:      widget.NewEntry(),
		font:       widget.NewEntry(),
		vt:         widget.NewSelect(valueTypeOptions(), nil),
		writable:   widget.NewCheck(i18n.T("settings.writable_only"), nil),
		align:      widget.NewEntry(),
		limit:      widget.NewEntry(),
		refresh:    widget.NewEntry(),
		snapshot:   widget.NewEntry(),
		epsilon:    widget.NewEntry(),
		logLevel:   widget.NewSelect([]string{"debug", "info", "warn", "error"}, nil),
		backend:    widget.NewSelect([]string{"ptrace", "gdbmi"}, nil),
		gdbPath:    widget.NewEntry(),
		icons:      widget.NewCheck(i18n.T("settings.show_process_icons"), nil),
		autoAttach: widget.NewEntry(),
		autoRegex:  widget.NewCheck(i18n.T("settings.auto_attach_regex"), nil),
	}
	s.famSel.SetSelected(familyLabel(parseFamily(a.cfg.UI.Theme)))
	s.varSel.SetSelected(variantLabel(parseVariant(a.cfg.UI.ThemeVariant)))
	s.langSel.SetSelected(i18n.Language())
	s.scale.SetText(strconv.FormatFloat(a.cfg.UI.Scale, 'g', -1, 64))
	s.font.SetText(strconv.FormatFloat(a.cfg.UI.FontSize, 'g', -1, 64))
	s.vt.SetSelected(ceValueTypeLabel(a.defaultValueType()))
	s.writable.SetChecked(a.cfg.Scan.WritableOnly)
	s.align.SetText(strconv.Itoa(a.cfg.Scan.Alignment))
	s.limit.SetText(strconv.Itoa(a.cfg.UI.ResultLimit))
	s.refresh.SetText(strconv.Itoa(a.cfg.UI.RefreshMS))
	s.snapshot.SetText(strconv.FormatInt(a.cfg.Scan.SnapshotLimit, 10))
	s.epsilon.SetText(strconv.FormatFloat(a.cfg.Scan.FloatEpsilon, 'g', -1, 64))
	s.logLevel.SetSelected(a.cfg.Log.Level)
	if a.cfg.Debugger.Backend == "" {
		s.backend.SetSelected("ptrace")
	} else {
		s.backend.SetSelected(a.cfg.Debugger.Backend)
	}
	s.gdbPath.SetText(a.cfg.Debugger.GDBPath)
	s.icons.SetChecked(a.cfg.UI.ProcessIcons)
	s.autoAttach.SetText(a.cfg.Process.AutoAttach)
	s.autoAttach.SetPlaceHolder(i18n.T("settings.auto_attach_placeholder"))
	s.autoRegex.SetChecked(a.cfg.Process.AutoAttachRegex)
	return s
}

// form lays the controls out with their translated labels.
func (s *settingsWidgets) form() *widget.Form {
	return widget.NewForm(
		widget.NewFormItem(i18n.T("settings.theme"), s.famSel),
		widget.NewFormItem(i18n.T("settings.theme_variant"), s.varSel),
		widget.NewFormItem(i18n.T("settings.language"), s.langSel),
		widget.NewFormItem(i18n.T("settings.ui_scale"), s.scale),
		widget.NewFormItem(i18n.T("settings.font_size"), s.font),
		widget.NewFormItem(i18n.T("settings.default_value_type"), s.vt),
		widget.NewFormItem(i18n.T("settings.writable_only"), s.writable),
		widget.NewFormItem(i18n.T("settings.alignment"), s.align),
		widget.NewFormItem(i18n.T("settings.result_limit"), s.limit),
		widget.NewFormItem(i18n.T("settings.refresh_ms"), s.refresh),
		widget.NewFormItem(i18n.T("settings.snapshot_limit"), s.snapshot),
		widget.NewFormItem(i18n.T("settings.float_epsilon"), s.epsilon),
		widget.NewFormItem(i18n.T("settings.log_level"), s.logLevel),
		widget.NewFormItem(i18n.T("settings.debugger_backend"), s.backend),
		widget.NewFormItem(i18n.T("settings.gdb_path"), s.gdbPath),
		widget.NewFormItem(i18n.T("settings.process_list"), s.icons),
		widget.NewFormItem(i18n.T("settings.auto_attach"), s.autoAttach),
		widget.NewFormItem(i18n.T("settings.auto_attach_regex"), s.autoRegex),
	)
}

func (a *App) showSettings() {
	s := a.newSettingsWidgets()
	d := dialog.NewCustomConfirm(i18n.T("settings.title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		s.form(), func(ok bool) {
			if !ok {
				return
			}
			if a.applySettings(s) {
				dialog.ShowInformation(i18n.T("settings.title"), i18n.T("settings.language_restart"), a.win)
			}
		}, a.win)
	d.Resize(fyne.NewSize(480, 640))
	d.Show()
}

// applySettings stores the dialog values and reports whether the language
// changed (which needs a restart).
func (a *App) applySettings(s *settingsWidgets) bool {
	a.cfg.UI.Theme = string(parseFamilyLabel(s.famSel.Selected))
	a.cfg.UI.ThemeVariant = parseVariantLabel(s.varSel.Selected).String()
	langChanged := s.langSel.Selected != "" && s.langSel.Selected != a.cfg.UI.Language
	if s.langSel.Selected != "" {
		a.cfg.UI.Language = s.langSel.Selected
	}
	if f, ok := parseSettingFloat(s.scale.Text, 0); ok && f > 0 {
		a.cfg.UI.Scale = f
	}
	if f, ok := parseSettingFloat(s.font.Text, 0); ok {
		a.cfg.UI.FontSize = f
	}
	a.cfg.Scan.ValueType = parseCEValueType(s.vt.Selected).String()
	a.cfg.Scan.WritableOnly = s.writable.Checked
	if n, ok := parseSettingInt(s.align.Text, 0); ok {
		a.cfg.Scan.Alignment = n
	}
	if n, ok := parseSettingInt(s.limit.Text, 0); ok {
		a.cfg.UI.ResultLimit = n
	}
	if n, ok := parseSettingInt(s.refresh.Text, 50); ok {
		a.cfg.UI.RefreshMS = n
	}
	if n, ok := parseSettingInt64(s.snapshot.Text); ok {
		a.cfg.Scan.SnapshotLimit = n
	}
	if f, ok := parseSettingFloat(s.epsilon.Text, 0); ok {
		a.cfg.Scan.FloatEpsilon = f
	}
	if s.logLevel.Selected != "" {
		a.cfg.Log.Level = s.logLevel.Selected
	}
	if s.backend.Selected != "" {
		a.cfg.Debugger.Backend = s.backend.Selected
	}
	a.cfg.Debugger.GDBPath = strings.TrimSpace(s.gdbPath.Text)
	a.cfg.UI.ProcessIcons = s.icons.Checked
	a.showIcons = s.icons.Checked
	a.cfg.Process.AutoAttach = strings.TrimSpace(s.autoAttach.Text)
	a.cfg.Process.AutoAttachRegex = s.autoRegex.Checked
	a.setAutoAttach(a.cfg.Process.AutoAttach, a.cfg.Process.AutoAttachRegex)
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
	return langChanged
}

func parseSettingInt(text string, min int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	return n, err == nil && n >= min
}

func parseSettingInt64(text string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	return n, err == nil && n >= 0
}

func parseSettingFloat(text string, min float64) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	return f, err == nil && f >= min
}

// applyTheme rebuilds the theme from the config and refreshes the widgets.
func (a *App) applyTheme() {
	a.th = newTheme(parseFamily(a.cfg.UI.Theme), parseVariant(a.cfg.UI.ThemeVariant), a.cfg.UI.FontSize)
	a.fapp.Settings().SetTheme(a.th)
	for _, t := range a.tabs {
		if t.foundList != nil {
			t.foundList.Refresh()
		}
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
