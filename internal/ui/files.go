//go:build gui

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

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
	d.Resize(fyne.NewSize(720, 520))
	d.Show()
}

func (a *App) applyTable(tbl *cheattable.Table) {
	a.entries = nil
	for _, e := range tbl.Entries {
		addr, err := e.AddressValue()
		if err != nil {
			continue
		}
		typ, err := scan.ParseValueType(e.Type)
		if err != nil {
			typ = a.defaultValueType()
		}
		var v scan.Value
		if strings.TrimSpace(e.Value) != "" {
			if parsed, perr := scan.ParseValue(typ, e.Value); perr == nil {
				v = parsed
			}
		}
		if len(v.Raw) == 0 && a.proc != nil && typ.Size() > 0 {
			if raw, rerr := a.proc.Read(addr, typ.Size()); rerr == nil {
				v = scan.NewValue(typ, raw)
			}
		}
		a.entries = append(a.entries, tableEntry{addr: addr, typ: typ, desc: e.Description, value: v, orig: v})
	}
	a.tableSel = -1
	a.table.Refresh()
	a.setStatus("loaded %d entries", len(a.entries))
}

func (a *App) saveTable() { a.saveTableAs() }

func (a *App) saveTableAs() {
	if len(a.entries) == 0 {
		a.setStatus("nothing to save")
		return
	}
	a.saveTableDialog("firstspark.ct", a.tableFromEntries())
}

func (a *App) saveScanResults() {
	if len(a.results) == 0 {
		a.setStatus("no scan results to save")
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
		a.setStatus("saved %s", path)
	}, a.win)
	d.SetFileName(name)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".ct", ".json"}))
	d.Resize(fyne.NewSize(720, 520))
	d.Show()
}

func (a *App) tableFromEntries() *cheattable.Table {
	t := &cheattable.Table{}
	for _, e := range a.entries {
		t.Add(e.desc, fmt.Sprintf("0x%x", e.addr), e.typ.String(), e.value.String())
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
