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
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/pointerscan"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// showPointerScan opens the pointer scan configuration dialog.
func (a *App) showPointerScan() {
	if a.proc == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.no_process")))
		return
	}
	target := widget.NewEntry()
	switch {
	case a.tableSel >= 0 && a.tableSel < len(a.entries) &&
		!a.entries[a.tableSel].group && a.entries[a.tableSel].expr == "":
		target.SetText(fmt.Sprintf("0x%x", a.entries[a.tableSel].addr))
	case len(a.results) > 0:
		target.SetText(fmt.Sprintf("0x%x", a.results[0].Addr))
	}
	target.SetPlaceHolder(i18n.T("debugger.address_placeholder"))
	level := widget.NewEntry()
	level.SetText("5")
	offset := widget.NewEntry()
	offset.SetText("2048")
	aligned := widget.NewCheck(i18n.T("pointerscan.aligned"), nil)
	aligned.SetChecked(true)
	staticOnly := widget.NewCheck(i18n.T("pointerscan.static_only"), nil)
	writable := widget.NewCheck(i18n.T("pointerscan.writable_only"), nil)
	writable.SetChecked(true)

	d := dialog.NewForm(i18n.T("pointerscan.title"), i18n.T("pointerscan.scan"), i18n.T("action.cancel"),
		[]*widget.FormItem{
			widget.NewFormItem(i18n.T("pointerscan.target"), target),
			widget.NewFormItem(i18n.T("pointerscan.max_level"), level),
			widget.NewFormItem(i18n.T("pointerscan.max_offset"), offset),
			widget.NewFormItem(i18n.T("pointerscan.options"), container.NewVBox(aligned, staticOnly, writable)),
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
	log.Info("pointer scan started", "pid", proc.PID, "target", fmt.Sprintf("0x%x", target), "level", level, "max_offset", maxOffset)
	progress := dialog.NewProgressInfinite(i18n.T("pointerscan.title"), i18n.T("pointerscan.building"), a.win)
	progress.Show()
	go func() {
		pm, err := pointerscan.BuildOrLoad(proc, pointerscan.BuildOptions{
			WritableOnly: writable,
			Aligned:      aligned,
			MaxBytes:     1 << 30,
		})
		if err != nil {
			log.Warn("pointer scan failed", "pid", proc.PID, "err", err)
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
		log.Info("pointer scan finished", "pid", proc.PID, "chains", len(chains))
		fyne.Do(func() { progress.Hide(); a.showPointerResults(chains) })
	}()
}

func (a *App) showPointerResults(chains []pointerscan.Chain) {
	if len(chains) == 0 {
		a.fail(fmt.Errorf("%s", i18n.T("pointerscan.no_chains")))
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

	add := widget.NewButton(i18n.T("pointerscan.add_to_table"), func() {
		if sel < 0 || sel >= len(chains) {
			return
		}
		a.addPointerChain(chains[sel])
	})
	save := widget.NewButton(i18n.T("pointerscan.save"), func() { a.saveChains(chains) })
	footer := container.NewHBox(
		widget.NewLabel(i18n.Tf("pointerscan.chains", map[string]any{"Count": len(chains)})),
		layout.NewSpacer(), save, add,
	)
	content := container.NewBorder(nil, footer, nil, nil, list)
	d := dialog.NewCustom(i18n.T("pointerscan.results"), i18n.T("menu.file.close"), content, a.win)
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
	if c.Module != "" {
		return &pointerChain{module: c.Module, offset: c.Offsets[0], offsets: offs}, true
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
	a.addRoot(&tableEntry{
		addr: base, typ: typ, desc: formatChain(c),
		value: v, orig: v, pointer: pc,
	})
	a.refreshEntries()
	a.syncFreezeTargets()
	a.table.Refresh()
	a.setStatusText(i18n.T("status.added_pointer_chain"))
}

// loadPointerScan opens a saved .ptr file and shows its chains.
func (a *App) loadPointerScan() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			a.fail(err)
			return
		}
		if r == nil {
			return
		}
		path := r.URI().Path()
		_ = r.Close()
		chains, lerr := pointerscan.Load(path)
		if lerr != nil {
			a.fail(lerr)
			return
		}
		a.showPointerResults(chains)
	}, a.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".ptr"}))
	d.Show()
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
		a.setStatusText(i18n.Tf("status.saved", map[string]any{"Path": path}))
	}, a.win)
	d.SetFileName("scan.ptr")
	d.Show()
}
