package scan

import "testing"

func TestParseExpression(t *testing.T) {
	cases := []struct {
		typ   ValueType
		input string
		want  int64
	}{
		{TypeDword, "360 * (10 ^ 6)", 360000000},
		{TypeDword, "2 + 3 * 4", 14},
		{TypeDword, "(2 + 3) * 4", 20},
		{TypeDword, "0xFF + 1", 256},
		{TypeDword, "-10 + 4", -6},
	}
	for _, c := range cases {
		v, err := ParseValue(c.typ, c.input)
		if err != nil {
			t.Fatalf("ParseValue(%s, %q): %v", c.typ, c.input, err)
		}
		if v.Int64() != c.want {
			t.Errorf("ParseValue(%s, %q) = %d, want %d", c.typ, c.input, v.Int64(), c.want)
		}
	}
}

func TestParseExpressionFloat(t *testing.T) {
	v, err := ParseValue(TypeDouble, "10 / 4")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if v.Float64() != 2.5 {
		t.Errorf("Float64() = %v, want 2.5", v.Float64())
	}
}

func TestParseExpressionRejectsNonInteger(t *testing.T) {
	if _, err := ParseValue(TypeDword, "10 / 4"); err == nil {
		t.Error("expected integer type to reject a non-integral expression")
	}
}

func TestParseExpressionPlainLiteralsStillWork(t *testing.T) {
	v, err := ParseValue(TypeDword, "-5")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if v.Int64() != -5 {
		t.Errorf("Int64() = %d, want -5", v.Int64())
	}
}
