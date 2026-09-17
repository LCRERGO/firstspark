//go:build gui

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/cheattable"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

func (a *App) loadTable() {
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
		var tbl *cheattable.Table
		var perr error
		if strings.HasSuffix(strings.ToLower(path), ".json") {
			tbl, perr = cheattable.ImportJSON(path)
		} else {
			tbl, perr = cheattable.Load(path)
		}
		if perr != nil {
			a.fail(perr)
			return
		}
		a.applyTable(tbl)
	}, a.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".ct", ".json"}))
	d.Show()
}

func (a *App) applyTable(tbl *cheattable.Table) {
	a.entries = nil
	for _, e := range tbl.Entries {
		addr, err := e.AddressValue()
		if err != nil {
			continue
		}
		var typ scan.ValueType
		var bit *bitSpec
		if strings.EqualFold(e.Type, "bitfield") {
			bit = &bitSpec{size: e.BitSize, offset: e.BitOffset, width: e.BitWidth, signed: e.BitSigned}
			typ = typeForSize(bit.size)
		} else if parsed, terr := scan.ParseValueType(e.Type); terr == nil {
			typ = parsed
		} else {
			typ = a.defaultValueType()
		}
		entry := tableEntry{addr: addr, typ: typ, desc: e.Description, display: parseDisplay(e.Display), bit: bit}
		if pc, ok := cheattable.ParsePointerChain(e.Pointer); ok {
			upc := &pointerChain{module: pc.Module, base: pc.Base, offset: pc.Offset, offsets: pc.Offsets}
			entry.pointer = upc
			if a.proc != nil {
				if resolved, rerr := resolvePointer(a.proc, upc); rerr == nil {
					entry.addr = resolved
				}
			} else if pc.Module == "" {
				entry.addr = pc.Base
			}
		}
		var v scan.Value
		if strings.TrimSpace(e.Value) != "" {
			if parsed, perr := scan.ParseValue(typ, e.Value); perr == nil {
				v = parsed
			}
		}
		if len(v.Raw) == 0 && a.proc != nil && typ.Size() > 0 {
			if raw, rerr := a.proc.Read(entry.addr, typ.Size()); rerr == nil {
				v = scan.NewValue(typ, raw)
			}
		}
		entry.value = v
		entry.orig = v
		a.entries = append(a.entries, entry)
		if key, kerr := parseHotkey(e.Hotkey); kerr == nil {
			a.bindHotkey(key, len(a.entries)-1)
			a.entries[len(a.entries)-1].hotkey = key
		}
		if e.Frozen {
			a.mu.Lock()
			a.frozen[entry.addr] = v
			a.mu.Unlock()
		}
	}
	a.tableSel = -1
	a.table.Refresh()
	a.setStatusText(i18n.Tf("status.loaded_entries", map[string]any{"Count": len(a.entries)}))
	a.updateScanControls()
}

func (a *App) saveTable() { a.saveTableAs() }

func (a *App) saveTableAs() {
	if len(a.entries) == 0 {
		a.setStatusText(i18n.T("status.nothing_to_save"))
		return
	}
	a.saveTableDialog("firstspark.ct", a.tableFromEntries())
}

func (a *App) saveScanResults() {
	if len(a.results) == 0 {
		a.setStatusText(i18n.T("status.no_scan_results"))
		return
	}
	a.saveTableDialog("scan-results.ct", a.resultsTable())
}

func (a *App) saveTableDialog(name string, tbl *cheattable.Table) {
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
		var serr error
		if strings.HasSuffix(strings.ToLower(path), ".json") {
			serr = tbl.ExportJSON(path)
		} else {
			serr = tbl.Save(path)
		}
		if serr != nil {
			a.fail(serr)
			return
		}
		a.setStatusText(i18n.Tf("status.saved", map[string]any{"Path": path}))
	}, a.win)
	d.SetFileName(name)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".ct", ".json"}))
	d.Show()
}

func (a *App) tableFromEntries() *cheattable.Table {
	t := &cheattable.Table{}
	for _, e := range a.entries {
		t.Add(e.desc, fmt.Sprintf("0x%x", e.addr), e.typ.String(), e.value.String())
		last := &t.Entries[len(t.Entries)-1]
		if e.hotkey != "" {
			last.Hotkey = string(e.hotkey)
		}
		last.Display = displayName(e.display)
		last.Frozen = a.isFrozen(e.addr)
		if e.bit != nil {
			last.Type = "bitfield"
			last.BitSize = e.bit.size
			last.BitOffset = e.bit.offset
			last.BitWidth = e.bit.width
			last.BitSigned = e.bit.signed
		}
		if e.pointer != nil {
			last.Pointer = cheattable.FormatPointerChain(cheattable.PointerChain{
				Module:  e.pointer.module,
				Base:    e.pointer.base,
				Offset:  e.pointer.offset,
				Offsets: e.pointer.offsets,
			})
		}
	}
	return t
}

func (a *App) resultsTable() *cheattable.Table {
	t := &cheattable.Table{}
	typ := a.defaultValueType()
	if a.session != nil {
		typ = a.session.Options().Type
	}
	for _, r := range a.results {
		t.Add("", fmt.Sprintf("0x%x", r.Addr), typ.String(), r.Prev.String())
	}
	return t
}
