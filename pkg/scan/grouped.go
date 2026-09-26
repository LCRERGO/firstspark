package scan

import (
	"fmt"
	"strings"
)

// GroupSegment is one typed value of a grouped scan pattern.
type GroupSegment struct {
	Type  ValueType
	Value Value
	Any   bool
}

// GroupedPattern is a contiguous sequence of typed values matched against a
// scanned address, Cheat Engine's "grouped" value type (4:75 4:* 4:100).
type GroupedPattern struct {
	Segments []GroupSegment
}

// ParseGrouped parses a whitespace-separated list of type:value segments.
// Recognised type codes are 1/b/byte, 2/w/word, 4/dword, 8/q/qword, f/float,
// d/double and s/string. A value of * is a wildcard that matches any bytes.
func ParseGrouped(input string) (*GroupedPattern, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil, fmt.Errorf("scan: empty grouped pattern")
	}
	p := &GroupedPattern{}
	for _, f := range fields {
		code, val, ok := strings.Cut(f, ":")
		if !ok {
			return nil, fmt.Errorf("scan: grouped segment %q must be type:value", f)
		}
		t, err := groupedType(code)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(val) == "*" {
			if t.Size() == 0 {
				return nil, fmt.Errorf("scan: grouped wildcard needs a fixed-width type in %q", f)
			}
			p.Segments = append(p.Segments, GroupSegment{Type: t, Any: true})
			continue
		}
		v, err := ParseValue(t, val)
		if err != nil {
			return nil, fmt.Errorf("scan: grouped segment %q: %w", f, err)
		}
		p.Segments = append(p.Segments, GroupSegment{Type: t, Value: v})
	}
	return p, nil
}

func groupedType(code string) (ValueType, error) {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "1", "b", "byte":
		return TypeByte, nil
	case "2", "w", "word":
		return TypeWord, nil
	case "4", "dword":
		return TypeDword, nil
	case "8", "q", "qword":
		return TypeQword, nil
	case "f", "float":
		return TypeFloat, nil
	case "d", "double":
		return TypeDouble, nil
	case "s", "string":
		return TypeString, nil
	default:
		return 0, fmt.Errorf("scan: unknown grouped type %q", code)
	}
}

// Size returns the number of bytes the whole pattern occupies.
func (p *GroupedPattern) Size() int {
	n := 0
	for _, s := range p.Segments {
		if w := s.Type.Size(); w > 0 {
			n += w
			continue
		}
		n += len(s.Value.Raw)
	}
	return n
}

// Match reports whether raw starts with the pattern. Wildcard segments match
// any bytes.
func (p *GroupedPattern) Match(raw []byte) bool {
	off := 0
	for _, s := range p.Segments {
		w := s.Type.Size()
		if w == 0 {
			w = len(s.Value.Raw)
		}
		if w == 0 || off+w > len(raw) {
			return false
		}
		if !s.Any {
			t := TypeByID(s.Type)
			if t == nil {
				return false
			}
			cur := NewValue(s.Type, raw[off:off+w])
			if !t.Compare(cur, s.Value, OpEqual, 1e-6) {
				return false
			}
		}
		off += w
	}
	return true
}
