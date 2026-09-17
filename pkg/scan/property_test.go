package scan

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

// TestPropertyIntegerRoundTrip checks that encoding an integer and decoding it
// back yields the same value for every integer width.
func TestPropertyIntegerRoundTrip(t *testing.T) {
	cases := []struct {
		typ ValueType
		fit func(int64) int64
	}{
		{TypeByte, func(n int64) int64 { return int64(int8(n)) }},
		{TypeWord, func(n int64) int64 { return int64(int16(n)) }},
		{TypeDword, func(n int64) int64 { return int64(int32(n)) }},
		{TypeQword, func(n int64) int64 { return n }},
	}
	for _, c := range cases {
		c := c
		t.Run(c.typ.String(), func(t *testing.T) {
			prop := func(n int64) bool {
				want := c.fit(n)
				raw := EncodeValue(c.typ, want)
				if raw == nil {
					return false
				}
				return Value{Type: c.typ, Raw: raw}.Int64() == want
			}
			if err := quick.Check(prop, nil); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestPropertyFloatBitsRoundTrip checks that the IEEE-754 bit pattern survives
// encode/decode for both float widths.
func TestPropertyFloatBitsRoundTrip(t *testing.T) {
	double := func(bits uint64) bool {
		raw := encodeFloat(TypeDouble, math.Float64frombits(bits)).Raw
		return math.Float64bits(Value{Type: TypeDouble, Raw: raw}.Float64()) == bits
	}
	if err := quick.Check(double, nil); err != nil {
		t.Errorf("double: %v", err)
	}
	single := func(bits uint32) bool {
		raw := encodeFloat(TypeFloat, float64(math.Float32frombits(bits))).Raw
		return math.Float32bits(float32(Value{Type: TypeFloat, Raw: raw}.Float64())) == bits
	}
	if err := quick.Check(single, nil); err != nil {
		t.Errorf("float: %v", err)
	}
}

// TestPropertyFormatParseRoundTrip checks that a decimal integer renders and
// parses back to itself.
func TestPropertyFormatParseRoundTrip(t *testing.T) {
	prop := func(n int64) bool {
		s := strconv.FormatInt(n, 10)
		v, err := ParseValue(TypeQword, s)
		return err == nil && v.Int64() == n
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyAOBFormatRoundTrip checks that formatting bytes as an AOB pattern
// and parsing it back preserves the bytes.
func TestPropertyAOBFormatRoundTrip(t *testing.T) {
	prop := func(b []byte) bool {
		if len(b) == 0 {
			return true
		}
		p, err := ParseAOB(formatBytes(b))
		if err != nil {
			return false
		}
		if !bytes.Equal(p.Bytes, b) {
			return false
		}
		for _, m := range p.Mask {
			if m != 0xFF {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyBinaryFormatRoundTrip checks that a bit pattern formats and
// reparses to the same bytes, mask and bit count.
func TestPropertyBinaryFormatRoundTrip(t *testing.T) {
	prop := func(seed []byte) bool {
		if len(seed) == 0 || len(seed) > 64 {
			return true
		}
		var b strings.Builder
		for _, x := range seed {
			switch x % 3 {
			case 0:
				b.WriteByte('0')
			case 1:
				b.WriteByte('1')
			default:
				b.WriteByte('?')
			}
		}
		p, err := ParseBinary(b.String())
		if err != nil {
			return false
		}
		v := Value{Type: TypeBinary, Raw: p.Bytes, Mask: p.Mask, Bits: p.Bits}
		p2, err := ParseBinary(FormatBinary(v))
		if err != nil {
			return false
		}
		return reflect.DeepEqual(p, p2)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyCompareInt checks the comparison operators against Go's.
func TestPropertyCompareInt(t *testing.T) {
	prop := func(a, b int64) bool {
		switch {
		case compareInt(a, b, OpEqual) != (a == b):
			return false
		case compareInt(a, b, OpGreater) != (a > b):
			return false
		case compareInt(a, b, OpLess) != (a < b):
			return false
		case compareInt(a, b, OpGreater) && compareInt(b, a, OpGreater):
			return false
		}
		return true
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyExpressionSum checks that a generated Lua arithmetic expression
// evaluates to the same value Go computes.
func TestPropertyExpressionSum(t *testing.T) {
	prop := func(a, b int16) bool {
		src := fmt.Sprintf("%d + %d", a, b)
		v, err := ParseValue(TypeQword, src)
		return err == nil && v.Int64() == int64(a)+int64(b)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyParseValueTypeRoundTrip checks that every registered type name
// maps back to the same type.
func TestPropertyParseValueTypeRoundTrip(t *testing.T) {
	for _, d := range Types() {
		if d.Name == "" {
			continue
		}
		got, err := ParseValueType(d.Name)
		if err != nil {
			t.Errorf("ParseValueType(%q): %v", d.Name, err)
			continue
		}
		if got != d.ID {
			t.Errorf("ParseValueType(%q) = %v, want %v", d.Name, got, d.ID)
		}
	}
}
