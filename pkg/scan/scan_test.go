package scan

import (
	"testing"
)

func TestParseValue(t *testing.T) {
	cases := []struct {
		typ   ValueType
		input string
		want  int64
	}{
		{TypeByte, "0x7f", 0x7f},
		{TypeWord, "0x1234", 0x1234},
		{TypeDword, "1000", 1000},
		{TypeQword, "-5", -5},
	}
	for _, c := range cases {
		v, err := ParseValue(c.typ, c.input)
		if err != nil {
			t.Fatalf("ParseValue(%s, %q): %v", c.typ, c.input, err)
		}
		if v.Int64() != c.want {
			t.Errorf("ParseValue(%s, %q).Int64() = %d, want %d", c.typ, c.input, v.Int64(), c.want)
		}
	}
}

func TestParseFloat(t *testing.T) {
	v, err := ParseValue(TypeFloat, "3.5")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if v.Float64() != 3.5 {
		t.Errorf("Float64() = %v, want 3.5", v.Float64())
	}
}

func TestAOBMatch(t *testing.T) {
	p, err := ParseAOB("48 8B ?? E5")
	if err != nil {
		t.Fatalf("ParseAOB: %v", err)
	}
	if !p.Match([]byte{0x48, 0x8B, 0x00, 0xE5}) {
		t.Error("expected wildcard pattern to match")
	}
	if p.Match([]byte{0x48, 0x8B, 0x00, 0xE6}) {
		t.Error("expected mismatch on last byte")
	}
}

func TestCompareInt(t *testing.T) {
	if !compareInt(5, 5, OpEqual) || compareInt(5, 4, OpEqual) {
		t.Error("OpEqual failed")
	}
	if !compareInt(5, 4, OpGreater) || compareInt(4, 5, OpGreater) {
		t.Error("OpGreater failed")
	}
}

func TestCompareFloatEpsilon(t *testing.T) {
	if !compareFloat(1.0000001, 1.0, OpEqual, 1e-6) {
		t.Error("expected values within epsilon to be equal")
	}
	if compareFloat(1.1, 1.0, OpEqual, 1e-6) {
		t.Error("expected values outside epsilon to differ")
	}
}

func TestValueDelta(t *testing.T) {
	cur := Value{Type: TypeDword, Raw: EncodeValue(TypeDword, 15)}
	prev := Value{Type: TypeDword, Raw: EncodeValue(TypeDword, 10)}
	delta := Value{Type: TypeDword, Raw: EncodeValue(TypeDword, 5)}
	if !valueDelta(cur, prev, delta, true, 1e-6) {
		t.Error("expected increased-by match")
	}
	if valueDelta(cur, prev, delta, false, 1e-6) {
		t.Error("did not expect decreased-by match")
	}
}

func TestParseScanMode(t *testing.T) {
	m, err := ParseScanMode("increased by")
	if err != nil || m != ModeIncreasedBy {
		t.Errorf("ParseScanMode = %v, %v", m, err)
	}
}
