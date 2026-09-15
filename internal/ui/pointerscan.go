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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/pointerscan"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// showPointerScan opens the pointer scan configuration dialog.
func (a *App) showPointerScan() {
	if a.proc == nil {
		a.fail(fmt.Errorf("no process selected"))
		return
	}
	target := widget.NewEntry()
	switch {
	case a.tableSel >= 0 && a.tableSel < len(a.entries):
		target.SetText(fmt.Sprintf("0x%x", a.entries[a.tableSel].addr))
	case len(a.results) > 0:
		target.SetText(fmt.Sprintf("0x%x", a.results[0].Addr))
	}
	target.SetPlaceHolder("0x1234")
	level := widget.NewEntry()
	level.SetText("5")
	offset := widget.NewEntry()
	offset.SetText("2048")
	aligned := widget.NewCheck("Aligned", nil)
	aligned.SetChecked(true)
	staticOnly := widget.NewCheck("Static only", nil)
	writable := widget.NewCheck("Writable memory only", nil)
	writable.SetChecked(true)

	d := dialog.NewForm("Pointer Scan", "Scan", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Target address", target),
			widget.NewFormItem("Max level", level),
			widget.NewFormItem("Max offset", offset),
			widget.NewFormItem("Options", container.NewVBox(aligned, staticOnly, writable)),
		},
		func(ok bool) {
			if !ok {
				return
			}
			addr, err := parseAddress(target.Text)
			if err != nil {
				a.fail(err)
				return
			}
			lvl, _ := strconv.Atoi(strings.TrimSpace(level.Text))
			off, _ := strconv.ParseUint(strings.TrimSpace(offset.Text), 10, 64)
			a.runPointerScan(addr, lvl, off, aligned.Checked, staticOnly.Checked, writable.Checked)
		}, a.win)
	d.Resize(fyne.NewSize(440, 420))
	d.Show()
}

func (a *App) runPointerScan(target uint64, level int, maxOffset uint64, aligned, staticOnly, writable bool) {
	proc := a.proc
	progress := dialog.NewProgressInfinite("Pointer Scan", "Building pointermap...", a.win)
	progress.Show()
	go func() {
		pm, err := pointerscan.BuildPointermap(proc, pointerscan.BuildOptions{
			WritableOnly: writable,
			Aligned:      aligned,
			MaxBytes:     1 << 30,
		})
		if err != nil {
			fyne.Do(func() { progress.Hide(); a.fail(err) })
			return
		}
		opts := pointerscan.DefaultOptions()
		if level > 0 {
			opts.MaxLevel = level
		}
		if maxOffset > 0 {
			opts.MaxOffset = maxOffset
		}
		opts.Aligned = aligned
		opts.StaticOnly = staticOnly
		chains := pm.Scan(target, opts)
		fyne.Do(func() { progress.Hide(); a.showPointerResults(chains) })
	}()
}

func (a *App) showPointerResults(chains []pointerscan.Chain) {
	if len(chains) == 0 {
		a.fail(fmt.Errorf("no pointer chains found"))
		return
	}
	labels := make([]string, len(chains))
	for i, c := range chains {
		labels[i] = formatChain(c)
	}
	sel := -1
	list := widget.NewList(
		func() int { return len(labels) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(labels) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = labels[id]
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	list.OnSelected = func(id widget.ListItemID) { sel = int(id) }

	add := widget.NewButton("Add to Table", func() {
		if sel < 0 || sel >= len(chains) {
			return
		}
		a.addPointerChain(chains[sel])
	})
	save := widget.NewButton("Save...", func() { a.saveChains(chains) })
	footer := container.NewHBox(
		widget.NewLabel(fmt.Sprintf("%d chains", len(chains))),
		layout.NewSpacer(), save, add,
	)
	content := container.NewBorder(nil, footer, nil, nil, list)
	d := dialog.NewCustom("Pointer Scan Results", "Close", content, a.win)
	d.Resize(fyne.NewSize(620, 480))
	d.Show()
}

func formatChain(c pointerscan.Chain) string {
	var b strings.Builder
	if c.Module != "" {
		fmt.Fprintf(&b, "%s+0x%x", c.Module, c.Base)
	} else {
		fmt.Fprintf(&b, "0x%x", c.Base)
	}
	for _, off := range c.Offsets {
		fmt.Fprintf(&b, " -> +0x%x", off)
	}
	return b.String()
}

// chainToPointer converts a scan chain into the cheat table's pointer model,
// where the base is the address of the first pointer.
func chainToPointer(c pointerscan.Chain) (*pointerChain, bool) {
	if len(c.Offsets) == 0 {
		return nil, false
	}
	offs := make([]int64, 0, len(c.Offsets)-1)
	for _, o := range c.Offsets[1:] {
		offs = append(offs, int64(o))
	}
	return &pointerChain{base: c.Base + c.Offsets[0], offsets: offs}, true
}

func (a *App) addPointerChain(c pointerscan.Chain) {
	pc, ok := chainToPointer(c)
	if !ok {
		return
	}
	base := pc.base
	typ := a.defaultValueType()
	var v scan.Value
	if w := typ.Size(); w > 0 && a.proc != nil {
		if raw, err := a.proc.Read(base, w); err == nil {
			v = scan.NewValue(typ, raw)
		}
	}
	a.entries = append(a.entries, tableEntry{
		addr: base, typ: typ, desc: formatChain(c),
		value: v, orig: v, pointer: pc,
	})
	a.resolvePointers()
	a.table.Refresh()
	a.setStatus("added pointer chain")
}

func (a *App) saveChains(chains []pointerscan.Chain) {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			a.fail(err)
			return
		}
		if w == nil {
			return
		}
		path := w.URI().Path()
		_ = w.Close()
		if err := pointerscan.Save(path, chains); err != nil {
			a.fail(err)
			return
		}
		a.setStatus("saved %s", path)
	}, a.win)
	d.SetFileName("scan.ptr")
	d.Resize(fyne.NewSize(640, 480))
	d.Show()
}
