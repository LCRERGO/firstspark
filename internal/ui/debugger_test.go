//go:build gui

package ui

import (
	"os"
	"testing"

	"github.com/LCRERGO/firstspark/pkg/debugger"
)

func TestEvaluateBreakpointCond(t *testing.T) {
	regs := debugger.Registers{RAX: 0x10, RIP: 0x400000}
	cases := map[string]bool{
		"RAX == 0x10": true,
		"RAX != 0x10": false,
		"RAX > 0xF":   true,
		"RAX < 0x10":  false,
		"RIP > RAX":   true,
		"RAX == 16":   true,
		"":            true,
		"garbage":     true,
		"RAX > nope":  true,
	}
	for cond, want := range cases {
		if got := evalBreakpointCond(cond, regs); got != want {
			t.Errorf("evalBreakpointCond(%q) = %v, want %v", cond, got, want)
		}
	}
}

func TestParseBreakpointCond(t *testing.T) {
	for _, cond := range []string{"", "RAX == 1", "RIP >= RSP", "RDX != 0xff"} {
		if _, err := parseBreakpointCond(cond); err != nil {
			t.Errorf("parseBreakpointCond(%q): %v", cond, err)
		}
	}
	for _, cond := range []string{"nonsense", "RAX", "== 1", "RAX =="} {
		if _, err := parseBreakpointCond(cond); err == nil {
			t.Errorf("parseBreakpointCond(%q): expected an error", cond)
		}
	}
}

func TestListThreads(t *testing.T) {
	tids := listThreads(os.Getpid())
	if len(tids) == 0 {
		t.Fatal("expected at least one thread")
	}
}
