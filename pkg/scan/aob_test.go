package scan

import "testing"

func TestAOBCompactWildcards(t *testing.T) {
	p, err := ParseAOB("488B??E5")
	if err != nil {
		t.Fatalf("ParseAOB: %v", err)
	}
	if p.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", p.Len())
	}
	if !p.Match([]byte{0x48, 0x8B, 0x00, 0xE5}) {
		t.Error("expected compact wildcard pattern to match")
	}
	if p.Match([]byte{0x48, 0x8B, 0x00, 0xE6}) {
		t.Error("expected mismatch on last byte")
	}
}
