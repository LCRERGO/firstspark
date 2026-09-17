package asm

import "testing"

func FuzzAssemble(f *testing.F) {
	f.Add("mov eax, [rdi]")
	f.Add("nop")
	f.Add("call 0x401000")
	f.Add("")
	f.Add("garbage !!")
	f.Fuzz(func(t *testing.T, line string) {
		_, _ = Assemble(line, 0x400000)
	})
}

func FuzzDisassemble(f *testing.F) {
	f.Add([]byte{0x90})
	f.Add([]byte{0x48, 0x8B, 0x45, 0x08})
	f.Add([]byte{0xC4, 0xD1, 0xB8}) // truncated VEX prefix, used to panic x86asm
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, code []byte) {
		if len(code) > 4096 {
			return
		}
		_ = Disassemble(code, 0)
		_ = DecodeOne(code, 0)
	})
}
