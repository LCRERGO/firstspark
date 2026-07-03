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
