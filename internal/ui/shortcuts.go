//go:build gui

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// shortcutBinding pairs a key combination with the action it runs. The same
// list drives the window shortcuts and the dispatch used while a text field or
// the code editor has focus, because Fyne routes modified keys to the focused
// widget and bypasses canvas shortcuts.
type shortcutBinding struct {
	key    fyne.KeyName
	mod    fyne.KeyModifier
	action func()
}

// appShortcutDispatch is set while the GUI is running. Widgets that can hold
// focus call it from TypedShortcut so the shortcuts keep working there.
var appShortcutDispatch func(fyne.Shortcut) bool

// matchesShortcut reports whether sc is the given key combination.
func matchesShortcut(sc fyne.Shortcut, key fyne.KeyName, mod fyne.KeyModifier) bool {
	cs, ok := sc.(*desktop.CustomShortcut)
	return ok && cs.KeyName == key && cs.Modifier == mod
}

// appShortcuts returns the built-in Cheat Engine parity shortcuts. Bare keys
// (Delete, Enter, Space, F5/F6) cannot be registered here and are handled by
// the focused widget instead (see cheatTable.TypedKey).
func (a *App) appShortcuts() []shortcutBinding {
	return []shortcutBinding{
		{fyne.KeyM, fyne.KeyModifierControl, a.openMemoryViewer},
		{fyne.KeyB, fyne.KeyModifierControl, func() { a.browseRow(a.tableSel) }},
		{fyne.KeyD, fyne.KeyModifierControl, func() { a.disassembleRow(a.tableSel) }},
		{fyne.KeyE, fyne.KeyModifierControl, a.changeValueSelected},
		{fyne.KeyZ, fyne.KeyModifierControl, func() { a.undoValue(a.tableSel) }},
		{fyne.KeyE, fyne.KeyModifierControl | fyne.KeyModifierAlt, func() { a.changeValueBack(a.tableSel) }},
		{fyne.KeyReturn, fyne.KeyModifierControl, func() { a.changeDescriptionDialog(a.tableSel) }},
		{fyne.KeyH, fyne.KeyModifierControl | fyne.KeyModifierAlt, func() { a.setDisplay(a.tableSel, displayHex) }},
		{fyne.KeyH, fyne.KeyModifierControl, func() { a.assignHotkey(a.tableSel) }},
		{fyne.KeyA, fyne.KeyModifierControl | fyne.KeyModifierAlt, a.openAutoAssemble},
		{fyne.KeyD, fyne.KeyModifierControl | fyne.KeyModifierAlt, a.openDissect},
	}
}

// installShortcuts registers the window shortcuts and the focused-widget
// dispatch.
func (a *App) installShortcuts() {
	bindings := a.appShortcuts()
	appShortcutDispatch = func(sc fyne.Shortcut) bool {
		for _, b := range bindings {
			if matchesShortcut(sc, b.key, b.mod) {
				b.action()
				return true
			}
		}
		return false
	}
	canvas := a.win.Canvas()
	for _, b := range bindings {
		sc := &desktop.CustomShortcut{KeyName: b.key, Modifier: b.mod}
		action := b.action
		canvas.AddShortcut(sc, func(fyne.Shortcut) { action() })
	}
	// Ctrl+C is canvas-only so focused text fields keep their own copy.
	canvas.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierControl},
		func(fyne.Shortcut) { a.copySelection() })
}

// cheatTable adds the Cheat Engine table key bindings. Fyne delivers bare keys
// to the focused widget rather than the shortcut system, so Delete, Enter,
// Space and the F-keys are handled here.
type cheatTable struct {
	widget.Table
	app *App
}

func (t *cheatTable) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyDelete:
		t.app.deleteRow(t.app.tableSel)
		return
	case fyne.KeyReturn, fyne.KeyEnter:
		t.app.changeValueDialog(t.app.tableSel)
		return
	case fyne.KeySpace:
		t.app.toggleFreezeRow(t.app.tableSel)
		return
	case fyne.KeyF5:
		t.app.findWhatWrites(t.app.tableSel, true)
		return
	case fyne.KeyF6:
		t.app.findWhatWrites(t.app.tableSel, false)
		return
	}
	t.Table.TypedKey(ev)
}
