//go:build gui

package ui

import "testing"

func TestFormatHexRowWithoutViewer(t *testing.T) {
	a := newTestApp(t)
	// The Memory Viewer has not been opened, so its display type is unset.
	line := a.formatHexRow(0x1000, []byte{0x01, 0x02, 0x03, 0x04})
	if line == "" {
		t.Fatal("expected a hex row")
	}
}
