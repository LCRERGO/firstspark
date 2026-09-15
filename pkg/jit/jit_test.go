//go:build cgo && linux && amd64

package jit

import "testing"

func TestCallReturnsArgument(t *testing.T) {
	p, err := New(64, 64)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	// mov rax, rdi ; ret
	if err := p.Load([]byte{0x48, 0x89, 0xF8, 0xC3}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := p.Seal(); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if got := p.Call(p.Entry(0), 42); got != 42 {
		t.Fatalf("Call returned %d, want 42", got)
	}
}

func TestCalleeSavedPreserved(t *testing.T) {
	p, err := New(64, 64)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	// mov rbx, rdi ; mov rax, rdi ; ret  (clobbers a callee-saved register)
	if err := p.Load([]byte{0x48, 0x89, 0xFB, 0x48, 0x89, 0xF8, 0xC3}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := p.Seal(); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	// The trampoline must restore RBX, so repeated calls stay correct.
	for i := 0; i < 3; i++ {
		if got := p.Call(p.Entry(0), 7); got != 7 {
			t.Fatalf("Call %d returned %d, want 7", i, got)
		}
	}
}

func TestDataBuffer(t *testing.T) {
	p, err := New(64, 16)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	p.SetData([]byte{1, 2, 3, 4})
	got := p.DataCopy(4)
	if len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("DataCopy = %v", got)
	}
}
