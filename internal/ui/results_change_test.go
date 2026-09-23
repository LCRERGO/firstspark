//go:build gui

package ui

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

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

func TestReplaceValueIdent(t *testing.T) {
	if got := replaceValueIdent("oldvalue", "value", "9"); got != "oldvalue" {
		t.Fatalf("substring replaced: %q", got)
	}
	if got := replaceValueIdent("value + value", "value", "9"); got != "9 + 9" {
		t.Fatalf("replace = %q", got)
	}
}
