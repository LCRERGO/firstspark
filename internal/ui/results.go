//go:build gui

package ui

import (
	"fmt"
	"strconv"
	"strings"

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

func (a *App) buildFoundList() {
	a.foundList = widget.NewTable(
		func() (int, int) { return len(a.results), 1 },
		func() fyne.CanvasObject {
			c := &foundCell{Label: widget.NewLabel(""), app: a}
			c.TextStyle = fyne.TextStyle{Monospace: true}
			return c
		},
		func(id widget.TableCellID, o fyne.CanvasObject) { a.updateFoundCell(id, o) },
	)
	a.foundList.SetColumnWidth(0, 320)
}

func (a *App) updateFoundCell(id widget.TableCellID, o fyne.CanvasObject) {
	c := o.(*foundCell)
	c.row = id.Row
	if id.Row < 0 || id.Row >= len(a.results) {
		c.SetText("")
		c.Refresh()
		return
	}
	r := a.results[id.Row]
	c.SetText(fmt.Sprintf("0x%012x  %s", r.Addr, r.Value.String()))
	if a.isFoundSelected(id.Row) {
		c.Importance = widget.HighImportance
	} else {
		c.Importance = widget.MediumImportance
	}
	c.Refresh()
}

// foundCell is a tappable Found-list row that reports clicks with modifiers.
type foundCell struct {
	*widget.Label
	app *App
	row int
}

func (c *foundCell) Tapped(*fyne.PointEvent) { c.app.foundTapped(c.row) }

func (c *foundCell) MouseDown(e *desktop.MouseEvent) {
	c.app.clickMod = e.Modifier
	c.app.selectFoundRow(c.row, e.Modifier)
}

// selectFoundRow updates the Found-list selection: plain selects one row, Ctrl
// toggles and Shift selects a contiguous range.
func (a *App) selectFoundRow(id int, mod fyne.KeyModifier) {
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
func (a *App) isFoundSelected(id int) bool {
	if a.foundMulti != nil && a.foundMulti[id] {
		return true
	}
	return id == a.foundSel
}

// selectedFoundIndices returns the selected result indices in order.
func (a *App) selectedFoundIndices() []int {
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
func (a *App) foundTapped(id int) {
	mod := a.clickMod
	a.clickMod = 0
	if mod != 0 {
		return
	}
	a.selectFound(id)
}

func (a *App) foundPanel() fyne.CanvasObject {
	head := container.NewHBox(
		a.th.heading(i18n.T("results.found"), a.th.size+2, a.pal().primary),
		layout.NewSpacer(),
		newHintButton(i18n.T("results.add_to_table"), "results.hint.add_to_table", func() { a.addResultToTable(a.foundSel) }),
	)
	return container.NewBorder(head, nil, nil, nil, a.foundList)
}

func (a *App) selectFound(id int) {
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

func (a *App) setResults(r []scan.Result) {
	if limit := a.cfg.UI.ResultLimit; limit > 0 && len(r) > limit {
		r = r[:limit]
	}
	a.results = r
	a.foundSel = -1
	a.foundMulti = nil
	a.foundCount.SetText(i18n.Tf("app.found_count", map[string]any{"Count": len(a.results)}))
	if a.foundList != nil {
		a.foundList.Refresh()
	}
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
	head := container.NewBorder(
		nil, nil,
		a.th.heading(i18n.T("results.cheat_table"), a.th.size+2, a.pal().primary),
		container.NewHBox(
			newHintButton(i18n.T("results.add_address_manually"), "results.hint.add_address", a.addAddressDialog),
			newHintButton(i18n.T("results.clear_list"), "results.hint.clear", a.clearTable),
		),
	)
	return container.NewBorder(head, nil, nil, nil, a.table)
}

func (a *App) newDataCell() *dataCell {
	c := &dataCell{Label: widget.NewLabel(""), app: a}
	c.TextStyle = fyne.TextStyle{Monospace: true}
	return c
}

func (a *App) updateDataCell(id widget.TableCellID, o fyne.CanvasObject) {
	c := o.(*dataCell)
	c.row, c.col = id.Row, id.Col
	if id.Row < 0 || id.Row >= len(a.entries) {
		c.SetText("")
		c.Refresh()
		return
	}
	c.SetText(a.cellText(id))
	if a.isTableSelected(a.entries[id.Row]) {
		c.Importance = widget.HighImportance
	} else {
		c.Importance = widget.MediumImportance
	}
	if id.Col == 1 {
		c.TextStyle = fyne.TextStyle{}
	} else {
		c.TextStyle = fyne.TextStyle{Monospace: true}
	}
	c.Refresh()
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
			fyne.NewMenuItem(i18n.T("menu.delete"), func() { a.deleteRow(row) }),
		)
		widget.ShowPopUpMenuAtRelativePosition(fyne.NewMenu("", items...), a.win.Canvas(), rel, anchor)
		return
	}
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(i18n.T("menu.change_value"), func() { a.changeValueDialog(row) }),
		fyne.NewMenuItem(i18n.T("menu.change_description"), func() { a.changeDescriptionDialog(row) }),
		fyne.NewMenuItem(i18n.T("menu.configure_bitfield"), func() { a.configureBitfieldDialog(row) }),
		fyne.NewMenuItem(i18n.T("menu.freeze"), func() { a.toggleFreezeRow(row) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.browse"), func() { a.browseRow(row) }),
		fyne.NewMenuItem(i18n.T("menu.disassemble"), func() { a.disassembleRow(row) }),
		fyne.NewMenuItem(i18n.T("menu.find_writes"), func() { a.findWhatWrites(row, true) }),
		fyne.NewMenuItem(i18n.T("menu.find_accesses"), func() { a.findWhatWrites(row, false) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.show_decimal"), func() { a.setDisplay(row, displayDefault) }),
		fyne.NewMenuItem(i18n.T("menu.show_hex"), func() { a.setDisplay(row, displayHex) }),
		fyne.NewMenuItem(i18n.T("menu.show_binary"), func() { a.setDisplay(row, displayBinary) }),
		fyne.NewMenuItem(i18n.T("menu.assign_hotkey"), func() { a.assignHotkey(row) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.delete"), func() { a.deleteRow(row) }),
	)
	widget.ShowPopUpMenuAtRelativePosition(menu, a.win.Canvas(), rel, anchor)
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
// stored Cheat Engine expression.
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

func (a *App) addResultToTable(i int) {
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
		a.changeFoundValue(a.foundSel)
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

// promptValue shows Cheat Engine's single-field Change value dialog.
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
func (a *App) changeFoundValue(i int) {
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
		if a.foundList != nil {
			a.foundList.Refresh()
		}
	})
}

// changeFoundValues applies one input to every selected Found result.
func (a *App) changeFoundValues(sel []int) {
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
func (a *App) foundValueType() scan.ValueType {
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

// expandValueInput rewrites a Cheat Engine-style change-value input for a cheat
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
func (a *App) expandFoundInput(input string, i int) string {
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
// a group, matching Cheat Engine's recursive set option.
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
	a.resetTree()
	a.syncFreezeTargets()
	a.table.Refresh()
	a.updateScanControls()
}

func parseAddress(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, fmt.Errorf("%s", i18n.T("error.empty_address"))
	}
	return strconv.ParseUint(s, 16, 64)
}

// dataCell is a table cell that reports taps and right-clicks.
type dataCell struct {
	*widget.Label
	app      *App
	row, col int
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
