//go:build gui

package ui

import (
	"strings"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// scanTypeOption pairs an engine mode with its translation key, keeping the
// display label independent of the parsing logic.
type scanTypeOption struct {
	mode scan.ScanMode
	key  string
}

// scanTypeOptions are Cheat Engine's scan types, restricted to the modes the
// engine implements.
var scanTypeOptions = []scanTypeOption{
	{scan.ModeExact, "scan.type.exact"},
	{scan.ModeIncreased, "scan.type.increased"},
	{scan.ModeIncreasedBy, "scan.type.increased_by"},
	{scan.ModeDecreased, "scan.type.decreased"},
	{scan.ModeDecreasedBy, "scan.type.decreased_by"},
	{scan.ModeBetween, "scan.type.between"},
	{scan.ModeChanged, "scan.type.changed"},
	{scan.ModeUnchanged, "scan.type.unchanged"},
	{scan.ModeUnknown, "scan.type.unknown"},
}

// scanTypeLabels returns the translated scan type labels.
func scanTypeLabels() []string {
	out := make([]string, len(scanTypeOptions))
	for i, o := range scanTypeOptions {
		out[i] = i18n.T(o.key)
	}
	return out
}

// scanTypeLabel returns the translated label of a single mode.
func scanTypeLabel(m scan.ScanMode) string {
	for _, o := range scanTypeOptions {
		if o.mode == m {
			return i18n.T(o.key)
		}
	}
	return i18n.T("scan.type.exact")
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
	for _, o := range scanTypeOptions {
		if i18n.T(o.key) == s {
			return o.mode
		}
	}
	return scan.ModeExact
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
