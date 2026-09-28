//go:build gui

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// toolTipEntry is a widget.Entry that can carry a hover hint. The tooltip
// library cannot extend an existing Entry, so the wrapper forwards the hover
// events to its embedded ToolTipWidgetExtend.
type toolTipEntry struct {
	widget.Entry
	ttwidget.ToolTipWidgetExtend
}

func newToolTipEntry() *toolTipEntry {
	e := &toolTipEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *toolTipEntry) ExtendBaseWidget(wid fyne.Widget) {
	e.ExtendToolTipWidget(wid)
	e.Entry.ExtendBaseWidget(wid)
}

func (e *toolTipEntry) MouseIn(ev *desktop.MouseEvent) { e.ToolTipWidgetExtend.MouseIn(ev) }
func (e *toolTipEntry) MouseMoved(ev *desktop.MouseEvent) {
	e.ToolTipWidgetExtend.MouseMoved(ev)
}
func (e *toolTipEntry) MouseOut() { e.ToolTipWidgetExtend.MouseOut() }

// TypedShortcut gives the application shortcuts a chance to run while the
// entry has focus, since Fyne does not fall through to canvas shortcuts then.
func (e *toolTipEntry) TypedShortcut(s fyne.Shortcut) {
	if appShortcutDispatch != nil && appShortcutDispatch(s) {
		return
	}
	e.Entry.TypedShortcut(s)
}

// toolTipper is the subset of a tooltip-enabled widget used to attach hints.
type toolTipper interface {
	SetToolTip(string)
}

func setHint(w toolTipper, text string) {
	w.SetToolTip(text)
}

// newHintEntry builds a single-line entry whose hint is the translated key.
func newHintEntry(key string) *toolTipEntry {
	e := newToolTipEntry()
	e.SetToolTip(i18n.T(key))
	return e
}

// newHintButton builds a button whose hint is the translated key.
func newHintButton(text, key string, tapped func()) *ttwidget.Button {
	b := ttwidget.NewButton(text, tapped)
	b.SetToolTip(i18n.T(key))
	return b
}

// newHintCheck builds a checkbox whose hint is the translated key.
func newHintCheck(text, key string, changed func(bool)) *ttwidget.Check {
	c := ttwidget.NewCheck(text, changed)
	c.SetToolTip(i18n.T(key))
	return c
}

// newHintSelect builds a dropdown whose hint is the translated key.
func newHintSelect(options []string, key string, changed func(string)) *ttwidget.Select {
	s := ttwidget.NewSelect(options, changed)
	s.SetToolTip(i18n.T(key))
	return s
}

// valueHintKey maps a value type to the translation key of its hover hint.
func valueHintKey(t scan.ValueType) string {
	switch t {
	case scan.TypeAOB:
		return "scan.hint.value_aob"
	case scan.TypeBinary:
		return "scan.hint.value_binary"
	case scan.TypeString, scan.TypeUTF16LE, scan.TypeUTF16BE, scan.TypeUTF32LE, scan.TypeUTF32BE:
		return "scan.hint.value_text"
	case scan.TypeFloat, scan.TypeDouble:
		return "scan.hint.value_float"
	case scan.TypeGrouped:
		return "scan.hint.value_grouped"
	default:
		return "scan.hint.value_int"
	}
}

// applyHints attaches the static hover hints to the scan controls.
func (a *scanTab) applyHints() {
	if a.scanType != nil {
		setHint(a.scanType, i18n.T("scan.hint.scan_type"))
	}
	if a.valueType != nil {
		setHint(a.valueType, i18n.T("scan.hint.value_type"))
	}
	if a.compareSelect != nil {
		setHint(a.compareSelect, i18n.T("scan.hint.compare"))
	}
	a.updateValueHint()
}

// updateValueHint refreshes the dynamic hint on the scan value boxes to match
// the selected value type.
func (a *scanTab) updateValueHint() {
	text := i18n.T(valueHintKey(parseCEValueType(a.valueType.Selected)))
	if a.valueEntry != nil {
		setHint(a.valueEntry, text)
	}
	if a.value2Entry != nil {
		setHint(a.value2Entry, text)
	}
}
