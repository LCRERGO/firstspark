package scan

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
)

// Kind is the comparison class of a value type.
type Kind uint8

const (
	// KindInt compares decoded integer values.
	KindInt Kind = iota
	// KindFloat compares decoded floating point values.
	KindFloat
	// KindString compares string bytes.
	KindString
	// KindBytes compares raw bytes.
	KindBytes
	// KindBinary compares bit patterns.
	KindBinary
)

// Type describes how a value type is parsed, formatted, encoded and compared.
// Built-ins are registered at startup; user-defined types are registered from
// pkg/customtype. The ValueType enum remains as the stable identifier used by
// Cheat Engine tables.
type Type struct {
	ID        ValueType
	Name      string
	Label     string
	Size      int
	Variable  bool
	Alignment int
	Kind      Kind
	Parse     func(input string) (Value, error)
	Format    func(v Value) string
	Encode    func(n int64) []byte
	Int64     func(v Value) int64
	Numeric   func(v Value) float64
	Text      func(v Value) string
}

var nextCustomID ValueType = 100

// NextTypeID allocates a fresh identifier for a user-defined type.
func NextTypeID() ValueType {
	typeMu.Lock()
	defer typeMu.Unlock()
	id := nextCustomID
	nextCustomID++
	return id
}

var (
	typeMu      sync.RWMutex
	typesByID   = map[ValueType]*Type{}
	typesByName = map[string]*Type{}
)

// RegisterType adds or replaces a type descriptor.
func RegisterType(t *Type) {
	typeMu.Lock()
	defer typeMu.Unlock()
	typesByID[t.ID] = t
	typesByName[strings.ToLower(t.Name)] = t
}

// TypeByID returns the descriptor for id, or nil.
func TypeByID(id ValueType) *Type {
	typeMu.RLock()
	defer typeMu.RUnlock()
	return typesByID[id]
}

// UnregisterType removes a type from the registry.
func UnregisterType(id ValueType) {
	typeMu.Lock()
	defer typeMu.Unlock()
	if t, ok := typesByID[id]; ok {
		delete(typesByName, strings.ToLower(t.Name))
		delete(typesByID, id)
	}
}

// LookupType finds a registered type by name (case-insensitive).
func LookupType(name string) (*Type, bool) {
	typeMu.RLock()
	defer typeMu.RUnlock()
	t, ok := typesByName[strings.ToLower(strings.TrimSpace(name))]
	return t, ok
}

// Types returns every registered type ordered by ID.
func Types() []*Type {
	typeMu.RLock()
	defer typeMu.RUnlock()
	out := make([]*Type, 0, len(typesByID))
	for _, t := range typesByID {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Compare applies op to two values of this type. String, byte and binary
// types support only equality; integer and float types compare numerically.
func (t *Type) Compare(a, b Value, op CompareOp, eps float64) bool {
	switch t.Kind {
	case KindString:
		x, y := t.text(a), t.text(b)
		switch op {
		case OpEqual:
			return x == y
		case OpNotEqual:
			return x != y
		case OpLess:
			return x < y
		case OpLessEqual:
			return x <= y
		case OpGreater:
			return x > y
		case OpGreaterEqual:
			return x >= y
		default:
			return false
		}
	case KindBytes, KindBinary:
		switch op {
		case OpEqual:
			return bytes.Equal(a.Raw, b.Raw)
		case OpNotEqual:
			return !bytes.Equal(a.Raw, b.Raw)
		default:
			return false
		}
	default:
		if t.Kind == KindFloat {
			return compareFloat(t.Numeric(a), t.Numeric(b), op, eps)
		}
		return compareInt(t.Int64(a), t.Int64(b), op)
	}
}

func (t *Type) text(v Value) string {
	if t.Text != nil {
		return t.Text(v)
	}
	return string(v.Raw)
}

func compareInt(a, b int64, op CompareOp) bool {
	switch op {
	case OpEqual:
		return a == b
	case OpNotEqual:
		return a != b
	case OpGreater:
		return a > b
	case OpGreaterEqual:
		return a >= b
	case OpLess:
		return a < b
	case OpLessEqual:
		return a <= b
	default:
		return false
	}
}

func compareFloat(a, b float64, op CompareOp, eps float64) bool {
	switch op {
	case OpEqual:
		return math.Abs(a-b) <= eps
	case OpNotEqual:
		return math.Abs(a-b) > eps
	case OpGreater:
		return a > b+eps
	case OpGreaterEqual:
		return a >= b-eps
	case OpLess:
		return a < b-eps
	case OpLessEqual:
		return a <= b+eps
	default:
		return false
	}
}

func registerBuiltins() {
	integer := func(id ValueType, name, label string, size int) *Type {
		return &Type{
			ID: id, Name: name, Label: label, Size: size, Kind: KindInt,
			Parse: func(input string) (Value, error) {
				n, err := parseInteger(input)
				if err != nil {
					return Value{}, err
				}
				return Value{Type: id, Raw: encodeInteger(id, n)}, nil
			},
			Format:  func(v Value) string { return strconv.FormatInt(v.Int64(), 10) },
			Encode:  func(n int64) []byte { return encodeInteger(id, n) },
			Int64:   func(v Value) int64 { return v.Int64() },
			Numeric: func(v Value) float64 { return float64(v.Int64()) },
		}
	}
	float := func(id ValueType, name, label string, size int) *Type {
		return &Type{
			ID: id, Name: name, Label: label, Size: size, Kind: KindFloat,
			Parse: func(input string) (Value, error) {
				f, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
				if err != nil {
					return Value{}, fmt.Errorf("scan: parse float %q: %w", input, err)
				}
				return encodeFloat(id, f), nil
			},
			Format:  func(v Value) string { return strconv.FormatFloat(v.Float64(), 'g', -1, 64) },
			Encode:  func(n int64) []byte { return encodeFloat(id, float64(n)).Raw },
			Numeric: func(v Value) float64 { return v.Float64() },
		}
	}
	RegisterType(integer(TypeByte, "byte", "Byte", 1))
	RegisterType(integer(TypeWord, "word", "2 Bytes", 2))
	RegisterType(integer(TypeDword, "dword", "4 Bytes", 4))
	RegisterType(integer(TypeQword, "qword", "8 Bytes", 8))
	RegisterType(float(TypeFloat, "float", "Float", 4))
	RegisterType(float(TypeDouble, "double", "Double", 8))
	RegisterType(&Type{
		ID: TypeString, Name: "string", Label: "Text", Variable: true, Kind: KindString,
		Parse: func(input string) (Value, error) {
			input = strings.TrimSpace(input)
			input = strings.Trim(input, `"`)
			return NewValue(TypeString, []byte(input)), nil
		},
		Format:  func(v Value) string { return string(v.Raw) },
		Encode:  func(n int64) []byte { return []byte(strconv.FormatInt(n, 10)) },
		Numeric: func(v Value) float64 { return 0 },
		Text:    func(v Value) string { return string(v.Raw) },
	})
	RegisterType(&Type{
		ID: TypeAOB, Name: "aob", Label: "Array of Bytes", Variable: true, Kind: KindBytes,
		Parse: func(input string) (Value, error) {
			p, err := ParseAOB(input)
			if err != nil {
				return Value{}, err
			}
			return Value{Type: TypeAOB, Raw: p.Bytes, Mask: p.Mask}, nil
		},
		Format:  func(v Value) string { return formatBytes(v.Raw) },
		Encode:  func(n int64) []byte { return []byte{byte(n)} },
		Numeric: func(v Value) float64 { return 0 },
	})
	RegisterType(&Type{
		ID: TypeBinary, Name: "binary", Label: "Binary", Variable: true, Kind: KindBinary,
		Parse: func(input string) (Value, error) {
			p, err := ParseBinary(input)
			if err != nil {
				return Value{}, err
			}
			return Value{Type: TypeBinary, Raw: p.Bytes, Mask: p.Mask, Bits: p.Bits}, nil
		},
		Format:  FormatBinary,
		Encode:  func(n int64) []byte { return []byte{byte(n)} },
		Numeric: func(v Value) float64 { return 0 },
	})
	stringType := func(id ValueType, name, label string, enc func(string) []byte, dec func([]byte) string) {
		RegisterType(&Type{
			ID: id, Name: name, Label: label, Variable: true, Kind: KindString,
			Parse: func(input string) (Value, error) {
				input = strings.Trim(strings.TrimSpace(input), `"`)
				return NewValue(id, enc(input)), nil
			},
			Format:  func(v Value) string { return dec(v.Raw) },
			Encode:  func(n int64) []byte { return enc(strconv.FormatInt(n, 10)) },
			Numeric: func(v Value) float64 { return 0 },
			Text:    func(v Value) string { return dec(v.Raw) },
		})
	}
	stringType(TypeUTF16LE, "utf16le", "UTF-16 LE", encodeUTF16LE, decodeUTF16LE)
	stringType(TypeUTF16BE, "utf16be", "UTF-16 BE", encodeUTF16BE, decodeUTF16BE)
	stringType(TypeUTF32LE, "utf32le", "UTF-32 LE", encodeUTF32LE, decodeUTF32LE)
	stringType(TypeUTF32BE, "utf32be", "UTF-32 BE", encodeUTF32BE, decodeUTF32BE)
	RegisterType(&Type{
		ID: TypeAll, Name: "all", Label: "All", Size: 8, Kind: KindInt,
		Parse: func(input string) (Value, error) {
			n, err := parseInteger(input)
			if err != nil {
				return Value{}, err
			}
			return Value{Type: TypeAll, Raw: encodeInteger(TypeQword, n)}, nil
		},
		Format:  func(v Value) string { return strconv.FormatInt(v.Int64(), 10) },
		Encode:  func(n int64) []byte { return encodeInteger(TypeQword, n) },
		Int64:   func(v Value) int64 { return v.Int64() },
		Numeric: func(v Value) float64 { return float64(v.Int64()) },
	})
}

func encodeUTF16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[i*2:], u)
	}
	return b
}

func encodeUTF16BE(s string) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		binary.BigEndian.PutUint16(b[i*2:], u)
	}
	return b
}

func decodeUTF16LE(b []byte) string {
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+2 <= len(b); i += 2 {
		units = append(units, binary.LittleEndian.Uint16(b[i:]))
	}
	return string(utf16.Decode(units))
}

func decodeUTF16BE(b []byte) string {
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+2 <= len(b); i += 2 {
		units = append(units, binary.BigEndian.Uint16(b[i:]))
	}
	return string(utf16.Decode(units))
}

func encodeUTF32LE(s string) []byte {
	runes := []rune(s)
	b := make([]byte, len(runes)*4)
	for i, r := range runes {
		binary.LittleEndian.PutUint32(b[i*4:], uint32(r))
	}
	return b
}

func encodeUTF32BE(s string) []byte {
	runes := []rune(s)
	b := make([]byte, len(runes)*4)
	for i, r := range runes {
		binary.BigEndian.PutUint32(b[i*4:], uint32(r))
	}
	return b
}

func decodeUTF32LE(b []byte) string {
	var sb strings.Builder
	for i := 0; i+4 <= len(b); i += 4 {
		sb.WriteRune(rune(binary.LittleEndian.Uint32(b[i:])))
	}
	return sb.String()
}

func decodeUTF32BE(b []byte) string {
	var sb strings.Builder
	for i := 0; i+4 <= len(b); i += 4 {
		sb.WriteRune(rune(binary.BigEndian.Uint32(b[i:])))
	}
	return sb.String()
}

func init() { registerBuiltins() }
