//go:build gui

package ui

import "fyne.io/fyne/v2"

// selectTableRow updates the cheat-table selection for a click with modifiers:
// plain selects one row, Ctrl toggles, Shift selects a contiguous range.
func (a *App) selectTableRow(row int, mod fyne.KeyModifier) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	e := a.entries[row]
	switch {
	case mod&fyne.KeyModifierControl != 0:
		if a.tableMulti == nil {
			a.tableMulti = map[*tableEntry]bool{}
			if a.tableSel >= 0 && a.tableSel < len(a.entries) {
				a.tableMulti[a.entries[a.tableSel]] = true
			}
		}
		if a.tableMulti[e] {
			delete(a.tableMulti, e)
		} else {
			a.tableMulti[e] = true
		}
		a.tableSel = row
	case mod&fyne.KeyModifierShift != 0 && a.tableSel >= 0 && a.tableSel < len(a.entries):
		lo, hi := a.tableSel, row
		if lo > hi {
			lo, hi = hi, lo
		}
		m := map[*tableEntry]bool{}
		for i := lo; i <= hi; i++ {
			m[a.entries[i]] = true
		}
		a.tableMulti = m
	default:
		a.tableMulti = nil
		a.tableSel = row
	}
	if a.table != nil {
		a.table.Refresh()
	}
}

// isTableSelected reports whether e is in the current selection.
func (a *App) isTableSelected(e *tableEntry) bool {
	if a.tableMulti != nil && a.tableMulti[e] {
		return true
	}
	return a.tableSel >= 0 && a.tableSel < len(a.entries) && a.entries[a.tableSel] == e
}

// selectedTableEntries returns the selected entries in visible order.
func (a *App) selectedTableEntries() []*tableEntry {
	var out []*tableEntry
	if a.tableMulti != nil {
		for _, e := range a.entries {
			if a.tableMulti[e] {
				out = append(out, e)
			}
		}
	}
	if len(out) == 0 && a.tableSel >= 0 && a.tableSel < len(a.entries) {
		out = append(out, a.entries[a.tableSel])
	}
	return out
}

// rowOf returns the visible row index of e, or -1.
func (a *App) rowOf(e *tableEntry) int {
	for i, x := range a.entries {
		if x == e {
			return i
		}
	}
	return -1
}

// rebuildVisible recomputes a.entries as the pre-order projection of the entry
// tree, hiding the children of collapsed groups and recording each visible
// node's depth for indentation. a.table row indices are indices into a.entries.
func (a *App) rebuildVisible() {
	vis := a.entries[:0]
	if vis == nil {
		vis = make([]*tableEntry, 0, len(a.entryRoots))
	}
	var walk func(nodes []*tableEntry, depth int, parent *tableEntry)
	walk = func(nodes []*tableEntry, depth int, parent *tableEntry) {
		for _, e := range nodes {
			e.parent = parent
			e.depth = depth
			vis = append(vis, e)
			if e.group && !e.expanded {
				continue
			}
			walk(e.children, depth+1, e)
		}
	}
	walk(a.entryRoots, 0, nil)
	a.entries = vis
	if a.tableSel >= len(a.entries) {
		a.tableSel = -1
	}
}

// walkEntries visits every node in the tree, visible or not.
func (a *App) walkEntries(fn func(*tableEntry)) {
	var walk func(nodes []*tableEntry)
	walk = func(nodes []*tableEntry) {
		for _, e := range nodes {
			fn(e)
			walk(e.children)
		}
	}
	walk(a.entryRoots)
}

// addRoot appends a top-level entry and rebuilds the visible projection.
func (a *App) addRoot(e *tableEntry) {
	a.entryRoots = append(a.entryRoots, e)
	a.rebuildVisible()
}

// removeEntry detaches e (and its subtree) from the tree, unbinds the hotkeys
// and rebuilds the projection.
func (a *App) removeEntry(e *tableEntry) {
	if e.parent == nil {
		for i, r := range a.entryRoots {
			if r == e {
				a.entryRoots = append(a.entryRoots[:i], a.entryRoots[i+1:]...)
				break
			}
		}
	} else {
		p := e.parent
		for i, c := range p.children {
			if c == e {
				p.children = append(p.children[:i], p.children[i+1:]...)
				break
			}
		}
	}
	a.removeSubtreeHooks(e)
	a.rebuildVisible()
}

func (a *App) removeSubtreeHooks(e *tableEntry) {
	a.unbindHotkey(e.addr)
	for _, c := range e.children {
		a.removeSubtreeHooks(c)
	}
}

// toggleExpand opens or closes a group row.
func (a *App) toggleExpand(row int) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	e := a.entries[row]
	if !e.group {
		return
	}
	e.expanded = !e.expanded
	a.rebuildVisible()
	a.table.Refresh()
}

// resetTree clears the whole cheat table.
func (a *App) resetTree() {
	a.entryRoots = nil
	a.entries = nil
	a.tableSel = -1
	a.meta = tableMeta{}
}
