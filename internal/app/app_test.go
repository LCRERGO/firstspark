package app

import (
	"path/filepath"
	"testing"

	"github.com/LCRERGO/firstspark/pkg/cheattable"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

func TestParseHexAddr(t *testing.T) {
	cases := map[string]uint64{"": 0, "0x10": 0x10, "10": 0x10, "FF": 0xff, " 0X2a ": 0x2a}
	for in, want := range cases {
		got, err := parseHexAddr(in)
		if err != nil || got != want {
			t.Errorf("parseHexAddr(%q) = %#x, %v; want %#x", in, got, err, want)
		}
	}
	if _, err := parseHexAddr("zz"); err == nil {
		t.Error("expected an error for a bad address")
	}
}

func TestModeTakesValue(t *testing.T) {
	takes := []scan.ScanMode{
		scan.ModeExact, scan.ModeBigger, scan.ModeSmaller,
		scan.ModeIncreasedBy, scan.ModeDecreasedBy,
	}
	for _, m := range takes {
		if !modeTakesValue(m) {
			t.Errorf("modeTakesValue(%v) = false", m)
		}
	}
	for _, m := range []scan.ScanMode{scan.ModeUnknown, scan.ModeChanged, scan.ModeSameAsFirst} {
		if modeTakesValue(m) {
			t.Errorf("modeTakesValue(%v) = true", m)
		}
	}
}

func TestRunVersion(t *testing.T) {
	if err := Run([]string{"--version"}); err != nil {
		t.Fatalf("Run --version: %v", err)
	}
}

func TestExportResults(t *testing.T) {
	results := []scan.Result{
		{Addr: 0x1000, Value: scan.NewValue(scan.TypeDword, []byte{42, 0, 0, 0})},
	}
	for _, name := range []string{"out.ct", "out.json", "out.yaml"} {
		path := filepath.Join(t.TempDir(), name)
		if err := exportResults(results, path); err != nil {
			t.Fatalf("exportResults(%s): %v", name, err)
		}
		tbl, err := cheattable.LoadAny(path)
		if err != nil {
			t.Fatalf("LoadAny(%s): %v", name, err)
		}
		if len(tbl.Entries) != 1 || tbl.Entries[0].Address != "0x1000" {
			t.Fatalf("LoadAny(%s) = %+v", name, tbl.Entries)
		}
	}
}
