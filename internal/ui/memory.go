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
	a.memAddrEntry = widget.NewEntry()
	a.memAddrEntry.SetText("0x0")
	a.memAddrEntry.OnSubmitted = func(string) { a.goToAddress() }

	a.memType = widget.NewSelect(memTypeLabels(), func(string) { a.reloadMemory() })
	a.memType.SetSelected(memTypeLabel(scan.TypeDword))

	bar := container.NewHBox(
		widget.NewLabel(i18n.T("memory.address")), a.memAddrEntry,
		widget.NewButton(i18n.T("memory.go"), a.goToAddress),
		widget.NewSeparator(),
		widget.NewLabel(i18n.T("memory.display")), a.memType,
		widget.NewButton(i18n.T("memory.find"), a.findDialog),
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
	a.memWin.SetContent(container.NewBorder(bar, nil, nil, nil, split))
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
