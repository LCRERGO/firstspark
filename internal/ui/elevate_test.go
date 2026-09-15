//go:build gui

package ui

import "testing"

func TestPrivilegeHelpers(t *testing.T) {
	// These read the process state and must not panic.
	_ = isRoot()
	_ = ptraceRestricted()
}
