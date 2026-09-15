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

	"github.com/LCRERGO/firstspark/pkg/scan"
)

var cheatHeaders = []string{"Active", "Description", "Address", "Type", "Value"}

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
		a.th.heading("Found", a.th.size+2, a.pal().primary),
		layout.NewSpacer(),
		widget.NewButton("Add to Table", func() { a.addResultToTable(a.foundSel) }),
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
	a.foundCount.SetText(fmt.Sprintf("Found: %d", len(a.results)))
	if a.foundList != nil {
		a.foundList.Refresh()
	}
}

// buildCheatTable creates the five-column cheat table.
func (a *App) buildCheatTable() {
	a.table = widget.NewTable(
		func() (int, int) { return len(a.entries), len(cheatHeaders) },
		func() fyne.CanvasObject { return a.newDataCell() },
		func(id widget.TableCellID, o fyne.CanvasObject) { a.updateDataCell(id, o) },
	)
	a.table.ShowHeaderRow = true
	a.table.CreateHeader = func() fyne.CanvasObject { return a.monoText("") }
	a.table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		t := o.(*canvas.Text)
		if id.Col < 0 || id.Col >= len(cheatHeaders) {
			t.Text = ""
			t.Refresh()
			return
		}
		t.Text = cheatHeaders[id.Col]
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
		a.th.heading("Cheat Table", a.th.size+2, a.pal().primary),
		container.NewHBox(
			widget.NewButton("Add Address Manually", a.addAddressDialog),
			widget.NewButton("Clear List", a.clearTable),
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
		return e.value.String()
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
		fyne.NewMenuItem("Change Value...", func() { a.changeValueDialog(row) }),
		fyne.NewMenuItem("Change Description...", func() { a.changeDescriptionDialog(row) }),
		fyne.NewMenuItem("Freeze/Unfreeze", func() { a.toggleFreezeRow(row) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Browse this memory region", func() { a.browseRow(row) }),
		fyne.NewMenuItem("Disassemble this memory region", func() { a.disassembleRow(row) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Delete this record", func() { a.deleteRow(row) }),
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
}

func (a *App) addResultToTable(i int) {
	if i < 0 || i >= len(a.results) {
		a.setStatus("select a found result first")
		return
	}
	r := a.results[i]
	typ := a.defaultValueType()
	if a.session != nil {
		typ = a.session.Options().Type
	}
	a.entries = append(a.entries, tableEntry{addr: r.Addr, typ: typ, value: r.Prev, orig: r.Prev})
	a.table.Refresh()
	a.setStatus("added 0x%x to the cheat table", r.Addr)
}

func (a *App) browseRow(row int) {
	if row < 0 || row >= len(a.entries) {
		a.setStatus("select a cheat table row first")
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
		a.setStatus("select a cheat table row first")
		return
	}
	e := a.entries[row]
	entry := widget.NewEntry()
	entry.SetText(e.value.String())
	d := dialog.NewForm("Change Value", "Apply", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Value", entry)},
		func(ok bool) {
			if !ok {
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
		a.setStatus("select a cheat table row first")
		return
	}
	entry := widget.NewEntry()
	entry.SetText(a.entries[row].desc)
	d := dialog.NewForm("Change Description", "Apply", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Description", entry)},
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
		return fmt.Errorf("no process selected")
	}
	if err := a.proc.Write(addr, v.Raw); err != nil {
		return err
	}
	return nil
}

func (a *App) addAddressDialog() {
	addr := widget.NewEntry()
	addr.SetPlaceHolder("0x1234 or 1234 (hex)")
	typ := widget.NewSelect(valueTypeOptions(), nil)
	typ.SetSelected(ceValueTypeLabel(a.defaultValueType()))
	desc := widget.NewEntry()
	val := widget.NewEntry()
	val.SetPlaceHolder("optional")
	d := dialog.NewForm("Add Address Manually", "Add", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Address", addr),
			widget.NewFormItem("Type", typ),
			widget.NewFormItem("Description", desc),
			widget.NewFormItem("Value", val),
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
			a.entries = append(a.entries, tableEntry{addr: target, typ: t, desc: desc.Text, value: v, orig: v})
			a.table.Refresh()
		}, a.win)
	d.Resize(fyne.NewSize(420, 320))
	d.Show()
}

func (a *App) clearTable() {
	a.entries = nil
	a.tableSel = -1
	a.table.Refresh()
}

func parseAddress(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, fmt.Errorf("empty address")
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
