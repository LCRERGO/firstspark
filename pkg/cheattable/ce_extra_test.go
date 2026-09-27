package cheattable

import (
	"strings"
	"testing"
)

func TestCEExtrasRoundTrip(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<CheatTable CheatEngineTableVersion="45">
  <CheatEntries>
    <CheatEntry>
      <ID>1</ID>
      <Description>"hp"</Description>
      <VariableType>4 Bytes</VariableType>
      <Address>1000</Address>
      <Color>00FF00</Color>
      <LastState RealAddress="1000" Value="42" Activated="1"/>
      <Hotkeys>
        <Hotkey><Action>Toggle Activation</Action><Keys><Key>112</Key></Keys></Hotkey>
      </Hotkeys>
      <UnknownThing>hello</UnknownThing>
    </CheatEntry>
  </CheatEntries>
</CheatTable>`)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tbl.Entries) != 1 {
		t.Fatalf("entries = %d", len(tbl.Entries))
	}
	e := tbl.Entries[0]
	if e.Color != "00FF00" {
		t.Errorf("Color = %q", e.Color)
	}
	if e.LastValue != "42" || e.LastAddress != "1000" || !e.Activated {
		t.Errorf("LastState = %+v", e)
	}
	if e.Hotkey != "F1" {
		t.Errorf("Hotkey = %q, want F1", e.Hotkey)
	}
	if len(e.CEHotkeys) != 1 || e.CEHotkeys[0].Keys != "112" {
		t.Errorf("CEHotkeys = %+v", e.CEHotkeys)
	}
	if len(e.ExtraElements) != 1 || e.ExtraElements[0].Name != "UnknownThing" || e.ExtraElements[0].Text != "hello" {
		t.Errorf("ExtraElements = %+v", e.ExtraElements)
	}

	out, err := tbl.MarshalCE()
	if err != nil {
		t.Fatalf("MarshalCE: %v", err)
	}
	for _, want := range []string{"Color", "00FF00", "LastState", "RealAddress", "Activated", "Hotkeys", "112", "UnknownThing", "hello"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("export missing %q:\n%s", want, out)
		}
	}

	back, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	e2 := back.Entries[0]
	if e2.Color != "00FF00" || !e2.Activated || e2.Hotkey != "F1" || len(e2.ExtraElements) != 1 {
		t.Fatalf("re-import = %+v", e2)
	}
}

func TestCECustomTypeFlags(t *testing.T) {
	body := `
TypeName:
  db 'mytype',0
ByteSize:
  dd 4
PREFEREDALIGNMENT:
  dd 2
CALLMETHOD:
  db 1
ConvertRoutine:
  mov eax, [rcx]
  ret
ConvertBackRoutine:
  mov [rdx], ecx
  ret
`
	def, ok := customTypeFromAA(body)
	if !ok {
		t.Fatal("custom type not extracted")
	}
	if def.Name != "mytype" || def.Size != 4 || def.Alignment != 2 || !def.CallMethod {
		t.Fatalf("def = %+v", def)
	}
	if def.UsesFloat || def.UsesString || def.MaxStringSize != 0 {
		t.Fatalf("unexpected flags: %+v", def)
	}
	if def.ConvertRoutine == "" || def.ConvertBackRoutine == "" {
		t.Fatalf("routines missing: %+v", def)
	}
}

func TestCECustomTypeStringFlag(t *testing.T) {
	body := `
TypeName:
  db 'text16',0
ByteSize:
  dd 8
USESSTRING:
  db 1
MAXSTRINGSIZE:
  dd 64
ConvertRoutine:
  ret
`
	def, ok := customTypeFromAA(body)
	if !ok {
		t.Fatal("custom type not extracted")
	}
	if !def.UsesString || def.MaxStringSize != 64 {
		t.Fatalf("def = %+v", def)
	}
}

func TestFirstsparkHotkeyExportsAsCEHotkey(t *testing.T) {
	tbl := &Table{Version: SchemaVersion, Entries: []Entry{
		{ID: 1, Description: "hp", Address: "0x1000", Type: "dword", Hotkey: "F5"},
	}}
	data, err := tbl.MarshalCE()
	if err != nil {
		t.Fatalf("MarshalCE: %v", err)
	}
	if !strings.Contains(string(data), "<Key>116</Key>") {
		t.Fatalf("F5 should export as VK 116:\n%s", data)
	}
}

func TestFrozenExportsAsActivated(t *testing.T) {
	tbl := &Table{Version: SchemaVersion, Entries: []Entry{
		{ID: 1, Description: "hp", Address: "0x1000", Type: "dword", Value: "42", Frozen: true},
	}}
	data, err := tbl.MarshalCE()
	if err != nil {
		t.Fatalf("MarshalCE: %v", err)
	}
	if !strings.Contains(string(data), "Activated") {
		t.Fatalf("frozen state not exported:\n%s", data)
	}
	back, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := back.Entries[0]
	if !e.Activated || e.LastValue != "42" || e.LastAddress != "1000" {
		t.Fatalf("LastState = %+v", e)
	}
}

func TestCEGroupNestingRoundTrip(t *testing.T) {
	tbl := &Table{Version: SchemaVersion, Entries: []Entry{
		{ID: 1, Description: "top", Group: true, Children: []Entry{
			{ID: 2, Description: "sub", Group: true, Children: []Entry{
				{ID: 3, Description: "leaf", Address: "0x2000", Type: "dword", Value: "7"},
			}},
		}},
	}}
	data, err := tbl.MarshalCE()
	if err != nil {
		t.Fatalf("MarshalCE: %v", err)
	}
	back, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(back.Entries) != 1 || len(back.Entries[0].Children) != 1 || len(back.Entries[0].Children[0].Children) != 1 {
		t.Fatalf("nesting lost: %+v", back.Entries)
	}
	if back.Entries[0].Children[0].Children[0].Description != "leaf" {
		t.Fatalf("leaf lost: %+v", back.Entries[0].Children[0].Children[0])
	}
}

func TestFirstsparkExtrasXMLRoundTrip(t *testing.T) {
	tbl := &Table{Version: SchemaVersion, Entries: []Entry{{
		ID: 1, Description: "x", Address: "0x10", Type: "dword",
		Color: "00FF00", LastValue: "5", LastAddress: "10", Activated: true,
		CEHotkeys:     []CEHotkey{{Action: "Toggle Activation", Keys: "112"}},
		ExtraElements: []RawElement{{Name: "Foo", Text: "bar"}},
	}}}
	data, err := tbl.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := back.Entries[0]
	if e.Color != "00FF00" || e.LastValue != "5" || !e.Activated {
		t.Fatalf("extras lost: %+v", e)
	}
	if len(e.CEHotkeys) != 1 || e.CEHotkeys[0].Keys != "112" {
		t.Fatalf("hotkeys lost: %+v", e.CEHotkeys)
	}
	if len(e.ExtraElements) != 1 || e.ExtraElements[0].Name != "Foo" || e.ExtraElements[0].Text != "bar" {
		t.Fatalf("elements lost: %+v", e.ExtraElements)
	}
}
