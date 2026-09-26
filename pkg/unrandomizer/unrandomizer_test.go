package unrandomizer

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/asm"
)

func TestBuildHandlerReturnsConstant(t *testing.T) {
	code, err := buildHandler(0x1234)
	if err != nil {
		t.Fatalf("buildHandler: %v", err)
	}
	ins := asm.Disassemble(code, 0)
	if len(ins) == 0 {
		t.Fatal("empty handler")
	}
	if last := ins[len(ins)-1]; last.Text != "ret" {
		t.Fatalf("handler does not end in ret: %q", last.Text)
	}
}
