package scan

import "testing"

func TestParseValueRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		typ   ValueType
		input string
	}{
		{TypeDword, "4 * (10 ^ 12)"}, // 4e12 wraps a 4-byte value
		{TypeDword, "4294967296"},    // 2^32
		{TypeDword, "-2147483649"},   // below int32 min
		{TypeWord, "65536"},          // 2^16
		{TypeWord, "-32769"},
		{TypeByte, "256"},
		{TypeByte, "-129"},
	}
	for _, c := range cases {
		if _, err := ParseValue(c.typ, c.input); err == nil {
			t.Errorf("ParseValue(%s, %q) accepted an out-of-range value", c.typ, c.input)
		}
	}
}

func TestParseValueAcceptsRangeBoundaries(t *testing.T) {
	cases := []struct {
		typ   ValueType
		input string
	}{
		{TypeDword, "4294967295"}, // uint32 max
		{TypeDword, "-2147483648"},
		{TypeDword, "0xFFFFFFFF"},
		{TypeWord, "65535"},
		{TypeByte, "255"},
		{TypeByte, "-128"},
		{TypeQword, "4000000000000"}, // fits 8 bytes
		{TypeAll, "4000000000000"},
	}
	for _, c := range cases {
		if _, err := ParseValue(c.typ, c.input); err != nil {
			t.Errorf("ParseValue(%s, %q): %v", c.typ, c.input, err)
		}
	}
}

func TestParseValueExpressionInRange(t *testing.T) {
	v, err := ParseValue(TypeQword, "4 * (10 ^ 12)")
	if err != nil {
		t.Fatalf("ParseValue(qword, 4e12): %v", err)
	}
	if v.Uint64() != 4_000_000_000_000 {
		t.Fatalf("value = %d", v.Uint64())
	}
}
