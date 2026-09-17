//go:build gui

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2/driver/desktop"
)

func TestAppShortcutsAreUnique(t *testing.T) {
	a := &App{}
	seen := map[string]bool{}
	for _, b := range a.appShortcuts() {
		key := fmt.Sprintf("%d:%s", b.mod, b.key)
		if seen[key] {
			t.Errorf("duplicate shortcut %s", key)
		}
		seen[key] = true
	}
}

func TestMatchesShortcut(t *testing.T) {
	sc := &desktop.CustomShortcut{KeyName: "M", Modifier: 0}
	if !matchesShortcut(sc, "M", 0) {
		t.Error("expected shortcut to match")
	}
	if matchesShortcut(sc, "N", 0) {
		t.Error("did not expect a different key to match")
	}
	if matchesShortcut(sc, "M", 1) {
		t.Error("did not expect a different modifier to match")
	}
}
