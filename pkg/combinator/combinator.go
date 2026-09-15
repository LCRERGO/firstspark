// Package combinator provides small, dependency-free parser combinators.
// Parsers are pure functions over an immutable input position; failures record
// the furthest position reached so the top level can report a useful error.
package combinator

import "fmt"

// Input is a position in a source string.
type Input struct {
	src  []rune
	pos  int
	line int
	col  int
	fail *failure
}

type failure struct {
	pos, line, col int
	msg            string
	set            bool
}

// Parser consumes input and returns a value, the remaining input and whether
// it matched. On failure the returned input is the original position.
type Parser[T any] func(Input) (T, Input, bool)

// New returns the initial input for src.
func New(src string) Input {
	return Input{src: []rune(src), pos: 0, line: 1, col: 1, fail: &failure{}}
}

// Pos, Line and Col report the current position.
func (in Input) Pos() int  { return in.pos }
func (in Input) Line() int { return in.line }
func (in Input) Col() int  { return in.col }

// EOF reports whether the input is exhausted.
func (in Input) EOF() bool { return in.pos >= len(in.src) }

func (in Input) peek() (rune, bool) {
	if in.pos >= len(in.src) {
		return 0, false
	}
	return in.src[in.pos], true
}

func (in Input) advance() Input {
	if in.pos >= len(in.src) {
		return in
	}
	next := in
	if in.src[in.pos] == '\n' {
		next.line++
		next.col = 1
	} else {
		next.col++
	}
	next.pos++
	return next
}

func (in Input) failHere(msg string) {
	f := in.fail
	if f.set && f.pos > in.pos {
		return
	}
	f.pos, f.line, f.col, f.msg, f.set = in.pos, in.line, in.col, msg, true
}

// Err returns the furthest recorded failure, or nil.
func (in Input) Err() error {
	if !in.fail.set {
		return nil
	}
	return &ParseError{Line: in.fail.line, Col: in.fail.col, Msg: in.fail.msg}
}

// ParseError describes where parsing failed.
type ParseError struct {
	Line, Col int
	Msg       string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg)
}

// Run applies p to src and requires all input to be consumed.
func Run[T any](p Parser[T], src string) (T, error) {
	in := New(src)
	v, rest, ok := p(in)
	if !ok {
		var zero T
		if err := in.Err(); err != nil {
			return zero, err
		}
		return zero, &ParseError{Line: in.line, Col: in.col, Msg: "parse error"}
	}
	if !rest.EOF() {
		rest.failHere("unexpected input")
		var zero T
		return zero, rest.Err()
	}
	return v, nil
}

// Rune matches a single rune satisfying pred.
func Rune(pred func(rune) bool) Parser[rune] {
	return func(in Input) (rune, Input, bool) {
		r, ok := in.peek()
		if !ok {
			in.failHere("unexpected end of input")
			return 0, in, false
		}
		if !pred(r) {
			in.failHere(fmt.Sprintf("unexpected %q", string(r)))
			return 0, in, false
		}
		return r, in.advance(), true
	}
}

// RuneLit matches exactly want.
func RuneLit(want rune) Parser[rune] {
	return func(in Input) (rune, Input, bool) {
		r, ok := in.peek()
		if !ok || r != want {
			in.failHere(fmt.Sprintf("expected %q", string(want)))
			return 0, in, false
		}
		return r, in.advance(), true
	}
}

// Str matches the literal string s.
func Str(s string) Parser[string] {
	return func(in Input) (string, Input, bool) {
		cur := in
		for _, want := range s {
			r, ok := cur.peek()
			if !ok || r != want {
				in.failHere(fmt.Sprintf("expected %q", s))
				return "", in, false
			}
			cur = cur.advance()
		}
		return s, cur, true
	}
}

// Any matches any single rune.
func Any() Parser[rune] { return Rune(func(rune) bool { return true }) }

// Eof matches only at the end of input.
func Eof() Parser[struct{}] {
	return func(in Input) (struct{}, Input, bool) {
		if in.EOF() {
			return struct{}{}, in, true
		}
		in.failHere("expected end of input")
		return struct{}{}, in, false
	}
}

// Fail always fails with msg.
func Fail[T any](msg string) Parser[T] {
	return func(in Input) (T, Input, bool) {
		var zero T
		in.failHere(msg)
		return zero, in, false
	}
}

// Map transforms a parser's result.
func Map[A, B any](p Parser[A], f func(A) B) Parser[B] {
	return func(in Input) (B, Input, bool) {
		v, rest, ok := p(in)
		if !ok {
			var zero B
			return zero, in, false
		}
		return f(v), rest, true
	}
}

// Bind sequences a parser with a function producing the next parser. It
// backtracks to the original position if the continuation fails.
func Bind[A, B any](p Parser[A], f func(A) Parser[B]) Parser[B] {
	return func(in Input) (B, Input, bool) {
		v, rest, ok := p(in)
		if !ok {
			var zero B
			return zero, in, false
		}
		b, rest2, ok := f(v)(rest)
		if !ok {
			var zero B
			return zero, in, false
		}
		return b, rest2, true
	}
}

// Pure succeeds with v without consuming input.
func Pure[T any](v T) Parser[T] {
	return func(in Input) (T, Input, bool) { return v, in, true }
}

// TakeWhile consumes runes satisfying pred.
func TakeWhile(pred func(rune) bool) Parser[string] {
	return func(in Input) (string, Input, bool) {
		cur := in
		var out []rune
		for {
			r, ok := cur.peek()
			if !ok || !pred(r) {
				return string(out), cur, true
			}
			out = append(out, r)
			cur = cur.advance()
		}
	}
}

// TakeWhile1 consumes one or more runes satisfying pred.
func TakeWhile1(pred func(rune) bool) Parser[string] {
	return func(in Input) (string, Input, bool) {
		s, rest, ok := TakeWhile(pred)(in)
		if !ok || s == "" {
			in.failHere("expected input")
			return "", in, false
		}
		return s, rest, true
	}
}

// TakeWhileN consumes up to n runes satisfying pred.
func TakeWhileN(pred func(rune) bool, n int) Parser[string] {
	return func(in Input) (string, Input, bool) {
		cur := in
		var out []rune
		for len(out) < n {
			r, ok := cur.peek()
			if !ok || !pred(r) {
				break
			}
			out = append(out, r)
			cur = cur.advance()
		}
		return string(out), cur, true
	}
}

// TakeUntil consumes runes until term matches, leaving term unconsumed.
func TakeUntil[T any](term Parser[T]) Parser[string] {
	return func(in Input) (string, Input, bool) {
		cur := in
		var out []rune
		for {
			if _, _, ok := term(cur); ok {
				return string(out), cur, true
			}
			r, ok := cur.peek()
			if !ok {
				return string(out), cur, true
			}
			out = append(out, r)
			cur = cur.advance()
		}
	}
}

// Choice returns the first matching parser.
func Choice[T any](ps ...Parser[T]) Parser[T] {
	return func(in Input) (T, Input, bool) {
		for _, p := range ps {
			if v, rest, ok := p(in); ok {
				return v, rest, true
			}
		}
		var zero T
		return zero, in, false
	}
}

// Many applies p zero or more times. It never fails.
func Many[T any](p Parser[T]) Parser[[]T] {
	return func(in Input) ([]T, Input, bool) {
		var out []T
		cur := in
		for {
			v, rest, ok := p(cur)
			if !ok || rest.pos == cur.pos {
				return out, cur, true
			}
			out = append(out, v)
			cur = rest
		}
	}
}

// Many1 applies p one or more times.
func Many1[T any](p Parser[T]) Parser[[]T] {
	return func(in Input) ([]T, Input, bool) {
		first, rest, ok := p(in)
		if !ok {
			return nil, in, false
		}
		restOf, cur, _ := Many(p)(rest)
		return append([]T{first}, restOf...), cur, true
	}
}

// Optional applies p, returning def if it does not match.
func Optional[T any](p Parser[T], def T) Parser[T] {
	return func(in Input) (T, Input, bool) {
		v, rest, ok := p(in)
		if !ok {
			return def, in, true
		}
		return v, rest, true
	}
}

// SepBy parses zero or more p separated by sep.
func SepBy[T, S any](p Parser[T], sep Parser[S]) Parser[[]T] {
	return func(in Input) ([]T, Input, bool) {
		first, cur, ok := p(in)
		if !ok {
			return nil, in, true
		}
		out := []T{first}
		for {
			_, afterSep, ok := sep(cur)
			if !ok {
				return out, cur, true
			}
			v, afterItem, ok := p(afterSep)
			if !ok {
				return out, cur, true
			}
			out = append(out, v)
			cur = afterItem
		}
	}
}

// SepBy1 parses one or more p separated by sep.
func SepBy1[T, S any](p Parser[T], sep Parser[S]) Parser[[]T] {
	return func(in Input) ([]T, Input, bool) {
		first, cur, ok := p(in)
		if !ok {
			return nil, in, false
		}
		out := []T{first}
		for {
			_, afterSep, ok := sep(cur)
			if !ok {
				return out, cur, true
			}
			v, afterItem, ok := p(afterSep)
			if !ok {
				return out, cur, true
			}
			out = append(out, v)
			cur = afterItem
		}
	}
}

// Lazy defers construction of a parser until its first use, allowing
// recursive grammars. The parser is built once and reused.
func Lazy[T any](f func() Parser[T]) Parser[T] {
	var p Parser[T]
	return func(in Input) (T, Input, bool) {
		if p == nil {
			p = f()
		}
		return p(in)
	}
}

// Label names a parser for error messages.
func Label[T any](p Parser[T], name string) Parser[T] {
	return func(in Input) (T, Input, bool) {
		v, rest, ok := p(in)
		if !ok {
			in.failHere(name)
			var zero T
			return zero, in, false
		}
		return v, rest, true
	}
}

// Peek matches p without consuming input.
func Peek[T any](p Parser[T]) Parser[T] {
	return func(in Input) (T, Input, bool) {
		v, _, ok := p(in)
		if !ok {
			var zero T
			return zero, in, false
		}
		return v, in, true
	}
}

// Not matches only when p does not, consuming nothing.
func Not[T any](p Parser[T]) Parser[struct{}] {
	return func(in Input) (struct{}, Input, bool) {
		if _, _, ok := p(in); ok {
			in.failHere("unexpected input")
			return struct{}{}, in, false
		}
		return struct{}{}, in, true
	}
}

// Skip discards the value of p.
func Skip[T any](p Parser[T]) Parser[struct{}] {
	return Map(p, func(T) struct{} { return struct{}{} })
}

// Pair is a two-element tuple.
type Pair[A, B any] struct {
	A A
	B B
}

// Triple is a three-element tuple.
type Triple[A, B, C any] struct {
	A A
	B B
	C C
}

// Seq2 sequences two parsers.
func Seq2[A, B any](pa Parser[A], pb Parser[B]) Parser[Pair[A, B]] {
	return func(in Input) (Pair[A, B], Input, bool) {
		a, rest, ok := pa(in)
		if !ok {
			return Pair[A, B]{}, in, false
		}
		b, rest2, ok := pb(rest)
		if !ok {
			return Pair[A, B]{}, in, false
		}
		return Pair[A, B]{A: a, B: b}, rest2, true
	}
}

// Seq3 sequences three parsers.
func Seq3[A, B, C any](pa Parser[A], pb Parser[B], pc Parser[C]) Parser[Triple[A, B, C]] {
	return func(in Input) (Triple[A, B, C], Input, bool) {
		a, rest, ok := pa(in)
		if !ok {
			return Triple[A, B, C]{}, in, false
		}
		b, rest2, ok := pb(rest)
		if !ok {
			return Triple[A, B, C]{}, in, false
		}
		c, rest3, ok := pc(rest2)
		if !ok {
			return Triple[A, B, C]{}, in, false
		}
		return Triple[A, B, C]{A: a, B: b, C: c}, rest3, true
	}
}

// Between parses open, p, close and keeps p's value.
func Between[A, B, C any](open Parser[A], p Parser[B], close Parser[C]) Parser[B] {
	return Map(Seq3(open, p, close), func(t Triple[A, B, C]) B { return t.B })
}
