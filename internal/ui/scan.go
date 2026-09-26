//go:build gui

package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/scan"
	"github.com/LCRERGO/firstspark/pkg/speedhack"
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

// progressLine is a minimal progress indicator: a thin track with a filled
// portion, without the chrome of widget.ProgressBar.
type progressLine struct {
	widget.BaseWidget
	fraction float32
}

func newProgressLine() *progressLine {
	p := &progressLine{}
	p.ExtendBaseWidget(p)
	return p
}

// SetValue sets the progress as a fraction between 0 and 1.
func (p *progressLine) SetValue(f float32) {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	if f == p.fraction {
		return
	}
	p.fraction = f
	p.Refresh()
}

func (p *progressLine) CreateRenderer() fyne.WidgetRenderer {
	return &progressLineRenderer{
		track: canvas.NewRectangle(theme.Color(theme.ColorNameSeparator)),
		fill:  canvas.NewRectangle(theme.Color(theme.ColorNamePrimary)),
		line:  p,
	}
}

type progressLineRenderer struct {
	track *canvas.Rectangle
	fill  *canvas.Rectangle
	line  *progressLine
}

func (r *progressLineRenderer) Layout(size fyne.Size) {
	r.track.Move(fyne.NewPos(0, 0))
	r.track.Resize(size)
	r.fill.Move(fyne.NewPos(0, 0))
	r.fill.Resize(fyne.NewSize(size.Width*r.line.fraction, size.Height))
}

func (r *progressLineRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 4) }

func (r *progressLineRenderer) Refresh() {
	r.track.FillColor = theme.Color(theme.ColorNameSeparator)
	r.fill.FillColor = theme.Color(theme.ColorNamePrimary)
	r.track.Refresh()
	r.fill.Refresh()
}

func (r *progressLineRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.track, r.fill}
}

func (r *progressLineRenderer) Destroy() {}

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
	a.scanBtn = newHintButton(i18n.T("scan.first"), "scan.hint.first", a.firstScan)
	a.nextBtn = newHintButton(i18n.T("scan.next"), "scan.hint.next", a.nextScan)
	a.undoBtn = newHintButton(i18n.T("scan.undo"), "scan.hint.undo", a.undoScan)
	a.stopBtn = newHintButton(i18n.T("scan.stop"), "scan.hint.stop", a.stopScan)
	buttons := container.NewHBox(a.scanBtn, a.nextBtn, a.undoBtn, a.stopBtn)

	a.scanProgress = newProgressLine()
	a.scanStatus = widget.NewLabel("")

	// Cheat Engine keeps both value boxes on one row for "Value between".
	a.andLabel = widget.NewLabel(i18n.T("scan.and"))
	a.valuePair = container.New(flexRow{weights: []float32{1, 0, 1}},
		a.valueEntry, a.andLabel, a.value2Entry)
	valueRow := container.NewBorder(nil, nil, scanLabel(i18n.T("scan.value_label")), a.hexBox, a.valuePair)

	a.scopeSelect = newHintSelect(scopeLabels(), "scan.hint.scope", nil)
	a.scopeSelect.SetSelected(scopeLabel(scan.ScopeAllWritable))

	body := container.NewVBox(
		a.th.heading(i18n.T("scan.heading"), a.th.size+2, a.pal().primary),
		buttons,
		a.scanProgress,
		a.scanStatus,
		valueRow,
		scanRow(i18n.T("scan.type_label"), a.scanType),
		scanRow(i18n.T("scan.value_type_label"), container.NewBorder(nil, nil, nil, newHintButton("…", "scan.hint.custom_types", a.showCustomTypes), a.valueType)),
		scanRow(i18n.T("scan.compare_label"), a.compareSelect),
		widget.NewSeparator(),
		a.th.heading(i18n.T("scan.options_heading"), a.th.size, a.pal().primary),
		a.writable,
		scanRow(i18n.T("scan.alignment_label"), a.alignEntry),
		scanRow(i18n.T("scan.region_scope_label"),
			container.NewBorder(nil, nil, nil, newHintButton(i18n.T("regions.manage"), "scan.hint.regions", a.showRegionManager), a.scopeSelect)),
		scanRow(i18n.T("scan.executable_label"), a.execSelect),
		a.cowCheck,
		scanRow(i18n.T("scan.range_label"),
			container.New(flexRow{weights: []float32{1, 0, 1}}, a.startEntry, widget.NewLabel(i18n.T("scan.range_to")), a.stopEntry)),
		widget.NewSeparator(),
		a.speedhack,
		scanRow(i18n.T("scan.speedhack_scale"), a.speedScale),
	)
	return container.NewVScroll(container.NewPadded(body))
}

// valuePlaceholder describes what the scan value box expects for a scan type.
func valuePlaceholder(mode scan.ScanMode) string {
	switch mode {
	case scan.ModeBetween:
		return i18n.T("scan.placeholder.lower_bound")
	case scan.ModeIncreasedBy, scan.ModeDecreasedBy:
		return i18n.T("scan.placeholder.delta")
	case scan.ModeUnknown:
		return i18n.T("scan.placeholder.not_used")
	default:
		return i18n.T("app.value_placeholder")
	}
}

// scopeOption pairs a region scope with its translation key.
type scopeOption struct {
	key   string
	scope scan.RegionScope
}

var scopeOptions = []scopeOption{
	{"scope.all_writable", scan.ScopeAllWritable},
	{"scope.heap_stack_exec_bss", scan.ScopeHeapStackExecBSS},
	{"scope.all_readable", scan.ScopeAllReadable},
}

func scopeLabels() []string {
	out := make([]string, len(scopeOptions))
	for i, o := range scopeOptions {
		out[i] = i18n.T(o.key)
	}
	return out
}

func scopeLabel(s scan.RegionScope) string {
	for _, o := range scopeOptions {
		if o.scope == s {
			return i18n.T(o.key)
		}
	}
	return i18n.T("scope.all_writable")
}

func parseScope(label string) scan.RegionScope {
	for _, o := range scopeOptions {
		if i18n.T(o.key) == label {
			return o.scope
		}
	}
	return scan.ScopeAllWritable
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
		a.fail(fmt.Errorf("%s", i18n.T("error.no_process")))
		return
	}
	opts, err := a.scanOptions()
	if err != nil {
		a.fail(err)
		return
	}
	log.Info("first scan", "pid", a.proc.PID, "mode", opts.Mode, "type", opts.Type)
	a.runScan(scan.NewSession(a.proc, opts), true)
}

func (a *App) nextScan() {
	if a.scanning {
		return
	}
	if a.session == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.run_first_scan")))
		return
	}
	opts, err := a.scanOptions()
	if err != nil {
		a.fail(err)
		return
	}
	log.Info("next scan", "pid", a.proc.PID, "mode", opts.Mode)
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
	a.scanStatus.SetText(i18n.T("scan.scanning"))
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
			a.scanProgress.SetValue(float32(float64(p.ScannedBytes) / float64(p.TotalBytes)))
		} else {
			a.scanProgress.SetValue(0)
		}
	}
	if a.scanStatus != nil {
		a.scanStatus.SetText(i18n.Tf("scan.progress", map[string]any{
			"Scanned": humanBytes(p.ScannedBytes),
			"Total":   humanBytes(p.TotalBytes),
			"Matches": p.Matches,
		}))
	}
}

func (a *App) finishScan(s *scan.Session, first bool, err error) {
	a.scanning = false
	a.scanCancel = nil
	switch {
	case errors.Is(err, context.Canceled):
		a.setStatusText(i18n.T("status.scan_cancelled"))
	case err != nil:
		a.fail(err)
	default:
		if first {
			a.session = s
			a.updateScanTypeOptions()
		}
		a.setResults(s.Results())
		if first {
			a.setStatusText(i18n.Tf("status.first_scan", map[string]any{"Count": len(a.results)}))
		} else {
			a.setStatusText(i18n.Tf("status.next_scan", map[string]any{"Count": len(a.results)}))
		}
	}
	if a.scanProgress != nil {
		a.scanProgress.SetValue(1)
	}
	if a.scanStatus != nil {
		a.scanStatus.SetText(i18n.Tf("scan.results_count", map[string]any{"Count": len(a.results)}))
	}
	log.Info("scan finished", "first", first, "results", len(a.results), "err", err)
	a.updateScanControls()
}

func (a *App) stopScan() {
	if a.scanCancel != nil {
		log.Info("scan cancel requested")
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
	opts.Compare = parseCompareLabel(a.compareSelect.Selected)
	opts.WritableOnly = a.writable.Checked
	opts.Alignment = a.cfg.Scan.Alignment
	if n, err := strconv.Atoi(strings.TrimSpace(a.alignEntry.Text)); err == nil && n >= 0 {
		opts.Alignment = n
	}
	opts.SnapshotLimit = a.cfg.Scan.SnapshotLimit
	opts.MaxResults = a.cfg.UI.ResultLimit
	opts.Scope = parseScope(a.scopeSelect.Selected)
	if len(a.regionSel) > 0 {
		opts.Regions = a.regionSel
		opts.Scope = scan.ScopeAllReadable
	}
	opts.Epsilon = a.cfg.Scan.FloatEpsilon
	opts.Executable = parseExecLabel(a.execSelect.Selected)
	opts.CopyOnWrite = a.cowCheck.Checked
	var err error
	if opts.Start, err = optionalAddr(a.startEntry.Text); err != nil {
		return opts, err
	}
	if opts.Stop, err = optionalAddr(a.stopEntry.Text); err != nil {
		return opts, err
	}
	if opts.Type == scan.TypeGrouped {
		gp, err := scan.ParseGrouped(a.valueText())
		if err != nil {
			return opts, err
		}
		opts.Grouped = gp
	} else if modeNeedsValue(opts.Mode) {
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

// optionalAddr parses an optional hexadecimal address entry.
func optionalAddr(s string) (uint64, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	return parseAddress(s)
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
	if !isBareHexLiteral(s) {
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

// isBareHexLiteral reports whether s is a single optionally-signed hex literal
// rather than an expression, so the Hex toggle does not corrupt inputs such as
// "0xFF + 1".
func isBareHexLiteral(s string) bool {
	if s == "" {
		return false
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func (a *App) undoScan() {
	if a.scanning {
		return
	}
	if a.session == nil || !a.session.CanUndo() {
		a.setStatusText(i18n.T("status.nothing_undo"))
		return
	}
	a.session.Undo()
	a.setResults(a.session.Results())
	a.setStatusText(i18n.Tf("status.undo", map[string]any{"Count": len(a.results)}))
	a.updateScanControls()
}

func (a *App) toggleSpeedhack() {
	a.speedhack.SetChecked(!a.speedhack.Checked)
}

// setSpeedhack installs or removes the time-scaling hooks on the selected
// process. It is driven by the checkbox and the Tools menu.
func (a *App) setSpeedhack(on bool) {
	if on == a.speedApplied {
		return
	}
	if on {
		if err := a.installSpeedhack(); err != nil {
			a.fail(err)
			a.speedhack.SetChecked(false)
			return
		}
		a.speedApplied = true
		a.setStatusText(i18n.T("status.speedhack_enabled"))
	} else {
		a.removeSpeedhack()
		a.speedApplied = false
		a.setStatusText(i18n.T("status.speedhack_disabled"))
	}
	a.cfg.Speedhack.Enabled = on
	if a.speedScale != nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(a.speedScale.Text), 64); err == nil && f > 0 {
			a.cfg.Speedhack.Scale = f
		}
	}
	if err := a.cfg.Save(config.DefaultPath()); err != nil {
		log.Warn("saving speedhack config failed", "err", err)
	}
}

// installSpeedhack attaches, hooks the time functions and detaches.
func (a *App) installSpeedhack() error {
	if a.proc == nil {
		return fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	scale := a.cfg.Speedhack.Scale
	if a.speedScale != nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(a.speedScale.Text), 64); err == nil && f > 0 {
			scale = f
		}
	}
	be, err := debugger.NewPtrace(a.proc.PID)
	if err != nil {
		return err
	}
	defer be.Close()
	if err := be.Attach(); err != nil {
		return err
	}
	defer be.Detach()
	for _, sym := range speedhack.DefaultSymbols {
		h, err := speedhack.Hook(be, a.proc.PID, sym, scale)
		if err != nil {
			a.removeSpeedhack()
			return fmt.Errorf("speedhack: %s: %w", sym, err)
		}
		a.speedHooks = append(a.speedHooks, h)
	}
	log.Info("speedhack installed", "pid", a.proc.PID, "scale", scale, "hooks", len(a.speedHooks))
	return nil
}

// removeSpeedhack restores the hooked prologues.
func (a *App) removeSpeedhack() {
	if len(a.speedHooks) == 0 {
		return
	}
	pid := 0
	if a.proc != nil {
		pid = a.proc.PID
		if be, err := debugger.NewPtrace(a.proc.PID); err == nil {
			if err := be.Attach(); err == nil {
				for _, h := range a.speedHooks {
					if err := h.Remove(); err != nil {
						log.Warn("speedhack hook removal failed", "pid", pid, "err", err)
					}
				}
				if err := be.Detach(); err != nil {
					log.Debug("speedhack detach failed", "pid", pid, "err", err)
				}
			} else {
				log.Warn("speedhack cleanup re-attach failed", "pid", pid, "err", err)
			}
			_ = be.Close()
		} else {
			log.Warn("speedhack cleanup skipped, target gone", "pid", pid, "err", err)
		}
	}
	log.Info("speedhack removed", "pid", pid)
	a.speedHooks = nil
}
