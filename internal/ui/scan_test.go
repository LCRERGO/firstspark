//go:build gui

package ui

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

func TestScanControlsFollowState(t *testing.T) {
	a := newTestApp(t)

	if !a.scanBtn.Disabled() {
		t.Fatal("First Scan should be disabled without a process")
	}
	if !a.nextBtn.Disabled() {
		t.Fatal("Next Scan should be disabled without a session")
	}
	if !a.undoBtn.Disabled() {
		t.Fatal("Undo Scan should be disabled without a session")
	}
	if !a.value2Entry.Disabled() {
		t.Fatal("the upper bound should be disabled for exact scans")
	}
	if a.compareEntry.Disabled() {
		t.Fatal("Compare should be enabled for exact scans")
	}

	if a.value2Row.Visible() {
		t.Fatal("the upper bound row should be hidden for exact scans")
	}

	a.scanType.SetSelected("Value between")
	if a.value2Entry.Disabled() {
		t.Fatal("the upper bound should be enabled for between scans")
	}
	if !a.value2Row.Visible() {
		t.Fatal("the upper bound row should be shown for between scans")
	}
	if !a.compareEntry.Disabled() {
		t.Fatal("Compare should be disabled for between scans")
	}

	a.scanType.SetSelected("Unknown initial value")
	if !a.valueEntry.Disabled() {
		t.Fatal("the scan value should be disabled for an unknown scan")
	}
	if a.value2Row.Visible() {
		t.Fatal("the upper bound row should be hidden for unknown scans")
	}
}

func TestHumanBytesAndScope(t *testing.T) {
	if got := humanBytes(512); got != "512 B" {
		t.Fatalf("humanBytes(512) = %q", got)
	}
	if got := humanBytes(2048); got != "2.0 KiB" {
		t.Fatalf("humanBytes(2048) = %q", got)
	}
	if parseScope("All readable") != scan.ScopeAllReadable {
		t.Fatal("parseScope(All readable)")
	}
	if parseScope("Heap + stack + exec + BSS") != scan.ScopeHeapStackExecBSS {
		t.Fatal("parseScope(Heap...)")
	}
	if parseScope("All writable") != scan.ScopeAllWritable {
		t.Fatal("parseScope(All writable)")
	}
}

func TestValuePlaceholder(t *testing.T) {
	cases := map[scan.ScanMode]string{
		scan.ModeBetween:     "lower bound",
		scan.ModeIncreasedBy: "delta",
		scan.ModeDecreasedBy: "delta",
		scan.ModeUnknown:     "not used",
		scan.ModeExact:       "value or AOB pattern",
	}
	for mode, want := range cases {
		if got := valuePlaceholder(mode); got != want {
			t.Errorf("valuePlaceholder(%v) = %q, want %q", mode, got, want)
		}
	}
}
