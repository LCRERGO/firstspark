//go:build gui

package ui

import "testing"

func TestIsBareHexLiteral(t *testing.T) {
	bare := []string{"FF", "0xFF", "-10", "+1a"}
	for _, s := range bare {
		if !isBareHexLiteral(s) {
			t.Errorf("isBareHexLiteral(%q) = false, want true", s)
		}
	}
	expr := []string{"", "0xFF + 1", "360 * (10 ^ 6)", "10 / 4", "0x"}
	for _, s := range expr {
		if isBareHexLiteral(s) {
			t.Errorf("isBareHexLiteral(%q) = true, want false", s)
		}
	}
}
