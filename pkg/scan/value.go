// Package scan implements a Cheat Engine style memory scanner on top of the
// mem package. It supports the standard value types, exact and unknown-value
// initial scans, and the usual change based next-scan filters.
package scan

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ValueType identifies the interpretation of a scanned value.
type ValueType int

const (
	TypeByte ValueType = iota
	TypeWord
	TypeDword
	TypeQword
	TypeFloat
	TypeDouble
	TypeString
	TypeAOB
)

// Size returns the width in bytes for fixed-width types and 0 for the
// variable-length types (string and AOB).
func (t ValueType) Size() int {
	switch t {
	case TypeByte:
		return 1
	case TypeWord:
		return 2
	case TypeDword:
		return 4
	case TypeQword, TypeDouble:
		return 8
	case TypeFloat:
		return 4
	default:
		return 0
	}
}

// Variable reports whether the type has a length supplied by the user.
func (t ValueType) Variable() bool { return t == TypeString || t == TypeAOB }

func (t ValueType) String() string {
	switch t {
	case TypeByte:
		return "byte"
	case TypeWord:
		return "word"
	case TypeDword:
		return "dword"
	case TypeQword:
		return "qword"
	case TypeFloat:
		return "float"
	case TypeDouble:
		return "double"
	case TypeString:
		return "string"
	case TypeAOB:
		return "aob"
	default:
		return "unknown"
	}
}

// ParseValueType maps a user supplied name to a ValueType.
func ParseValueType(s string) (ValueType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "byte", "int8", "1":
		return TypeByte, nil
	case "word", "int16", "short", "2":
		return TypeWord, nil
	case "dword", "int32", "int", "4":
		return TypeDword, nil
	case "qword", "int64", "long", "8":
		return TypeQword, nil
	case "float", "float32":
		return TypeFloat, nil
	case "double", "float64":
		return TypeDouble, nil
	case "string", "str":
		return TypeString, nil
	case "aob", "bytes", "array":
		return TypeAOB, nil
	default:
		return 0, fmt.Errorf("scan: unknown value type %q", s)
	}
}

// Value is a concrete value of a given type. For AOB values Mask marks which
// bytes are significant (a zero mask entry is a wildcard).
type Value struct {
	Type ValueType
	Raw  []byte
	Mask []byte
}

// NewValue builds a value by copying raw.
func NewValue(t ValueType, raw []byte) Value {
	v := Value{Type: t}
	if len(raw) > 0 {
		v.Raw = make([]byte, len(raw))
		copy(v.Raw, raw)
	}
	return v
}

// Uint64 decodes the value as an unsigned integer.
func (v Value) Uint64() uint64 {
	switch len(v.Raw) {
	case 1:
		return uint64(v.Raw[0])
	case 2:
		return uint64(binary.LittleEndian.Uint16(v.Raw))
	case 4:
		return uint64(binary.LittleEndian.Uint32(v.Raw))
	case 8:
		return binary.LittleEndian.Uint64(v.Raw)
	default:
		return 0
	}
}

// Int64 decodes the value as a sign-extended integer.
func (v Value) Int64() int64 {
	switch len(v.Raw) {
	case 1:
		return int64(int8(v.Raw[0]))
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(v.Raw)))
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(v.Raw)))
	case 8:
		return int64(binary.LittleEndian.Uint64(v.Raw))
	default:
		return 0
	}
}

// Float64 decodes the value as a float. Float types are decoded from their
// IEEE-754 representation, integer types are converted numerically.
func (v Value) Float64() float64 {
	switch v.Type {
	case TypeFloat:
		if len(v.Raw) == 4 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(v.Raw)))
		}
	case TypeDouble:
		if len(v.Raw) == 8 {
			return math.Float64frombits(binary.LittleEndian.Uint64(v.Raw))
		}
	}
	return float64(v.Int64())
}

// String returns the value rendered for display.
func (v Value) String() string {
	switch v.Type {
	case TypeString:
		return string(v.Raw)
	case TypeAOB:
		return formatBytes(v.Raw)
	case TypeFloat, TypeDouble:
		return strconv.FormatFloat(v.Float64(), 'g', -1, 64)
	default:
		return strconv.FormatInt(v.Int64(), 10)
	}
}

// EncodeValue converts a Go value into raw little-endian bytes for the type.
func EncodeValue(t ValueType, n int64) []byte {
	switch t {
	case TypeByte:
		return []byte{byte(n)}
	case TypeWord:
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(n))
		return b
	case TypeDword:
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(n))
		return b
	case TypeQword:
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(n))
		return b
	default:
		return nil
	}
}

// ParseValue converts user input into a Value of the requested type.
func ParseValue(t ValueType, input string) (Value, error) {
	input = strings.TrimSpace(input)
	switch t {
	case TypeString:
		input = strings.Trim(input, `"`)
		return NewValue(TypeString, []byte(input)), nil
	case TypeAOB:
		p, err := ParseAOB(input)
		if err != nil {
			return Value{}, err
		}
		return Value{Type: TypeAOB, Raw: p.Bytes, Mask: p.Mask}, nil
	case TypeFloat, TypeDouble:
		f, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return Value{}, fmt.Errorf("scan: parse float %q: %w", input, err)
		}
		return encodeFloat(t, f), nil
	default:
		n, err := parseInteger(input)
		if err != nil {
			return Value{}, err
		}
		return Value{Type: t, Raw: EncodeValue(t, n)}, nil
	}
}

func encodeFloat(t ValueType, f float64) Value {
	if t == TypeFloat {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, math.Float32bits(float32(f)))
		return Value{Type: TypeFloat, Raw: b}
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(f))
	return Value{Type: TypeDouble, Raw: b}
}

func parseInteger(s string) (int64, error) {
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	var (
		n   uint64
		err error
	)
	switch {
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0X"):
		n, err = strconv.ParseUint(s[2:], 16, 64)
	default:
		n, err = strconv.ParseUint(s, 10, 64)
	}
	if err != nil {
		return 0, fmt.Errorf("scan: parse integer %q: %w", s, err)
	}
	if neg {
		return -int64(n), nil
	}
	return int64(n), nil
}

func formatBytes(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = fmt.Sprintf("%02X", x)
	}
	return strings.Join(parts, " ")
}
