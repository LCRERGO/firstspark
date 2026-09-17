//go:build gui

package ui

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

func TestValueHintKey(t *testing.T) {
	cases := map[scan.ValueType]string{
		scan.TypeAOB:        "scan.hint.value_aob",
		scan.TypeBinary:     "scan.hint.value_binary",
		scan.TypeString:     "scan.hint.value_text",
		scan.TypeFloat:      "scan.hint.value_float",
		scan.TypeDouble:     "scan.hint.value_float",
		scan.TypeDword:      "scan.hint.value_int",
		scan.TypeQword:      "scan.hint.value_int",
		scan.ValueType(200): "scan.hint.value_int",
	}
	for typ, want := range cases {
		if got := valueHintKey(typ); got != want {
			t.Errorf("valueHintKey(%v) = %q, want %q", typ, got, want)
		}
	}
}
