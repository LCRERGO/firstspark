package debugger

import (
	"fmt"
	"strings"
)

// miRecord is one parsed MI output record. kind is '^' (result), '*' (exec
// async), '+' (status) or '=' (notify).
type miRecord struct {
	kind    byte
	token   string
	class   string
	results map[string]any
}

// parseMILine parses an MI record line. It returns false for prompts, blank
// lines and stream records (the caller handles streams separately).
func parseMILine(line string) (miRecord, bool) {
	line = strings.TrimRight(line, "\r\n")
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i >= len(line) {
		return miRecord{}, false
	}
	kind := line[i]
	if strings.IndexByte("^*+=", kind) < 0 {
		return miRecord{}, false
	}
	class, results := splitMIClass(line[i+1:])
	return miRecord{kind: kind, token: line[:i], class: class, results: results}, true
}

func splitMIClass(s string) (string, map[string]any) {
	i := 0
	for i < len(s) && (isMIWordByte(s[i])) {
		i++
	}
	class := s[:i]
	results := map[string]any{}
	if i < len(s) && s[i] == ',' {
		p := &miParser{s: s[i+1:]}
		if m, err := p.parseResults(); err == nil {
			results = m
		}
	}
	return class, results
}

func isMIWordByte(c byte) bool {
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// miParser is a recursive-descent parser for MI result values.
type miParser struct {
	s string
	i int
}

func (p *miParser) parseResults() (map[string]any, error) {
	m := map[string]any{}
	for p.i < len(p.s) {
		key := p.parseKey()
		if key == "" {
			return m, fmt.Errorf("mi: empty result key")
		}
		if p.i < len(p.s) && p.s[p.i] == '=' {
			p.i++
		} else {
			return m, fmt.Errorf("mi: missing '=' after %q", key)
		}
		v, err := p.parseValue()
		if err != nil {
			return m, err
		}
		m[key] = v
		if p.i < len(p.s) && p.s[p.i] == ',' {
			p.i++
			continue
		}
		break
	}
	return m, nil
}

func (p *miParser) parseValue() (any, error) {
	if p.i >= len(p.s) {
		return "", nil
	}
	switch p.s[p.i] {
	case '{':
		return p.parseTuple()
	case '[':
		return p.parseList()
	case '"':
		return p.parseCString()
	default:
		return p.parseBare(), nil
	}
}

func (p *miParser) parseTuple() (map[string]any, error) {
	p.i++ // consume '{'
	m := map[string]any{}
	for {
		if p.i >= len(p.s) {
			return m, fmt.Errorf("mi: unterminated tuple")
		}
		switch p.s[p.i] {
		case '}':
			p.i++
			return m, nil
		case ',':
			p.i++
			continue
		}
		key := p.parseKey()
		if p.i < len(p.s) && p.s[p.i] == '=' {
			p.i++
		} else {
			return m, fmt.Errorf("mi: missing '=' in tuple")
		}
		v, err := p.parseValue()
		if err != nil {
			return m, err
		}
		m[key] = v
	}
}

func (p *miParser) parseList() ([]any, error) {
	p.i++ // consume '['
	var out []any
	for {
		if p.i >= len(p.s) {
			return out, fmt.Errorf("mi: unterminated list")
		}
		switch p.s[p.i] {
		case ']':
			p.i++
			return out, nil
		case ',':
			p.i++
			continue
		}
		v, err := p.parseValue()
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
}

func (p *miParser) parseCString() (string, error) {
	p.i++ // consume '"'
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\\' && p.i+1 < len(p.s) {
			p.i++
			switch p.s[p.i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(p.s[p.i])
			}
			p.i++
			continue
		}
		if c == '"' {
			p.i++
			return b.String(), nil
		}
		b.WriteByte(c)
		p.i++
	}
	return b.String(), fmt.Errorf("mi: unterminated string")
}

func (p *miParser) parseKey() string {
	start := p.i
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case '=', ',', '}', ']':
			return strings.TrimSpace(p.s[start:p.i])
		}
		p.i++
	}
	return strings.TrimSpace(p.s[start:p.i])
}

func (p *miParser) parseBare() string {
	start := p.i
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ',', '}', ']':
			return strings.TrimSpace(p.s[start:p.i])
		}
		p.i++
	}
	return strings.TrimSpace(p.s[start:p.i])
}

// miString returns a string result value, or "".
func miString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
