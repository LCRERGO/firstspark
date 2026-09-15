package scan

import (
	"strconv"
	"strings"
	"testing"
)

func TestBuiltinDescriptors(t *testing.T) {
	d, ok := LookupType("dword")
	if !ok {
		t.Fatal("dword is not registered")
	}
	if d.Label != "4 Bytes" || d.Size != 4 || d.Kind != KindInt {
		t.Fatalf("unexpected descriptor: %+v", d)
	}
	if got, err := ParseValueType("DWORD"); err != nil || got != TypeDword {
		t.Fatalf("case-insensitive lookup failed: %v, %v", got, err)
	}
	if got, err := ParseValueType("int32"); err != nil || got != TypeDword {
		t.Fatalf("alias lookup failed: %v, %v", got, err)
	}
}

func TestRegisterCustomType(t *testing.T) {
	const id ValueType = 100
	RegisterType(&Type{
		ID: id, Name: "u24", Label: "3 Bytes", Size: 3, Kind: KindInt,
		Parse: func(s string) (Value, error) {
			n, err := strconv.ParseInt(strings.TrimSpace(s), 0, 64)
			if err != nil {
				return Value{}, err
			}
			return Value{Type: id, Raw: []byte{byte(n), byte(n >> 8), byte(n >> 16)}}, nil
		},
		Format: func(v Value) string { return strconv.FormatInt(decodeU24(v), 10) },
		Encode: func(n int64) []byte { return []byte{byte(n), byte(n >> 8), byte(n >> 16)} },
		Int64:  decodeU24,
	})

	if got, err := ParseValueType("u24"); err != nil || got != id {
		t.Fatalf("ParseValueType(u24) = %d, %v", got, err)
	}
	v, err := ParseValue(id, "197121")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if len(v.Raw) != 3 || v.Raw[0] != 0x01 || v.Raw[1] != 0x02 || v.Raw[2] != 0x03 {
		t.Fatalf("raw = %v", v.Raw)
	}
	if v.String() != "197121" {
		t.Fatalf("format = %q", v.String())
	}
	target, _ := ParseValue(id, "197121")
	if !TypeByID(id).Compare(v, target, OpEqual, 0) {
		t.Fatal("compare equal failed")
	}
	if TypeByID(id).Compare(v, target, OpGreater, 0) {
		t.Fatal("unexpected greater for equal values")
	}
	if !TypeByID(id).Compare(v, target, OpLessEqual, 0) {
		t.Fatal("less-or-equal failed")
	}
}

func decodeU24(v Value) int64 {
	if len(v.Raw) < 3 {
		return 0
	}
	return int64(v.Raw[0]) | int64(v.Raw[1])<<8 | int64(v.Raw[2])<<16
}
