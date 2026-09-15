//go:build linux && amd64

package debugger

import (
	"os"
	"testing"
)

func TestLengthBits(t *testing.T) {
	cases := map[int]uint64{1: 0, 2: 1, 4: 3, 8: 2, 3: 0}
	for size, want := range cases {
		if got := lengthBits(size); got != want {
			t.Errorf("lengthBits(%d) = %d, want %d", size, got, want)
		}
	}
}

func TestSessionSupportsWatchpoints(t *testing.T) {
	s, err := NewSession("", os.Getpid(), Options{})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if !s.SupportsWatchpoints() {
		t.Fatal("the ptrace backend should support hardware watchpoints")
	}
}
