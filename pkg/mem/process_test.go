package mem

import "testing"

func TestParsePPID(t *testing.T) {
	// comm may contain spaces and parentheses.
	stat := "1234 (my (odd) process) S 42 1234 1234 0 -1 4194304"
	if got := parsePPID(stat); got != 42 {
		t.Fatalf("parsePPID = %d, want 42", got)
	}
	if got := parsePPID("garbage"); got != 0 {
		t.Fatalf("parsePPID(garbage) = %d, want 0", got)
	}
}
