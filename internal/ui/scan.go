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

func (a *App) scanPanel() fyne.CanvasObject {
	form := widget.NewForm(
		widget.NewFormItem("Scan Value", a.valueEntry),
		widget.NewFormItem("Scan Type", a.scanType),
		widget.NewFormItem("Value Type", a.valueType),
		widget.NewFormItem("Compare", a.compareEntry),
	)
	buttons := container.NewHBox(
		widget.NewButton("First Scan", a.firstScan),
		widget.NewButton("Next Scan", a.nextScan),
	)
	options := widget.NewForm(widget.NewFormItem("Alignment", a.alignEntry))
	body := container.NewVBox(
		a.th.heading("Scan", a.th.size+2, a.pal().primary),
		form,
		a.hexBox,
		buttons,
		widget.NewSeparator(),
		a.th.heading("Memory Scan Options", a.th.size, a.pal().primary),
		a.writable,
		options,
		widget.NewSeparator(),
		a.speedhack,
	)
	return container.NewVScroll(container.NewPadded(body))
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
	if err := a.session.Next(); err != nil {
		a.fail(err)
		return
	}
	a.setResults(a.session.Results())
	a.setStatus("next scan: %d results", len(a.results))
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
	return opts, nil
}

// valueText returns the scan value, converting a bare hex string to 0x form
// when the Hex box is checked.
func (a *App) valueText() string {
	s := strings.TrimSpace(a.valueEntry.Text)
	if !a.hexBox.Checked {
		return s
	}
	switch parseCEValueType(a.valueType.Selected) {
	case scan.TypeFloat, scan.TypeDouble, scan.TypeString, scan.TypeAOB:
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
