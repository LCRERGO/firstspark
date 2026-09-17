package combinator

import "testing"

func FuzzRunTakeWhile(f *testing.F) {
	f.Add("12345")
	f.Add("")
	f.Add("abc")
	p := TakeWhile(func(r rune) bool { return r >= '0' && r <= '9' })
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = Run(p, s)
	})
}

func FuzzRunSepBy(f *testing.F) {
	f.Add("1,2,3")
	f.Add("")
	f.Add(",,")
	digit := TakeWhile1(func(r rune) bool { return r >= '0' && r <= '9' })
	comma := RuneLit(',')
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = Run(SepBy(digit, comma), s)
	})
}
