// Package asm provides x86-64 disassembly (pure Go) and a small pure-Go
// Intel-syntax assembler used for patching and trampolines.
package asm

import (
	"fmt"

	"golang.org/x/arch/x86/x86asm"
)

// Instruction is one decoded instruction.
type Instruction struct {
	Addr  uint64
	Bytes []byte
	Text  string
	Len   int
}

// Disassemble decodes code as 64-bit x86 starting at virtual address base.
// Bytes that fail to decode are emitted one at a time as `db` pseudo-ops so
// the output always covers the full input.
func Disassemble(code []byte, base uint64) []Instruction {
	out := make([]Instruction, 0, len(code)/2)
	for off := 0; off < len(code); {
		inst, err := x86asm.Decode(code[off:], 64)
		if err != nil || inst.Len == 0 || off+inst.Len > len(code) {
			out = append(out, Instruction{
				Addr:  base + uint64(off),
				Bytes: []byte{code[off]},
				Text:  fmt.Sprintf("db 0x%02x", code[off]),
				Len:   1,
			})
			off++
			continue
		}
		raw := code[off : off+inst.Len]
		out = append(out, Instruction{
			Addr:  base + uint64(off),
			Bytes: raw,
			Text:  x86asm.IntelSyntax(inst, base+uint64(off), nil),
			Len:   inst.Len,
		})
		off += inst.Len
	}
	return out
}

// DecodeOne decodes a single instruction and returns it. When the bytes do not
// form a valid instruction, Len is 1 and Text is a `db` pseudo-op.
func DecodeOne(code []byte, base uint64) Instruction {
	if len(code) == 0 {
		return Instruction{Addr: base, Text: "db 0x??", Len: 1}
	}
	inst, err := x86asm.Decode(code, 64)
	if err != nil || inst.Len == 0 || inst.Len > len(code) {
		return Instruction{
			Addr:  base,
			Bytes: []byte{code[0]},
			Text:  fmt.Sprintf("db 0x%02x", code[0]),
			Len:   1,
		}
	}
	return Instruction{
		Addr:  base,
		Bytes: code[:inst.Len],
		Text:  x86asm.IntelSyntax(inst, base, nil),
		Len:   inst.Len,
	}
}

// InstructionLen returns the length of the instruction at the start of code.
// It returns 1 when the bytes cannot be decoded.
func InstructionLen(code []byte) int {
	inst, err := x86asm.Decode(code, 64)
	if err != nil || inst.Len == 0 {
		return 1
	}
	return inst.Len
}
