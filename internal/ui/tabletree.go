//go:build gui

package ui

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
}
