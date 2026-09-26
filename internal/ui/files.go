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
	"github.com/LCRERGO/firstspark/pkg/customtype"
	"github.com/LCRERGO/firstspark/pkg/log"
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
		for _, ct := range tbl.CustomTypes {
			if _, ok := scan.LookupType(ct.Name); ok {
				continue
			}
			if _, err := customtype.RegisterRaw(ct.Name, ct.Size); err != nil {
				log.Warn("custom type import failed", "name", ct.Name, "err", err)
			}
		}
		a.applyTable(tbl)
		a.reportCEImport(tbl.Stats)
	}, a.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".ct", ".json"}))
	d.Show()
}

// reportCEImport surfaces entries that a Cheat Engine conversion could not
// represent, so an unsupported table is never imported silently.
func (a *App) reportCEImport(stats cheattable.ImportStats) {
	if stats.Skipped == 0 {
		return
	}
	a.setStatusText(i18n.Tf("status.ce_imported", map[string]any{"Imported": stats.Imported, "Skipped": stats.Skipped}))
	if stats.Imported == 0 {
		dialog.ShowInformation(i18n.T("dialog.ce_import_title"),
			i18n.Tf("dialog.ce_import_body", map[string]any{"Skipped": stats.Skipped}), a.win)
	}
}

func (a *App) applyTable(tbl *cheattable.Table) {
	a.resetTree()
	for i := range tbl.Entries {
		a.entryRoots = append(a.entryRoots, a.entryFromStored(&tbl.Entries[i]))
	}
	a.rebuildVisible()
	a.syncFreezeTargets()
	a.table.Refresh()
	total := 0
	a.walkEntries(func(*tableEntry) { total++ })
	a.setStatusText(i18n.Tf("status.loaded_entries", map[string]any{"Count": total}))
	a.updateScanControls()
}

// entryFromStored converts a stored (possibly nested) entry into a tree node.
func (a *App) entryFromStored(e *cheattable.Entry) *tableEntry {
	node := &tableEntry{
		desc:     e.Description,
		group:    e.Group,
		expr:     e.Expr,
		script:   e.Script,
		display:  parseDisplay(e.Display),
		unsigned: !e.ShowAsSigned,
	}
	node.exprOffsets = splitOffsets(e.Offsets)
	node.typ, node.bit = a.storedType(e)
	if !node.group && node.expr == "" {
		if addr, err := e.AddressValue(); err == nil {
			node.addr = addr
		}
	}
	a.applyStoredPointer(node, e)
	if !node.group {
		a.loadStoredValue(node, e, node.typ)
	}
	for j := range e.Children {
		child := a.entryFromStored(&e.Children[j])
		child.parent = node
		node.children = append(node.children, child)
	}
	return node
}

// splitOffsets parses the comma-separated Cheat Engine offset list.
func splitOffsets(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// storedType resolves the entry's value type, handling the bitfield pseudo-type.
func (a *App) storedType(e *cheattable.Entry) (scan.ValueType, *bitSpec) {
	if strings.EqualFold(e.Type, "bitfield") {
		bit := &bitSpec{size: e.BitSize, offset: e.BitOffset, width: e.BitWidth, signed: e.BitSigned}
		return typeForSize(bit.size), bit
	}
	if parsed, err := scan.ParseValueType(e.Type); err == nil {
		return parsed, nil
	}
	return a.defaultValueType(), nil
}

// applyStoredPointer resolves a stored pointer chain into node.addr.
func (a *App) applyStoredPointer(node *tableEntry, e *cheattable.Entry) {
	pc, ok := cheattable.ParsePointerChain(e.Pointer)
	if !ok {
		return
	}
	upc := &pointerChain{module: pc.Module, base: pc.Base, offset: pc.Offset, offsets: pc.Offsets}
	node.pointer = upc
	if a.proc != nil {
		if resolved, err := resolvePointer(a.proc, upc); err == nil {
			node.addr = resolved
		}
	} else if pc.Module == "" {
		node.addr = pc.Base
	}
}

// loadStoredValue restores the value, frozen state and hotkey of a leaf.
func (a *App) loadStoredValue(node *tableEntry, e *cheattable.Entry, typ scan.ValueType) {
	var v scan.Value
	if strings.TrimSpace(e.Value) != "" {
		if parsed, err := scan.ParseValue(typ, e.Value); err == nil {
			v = parsed
		}
	}
	if len(v.Raw) == 0 && a.proc != nil && node.expr == "" && typ.Size() > 0 {
		if raw, err := a.proc.Read(node.addr, typ.Size()); err == nil {
			v = scan.NewValue(typ, raw)
		}
	}
	node.value = v
	node.orig = v
	if e.Frozen {
		node.frozen = true
		node.frozenValue = v
	}
	if key, err := parseHotkey(e.Hotkey); err == nil && node.addr != 0 {
		node.hotkey = key
		a.bindHotkey(key, node.addr)
	}
}

func (a *App) saveTable() { a.saveTableAs() }

// saveTableAsCE exports the cheat table as a Cheat Engine .CT document.
func (a *App) saveTableAsCE() {
	if len(a.entryRoots) == 0 {
		a.setStatusText(i18n.T("status.nothing_to_save"))
		return
	}
	tbl := a.tableFromEntries()
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
		if err := tbl.ExportCE(path); err != nil {
			a.fail(err)
			return
		}
		a.setStatusText(i18n.Tf("status.saved", map[string]any{"Path": path}))
	}, a.win)
	d.SetFileName("firstspark.CT")
	d.SetFilter(storage.NewExtensionFileFilter([]string{".ct"}))
	d.Show()
}

func (a *App) saveTableAs() {
	if len(a.entryRoots) == 0 {
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
	t := &cheattable.Table{Version: cheattable.SchemaVersion}
	for _, root := range a.entryRoots {
		t.Entries = append(t.Entries, *entryToStored(root))
	}
	return t
}

// entryToStored serialises a tree node and its children.
func entryToStored(e *tableEntry) *cheattable.Entry {
	out := &cheattable.Entry{Description: e.desc, Group: e.group, Expr: e.expr, Script: e.script}
	if len(e.exprOffsets) > 0 {
		out.Offsets = strings.Join(e.exprOffsets, ",")
	}
	if !e.group {
		v := e.value
		if e.frozen {
			v = e.frozenValue
		}
		out.Address = fmt.Sprintf("0x%x", e.addr)
		out.Type = e.typ.String()
		out.Value = v.String()
		if e.hotkey != "" {
			out.Hotkey = string(e.hotkey)
		}
		out.Display = displayName(e.display)
		out.ShowAsSigned = !e.unsigned
		out.Frozen = e.frozen
		if e.bit != nil {
			out.Type = "bitfield"
			out.BitSize = e.bit.size
			out.BitOffset = e.bit.offset
			out.BitWidth = e.bit.width
			out.BitSigned = e.bit.signed
		}
		if e.pointer != nil {
			out.Pointer = cheattable.FormatPointerChain(cheattable.PointerChain{
				Module:  e.pointer.module,
				Base:    e.pointer.base,
				Offset:  e.pointer.offset,
				Offsets: e.pointer.offsets,
			})
		}
	}
	for _, c := range e.children {
		out.Children = append(out.Children, *entryToStored(c))
	}
	return out
}

func (a *App) resultsTable() *cheattable.Table {
	t := &cheattable.Table{}
	typ := a.defaultValueType()
	if a.session != nil {
		typ = a.session.Options().Type
	}
	for _, r := range a.results {
		t.Add("", fmt.Sprintf("0x%x", r.Addr), typ.String(), r.Value.String())
	}
	return t
}
