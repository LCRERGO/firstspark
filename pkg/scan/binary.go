package scan

import (
	"fmt"
	"strings"
)

// BinaryPattern is a bit-level pattern with wildcards. Bits are ordered
// least-significant first within each byte.
type BinaryPattern struct {
	Bytes []byte
	Mask  []byte
	Bits  int
}

// ParseBinary parses a bit pattern such as "1010??11".
func ParseBinary(s string) (*BinaryPattern, error) {
	p := &BinaryPattern{}
	bit := 0
	for _, r := range s {
		value, significant := byte(0), byte(1)
		switch r {
		case ' ', '\t', '_':
			continue
		case '0':
		case '1':
			value = 1
		case '?', 'x', 'X', '*':
			significant = 0
		default:
			return nil, fmt.Errorf("scan: invalid binary digit %q", string(r))
		}
		if bit%8 == 0 {
			p.Bytes = append(p.Bytes, 0)
			p.Mask = append(p.Mask, 0)
		}
		if value == 1 {
			p.Bytes[bit/8] |= 1 << uint(bit%8)
		}
		if significant == 1 {
			p.Mask[bit/8] |= 1 << uint(bit%8)
		}
		bit++
	}
	if bit == 0 {
		return nil, fmt.Errorf("scan: empty binary pattern")
	}
	p.Bits = bit
	return p, nil
}

// Match reports whether raw satisfies the pattern.
func (p *BinaryPattern) Match(raw []byte) bool {
	for i := 0; i < p.Bits; i++ {
		if p.Mask[i/8]&(1<<uint(i%8)) == 0 {
			continue
		}
		if bitAt(raw, i) != bitAt(p.Bytes, i) {
			return false
		}
	}
	return true
}

func bitAt(b []byte, i int) byte {
	if i/8 >= len(b) {
		return 0
	}
	return (b[i/8] >> uint(i%8)) & 1
}

// FormatBinary renders a binary value as a bit string.
func FormatBinary(v Value) string {
	bits := v.Bits
	if bits == 0 {
		bits = len(v.Raw) * 8
	}
	var b strings.Builder
	for i := 0; i < bits; i++ {
		if v.Mask != nil && v.Mask[i/8]&(1<<uint(i%8)) == 0 {
			b.WriteByte('?')
			continue
		}
		if bitAt(v.Raw, i) == 1 {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}
