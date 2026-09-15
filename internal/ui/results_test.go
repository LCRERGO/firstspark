//go:build gui

package ui

import "testing"

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
