//go:build gui

package ui

import "testing"

func TestBuildHexLinesWithoutViewer(t *testing.T) {
	a := newTestApp(t)
	// The Memory Viewer has not been opened, so its display type is unset.
	lines := a.buildHexLines(0x1000, []byte{0x01, 0x02, 0x03, 0x04})
	if len(lines) == 0 {
		t.Fatal("expected hex lines")
	}
}
