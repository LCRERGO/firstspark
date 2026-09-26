package aaexec

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/autoasm"
)

func compileScript(t *testing.T, script string, valueSize int) *Program {
	t.Helper()
	s, err := autoasm.Parse(script)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for i := range s.Sections {
		if s.Sections[i].Enable {
			p, err := Compile(s.Sections[i].Items, 4096, valueSize)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			return p
		}
	}
	t.Fatal("no [ENABLE] section")
	return nil
}

func TestInterpArithmetic(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  mov eax, [rdi]
  add eax, 1
  ret
ConvertBackRoutine:
  mov eax, edi
  sub eax, 1
  mov [rsi], eax
  ret
`, 4)
	defer p.Close()

	read, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{9, 0, 0, 0})
	if got := p.Call(read, p.DataPtr()); got != 10 {
		t.Fatalf("read = %d, want 10", got)
	}
	write, _ := p.Entry("ConvertBackRoutine")
	p.Call(write, 10, p.DataPtr())
	if got := p.DataCopy(4); got[0] != 9 {
		t.Fatalf("write = %v, want 9,0,0,0", got)
	}
	if err := p.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
}

func TestInterpLoopAndBranch(t *testing.T) {
	// Counts bytes up to a zero terminator.
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  xor eax, eax
  mov rcx, rdi
loop:
  movzx edx, byte [rcx]
  test edx, edx
  jz done
  add eax, 1
  add rcx, 1
  jmp loop
done:
  ret
`, 8)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{'a', 'b', 'c', 0})
	if got := p.Call(entry, p.DataPtr()); got != 3 {
		t.Fatalf("len = %d, want 3", got)
	}
}

func TestInterpShiftsAndMul(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  mov eax, [rdi]
  shl eax, 2
  imul eax, 3
  ret
`, 4)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{4, 0, 0, 0})
	if got := p.Call(entry, p.DataPtr()); got != 48 {
		t.Fatalf("value = %d, want 48", got)
	}
}

func TestInterpSignedDivide(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  mov eax, [rdi]
  cdq
  mov ecx, 10
  idiv ecx
  ret
`, 4)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	// 100 -> 10, and -100 -> -10.
	p.SetData([]byte{100, 0, 0, 0})
	if got := int64(int32(p.Call(entry, p.DataPtr()))); got != 10 {
		t.Fatalf("100/10 = %d", got)
	}
	p.SetData([]byte{0x9c, 0xff, 0xff, 0xff}) // -100
	if got := int64(int32(p.Call(entry, p.DataPtr()))); got != -10 {
		t.Fatalf("-100/10 = %d", got)
	}
}

func TestCompileRejectsUnsupported(t *testing.T) {
	s, err := autoasm.Parse("[ENABLE]\nConvertRoutine:\n  vpxor ymm0, ymm0, ymm0\n  ret\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for i := range s.Sections {
		if s.Sections[i].Enable {
			if _, err := Compile(s.Sections[i].Items, 4096, 4); err == nil {
				t.Fatal("expected an error for an unsupported instruction")
			}
			return
		}
	}
}
