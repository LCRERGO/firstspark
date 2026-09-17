package inject

import (
	"errors"
	"testing"

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

func TestPlanSizeRejectsRIPRelative(t *testing.T) {
	code, err := asm.AssembleBytes("mov rax, [rip+0x10]", 0x1000)
	if err != nil {
		t.Fatalf("AssembleBytes: %v", err)
	}
	if _, err := PlanSize(code, 5); !errors.Is(err, ErrUnrelocatable) {
		t.Errorf("err = %v, want ErrUnrelocatable", err)
	}
}

func TestPlanSizeRejectsRelativeJump(t *testing.T) {
	code, err := asm.AssembleBytes("jmp 0x2000", 0x1000)
	if err != nil {
		t.Fatalf("AssembleBytes: %v", err)
	}
	if _, err := PlanSize(code, 5); !errors.Is(err, ErrUnrelocatable) {
		t.Errorf("err = %v, want ErrUnrelocatable", err)
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
