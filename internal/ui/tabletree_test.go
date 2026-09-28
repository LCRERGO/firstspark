//go:build gui

package ui

import (
	"testing"

	"fyne.io/fyne/v2"

	"github.com/LCRERGO/firstspark/pkg/cheattable"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

func TestFoundMultiSelect(t *testing.T) {
	a := newTestApp(t)
	tab := a.tab()
	tab.results = []scan.Result{{Addr: 0x1}, {Addr: 0x2}, {Addr: 0x3}}

	tab.selectFoundRow(0, 0)
	if got := tab.selectedFoundIndices(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("plain select = %v", got)
	}
	tab.selectFoundRow(2, fyne.KeyModifierShift)
	if got := tab.selectedFoundIndices(); len(got) != 3 {
		t.Fatalf("shift select = %v", got)
	}
	tab.selectFoundRow(1, fyne.KeyModifierControl)
	if got := tab.selectedFoundIndices(); len(got) != 2 {
		t.Fatalf("ctrl toggle = %v", got)
	}
	tab.selectFoundRow(1, 0)
	if got := tab.selectedFoundIndices(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("plain reselect = %v", got)
	}
}

func TestTableMultiSelect(t *testing.T) {
	a := newTestApp(t)
	a.entryRoots = []*tableEntry{{desc: "a"}, {desc: "b"}, {desc: "c"}}
	a.rebuildVisible()

	a.selectTableRow(0, 0)
	if got := a.selectedTableEntries(); len(got) != 1 || got[0].desc != "a" {
		t.Fatalf("plain select = %v", got)
	}
	a.selectTableRow(2, fyne.KeyModifierShift)
	if got := a.selectedTableEntries(); len(got) != 3 {
		t.Fatalf("shift select = %d entries", len(got))
	}
	a.selectTableRow(1, fyne.KeyModifierControl)
	if got := a.selectedTableEntries(); len(got) != 2 {
		t.Fatalf("ctrl toggle = %d entries", len(got))
	}
	a.selectTableRow(1, 0)
	if got := a.selectedTableEntries(); len(got) != 1 || got[0].desc != "b" {
		t.Fatalf("plain reselect = %v", got)
	}
}

func TestTreeProjectionAndCollapse(t *testing.T) {
	a := newTestApp(t)
	child := &tableEntry{desc: "child"}
	group := &tableEntry{desc: "group", group: true, expanded: true, children: []*tableEntry{child}}
	leaf := &tableEntry{desc: "leaf"}
	a.entryRoots = []*tableEntry{group, leaf}
	a.rebuildVisible()

	if len(a.entries) != 3 {
		t.Fatalf("expanded projection = %d, want 3", len(a.entries))
	}
	if group.depth != 0 || child.depth != 1 || leaf.depth != 0 {
		t.Fatalf("depths = %d,%d,%d", group.depth, child.depth, leaf.depth)
	}

	a.toggleExpand(0)
	if len(a.entries) != 2 || a.entries[1] != leaf {
		t.Fatalf("collapsed projection = %+v", a.entries)
	}

	a.removeEntry(group)
	if len(a.entryRoots) != 1 || len(a.entries) != 1 || a.entries[0] != leaf {
		t.Fatalf("after remove: roots=%d visible=%d", len(a.entryRoots), len(a.entries))
	}
}

func TestResolveExpression(t *testing.T) {
	a := newTestApp(t)
	r := symbolResolver{symbols: map[string]uint64{"base": 0x5000}}

	parent := &tableEntry{desc: "p", addr: 0x1000}
	child := &tableEntry{desc: "c", expr: "+18", parent: parent}
	if got, err := a.resolveExpression(child, r); err != nil || got != 0x1018 {
		t.Fatalf("parent-relative = %#x, %v", got, err)
	}

	named := &tableEntry{desc: "n", expr: "base+10"}
	if got, err := a.resolveExpression(named, r); err != nil || got != 0x5010 {
		t.Fatalf("symbol expression = %#x, %v", got, err)
	}
}

func TestEntryOffsetsRoundTrip(t *testing.T) {
	tbl := &cheattable.Table{}
	tbl.Entries = append(tbl.Entries, cheattable.Entry{Description: "g", Group: true, Expr: "pChar", Offsets: "18,-4"})
	data, err := tbl.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := cheattable.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := back.Entries[0]
	if e.Expr != "pChar" || e.Offsets != "18,-4" {
		t.Fatalf("expr/offsets lost: %+v", e)
	}
}

func TestGroupFreezeCascades(t *testing.T) {
	a := newTestApp(t)
	child := &tableEntry{desc: "child", addr: 0x1000, frozen: true}
	group := &tableEntry{desc: "group", group: true, expanded: true, frozen: true, children: []*tableEntry{child}}
	a.entryRoots = []*tableEntry{group}
	a.rebuildVisible()

	a.toggleFreezeEntry(group)
	if group.frozen || child.frozen {
		t.Fatalf("unfreeze did not cascade: group=%v child=%v", group.frozen, child.frozen)
	}
}
