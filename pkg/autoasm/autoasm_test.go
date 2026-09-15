package autoasm

import "testing"

func TestParseSectionsAndDirectives(t *testing.T) {
	src := `[ENABLE]
alloc(newmem, 1024)
label(return)
define(val, 4)
db 90 90
newmem:
  mov eax, val
  ret
return:

[DISABLE]
dealloc(newmem)`
	s, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Sections) != 2 {
		t.Fatalf("sections = %d", len(s.Sections))
	}
	en := s.Sections[0]
	if !en.Enable || len(en.Items) != 8 {
		t.Fatalf("enable items = %d (%+v)", len(en.Items), en.Items)
	}
	if en.Items[0].Kind != KindAlloc || en.Items[0].Name != "newmem" {
		t.Fatalf("alloc = %+v", en.Items[0])
	}
	if en.Items[2].Kind != KindDefine || en.Items[2].Args[1] != "4" {
		t.Fatalf("define = %+v", en.Items[2])
	}
	if en.Items[3].Kind != KindData || len(en.Items[3].Data) != 2 {
		t.Fatalf("db = %+v", en.Items[3])
	}
	if en.Items[5].Text != "mov eax, 4" {
		t.Fatalf("define substitution failed: %q", en.Items[5].Text)
	}
	if s.Sections[1].Enable {
		t.Fatal("second section should be DISABLE")
	}
}

func TestAssembleDataLabelsAndInstructions(t *testing.T) {
	s := &Section{Enable: true, Items: []Item{
		{Kind: KindData, Data: []byte{0x90, 0x90}},
		{Kind: KindLabel, Name: "here"},
		{Kind: KindInstruction, Text: "mov eax, 1"},
		{Kind: KindInstruction, Text: "ret"},
	}}
	code, labels, err := Assemble(s, 0x1000, nil)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if labels["here"] != 0x1002 {
		t.Fatalf("label here = 0x%x", labels["here"])
	}
	if len(code) != 8 || code[0] != 0x90 || code[7] != 0xC3 {
		t.Fatalf("code = % x", code)
	}
}

func TestAssembleResolvesLabelsInJumps(t *testing.T) {
	s := &Section{Enable: true, Items: []Item{
		{Kind: KindInstruction, Text: "jmp done"},
		{Kind: KindInstruction, Text: "nop"},
		{Kind: KindLabel, Name: "done"},
		{Kind: KindInstruction, Text: "ret"},
	}}
	code, _, err := Assemble(s, 0x2000, nil)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if code[0] != 0xE9 {
		t.Fatalf("expected a relative jmp, got % x", code[:2])
	}
}
