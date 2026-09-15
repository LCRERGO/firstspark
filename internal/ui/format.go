//go:build gui

package ui

import (
	"strings"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

// scanTypeOptions are Cheat Engine's scan type labels, restricted to the modes
// the engine implements.
var scanTypeOptions = []string{
	"Exact value",
	"Increased value",
	"Increased value by ...",
	"Decreased value",
	"Decreased value by ...",
	"Value between",
	"Changed value",
	"Unchanged value",
	"Unknown initial value",
}

// valueTypeOptions returns the display labels of every registered type:
// built-ins first, then user-defined types.
func valueTypeOptions() []string {
	types := scan.Types()
	out := make([]string, 0, len(types))
	for _, t := range types {
		out = append(out, t.Label)
	}
	return out
}

func parseCEScanType(s string) scan.ScanMode {
	switch s {
	case "Increased value":
		return scan.ModeIncreased
	case "Increased value by ...":
		return scan.ModeIncreasedBy
	case "Decreased value":
		return scan.ModeDecreased
	case "Decreased value by ...":
		return scan.ModeDecreasedBy
	case "Value between":
		return scan.ModeBetween
	case "Changed value":
		return scan.ModeChanged
	case "Unchanged value":
		return scan.ModeUnchanged
	case "Unknown initial value":
		return scan.ModeUnknown
	default:
		return scan.ModeExact
	}
}

// parseCEValueType maps a display label to a registered type ID.
func parseCEValueType(label string) scan.ValueType {
	if t, ok := scan.LookupType(label); ok {
		return t.ID
	}
	for _, t := range scan.Types() {
		if strings.EqualFold(t.Label, label) {
			return t.ID
		}
	}
	return scan.TypeDword
}

// ceValueTypeLabel maps a value type to its display label.
func ceValueTypeLabel(t scan.ValueType) string {
	if d := scan.TypeByID(t); d != nil {
		return d.Label
	}
	return "4 Bytes"
}

// modeNeedsValue reports whether a scan mode consumes the scan value.
func modeNeedsValue(m scan.ScanMode) bool {
	switch m {
	case scan.ModeExact, scan.ModeIncreasedBy, scan.ModeDecreasedBy, scan.ModeBetween:
		return true
	default:
		return false
	}
}
