package asm

import "fmt"

// reg describes an x86-64 general purpose register.
type reg struct {
	name  string
	code  int  // 0-15
	size  int  // 1, 2, 4 or 8
	high8 bool // AH/CH/DH/BH use codes 4-7 without REX
}

var registers = map[string]reg{}

func init() {
	// 8-bit low registers 0-3.
	for i, n := range []string{"al", "cl", "dl", "bl"} {
		registers[n] = reg{name: n, code: i, size: 1}
	}
	// Legacy high bytes 4-7 (no REX).
	for i, n := range []string{"ah", "ch", "dh", "bh"} {
		registers[n] = reg{name: n, code: 4 + i, size: 1, high8: true}
	}
	// New 8-bit registers 4-7 (require REX).
	for i, n := range []string{"spl", "bpl", "sil", "dil"} {
		registers[n] = reg{name: n, code: 4 + i, size: 1}
	}
	for i := 0; i < 8; i++ {
		n := fmt.Sprintf("r%db", 8+i)
		registers[n] = reg{name: n, code: 8 + i, size: 1}
	}

	// 16-bit.
	for i, n := range []string{"ax", "cx", "dx", "bx", "sp", "bp", "si", "di"} {
		registers[n] = reg{name: n, code: i, size: 2}
	}
	for i := 0; i < 8; i++ {
		n := fmt.Sprintf("r%dw", 8+i)
		registers[n] = reg{name: n, code: 8 + i, size: 2}
	}

	// 32-bit.
	for i, n := range []string{"eax", "ecx", "edx", "ebx", "esp", "ebp", "esi", "edi"} {
		registers[n] = reg{name: n, code: i, size: 4}
	}
	for i := 0; i < 8; i++ {
		n := fmt.Sprintf("r%dd", 8+i)
		registers[n] = reg{name: n, code: 8 + i, size: 4}
	}

	// 64-bit.
	for i, n := range []string{"rax", "rcx", "rdx", "rbx", "rsp", "rbp", "rsi", "rdi"} {
		registers[n] = reg{name: n, code: i, size: 8}
	}
	for i := 0; i < 8; i++ {
		n := fmt.Sprintf("r%d", 8+i)
		registers[n] = reg{name: n, code: 8 + i, size: 8}
	}

	// Instruction pointer, only valid inside memory operands.
	registers["rip"] = reg{name: "rip", code: 5, size: 8}
	registers["eip"] = reg{name: "eip", code: 5, size: 4}
}

func lookupReg(name string) (reg, bool) {
	r, ok := registers[name]
	return r, ok
}
