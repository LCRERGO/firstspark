//go:build gui

package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/sys/unix"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/hotkey"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// hotkeyAction is one configurable global-hotkey action.
type hotkeyAction struct {
	id     string
	label  string
	action func()
}

// hotkeyActions returns the configurable actions, mirroring the reference tool's
// Settings ▸ Hotkeys list restricted to what Firstspark implements.
func (a *App) hotkeyActions() []hotkeyAction {
	return []hotkeyAction{
		{"speedhack.toggle", "hotkeys.action.speedhack_toggle", a.toggleSpeedhack},
		{"speedhack.faster", "hotkeys.action.speedhack_faster", func() { a.adjustSpeedhack(1) }},
		{"speedhack.slower", "hotkeys.action.speedhack_slower", func() { a.adjustSpeedhack(-1) }},
		{"type.byte", "hotkeys.action.type_byte", func() { a.setValueType(scan.TypeByte) }},
		{"type.word", "hotkeys.action.type_word", func() { a.setValueType(scan.TypeWord) }},
		{"type.dword", "hotkeys.action.type_dword", func() { a.setValueType(scan.TypeDword) }},
		{"type.qword", "hotkeys.action.type_qword", func() { a.setValueType(scan.TypeQword) }},
		{"type.float", "hotkeys.action.type_float", func() { a.setValueType(scan.TypeFloat) }},
		{"type.double", "hotkeys.action.type_double", func() { a.setValueType(scan.TypeDouble) }},
		{"type.string", "hotkeys.action.type_string", func() { a.setValueType(scan.TypeString) }},
		{"type.aob", "hotkeys.action.type_aob", func() { a.setValueType(scan.TypeAOB) }},
		{"scan.new", "hotkeys.action.scan_new", a.firstScan},
		{"scan.new_exact", "hotkeys.action.scan_new_exact", func() { a.setScanType(scan.ModeExact); a.firstScan() }},
		{"scan.new_unknown", "hotkeys.action.scan_new_unknown", func() { a.setScanType(scan.ModeUnknown); a.firstScan() }},
		{"scan.next_exact", "hotkeys.action.scan_next_exact", func() { a.setScanType(scan.ModeExact); a.nextScan() }},
		{"scan.next_increased", "hotkeys.action.scan_next_increased", func() { a.setScanType(scan.ModeIncreased); a.nextScan() }},
		{"scan.next_decreased", "hotkeys.action.scan_next_decreased", func() { a.setScanType(scan.ModeDecreased); a.nextScan() }},
		{"scan.next_changed", "hotkeys.action.scan_next_changed", func() { a.setScanType(scan.ModeChanged); a.nextScan() }},
		{"scan.next_unchanged", "hotkeys.action.scan_next_unchanged", func() { a.setScanType(scan.ModeUnchanged); a.nextScan() }},
		{"scan.undo", "hotkeys.action.scan_undo", a.undoScan},
		{"scan.cancel", "hotkeys.action.scan_cancel", a.stopScan},
		{"debug.run", "hotkeys.action.debug_run", a.hotkeyDebugRun},
		{"process.pause", "hotkeys.action.pause", a.togglePauseProcess},
		{"process.attach_foreground", "hotkeys.action.attach_foreground", a.attachForegroundProcess},
	}
}

// setupHotkeys opens the X11 manager and registers the configured bindings.
func (a *App) setupHotkeys() {
	m, err := hotkey.New()
	if err != nil {
		log.Warn("global hotkeys unavailable", "err", err)
	}
	a.hotkeys = m
	a.applyHotkeys()
}

// applyHotkeys (re)registers every configured binding, recording a status per
// action for the Hotkeys window.
func (a *App) applyHotkeys() {
	a.hotkeyStatus = map[string]string{}
	if a.hotkeys == nil {
		return
	}
	actions := a.hotkeyActions()
	byID := make(map[string]hotkeyAction, len(actions))
	for _, act := range actions {
		byID[act.id] = act
		a.hotkeys.Unregister(act.id)
	}
	used := map[string]string{}
	for _, act := range actions {
		combo := strings.TrimSpace(a.cfg.Hotkeys[act.id])
		if combo == "" {
			continue
		}
		if other, ok := used[combo]; ok {
			a.hotkeyStatus[act.id] = i18n.Tf("hotkeys.status.duplicate", map[string]any{"Action": i18n.T(other)})
			log.Warn("duplicate hotkey", "combo", combo, "id", act.id)
			continue
		}
		if err := a.hotkeys.Register(act.id, combo, a.hotkeyCallback(act.action)); err != nil {
			a.hotkeyStatus[act.id] = err.Error()
			log.Warn("hotkey registration failed", "id", act.id, "combo", combo, "err", err)
			continue
		}
		used[combo] = act.label
		a.hotkeyStatus[act.id] = i18n.T("hotkeys.status.bound")
	}
	a.refreshHotkeyStatus()
}

// hotkeyCallback marshals a global-hotkey action onto the UI goroutine.
func (a *App) hotkeyCallback(action func()) func() {
	return func() { fyne.Do(action) }
}

func (a *App) setValueType(t scan.ValueType) {
	tab := a.tab()
	if tab == nil || tab.valueType == nil {
		return
	}
	tab.valueType.SetSelected(ceValueTypeLabel(t))
}

func (a *App) setScanType(m scan.ScanMode) {
	tab := a.tab()
	if tab == nil || tab.scanType == nil {
		return
	}
	tab.scanType.SetSelected(scanTypeLabel(m))
	tab.updateScanControls()
}

// adjustSpeedhack changes the scale by ±one delta and re-installs the hooks
// when the speedhack is active.
func (a *App) adjustSpeedhack(dir int) {
	delta := a.cfg.Speedhack.Delta
	if delta <= 0 {
		delta = 0.5
	}
	scale := a.cfg.Speedhack.Scale + float64(dir)*delta
	if scale < 0.1 {
		scale = 0.1
	}
	a.cfg.Speedhack.Scale = scale
	if a.speedScale != nil {
		a.speedScale.SetText(formatSpeed(scale))
	}
	if a.speedSlider != nil {
		a.speedSlider.Value = float64(nearestSpeedStep(scale))
		a.speedSlider.Refresh()
	}
	if a.speedApplied && a.speedMgr != nil {
		if err := a.speedMgr.UpdateScale(scale); err != nil {
			log.Warn("speedhack scale update failed", "err", err)
			a.fail(err)
		}
	}
	a.setStatusText(i18n.Tf("status.speedhack_scale", map[string]any{"Scale": scale}))
	if err := a.cfg.Save(config.DefaultPath()); err != nil {
		log.Warn("saving speedhack config failed", "err", err)
	}
}

// hotkeyDebugRun continues the debugger, opening it first when needed.
func (a *App) hotkeyDebugRun() {
	if a.dbgSession == nil {
		a.openDebugger()
		return
	}
	a.debuggerContinue()
}

// togglePauseProcess stops or resumes the target with SIGSTOP/SIGCONT.
func (a *App) togglePauseProcess() {
	if a.proc == nil {
		a.setStatusText(i18n.T("error.no_process"))
		return
	}
	if a.dbgSession != nil {
		a.setStatusText(i18n.T("status.pause_debugger"))
		return
	}
	pid := a.proc.PID
	sig := unix.SIGSTOP
	if a.paused {
		sig = unix.SIGCONT
	}
	if err := unix.Kill(pid, sig); err != nil {
		log.Warn("pause toggle failed", "pid", pid, "err", err)
		a.setStatusText(i18n.Tf("status.pause_failed", map[string]any{"PID": pid}))
		return
	}
	a.paused = !a.paused
	if a.paused {
		a.setStatusText(i18n.Tf("status.paused", map[string]any{"PID": pid}))
	} else {
		a.setStatusText(i18n.Tf("status.resumed", map[string]any{"PID": pid}))
	}
}

// resumeTarget sends SIGCONT if the target was paused, used before switching.
func (a *App) resumeTarget() {
	if !a.paused || a.proc == nil {
		return
	}
	if err := unix.Kill(a.proc.PID, unix.SIGCONT); err != nil {
		log.Debug("resume before switch failed", "pid", a.proc.PID, "err", err)
	}
	a.paused = false
}

// attachForegroundProcess selects the process owning the focused X11 window.
func (a *App) attachForegroundProcess() {
	pid, err := a.icons.x.activePID()
	if err != nil || pid <= 0 {
		a.setStatusText(i18n.T("status.no_foreground"))
		return
	}
	for i, p := range a.procs {
		if p.PID == pid {
			a.selectProcess(i)
			return
		}
	}
	a.setStatusText(i18n.Tf("status.foreground_not_found", map[string]any{"PID": pid}))
}

// keyCapture is a focusable field that records a key combination.
type keyCapture struct {
	*widget.Label
	combo string
	onSet func(string)
}

func newKeyCapture(onSet func(string)) *keyCapture {
	c := &keyCapture{Label: widget.NewLabel(""), onSet: onSet}
	c.ExtendBaseWidget(c)
	c.refresh()
	return c
}

func (c *keyCapture) refresh() {
	if c.combo == "" {
		c.SetText(i18n.T("hotkeys.unassigned"))
	} else {
		c.SetText(c.combo)
	}
}

func (c *keyCapture) set(combo string) {
	c.combo = combo
	c.refresh()
	if c.onSet != nil {
		c.onSet(combo)
	}
}

func (c *keyCapture) Tapped(*fyne.PointEvent) {
	if cv := fyne.CurrentApp().Driver().CanvasForObject(c); cv != nil {
		cv.Focus(c)
	}
}

func (c *keyCapture) FocusGained()            {}
func (c *keyCapture) FocusLost()              {}
func (c *keyCapture) TypedRune(rune)          {}
func (c *keyCapture) TypedKey(*fyne.KeyEvent) {}
func (c *keyCapture) KeyUp(*fyne.KeyEvent)    {}

var (
	_ fyne.Focusable  = (*keyCapture)(nil)
	_ desktop.Keyable = (*keyCapture)(nil)
	_ fyne.Tappable   = (*keyCapture)(nil)
)

func (c *keyCapture) KeyDown(ev *fyne.KeyEvent) {
	switch ev.Name {
	case desktop.KeyShiftLeft, desktop.KeyShiftRight,
		desktop.KeyControlLeft, desktop.KeyControlRight,
		desktop.KeyAltLeft, desktop.KeyAltRight,
		desktop.KeySuperLeft, desktop.KeySuperRight:
		return
	case fyne.KeyEscape:
		return
	case fyne.KeyBackspace, fyne.KeyDelete:
		c.set("")
		return
	}
	combo := formatCombo(currentModifiers(), string(ev.Name))
	if combo == "" {
		return
	}
	c.set(combo)
}

// formatCombo renders a modifier set and key as a canonical combo. A letter or
// digit needs at least one modifier; function keys may stand alone.
func formatCombo(mods fyne.KeyModifier, key string) string {
	if key == "" || key == "Unknown" {
		return ""
	}
	if mods == 0 && !isFunctionKey(key) {
		return ""
	}
	var parts []string
	if mods&fyne.KeyModifierControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&fyne.KeyModifierAlt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&fyne.KeyModifierShift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&fyne.KeyModifierSuper != 0 {
		parts = append(parts, "Super")
	}
	return strings.Join(append(parts, key), "+")
}

func isFunctionKey(key string) bool {
	if len(key) < 2 || (key[0] != 'F' && key[0] != 'f') {
		return false
	}
	for _, r := range key[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// showHotkeys opens the Hotkeys window, creating it lazily.
func (a *App) showHotkeys() {
	if a.hkWin == nil {
		a.hkWin = a.fapp.NewWindow(i18n.T("hotkeys.title"))
		a.hkWin.Resize(fyne.NewSize(560, 600))
		a.buildHotkeys()
	}
	a.refreshHotkeyStatus()
	a.hkWin.Show()
}

func (a *App) buildHotkeys() {
	a.hotkeyStatusLabels = map[string]*widget.Label{}
	var rows []fyne.CanvasObject
	if a.hotkeys == nil || !a.hotkeys.Available() {
		rows = append(rows, widget.NewLabel(i18n.T("hotkeys.unavailable")))
	}
	for _, act := range a.hotkeyActions() {
		act := act
		capture := newKeyCapture(func(combo string) { a.setHotkey(act.id, combo) })
		capture.combo = strings.TrimSpace(a.cfg.Hotkeys[act.id])
		capture.refresh()
		clear := newHintButton(i18n.T("hotkeys.clear"), "hotkeys.hint.clear", func() {
			capture.set("")
		})
		status := widget.NewLabel("")
		a.hotkeyStatusLabels[act.id] = status
		row := container.NewBorder(nil, nil,
			container.NewGridWrap(fyne.NewSize(210, 34), widget.NewLabel(i18n.T(act.label))), nil,
			container.NewBorder(nil, nil, nil, container.NewHBox(clear, status), capture))
		rows = append(rows, row)
	}
	content := container.NewVScroll(container.NewVBox(rows...))
	a.hkWin.SetContent(fynetooltip.AddWindowToolTipLayer(content, a.hkWin.Canvas()))
}

func (a *App) refreshHotkeyStatus() {
	for id, label := range a.hotkeyStatusLabels {
		if s, ok := a.hotkeyStatus[id]; ok {
			label.SetText(s)
		} else {
			label.SetText("")
		}
	}
}

// setHotkey stores a binding (or clears it) and re-registers everything.
func (a *App) setHotkey(id, combo string) {
	if a.cfg.Hotkeys == nil {
		a.cfg.Hotkeys = map[string]string{}
	}
	if combo == "" {
		delete(a.cfg.Hotkeys, id)
	} else {
		a.cfg.Hotkeys[id] = combo
	}
	if err := a.cfg.Save(config.DefaultPath()); err != nil {
		log.Warn("saving hotkeys failed", "err", err)
	}
	a.applyHotkeys()
}
