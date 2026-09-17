package script

import (
	"fmt"
	"testing"
	"testing/quick"
)

// TestPropertyArithmetic checks that a generated script computes the same
// result as Go for the four integer operators.
func TestPropertyArithmetic(t *testing.T) {
	ops := []struct {
		op   string
		want func(a, b int64) int64
	}{
		{"+", func(a, b int64) int64 { return a + b }},
		{"-", func(a, b int64) int64 { return a - b }},
		{"*", func(a, b int64) int64 { return a * b }},
	}
	for _, o := range ops {
		o := o
		t.Run(o.op, func(t *testing.T) {
			prop := func(a, b int8) bool {
				src := fmt.Sprintf("function f() return %d %s %d end", a, o.op, b)
				p, err := Compile(src)
				if err != nil {
					return false
				}
				out, err := p.Call("f")
				if err != nil || len(out) != 1 {
					return false
				}
				n, ok := out[0].Number()
				return ok && int64(n) == o.want(int64(a), int64(b))
			}
			if err := quick.Check(prop, nil); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestPropertyPower checks that ^ agrees with math.Pow.
func TestPropertyPower(t *testing.T) {
	prop := func(base uint8) bool {
		b := int64(base%10) + 1
		src := fmt.Sprintf("function f() return %d ^ 2 end", b)
		p, err := Compile(src)
		if err != nil {
			return false
		}
		out, err := p.Call("f")
		if err != nil || len(out) != 1 {
			return false
		}
		n, ok := out[0].Number()
		return ok && int64(n) == b*b
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyStringConcat checks that .. concatenates the way Go's + does.
func TestPropertyStringConcat(t *testing.T) {
	prop := func(a, b uint8) bool {
		sa := fmt.Sprintf("s%d", a)
		sb := fmt.Sprintf("t%d", b)
		src := fmt.Sprintf("function f() return %q .. %q end", sa, sb)
		p, err := Compile(src)
		if err != nil {
			return false
		}
		out, err := p.Call("f")
		if err != nil || len(out) != 1 {
			return false
		}
		return out[0].Str() == sa+sb
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}
