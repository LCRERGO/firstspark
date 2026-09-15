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

	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

const hexWindow = 256

// openMemoryViewer shows the separate Memory Viewer window, creating it lazily.
func (a *App) openMemoryViewer() {
	if a.memWin == nil {
		a.memWin = a.fapp.NewWindow("Memory Viewer")
		a.memWin.Resize(fyne.NewSize(801, 530))
		a.buildMemoryViewer()
	}
	a.memWin.Show()
}

func (a *App) buildMemoryViewer() {
	a.memAddrEntry = widget.NewEntry()
	a.memAddrEntry.SetText("0x0")
	a.memAddrEntry.OnSubmitted = func(string) { a.goToAddress() }

	a.memType = widget.NewSelect([]string{"Byte", "2 Bytes", "4 Bytes", "8 Bytes", "Float", "Double"},
		func(string) { a.reloadMemory() })
	a.memType.SetSelected("4 Bytes")

	bar := container.NewHBox(
		widget.NewLabel("Address"), a.memAddrEntry,
		widget.NewButton("Go", a.goToAddress),
		widget.NewSeparator(),
		widget.NewLabel("Display"), a.memType,
		widget.NewButton("Find...", a.findDialog),
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
	a.memWin.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("File", fyne.NewMenuItem("Close", func() { a.memWin.Hide() })),
		fyne.NewMenu("Search",
			fyne.NewMenuItem("Find...", a.findDialog),
			fyne.NewMenuItem("Find Next", a.findNext),
		),
		fyne.NewMenu("View",
			fyne.NewMenuItem("Byte", func() { a.memType.SetSelected("Byte") }),
			fyne.NewMenuItem("2 Bytes", func() { a.memType.SetSelected("2 Bytes") }),
			fyne.NewMenuItem("4 Bytes", func() { a.memType.SetSelected("4 Bytes") }),
			fyne.NewMenuItem("8 Bytes", func() { a.memType.SetSelected("8 Bytes") }),
			fyne.NewMenuItem("Float", func() { a.memType.SetSelected("Float") }),
			fyne.NewMenuItem("Double", func() { a.memType.SetSelected("Double") }),
		),
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
		a.setStatus("no process selected")
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
		typ = parseCEValueType(a.memType.Selected)
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
	entry.SetPlaceHolder("e.g. 48 8B ?? E8")
	d := dialog.NewForm("Find", "Find", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("AOB pattern", entry)},
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
		a.setStatus("no search pattern; use Search > Find...")
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
			a.setStatus("found at 0x%x", addr)
			return
		}
	}
	a.setStatus("pattern not found in this window")
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
