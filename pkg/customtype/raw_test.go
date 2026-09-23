package customtype

import "testing"

func TestRegisterRaw(t *testing.T) {
	ty, err := RegisterRaw("Test Raw 4", 4)
	if err != nil {
		t.Fatalf("RegisterRaw: %v", err)
	}
	if ty.Size != 4 {
		t.Fatalf("size = %d, want 4", ty.Size)
	}
	v, err := ty.Parse("42")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := ty.Format(v); got != "42" {
		t.Fatalf("Format = %q, want 42", got)
	}
	neg, err := ty.Parse("-1")
	if err != nil {
		t.Fatalf("Parse(-1): %v", err)
	}
	if ty.Int64(neg) != -1 {
		t.Fatalf("Int64(-1) = %d", ty.Int64(neg))
	}
	if _, err := RegisterRaw("Test Raw Bad", 3); err == nil {
		t.Fatal("expected an error for an unsupported size")
	}
}
