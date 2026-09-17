package cheattable

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	tbl := &Table{}
	tbl.Add("health", "0x7ffe1234", "4 Bytes", "100")
	tbl.Add("gold", "0x7ffe5678", "4 Bytes", "999")

	dir := t.TempDir()
	ct := filepath.Join(dir, "table.CT")
	if err := tbl.Save(ct); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(ct)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(loaded.Entries))
	}
	addr, err := loaded.Entries[0].AddressValue()
	if err != nil {
		t.Fatalf("AddressValue: %v", err)
	}
	if addr != 0x7ffe1234 {
		t.Errorf("addr = %#x", addr)
	}
	if loaded.Entries[1].Value != "999" {
		t.Errorf("value = %q", loaded.Entries[1].Value)
	}
}

func TestPointerChainFormat(t *testing.T) {
	cases := []PointerChain{
		{Module: "libc.so.6", Offset: 0x1234, Offsets: []int64{0x10, -0x8}},
		{Base: 0x7ffe1000, Offsets: []int64{0x20}},
		{Base: 0x400000},
	}
	for _, want := range cases {
		got, ok := ParsePointerChain(FormatPointerChain(want))
		if !ok {
			t.Fatalf("ParsePointerChain(%q) failed", FormatPointerChain(want))
		}
		if got.Module != want.Module || got.Base != want.Base || got.Offset != want.Offset || len(got.Offsets) != len(want.Offsets) {
			t.Fatalf("round trip = %+v, want %+v", got, want)
		}
		for i := range want.Offsets {
			if got.Offsets[i] != want.Offsets[i] {
				t.Fatalf("offsets = %v, want %v", got.Offsets, want.Offsets)
			}
		}
	}
}

func TestExtendedAttributesRoundTrip(t *testing.T) {
	tbl := &Table{}
	tbl.Add("hp", "0x1000", "4 Bytes", "42")
	tbl.Entries[0].Hotkey = "F5"
	tbl.Entries[0].Display = "hex"
	tbl.Entries[0].Frozen = true
	tbl.Entries[0].Pointer = FormatPointerChain(PointerChain{Module: "libc.so.6", Offset: 0x10, Offsets: []int64{0x20}})

	path := filepath.Join(t.TempDir(), "t.json")
	if err := tbl.ExportJSON(path); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	loaded, err := ImportJSON(path)
	if err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}
	e := loaded.Entries[0]
	if e.Hotkey != "F5" || e.Display != "hex" || !e.Frozen || e.Pointer == "" {
		t.Fatalf("extended attributes lost: %+v", e)
	}
	if loaded.Version == "" {
		t.Fatal("schema version not written")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	tbl := &Table{}
	tbl.Add("health", "0x1000", "4 Bytes", "42")
	path := filepath.Join(t.TempDir(), "session.json")
	if err := tbl.ExportJSON(path); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	loaded, err := ImportJSON(path)
	if err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].Description != "health" {
		t.Errorf("unexpected round trip: %+v", loaded)
	}
}
