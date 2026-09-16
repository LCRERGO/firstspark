//go:build gui

package ui

import (
	"context"
	"errors"
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

// flexRow lays out children horizontally: children with a positive weight
// share the space left after the rigid (weight 0) children take their MinSize.
type flexRow struct {
	weights []float32
}

func (f flexRow) MinSize(objs []fyne.CanvasObject) fyne.Size {
	size := fyne.NewSize(0, 0)
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		size.Width += m.Width
		if m.Height > size.Height {
			size.Height = m.Height
		}
	}
	return size
}

func (f flexRow) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	var rigid, totalWeight float32
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		if w := f.weight(i); w > 0 {
			totalWeight += w
		} else {
			rigid += o.MinSize().Width
		}
	}
	leftover := size.Width - rigid
	if leftover < 0 {
		leftover = 0
	}
	x := float32(0)
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		w := f.weight(i)
		cw := o.MinSize().Width
		if w > 0 && totalWeight > 0 {
			cw = leftover * w / totalWeight
		}
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(cw, size.Height))
		x += cw
	}
}

func (f flexRow) weight(i int) float32 {
	if i < len(f.weights) {
		return f.weights[i]
	}
	return 0
}

// fixedHeight gives a child a fixed height while filling the available width.
type fixedHeight struct{ h float32 }

func (f fixedHeight) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, f.h) }

func (f fixedHeight) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(size.Width, f.h))
	}
}

// scanLabel is a fixed-width, right-aligned row label.
func scanLabel(text string) fyne.CanvasObject {
	lbl := widget.NewLabel(text)
	lbl.Alignment = fyne.TextAlignTrailing
	return container.NewGridWrap(fyne.NewSize(scanLabelWidth, 34), lbl)
}

// scanRow lays out a label and a control on one row, right-aligning the label
// the way a form does.
func scanRow(label string, w fyne.CanvasObject) *fyne.Container {
	return container.NewBorder(nil, nil, scanLabel(label), nil, w)
}

// scanPanel mirrors Cheat Engine's scan region: the scan buttons at the top,
// the progress bar and status, then the scan value with a Hex checkbox beside
// it and the scan and value type dropdowns.
func (a *App) scanPanel() fyne.CanvasObject {
	a.scanBtn = widget.NewButton("First Scan", a.firstScan)
	a.nextBtn = widget.NewButton("Next Scan", a.nextScan)
	a.undoBtn = widget.NewButton("Undo Scan", a.undoScan)
	a.stopBtn = widget.NewButton("Stop", a.stopScan)
	buttons := container.NewHBox(a.scanBtn, a.nextBtn, a.undoBtn, a.stopBtn)

	a.scanProgress = widget.NewProgressBar()
	a.scanProgress.SetValue(0)
	progressRow := container.New(fixedHeight{h: 8}, a.scanProgress)
	a.scanStatus = widget.NewLabel("")

	// Cheat Engine keeps both value boxes on one row for "Value between".
	a.andLabel = widget.NewLabel("and")
	a.valuePair = container.New(flexRow{weights: []float32{1, 0, 1}},
		a.valueEntry, a.andLabel, a.value2Entry)
	valueRow := container.NewBorder(nil, nil, scanLabel("Scan Value"), a.hexBox, a.valuePair)

	a.scopeSelect = widget.NewSelect([]string{
		"All writable", "Heap + stack + exec + BSS", "All readable",
	}, nil)
	a.scopeSelect.SetSelected("All writable")

	body := container.NewVBox(
		a.th.heading("Scan", a.th.size+2, a.pal().primary),
		buttons,
		progressRow,
		a.scanStatus,
		valueRow,
		scanRow("Scan Type", a.scanType),
		scanRow("Value Type", container.NewBorder(nil, nil, nil, widget.NewButton("…", a.showCustomTypes), a.valueType)),
		scanRow("Compare", a.compareEntry),
		widget.NewSeparator(),
		a.th.heading("Memory Scan Options", a.th.size, a.pal().primary),
		a.writable,
		scanRow("Alignment", a.alignEntry),
		scanRow("Region scope", a.scopeSelect),
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

func parseScope(label string) scan.RegionScope {
	switch label {
	case "Heap + stack + exec + BSS":
		return scan.ScopeHeapStackExecBSS
	case "All readable":
		return scan.ScopeAllReadable
	default:
		return scan.ScopeAllWritable
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
	if a.scanning {
		return
	}
	if a.proc == nil {
		a.fail(fmt.Errorf("no process selected"))
		return
	}
	opts, err := a.scanOptions()
	if err != nil {
		a.fail(err)
		return
	}
	a.runScan(scan.NewSession(a.proc, opts), true)
}

func (a *App) nextScan() {
	if a.scanning {
		return
	}
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
	a.runScan(a.session, false)
}

// runScan executes a scan on a background goroutine, reporting progress and
// allowing cancellation.
func (a *App) runScan(s *scan.Session, first bool) {
	ctx, cancel := context.WithCancel(context.Background())
	a.scanCancel = cancel
	a.scanning = true
	if a.scanProgress != nil {
		a.scanProgress.SetValue(0)
	}
	a.scanStatus.SetText("scanning…")
	a.updateScanControls()

	onProgress := func(p scan.Progress) {
		fyne.Do(func() { a.updateScanProgress(p) })
	}
	go func() {
		var err error
		if first {
			err = s.First(ctx, onProgress)
		} else {
			err = s.Next(ctx, onProgress)
		}
		fyne.Do(func() { a.finishScan(s, first, err) })
	}()
}

func (a *App) updateScanProgress(p scan.Progress) {
	if a.scanProgress != nil {
		if p.TotalBytes > 0 {
			a.scanProgress.SetValue(float64(p.ScannedBytes) / float64(p.TotalBytes))
		} else {
			a.scanProgress.SetValue(0)
		}
	}
	if a.scanStatus != nil {
		a.scanStatus.SetText(fmt.Sprintf("Scanned %s / %s, %d matches",
			humanBytes(p.ScannedBytes), humanBytes(p.TotalBytes), p.Matches))
	}
}

func (a *App) finishScan(s *scan.Session, first bool, err error) {
	a.scanning = false
	a.scanCancel = nil
	switch {
	case errors.Is(err, context.Canceled):
		a.setStatus("scan cancelled")
	case err != nil:
		a.fail(err)
	default:
		if first {
			a.session = s
		}
		a.setResults(s.Results())
		if first {
			a.setStatus("first scan: %d results", len(a.results))
		} else {
			a.setStatus("next scan: %d results", len(a.results))
		}
	}
	if a.scanProgress != nil {
		a.scanProgress.SetValue(1)
	}
	if a.scanStatus != nil {
		a.scanStatus.SetText(fmt.Sprintf("%d results", len(a.results)))
	}
	a.updateScanControls()
}

func (a *App) stopScan() {
	if a.scanCancel != nil {
		a.scanCancel()
	}
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
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
	opts.MaxResults = a.cfg.UI.ResultLimit
	opts.Scope = parseScope(a.scopeSelect.Selected)
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
	return a.hexValue(a.value2Entry.Text)
}

// valueText returns the scan value, converting a bare hex string to 0x form
// when the Hex box is checked.
func (a *App) valueText() string {
	return a.hexValue(a.valueEntry.Text)
}

func (a *App) hexValue(raw string) string {
	s := strings.TrimSpace(raw)
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
	if a.scanning {
		return
	}
	if a.session == nil || !a.session.CanUndo() {
		a.setStatus("nothing to undo")
		return
	}
	a.session.Undo()
	a.setResults(a.session.Results())
	a.setStatus("undo: %d results", len(a.results))
	a.updateScanControls()
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
