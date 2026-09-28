//go:build gui

package ui

import (
	"strings"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// scanTypeKeys maps every engine mode to its translation key, keeping the
// display label independent of the parsing logic.
var scanTypeKeys = map[scan.ScanMode]string{
	scan.ModeExact:       "scan.type.exact",
	scan.ModeUnknown:     "scan.type.unknown",
	scan.ModeChanged:     "scan.type.changed",
	scan.ModeUnchanged:   "scan.type.unchanged",
	scan.ModeIncreased:   "scan.type.increased",
	scan.ModeDecreased:   "scan.type.decreased",
	scan.ModeIncreasedBy: "scan.type.increased_by",
	scan.ModeDecreasedBy: "scan.type.decreased_by",
	scan.ModeBetween:     "scan.type.between",
	scan.ModeBigger:      "scan.type.bigger",
	scan.ModeSmaller:     "scan.type.smaller",
	scan.ModeSameAsFirst: "scan.type.same_as_first",
}

// firstScanModes and nextScanModes are the reference tool's phase-specific scan
// types: change-based filters only make sense after an initial scan.
var firstScanModes = []scan.ScanMode{
	scan.ModeExact, scan.ModeBigger, scan.ModeSmaller, scan.ModeBetween, scan.ModeUnknown,
}

var nextScanModes = []scan.ScanMode{
	scan.ModeExact, scan.ModeBigger, scan.ModeSmaller, scan.ModeBetween,
	scan.ModeIncreased, scan.ModeIncreasedBy, scan.ModeDecreased, scan.ModeDecreasedBy,
	scan.ModeChanged, scan.ModeUnchanged, scan.ModeSameAsFirst,
}

// scanTypeLabelsFor returns the translated labels for a scan phase; next is
// true once a session exists.
func scanTypeLabelsFor(next bool) []string {
	modes := firstScanModes
	if next {
		modes = nextScanModes
	}
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = scanTypeLabel(m)
	}
	return out
}

// scanTypeLabel returns the translated label of a single mode.
func scanTypeLabel(m scan.ScanMode) string {
	if key, ok := scanTypeKeys[m]; ok {
		return i18n.T(key)
	}
	return i18n.T("scan.type.exact")
}

// compareOps are the operators offered for an exact scan, in display order.
var compareOps = []scan.CompareOp{
	scan.OpEqual, scan.OpNotEqual, scan.OpGreater, scan.OpGreaterEqual, scan.OpLess, scan.OpLessEqual,
}

// compareLabels returns the display labels of the comparison operators.
func compareLabels() []string {
	out := make([]string, len(compareOps))
	for i, op := range compareOps {
		out[i] = op.String()
	}
	return out
}

// parseCompareLabel maps a comparison label back to an operator.
func parseCompareLabel(s string) scan.CompareOp {
	for _, op := range compareOps {
		if op.String() == s {
			return op
		}
	}
	if op, err := scan.ParseCompareOp(s); err == nil {
		return op
	}
	return scan.OpEqual
}

// execModes are the executable-region filters, in display order.
var execModes = []scan.ExecutableMode{scan.ExecAny, scan.ExecOnly, scan.ExecNonExec}

// execLabels returns the translated executable-filter labels.
func execLabels() []string {
	return []string{i18n.T("scan.exec.any"), i18n.T("scan.exec.only"), i18n.T("scan.exec.non")}
}

// execLabel returns the translated label of an executable filter.
func execLabel(m scan.ExecutableMode) string {
	for i, mode := range execModes {
		if mode == m {
			return execLabels()[i]
		}
	}
	return i18n.T("scan.exec.any")
}

// parseExecLabel maps an executable-filter label back to its mode.
func parseExecLabel(s string) scan.ExecutableMode {
	for i, l := range execLabels() {
		if l == s {
			return execModes[i]
		}
	}
	return scan.ExecAny
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
	for m, key := range scanTypeKeys {
		if i18n.T(key) == s {
			return m
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
	case scan.ModeExact, scan.ModeBigger, scan.ModeSmaller, scan.ModeIncreasedBy, scan.ModeDecreasedBy, scan.ModeBetween:
		return true
	default:
		return false
	}
}
