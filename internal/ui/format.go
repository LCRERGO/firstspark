//go:build gui

package ui

import (
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
	"Changed value",
	"Unchanged value",
	"Unknown initial value",
}

// valueTypeOptions are Cheat Engine's value type labels.
var valueTypeOptions = []string{
	"Byte",
	"2 Bytes",
	"4 Bytes",
	"8 Bytes",
	"Float",
	"Double",
	"Text",
	"Array of Bytes",
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

func parseCEValueType(s string) scan.ValueType {
	switch s {
	case "Byte":
		return scan.TypeByte
	case "2 Bytes":
		return scan.TypeWord
	case "8 Bytes":
		return scan.TypeQword
	case "Float":
		return scan.TypeFloat
	case "Double":
		return scan.TypeDouble
	case "Text":
		return scan.TypeString
	case "Array of Bytes":
		return scan.TypeAOB
	default:
		return scan.TypeDword
	}
}

// ceValueTypeLabel maps a value type to its Cheat Engine label.
func ceValueTypeLabel(t scan.ValueType) string {
	switch t {
	case scan.TypeByte:
		return "Byte"
	case scan.TypeWord:
		return "2 Bytes"
	case scan.TypeDword:
		return "4 Bytes"
	case scan.TypeQword:
		return "8 Bytes"
	case scan.TypeFloat:
		return "Float"
	case scan.TypeDouble:
		return "Double"
	case scan.TypeString:
		return "Text"
	case scan.TypeAOB:
		return "Array of Bytes"
	default:
		return "4 Bytes"
	}
}

// modeNeedsValue reports whether a scan mode consumes the scan value.
func modeNeedsValue(m scan.ScanMode) bool {
	switch m {
	case scan.ModeExact, scan.ModeIncreasedBy, scan.ModeDecreasedBy:
		return true
	default:
		return false
	}
}
