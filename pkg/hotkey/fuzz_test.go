//go:build linux

package hotkey

import "testing"

func FuzzXString(f *testing.F) {
	f.Add("Ctrl+Alt+S")
	f.Add("F5")
	f.Add("Super+1")
	f.Add("")
	f.Add("+")
	f.Add("Hyper+S")
	f.Fuzz(func(t *testing.T, combo string) {
		_, _ = xString(combo)
	})
}
