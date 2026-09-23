//go:build gui

package ui

import (
	"os"
	"testing"

	"github.com/LCRERGO/firstspark/internal/i18n"
)

// TestMain initialises the English catalog so widget labels and parse helpers
// match their translated strings in tests.
func TestMain(m *testing.M) {
	_ = i18n.Init("en")
	os.Exit(m.Run())
}
