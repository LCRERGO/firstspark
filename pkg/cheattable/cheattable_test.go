package cheattable

import (
	"path/filepath"
	"testing"
)

func TestParseCheatEngineTable(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<CheatTable CheatEngineTableVersion="45">
  <CheatEntries>
    <CheatEntry>
      <ID>10</ID>
      <Description>"health"</Description>
      <VariableType>4 Bytes</VariableType>
      <Address>7FF6ABCD</Address>
    </CheatEntry>
    <CheatEntry>
      <ID>11</ID>
      <Description>"player"</Description>
      <VariableType>4 Bytes</VariableType>
      <Address>ck3.exe+1A2B</Address>
      <Offsets>
        <Offset>+18</Offset>
        <Offset>-4</Offset>
      </Offsets>
    </CheatEntry>
    <CheatEntry>
      <ID>12</ID>
      <Description>"name"</Description>
      <VariableType>String</VariableType>
      <Address>7FF60000</Address>
      <Length>16</Length>
      <Unicode>1</Unicode>
    </CheatEntry>
    <CheatEntry>
      <ID>13</ID>
      <Description>"script"</Description>
      <VariableType>Auto Assembler Script</VariableType>
      <AssemblerScript>[ENABLE]</AssemblerScript>
      <CheatEntries>
        <CheatEntry>
          <ID>14</ID>
          <Description>"relative child"</Description>
          <VariableType>4 Bytes</VariableType>
          <Address>+20</Address>
        </CheatEntry>
      </CheatEntries>
    </CheatEntry>
    <CheatEntry>
      <ID>15</ID>
      <Description>"group"</Description>
      <GroupHeader>1</GroupHeader>
      <Address>pSelectedCharacter</Address>
    </CheatEntry>
  </CheatEntries>
</CheatTable>`)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tbl.Stats.Imported != 6 || tbl.Stats.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d reasons=%v", tbl.Stats.Imported, tbl.Stats.Skipped, tbl.Stats.Reasons)
	}
	// The script is preserved as a group with its source; its relative child
	// nests under it.
	script := findEntry(tbl.Entries, "script")
	if script == nil || !script.Group || script.Script != "[ENABLE]" {
		t.Fatalf("script = %+v", script)
	}
	child := findEntryDeep(tbl.Entries, "relative child")
	if child == nil || child.Expr != "+20" || child.Address != "" {
		t.Fatalf("relative child = %+v", child)
	}
	group := findEntry(tbl.Entries, "group")
	if group == nil || !group.Group || group.Expr != "pSelectedCharacter" {
		t.Fatalf("group = %+v", group)
	}
	health := findEntry(tbl.Entries, "health")
	if health == nil || health.Type != "dword" || health.Address != "0x7ff6abcd" {
		t.Fatalf("health = %+v", health)
	}
	player := findEntry(tbl.Entries, "player")
	if player == nil || player.Address != "0x0" {
		t.Fatalf("player = %+v", player)
	}
	if pc, ok := ParsePointerChain(player.Pointer); !ok || pc.Module != "ck3.exe" || len(pc.Offsets) != 2 {
		t.Fatalf("pointer chain = %+v ok=%v", pc, ok)
	}
	name := findEntry(tbl.Entries, "name")
	if name == nil || name.Type != "utf16le" {
		t.Fatalf("unicode string = %+v", name)
	}
}

func findEntry(entries []Entry, desc string) *Entry {
	for i := range entries {
		if entries[i].Description == desc {
			return &entries[i]
		}
	}
	return nil
}

func findEntryDeep(entries []Entry, desc string) *Entry {
	if e := findEntry(entries, desc); e != nil {
		return e
	}
	for i := range entries {
		if e := findEntryDeep(entries[i].Children, desc); e != nil {
			return e
		}
	}
	return nil
}

func TestExportCERoundTrip(t *testing.T) {
	tbl := &Table{Version: SchemaVersion}
	tbl.Entries = []Entry{
		{ID: 1, Description: "grp", Group: true, Children: []Entry{
			{ID: 2, Description: "hp", Address: "0x1000", Type: "dword"},
		}},
		{ID: 3, Description: "ptr", Type: "dword", Pointer: FormatPointerChain(PointerChain{
			Module: "ck3.exe", Offset: 0x1a2b, Offsets: []int64{0x18, -4},
		})},
		{ID: 4, Description: "script", Group: true, Script: "[ENABLE]\nnop\n"},
		{ID: 5, Description: "unresolved", Type: "dword", Expr: "+18", Offsets: "18,-4"},
		{ID: 6, Description: "custom", Type: "mystruct", Address: "0x2000"},
	}
	data, err := tbl.MarshalCE()
	if err != nil {
		t.Fatalf("MarshalCE: %v", err)
	}
	back, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.Stats.Imported != 6 || back.Stats.Skipped != 0 {
		t.Fatalf("imported=%d skipped=%d (%v)", back.Stats.Imported, back.Stats.Skipped, back.Stats.Reasons)
	}
	grp := findEntry(back.Entries, "grp")
	if grp == nil || !grp.Group || len(grp.Children) != 1 {
		t.Fatalf("group lost: %+v", grp)
	}
	ptr := findEntry(back.Entries, "ptr")
	if pc, ok := ParsePointerChain(ptr.Pointer); !ok || pc.Module != "ck3.exe" || len(pc.Offsets) != 2 {
		t.Fatalf("pointer lost: %+v", ptr)
	}
	script := findEntry(back.Entries, "script")
	if script == nil || !script.Group || script.Script == "" {
		t.Fatalf("script lost: %+v", script)
	}
	unresolved := findEntry(back.Entries, "unresolved")
	if unresolved == nil || unresolved.Expr != "+18" || unresolved.Offsets != "18,-4" {
		t.Fatalf("expression lost: %+v", unresolved)
	}
	custom := findEntry(back.Entries, "custom")
	if custom == nil || custom.Type != "mystruct" {
		t.Fatalf("custom type lost: %+v", custom)
	}
}

func TestGroupRoundTrip(t *testing.T) {
	tbl := &Table{}
	tbl.Add("root", "0x1000", "dword", "1")
	tbl.Entries[0].Group = true
	tbl.Entries[0].Children = []Entry{{Description: "child", Address: "0x1004", Type: "dword"}}

	path := filepath.Join(t.TempDir(), "tree.json")
	if err := tbl.ExportJSON(path); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	loaded, err := ImportJSON(path)
	if err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}
	if !loaded.Entries[0].Group || len(loaded.Entries[0].Children) != 1 || loaded.Entries[0].Children[0].Description != "child" {
		t.Fatalf("hierarchy lost: %+v", loaded.Entries[0])
	}

	ct := filepath.Join(t.TempDir(), "tree.ct")
	if err := tbl.Save(ct); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fromXML, err := Load(ct)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !fromXML.Entries[0].Group || len(fromXML.Entries[0].Children) != 1 || fromXML.Entries[0].Children[0].Address != "0x1004" {
		t.Fatalf("xml hierarchy lost: %+v", fromXML.Entries[0])
	}
}

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
