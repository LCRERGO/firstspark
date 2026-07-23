//go:build gui

package ui

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// Process list column widths.
const (
	procIconW float32 = 26
	procNameW float32 = 190
	procPIDW  float32 = 80
	procUserW float32 = 130
	procRowH  float32 = 22
)

// procRow is a visible process list row.
type procRow struct {
	index int
	pid   int
	name  string
	user  string
}

// openProcessList shows the separate Process List window, creating it lazily.
func (a *App) openProcessList() {
	if a.procWin == nil {
		a.procWin = a.fapp.NewWindow("Process List")
		a.procWin.Resize(fyne.NewSize(480, 480))
		a.buildProcessList()
	}
	a.refreshProcesses()
	a.procWin.Show()
}

func (a *App) buildProcessList() {
	a.procFilter = widget.NewEntry()
	a.procFilter.SetPlaceHolder("filter by name, pid or user")
	a.procFilter.OnChanged = func(string) {
		a.applyFilter()
	}

	a.procList = widget.NewList(
		func() int { return len(a.procRows) },
		func() fyne.CanvasObject { return a.newProcRowWidget() },
		func(id widget.ListItemID, o fyne.CanvasObject) { a.updateProcRow(id, o) },
	)
	a.procList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(a.procRows) {
			return
		}
		a.selectProcess(a.procRows[id].index)
		a.procWin.Hide()
	}

	body := container.NewBorder(
		a.procFilter, nil, nil, nil,
		container.NewBorder(a.procHeader(), nil, nil, nil, a.procList),
	)
	a.procWin.SetContent(body)
}

func (a *App) procHeader() fyne.CanvasObject {
	labels := []string{"Name", "PID", "User"}
	widths := []float32{procNameW, procPIDW, procUserW}
	a.procHeaderBtns = make([]*widget.Button, len(labels))
	row := container.NewHBox(fixedWidth(procIconW, canvas.NewRectangle(nil)))
	for i, label := range labels {
		col := i
		btn := widget.NewButton(label, func() { a.sortProcs(col) })
		btn.Importance = widget.LowImportance
		a.procHeaderBtns[i] = btn
		row.Add(fixedWidth(widths[i], btn))
	}
	return row
}

func fixedWidth(w float32, obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(w, procRowH), obj)
}

// newProcRowWidget builds a reusable row: icon, name, PID and user cells. The
// template must be a concrete *fyne.Container (not a wrapper type) so Fyne's
// painter renders it.
func (a *App) newProcRowWidget() fyne.CanvasObject {
	icon := canvas.NewImageFromResource(nil)
	icon.FillMode = canvas.ImageFillContain
	icon.ScaleMode = canvas.ImageScaleSmooth
	return container.NewHBox(
		fixedWidth(procIconW, icon),
		fixedWidth(procNameW, widget.NewLabel("")),
		fixedWidth(procPIDW, widget.NewLabel("")),
		fixedWidth(procUserW, widget.NewLabel("")),
	)
}

func (a *App) updateProcRow(id widget.ListItemID, o fyne.CanvasObject) {
	icon, name, pid, user := procRowCells(o)
	if id < 0 || id >= len(a.procRows) {
		name.SetText("")
		pid.SetText("")
		user.SetText("")
		icon.Resource = nil
		icon.Refresh()
		return
	}
	r := a.procRows[id]
	name.SetText(r.name)
	pid.SetText(strconv.Itoa(r.pid))
	user.SetText(r.user)
	if a.showIcons {
		icon.Resource = a.icons.get(r.pid)
	} else {
		icon.Resource = nil
	}
	icon.Refresh()
}

// procRowCells unpacks the row template's cells by position.
func procRowCells(o fyne.CanvasObject) (*canvas.Image, *widget.Label, *widget.Label, *widget.Label) {
	row := o.(*fyne.Container)
	icon := row.Objects[0].(*fyne.Container).Objects[0].(*canvas.Image)
	name := row.Objects[1].(*fyne.Container).Objects[0].(*widget.Label)
	pid := row.Objects[2].(*fyne.Container).Objects[0].(*widget.Label)
	user := row.Objects[3].(*fyne.Container).Objects[0].(*widget.Label)
	return icon, name, pid, user
}

// refreshProcesses reloads /proc, reapplies the filter and starts icon lookup.
func (a *App) refreshProcesses() {
	procs, err := mem.List()
	if err != nil {
		a.fail(err)
		return
	}
	a.procs = procs
	a.applyFilter()
	a.setStatus("%d processes", len(procs))
	if a.showIcons {
		go a.loadIcons()
	}
}

// loadIcons resolves process icons in the background and refreshes the list.
func (a *App) loadIcons() {
	rows := make([]procRow, len(a.procRows))
	copy(rows, a.procRows)
	for _, r := range rows {
		a.icons.resolve(r.pid)
	}
	fyne.Do(func() {
		if a.procList != nil {
			a.procList.Refresh()
		}
	})
}

// applyFilter recomputes the visible, sorted process rows.
func (a *App) applyFilter() {
	q := ""
	if a.procFilter != nil {
		q = strings.ToLower(strings.TrimSpace(a.procFilter.Text))
	}
	a.procRows = a.procRows[:0]
	for i, p := range a.procs {
		user := a.username(p.UID)
		if q != "" &&
			!strings.Contains(strings.ToLower(p.Name), q) &&
			!strings.Contains(strings.ToLower(p.Cmdline), q) &&
			!strings.Contains(strings.ToLower(user), q) &&
			!strings.Contains(strconv.Itoa(p.PID), q) {
			continue
		}
		a.procRows = append(a.procRows, procRow{index: i, pid: p.PID, name: p.Name, user: user})
	}
	a.sortProcRows()
	a.updateProcHeaders()
	if a.procList != nil {
		a.procList.Refresh()
	}
}

// sortProcs changes the sort column, toggling direction when it is unchanged.
func (a *App) sortProcs(col int) {
	if a.procSortCol == col {
		a.procSortAsc = !a.procSortAsc
	} else {
		a.procSortCol = col
		a.procSortAsc = true
	}
	a.applyFilter()
}

func (a *App) sortProcRows() {
	col := a.procSortCol
	asc := a.procSortAsc
	sort.SliceStable(a.procRows, func(i, j int) bool {
		ri, rj := a.procRows[i], a.procRows[j]
		var less, equal bool
		switch col {
		case 1:
			less, equal = ri.pid < rj.pid, ri.pid == rj.pid
		case 2:
			li, lj := strings.ToLower(ri.user), strings.ToLower(rj.user)
			less, equal = li < lj, li == lj
		default:
			li, lj := strings.ToLower(ri.name), strings.ToLower(rj.name)
			less, equal = li < lj, li == lj
		}
		if equal {
			return ri.pid < rj.pid
		}
		if asc {
			return less
		}
		return !less
	})
}

func (a *App) updateProcHeaders() {
	labels := []string{"Name", "PID", "User"}
	for i, btn := range a.procHeaderBtns {
		text := labels[i]
		if i == a.procSortCol {
			if a.procSortAsc {
				text += " \u25B2"
			} else {
				text += " \u25BC"
			}
		}
		btn.SetText(text)
	}
}

// username resolves a UID to a login name, falling back to the number.
func (a *App) username(uid int) string {
	if a.users == nil {
		a.users = loadUsers()
	}
	if name, ok := a.users[uid]; ok {
		return name
	}
	return strconv.Itoa(uid)
}

func loadUsers() map[int]string {
	users := map[int]string{}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return users
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		uid, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		users[uid] = parts[0]
	}
	return users
}

// selectProcess makes the process at idx the scan target.
func (a *App) selectProcess(idx int) {
	if idx < 0 || idx >= len(a.procs) {
		return
	}
	p := a.procs[idx]
	a.proc = &p
	a.processLabel.SetText(fmt.Sprintf("Process: %s (%d)", p.Name, p.PID))
	a.session = nil
	a.results = nil
	a.foundSel = -1
	if a.foundList != nil {
		a.foundList.Refresh()
	}
	a.foundCount.SetText("Found: 0")
	a.setStatus("selected %s (%d)", p.Name, p.PID)
}
