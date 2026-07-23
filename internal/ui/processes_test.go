//go:build gui

package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{
		cfg:         config.Default(),
		frozen:      map[uint64]scan.Value{},
		stop:        make(chan struct{}),
		foundSel:    -1,
		tableSel:    -1,
		procSortCol: 0,
		procSortAsc: true,
		showIcons:   false,
	}
	a.icons = newIconResolver()
	a.fapp = test.NewApp()
	a.th = newTheme(schemeLight, 14)
	a.fapp.Settings().SetTheme(a.th)
	a.build()
	return a
}

func TestProcessListBuildsAndRenders(t *testing.T) {
	a := newTestApp(t)
	a.openProcessList()
	if a.procList == nil {
		t.Fatal("process list was not created")
	}
	if len(a.procRows) == 0 {
		t.Fatal("no process rows were built")
	}
	row := a.newProcRowWidget()
	for i := range a.procRows {
		a.updateProcRow(i, row)
	}
	_, name, pid, _ := procRowCells(row)
	if name.Text == "" || pid.Text == "" {
		t.Fatalf("row not populated: name=%q pid=%q", name.Text, pid.Text)
	}
}

func TestProcessSortToggles(t *testing.T) {
	a := newTestApp(t)
	a.openProcessList()
	if len(a.procRows) < 2 {
		t.Skip("not enough processes to test sorting")
	}
	a.sortProcs(1)
	if a.procSortCol != 1 || !a.procSortAsc {
		t.Fatalf("expected PID ascending, got col=%d asc=%v", a.procSortCol, a.procSortAsc)
	}
	for i := 1; i < len(a.procRows); i++ {
		if a.procRows[i-1].pid > a.procRows[i].pid {
			t.Fatalf("PID sort not ascending at %d", i)
		}
	}
	a.sortProcs(1)
	if a.procSortAsc {
		t.Fatal("expected descending after second click")
	}
}
