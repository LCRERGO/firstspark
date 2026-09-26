package cheattable

import (
	"path/filepath"
	"testing"
)

func TestFormatFor(t *testing.T) {
	cases := map[string]Format{
		"table.CT":    FormatCT,
		"table.ct":    FormatCT,
		"table.json":  FormatJSON,
		"table.yaml":  FormatYAML,
		"table.YML":   FormatYAML,
		"table.txt":   FormatCT,
		"noextension": FormatCT,
	}
	for path, want := range cases {
		if got := FormatFor(path); got != want {
			t.Errorf("FormatFor(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	tbl := &Table{Version: SchemaVersion, Entries: []Entry{
		{ID: 1, Description: "grp", Group: true, Children: []Entry{
			{ID: 2, Description: "hp", Address: "0x1000", Type: "dword", Value: "42", ShowAsSigned: true},
		}},
		{ID: 3, Description: "ptr", Type: "dword", Pointer: FormatPointerChain(PointerChain{
			Module: "game", Offset: 0x10, Offsets: []int64{0x8},
		})},
	}}
	data, err := tbl.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	back, err := ParseYAML(data)
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if len(back.Entries) != 2 || !back.Entries[0].Group || len(back.Entries[0].Children) != 1 {
		t.Fatalf("tree lost: %+v", back.Entries)
	}
	if !back.Entries[0].Children[0].ShowAsSigned {
		t.Fatal("ShowAsSigned lost")
	}
	if back.Entries[1].Pointer != tbl.Entries[1].Pointer {
		t.Fatalf("pointer lost: %q", back.Entries[1].Pointer)
	}
}

func TestSaveAsAndLoadAny(t *testing.T) {
	dir := t.TempDir()
	tbl := &Table{Version: SchemaVersion}
	tbl.Add("hp", "0x1000", "dword", "42")

	for _, name := range []string{"t.ct", "t.json", "t.yaml"} {
		path := filepath.Join(dir, name)
		if err := tbl.SaveAs(path); err != nil {
			t.Fatalf("SaveAs(%s): %v", name, err)
		}
		back, err := LoadAny(path)
		if err != nil {
			t.Fatalf("LoadAny(%s): %v", name, err)
		}
		if len(back.Entries) != 1 || back.Entries[0].Description != "hp" {
			t.Fatalf("LoadAny(%s) = %+v", name, back.Entries)
		}
	}
}
