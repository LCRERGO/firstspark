//go:build gui

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

const hexWindow = 256

// memTypeOption pairs a memory-viewer display width with its translation key.
type memTypeOption struct {
	key string
	typ scan.ValueType
}

var memTypeOptions = []memTypeOption{
	{"memory.type.byte", scan.TypeByte},
	{"memory.type.2bytes", scan.TypeWord},
	{"memory.type.4bytes", scan.TypeDword},
	{"memory.type.8bytes", scan.TypeQword},
	{"memory.type.float", scan.TypeFloat},
	{"memory.type.double", scan.TypeDouble},
}

func memTypeLabels() []string {
	out := make([]string, len(memTypeOptions))
	for i, o := range memTypeOptions {
		out[i] = i18n.T(o.key)
	}
	return out
}

func memTypeLabel(t scan.ValueType) string {
	for _, o := range memTypeOptions {
		if o.typ == t {
			return i18n.T(o.key)
		}
	}
	return i18n.T("memory.type.4bytes")
}

func parseMemType(label string) scan.ValueType {
	for _, o := range memTypeOptions {
		if i18n.T(o.key) == label {
			return o.typ
		}
	}
	return scan.TypeDword
}

// openMemoryViewer shows the separate Memory Viewer window, creating it lazily.
func (a *App) openMemoryViewer() {
	if a.memWin == nil {
		a.memWin = a.fapp.NewWindow(i18n.T("memory.title"))
		a.memWin.Resize(fyne.NewSize(801, 530))
		a.buildMemoryViewer()
	}
	a.memWin.Show()
}

func (a *App) buildMemoryViewer() {
	a.memAddrEntry = newHintEntry("memory.hint.address")
	a.memAddrEntry.SetText("0x0")
	a.memAddrEntry.OnSubmitted = func(string) { a.goToAddress() }

	a.memType = newHintSelect(memTypeLabels(), "memory.hint.display", func(string) { a.reloadMemory() })
	a.memType.SetSelected(memTypeLabel(scan.TypeDword))

	bar := container.NewHBox(
		widget.NewLabel(i18n.T("memory.address")), a.memAddrEntry,
		newHintButton(i18n.T("memory.go"), "memory.hint.go", a.goToAddress),
		widget.NewSeparator(),
		widget.NewLabel(i18n.T("memory.display")), a.memType,
		newHintButton(i18n.T("memory.find"), "memory.hint.find", a.findDialog),
		newHintButton(i18n.T("memory.change_value"), "memory.hint.change_value", a.changeMemoryValue),
	)

	a.disasmList = widget.NewList(
		func() int { return len(a.disasm) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.disasm) {
				t.Text = ""
				t.Refresh()
				return
			}
			ins := a.disasm[id]
			t.Text = fmt.Sprintf("%016x  %-24x  %s", ins.Addr, ins.Bytes, ins.Text)
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.hexList = widget.NewList(
		func() int { return len(a.hexLines) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.hexLines) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = a.hexLines[id]
			t.Color = a.pal().text
			t.Refresh()
		},
	)

	split := container.NewVSplit(a.disasmList, a.hexList)
	split.SetOffset(0.69)
	a.memWin.SetContent(fynetooltip.AddWindowToolTipLayer(container.NewBorder(bar, nil, nil, nil, split), a.memWin.Canvas()))
	viewItems := make([]*fyne.MenuItem, len(memTypeOptions))
	for i, o := range memTypeOptions {
		opt := o
		viewItems[i] = fyne.NewMenuItem(i18n.T(opt.key), func() { a.memType.SetSelected(i18n.T(opt.key)) })
	}
	a.memWin.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu(i18n.T("menu.file"), fyne.NewMenuItem(i18n.T("menu.file.close"), func() { a.memWin.Hide() })),
		fyne.NewMenu(i18n.T("menu.search"),
			fyne.NewMenuItem(i18n.T("menu.search.find"), a.findDialog),
			fyne.NewMenuItem(i18n.T("menu.search.find_next"), a.findNext),
		),
		fyne.NewMenu(i18n.T("menu.view"), viewItems...),
	))
	a.installMemoryShortcuts()
}

func (a *App) installMemoryShortcuts() {
	c := a.memWin.Canvas()
	c.AddShortcut(ctrl(fyne.KeyG), func(fyne.Shortcut) { c.Focus(a.memAddrEntry) })
	c.AddShortcut(ctrl(fyne.KeyF), func(fyne.Shortcut) { a.findDialog() })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF3}, func(fyne.Shortcut) { a.findNext() })
	// Cheat Engine selects the display width with Ctrl+1..0.
	keys := []fyne.KeyName{fyne.Key1, fyne.Key2, fyne.Key3, fyne.Key4, fyne.Key5, fyne.Key6}
	for i, k := range keys {
		if i >= len(memTypeOptions) {
			break
		}
		label := i18n.T(memTypeOptions[i].key)
		c.AddShortcut(ctrl(k), func(fyne.Shortcut) { a.memType.SetSelected(label) })
	}
}

func (a *App) goToAddress() {
	addr, err := parseAddress(a.memAddrEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	a.loadMemory(addr)
}

// loadMemory reads a page around addr and refreshes the viewer panes.
func (a *App) loadMemory(addr uint64) {
	if a.proc == nil {
		a.setStatusText(i18n.T("error.no_process"))
		return
	}
	page := addr &^ 0xFF
	data, err := a.proc.Read(page, hexWindow)
	if err != nil && len(data) == 0 {
		a.fail(err)
		return
	}
	a.hexAddr = page
	a.hexData = data
	a.disasm = asm.Disassemble(data, page)
	a.hexLines = a.buildHexLines(page, data)
	if a.memAddrEntry != nil {
		a.memAddrEntry.SetText(fmt.Sprintf("0x%x", addr))
	}
	if a.disasmList != nil {
		a.disasmList.Refresh()
	}
	if a.hexList != nil {
		a.hexList.Refresh()
	}
}

func (a *App) reloadMemory() {
	if a.hexData != nil {
		a.loadMemory(a.hexAddr)
	}
}

// buildHexLines renders the hex pane rows, appending a decoded value column.
func (a *App) buildHexLines(base uint64, data []byte) []string {
	typ := scan.TypeDword
	if a.memType != nil {
		typ = parseMemType(a.memType.Selected)
	}
	w := typ.Size()
	if w <= 0 {
		w = 1
	}
	lines := make([]string, 0, (len(data)+15)/16)
	for i := 0; i < len(data); i += 16 {
		end := min(i+16, len(data))
		var b strings.Builder
		fmt.Fprintf(&b, "%016x  ", base+uint64(i))
		for j := i; j < end; j++ {
			fmt.Fprintf(&b, "%02x ", data[j])
		}
		for j := end; j < i+16; j++ {
			b.WriteString("   ")
		}
		b.WriteByte(' ')
		for j := i; j < end; j++ {
			c := data[j]
			if c >= 0x20 && c < 0x7f {
				b.WriteByte(c)
			} else {
				b.WriteByte('.')
			}
		}
		if i+w <= len(data) {
			v := scan.NewValue(typ, data[i:i+w])
			fmt.Fprintf(&b, "  = %s", v.String())
		}
		lines = append(lines, b.String())
	}
	return lines
}

func (a *App) findDialog() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(i18n.T("placeholder.aob_pattern"))
	d := dialog.NewForm(i18n.T("dialog.find.title"), i18n.T("action.find"), i18n.T("action.cancel"),
		[]*widget.FormItem{widget.NewFormItem(i18n.T("field.aob_pattern"), entry)},
		func(ok bool) {
			if !ok {
				return
			}
			p, err := scan.ParseAOB(entry.Text)
			if err != nil {
				a.fail(err)
				return
			}
			a.searchPat = p.Bytes
			a.searchMask = p.Mask
			a.searchNext = a.hexAddr
			a.findNext()
		}, a.memWin)
	d.Resize(fyne.NewSize(420, 180))
	d.Show()
}

func (a *App) findNext() {
	if len(a.searchPat) == 0 {
		a.setStatusText(i18n.T("status.no_search_pattern"))
		return
	}
	start := 0
	if a.searchNext > a.hexAddr {
		start = int(a.searchNext - a.hexAddr)
	}
	for i := start; i+len(a.searchPat) <= len(a.hexData); i++ {
		if matchAt(a.hexData, i, a.searchPat, a.searchMask) {
			addr := a.hexAddr + uint64(i)
			a.searchNext = addr + 1
			a.loadMemory(addr)
			a.setStatusText(i18n.Tf("status.found_at", map[string]any{"Addr": fmt.Sprintf("%x", addr)}))
			return
		}
	}
	a.setStatusText(i18n.T("status.pattern_not_found"))
}

func matchAt(data []byte, off int, pat, mask []byte) bool {
	for k := range pat {
		if len(mask) > k && mask[k] == 0 {
			continue
		}
		if data[off+k] != pat[k] {
			return false
		}
	}
	return true
}

// memValueOption is one entry of the Change value form's type list.
type memValueOption struct {
	label   string
	typ     scan.ValueType
	integer bool
	text    bool
}

func memValueOptions() []memValueOption {
	return []memValueOption{
		{i18n.T("memory.type.byte"), scan.TypeByte, true, false},
		{i18n.T("memory.type.2bytes"), scan.TypeWord, true, false},
		{i18n.T("memory.type.4bytes"), scan.TypeDword, true, false},
		{i18n.T("memory.type.8bytes"), scan.TypeQword, true, false},
		{i18n.T("memory.type.float"), scan.TypeFloat, false, false},
		{i18n.T("memory.type.double"), scan.TypeDouble, false, false},
		{i18n.T("memory.type.text"), scan.TypeString, false, true},
		{i18n.T("memory.type.aob"), scan.TypeAOB, false, false},
	}
}

// readMemoryValue reads a value of the given type at addr. Variable-width
// types use a default window (64 bytes for text, 8 for AOB).
func (a *App) readMemoryValue(addr uint64, typ scan.ValueType) (scan.Value, error) {
	if a.proc == nil {
		return scan.Value{}, fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	size := typ.Size()
	if size <= 0 {
		if typ == scan.TypeAOB {
			size = 8
		} else {
			size = 64
		}
	}
	raw, err := a.proc.Read(addr, size)
	if err != nil {
		return scan.Value{}, err
	}
	return scan.NewValue(typ, raw), nil
}

// changeMemoryValue opens Cheat Engine's Memory Viewer change-value form: a
// value field, a type selector and a Hexadecimal/Unicode checkbox that depends
// on the type. It reads the address fresh when the type changes and writes on
// OK (ADR 0037 value-changer phase 2).
func (a *App) changeMemoryValue() {
	if a.proc == nil {
		a.setStatusText(i18n.T("error.no_process"))
		return
	}
	addr, err := parseAddress(a.memAddrEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	opts := memValueOptions()
	labels := make([]string, len(opts))
	for i, o := range opts {
		labels[i] = o.label
	}
	valueEntry := widget.NewEntry()
	typeSel := widget.NewSelect(labels, nil)
	typeSel.SetSelected(labels[2])
	check := widget.NewCheck(i18n.T("memory.hexadecimal"), nil)

	selected := func() memValueOption {
		for _, o := range opts {
			if o.label == typeSel.Selected {
				return o
			}
		}
		return opts[2]
	}
	effectiveType := func() scan.ValueType {
		o := selected()
		if o.text && check.Checked {
			return scan.TypeUTF16LE
		}
		return o.typ
	}
	refresh := func() {
		o := selected()
		switch {
		case o.integer:
			check.SetText(i18n.T("memory.hexadecimal"))
			check.Enable()
		case o.text:
			check.SetText(i18n.T("memory.unicode"))
			check.Enable()
		default:
			check.SetText(i18n.T("memory.hexadecimal"))
			check.Disable()
		}
		v, rerr := a.readMemoryValue(addr, effectiveType())
		if rerr != nil {
			valueEntry.SetText("")
			return
		}
		if o.integer && check.Checked {
			valueEntry.SetText(hexOf(v))
		} else {
			valueEntry.SetText(v.String())
		}
	}
	typeSel.OnChanged = func(string) { refresh() }
	check.OnChanged = func(bool) { refresh() }
	refresh()

	form := widget.NewForm(
		widget.NewFormItem(i18n.T("field.value"), valueEntry),
		widget.NewFormItem(i18n.T("field.type"), typeSel),
		widget.NewFormItem("", check),
	)
	d := dialog.NewCustomConfirm(i18n.T("dialog.change_value.title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		form, func(ok bool) {
			if !ok || a.proc == nil {
				return
			}
			v, perr := scan.ParseValue(effectiveType(), valueEntry.Text)
			if perr != nil {
				a.fail(perr)
				return
			}
			if werr := a.proc.Write(addr, v.Raw); werr != nil {
				a.fail(werr)
				return
			}
			a.loadMemory(a.hexAddr)
			a.setStatusText(i18n.Tf("status.memory_changed", map[string]any{"Addr": fmt.Sprintf("%x", addr)}))
		}, a.memWin)
	d.Resize(fyne.NewSize(440, 280))
	d.Show()
}
