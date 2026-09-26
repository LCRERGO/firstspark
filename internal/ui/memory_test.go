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

func TestHexNibble(t *testing.T) {
	for r, want := range map[rune]int{'0': 0, '9': 9, 'a': 10, 'F': 15, 'z': -1} {
		if got := hexNibble(r); got != want {
			t.Fatalf("hexNibble(%q) = %d, want %d", r, got, want)
		}
	}
}

func TestHexRowParts(t *testing.T) {
	a := newTestApp(t)
	h := a.newHexRow()
	bytes := []string{"00 ", "11 ", "22 ", "33 ", "44 ", "55 ", "66 ", "77 ", "88 ", "99 ", "aa ", "bb ", "cc ", "dd ", "ee ", "ff "}
	h.setParts("0x", bytes, "ascii", "=1", 2, 4)
	if h.pre.Text != "0x00 11 " {
		t.Fatalf("pre = %q", h.pre.Text)
	}
	if h.sel.Text != "22 33 " {
		t.Fatalf("sel = %q", h.sel.Text)
	}
	if h.post.Text != "44 55 66 77 88 99 aa bb cc dd ee ff  ascii=1" {
		t.Fatalf("post = %q", h.post.Text)
	}
}

func TestMemSelection(t *testing.T) {
	a := newTestApp(t)
	a.memSelStart, a.memSelEnd = 0x20, 0x10
	lo, hi := a.memSelection()
	if lo != 0x10 || hi != 0x20 {
		t.Fatalf("selection = %#x..%#x", lo, hi)
	}
}
