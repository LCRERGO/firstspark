package combinator

import "testing"

func TestStrAndSeq(t *testing.T) {
	p := Seq2(Str("ab"), Str("cd"))
	got, err := Run(p, "abcd")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.A != "ab" || got.B != "cd" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestChoiceBacktracks(t *testing.T) {
	p := Choice(Str("abc"), Str("abd"))
	got, err := Run(p, "abd")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "abd" {
		t.Fatalf("got %q", got)
	}
}

func TestManyAndSepBy(t *testing.T) {
	digit := Rune(func(r rune) bool { return r >= '0' && r <= '9' })
	got, err := Run(Many1(digit), "123")
	if err != nil || len(got) != 3 {
		t.Fatalf("Many1: %v %v", got, err)
	}
	nums := SepBy(Str("a"), Str(","))
	got2, err := Run(nums, "a,a,a")
	if err != nil || len(got2) != 3 {
		t.Fatalf("SepBy: %v %v", got2, err)
	}
	empty, err := Run(nums, "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("SepBy empty: %v %v", empty, err)
	}
}

func TestRunRejectsTrailing(t *testing.T) {
	_, err := Run(Str("a"), "ab")
	if err == nil {
		t.Fatal("expected trailing input error")
	}
}

func TestErrorPosition(t *testing.T) {
	_, err := Run(Seq2(Str("ab"), Str("cd")), "abXX")
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Line != 1 || pe.Col != 3 {
		t.Fatalf("expected 1:3, got %d:%d", pe.Line, pe.Col)
	}
}

func TestLazyRecursion(t *testing.T) {
	var nested Parser[int]
	nested = Lazy(func() Parser[int] {
		return Choice(
			Map(Str("x"), func(string) int { return 1 }),
			Map(Seq3(Str("("), nested, Str(")")), func(t Triple[string, int, string]) int { return t.B + 1 }),
		)
	})
	got, err := Run(nested, "((x))")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != 3 {
		t.Fatalf("got %d", got)
	}
}
