//go:build gui

package ui

import (
	"fmt"
	"image/color"
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
	procIconW      float32 = 26
	procNameW      float32 = 190
	procPIDW       float32 = 80
	procUserW      float32 = 130
	procRowH       float32 = 22
	procTreeIndent float32 = 14
)

// procRow is a visible process list row.
type procRow struct {
	index    int
	pid      int
	ppid     int
	name     string
	user     string
	depth    int
	hasKids  bool
	expanded bool
}

// treeToggle is a clickable disclosure triangle for the process tree.
type treeToggle struct {
	widget.BaseWidget
	label *widget.Label
	onTap func()
}

func newTreeToggle() *treeToggle {
	t := &treeToggle{label: widget.NewLabel("")}
	t.ExtendBaseWidget(t)
	return t
}

func (t *treeToggle) SetText(s string) { t.label.SetText(s) }

func (t *treeToggle) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *treeToggle) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.label)
}

// openProcessList shows the separate Process List window, creating it lazily.
func (a *App) openProcessList() {
	if a.procWin == nil {
		a.procWin = a.fapp.NewWindow("Process List")
		a.procWin.Resize(fyne.NewSize(520, 480))
		a.buildProcessList()
	}
	a.refreshProcesses()
	a.procWin.Show()
}

func (a *App) buildProcessList() {
	a.procFilter = widget.NewEntry()
	a.procFilter.SetPlaceHolder("filter by name, pid or user")
	a.procFilter.OnChanged = func(string) { a.applyFilter() }

	a.procTree = widget.NewCheck("Tree", func(on bool) {
		a.treeMode = on
		a.applyFilter()
	})

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

	filterRow := container.NewBorder(nil, nil, nil, a.procTree, a.procFilter)
	body := container.NewBorder(filterRow, nil, nil, nil,
		container.NewBorder(a.procHeader(), nil, nil, nil, a.procList))
	a.procWin.SetContent(body)
}

func (a *App) procHeader() fyne.CanvasObject {
	labels := []string{"Name", "PID", "User"}
	widths := []float32{procNameW, procPIDW, procUserW}
	a.procHeaderBtns = make([]*widget.Button, len(labels))
	row := container.NewHBox(
		container.NewGridWrap(fyne.NewSize(procTreeIndent, procRowH), canvas.NewRectangle(nil)),
		container.NewGridWrap(fyne.NewSize(18, procRowH), canvas.NewRectangle(nil)),
		fixedWidth(procIconW, canvas.NewRectangle(nil)),
	)
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

// newProcRowWidget builds a reusable row: indent, disclosure triangle, icon,
// name, PID and user cells. The template must be a concrete *fyne.Container so
// Fyne's painter renders it.
func (a *App) newProcRowWidget() fyne.CanvasObject {
	indent := canvas.NewRectangle(color.Transparent)
	indent.SetMinSize(fyne.NewSize(0, procRowH))
	toggle := newTreeToggle()
	icon := canvas.NewImageFromResource(nil)
	icon.FillMode = canvas.ImageFillContain
	icon.ScaleMode = canvas.ImageScaleSmooth
	return container.NewHBox(
		indent,
		fixedWidth(18, toggle),
		fixedWidth(procIconW, icon),
		fixedWidth(procNameW, widget.NewLabel("")),
		fixedWidth(procPIDW, widget.NewLabel("")),
		fixedWidth(procUserW, widget.NewLabel("")),
	)
}

func (a *App) updateProcRow(id widget.ListItemID, o fyne.CanvasObject) {
	row := o.(*fyne.Container)
	indent := row.Objects[0].(*canvas.Rectangle)
	toggle := row.Objects[1].(*fyne.Container).Objects[0].(*treeToggle)
	icon := row.Objects[2].(*fyne.Container).Objects[0].(*canvas.Image)
	name := row.Objects[3].(*fyne.Container).Objects[0].(*widget.Label)
	pid := row.Objects[4].(*fyne.Container).Objects[0].(*widget.Label)
	user := row.Objects[5].(*fyne.Container).Objects[0].(*widget.Label)

	if id < 0 || id >= len(a.procRows) {
		indent.SetMinSize(fyne.NewSize(0, procRowH))
		toggle.SetText("")
		toggle.onTap = nil
		name.SetText("")
		pid.SetText("")
		user.SetText("")
		icon.Resource = nil
		icon.Refresh()
		return
	}
	r := a.procRows[id]
	indent.SetMinSize(fyne.NewSize(float32(r.depth)*procTreeIndent, procRowH))
	switch {
	case !r.hasKids:
		toggle.SetText("")
		toggle.onTap = nil
	case r.expanded:
		toggle.SetText("\u25be")
		toggle.onTap = func() { a.toggleTreeRow(id) }
	default:
		toggle.SetText("\u25b8")
		toggle.onTap = func() { a.toggleTreeRow(id) }
	}
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

// toggleTreeRow expands or collapses a node.
func (a *App) toggleTreeRow(row int) {
	if row < 0 || row >= len(a.procRows) {
		return
	}
	r := a.procRows[row]
	if !r.hasKids {
		return
	}
	if a.expanded == nil {
		a.expanded = map[int]bool{}
	}
	a.expanded[r.pid] = !r.expanded
	a.applyFilter()
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

// applyFilter recomputes the visible process rows, as a flat list or a tree.
func (a *App) applyFilter() {
	q := ""
	if a.procFilter != nil {
		q = strings.ToLower(strings.TrimSpace(a.procFilter.Text))
	}
	a.procRows = a.procRows[:0]
	if a.treeMode {
		a.buildTreeRows(q)
	} else {
		for i, p := range a.procs {
			if !a.matches(p, q) {
				continue
			}
			a.procRows = append(a.procRows, procRow{index: i, pid: p.PID, ppid: p.PPID, name: p.Name, user: a.username(p.UID)})
		}
		a.sortProcRows()
	}
	a.updateProcHeaders()
	if a.procList != nil {
		a.procList.Refresh()
	}
}

func (a *App) matches(p mem.Process, q string) bool {
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Cmdline), q) ||
		strings.Contains(strings.ToLower(a.username(p.UID)), q) ||
		strings.Contains(strconv.Itoa(p.PID), q)
}

// buildTreeRows flattens the process tree, honouring collapsed nodes. With a
// filter, a process is shown when it or any descendant matches, so matches
// keep their ancestry.
func (a *App) buildTreeRows(q string) {
	byPID := make(map[int]int, len(a.procs))
	for i, p := range a.procs {
		byPID[p.PID] = i
	}
	children := map[int][]int{}
	var roots []int
	for i, p := range a.procs {
		parent, ok := byPID[p.PPID]
		if !ok || p.PPID == p.PID || p.PPID <= 0 {
			roots = append(roots, i)
			continue
		}
		children[parent] = append(children[parent], i)
	}

	visible := make(map[int]bool, len(a.procs))
	if q == "" {
		for i := range a.procs {
			visible[i] = true
		}
	} else {
		var mark func(i int)
		mark = func(i int) {
			if visible[i] {
				return
			}
			visible[i] = true
			if parent, ok := byPID[a.procs[i].PPID]; ok && parent != i {
				mark(parent)
			}
		}
		for i, p := range a.procs {
			if a.matches(p, q) {
				mark(i)
			}
		}
	}

	var walk func(i, depth int)
	walk = func(i, depth int) {
		p := a.procs[i]
		kids := children[i]
		a.sortIndices(kids)
		hasKids := false
		for _, k := range kids {
			if visible[k] {
				hasKids = true
				break
			}
		}
		expanded, ok := a.expanded[p.PID]
		if !ok {
			expanded = true
		}
		a.procRows = append(a.procRows, procRow{
			index: i, pid: p.PID, ppid: p.PPID, name: p.Name,
			user: a.username(p.UID), depth: depth, hasKids: hasKids, expanded: expanded,
		})
		if !hasKids || !expanded {
			return
		}
		for _, k := range kids {
			if visible[k] {
				walk(k, depth+1)
			}
		}
	}
	a.sortIndices(roots)
	for _, r := range roots {
		if visible[r] {
			walk(r, 0)
		}
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
	sort.SliceStable(a.procRows, func(i, j int) bool {
		ri, rj := a.procRows[i], a.procRows[j]
		var less, equal bool
		switch a.procSortCol {
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
		if a.procSortAsc {
			return less
		}
		return !less
	})
}

func (a *App) sortIndices(idxs []int) {
	sort.SliceStable(idxs, func(i, j int) bool {
		return a.lessProcess(a.procs[idxs[i]], a.procs[idxs[j]])
	})
}

func (a *App) lessProcess(pi, pj mem.Process) bool {
	var less, equal bool
	switch a.procSortCol {
	case 1:
		less, equal = pi.PID < pj.PID, pi.PID == pj.PID
	case 2:
		li, lj := strings.ToLower(a.username(pi.UID)), strings.ToLower(a.username(pj.UID))
		less, equal = li < lj, li == lj
	default:
		li, lj := strings.ToLower(pi.Name), strings.ToLower(pj.Name)
		less, equal = li < lj, li == lj
	}
	if equal {
		return pi.PID < pj.PID
	}
	if a.procSortAsc {
		return less
	}
	return !less
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
	a.updateScanControls()
}
