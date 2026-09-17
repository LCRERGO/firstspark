//go:build linux

package hotkey

import "testing"

func TestXString(t *testing.T) {
	cases := map[string]string{
		"Ctrl+Alt+S": "Control-Mod1-s",
		"F5":         "F5",
		"Super+1":    "Mod4-1",
		"Shift+F12":  "Shift-F12",
		"Ctrl+H":     "Control-h",
	}
	for in, want := range cases {
		got, err := xString(in)
		if err != nil {
			t.Fatalf("xString(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("xString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestXStringRejectsBadInput(t *testing.T) {
	for _, in := range []string{"", "Ctrl+", "Hyper+S"} {
		if _, err := xString(in); err == nil {
			t.Errorf("xString(%q) succeeded, want an error", in)
		}
	}
}
