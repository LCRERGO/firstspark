//go:build gui

package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// cheatHeaders returns the translated cheat-table column titles.
func cheatHeaders() []string {
	return []string{
		i18n.T("header.active"),
		i18n.T("header.description"),
		i18n.T("header.address"),
		i18n.T("header.type"),
		i18n.T("header.value"),
	}
}

// displayFormat selects how a cheat-table value is rendered.
type displayFormat int

const (
	displayDefault displayFormat = iota
	displayHex
	displayBinary
)

// pointerChain resolves an address as [[base]+off0]+off1... When module is set
// the base is the module's load base plus offset, so the chain survives ASLR.
type pointerChain struct {
	module  string
	base    uint64
	offset  uint64
	offsets []int64
}

func (c *pointerChain) baseAddr(p *mem.Process) (uint64, error) {
	if c.module == "" {
		return c.base, nil
	}
	regions, err := mem.Regions(p.PID)
	if err != nil {
		return 0, err
	}
	mb, ok := mem.ModuleBase(regions, c.module)
	if !ok {
		return 0, fmt.Errorf("module %q not found", c.module)
	}
	return mb + c.offset, nil
}

func resolvePointer(p *mem.Process, c *pointerChain) (uint64, error) {
	base, err := c.baseAddr(p)
	if err != nil {
		return 0, err
	}
	if len(c.offsets) == 0 {
		return base, nil
	}
	addr, err := p.ReadUint64(base)
	if err != nil {
		return 0, err
	}
	addr += uint64(c.offsets[0])
	for i := 1; i < len(c.offsets); i++ {
		addr, err = p.ReadUint64(addr)
		if err != nil {
			return 0, err
		}
		addr += uint64(c.offsets[i])
	}
	return addr, nil
}

// formatEntryValue renders a value using the entry's display format.
func (a *App) formatEntryValue(e *tableEntry) string {
	switch e.display {
	case displayHex:
		return hexOf(e.value)
	case displayBinary:
		return binaryOf(e.value)
	default:
		if e.unsigned {
			if d := scan.TypeByID(e.typ); d != nil && d.Kind == scan.KindInt {
				return strconv.FormatUint(e.value.Uint64(), 10)
			}
		}
		return e.value.String()
	}
}

func hexOf(v scan.Value) string {
	switch len(v.Raw) {
	case 1, 2, 4, 8:
		return fmt.Sprintf("0x%X", v.Uint64())
	default:
		parts := make([]string, len(v.Raw))
		for i, b := range v.Raw {
			parts[i] = fmt.Sprintf("%02X", b)
		}
		return strings.Join(parts, " ")
	}
}

func binaryOf(v scan.Value) string {
	switch len(v.Raw) {
	case 1, 2, 4, 8:
		return strconv.FormatUint(v.Uint64(), 2)
	default:
		return hexOf(v)
	}
}

// foundHeaders returns the translated Found-list column titles.
func foundHeaders() []string {
	return []string{
		i18n.T("header.address"),
		i18n.T("header.value"),
		i18n.T("header.previous"),
	}
}

func (a *scanTab) buildFoundList() {
	headers := foundHeaders()
	a.foundList = &foundTable{
		tab: a,
		Table: widget.NewTable(
			func() (int, int) { return a.foundLen(), len(headers) },
			func() fyne.CanvasObject { return a.newFoundCell() },
			func(id widget.TableCellID, o fyne.CanvasObject) { a.updateFoundCell(id, o) },
		),
	}
	a.foundList.ShowHeaderRow = true
	a.foundList.CreateHeader = func() fyne.CanvasObject { return a.newFoundHeader() }
	a.foundList.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		h := o.(*foundHeader)
		h.col = id.Col
		if id.Col < 0 || id.Col >= len(headers) {
			h.setText("")
			h.setColor(a.pal().primary)
			return
		}
		h.setText(headers[id.Col])
		h.setColor(a.pal().primary)
	}
	a.foundList.SetColumnWidth(0, 170)
	a.foundList.SetColumnWidth(1, 130)
	a.foundList.SetColumnWidth(2, 130)
}

// foundTable wraps the Found list table so it can handle the reference tool's
// bare-key bindings (Delete, Enter), which Fyne routes to the focused widget
// rather than the shortcut system.
type foundTable struct {
	*widget.Table
	tab *scanTab
}

func (t *foundTable) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyDelete:
		t.tab.deleteSelectedFound()
		return
	case fyne.KeyReturn, fyne.KeyEnter:
		t.tab.addFoundToTable()
		return
	}
	t.Table.TypedKey(ev)
}

// foundLimit returns the display cap (0 = unlimited).
func (a *scanTab) foundLimit() int {
	if a.cfg.UI.ResultLimit > 0 {
		return a.cfg.UI.ResultLimit
	}
	return 0
}

// foundLen is the number of rows the Found list displays: the collected results
// capped by ui.result_limit.
func (a *scanTab) foundLen() int {
	n := len(a.results)
	if lim := a.foundLimit(); lim > 0 && n > lim {
		return lim
	}
	return n
}

func (a *scanTab) updateFoundCell(id widget.TableCellID, o fyne.CanvasObject) {
	c := o.(*foundCell)
	c.row, c.col = id.Row, id.Col
	idx := a.foundResult(id.Row)
	if idx < 0 {
		c.setText("")
		c.setColor(a.pal().text)
		return
	}
	c.setText(a.foundCellText(id.Col, idx))
	switch {
	case a.isFoundSelected(idx):
		c.setColor(a.pal().primary)
	case id.Col == 0 && a.isStatic(a.results[idx].Addr):
		c.setColor(a.pal().success)
	default:
		c.setColor(a.pal().text)
	}
}

// foundCellText renders one Found-list cell: Address, live Value or Previous.
func (a *scanTab) foundCellText(col, idx int) string {
	r := a.results[idx]
	switch col {
	case 0:
		if name, off, ok := a.staticInfo(r.Addr); ok {
			return fmt.Sprintf("%s+0x%x", name, off)
		}
		return fmt.Sprintf("0x%x", r.Addr)
	case 1:
		if v, ok := a.foundLive[idx]; ok {
			return a.displayFoundValue(v)
		}
		return a.displayFoundValue(r.Value)
	case 2:
		if len(r.Previous.Raw) == 0 {
			return "-"
		}
		return a.displayFoundValue(r.Previous)
	default:
		return ""
	}
}

// displayFoundValue renders a Found value using the list-wide display format.
func (a *scanTab) displayFoundValue(v scan.Value) string {
	switch a.foundDisplay {
	case displayHex:
		return hexOf(v)
	case displayBinary:
		return binaryOf(v)
	default:
		return v.String()
	}
}

// foundCell is a Found-list cell that paints its own text colour and reports
// taps, double-clicks and right-clicks.
type foundCell struct {
	widget.BaseWidget
	tab  *scanTab
	text *canvas.Text
	row  int
	col  int
}

func (a *scanTab) newFoundCell() *foundCell {
	c := &foundCell{tab: a, text: a.th.monoText("", a.pal().text)}
	c.ExtendBaseWidget(c)
	return c
}

func (c *foundCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.text)
}

func (c *foundCell) setText(s string) {
	c.text.Text = s
	c.text.Refresh()
}

func (c *foundCell) setColor(col color.Color) {
	c.text.Color = col
	c.text.Refresh()
}

func (c *foundCell) Tapped(*fyne.PointEvent) { c.tab.foundTapped(c.row) }

func (c *foundCell) DoubleTapped(*fyne.PointEvent) { c.tab.foundDoubleTapped(c.row) }

func (c *foundCell) MouseDown(e *desktop.MouseEvent) {
	c.tab.clickMod = e.Modifier
	if e.Button == desktop.MouseButtonSecondary {
		c.tab.foundMenu(c.row, e.Position, c)
		return
	}
	c.tab.selectFoundRow(c.tab.foundResult(c.row), e.Modifier)
}

// foundHeader is a clickable column header that sorts the Found list.
type foundHeader struct {
	widget.BaseWidget
	tab  *scanTab
	text *canvas.Text
	col  int
}

func (a *scanTab) newFoundHeader() *foundHeader {
	h := &foundHeader{tab: a, text: a.th.monoText("", a.pal().primary)}
	h.ExtendBaseWidget(h)
	return h
}

func (h *foundHeader) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(h.text)
}

func (h *foundHeader) setText(s string) {
	h.text.Text = s
	h.text.Refresh()
}

func (h *foundHeader) setColor(c color.Color) {
	h.text.Color = c
	h.text.Refresh()
}

func (h *foundHeader) Tapped(*fyne.PointEvent) { h.tab.sortFound(h.col) }

// sortFound cycles a column through ascending, descending and unsorted scan
// order, reordering only the foundOrder view.
func (a *scanTab) sortFound(col int) {
	if col < 0 || col > 2 {
		return
	}
	switch {
	case a.foundSortCol != col:
		a.foundSortCol = col
		a.foundSortAsc = true
	case a.foundSortAsc:
		a.foundSortAsc = false
	default:
		a.foundSortCol = -1
	}
	// Sorting by the live Value column compares every collected result, so it
	// needs a one-time full read rather than the displayed-only refresh.
	if a.foundSortCol == 1 {
		a.readAllFoundValues()
	}
	a.applyFoundSort()
	a.refreshFound()
}

// readAllFoundValues snapshots every result's live value for a Value sort.
func (a *scanTab) readAllFoundValues() {
	if a.proc == nil || len(a.results) == 0 {
		a.sortLive = nil
		return
	}
	live := make(map[int]scan.Value, len(a.results))
	for i := range a.results {
		r := a.results[i]
		w := len(r.Value.Raw)
		if w == 0 {
			continue
		}
		if raw, err := a.proc.Read(r.Addr, w); err == nil && len(raw) == w {
			live[i] = scan.NewValue(r.Value.Type, raw)
		}
	}
	a.sortLive = live
}

// applyFoundSort rebuilds foundOrder from the active sort column.
func (a *scanTab) applyFoundSort() {
	if a.foundSortCol < 0 {
		a.foundOrder = identityOrder(len(a.results))
		return
	}
	order := identityOrder(len(a.results))
	col := a.foundSortCol
	sort.SliceStable(order, func(i, j int) bool {
		if a.foundSortAsc {
			return a.foundLess(order[i], order[j], col)
		}
		return a.foundLess(order[j], order[i], col)
	})
	a.foundOrder = order
}

// foundLess orders two results by column: address, live value or previous.
func (a *scanTab) foundLess(i, j, col int) bool {
	switch col {
	case 0:
		return a.results[i].Addr < a.results[j].Addr
	case 1:
		return valueLess(a.sortValue(i), a.sortValue(j))
	default:
		return valueLess(a.results[i].Previous, a.results[j].Previous)
	}
}

// sortValue returns the freshest known value for a result: the full read taken
// for a Value sort, the displayed-row refresh, or the stored scan value.
func (a *scanTab) sortValue(i int) scan.Value {
	if v, ok := a.sortLive[i]; ok {
		return v
	}
	if v, ok := a.foundLive[i]; ok {
		return v
	}
	return a.results[i].Value
}

// valueLess orders two values numerically when both are numeric and textually
// otherwise.
func valueLess(a, b scan.Value) bool {
	ta, tb := scan.TypeByID(a.Type), scan.TypeByID(b.Type)
	if numericKind(ta) && numericKind(tb) {
		return numericValue(ta, a) < numericValue(tb, b)
	}
	return a.String() < b.String()
}

func numericKind(t *scan.Type) bool {
	return t != nil && (t.Kind == scan.KindInt || t.Kind == scan.KindFloat)
}

func numericValue(t *scan.Type, v scan.Value) float64 {
	if t.Kind == scan.KindFloat {
		return v.Float64()
	}
	return float64(v.Int64())
}

// refreshFound repaints the found list.
func (a *scanTab) refreshFound() {
	if a.foundList != nil {
		a.foundList.Refresh()
	}
}

// selectFoundRow updates the Found-list selection: plain selects one row, Ctrl
// toggles and Shift selects a contiguous range.
func (a *scanTab) selectFoundRow(id int, mod fyne.KeyModifier) {
	if id < 0 || id >= len(a.results) {
		return
	}
	a.activePanel = panelFound
	switch {
	case mod&fyne.KeyModifierControl != 0:
		if a.foundMulti == nil {
			a.foundMulti = map[int]bool{}
			if a.foundSel >= 0 {
				a.foundMulti[a.foundSel] = true
			}
		}
		if a.foundMulti[id] {
			delete(a.foundMulti, id)
		} else {
			a.foundMulti[id] = true
		}
		a.foundSel = id
	case mod&fyne.KeyModifierShift != 0 && a.foundSel >= 0:
		lo, hi := a.foundSel, id
		if lo > hi {
			lo, hi = hi, lo
		}
		m := map[int]bool{}
		for i := lo; i <= hi; i++ {
			m[i] = true
		}
		a.foundMulti = m
	default:
		a.foundMulti = nil
		a.foundSel = id
	}
	if a.foundList != nil {
		a.foundList.Refresh()
	}
}

// isFoundSelected reports whether result id is selected.
func (a *scanTab) isFoundSelected(id int) bool {
	if a.foundMulti != nil && a.foundMulti[id] {
		return true
	}
	return id == a.foundSel
}

// selectedFoundIndices returns the selected result indices in order.
func (a *scanTab) selectedFoundIndices() []int {
	var out []int
	if a.foundMulti != nil {
		for i := range a.results {
			if a.foundMulti[i] {
				out = append(out, i)
			}
		}
	}
	if len(out) == 0 && a.foundSel >= 0 && a.foundSel < len(a.results) {
		out = append(out, a.foundSel)
	}
	return out
}

// foundTapped handles a plain click: select and browse the address.
func (a *scanTab) foundTapped(row int) {
	mod := a.clickMod
	a.clickMod = 0
	if mod != 0 {
		return
	}
	a.selectFound(a.foundResult(row))
}

// foundDoubleTapped adds the double-clicked result to the cheat table, like
// the reference tool.
func (a *scanTab) foundDoubleTapped(row int) {
	idx := a.foundResult(row)
	if idx < 0 {
		return
	}
	a.addResultToTable(idx)
}

// foundMenu shows the Found-list context menu for a view row.
func (a *scanTab) foundMenu(row int, rel fyne.Position, anchor fyne.CanvasObject) {
	idx := a.foundResult(row)
	if idx < 0 {
		return
	}
	if !a.isFoundSelected(idx) {
		a.foundMulti = nil
		a.foundSel = idx
	}
	a.activePanel = panelFound
	if a.foundList != nil {
		a.foundList.Refresh()
	}
	addr := a.results[idx].Addr
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(i18n.T("menu.change_value"), a.changeValueSelected),
		fyne.NewMenuItem(i18n.T("results.add_to_table"), func() { a.addResultToTable(idx) }),
		fyne.NewMenuItem(i18n.T("menu.copy_address"), func() { a.copyFoundAddr(idx) }),
		fyne.NewMenuItem(i18n.T("menu.copy_value"), func() { a.copyFoundValue(idx) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.browse"), func() { a.browseFoundAddr(addr) }),
		fyne.NewMenuItem(i18n.T("menu.disassemble"), func() { a.browseFoundAddr(addr) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.find_writes"), func() { a.findWhatWritesAddr(addr, true) }),
		fyne.NewMenuItem(i18n.T("menu.find_accesses"), func() { a.findWhatWritesAddr(addr, false) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.show_decimal"), func() { a.setFoundDisplay(displayDefault) }),
		fyne.NewMenuItem(i18n.T("menu.show_hex"), func() { a.setFoundDisplay(displayHex) }),
		fyne.NewMenuItem(i18n.T("menu.show_binary"), func() { a.setFoundDisplay(displayBinary) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.delete"), a.deleteFoundResults),
	)
	widget.ShowPopUpMenuAtRelativePosition(menu, a.win.Canvas(), rel, anchor)
}

// copyFoundAddr puts a result's address on the clipboard.
func (a *scanTab) copyFoundAddr(idx int) {
	if idx < 0 || idx >= len(a.results) {
		return
	}
	a.fapp.Clipboard().SetContent(fmt.Sprintf("0x%x", a.results[idx].Addr))
}

// copyFoundValue puts a result's current value on the clipboard.
func (a *scanTab) copyFoundValue(idx int) {
	if idx < 0 || idx >= len(a.results) {
		return
	}
	v := a.results[idx].Value
	if live, ok := a.foundLive[idx]; ok {
		v = live
	}
	a.fapp.Clipboard().SetContent(a.displayFoundValue(v))
}

// setFoundDisplay switches the Found list value format.
func (a *scanTab) setFoundDisplay(d displayFormat) {
	a.foundDisplay = d
	a.refreshFound()
}

// copySelection handles Ctrl+C: it copies the active panel's selection, the
// cheat-table addresses or the selected Found address. Text fields keep their
// own clipboard handling because Fyne consumes the shortcut first.
func (a *App) copySelection() {
	if a.activePanel == panelCheatTable {
		a.copyTableSelection()
		return
	}
	t := a.tab()
	if t == nil || t.foundSel < 0 || t.foundSel >= len(t.results) {
		return
	}
	t.copyFoundAddr(t.foundSel)
}

// pasteSelection handles Ctrl+V: it pastes clipboard addresses into the cheat
// table.
func (a *App) pasteSelection() {
	a.pasteTable()
}

// browseFoundAddr opens the memory viewer at addr.
func (a *App) browseFoundAddr(addr uint64) {
	a.openMemoryViewer()
	a.loadMemory(addr)
}

// deleteFoundResults drops every selected scan result from the list and the
// session, so a later Next Scan does not bring them back.
func (a *scanTab) deleteFoundResults() {
	sel := a.selectedFoundIndices()
	if len(sel) == 0 {
		return
	}
	drop := make(map[uint64]bool, len(sel))
	for _, i := range sel {
		drop[a.results[i].Addr] = true
	}
	kept := a.results[:0]
	for _, r := range a.results {
		if !drop[r.Addr] {
			kept = append(kept, r)
		}
	}
	for i := len(kept); i < len(a.results); i++ {
		a.results[i] = scan.Result{}
	}
	a.results = kept
	if a.session != nil {
		a.session.Delete(func(r scan.Result) bool { return !drop[r.Addr] })
	}
	a.applyFoundSort()
	a.foundLive = nil
	a.sortLive = nil
	a.foundSel = -1
	a.foundMulti = nil
	a.updateFoundCount()
	if a.foundList != nil {
		a.foundList.Refresh()
	}
	a.updateScanControls()
}

// deleteSelectedFound removes the selected Found results. It backs Delete on
// the focused list and Ctrl+Delete when the Found panel is active.
func (a *App) deleteSelectedFound() {
	if t := a.tab(); t != nil {
		t.deleteFoundResults()
	}
}

// addFoundToTable adds the selected Found results to the cheat table (Enter),
// mirroring the list's double-click.
func (t *scanTab) addFoundToTable() {
	if t.foundSel < 0 && len(t.selectedFoundIndices()) == 0 {
		return
	}
	t.addResultToTable(t.foundSel)
}

// browseSelected routes Ctrl+B by the active panel.
func (a *App) browseSelected() {
	if a.activePanel == panelFound {
		if t := a.tab(); t != nil && t.foundSel >= 0 && t.foundSel < len(t.results) {
			a.browseFoundAddr(t.results[t.foundSel].Addr)
		}
		return
	}
	a.browseRow(a.tableSel)
}

// disassembleSelected routes Ctrl+D by the active panel.
func (a *App) disassembleSelected() {
	if a.activePanel == panelFound {
		if t := a.tab(); t != nil && t.foundSel >= 0 && t.foundSel < len(t.results) {
			a.browseFoundAddr(t.results[t.foundSel].Addr)
		}
		return
	}
	a.disassembleRow(a.tableSel)
}

// findWritesSelected routes Ctrl+F5 / Ctrl+F6 by the active panel.
func (a *App) findWritesSelected(writeOnly bool) {
	if a.activePanel == panelFound {
		if t := a.tab(); t != nil && t.foundSel >= 0 && t.foundSel < len(t.results) {
			a.findWhatWritesAddr(t.results[t.foundSel].Addr, writeOnly)
		}
		return
	}
	a.findWhatWrites(a.tableSel, writeOnly)
}

// setHexSelected routes Ctrl+Alt+H by the active panel.
func (a *App) setHexSelected() {
	if a.activePanel == panelFound {
		if t := a.tab(); t != nil {
			t.setFoundDisplay(displayHex)
		}
		return
	}
	a.setDisplay(a.tableSel, displayHex)
}

// selectAll selects every row of the active panel.
func (a *App) selectAll() {
	if a.activePanel == panelFound {
		if t := a.tab(); t != nil {
			t.selectAllFound()
		}
	}
}

// selectAllFound selects every displayed Found row (Ctrl+A on the list).
func (a *scanTab) selectAllFound() {
	n := a.foundLen()
	if n == 0 {
		return
	}
	a.activePanel = panelFound
	a.foundMulti = make(map[int]bool, n)
	for row := 0; row < n; row++ {
		if idx := a.foundResult(row); idx >= 0 {
			a.foundMulti[idx] = true
		}
	}
	a.refreshFound()
}

// foundResult maps a display row to a result index. Sorting can reorder
// foundOrder without touching a.results or the scan session.
func (a *scanTab) foundResult(row int) int {
	if row < 0 || row >= a.foundLen() {
		return -1
	}
	if row < len(a.foundOrder) {
		return a.foundOrder[row]
	}
	return row
}

// identityOrder is the unsorted display order over n results.
func identityOrder(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}

// refreshFoundValues re-reads the displayed rows' live values on a background
// goroutine and refreshes the module map used to colour static addresses. Only
// the displayed set (capped by ui.result_limit) is read, never every collected
// result, so the cost stays bounded regardless of how many matches were found.
func (a *scanTab) refreshFoundValues() {
	if a.proc == nil || len(a.results) == 0 {
		a.foundLive = nil
		a.foundRegions = nil
		return
	}
	a.foundRegions, _ = mem.Regions(a.proc.PID)
	n := a.foundLen()
	type pending struct {
		idx  int
		addr uint64
		typ  scan.ValueType
		w    int
	}
	rows := make([]pending, 0, n)
	for row := 0; row < n; row++ {
		i := a.foundResult(row)
		if i < 0 {
			continue
		}
		r := a.results[i]
		w := len(r.Value.Raw)
		if w == 0 {
			continue
		}
		rows = append(rows, pending{i, r.Addr, r.Value.Type, w})
	}
	if len(rows) == 0 {
		a.foundLive = nil
		return
	}
	proc := a.proc
	a.foundSeq++
	seq := a.foundSeq
	go func() {
		live := make(map[int]scan.Value, len(rows))
		for _, r := range rows {
			raw, err := proc.Read(r.addr, r.w)
			if err != nil || len(raw) != r.w {
				continue
			}
			live[r.idx] = scan.NewValue(r.typ, raw)
		}
		fyne.Do(func() {
			if a.proc != proc || a.foundSeq != seq {
				return
			}
			a.foundLive = live
			a.refreshFound()
		})
	}()
}

// staticInfo reports whether addr belongs to a file-backed module and, if so,
// its module name and offset from the load base.
func (a *scanTab) staticInfo(addr uint64) (string, uint64, bool) {
	r, ok := mem.RegionFor(a.foundRegions, addr)
	if !ok || !r.FileBacked() || addr < r.Offset {
		return "", 0, false
	}
	base := r.Start - r.Offset
	if addr < base {
		return "", 0, false
	}
	name := filepath.Base(strings.TrimSuffix(r.Path, " (deleted)"))
	return name, addr - base, true
}

// isStatic reports whether addr is inside a file-backed module region.
func (a *scanTab) isStatic(addr uint64) bool {
	_, _, ok := a.staticInfo(addr)
	return ok
}

func (a *scanTab) foundPanel() fyne.CanvasObject {
	head := container.NewHBox(
		a.th.heading(i18n.T("results.found"), a.th.size+2, a.pal().primary),
		a.foundCount,
		layout.NewSpacer(),
		newHintButton(i18n.T("results.add_to_table"), "results.hint.add_to_table", func() { a.addResultToTable(a.foundSel) }),
	)
	return container.NewBorder(head, nil, nil, nil, a.foundList)
}

func (a *scanTab) selectFound(id int) {
	if id < 0 || id >= len(a.results) {
		return
	}
	a.foundSel = id
	a.foundMulti = nil
	a.activePanel = panelFound
	if a.foundList != nil {
		a.foundList.Refresh()
	}
	a.loadMemory(a.results[id].Addr)
}

func (a *scanTab) setResults(r []scan.Result) {
	a.results = append([]scan.Result(nil), r...)
	a.foundLive = nil
	a.sortLive = nil
	a.applyFoundSort()
	a.foundSel = -1
	a.foundMulti = nil
	a.updateFoundCount()
	if a.foundList != nil {
		a.foundList.Refresh()
	}
}

// updateFoundCount paints the Found-list label, distinguishing the collected
// count from the number of rows actually shown when the display is capped.
func (a *scanTab) updateFoundCount() {
	if a.foundCount == nil {
		return
	}
	total := len(a.results)
	if lim := a.foundLimit(); lim > 0 && total > lim {
		a.foundCount.SetText(i18n.Tf("app.found_count_limited", map[string]any{"Shown": lim, "Total": total}))
		return
	}
	a.foundCount.SetText(i18n.Tf("app.found_count", map[string]any{"Count": total}))
}

// buildCheatTable creates the five-column cheat table.
func (a *App) buildCheatTable() {
	headers := cheatHeaders()
	a.table = &cheatTable{app: a}
	a.table.Length = func() (int, int) { return len(a.entries), len(headers) }
	a.table.CreateCell = func() fyne.CanvasObject { return a.newDataCell() }
	a.table.UpdateCell = func(id widget.TableCellID, o fyne.CanvasObject) { a.updateDataCell(id, o) }
	a.table.ExtendBaseWidget(a.table)
	a.table.ShowHeaderRow = true
	a.table.CreateHeader = func() fyne.CanvasObject { return a.monoText("") }
	a.table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		t := o.(*canvas.Text)
		if id.Col < 0 || id.Col >= len(headers) {
			t.Text = ""
			t.Refresh()
			return
		}
		t.Text = headers[id.Col]
		t.Color = a.pal().primary
		t.Refresh()
	}
	a.table.SetColumnWidth(0, 56)
	a.table.SetColumnWidth(1, 200)
	a.table.SetColumnWidth(2, 140)
	a.table.SetColumnWidth(3, 96)
	a.table.SetColumnWidth(4, 160)
}

func (a *App) cheatPanel() fyne.CanvasObject {
	head := a.th.heading(i18n.T("results.cheat_table"), a.th.size+2, a.pal().primary)
	actions := container.NewHBox(
		newHintButton(i18n.T("results.add_address_manually"), "results.hint.add_address", a.addAddressDialog),
		newHintButton(i18n.T("menu.change_value"), "results.hint.change_value", a.changeValueSelected),
		newHintButton(i18n.T("menu.delete"), "results.hint.delete", func() { a.deleteRow(a.tableSel) }),
		newHintButton(i18n.T("results.clear_list"), "results.hint.clear", a.clearTable),
	)
	return container.NewBorder(head, actions, nil, nil, a.table)
}

func (a *App) newDataCell() *dataCell {
	c := &dataCell{app: a, text: canvas.NewText("", a.pal().text)}
	c.text.TextSize = a.th.size
	c.text.TextStyle = fyne.TextStyle{Monospace: true}
	c.ExtendBaseWidget(c)
	return c
}

func (a *App) updateDataCell(id widget.TableCellID, o fyne.CanvasObject) {
	c := o.(*dataCell)
	c.row, c.col = id.Row, id.Col
	if id.Row < 0 || id.Row >= len(a.entries) {
		c.setText("")
		c.setColor(a.pal().text)
		return
	}
	c.setText(a.cellText(id))
	if a.isTableSelected(a.entries[id.Row]) {
		c.setColor(a.pal().primary)
	} else {
		c.setColor(a.entryColor(a.entries[id.Row]))
	}
	c.setMono(id.Col != 1)
}

func (a *App) cellText(id widget.TableCellID) string {
	e := a.entries[id.Row]
	switch id.Col {
	case 0:
		if e.frozen {
			return "X"
		}
		return ""
	case 1:
		indent := strings.Repeat("  ", e.depth)
		if e.group {
			marker := "▾ "
			if !e.expanded {
				marker = "▸ "
			}
			if e.script != "" {
				return indent + marker + e.desc + "  [script]"
			}
			return indent + marker + e.desc
		}
		return indent + e.desc
	case 2:
		if e.group {
			return ""
		}
		if e.addr != 0 {
			return fmt.Sprintf("0x%x", e.addr)
		}
		return e.expr
	case 3:
		if e.group {
			return ""
		}
		return ceValueTypeLabel(e.typ)
	case 4:
		if e.group {
			return ""
		}
		if e.bit != nil {
			return e.bit.format(e.value.Raw)
		}
		return a.formatEntryValue(e)
	default:
		return ""
	}
}

func (a *App) tableTapped(row, col int) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	e := a.entries[row]
	mod := a.clickMod
	a.clickMod = 0
	a.tableSel = row
	a.activePanel = panelCheatTable
	if mod != 0 {
		a.table.Refresh()
		return
	}
	now := time.Now()
	double := a.lastTapRow == row && a.lastTapCol == col && now.Sub(a.lastTap) < doubleTapWindow
	a.lastTap, a.lastTapRow, a.lastTapCol = now, row, col
	if double {
		a.tableDoubleTapped(row, col)
		return
	}
	if col == 1 && e.group {
		a.toggleExpand(row)
		return
	}
	if col == 0 {
		a.toggleFreezeRow(row)
		return
	}
	a.table.Refresh()
}

// doubleTapWindow is how close two clicks must be to count as a double-click.
const doubleTapWindow = 350 * time.Millisecond

// tableDoubleTapped implements the reference tool's cell editing: double-clicking the
// Description renames the record, and double-clicking the Value opens the
// change-value form.
func (a *App) tableDoubleTapped(row, col int) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	switch col {
	case 1:
		a.changeDescriptionDialog(row)
	case 4:
		a.changeValueDialog(row)
	}
}

func (a *App) tableMenu(row, col int, rel fyne.Position, anchor fyne.CanvasObject) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	if !a.isTableSelected(a.entries[row]) {
		a.tableMulti = nil
		a.tableSel = row
	}
	a.activePanel = panelCheatTable
	if a.entries[row].group {
		e := a.entries[row]
		var items []*fyne.MenuItem
		if e.script != "" {
			items = append(items,
				fyne.NewMenuItem(i18n.T("menu.run_script"), func() { a.runEntryScript(e) }),
				fyne.NewMenuItem(i18n.T("menu.disable_script"), func() { a.disableEntryScript(e) }),
				fyne.NewMenuItemSeparator(),
			)
		}
		items = append(items,
			fyne.NewMenuItem(i18n.T("menu.change_description"), func() { a.changeDescriptionDialog(row) }),
			fyne.NewMenuItem(i18n.T("menu.set_children_value"), func() { a.setChildrenValueDialog(row) }),
			fyne.NewMenuItem(i18n.T("menu.freeze"), func() { a.toggleFreezeRow(row) }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem(i18n.T("menu.create_group"), a.groupSelection),
			fyne.NewMenuItem(i18n.T("menu.duplicate"), a.duplicateSelection),
			fyne.NewMenuItemSeparator(),
			a.moveUpItem(row), a.moveDownItem(row), a.moveTopItem(row), a.moveBottomItem(row),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem(i18n.T("menu.delete"), func() { a.deleteRow(row) }),
		)
		widget.ShowPopUpMenuAtRelativePosition(fyne.NewMenu("", items...), a.win.Canvas(), rel, anchor)
		return
	}
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(i18n.T("menu.change_value"), func() { a.changeValueDialog(row) }),
		fyne.NewMenuItem(i18n.T("menu.change_value_back"), func() { a.changeValueBack(row) }),
		fyne.NewMenuItem(i18n.T("menu.change_description"), func() { a.changeDescriptionDialog(row) }),
		a.changeTypeItem(row),
		fyne.NewMenuItem(i18n.T("menu.configure_bitfield"), func() { a.configureBitfieldDialog(row) }),
		fyne.NewMenuItem(i18n.T("menu.freeze"), func() { a.toggleFreezeRow(row) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.increase_value"), func() { a.adjustSelectedValues(true) }),
		fyne.NewMenuItem(i18n.T("menu.decrease_value"), func() { a.adjustSelectedValues(false) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.copy"), func() { a.copyTableSelection() }),
		fyne.NewMenuItem(i18n.T("menu.paste"), func() { a.pasteTable() }),
		fyne.NewMenuItem(i18n.T("menu.duplicate"), a.duplicateSelection),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.browse"), func() { a.browseRow(row) }),
		fyne.NewMenuItem(i18n.T("menu.disassemble"), func() { a.disassembleRow(row) }),
		fyne.NewMenuItem(i18n.T("menu.find_writes"), func() { a.findWhatWrites(row, true) }),
		fyne.NewMenuItem(i18n.T("menu.find_accesses"), func() { a.findWhatWrites(row, false) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.show_decimal"), func() { a.setDisplay(row, displayDefault) }),
		fyne.NewMenuItem(i18n.T("menu.show_hex"), func() { a.setDisplay(row, displayHex) }),
		fyne.NewMenuItem(i18n.T("menu.show_binary"), func() { a.setDisplay(row, displayBinary) }),
		fyne.NewMenuItem(i18n.T("menu.show_signed"), func() { a.setUnsigned(row, false) }),
		fyne.NewMenuItem(i18n.T("menu.show_unsigned"), func() { a.setUnsigned(row, true) }),
		fyne.NewMenuItem(i18n.T("menu.assign_hotkey"), func() { a.assignHotkey(row) }),
		fyne.NewMenuItemSeparator(),
		a.moveUpItem(row), a.moveDownItem(row), a.moveTopItem(row), a.moveBottomItem(row),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.delete"), func() { a.deleteRow(row) }),
	)
	widget.ShowPopUpMenuAtRelativePosition(menu, a.win.Canvas(), rel, anchor)
}

// changeTypeItem builds the "Change record type" submenu.
func (a *App) changeTypeItem(row int) *fyne.MenuItem {
	item := fyne.NewMenuItem(i18n.T("menu.change_type"), nil)
	var children []*fyne.MenuItem
	for _, t := range scan.Types() {
		if t.ID == scan.TypeAll || t.ID == scan.TypeGrouped {
			continue
		}
		id := t.ID
		children = append(children, fyne.NewMenuItem(t.Label, func() { a.changeEntryType(row, id) }))
	}
	item.ChildMenu = fyne.NewMenu("", children...)
	return item
}

func (a *App) moveUpItem(row int) *fyne.MenuItem {
	return fyne.NewMenuItem(i18n.T("menu.move_up"), func() { a.moveEntry(row, true) })
}

func (a *App) moveDownItem(row int) *fyne.MenuItem {
	return fyne.NewMenuItem(i18n.T("menu.move_down"), func() { a.moveEntry(row, false) })
}

func (a *App) moveTopItem(row int) *fyne.MenuItem {
	return fyne.NewMenuItem(i18n.T("menu.move_top"), func() { a.moveEntryEdge(row, true) })
}

func (a *App) moveBottomItem(row int) *fyne.MenuItem {
	return fyne.NewMenuItem(i18n.T("menu.move_bottom"), func() { a.moveEntryEdge(row, false) })
}

// setUnsigned switches an entry between signed and unsigned integer display.
func (a *App) setUnsigned(row int, u bool) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		return
	}
	a.entries[row].unsigned = u
	a.table.Refresh()
}

// changeEntryType reinterprets an entry's bytes as another type, re-reading the
// value at the same address and dropping an incompatible bitfield.
func (a *App) changeEntryType(row int, t scan.ValueType) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		return
	}
	e := a.entries[row]
	e.typ = t
	e.bit = nil
	if a.proc != nil && e.addr != 0 {
		w := t.Size()
		if w == 0 {
			w = len(e.value.Raw)
		}
		if w > 0 {
			if raw, err := a.proc.Read(e.addr, w); err == nil {
				e.value = scan.NewValue(t, raw)
				e.orig = e.value
				if e.frozen {
					e.frozenValue = e.value
					a.syncFreezeTargets()
				}
			}
		}
	}
	a.table.Refresh()
}

// adjustSelectedValues prompts once and adds or subtracts a delta from every
// selected numeric leaf.
func (a *App) adjustSelectedValues(increase bool) {
	sel := a.selectedTableEntries()
	if len(sel) == 0 {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	a.promptValue("1", func(input string) {
		changed := 0
		for _, e := range sel {
			if a.adjustEntryValue(e, input, increase) {
				changed++
			}
		}
		a.syncFreezeTargets()
		a.table.Refresh()
		a.setStatusText(i18n.Tf("status.values_set", map[string]any{"Count": changed}))
	})
}

// adjustEntryValue applies a delta to one numeric entry and reports success.
func (a *App) adjustEntryValue(e *tableEntry, deltaText string, increase bool) bool {
	if e.group || e.bit != nil || e.addr == 0 || e.expr != "" {
		return false
	}
	d := scan.TypeByID(e.typ)
	if d == nil || (d.Kind != scan.KindInt && d.Kind != scan.KindFloat) {
		return false
	}
	delta, err := scan.ParseValue(e.typ, deltaText)
	if err != nil {
		return false
	}
	var v scan.Value
	if d.Kind == scan.KindFloat {
		n := e.value.Float64()
		if increase {
			n += delta.Float64()
		} else {
			n -= delta.Float64()
		}
		v, err = scan.ParseValue(e.typ, strconv.FormatFloat(n, 'g', -1, 64))
		if err != nil {
			return false
		}
	} else {
		n := e.value.Int64()
		if increase {
			n += delta.Int64()
		} else {
			n -= delta.Int64()
		}
		v = scan.NewValue(e.typ, scan.EncodeValue(e.typ, n))
	}
	if err := a.writeValue(e.addr, v); err != nil {
		return false
	}
	e.hasUndo = true
	e.undoValue = e.value
	e.value = v
	if e.frozen {
		e.frozenValue = v
	}
	return true
}

// copyTableSelection copies the selected addresses as newline-separated hex.
func (a *App) copyTableSelection() {
	sel := a.selectedTableEntries()
	lines := make([]string, 0, len(sel))
	for _, e := range sel {
		if !e.group && e.addr != 0 {
			lines = append(lines, fmt.Sprintf("0x%x", e.addr))
		}
	}
	if len(lines) == 0 {
		return
	}
	a.fapp.Clipboard().SetContent(strings.Join(lines, "\n"))
}

// pasteTable creates a record for every address on the clipboard, accepting
// "0x1234" or "0x1234 - description" lines.
func (a *App) pasteTable() {
	text := a.fapp.Clipboard().Content()
	if strings.TrimSpace(text) == "" {
		return
	}
	typ := a.defaultValueType()
	added := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		addrText, desc := line, ""
		if i := strings.Index(line, " - "); i >= 0 {
			addrText, desc = strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+3:])
		}
		addr, err := parseAddress(addrText)
		if err != nil {
			continue
		}
		e := &tableEntry{addr: addr, typ: typ, desc: desc}
		if a.proc != nil && typ.Size() > 0 {
			if raw, rerr := a.proc.Read(addr, typ.Size()); rerr == nil {
				e.value = scan.NewValue(typ, raw)
				e.orig = e.value
			}
		}
		a.addRoot(e)
		added++
	}
	if added == 0 {
		return
	}
	a.table.Refresh()
	a.updateScanControls()
	a.setStatusText(i18n.Tf("status.records_added", map[string]any{"Count": added}))
}

// duplicateSelection appends a deep copy of every selected entry as a root.
func (a *App) duplicateSelection() {
	sel := a.selectedTableEntries()
	if len(sel) == 0 {
		return
	}
	for _, e := range sel {
		a.addRoot(cloneEntry(e))
	}
	a.table.Refresh()
	a.updateScanControls()
}

// cloneEntry deep-copies an entry and its subtree.
func cloneEntry(e *tableEntry) *tableEntry {
	c := *e
	c.parent = nil
	c.exprOffsets = append([]string(nil), e.exprOffsets...)
	if e.pointer != nil {
		p := *e.pointer
		p.offsets = append([]int64(nil), e.pointer.offsets...)
		c.pointer = &p
	}
	if e.bit != nil {
		b := *e.bit
		c.bit = &b
	}
	c.children = nil
	for _, ch := range e.children {
		c.children = append(c.children, cloneEntry(ch))
	}
	return &c
}

// newGroup appends an empty group header.
func (a *App) newGroup() {
	g := &tableEntry{group: true, expanded: true, desc: i18n.T("group.default")}
	a.addRoot(g)
	a.tableSel = a.rowOf(g)
	a.table.Refresh()
	a.updateScanControls()
	a.changeDescriptionDialog(a.tableSel)
}

// groupSelection wraps the selected top-level entries under a new group.
func (a *App) groupSelection() {
	roots := a.selectedRoots()
	if len(roots) == 0 {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	mark := map[*tableEntry]bool{}
	for _, r := range roots {
		mark[r] = true
	}
	insert := -1
	for i, r := range a.entryRoots {
		if mark[r] {
			insert = i
			break
		}
	}
	kept := a.entryRoots[:0]
	for _, r := range a.entryRoots {
		if !mark[r] {
			kept = append(kept, r)
		}
	}
	a.entryRoots = kept
	if insert < 0 || insert > len(a.entryRoots) {
		insert = len(a.entryRoots)
	}
	g := &tableEntry{group: true, expanded: true, desc: i18n.T("group.default"), children: roots}
	for _, r := range roots {
		r.parent = g
	}
	a.entryRoots = append(a.entryRoots, nil)
	copy(a.entryRoots[insert+1:], a.entryRoots[insert:])
	a.entryRoots[insert] = g
	a.rebuildVisible()
	a.tableSel = a.rowOf(g)
	a.table.Refresh()
	a.updateScanControls()
}

// selectedRoots returns the top-level ancestors of the selection in tree order.
func (a *App) selectedRoots() []*tableEntry {
	mark := map[*tableEntry]bool{}
	for _, e := range a.selectedTableEntries() {
		for e.parent != nil {
			e = e.parent
		}
		mark[e] = true
	}
	var out []*tableEntry
	for _, r := range a.entryRoots {
		if mark[r] {
			out = append(out, r)
		}
	}
	return out
}

// siblingsOf returns the slice an entry lives in.
func (a *App) siblingsOf(e *tableEntry) []*tableEntry {
	if e.parent == nil {
		return a.entryRoots
	}
	return e.parent.children
}

func indexOfEntry(sib []*tableEntry, e *tableEntry) int {
	for i, x := range sib {
		if x == e {
			return i
		}
	}
	return -1
}

// moveEntry swaps an entry with its previous or next sibling.
func (a *App) moveEntry(row int, up bool) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	e := a.entries[row]
	sib := a.siblingsOf(e)
	i := indexOfEntry(sib, e)
	j := i + 1
	if up {
		j = i - 1
	}
	if i < 0 || j < 0 || j >= len(sib) {
		return
	}
	sib[i], sib[j] = sib[j], sib[i]
	a.finishMove(e)
}

// moveEntryEdge moves an entry to the start or end of its sibling list.
func (a *App) moveEntryEdge(row int, top bool) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	e := a.entries[row]
	sib := a.siblingsOf(e)
	i := indexOfEntry(sib, e)
	if i < 0 {
		return
	}
	j := len(sib) - 1
	if top {
		j = 0
	}
	if i == j {
		return
	}
	if j > i {
		copy(sib[i:j], sib[i+1:j+1])
	} else {
		copy(sib[j+1:i+1], sib[j:i])
	}
	sib[j] = e
	a.finishMove(e)
}

func (a *App) finishMove(e *tableEntry) {
	a.rebuildVisible()
	a.tableSel = a.rowOf(e)
	a.table.Refresh()
}

func (a *App) toggleFreezeRow(row int) {
	if a.tableMulti != nil && len(a.tableMulti) > 0 {
		for _, e := range a.selectedTableEntries() {
			a.toggleFreezeEntry(e)
		}
		a.table.Refresh()
		return
	}
	if row < 0 || row >= len(a.entries) {
		return
	}
	a.toggleFreezeEntry(a.entries[row])
	a.table.Refresh()
}

// toggleFreezeEntry freezes or unfreezes a record; a group applies to its whole
// subtree.
func (a *App) toggleFreezeEntry(e *tableEntry) {
	if e.group {
		on := !e.frozen
		e.frozen = on
		a.freezeSubtree(e.children, on)
		a.syncFreezeTargets()
		return
	}
	if e.expr != "" {
		return
	}
	if e.frozen {
		e.frozen = false
		e.frozenValue = scan.Value{}
		a.syncFreezeTargets()
		return
	}
	v := e.value
	if a.proc != nil {
		w := e.typ.Size()
		if w == 0 {
			w = len(e.value.Raw)
		}
		if w > 0 {
			if raw, err := a.proc.Read(e.addr, w); err == nil {
				v = scan.NewValue(e.typ, raw)
			}
		}
	}
	if err := a.writeValue(e.addr, v); err != nil {
		a.fail(err)
		return
	}
	e.value = v
	e.frozenValue = v
	e.frozen = true
	a.syncFreezeTargets()
}

func (a *App) freezeSubtree(nodes []*tableEntry, on bool) {
	for _, c := range nodes {
		if on {
			c.frozen = false
			a.toggleFreezeEntry(c)
			continue
		}
		c.frozen = false
		c.frozenValue = scan.Value{}
	}
}

func (a *App) deleteRow(row int) {
	if a.tableMulti != nil && len(a.tableMulti) > 0 {
		for _, e := range a.selectedTableEntries() {
			a.removeEntry(e)
		}
		a.tableMulti = nil
		a.tableSel = -1
		a.syncFreezeTargets()
		a.table.Refresh()
		a.updateScanControls()
		return
	}
	if row < 0 || row >= len(a.entries) {
		return
	}
	a.removeEntry(a.entries[row])
	a.tableSel = -1
	a.syncFreezeTargets()
	a.table.Refresh()
	a.updateScanControls()
}

func (a *App) setDisplay(row int, d displayFormat) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	e := a.entries[row]
	if e.group {
		return
	}
	e.display = d
	a.table.Refresh()
}

// refreshEntries re-reads every entry's value from the target process and
// recomputes pointer-entry addresses. It must run on the UI goroutine because
// it touches the entry slice.
func (a *App) refreshEntries() bool {
	if a.proc == nil {
		return false
	}
	regions, _ := mem.Regions(a.proc.PID)
	res := symbolResolver{symbols: a.symbols, regions: regions}
	changed := false
	a.walkEntries(func(e *tableEntry) {
		if e.expr != "" || e.pointer != nil {
			a.resolveEntryAddr(e, res, &changed)
		}
		if e.group || e.addr == 0 {
			return
		}
		w := e.typ.Size()
		if w == 0 {
			w = len(e.value.Raw)
		}
		if w <= 0 {
			return
		}
		raw, err := a.proc.Read(e.addr, w)
		if err != nil {
			return
		}
		v := scan.NewValue(e.typ, raw)
		if v.Type != e.value.Type || string(v.Raw) != string(e.value.Raw) {
			e.value = v
			changed = true
		}
	})
	return changed
}

// resolveEntryAddr recomputes an entry's address from its pointer chain or its
// stored address expression.
func (a *App) resolveEntryAddr(e *tableEntry, r symbolResolver, changed *bool) {
	if e.expr != "" {
		if addr, err := a.resolveExpression(e, r); err == nil && addr != e.addr {
			e.addr = addr
			*changed = true
		}
		return
	}
	if e.pointer == nil {
		return
	}
	if addr, err := resolvePointer(a.proc, e.pointer); err == nil && addr != e.addr {
		e.addr = addr
		*changed = true
	}
}

func (a *App) assignHotkey(row int) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	entry := widget.NewEntry()
	entry.SetPlaceHolder(i18n.T("dialog.assign_hotkey.placeholder"))
	d := dialog.NewForm(i18n.T("dialog.assign_hotkey.title"), i18n.T("action.assign"), i18n.T("action.cancel"),
		[]*widget.FormItem{widget.NewFormItem(i18n.T("dialog.assign_hotkey.key"), entry)},
		func(ok bool) {
			if !ok {
				return
			}
			key, err := parseHotkey(entry.Text)
			if err != nil {
				a.fail(err)
				return
			}
			a.entries[row].hotkey = key
			a.bindHotkey(key, a.entries[row].addr)
			a.setStatusText(i18n.Tf("status.hotkey_assigned", map[string]any{"Key": key}))
		}, a.win)
	d.Resize(fyne.NewSize(340, 160))
	d.Show()
}

// bindHotkey registers a shortcut that freezes/unfreezes the entry at addr.
// Re-binding an address replaces the previous shortcut, and the handler looks
// the address up at fire time so it survives row reordering.
func (a *App) bindHotkey(key fyne.KeyName, addr uint64) {
	if a.hotkeyShortcuts == nil {
		a.hotkeyShortcuts = map[uint64]fyne.Shortcut{}
	}
	a.unbindHotkey(addr)
	sc := &desktop.CustomShortcut{KeyName: key}
	if !strings.HasPrefix(string(key), "F") {
		sc.Modifier = fyne.KeyModifierControl | fyne.KeyModifierAlt
	}
	a.win.Canvas().AddShortcut(sc, func(fyne.Shortcut) { a.toggleFreezeAddr(addr) })
	a.hotkeyShortcuts[addr] = sc
}

// unbindHotkey removes the shortcut registered for addr, if any.
func (a *App) unbindHotkey(addr uint64) {
	sc, ok := a.hotkeyShortcuts[addr]
	if !ok {
		return
	}
	a.win.Canvas().RemoveShortcut(sc)
	delete(a.hotkeyShortcuts, addr)
}

// toggleFreezeAddr toggles the frozen state of the entry at addr.
func (a *App) toggleFreezeAddr(addr uint64) {
	if e := a.findEntry(addr); e != nil {
		a.toggleFreezeEntry(e)
		a.table.Refresh()
	}
}

// findEntry returns the first tree node at addr, or nil.
func (a *App) findEntry(addr uint64) *tableEntry {
	var found *tableEntry
	a.walkEntries(func(e *tableEntry) {
		if found == nil && e.addr == addr && !e.group {
			found = e
		}
	})
	return found
}

func displayName(d displayFormat) string {
	switch d {
	case displayHex:
		return "hex"
	case displayBinary:
		return "binary"
	default:
		return "decimal"
	}
}

func parseDisplay(s string) displayFormat {
	switch s {
	case "hex":
		return displayHex
	case "binary":
		return displayBinary
	default:
		return displayDefault
	}
}

func parseHotkey(s string) (fyne.KeyName, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) == 1 && s[0] >= 'A' && s[0] <= 'Z' {
		return fyne.KeyName(s), nil
	}
	if strings.HasPrefix(s, "F") {
		if n, err := strconv.Atoi(s[1:]); err == nil && n >= 1 && n <= 12 {
			return fyne.KeyName(s), nil
		}
	}
	return "", fmt.Errorf("%s", i18n.Tf("error.unsupported_hotkey", map[string]any{"Key": s}))
}

func parseOffsets(base uint64, s string) (*pointerChain, error) {
	var offsets []int64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		digits := strings.TrimPrefix(strings.TrimPrefix(part, "0x"), "0X")
		n, err := strconv.ParseInt(digits, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("%s", i18n.Tf("error.invalid_pointer_offset", map[string]any{"Offset": part}))
		}
		offsets = append(offsets, n)
	}
	if len(offsets) == 0 {
		return nil, nil
	}
	return &pointerChain{base: base, offsets: offsets}, nil
}

func (a *scanTab) addResultToTable(i int) {
	sel := a.selectedFoundIndices()
	if len(sel) > 1 {
		typ := a.foundValueType()
		for _, idx := range sel {
			r := a.results[idx]
			a.entryRoots = append(a.entryRoots, &tableEntry{addr: r.Addr, typ: typ, value: r.Value, orig: r.Value})
		}
		a.rebuildVisible()
		a.table.Refresh()
		a.setStatusText(i18n.Tf("status.added_to_table", map[string]any{"Addr": fmt.Sprintf("%d results", len(sel))}))
		a.updateScanControls()
		return
	}
	if i < 0 || i >= len(a.results) {
		a.setStatusText(i18n.T("status.select_found"))
		return
	}
	r := a.results[i]
	typ := a.defaultValueType()
	if a.session != nil {
		typ = a.session.Options().Type
	}
	a.entryRoots = append(a.entryRoots, &tableEntry{addr: r.Addr, typ: typ, value: r.Value, orig: r.Value})
	a.rebuildVisible()
	a.table.Refresh()
	a.setStatusText(i18n.Tf("status.added_to_table", map[string]any{"Addr": fmt.Sprintf("%x", r.Addr)}))
	a.updateScanControls()
}

func (a *App) browseRow(row int) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	a.openMemoryViewer()
	a.loadMemory(a.entries[row].addr)
}

func (a *App) disassembleRow(row int) {
	a.browseRow(row)
}

// changeValueSelected routes Ctrl+E to the selection in the active panel.
func (a *App) changeValueSelected() {
	if a.activePanel == panelFound {
		if t := a.tab(); t != nil {
			t.changeFoundValue(t.foundSel)
		}
		return
	}
	a.changeSelectedTableValues()
}

// changeSelectedTableValues edits one row or, when a multi-selection exists,
// applies a single input to every selected leaf.
func (a *App) changeSelectedTableValues() {
	sel := a.selectedTableEntries()
	if len(sel) == 0 {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	if len(sel) == 1 {
		a.changeValueDialog(a.rowOf(sel[0]))
		return
	}
	a.promptValue("", func(input string) {
		written := 0
		for _, e := range sel {
			if e.group || e.expr != "" || e.addr == 0 || e.bit != nil {
				continue
			}
			v, err := scan.ParseValue(e.typ, a.expandValueInput(input, e))
			if err != nil {
				continue
			}
			if err := a.writeValue(e.addr, v); err != nil {
				continue
			}
			e.hasUndo = true
			e.undoValue = e.value
			e.value = v
			if e.frozen {
				e.frozenValue = v
			}
			written++
		}
		a.syncFreezeTargets()
		a.table.Refresh()
		a.setStatusText(i18n.Tf("status.values_set", map[string]any{"Count": written}))
	})
}

// promptValue shows the reference tool's single-field Change value dialog.
func (a *App) promptValue(current string, onOK func(string)) {
	entry := widget.NewEntry()
	entry.SetText(current)
	d := dialog.NewForm(i18n.T("dialog.change_value.title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		[]*widget.FormItem{widget.NewFormItem(i18n.T("field.value"), entry)},
		func(ok bool) {
			if !ok {
				return
			}
			onOK(entry.Text)
		}, a.win)
	d.Resize(fyne.NewSize(360, 180))
	d.Show()
}

// changeFoundValue edits a Found scan-result value in place, mirroring Cheat
// Engine's Ctrl+E on the results list. It writes memory once and updates the
// row; it does not touch a cheat-table entry for the same address.
func (a *scanTab) changeFoundValue(i int) {
	sel := a.selectedFoundIndices()
	if len(sel) > 1 {
		a.changeFoundValues(sel)
		return
	}
	if i < 0 || i >= len(a.results) {
		a.setStatusText(i18n.T("status.select_found"))
		return
	}
	typ := a.foundValueType()
	text := a.results[i].Value.String()
	if a.hexBox != nil && a.hexBox.Checked {
		text = hexOf(a.results[i].Value)
	}
	a.promptValue(text, func(input string) {
		if i < 0 || i >= len(a.results) {
			return
		}
		v, err := scan.ParseValue(typ, a.expandFoundInput(input, i))
		if err != nil {
			a.fail(err)
			return
		}
		if err := a.writeValue(a.results[i].Addr, v); err != nil {
			a.fail(err)
			return
		}
		a.results[i].Value = v
		if a.foundLive != nil {
			a.foundLive[i] = v
		}
		if a.foundList != nil {
			a.foundList.Refresh()
		}
	})
}

// changeFoundValues applies one input to every selected Found result.
func (a *scanTab) changeFoundValues(sel []int) {
	typ := a.foundValueType()
	a.promptValue("", func(input string) {
		written := 0
		for _, i := range sel {
			if i < 0 || i >= len(a.results) {
				continue
			}
			v, err := scan.ParseValue(typ, a.expandFoundInput(input, i))
			if err != nil {
				continue
			}
			if err := a.writeValue(a.results[i].Addr, v); err != nil {
				continue
			}
			a.results[i].Value = v
			if a.foundLive != nil {
				a.foundLive[i] = v
			}
			written++
		}
		if a.foundList != nil {
			a.foundList.Refresh()
		}
		a.setStatusText(i18n.Tf("status.values_set", map[string]any{"Count": written}))
	})
}

// foundValueType is the type used to parse a Found-list edit: the type
// selected in the scan panel, falling back to the session's scan type and then
// the configured default.
func (a *scanTab) foundValueType() scan.ValueType {
	if a.valueType != nil {
		if t, ok := scan.LookupType(a.valueType.Selected); ok {
			return t.ID
		}
		if a.session != nil {
			return a.session.Options().Type
		}
	}
	return a.defaultValueType()
}

// expandValueInput rewrites a change-value input for a cheat
// table row: it substitutes (description) references and the value/oldvalue
// identifiers, and turns a bare hex entry into 0x form when the row is shown as
// hexadecimal.
func (a *App) expandValueInput(input string, cur *tableEntry) string {
	input = a.substituteDescriptions(input)
	if cur == nil {
		return input
	}
	if d := scan.TypeByID(cur.typ); d != nil && (d.Kind == scan.KindInt || d.Kind == scan.KindFloat) {
		curText := cur.value.String()
		input = replaceValueIdent(input, "oldvalue", curText)
		input = replaceValueIdent(input, "value", curText)
	}
	if cur.display == displayHex && cur.bit == nil {
		input = asHexLiteral(input)
	}
	return input
}

// expandFoundInput is the Found-list equivalent; its hex handling follows the
// scan panel's Hex toggle.
func (a *scanTab) expandFoundInput(input string, i int) string {
	input = a.substituteDescriptions(input)
	if d := scan.TypeByID(a.foundValueType()); d != nil && (d.Kind == scan.KindInt || d.Kind == scan.KindFloat) {
		curText := a.results[i].Value.String()
		input = replaceValueIdent(input, "oldvalue", curText)
		input = replaceValueIdent(input, "value", curText)
	}
	return a.hexValue(input)
}

// substituteDescriptions replaces (description) with the referenced record's
// displayed value, leaving unmatched parentheses for the expression parser.
func (a *App) substituteDescriptions(input string) string {
	open := strings.IndexByte(input, '(')
	if open < 0 {
		return input
	}
	rel := strings.IndexByte(input[open:], ')')
	if rel < 0 {
		return input
	}
	close := open + rel
	if ref := a.findEntryByDesc(input[open+1 : close]); ref != nil {
		return input[:open] + a.formatEntryValue(ref) + a.substituteDescriptions(input[close+1:])
	}
	return input[:close+1] + a.substituteDescriptions(input[close+1:])
}

// findEntryByDesc returns the first non-group entry with the given description.
func (a *App) findEntryByDesc(desc string) *tableEntry {
	var found *tableEntry
	a.walkEntries(func(e *tableEntry) {
		if found == nil && !e.group && e.desc == desc {
			found = e
		}
	})
	return found
}

// asHexLiteral prefixes a bare hex string with 0x so it parses as hex.
func asHexLiteral(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") {
		return s
	}
	if isBareHexLiteral(t) {
		return "0x" + t
	}
	return s
}

// replaceValueIdent replaces whole-word identifiers (not substrings).
func replaceValueIdent(s, name, repl string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if valueIdentStart(s[i]) {
			j := i
			for j < len(s) && valueIdentByte(s[j]) {
				j++
			}
			if s[i:j] == name {
				b.WriteString(repl)
			} else {
				b.WriteString(s[i:j])
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func valueIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func valueIdentByte(c byte) bool {
	return valueIdentStart(c) || (c >= '0' && c <= '9')
}

// setChildrenValueDialog prompts once and writes the value to every leaf under
// a group, matching the reference tool's recursive set option.
func (a *App) setChildrenValueDialog(row int) {
	if row < 0 || row >= len(a.entries) || !a.entries[row].group {
		return
	}
	root := a.entries[row]
	a.promptValue("", func(input string) {
		written, _ := a.setSubtreeValue(root, input)
		if written > 0 && a.table != nil {
			a.table.Refresh()
		}
		a.setStatusText(i18n.Tf("status.children_set", map[string]any{"Count": written}))
	})
}

// setSubtreeValue writes input to every resolvable leaf below e, parsing it
// with each leaf's own type.
func (a *App) setSubtreeValue(e *tableEntry, input string) (written, failed int) {
	for _, c := range e.children {
		if c.group {
			w, f := a.setSubtreeValue(c, input)
			written += w
			failed += f
			continue
		}
		if c.expr != "" || c.addr == 0 {
			failed++
			continue
		}
		v, err := scan.ParseValue(c.typ, input)
		if err != nil {
			failed++
			continue
		}
		if err := a.writeValue(c.addr, v); err != nil {
			failed++
			continue
		}
		c.hasUndo = true
		c.undoValue = c.value
		c.value = v
		if c.frozen {
			c.frozenValue = v
		}
		written++
	}
	if written > 0 {
		a.syncFreezeTargets()
	}
	return written, failed
}

func (a *App) changeValueDialog(row int) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	cur := a.entries[row]
	text := a.formatEntryValue(cur)
	if cur.bit != nil {
		text = cur.bit.format(cur.value.Raw)
	}
	a.promptValue(text, func(input string) {
		if row >= len(a.entries) {
			return
		}
		e := a.entries[row]
		if e.bit != nil {
			field, err := e.bit.parse(input)
			if err != nil {
				a.fail(err)
				return
			}
			updated, err := a.writeBitfield(e.addr, *e.bit, field)
			if err != nil {
				a.fail(err)
				return
			}
			a.setEntryValue(row, scan.NewValue(e.typ, updated), true)
			a.table.Refresh()
			return
		}
		v, err := scan.ParseValue(e.typ, a.expandValueInput(input, e))
		if err != nil {
			a.fail(err)
			return
		}
		if err := a.writeValue(e.addr, v); err != nil {
			a.fail(err)
			return
		}
		a.setEntryValue(row, v, false)
		a.table.Refresh()
	})
}

// setEntryValue records the pre-edit value for undo, stores v as the current
// value and moves the frozen target when the row is frozen. updateOrig keeps
// the historical bitfield behaviour of refreshing the change-back baseline.
func (a *App) setEntryValue(row int, v scan.Value, updateOrig bool) {
	e := a.entries[row]
	e.hasUndo = true
	e.undoValue = e.value
	e.value = v
	if updateOrig {
		e.orig = v
	}
	if e.frozen {
		e.frozenValue = v
		a.syncFreezeTargets()
	}
}

// undoValue swaps the current value with the last pre-edit value, so pressing
// Ctrl+Z again redoes the edit.
func (a *App) undoValue(row int) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		return
	}
	e := a.entries[row]
	if !e.hasUndo {
		return
	}
	prev := e.value
	if err := a.writeValue(e.addr, e.undoValue); err != nil {
		a.fail(err)
		return
	}
	e.value = e.undoValue
	e.undoValue = prev
	if e.frozen {
		e.frozenValue = e.value
		a.syncFreezeTargets()
	}
	a.table.Refresh()
}

// configureBitfieldDialog turns a row into a bitfield edit/display entry.
func (a *App) configureBitfieldDialog(row int) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	e := a.entries[row]
	size := e.typ.Size()
	if e.bit != nil {
		size = e.bit.size
	}
	offset := widget.NewEntry()
	width := widget.NewEntry()
	signed := widget.NewCheck(i18n.T("bitfield.signed"), nil)
	if e.bit != nil {
		offset.SetText(strconv.Itoa(e.bit.offset))
		width.SetText(strconv.Itoa(e.bit.width))
		signed.SetChecked(e.bit.signed)
	} else {
		offset.SetText("0")
		width.SetText(strconv.Itoa(size * 8))
	}
	d := dialog.NewForm(i18n.T("menu.configure_bitfield"), i18n.T("action.apply"), i18n.T("action.cancel"),
		[]*widget.FormItem{
			widget.NewFormItem(i18n.T("bitfield.offset"), offset),
			widget.NewFormItem(i18n.T("bitfield.width"), width),
			widget.NewFormItem(i18n.T("bitfield.signed"), signed),
		},
		func(ok bool) {
			if !ok {
				return
			}
			off, err1 := strconv.Atoi(strings.TrimSpace(offset.Text))
			w, err2 := strconv.Atoi(strings.TrimSpace(width.Text))
			if err1 != nil || err2 != nil || off < 0 || w < 1 || off+w > size*8 {
				a.fail(fmt.Errorf("%s", i18n.T("error.bitfield_range")))
				return
			}
			a.entries[row].bit = &bitSpec{size: size, offset: off, width: w, signed: signed.Checked}
			if a.proc != nil {
				if raw, rerr := a.proc.Read(e.addr, size); rerr == nil {
					a.entries[row].value = scan.NewValue(e.typ, raw)
					a.entries[row].orig = a.entries[row].value
					if a.entries[row].frozen {
						a.entries[row].frozenValue = a.entries[row].value
						a.syncFreezeTargets()
					}
				}
			}
			a.table.Refresh()
		}, a.win)
	d.Resize(fyne.NewSize(360, 240))
	d.Show()
}

func (a *App) changeValueBack(row int) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group {
		return
	}
	e := a.entries[row]
	if err := a.writeValue(e.addr, e.orig); err != nil {
		a.fail(err)
		return
	}
	e.value = e.orig
	if e.frozen {
		e.frozenValue = e.orig
		a.syncFreezeTargets()
	}
	a.table.Refresh()
}

func (a *App) changeDescriptionDialog(row int) {
	if row < 0 || row >= len(a.entries) {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	entry := widget.NewEntry()
	entry.SetText(a.entries[row].desc)
	d := dialog.NewForm(i18n.T("dialog.change_description.title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		[]*widget.FormItem{widget.NewFormItem(i18n.T("field.description"), entry)},
		func(ok bool) {
			if !ok {
				return
			}
			a.entries[row].desc = entry.Text
			a.table.Refresh()
		}, a.win)
	d.Resize(fyne.NewSize(360, 160))
	d.Show()
}

func (a *App) writeValue(addr uint64, v scan.Value) error {
	if a.proc == nil {
		return fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	if err := a.proc.Write(addr, v.Raw); err != nil {
		return err
	}
	return nil
}

func (a *App) addAddressDialog() {
	addr := widget.NewEntry()
	addr.SetPlaceHolder(i18n.T("placeholder.address"))
	typ := widget.NewSelect(valueTypeOptions(), nil)
	typ.SetSelected(ceValueTypeLabel(a.defaultValueType()))
	desc := widget.NewEntry()
	val := widget.NewEntry()
	val.SetPlaceHolder(i18n.T("placeholder.optional"))
	offs := widget.NewEntry()
	offs.SetPlaceHolder(i18n.T("placeholder.pointer_offsets"))
	d := dialog.NewForm(i18n.T("dialog.add_address.title"), i18n.T("action.add"), i18n.T("action.cancel"),
		[]*widget.FormItem{
			widget.NewFormItem(i18n.T("field.address"), addr),
			widget.NewFormItem(i18n.T("field.pointer_offsets"), offs),
			widget.NewFormItem(i18n.T("field.type"), typ),
			widget.NewFormItem(i18n.T("field.description"), desc),
			widget.NewFormItem(i18n.T("field.value"), val),
		},
		func(ok bool) {
			if !ok {
				return
			}
			target, err := parseAddress(addr.Text)
			if err != nil {
				a.fail(err)
				return
			}
			t := parseCEValueType(typ.Selected)
			var v scan.Value
			if strings.TrimSpace(val.Text) != "" {
				v, err = scan.ParseValue(t, val.Text)
				if err != nil {
					a.fail(err)
					return
				}
			} else if a.proc != nil && t.Size() > 0 {
				if raw, rerr := a.proc.Read(target, t.Size()); rerr == nil {
					v = scan.NewValue(t, raw)
				}
			}
			var pc *pointerChain
			if strings.TrimSpace(offs.Text) != "" {
				pc, err = parseOffsets(target, offs.Text)
				if err != nil {
					a.fail(err)
					return
				}
			}
			a.addRoot(&tableEntry{addr: target, typ: t, desc: desc.Text, value: v, orig: v, pointer: pc})
			a.table.Refresh()
			a.updateScanControls()
		}, a.win)
	d.Resize(fyne.NewSize(440, 420))
	d.Show()
}

func (a *App) clearTable() {
	for _, r := range a.entryRoots {
		a.removeSubtreeHooks(r)
	}
	a.resetTree()
	a.syncFreezeTargets()
	a.table.Refresh()
	a.updateScanControls()
}

// newTable starts a fresh cheat table after confirming if it is not empty.
func (a *App) newTable() {
	if len(a.entryRoots) == 0 {
		a.resetScanState()
		return
	}
	dialog.ShowConfirm(i18n.T("dialog.new_table.title"), i18n.T("dialog.new_table.body"), func(ok bool) {
		if ok {
			a.resetScanState()
		}
	}, a.win)
}

// resetScanState clears the cheat table, every scan session and the symbols.
func (a *App) resetScanState() {
	a.clearTable()
	a.symbols = nil
	a.dbgBreakpoints = map[uint64]*dbgBreakpoint{}
	a.refreshBreakpointList()
	a.resetTabs()
	a.setStatusText(i18n.T("status.new_table"))
}

func parseAddress(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, fmt.Errorf("%s", i18n.T("error.empty_address"))
	}
	return strconv.ParseUint(s, 16, 64)
}

// dataCell is a table cell that paints its own text colour and reports taps and
// right-clicks.
type dataCell struct {
	widget.BaseWidget
	app      *App
	text     *canvas.Text
	row, col int
}

func (c *dataCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.text)
}

func (c *dataCell) setText(s string) {
	c.text.Text = s
	c.text.Refresh()
}

func (c *dataCell) setColor(col color.Color) {
	c.text.Color = col
	c.text.Refresh()
}

func (c *dataCell) setMono(mono bool) {
	c.text.TextStyle = fyne.TextStyle{Monospace: mono}
	c.text.Refresh()
}

func (c *dataCell) Tapped(*fyne.PointEvent) {
	c.app.tableTapped(c.row, c.col)
}

func (c *dataCell) MouseDown(e *desktop.MouseEvent) {
	if e.Button == desktop.MouseButtonSecondary {
		c.app.tableMenu(c.row, c.col, e.Position, c)
		return
	}
	c.app.clickMod = e.Modifier
	c.app.selectTableRow(c.row, e.Modifier)
}

// entryColor returns the row's row colour, or the default text colour.
func (a *App) entryColor(e *tableEntry) color.Color {
	if c := cheatColor(e.color); c != nil {
		return c
	}
	return a.pal().text
}

// cheatColor parses a row colour (6 hex digits, TColor $00BBGGRR) into
// an RGBA colour, returning nil for an empty or invalid value.
func cheatColor(s string) color.Color {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil
	}
	return color.NRGBA{
		R: uint8(n & 0xff),
		G: uint8((n >> 8) & 0xff),
		B: uint8((n >> 16) & 0xff),
		A: 0xff,
	}
}
