//go:build linux

package hotkey

import (
	"strings"
	"testing"
	"testing/quick"
)

// TestPropertyXStringAcceptsKnownCombos checks every modifier/key combination
// the capture UI can produce converts to a non-empty keybind string.
func TestPropertyXStringAcceptsKnownCombos(t *testing.T) {
	mods := []string{"Ctrl", "Alt", "Shift", "Super"}
	keys := []string{"a", "Z", "1", "9", "F1", "F12"}
	prop := func(mi, ki uint8) bool {
		combo := mods[int(mi)%len(mods)] + "+" + keys[int(ki)%len(keys)]
		s, err := xString(combo)
		if err != nil {
			return false
		}
		if s == "" || strings.ContainsAny(s, " +") {
			return false
		}
		// The key must survive lower/upper normalisation.
		key := strings.ToLower(keys[int(ki)%len(keys)])
		return strings.HasSuffix(strings.ToLower(s), key)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}
