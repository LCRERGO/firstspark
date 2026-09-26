//go:build gui

package ui

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/pointerscan"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

func TestChainToPointer(t *testing.T) {
	c := pointerscan.Chain{Module: "lib.so", Base: 0x1000, Offsets: []uint64{0x10, 0, 0x10}}
	pc, ok := chainToPointer(c)
	if !ok {
		t.Fatal("conversion failed")
	}
	if pc.module != "lib.so" || pc.offset != 0x10 || len(pc.offsets) != 2 || pc.offsets[0] != 0 || pc.offsets[1] != 0x10 {
		t.Fatalf("module pointer = %+v", pc)
	}
	if _, ok := chainToPointer(pointerscan.Chain{}); ok {
		t.Fatal("expected an empty chain to be rejected")
	}
}

func TestChainToPointerAbsolute(t *testing.T) {
	c := pointerscan.Chain{Base: 0x1000, Offsets: []uint64{0x10, 0x20}}
	pc, ok := chainToPointer(c)
	if !ok {
		t.Fatal("conversion failed")
	}
	if pc.module != "" || pc.base != 0x1010 || len(pc.offsets) != 1 || pc.offsets[0] != 0x20 {
		t.Fatalf("absolute pointer = %+v", pc)
	}
}

func TestParseOffsets(t *testing.T) {
	pc, err := parseOffsets(0x1000, "0x10, 0x20")
	if err != nil {
		t.Fatalf("parseOffsets: %v", err)
	}
	if pc == nil || pc.base != 0x1000 || len(pc.offsets) != 2 || pc.offsets[0] != 0x10 || pc.offsets[1] != 0x20 {
		t.Fatalf("chain = %+v", pc)
	}
	if _, err := parseOffsets(0x1000, "zz"); err == nil {
		t.Fatal("expected an error for a bad offset")
	}
}

func TestParseHotkey(t *testing.T) {
	if k, err := parseHotkey("f1"); err != nil || k != "F1" {
		t.Fatalf("F1: %v, %v", k, err)
	}
	if k, err := parseHotkey("a"); err != nil || k != "A" {
		t.Fatalf("A: %v, %v", k, err)
	}
	if _, err := parseHotkey("F13"); err == nil {
		t.Fatal("expected an error for F13")
	}
}

func TestFoundCellText(t *testing.T) {
	a := newTestApp(t)
	cur := scan.NewValue(scan.TypeDword, []byte{42, 0, 0, 0})
	prev := scan.NewValue(scan.TypeDword, []byte{1, 0, 0, 0})
	a.results = []scan.Result{
		{Addr: 0x1000, Value: cur, Previous: prev},
		{Addr: 0x2000, Value: cur},
	}
	a.foundOrder = identityOrder(len(a.results))

	if got := a.foundCellText(0, 0); got != "0x1000" {
		t.Fatalf("address = %q", got)
	}
	if got := a.foundCellText(1, 0); got != cur.String() {
		t.Fatalf("value = %q, want %q", got, cur.String())
	}
	if got := a.foundCellText(2, 0); got != prev.String() {
		t.Fatalf("previous = %q, want %q", got, prev.String())
	}
	if got := a.foundCellText(2, 1); got != "-" {
		t.Fatalf("missing previous = %q, want -", got)
	}

	live := scan.NewValue(scan.TypeDword, []byte{9, 0, 0, 0})
	a.foundLive = map[int]scan.Value{0: live}
	if got := a.foundCellText(1, 0); got != live.String() {
		t.Fatalf("live value = %q, want %q", got, live.String())
	}
}

func TestStaticInfo(t *testing.T) {
	a := newTestApp(t)
	a.foundRegions = []mem.Region{
		{Start: 0x400000, End: 0x500000, Perms: "r-xp", Offset: 0, Path: "/usr/bin/game"},
		{Start: 0x7f0000000000, End: 0x7f0000001000, Perms: "rw-p", Offset: 0},
	}
	name, off, ok := a.staticInfo(0x401234)
	if !ok || name != "game" || off != 0x1234 {
		t.Fatalf("staticInfo = %q, %#x, %v", name, off, ok)
	}
	if _, _, ok := a.staticInfo(0x7f0000000000); ok {
		t.Fatal("anonymous region should not be static")
	}
	if _, _, ok := a.staticInfo(0x999999); ok {
		t.Fatal("unmapped address should not be static")
	}
}

func TestFoundSort(t *testing.T) {
	a := newTestApp(t)
	a.results = []scan.Result{{Addr: 0x30}, {Addr: 0x10}, {Addr: 0x20}}
	a.applyFoundSort()

	a.sortFound(0)
	if a.foundOrder[0] != 1 || a.foundOrder[1] != 2 || a.foundOrder[2] != 0 {
		t.Fatalf("ascending order = %v", a.foundOrder)
	}
	a.sortFound(0)
	if a.foundOrder[0] != 0 || a.foundOrder[2] != 1 {
		t.Fatalf("descending order = %v", a.foundOrder)
	}
	a.sortFound(0)
	if a.foundOrder[0] != 0 || a.foundOrder[1] != 1 || a.foundOrder[2] != 2 {
		t.Fatalf("cleared order = %v", a.foundOrder)
	}
}

func TestFoundDisplayFormat(t *testing.T) {
	a := newTestApp(t)
	v := scan.NewValue(scan.TypeDword, []byte{0x2A, 0, 0, 0})
	if got := a.displayFoundValue(v); got != v.String() {
		t.Fatalf("decimal = %q", got)
	}
	a.setFoundDisplay(displayHex)
	if got := a.displayFoundValue(v); got != hexOf(v) {
		t.Fatalf("hex = %q, want %q", got, hexOf(v))
	}
}

func TestEntrySignedUnsigned(t *testing.T) {
	a := newTestApp(t)
	e := &tableEntry{typ: scan.TypeDword, value: scan.NewValue(scan.TypeDword, []byte{0xFF, 0xFF, 0xFF, 0xFF})}
	if got := a.formatEntryValue(e); got != "-1" {
		t.Fatalf("signed = %q, want -1", got)
	}
	e.unsigned = true
	if got := a.formatEntryValue(e); got != "4294967295" {
		t.Fatalf("unsigned = %q", got)
	}
}

func TestChangeEntryTypeClearsBitfield(t *testing.T) {
	a := newTestApp(t)
	a.entryRoots = []*tableEntry{{addr: 0x10, typ: scan.TypeDword, value: scan.NewValue(scan.TypeDword, []byte{1, 0, 0, 0}), bit: &bitSpec{size: 4, width: 4}}}
	a.rebuildVisible()
	a.changeEntryType(0, scan.TypeQword)
	if a.entryRoots[0].typ != scan.TypeQword || a.entryRoots[0].bit != nil {
		t.Fatalf("entry after type change = %+v", a.entryRoots[0])
	}
}

func TestMoveEntry(t *testing.T) {
	a := newTestApp(t)
	a.entryRoots = []*tableEntry{{addr: 1}, {addr: 2}, {addr: 3}}
	a.rebuildVisible()
	a.moveEntry(0, false)
	if a.entryRoots[0].addr != 2 || a.entryRoots[1].addr != 1 {
		t.Fatalf("after down = %d,%d", a.entryRoots[0].addr, a.entryRoots[1].addr)
	}
	a.moveEntryEdge(2, true)
	if a.entryRoots[0].addr != 3 {
		t.Fatalf("after top = %d", a.entryRoots[0].addr)
	}
}

func TestGroupSelection(t *testing.T) {
	a := newTestApp(t)
	a.entryRoots = []*tableEntry{{addr: 1}, {addr: 2}, {addr: 3}}
	a.rebuildVisible()
	a.tableMulti = map[*tableEntry]bool{a.entryRoots[0]: true, a.entryRoots[1]: true}
	a.groupSelection()
	if len(a.entryRoots) != 2 {
		t.Fatalf("roots = %d, want 2", len(a.entryRoots))
	}
	if !a.entryRoots[0].group || len(a.entryRoots[0].children) != 2 {
		t.Fatalf("group = %+v", a.entryRoots[0])
	}
	if a.entryRoots[1].addr != 3 {
		t.Fatalf("remaining root = %d", a.entryRoots[1].addr)
	}
}

func TestCloneEntry(t *testing.T) {
	e := &tableEntry{addr: 7, desc: "x", pointer: &pointerChain{base: 1, offsets: []int64{2, 3}}, children: []*tableEntry{{addr: 8}}}
	c := cloneEntry(e)
	if c == e || c.addr != 7 || c.pointer == e.pointer || len(c.children) != 1 {
		t.Fatalf("clone = %+v", c)
	}
	c.pointer.offsets[0] = 9
	if e.pointer.offsets[0] != 2 {
		t.Fatal("clone shares pointer offsets")
	}
}
