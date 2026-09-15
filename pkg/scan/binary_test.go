package scan

import "testing"

func TestParseBinaryAndMatch(t *testing.T) {
	p, err := ParseBinary("1010 ??11")
	if err != nil {
		t.Fatalf("ParseBinary: %v", err)
	}
	if p.Bits != 8 {
		t.Fatalf("bits = %d", p.Bits)
	}
	// pattern "1010??11" sets bits 0,2,6,7 => 0b11000101 = 0xC5
	if !p.Match([]byte{0xC5}) {
		t.Fatal("expected 0xC5 to match")
	}
	if p.Match([]byte{0xCB}) {
		t.Fatal("did not expect 0xCB to match")
	}
}

func TestBinaryTypeIsRegistered(t *testing.T) {
	d, ok := LookupType("binary")
	if !ok || d.Kind != KindBinary {
		t.Fatalf("binary type = %+v, %v", d, ok)
	}
	v, err := ParseValue(TypeBinary, "11110000")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if v.String() != "11110000" {
		t.Fatalf("format = %q", v.String())
	}
}
