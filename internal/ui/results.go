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
func (a *App) formatEntryValue(e tableEntry) string {
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
	a.foundList = widget.NewList(
		func() int { return len(a.results) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.results) {
				t.Text = ""
				t.Refresh()
				return
			}
			r := a.results[id]
			t.Text = fmt.Sprintf("0x%012x  %s", r.Addr, r.Prev.String())
			if int(id) == a.foundSel {
				t.Color = a.pal().primary
			} else {
				t.Color = a.pal().text
			}
			t.Refresh()
		},
	)
	a.foundList.OnSelected = func(id widget.ListItemID) { a.selectFound(int(id)) }
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
	a.foundCount.SetText(i18n.Tf("app.found_count", map[string]any{"Count": len(a.results)}))
	if a.foundList != nil {
		a.foundList.Refresh()
	}
}

// buildCheatTable creates the five-column cheat table.
func (a *App) buildCheatTable() {
	headers := cheatHeaders()
	a.table = widget.NewTable(
		func() (int, int) { return len(a.entries), len(headers) },
		func() fyne.CanvasObject { return a.newDataCell() },
		func(id widget.TableCellID, o fyne.CanvasObject) { a.updateDataCell(id, o) },
	)
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
		if a.isFrozen(e.addr) {
			return "X"
		}
		return ""
	case 1:
		return e.desc
	case 2:
		return fmt.Sprintf("0x%x", e.addr)
	case 3:
		return ceValueTypeLabel(e.typ)
	case 4:
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
	a.tableSel = row
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
	a.tableSel = row
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
	if row < 0 || row >= len(a.entries) {
		return
	}
	e := a.entries[row]
	a.mu.Lock()
	if _, ok := a.frozen[e.addr]; ok {
		delete(a.frozen, e.addr)
	} else {
		a.frozen[e.addr] = e.value
	}
	a.mu.Unlock()
	a.table.Refresh()
}

func (a *App) deleteRow(row int) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	a.entries = append(a.entries[:row], a.entries[row+1:]...)
	a.tableSel = -1
	a.table.Refresh()
	a.updateScanControls()
}

func (a *App) setDisplay(row int, d displayFormat) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	a.entries[row].display = d
	a.table.Refresh()
}

// resolvePointers recomputes pointer-entry addresses and values. It must run
// on the UI goroutine because it touches the entry slice.
func (a *App) resolvePointers() bool {
	if a.proc == nil {
		return false
	}
	changed := false
	for i := range a.entries {
		e := &a.entries[i]
		if e.pointer == nil {
			continue
		}
		addr, err := resolvePointer(a.proc, e.pointer)
		if err != nil {
			continue
		}
		e.addr = addr
		if w := e.typ.Size(); w > 0 {
			if raw, err := a.proc.Read(addr, w); err == nil {
				e.value = scan.NewValue(e.typ, raw)
			}
		}
		changed = true
	}
	return changed
}

func (a *App) assignHotkey(row int) {
	if row < 0 || row >= len(a.entries) {
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
			a.bindHotkey(key, row)
			a.setStatusText(i18n.Tf("status.hotkey_assigned", map[string]any{"Key": key}))
		}, a.win)
	d.Resize(fyne.NewSize(340, 160))
	d.Show()
}

// bindHotkey registers a shortcut that freezes/unfreezes the given row.
func (a *App) bindHotkey(key fyne.KeyName, row int) {
	sc := &desktop.CustomShortcut{KeyName: key}
	if !strings.HasPrefix(string(key), "F") {
		sc.Modifier = fyne.KeyModifierControl | fyne.KeyModifierAlt
	}
	a.win.Canvas().AddShortcut(sc, func(fyne.Shortcut) { a.toggleFreezeRow(row) })
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
	if i < 0 || i >= len(a.results) {
		a.setStatusText(i18n.T("status.select_found"))
		return
	}
	r := a.results[i]
	typ := a.defaultValueType()
	if a.session != nil {
		typ = a.session.Options().Type
	}
	a.entries = append(a.entries, tableEntry{addr: r.Addr, typ: typ, value: r.Prev, orig: r.Prev})
	a.table.Refresh()
	a.setStatusText(i18n.Tf("status.added_to_table", map[string]any{"Addr": fmt.Sprintf("%x", r.Addr)}))
	a.updateScanControls()
}

func (a *App) browseRow(row int) {
	if row < 0 || row >= len(a.entries) {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	a.openMemoryViewer()
	a.loadMemory(a.entries[row].addr)
}

func (a *App) disassembleRow(row int) {
	a.browseRow(row)
}

func (a *App) changeValueDialog(row int) {
	if row < 0 || row >= len(a.entries) {
		a.setStatusText(i18n.T("status.select_cheat_row"))
		return
	}
	e := a.entries[row]
	entry := widget.NewEntry()
	if e.bit != nil {
		entry.SetText(e.bit.format(e.value.Raw))
	} else {
		entry.SetText(e.value.String())
	}
	d := dialog.NewForm(i18n.T("dialog.change_value.title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		[]*widget.FormItem{widget.NewFormItem(i18n.T("field.value"), entry)},
		func(ok bool) {
			if !ok {
				return
			}
			if e.bit != nil {
				field, err := e.bit.parse(entry.Text)
				if err != nil {
					a.fail(err)
					return
				}
				updated, err := a.writeBitfield(e.addr, *e.bit, field)
				if err != nil {
					a.fail(err)
					return
				}
				a.entries[row].value = scan.NewValue(e.typ, updated)
				a.entries[row].orig = a.entries[row].value
				a.table.Refresh()
				return
			}
			v, err := scan.ParseValue(e.typ, entry.Text)
			if err != nil {
				a.fail(err)
				return
			}
			if err := a.writeValue(e.addr, v); err != nil {
				a.fail(err)
				return
			}
			a.entries[row].value = v
			a.table.Refresh()
		}, a.win)
	d.Resize(fyne.NewSize(360, 180))
	d.Show()
}

// configureBitfieldDialog turns a row into a bitfield edit/display entry.
func (a *App) configureBitfieldDialog(row int) {
	if row < 0 || row >= len(a.entries) {
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
				}
			}
			a.table.Refresh()
		}, a.win)
	d.Resize(fyne.NewSize(360, 240))
	d.Show()
}

func (a *App) changeValueBack(row int) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	if err := a.writeValue(a.entries[row].addr, a.entries[row].orig); err != nil {
		a.fail(err)
		return
	}
	a.entries[row].value = a.entries[row].orig
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
			a.entries = append(a.entries, tableEntry{addr: target, typ: t, desc: desc.Text, value: v, orig: v, pointer: pc})
			a.table.Refresh()
			a.updateScanControls()
		}, a.win)
	d.Resize(fyne.NewSize(440, 420))
	d.Show()
}

func (a *App) clearTable() {
	a.entries = nil
	a.tableSel = -1
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
	}
}
