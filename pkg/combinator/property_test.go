package combinator

import (
	"strings"
	"testing"
	"testing/quick"
)

func digitParser() Parser[string] {
	return TakeWhile1(func(r rune) bool { return r >= '0' && r <= '9' })
}

// TestPropertyTakeWhileDigits checks the parser accepts exactly the digit
// strings it should and returns them unchanged.
func TestPropertyTakeWhileDigits(t *testing.T) {
	prop := func(seed []byte) bool {
		if len(seed) == 0 {
			return true
		}
		var b strings.Builder
		for _, x := range seed {
			b.WriteByte('0' + x%10)
		}
		s := b.String()
		got, err := Run(digitParser(), s)
		return err == nil && got == s
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyStrMatchesLiteral checks a literal parser only accepts its own
// string.
func TestPropertyStrMatchesLiteral(t *testing.T) {
	prop := func(seed []byte) bool {
		if len(seed) == 0 || len(seed) > 32 {
			return true
		}
		var b strings.Builder
		for _, x := range seed {
			b.WriteRune(rune('a' + x%26))
		}
		s := b.String()
		got, err := Run(Str(s), s)
		return err == nil && got == s
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}
