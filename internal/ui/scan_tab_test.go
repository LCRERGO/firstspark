//go:build gui

package ui

import (
	"testing"

	"github.com/LCRERGO/firstspark/internal/i18n"
)

func TestScanTabLifecycle(t *testing.T) {
	a := newTestApp(t)
	if len(a.tabs) != 1 || a.activeTab != 0 {
		t.Fatalf("initial tabs = %d active = %d", len(a.tabs), a.activeTab)
	}
	a.addScanTab()
	if len(a.tabs) != 2 || a.activeTab != 1 {
		t.Fatalf("after add: tabs = %d active = %d", len(a.tabs), a.activeTab)
	}
	a.cycleTab(1)
	if a.activeTab != 0 {
		t.Fatalf("cycle wrap active = %d", a.activeTab)
	}
	a.cycleTab(-1)
	if a.activeTab != 1 {
		t.Fatalf("cycle back active = %d", a.activeTab)
	}
	a.removeTabAt(1)
	if len(a.tabs) != 1 || a.activeTab != 0 {
		t.Fatalf("after remove: tabs = %d active = %d", len(a.tabs), a.activeTab)
	}
}

func TestResetTabs(t *testing.T) {
	a := newTestApp(t)
	a.addScanTab()
	a.resetTabs()
	if len(a.tabs) != 1 || a.activeTab != 0 {
		t.Fatalf("reset: tabs = %d active = %d", len(a.tabs), a.activeTab)
	}
}

func TestCompareOpRoundTrip(t *testing.T) {
	for _, o := range tabCompareOps {
		got, ok := compareOpFor(i18n.T(o.key))
		if !ok || got != o.op {
			t.Fatalf("compareOpFor(%q) = %v, %v; want %v", i18n.T(o.key), got, ok, o.op)
		}
	}
}
