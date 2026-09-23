//go:build gui

package ui

import (
	"encoding/binary"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

var childrenBuf = make([]byte, 8)

func dword(t *testing.T, s string) scan.Value {
	t.Helper()
	v, err := scan.ParseValue(scan.TypeDword, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func TestExpandValueInput(t *testing.T) {
	a := newTestApp(t)
	ref := &tableEntry{desc: "gold", typ: scan.TypeDword, value: dword(t, "500")}
	cur := &tableEntry{desc: "hp", typ: scan.TypeDword, value: dword(t, "100"), display: displayHex}
	a.entryRoots = []*tableEntry{ref, cur}
	a.rebuildVisible()

	if got := a.expandValueInput("(gold)+1", cur); got != "500+1" {
		t.Fatalf("(gold)+1 = %q", got)
	}
	if got := a.expandValueInput("oldvalue+10", cur); got != "100+10" {
		t.Fatalf("oldvalue+10 = %q", got)
	}
	if got := a.expandValueInput("FF", cur); got != "0xFF" {
		t.Fatalf("bare hex = %q", got)
	}
	if got := a.expandValueInput("(missing)+1", cur); got != "(missing)+1" {
		t.Fatalf("missing ref = %q", got)
	}
}

func TestSetSubtreeValue(t *testing.T) {
	p, err := mem.Find(os.Getpid())
	if err != nil {
		t.Skipf("cannot open self: %v", err)
	}
	a := newTestApp(t)
	a.proc = p
	base := uint64(uintptr(unsafe.Pointer(&childrenBuf[0])))
	c1 := &tableEntry{desc: "a", typ: scan.TypeDword, addr: base}
	c2 := &tableEntry{desc: "b", typ: scan.TypeDword, addr: base + 4}
	grp := &tableEntry{group: true, children: []*tableEntry{c1, c2}}

	w, f := a.setSubtreeValue(grp, "7")
	if w != 2 || f != 0 {
		t.Fatalf("written=%d failed=%d", w, f)
	}
	if binary.LittleEndian.Uint32(childrenBuf[0:]) != 7 || binary.LittleEndian.Uint32(childrenBuf[4:]) != 7 {
		t.Fatalf("buffer = %v", childrenBuf)
	}
	runtime.KeepAlive(childrenBuf)
}

func TestReplaceValueIdent(t *testing.T) {
	if got := replaceValueIdent("oldvalue", "value", "9"); got != "oldvalue" {
		t.Fatalf("substring replaced: %q", got)
	}
	if got := replaceValueIdent("value + value", "value", "9"); got != "9 + 9" {
		t.Fatalf("replace = %q", got)
	}
}
