package asm

import (
	"fmt"
	"strconv"
	"strings"
)

type opKind int

const (
	kindReg opKind = iota
	kindImm
	kindMem
)

type memOperand struct {
	base    *reg
	index   *reg
	scale   int
	disp    int64
	rip     bool
	size    int
	hasSize bool
}

type operand struct {
	kind    opKind
	reg     reg
	imm     int64
	mem     memOperand
	size    int
	hasSize bool
}

var sizeHints = []struct {
	name string
	size int
}{
	{"qword ptr", 8},
	{"dword ptr", 4},
	{"word ptr", 2},
	{"byte ptr", 1},
}

func parseOperand(tok string) (operand, error) {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return operand{}, fmt.Errorf("asm: empty operand")
	}
	var hint int
	lower := strings.ToLower(tok)
	for _, h := range sizeHints {
		if strings.HasPrefix(lower, h.name) {
			hint = h.size
			tok = strings.TrimSpace(tok[len(h.name):])
			break
		}
	}
	if strings.HasPrefix(tok, "[") {
		m, err := parseMem(tok)
		if err != nil {
			return operand{}, err
		}
		if hint != 0 {
			m.size = hint
			m.hasSize = true
		}
		return operand{kind: kindMem, mem: m, size: hint, hasSize: hint != 0}, nil
	}
	if r, ok := lookupReg(strings.ToLower(tok)); ok {
		return operand{kind: kindReg, reg: r, size: r.size, hasSize: true}, nil
	}
	n, err := parseImm(tok)
	if err != nil {
		return operand{}, err
	}
	return operand{kind: kindImm, imm: n}, nil
}

func parseMem(tok string) (memOperand, error) {
	if !strings.HasSuffix(tok, "]") {
		return memOperand{}, fmt.Errorf("asm: malformed memory operand %q", tok)
	}
	inner := strings.TrimSpace(tok[1 : len(tok)-1])
	inner = strings.ReplaceAll(inner, " ", "")
	if inner == "" {
		return memOperand{}, fmt.Errorf("asm: empty memory operand")
	}
	lower := strings.ToLower(inner)
	if strings.Contains(lower, "rip") || strings.Contains(lower, "eip") {
		inner = strings.ReplaceAll(inner, "rip", "")
		inner = strings.ReplaceAll(inner, "eip", "")
		inner = strings.ReplaceAll(inner, "RIP", "")
		inner = strings.TrimPrefix(inner, "+")
		var disp int64
		if inner != "" {
			n, err := parseImm(inner)
			if err != nil {
				return memOperand{}, err
			}
			disp = n
		}
		return memOperand{rip: true, scale: 1, disp: disp}, nil
	}

	m := memOperand{scale: 1}
	inner = strings.ReplaceAll(inner, "-", "+-")
	for _, part := range strings.Split(inner, "+") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.IndexByte(part, '*'); i >= 0 {
			rname := strings.ToLower(strings.TrimSpace(part[:i]))
			r, ok := lookupReg(rname)
			if !ok {
				return memOperand{}, fmt.Errorf("asm: unknown index register %q", rname)
			}
			if r.size == 1 {
				return memOperand{}, fmt.Errorf("asm: 8-bit index register not allowed")
			}
			scale, err := strconv.Atoi(strings.TrimSpace(part[i+1:]))
			if err != nil || (scale != 1 && scale != 2 && scale != 4 && scale != 8) {
				return memOperand{}, fmt.Errorf("asm: invalid scale in %q", part)
			}
			rr := r
			m.index = &rr
			m.scale = scale
			continue
		}
		if r, ok := lookupReg(strings.ToLower(part)); ok {
			if r.size == 1 {
				return memOperand{}, fmt.Errorf("asm: 8-bit base/index register not allowed")
			}
			rr := r
			switch {
			case m.base == nil:
				m.base = &rr
			case m.index == nil:
				m.index = &rr
			default:
				return memOperand{}, fmt.Errorf("asm: too many registers in memory operand")
			}
			continue
		}
		n, err := parseImm(part)
		if err != nil {
			return memOperand{}, err
		}
		m.disp += n
	}
	if m.base == nil && m.index == nil {
		return memOperand{}, fmt.Errorf("asm: memory operand has no register")
	}
	return m, nil
}

func parseImm(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
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
	case strings.HasPrefix(s, "0b"), strings.HasPrefix(s, "0B"):
		n, err = strconv.ParseUint(s[2:], 2, 64)
	default:
		n, err = strconv.ParseUint(s, 10, 64)
	}
	if err != nil {
		return 0, fmt.Errorf("asm: cannot parse %q as a number", s)
	}
	if neg {
		return -int64(n), nil
	}
	return int64(n), nil
}

func stripComment(line string) string {
	if i := strings.IndexAny(line, ";#"); i >= 0 {
		line = line[:i]
	}
	return line
}

func splitMnemonic(line string) (string, string) {
	if i := strings.IndexAny(line, " \t"); i >= 0 {
		return line[:i], strings.TrimSpace(line[i+1:])
	}
	return line, ""
}

func splitOperands(s string) []string {
	var (
		out   []string
		depth int
		start int
	)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}
