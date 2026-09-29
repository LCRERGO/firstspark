package inject

import (
	"encoding/binary"
	"errors"
	"testing"

	"golang.org/x/arch/x86/x86asm"

	"github.com/LCRERGO/firstspark/pkg/asm"
)

func TestPlanSize(t *testing.T) {
	code, err := asm.AssembleBytes("mov rax, rbx\nadd rax, 1", 0x1000)
	if err != nil {
		t.Fatalf("AssembleBytes: %v", err)
	}
	size, err := PlanSize(code, 5)
	if err != nil {
		t.Fatalf("PlanSize: %v", err)
	}
	if size != 7 {
		t.Errorf("size = %d, want 7", size)
	}
}

func TestPlanSizeAcceptsRelocatable(t *testing.T) {
	for _, src := range []string{"mov rax, [rip+0x10]", "jmp 0x2000"} {
		code, err := asm.AssembleBytes(src, 0x1000)
		if err != nil {
			t.Fatalf("AssembleBytes(%q): %v", src, err)
		}
		if _, err := PlanSize(code, 5); err != nil {
			t.Errorf("PlanSize(%q) = %v, want success", src, err)
		}
	}
}

func TestPlanSizeRejectsUnsupportedBranch(t *testing.T) {
	// loop rel8 (E2) cannot be widened without changing the code size.
	if _, err := PlanSize([]byte{0xE2, 0xFE}, 2); !errors.Is(err, ErrUnrelocatable) {
		t.Errorf("err = %v, want ErrUnrelocatable", err)
	}
}

func TestRelocateRIPRelative(t *testing.T) {
	code, err := asm.AssembleBytes("mov rax, [rip+0x10]", 0x1000)
	if err != nil {
		t.Fatalf("AssembleBytes: %v", err)
	}
	moved, err := relocate(code, 0x1000, 0x9000)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	orig, err := x86asm.Decode(code, 64)
	if err != nil {
		t.Fatalf("decode original: %v", err)
	}
	oldDisp := int64(int32(binary.LittleEndian.Uint32(code[3:])))
	want := 0x1000 + int64(orig.Len) + oldDisp

	newDisp := int64(int32(binary.LittleEndian.Uint32(moved[3:])))
	if got := 0x9000 + int64(orig.Len) + newDisp; got != want {
		t.Errorf("effective address = %#x, want %#x", got, want)
	}
}

func TestRelocateRelativeJump(t *testing.T) {
	code, err := asm.AssembleBytes("jmp 0x2000", 0x1000)
	if err != nil {
		t.Fatalf("AssembleBytes: %v", err)
	}
	moved, err := relocate(code, 0x1000, 0x9000)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	inst, err := x86asm.Decode(moved, 64)
	if err != nil {
		t.Fatalf("decode moved: %v", err)
	}
	rel := int64(int32(binary.LittleEndian.Uint32(moved[1:])))
	target := int64(0x9000) + int64(inst.Len) + rel
	if target != 0x2000 {
		t.Errorf("jump target = %#x, want 0x2000", target)
	}
}

func TestRelocateWidensJcc8(t *testing.T) {
	// jz +0x10 from 0x1000: target 0x1012.
	moved, err := relocate([]byte{0x74, 0x10}, 0x1000, 0x9000)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	if len(moved) != 6 || moved[0] != 0x0F || moved[1] != 0x84 {
		t.Fatalf("moved bytes = % x, want 0f 84 <rel32>", moved)
	}
	rel := int64(int32(binary.LittleEndian.Uint32(moved[2:])))
	if target := int64(0x9000) + 6 + rel; target != 0x1012 {
		t.Errorf("jcc target = %#x, want 0x1012", target)
	}
}

func TestEncodeRelJumpRange(t *testing.T) {
	if _, err := encodeRelJump(0x1000, 0x2000); err != nil {
		t.Fatalf("near jump: %v", err)
	}
	if _, err := encodeRelJump(0x1000, 0x1_0000_0000); err == nil {
		t.Fatal("far jump should not fit in rel32")
	}
	if got := encodeAbsJump(0x1_0000_0000); len(got) != absJumpLen {
		t.Fatalf("abs jump length = %d, want %d", len(got), absJumpLen)
	}
}
