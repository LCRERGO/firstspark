//go:build gui

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{
		cfg:           config.Default(),
		freezeTargets: map[uint64]scan.Value{},
		stop:          make(chan struct{}),
		tableSel:      -1,
		procSortCol:   0,
		procSortAsc:   true,
		showIcons:     false,
		expanded:      map[int]bool{},
		treeMode:      true,
	}
	a.icons = newIconResolver()
	a.fapp = test.NewApp()
	a.th = newTheme(familyCyberpunk, variantLight, 14)
	a.fapp.Settings().SetTheme(a.th)
	a.build()
	return a
}

func TestMatchAutoAttach(t *testing.T) {
	procs := []mem.Process{
		{PID: 1, Name: "game"},
		{PID: 2, Name: "game-launcher"},
		{PID: 3, Name: "other"},
	}
	if p, ok, err := matchAutoAttach(procs, "launcher;other", false); err != nil || !ok || p.PID != 2 {
		t.Fatalf("substring match = %+v, %v, %v; want PID 2", p, ok, err)
	}
	if p, ok, err := matchAutoAttach(procs, "game", false); err != nil || !ok || p.PID != 1 {
		t.Fatalf("earlier process should win = %+v, %v, %v; want PID 1", p, ok, err)
	}
	if p, ok, err := matchAutoAttach(procs, "other;game", false); err != nil || !ok || p.PID != 3 {
		t.Fatalf("earlier pattern should win = %+v, %v, %v; want PID 3", p, ok, err)
	}
	if p, ok, err := matchAutoAttach(procs, "launcher$", true); err != nil || !ok || p.PID != 2 {
		t.Fatalf("regex match = %+v, %v, %v; want PID 2", p, ok, err)
	}
	if _, ok, err := matchAutoAttach(procs, "missing", false); err != nil || ok {
		t.Fatalf("no match = %v, %v; want false", ok, err)
	}
	if _, _, err := matchAutoAttach(procs, "(", true); err == nil {
		t.Fatal("expected an error for an invalid regex")
	}
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
	name, pid := procRowLabels(row)
	if name.Text == "" || pid.Text == "" {
		t.Fatalf("row not populated: name=%q pid=%q", name.Text, pid.Text)
	}
}

func procRowLabels(o fyne.CanvasObject) (*widget.Label, *widget.Label) {
	row := o.(*fyne.Container)
	name := row.Objects[3].(*fyne.Container).Objects[0].(*widget.Label)
	pid := row.Objects[4].(*fyne.Container).Objects[0].(*widget.Label)
	return name, pid
}

func TestProcessTreeCollapse(t *testing.T) {
	a := newTestApp(t)
	a.treeMode = true
	a.openProcessList()
	if len(a.procRows) == 0 {
		t.Fatal("no tree rows were built")
	}
	total := len(a.procRows)
	for i, r := range a.procRows {
		if !r.hasKids || !r.expanded {
			continue
		}
		a.toggleTreeRow(i)
		if len(a.procRows) >= total {
			t.Fatalf("collapsing should hide descendants: %d -> %d", total, len(a.procRows))
		}
		return
	}
	t.Skip("no expanded process with children")
}

func TestProcessSortToggles(t *testing.T) {
	a := newTestApp(t)
	a.treeMode = false // exercise the flat, globally sorted list
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
