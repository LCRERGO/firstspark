package speedhack

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/asm"
)

func TestRatio(t *testing.T) {
	cases := []struct {
		scale    float64
		num, den int64
	}{
		{2.0, 2, 1},
		{0.5, 1, 2},
		{1.5, 3, 2},
		{1.0, 1, 1},
	}
	for _, c := range cases {
		num, den, err := ratio(c.scale)
		if err != nil {
			t.Fatalf("ratio(%v): %v", c.scale, err)
		}
		if num != c.num || den != c.den {
			t.Errorf("ratio(%v) = %d/%d, want %d/%d", c.scale, num, den, c.num, c.den)
		}
	}
}

func TestBuildHandler(t *testing.T) {
	h, err := BuildHandler("clock_gettime", 2.0)
	if err != nil {
		t.Fatalf("BuildHandler: %v", err)
	}
	if h.TrampSlot <= 0 || h.TrampSlot+8 > len(h.Code) {
		t.Fatalf("bad trampoline slot %d for %d bytes", h.TrampSlot, len(h.Code))
	}
	ins := asm.Disassemble(h.Code, 0)
	if len(ins) < 10 {
		t.Fatalf("handler too short: %d instructions", len(ins))
	}
	if ins[len(ins)-1].Text != "ret" {
		t.Errorf("handler does not end in ret: %q", ins[len(ins)-1].Text)
	}
}

func TestBuildHandlerUnknownSymbol(t *testing.T) {
	if _, err := BuildHandler("time", 2.0); err == nil {
		t.Error("expected error for unsupported symbol")
	}
}
