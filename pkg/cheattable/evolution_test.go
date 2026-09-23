package cheattable

import (
	"reflect"
	"testing"
)

// TestParseLegacyTableWithoutVersion covers a .CT file written before the
// Version attribute and the extended entry fields existed.
func TestParseLegacyTableWithoutVersion(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<CheatTable>
  <CheatEntries>
    <CheatEntry ID="1" Description="Health" Address="0x1234" Type="4 Bytes">100</CheatEntry>
  </CheatEntries>
</CheatTable>`)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tbl.Version != "" {
		t.Errorf("Version = %q, want empty", tbl.Version)
	}
	if len(tbl.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(tbl.Entries))
	}
	e := tbl.Entries[0]
	if e.Address != "0x1234" || e.Type != "4 Bytes" || e.Value != "100" {
		t.Errorf("legacy entry decoded wrong: %+v", e)
	}
	// A legacy file must survive a save/load cycle.
	out, err := tbl.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if !reflect.DeepEqual(back.Entries[0], e) {
		t.Errorf("legacy round trip changed the entry: %+v", back.Entries[0])
	}
}

// TestExtendedFieldsRoundTrip guards the v2 additions (hotkey, display,
// pointer, bitfield, encoding, frozen).
func TestExtendedFieldsRoundTrip(t *testing.T) {
	tbl := &Table{Version: SchemaVersion}
	tbl.Entries = []Entry{{
		ID: 1, Description: "hp", Address: "0x1000", Type: "4 Bytes", Value: "42",
		Hotkey: "F5", Display: "hex", Frozen: true, Encoding: "utf16le",
		Pointer: "lib.so+0x10:+0x8", BitSize: 4, BitOffset: 2, BitWidth: 3, BitSigned: true,
	}}
	data, err := tbl.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.Version != SchemaVersion {
		t.Errorf("Version = %q, want %q", back.Version, SchemaVersion)
	}
	if !reflect.DeepEqual(back.Entries[0], tbl.Entries[0]) {
		t.Errorf("extended fields changed: %+v", back.Entries[0])
	}
}

// TestAddAssignsVersionAndIDs checks the writer stamps the current schema.
func TestAddAssignsVersionAndIDs(t *testing.T) {
	tbl := &Table{}
	tbl.Add("a", "0x1", "4 Bytes", "1")
	tbl.Add("b", "0x2", "4 Bytes", "2")
	if tbl.Version != SchemaVersion {
		t.Errorf("Version = %q, want %q", tbl.Version, SchemaVersion)
	}
	if tbl.Entries[0].ID != 1 || tbl.Entries[1].ID != 2 {
		t.Errorf("ids = %d,%d, want 1,2", tbl.Entries[0].ID, tbl.Entries[1].ID)
	}
}
