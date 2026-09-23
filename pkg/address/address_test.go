package address

import "testing"

type mapResolver map[string]uint64

func (m mapResolver) ResolveName(name string) (uint64, bool) {
	v, ok := m[name]
	return v, ok
}

func TestEvalAbsolute(t *testing.T) {
	cases := []struct {
		expr string
		want uint64
	}{
		{"148+8", 0x150},
		{"7FF6ABCD", 0x7FF6ABCD},
		{"A0", 0xA0},
		{"$10*2+4", 0x24},
		{"(2+3)*4", 0x14},
		{"0x20+4", 0x24},
		{"10/2", 0x8},
	}
	for _, c := range cases {
		got, err := Eval(c.expr, 0, false, nil)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %#x, want %#x", c.expr, got, c.want)
		}
	}
}

func TestEvalRelative(t *testing.T) {
	cases := []struct {
		expr string
		want uint64
	}{
		{"+18", 0x1018},
		{"-8", 0xFF8},
		{"+4*$1", 0x1004},
		{"+", 0x1000},
	}
	for _, c := range cases {
		got, err := Eval(c.expr, 0x1000, true, nil)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %#x, want %#x", c.expr, got, c.want)
		}
	}
	if _, err := Eval("+18", 0, false, nil); err == nil {
		t.Error("relative expression without a parent should fail")
	}
}

func TestEvalNames(t *testing.T) {
	r := mapResolver{"pSelected": 0x5000, "ck3.exe": 0x140000000}
	if got, err := Eval("ck3.exe+1A2B", 0, false, r); err != nil || got != 0x140001A2B {
		t.Fatalf("module+offset = %#x, %v", got, err)
	}
	if got, err := Eval("pSelected", 0, false, r); err != nil || got != 0x5000 {
		t.Fatalf("symbol = %#x, %v", got, err)
	}
	if _, err := Eval("missing", 0, false, r); err == nil {
		t.Error("unknown name should fail")
	}
}

func TestEvalOffset(t *testing.T) {
	if v, err := EvalOffset("-4", nil); err != nil || v != -4 {
		t.Fatalf("EvalOffset(-4) = %d, %v", v, err)
	}
	if v, err := EvalOffset("18", nil); err != nil || v != 0x18 {
		t.Fatalf("EvalOffset(18) = %d, %v", v, err)
	}
	if _, err := EvalOffset("(", nil); err == nil {
		t.Error("malformed offset should fail")
	}
}
