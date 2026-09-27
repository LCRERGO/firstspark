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

func TestInterpMovExtend(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  movzx eax, byte [rdi]
  ret
`, 1)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{0xff})
	if got := p.Call(entry, p.DataPtr()); got != 0xff {
		t.Fatalf("movzx = %#x, want 0xff", got)
	}

	q := compileScript(t, `[ENABLE]
ConvertRoutine:
  movsx eax, byte [rdi]
  ret
`, 1)
	defer q.Close()
	e2, _ := q.Entry("ConvertRoutine")
	q.SetData([]byte{0xff})
	if got := uint32(q.Call(e2, q.DataPtr())); got != 0xffffffff {
		t.Fatalf("movsx = %#x, want 0xffffffff", got)
	}
}

func TestInterpLogicAndBranch(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  movzx eax, byte [rdi]
  cmp eax, 0x2a
  jne no
  mov eax, 1
  ret
no:
  xor eax, eax
  ret
`, 1)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{0x2a})
	if got := p.Call(entry, p.DataPtr()); got != 1 {
		t.Fatalf("equal branch = %d, want 1", got)
	}
	p.SetData([]byte{0x2b})
	if got := p.Call(entry, p.DataPtr()); got != 0 {
		t.Fatalf("not-equal branch = %d, want 0", got)
	}
}

func TestInterpLea(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  lea rax, [rdi+4]
  ret
`, 4)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	want := uint64(p.DataPtr()) + 4
	if got := uint64(p.Call(entry, p.DataPtr())); got != want {
		t.Fatalf("lea = %#x, want %#x", got, want)
	}
}

func TestInterpArithmeticShift(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  mov eax, [rdi]
  sar eax, 1
  ret
`, 4)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{0xf8, 0xff, 0xff, 0xff}) // -8
	if got := uint32(p.Call(entry, p.DataPtr())); got != 0xfffffffc {
		t.Fatalf("sar = %#x, want 0xfffffffc", got)
	}
}

func TestInterpUnsignedDivide(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  mov eax, [rdi]
  xor edx, edx
  mov ecx, 4
  div ecx
  ret
`, 4)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{40, 0, 0, 0})
	if got := p.Call(entry, p.DataPtr()); got != 10 {
		t.Fatalf("div = %d, want 10", got)
	}
}

func TestInterpOutOfBuffer(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  mov eax, [rdi+0x100000]
  ret
`, 4)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.SetData([]byte{1, 2, 3, 4})
	p.Call(entry, p.DataPtr())
	if p.Err() == nil {
		t.Fatal("expected an out-of-buffer error")
	}
}

func TestInterpStepLimit(t *testing.T) {
	p := compileScript(t, `[ENABLE]
ConvertRoutine:
  jmp ConvertRoutine
`, 4)
	defer p.Close()
	entry, _ := p.Entry("ConvertRoutine")
	p.Call(entry, p.DataPtr())
	if p.Err() == nil {
		t.Fatal("expected a step-limit error")
	}
}
