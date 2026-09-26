package scan

import "testing"

func TestParseGrouped(t *testing.T) {
	p, err := ParseGrouped("4:75 4:* 4:100")
	if err != nil {
		t.Fatalf("ParseGrouped: %v", err)
	}
	if p.Size() != 12 {
		t.Fatalf("size = %d, want 12", p.Size())
	}
	raw := encodeInteger(TypeDword, 75)
	raw = append(raw, 0xDE, 0xAD, 0xBE, 0xEF)
	raw = append(raw, encodeInteger(TypeDword, 100)...)
	if !p.Match(raw) {
		t.Fatal("expected the pattern to match")
	}
	raw[0] = 76
	if p.Match(raw) {
		t.Fatal("expected a mismatch after changing the first segment")
	}
}

func TestParseGroupedRejectsBadInput(t *testing.T) {
	if _, err := ParseGrouped(""); err == nil {
		t.Fatal("expected an error for an empty pattern")
	}
	if _, err := ParseGrouped("75"); err == nil {
		t.Fatal("expected an error for a segment without a type")
	}
	if _, err := ParseGrouped("4:notanumber"); err == nil {
		t.Fatal("expected an error for a bad value")
	}
	if _, err := ParseGrouped("s:*"); err == nil {
		t.Fatal("expected an error for a wildcard string")
	}
}

func TestMatchBiggerSmaller(t *testing.T) {
	bigger := &Session{opts: Options{
		Type:  TypeDword,
		Mode:  ModeBigger,
		Value: NewValue(TypeDword, encodeInteger(TypeDword, 10)),
	}}
	if !bigger.matchExact(encodeInteger(TypeDword, 11)) {
		t.Fatal("11 should be bigger than 10")
	}
	if bigger.matchExact(encodeInteger(TypeDword, 10)) {
		t.Fatal("10 should not be strictly bigger than 10")
	}

	smaller := &Session{opts: Options{
		Type:  TypeDword,
		Mode:  ModeSmaller,
		Value: NewValue(TypeDword, encodeInteger(TypeDword, 10)),
	}}
	if !smaller.matchExact(encodeInteger(TypeDword, 9)) {
		t.Fatal("9 should be smaller than 10")
	}
	if smaller.matchExact(encodeInteger(TypeDword, 10)) {
		t.Fatal("10 should not be strictly smaller than 10")
	}
}

func TestKeepSameAsFirst(t *testing.T) {
	s := &Session{opts: Options{Type: TypeDword, Mode: ModeSameAsFirst}}
	first := NewValue(TypeDword, encodeInteger(TypeDword, 7))
	res := Result{First: first}
	if !s.keep(NewValue(TypeDword, encodeInteger(TypeDword, 7)), res) {
		t.Fatal("unchanged value should match the first scan")
	}
	if s.keep(NewValue(TypeDword, encodeInteger(TypeDword, 8)), res) {
		t.Fatal("changed value should not match the first scan")
	}
}
