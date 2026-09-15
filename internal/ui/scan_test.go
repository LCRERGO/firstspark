//go:build gui

package ui

import "testing"

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

	a.scanType.SetSelected("Value between")
	if a.value2Entry.Disabled() {
		t.Fatal("the upper bound should be enabled for between scans")
	}
	if !a.compareEntry.Disabled() {
		t.Fatal("Compare should be disabled for between scans")
	}

	a.scanType.SetSelected("Unknown initial value")
	if a.valueEntry.Disabled() == false {
		t.Fatal("the scan value should be disabled for an unknown scan")
	}
}
