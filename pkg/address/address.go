// Package address evaluates address expressions. The syntax is
// hexadecimal: bare tokens (148, A0, 7FF6ABCD) and $- or 0x-prefixed numbers
// are hex, names resolve as symbols or modules first, and the operators
// + - * / and parentheses are supported. A leading + or - makes the expression
// relative to a parent record's resolved address.
package address

import (
	"fmt"
	"strconv"
	"strings"
)

// Resolver resolves a name to its base address. A name may be a script symbol
// or a module; implementations should try symbols first.
type Resolver interface {
	ResolveName(name string) (uint64, bool)
}

// Eval evaluates expr. When expr begins with + or -, parent (which must be
// present) is the base; otherwise expr is absolute.
func Eval(expr string, parent uint64, hasParent bool, r Resolver) (uint64, error) {
	s := strings.TrimSpace(expr)
	if s == "" {
		return 0, fmt.Errorf("address: empty expression")
	}
	if s[0] == '+' || s[0] == '-' {
		if !hasParent {
			return 0, fmt.Errorf("address: %q is relative but has no parent", expr)
		}
		rest := strings.TrimSpace(s[1:])
		if rest == "" {
			return parent, nil
		}
		v, err := evalAbsolute(rest, r)
		if err != nil {
			return 0, err
		}
		if s[0] == '-' {
			return parent - v, nil
		}
		return parent + v, nil
	}
	return evalAbsolute(s, r)
}

// EvalOffset evaluates an offset expression as a signed value.
func EvalOffset(offset string, r Resolver) (int64, error) {
	v, err := evalAbsolute(strings.TrimSpace(offset), r)
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

func evalAbsolute(s string, r Resolver) (uint64, error) {
	toks, err := lex(s)
	if err != nil {
		return 0, err
	}
	p := &parser{toks: toks, r: r}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.peek().kind != tokEOF {
		return 0, fmt.Errorf("address: trailing input in %q", s)
	}
	return v, nil
}

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokWord
	tokOp
	tokLParen
	tokRParen
)

type token struct {
	kind tokenKind
	text string
	val  uint64
}

func lex(s string) ([]token, error) {
	var toks []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '(':
			toks = append(toks, token{kind: tokLParen, text: "("})
			i++
		case c == ')':
			toks = append(toks, token{kind: tokRParen, text: ")"})
			i++
		case c == '+' || c == '-' || c == '*' || c == '/':
			toks = append(toks, token{kind: tokOp, text: string(c)})
			i++
		case c == '$':
			j := i + 1
			for j < len(s) && isHexDigit(s[j]) {
				j++
			}
			if j == i+1 {
				return nil, fmt.Errorf("address: %q is not a hex number", s[i:])
			}
			v, err := parseHex(s[i+1 : j])
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{kind: tokNumber, text: s[i:j], val: v})
			i = j
		default:
			j := i
			for j < len(s) && isWordChar(s[j]) {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("address: unexpected character %q", string(c))
			}
			toks = append(toks, token{kind: tokWord, text: s[i:j]})
			i = j
		}
	}
	return append(toks, token{kind: tokEOF}), nil
}

type parser struct {
	toks []token
	pos  int
	r    Resolver
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) parseExpr() (uint64, error) { return p.parseAdd() }

func (p *parser) parseAdd() (uint64, error) {
	v, err := p.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		t := p.peek()
		if t.kind != tokOp || (t.text != "+" && t.text != "-") {
			return v, nil
		}
		p.next()
		rhs, err := p.parseMul()
		if err != nil {
			return 0, err
		}
		if t.text == "+" {
			v += rhs
		} else {
			v -= rhs
		}
	}
}

func (p *parser) parseMul() (uint64, error) {
	v, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		t := p.peek()
		if t.kind != tokOp || (t.text != "*" && t.text != "/") {
			return v, nil
		}
		p.next()
		rhs, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		if t.text == "*" {
			v *= rhs
		} else {
			if rhs == 0 {
				return 0, fmt.Errorf("address: division by zero")
			}
			v /= rhs
		}
	}
}

func (p *parser) parseUnary() (uint64, error) {
	t := p.peek()
	if t.kind == tokOp && (t.text == "-" || t.text == "+") {
		p.next()
		v, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		if t.text == "-" {
			return -v, nil
		}
		return v, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (uint64, error) {
	t := p.next()
	switch t.kind {
	case tokNumber:
		return t.val, nil
	case tokWord:
		return evalWord(t.text, p.r)
	case tokLParen:
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.next().kind != tokRParen {
			return 0, fmt.Errorf("address: missing )")
		}
		return v, nil
	default:
		return 0, fmt.Errorf("address: unexpected token %q", t.text)
	}
}

// evalWord resolves a name first, then falls back to interpreting an
// all-hexadecimal token as a number.
func evalWord(w string, r Resolver) (uint64, error) {
	if r != nil {
		if v, ok := r.ResolveName(w); ok {
			return v, nil
		}
	}
	if isHexLiteral(w) {
		return parseHex(w)
	}
	if len(w) > 2 && w[0] == '0' && (w[1] == 'x' || w[1] == 'X') {
		return parseHex(w[2:])
	}
	return 0, fmt.Errorf("address: unknown name %q", w)
}

func parseHex(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("address: %q is not a hex number", s)
	}
	return v, nil
}

func isHexLiteral(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isWordChar(c byte) bool {
	return c == '_' || c == '.' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
