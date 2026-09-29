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

func TestBuildHandlerEverySymbol(t *testing.T) {
	symbols := append(append([]string{}, DefaultSymbols...), "usleep", "sleep")
	for _, sym := range symbols {
		h, err := BuildHandler(sym, 2.0)
		if err != nil {
			t.Fatalf("BuildHandler(%q): %v", sym, err)
		}
		if h.TrampSlot <= 0 || h.TrampSlot+8 > len(h.Code) {
			t.Fatalf("%s: bad trampoline slot %d for %d bytes", sym, h.TrampSlot, len(h.Code))
		}
		ins := asm.Disassemble(h.Code, 0)
		if len(ins) < 6 {
			t.Fatalf("%s: handler too short: %d instructions", sym, len(ins))
		}
		if ins[len(ins)-1].Text != "ret" {
			t.Errorf("%s: handler does not end in ret: %q", sym, ins[len(ins)-1].Text)
		}
	}
}

func TestBuildHandlerUnknownSymbol(t *testing.T) {
	if _, err := BuildHandler("frobnicate_time", 2.0); err == nil {
		t.Error("expected error for unsupported symbol")
	}
}

func TestBuildHandlerInvalidScale(t *testing.T) {
	for _, scale := range []float64{0, -1, 1e-12} {
		if _, err := BuildHandler("clock_gettime", scale); err == nil {
			t.Errorf("expected error for scale %v", scale)
		}
	}
}
