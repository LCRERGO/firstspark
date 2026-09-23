package autoasm

import (
	"strings"
	"testing"
)

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

func TestParseLuaAndSections(t *testing.T) {
	src := "{$lua}\nif syntaxcheck then return end\n[ENABLE]\nshowMessage('on')\n[DISABLE]\nshowMessage('off')\n{$asm}\nunregistersymbol(foo)\n"
	s, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Sections) != 3 {
		t.Fatalf("sections = %d, want 3: %+v", len(s.Sections), s.Sections)
	}
	if !s.Sections[0].Enable || s.Sections[0].Items[0].Kind != KindLua ||
		!strings.Contains(s.Sections[0].Items[0].Text, "syntaxcheck") {
		t.Fatalf("section 0 = %+v", s.Sections[0])
	}
	if !s.Sections[1].Enable || s.Sections[1].Items[0].Kind != KindLua ||
		!strings.Contains(s.Sections[1].Items[0].Text, "showMessage('on')") {
		t.Fatalf("section 1 = %+v", s.Sections[1])
	}
	last := s.Sections[2]
	if last.Enable || len(last.Items) != 2 || last.Items[0].Kind != KindLua || last.Items[1].Kind != KindUnregisterSymbol {
		t.Fatalf("section 2 = %+v", last)
	}
}

func TestTokenizeAutoAssembler(t *testing.T) {
	toks := Tokenize("[ENABLE]\nalloc(newmem, 1024)\nlabel(x)\nnewmem:\ndb 90 // note\nmov eax, 1")
	kinds := map[string]TokenKind{}
	for _, tok := range toks {
		kinds[tok.Text] = tok.Kind
	}
	if kinds["[ENABLE]"] != TokenSection {
		t.Fatalf("section: %+v", kinds)
	}
	if kinds["alloc"] != TokenDirective || kinds["db"] != TokenDirective {
		t.Fatalf("directives: %+v", kinds)
	}
	if kinds["newmem"] != TokenLabel {
		t.Fatalf("label: %+v", kinds)
	}
	if kinds["// note"] != TokenComment {
		t.Fatalf("comment: %+v", kinds)
	}
	if kinds["1024"] != TokenNumber {
		t.Fatalf("number: %+v", kinds)
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
