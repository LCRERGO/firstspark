//go:build gui

package ui

import (
	"context"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// scanTab is a browser-style scan workspace. It owns a Found list, the scan
// controls and one scan session; the cheat table and process controls are
// shared on App.
type scanTab struct {
	*App

	item *container.TabItem

	session   *scan.Session
	regionSel []mem.Region
	results   []scan.Result

	foundList    *widget.Table
	foundOrder   []int
	foundLive    map[int]scan.Value
	foundRegions []mem.Region
	foundSel     int
	foundMulti   map[int]bool
	foundCount   *widget.Label
	foundDisplay displayFormat
	foundSortCol int
	foundSortAsc bool

	scanType      *ttwidget.Select
	valueType     *ttwidget.Select
	hexBox        *ttwidget.Check
	valueEntry    *toolTipEntry
	value2Entry   *toolTipEntry
	compareSelect *ttwidget.Select
	execSelect    *ttwidget.Select
	cowCheck      *ttwidget.Check
	startEntry    *toolTipEntry
	stopEntry     *toolTipEntry
	writable      *ttwidget.Check
	alignEntry    *toolTipEntry
	scanBtn       *ttwidget.Button
	nextBtn       *ttwidget.Button
	undoBtn       *ttwidget.Button
	stopBtn       *ttwidget.Button
	andLabel      *widget.Label
	valuePair     *fyne.Container
	scanProgress  *progressLine
	scanStatus    *widget.Label
	scopeSelect   *ttwidget.Select
	scanCancel    context.CancelFunc
	scanning      bool
}

// newScanTab builds a tab and its widgets.
func (a *App) newScanTab(name string) *scanTab {
	t := &scanTab{App: a, foundSel: -1, foundSortCol: -1}

	t.valueEntry = newToolTipEntry()
	t.valueEntry.SetPlaceHolder(i18n.T("app.value_placeholder"))
	t.valueEntry.OnSubmitted = func(string) { t.scanAction() }

	t.value2Entry = newToolTipEntry()
	t.value2Entry.SetPlaceHolder(i18n.T("app.upper_bound_placeholder"))
	t.value2Entry.OnSubmitted = func(string) { t.scanAction() }

	t.compareSelect = newHintSelect(compareLabels(), "scan.hint.compare", nil)
	t.compareSelect.SetSelected(scan.OpEqual.String())

	t.execSelect = newHintSelect(execLabels(), "scan.hint.executable", nil)
	t.execSelect.SetSelected(execLabel(scan.ExecAny))
	t.cowCheck = newHintCheck(i18n.T("scan.copy_on_write"), "scan.hint.copy_on_write", nil)
	t.startEntry = newHintEntry("scan.hint.range")
	t.stopEntry = newHintEntry("scan.hint.range")

	t.scanType = ttwidget.NewSelect(scanTypeLabelsFor(false), func(string) { t.updateScanControls() })
	t.scanType.SetSelected(scanTypeLabel(scan.ModeExact))

	t.alignEntry = newHintEntry("scan.hint.alignment")
	t.alignEntry.SetText(strconv.Itoa(a.cfg.Scan.Alignment))

	t.valueType = ttwidget.NewSelect(valueTypeOptions(), func(label string) {
		if n := customTypeAlignment(label); n > 0 {
			t.alignEntry.SetText(strconv.Itoa(n))
		}
		t.updateValueHint()
	})
	t.valueType.SetSelected(ceValueTypeLabel(a.defaultValueType()))

	t.hexBox = newHintCheck(i18n.T("app.hex"), "scan.hint.hex", func(bool) {})
	t.writable = newHintCheck(i18n.T("app.writable"), "scan.hint.writable", func(bool) {})
	t.writable.SetChecked(a.cfg.Scan.WritableOnly)

	t.foundCount = widget.NewLabel(i18n.Tf("app.found_count", map[string]any{"Count": 0}))
	t.buildFoundList()
	t.applyHints()

	t.item = container.NewTabItem(name, t.workspace())
	return t
}

// workspace is the tab content: the Found list beside the scan panel.
func (t *scanTab) workspace() fyne.CanvasObject {
	top := container.NewHSplit(t.foundPanel(), t.scanPanel())
	top.SetOffset(0.46)
	return top
}

// clearResults empties the tab's scan session and Found list, cancelling any
// in-flight scan.
func (t *scanTab) clearResults() {
	if t.scanCancel != nil {
		t.scanCancel()
	}
	t.session = nil
	t.regionSel = nil
	t.results = nil
	t.foundOrder = nil
	t.foundLive = nil
	t.foundRegions = nil
	t.foundSel = -1
	t.foundMulti = nil
	if t.foundCount != nil {
		t.foundCount.SetText(i18n.Tf("app.found_count", map[string]any{"Count": 0}))
	}
	t.refreshFound()
	t.updateScanTypeOptions()
	t.updateScanControls()
}

// createTab registers a new tab and returns its TabItem. It backs DocTabs'
// CreateTab hook.
func (a *App) createTab() *container.TabItem {
	t := a.newScanTab(i18n.Tf("tabs.default_name", map[string]any{"N": a.tabSeq}))
	a.tabSeq++
	a.tabs = append(a.tabs, t)
	return t.item
}

// firstScan and nextScan run on the active tab, for menu and hotkey callers.
func (a *App) firstScan() {
	if t := a.tab(); t != nil {
		t.firstScan()
	}
}

func (a *App) nextScan() {
	if t := a.tab(); t != nil {
		t.nextScan()
	}
}

func (a *App) undoScan() {
	if t := a.tab(); t != nil {
		t.undoScan()
	}
}

func (a *App) stopScan() {
	if t := a.tab(); t != nil {
		t.stopScan()
	}
}

// addScanTab is the Ctrl+T / menu action: create and select a tab.
func (a *App) addScanTab() {
	if a.tabsWidget == nil {
		return
	}
	item := a.createTab()
	a.tabsWidget.Append(item)
	a.tabsWidget.SelectIndex(len(a.tabsWidget.Items) - 1)
}

// closeScanTab backs DocTabs' CloseIntercept. The last tab is cleared rather
// than removed, and a tab with results asks for confirmation.
func (a *App) closeScanTab(item *container.TabItem) {
	idx := a.tabIndex(item)
	if idx < 0 {
		return
	}
	if len(a.tabs) == 1 {
		a.tabs[idx].clearResults()
		return
	}
	if len(a.tabs[idx].results) > 0 {
		dialog.ShowConfirm(i18n.T("tabs.close_title"), i18n.T("tabs.close_body"), func(ok bool) {
			if ok {
				a.removeTabAt(idx)
			}
		}, a.win)
		return
	}
	a.removeTabAt(idx)
}

// removeTabAt drops the tab at idx from both the model and the container.
func (a *App) removeTabAt(idx int) {
	if idx < 0 || idx >= len(a.tabs) {
		return
	}
	if a.tabs[idx].scanCancel != nil {
		a.tabs[idx].scanCancel()
	}
	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)
	if a.tabsWidget != nil {
		a.tabsWidget.RemoveIndex(idx)
		if sel := a.tabsWidget.SelectedIndex(); sel >= 0 && sel < len(a.tabs) {
			a.activeTab = sel
		} else {
			a.activeTab = 0
		}
	}
	a.syncActiveTab()
}

// tabIndex maps a TabItem to its position in the model.
func (a *App) tabIndex(item *container.TabItem) int {
	for i, t := range a.tabs {
		if t.item == item {
			return i
		}
	}
	return -1
}

// onTabSelected makes the clicked tab active and refreshes its live values.
func (a *App) onTabSelected(item *container.TabItem) {
	idx := a.tabIndex(item)
	if idx < 0 {
		return
	}
	a.activeTab = idx
	a.syncActiveTab()
}

// syncActiveTab refreshes the active tab's live values and controls.
func (a *App) syncActiveTab() {
	t := a.tab()
	if t == nil {
		return
	}
	t.refreshFoundValues()
	t.refreshFound()
	t.updateScanTypeOptions()
	t.updateScanControls()
}

// cycleTab switches to the next (dir=1) or previous (dir=-1) tab, wrapping.
func (a *App) cycleTab(dir int) {
	if a.tabsWidget == nil || len(a.tabs) == 0 {
		return
	}
	n := len(a.tabs)
	a.tabsWidget.SelectIndex(((a.activeTab+dir)%n + n) % n)
}

// renameScanTab prompts for a new tab name.
func (a *App) renameScanTab() {
	t := a.tab()
	if t == nil {
		return
	}
	entry := widget.NewEntry()
	entry.SetText(t.item.Text)
	d := dialog.NewForm(i18n.T("tabs.rename_title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		[]*widget.FormItem{widget.NewFormItem(i18n.T("tabs.name"), entry)},
		func(ok bool) {
			name := strings.TrimSpace(entry.Text)
			if !ok || name == "" {
				return
			}
			t.item.Text = name
			if a.tabsWidget != nil {
				a.tabsWidget.Refresh()
			}
		}, a.win)
	d.Resize(fyne.NewSize(340, 160))
	d.Show()
}

// resetTabs replaces all tabs with one fresh tab, used by New Table.
func (a *App) resetTabs() {
	for _, t := range a.tabs {
		if t.scanCancel != nil {
			t.scanCancel()
		}
	}
	a.tabs = nil
	a.activeTab = 0
	a.tabSeq = 1
	if a.tabsWidget == nil {
		return
	}
	a.tabsWidget.SetItems(nil)
	item := a.createTab()
	a.tabsWidget.Append(item)
	a.tabsWidget.SelectIndex(0)
	a.syncActiveTab()
}

// clearTabSessions empties every tab's scan session on a process change while
// keeping the tab layout.
func (a *App) clearTabSessions() {
	for _, t := range a.tabs {
		t.clearResults()
	}
	if t := a.tab(); t != nil {
		t.updateScanTypeOptions()
		t.updateScanControls()
	}
}

// tabCompareOps pairs a translation key with a scan-tab set operation.
var tabCompareOps = []struct {
	key string
	op  scan.SetOp
}{
	{"compare.only_this", scan.SetOnlyA},
	{"compare.only_other", scan.SetOnlyB},
	{"compare.both", scan.SetBoth},
	{"compare.either", scan.SetEither},
}

func compareOpLabels() []string {
	out := make([]string, len(tabCompareOps))
	for i, o := range tabCompareOps {
		out[i] = i18n.T(o.key)
	}
	return out
}

func compareOpFor(label string) (scan.SetOp, bool) {
	for _, o := range tabCompareOps {
		if i18n.T(o.key) == label {
			return o.op, true
		}
	}
	return scan.SetOnlyA, false
}

// compareTabs combines the active tab's results with another tab's into a new
// tab. It is offered only when both tabs share a value type.
func (a *App) compareTabs() {
	active := a.tab()
	if active == nil || active.session == nil {
		a.setStatusText(i18n.T("status.compare_need_scans"))
		return
	}
	if len(a.tabs) < 2 {
		a.setStatusText(i18n.T("status.compare_need_two"))
		return
	}
	others := make([]string, 0, len(a.tabs)-1)
	byName := map[string]*scanTab{}
	for _, t := range a.tabs {
		if t == active {
			continue
		}
		others = append(others, t.item.Text)
		byName[t.item.Text] = t
	}
	tabSel := widget.NewSelect(others, nil)
	if len(others) > 0 {
		tabSel.SetSelectedIndex(0)
	}
	opSel := widget.NewSelect(compareOpLabels(), nil)
	opSel.SetSelectedIndex(0)

	d := dialog.NewForm(i18n.T("compare.title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		[]*widget.FormItem{
			widget.NewFormItem(i18n.T("compare.tab"), tabSel),
			widget.NewFormItem(i18n.T("compare.operation"), opSel),
		}, func(ok bool) {
			if !ok {
				return
			}
			other := byName[tabSel.Selected]
			op, _ := compareOpFor(opSel.Selected)
			if other != nil {
				a.runCompare(active, other, op)
			}
		}, a.win)
	d.Resize(fyne.NewSize(400, 220))
	d.Show()
}

// runCompare seeds a new tab with the compared result set.
func (a *App) runCompare(active, other *scanTab, op scan.SetOp) {
	if active.session == nil || other.session == nil {
		a.setStatusText(i18n.T("status.compare_need_scans"))
		return
	}
	if active.session.Options().Type != other.session.Options().Type {
		a.setStatusText(i18n.T("status.compare_type_mismatch"))
		return
	}
	merged := scan.CompareResultSets(active.results, other.results, op)
	item := a.createTab()
	tab := a.tabs[len(a.tabs)-1]
	tab.item.Text = i18n.Tf("tabs.compare_name", map[string]any{"N": a.tabSeq - 1})
	tab.session = scan.NewSessionFromResults(a.proc, active.session.Options(), merged)
	tab.setResults(merged)
	a.tabsWidget.Append(item)
	a.tabsWidget.SelectIndex(len(a.tabs) - 1)
	a.tabsWidget.Refresh()
	a.setStatusText(i18n.Tf("status.compare_done", map[string]any{"Count": len(merged)}))
}
