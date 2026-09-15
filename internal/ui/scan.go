//go:build gui

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

const scanLabelWidth float32 = 96

// scanRow lays out a label and a control on one row, right-aligning the label
// the way a form does.
func scanRow(label string, w fyne.CanvasObject) *fyne.Container {
	lbl := widget.NewLabel(label)
	lbl.Alignment = fyne.TextAlignTrailing
	return container.NewBorder(nil, nil,
		container.NewGridWrap(fyne.NewSize(scanLabelWidth, 34), lbl), nil, w)
}

// scanPanel mirrors Cheat Engine's scan region: the three scan buttons at the
// top, then the scan value with a Hex checkbox beside it, a second value for
// "Value between", and the scan and value type dropdowns.
func (a *App) scanPanel() fyne.CanvasObject {
	a.scanBtn = widget.NewButton("First Scan", a.firstScan)
	a.nextBtn = widget.NewButton("Next Scan", a.nextScan)
	a.undoBtn = widget.NewButton("Undo Scan", a.undoScan)
	buttons := container.NewHBox(a.scanBtn, a.nextBtn, a.undoBtn)

	valueRow := container.NewBorder(nil, nil, nil, a.hexBox, a.valueEntry)
	a.value2Row = scanRow("and", a.value2Entry)

	body := container.NewVBox(
		a.th.heading("Scan", a.th.size+2, a.pal().primary),
		buttons,
		scanRow("Scan Value", valueRow),
		a.value2Row,
		scanRow("Scan Type", a.scanType),
		scanRow("Value Type", container.NewBorder(nil, nil, nil, widget.NewButton("…", a.showCustomTypes), a.valueType)),
		scanRow("Compare", a.compareEntry),
		widget.NewSeparator(),
		a.th.heading("Memory Scan Options", a.th.size, a.pal().primary),
		a.writable,
		scanRow("Alignment", a.alignEntry),
		widget.NewSeparator(),
		a.speedhack,
	)
	return container.NewVScroll(container.NewPadded(body))
}

// valuePlaceholder describes what the scan value box expects for a scan type.
func valuePlaceholder(mode scan.ScanMode) string {
	switch mode {
	case scan.ModeBetween:
		return "lower bound"
	case scan.ModeIncreasedBy, scan.ModeDecreasedBy:
		return "delta"
	case scan.ModeUnknown:
		return "not used"
	default:
		return "value or AOB pattern"
	}
}

// scanAction runs a first scan when no session exists, otherwise a next scan.
func (a *App) scanAction() {
	if a.session == nil {
		a.firstScan()
		return
	}
	a.nextScan()
}

func (a *App) firstScan() {
	if a.proc == nil {
		a.fail(fmt.Errorf("no process selected"))
		return
	}
	opts, err := a.scanOptions()
	if err != nil {
		a.fail(err)
		return
	}
	s := scan.NewSession(a.proc, opts)
	if err := s.First(); err != nil {
		a.fail(err)
		return
	}
	a.session = s
	a.setResults(s.Results())
	a.setStatus("first scan: %d results", len(a.results))
	a.updateScanControls()
}

func (a *App) nextScan() {
	if a.session == nil {
		a.fail(fmt.Errorf("run a first scan first"))
		return
	}
	opts, err := a.scanOptions()
	if err != nil {
		a.fail(err)
		return
	}
	a.session.SetMode(opts.Mode)
	a.session.SetCompare(opts.Compare)
	if modeNeedsValue(opts.Mode) {
		a.session.SetValue(opts.Value)
	}
	if opts.Mode == scan.ModeBetween {
		a.session.SetValue2(opts.Value2)
	}
	if err := a.session.Next(); err != nil {
		a.fail(err)
		return
	}
	a.setResults(a.session.Results())
	a.setStatus("next scan: %d results", len(a.results))
	a.updateScanControls()
}

func (a *App) scanOptions() (scan.Options, error) {
	opts := scan.DefaultOptions()
	opts.Type = parseCEValueType(a.valueType.Selected)
	opts.Mode = parseCEScanType(a.scanType.Selected)
	cmp, err := scan.ParseCompareOp(a.compareEntry.Text)
	if err != nil {
		return opts, err
	}
	opts.Compare = cmp
	opts.WritableOnly = a.writable.Checked
	opts.Alignment = a.cfg.Scan.Alignment
	if n, err := strconv.Atoi(strings.TrimSpace(a.alignEntry.Text)); err == nil && n >= 0 {
		opts.Alignment = n
	}
	opts.SnapshotLimit = a.cfg.Scan.SnapshotLimit
	opts.Epsilon = a.cfg.Scan.FloatEpsilon
	if modeNeedsValue(opts.Mode) {
		v, err := scan.ParseValue(opts.Type, a.valueText())
		if err != nil {
			return opts, err
		}
		opts.Value = v
	}
	if opts.Mode == scan.ModeBetween {
		v, err := scan.ParseValue(opts.Type, a.upperText())
		if err != nil {
			return opts, err
		}
		opts.Value2 = v
	}
	return opts, nil
}

// upperText returns the upper bound for a Value-between scan.
func (a *App) upperText() string {
	s := strings.TrimSpace(a.value2Entry.Text)
	if !a.hexBox.Checked {
		return s
	}
	switch parseCEValueType(a.valueType.Selected) {
	case scan.TypeFloat, scan.TypeDouble, scan.TypeString, scan.TypeAOB, scan.TypeBinary:
		return s
	}
	if s == "" {
		return s
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		s = "0x" + s
	}
	if neg {
		s = "-" + s
	}
	return s
}

func (a *App) undoScan() {
	if a.session == nil || !a.session.CanUndo() {
		a.setStatus("nothing to undo")
		return
	}
	a.session.Undo()
	a.setResults(a.session.Results())
	a.setStatus("undo: %d results", len(a.results))
	a.updateScanControls()
}

// valueText returns the scan value, converting a bare hex string to 0x form
// when the Hex box is checked.
func (a *App) valueText() string {
	s := strings.TrimSpace(a.valueEntry.Text)
	if !a.hexBox.Checked {
		return s
	}
	switch parseCEValueType(a.valueType.Selected) {
	case scan.TypeFloat, scan.TypeDouble, scan.TypeString, scan.TypeAOB, scan.TypeBinary:
		return s
	}
	if s == "" {
		return s
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		s = "0x" + s
	}
	if neg {
		s = "-" + s
	}
	return s
}

func (a *App) toggleSpeedhack() {
	a.speedhack.SetChecked(!a.speedhack.Checked)
	a.cfg.Speedhack.Enabled = a.speedhack.Checked
	if a.speedhack.Checked {
		a.setStatus("speedhack enabled")
	} else {
		a.setStatus("speedhack disabled")
	}
	_ = a.cfg.Save(config.DefaultPath())
}
