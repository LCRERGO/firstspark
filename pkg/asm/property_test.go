package asm

import (
	"testing"
	"testing/quick"
)

// TestPropertyDisassembleNeverPanics checks the decoder is total over arbitrary
// byte sequences.
func TestPropertyDisassembleNeverPanics(t *testing.T) {
	prop := func(code []byte) bool {
		if len(code) > 64 {
			return true
		}
		for _, ins := range Disassemble(code, 0x400000) {
			if ins.Len < 0 || ins.Len > len(code) {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyDecodeOneBounded checks a single decode never consumes more bytes
// than were supplied.
func TestPropertyDecodeOneBounded(t *testing.T) {
	prop := func(code []byte) bool {
		if len(code) == 0 {
			return true
		}
		ins := DecodeOne(code, 0x1000)
		return ins.Len >= 0 && ins.Len <= len(code)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestDisassembleTruncatedVEX is the regression for an x86asm panic on a
// three-byte VEX prefix with no opcode, found by the property test above.
func TestDisassembleTruncatedVEX(t *testing.T) {
	code := []byte{0xC4, 0xD1, 0xB8}
	if ins := Disassemble(code, 0); len(ins) == 0 {
		t.Fatal("expected at least one instruction")
	}
	_ = DecodeOne(code, 0)
	_ = InstructionLen(code)
}

// TestPropertyAssembleNeverPanics checks the encoder is total over arbitrary
// text.
func TestPropertyAssembleNeverPanics(t *testing.T) {
	prop := func(line string) bool {
		if len(line) > 200 {
			return true
		}
		_, _ = Assemble(line, 0x400000)
		return true
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}
