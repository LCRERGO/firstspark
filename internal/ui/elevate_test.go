//go:build gui

package ui

import (
	"strings"
	"testing"
)

func TestPrivilegeHelpers(t *testing.T) {
	// These read the process state and must not panic.
	_ = isRoot()
	_ = ptraceRestricted()
}

func TestElevatedArgsForwardUserEnvironment(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("DISPLAY", ":0")
	args, err := elevatedArgs()
	if err != nil {
		t.Fatalf("elevatedArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"XDG_CONFIG_HOME=/home/alice/.config",
		"XDG_DATA_HOME=/home/alice/.local/share",
		"DISPLAY=:0",
		parentEnv + "=",
		origUIDEnv + "=",
		elevatedEnv + "=1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}
