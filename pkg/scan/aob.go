package scan

import (
	"fmt"
	"strconv"
	"strings"
)

// AOBPattern is a byte pattern where wildcard positions are ignored.
type AOBPattern struct {
	Bytes []byte
	Mask  []byte // non-zero means the byte is significant
}

// Len returns the number of bytes in the pattern.
func (p *AOBPattern) Len() int { return len(p.Bytes) }

// ParseAOB parses a pattern such as "48 8B ?? E5" or "488B??E5".
func ParseAOB(s string) (*AOBPattern, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	if len(fields) == 1 && len(fields[0]) > 2 && !strings.ContainsAny(fields[0], "?*") {
		// Compact form without separators, e.g. 488BE5.
		raw := fields[0]
		if len(raw)%2 != 0 {
			return nil, fmt.Errorf("scan: aob %q has odd length", s)
		}
		fields = make([]string, 0, len(raw)/2)
		for i := 0; i < len(raw); i += 2 {
			fields = append(fields, raw[i:i+2])
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("scan: empty aob pattern")
	}
	p := &AOBPattern{
		Bytes: make([]byte, len(fields)),
		Mask:  make([]byte, len(fields)),
	}
	for i, f := range fields {
		switch f {
		case "?", "??", "*":
			p.Bytes[i] = 0
			p.Mask[i] = 0
			continue
		}
		f = strings.TrimPrefix(strings.TrimPrefix(f, "0x"), "0X")
		if len(f) == 1 {
			f = "0" + f
		}
		b, err := strconv.ParseUint(f, 16, 8)
		if err != nil {
			return nil, fmt.Errorf("scan: bad aob byte %q: %w", f, err)
		}
		p.Bytes[i] = byte(b)
		p.Mask[i] = 0xFF
	}
	return p, nil
}

// Match reports whether data starts with the pattern.
func (p *AOBPattern) Match(data []byte) bool {
	if len(data) < len(p.Bytes) {
		return false
	}
	for i := range p.Bytes {
		if p.Mask[i] != 0 && data[i] != p.Bytes[i] {
			return false
		}
	}
	return true
}
