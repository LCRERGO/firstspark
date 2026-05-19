package asm

import (
	"encoding/hex"
	"testing"
)

func TestAssemble(t *testing.T) {
	cases := []struct {
		src  string
		addr uint64
		want string
	}{
		{"nop", 0, "90"},
		{"ret", 0, "c3"},
		{"mov rax, rbx", 0, "4889d8"},
		{"mov eax, ebx", 0, "89d8"},
		{"mov rax, 0x1122334455667788", 0, "48b88877665544332211"},
		{"mov eax, 0x11223344", 0, "b844332211"},
		{"add rax, rbx", 0, "4801d8"},
		{"add rax, 1", 0, "4883c001"},
		{"add rax, 0x1234", 0, "4881c034120000"},
		{"sub rsp, 0x28", 0, "4883ec28"},
		{"lea rax, [rbx+0x10]", 0, "488d4310"},
		{"lea rax, [rip+0x100]", 0, "488d0500010000"},
		{"push rbp", 0, "55"},
		{"pop rbp", 0, "5d"},
		{"jmp 0x1000", 0, "e9fb0f0000"},
		{"call rax", 0, "48ffd0"},
		{"test rax, rax", 0, "4885c0"},
		{"xor eax, eax", 0, "31c0"},
		{"movzx eax, byte ptr [rbx]", 0, "0fb603"},
		{"mov dword ptr [rax], 1", 0, "c70001000000"},
		{"inc qword ptr [rax]", 0, "48ff00"},
		{"shl rax, 3", 0, "48c1e003"},
		{"imul rax, rbx", 0, "480fafc3"},
		{"mov r8, rax", 0, "4989c0"},
		{"add r15, r8", 0, "4d01c7"},
		{"je 0x20", 0, "0f841a000000"},
	}
	for _, c := range cases {
		got, err := Assemble(c.src, c.addr)
		if err != nil {
			t.Errorf("Assemble(%q) error: %v", c.src, err)
			continue
		}
		if hex.EncodeToString(got) != c.want {
			t.Errorf("Assemble(%q) = %s, want %s", c.src, hex.EncodeToString(got), c.want)
		}
	}
}

func TestAssembleErrors(t *testing.T) {
	bad := []string{
		"mov [rax], 1",
		"frobnicate rax",
		"mov rax, rbx, rcx",
		"mov ah, r8b",
	}
	for _, src := range bad {
		if _, err := Assemble(src, 0); err == nil {
			t.Errorf("Assemble(%q) expected error", src)
		}
	}
}

func TestAssembleProgramLabels(t *testing.T) {
	prog := `
start:
	mov rax, 1
	jmp end
	mov rax, 2
end:
	ret`
	code, labels, err := AssembleProgram(prog, 0x1000)
	if err != nil {
		t.Fatalf("AssembleProgram: %v", err)
	}
	if labels["start"] != 0x1000 {
		t.Errorf("start = %#x, want 0x1000", labels["start"])
	}
	if labels["end"] != 0x1019 {
		t.Errorf("end = %#x, want 0x1019", labels["end"])
	}
	if len(code) != 0x1A {
		t.Errorf("len = %d, want 26", len(code))
	}
	ins := Disassemble(code, 0x1000)
	if len(ins) != 4 {
		t.Fatalf("got %d instructions, want 4", len(ins))
	}
	if ins[1].Text != "jmp 0x1019" {
		t.Errorf("jmp text = %q, want jmp 0x1019", ins[1].Text)
	}
}

func TestDisassembleRoundTrip(t *testing.T) {
	code, err := AssembleBytes("mov rax, rbx\nadd rax, 1\nret", 0x1000)
	if err != nil {
		t.Fatalf("AssembleBytes: %v", err)
	}
	ins := Disassemble(code, 0x1000)
	if len(ins) != 3 {
		t.Fatalf("got %d instructions, want 3", len(ins))
	}
	if ins[0].Len != 3 || ins[1].Len != 4 || ins[2].Len != 1 {
		t.Errorf("unexpected lengths: %d %d %d", ins[0].Len, ins[1].Len, ins[2].Len)
	}
}
