//go:build gui

package ui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/autoasm"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/customtype"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/dissect"
	"github.com/LCRERGO/firstspark/pkg/inject"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// tableEntry is one row of the cheat table.
type tableEntry struct {
	addr    uint64
	typ     scan.ValueType
	desc    string
	value   scan.Value
	orig    scan.Value
	pointer *pointerChain
	bit     *bitSpec
	display displayFormat
	hotkey  fyne.KeyName
}

// App is the root UI state.
type App struct {
	cfg  config.Config
	fapp fyne.App
	win  fyne.Window
	th   *cyberTheme

	procs          []mem.Process
	proc           *mem.Process
	procRows       []procRow
	procSortCol    int
	procSortAsc    bool
	procHeaderBtns []*ttwidget.Button
	procFilter     *toolTipEntry
	procTree       *ttwidget.Check
	procList       *widget.List
	procWin        fyne.Window
	processLabel   *tapLabel
	treeMode       bool
	expanded       map[int]bool

	users     map[int]string
	icons     *iconResolver
	showIcons bool

	viewMenu   *fyne.Menu
	themeItems []*fyne.MenuItem

	session    *scan.Session
	regionSel  []mem.Region
	results    []scan.Result
	foundList  *widget.List
	foundSel   int
	foundCount *widget.Label
	status     *widget.Label

	entries  []tableEntry
	table    *widget.Table
	tableSel int

	scanType     *ttwidget.Select
	valueType    *ttwidget.Select
	hexBox       *ttwidget.Check
	valueEntry   *toolTipEntry
	value2Entry  *toolTipEntry
	compareEntry *toolTipEntry
	writable     *ttwidget.Check
	speedhack    *ttwidget.Check
	speedScale   *toolTipEntry
	speedHooks   []*inject.Hook
	speedApplied bool
	alignEntry   *toolTipEntry
	scanBtn      *ttwidget.Button
	nextBtn      *ttwidget.Button
	undoBtn      *ttwidget.Button
	stopBtn      *ttwidget.Button
	andLabel     *widget.Label
	valuePair    *fyne.Container
	scanProgress *progressLine
	scanStatus   *widget.Label
	scopeSelect  *ttwidget.Select
	scanCancel   context.CancelFunc
	scanning     bool

	openProcAction *widget.ToolbarAction
	loadAction     *widget.ToolbarAction
	saveAction     *widget.ToolbarAction
	saveAsAction   *widget.ToolbarAction
	memViewAction  *widget.ToolbarAction
	addAddrAction  *widget.ToolbarAction
	clearAction    *widget.ToolbarAction
	settingsAction *widget.ToolbarAction

	dbgWin         fyne.Window
	dbgSession     *debugger.Session
	dbgRegs        *widget.List
	dbgHits        *widget.List
	dbgRegVals     []string
	dbgHitLabels   []string
	dbgAddrEntry   *widget.Entry
	dbgStatus      *widget.Label
	dbgStop        chan struct{}
	dbgBreakpoints map[uint64]bool
	dbgWatchpoints map[uint64]int
	dbgWatchWrite  map[uint64]bool
	dbgBPList      *widget.List
	dbgBPLabels    []string
	dbgRegEdit     *widget.Entry

	asmWin    fyne.Window
	asmEditor *codeEditor
	asmStatus *widget.Label
	asmExec   *autoasm.Executor

	ctWin       fyne.Window
	ctList      *widget.List
	ctDefs      []customtype.Definition
	ctSel       int
	ctID        scan.ValueType
	ctName      *widget.Entry
	ctMode      *widget.Select
	ctSize      *widget.Entry
	ctKind      *widget.Select
	ctAlign     *widget.Entry
	ctDesc      *widget.Entry
	ctEditor    *codeEditor
	ctStatus    *widget.Label
	ctTestBytes *widget.Entry
	ctTestAddr  *widget.Entry
	ctTestOut   *widget.Label

	dissectWin       fyne.Window
	dissectTable     *widget.Table
	dissectFields    []dissect.Field
	dissectBases     []uint64
	dissectBase      uint64
	dissectSel       int
	dissectBaseEntry *widget.Entry
	dissectSizeEntry *widget.Entry
	dissectInstEntry *widget.Entry
	dissectStatus    *widget.Label

	memWin       fyne.Window
	hexAddr      uint64
	hexData      []byte
	disasm       []asm.Instruction
	disasmList   *widget.List
	hexList      *widget.List
	hexLines     []string
	memAddrEntry *widget.Entry
	memType      *widget.Select
	searchPat    []byte
	searchMask   []byte
	searchNext   uint64

	mu     sync.Mutex
	frozen map[uint64]scan.Value
	stop   chan struct{}
	err    error
}

// Run opens the application window and blocks until it is closed.
func Run(cfg config.Config) error {
	// Fyne reads the interface scale from the environment at startup; there is
	// no runtime setter, so a scale change takes effect on the next launch.
	if cfg.UI.Scale > 0 && cfg.UI.Scale != 1 {
		os.Setenv("FYNE_SCALE", strconv.FormatFloat(cfg.UI.Scale, 'g', -1, 64))
	}
	a := &App{
		cfg:         cfg,
		frozen:      map[uint64]scan.Value{},
		stop:        make(chan struct{}),
		foundSel:    -1,
		tableSel:    -1,
		procSortCol: 0,
		procSortAsc: true,
		showIcons:   cfg.UI.ProcessIcons,
		expanded:    map[int]bool{},
		treeMode:    true,
	}
	a.icons = newIconResolver()
	a.fapp = app.NewWithID("com.firstspark.app")
	a.th = newTheme(parseScheme(cfg.UI.Theme), cfg.UI.FontSize)
	a.fapp.Settings().SetTheme(a.th)
	loadErr := func() error {
		_, err := customtype.LoadAndRegister(config.CustomTypesPath())
		return err
	}()
	a.build()
	if loadErr != nil {
		a.fail(loadErr)
	}
	a.refreshProcesses()
	go a.freezeLoop()
	a.win.Show()
	closeParentInstance()
	a.maybeAskElevation()
	a.fapp.Run()
	return a.err
}

func (a *App) build() {
	a.win = a.fapp.NewWindow(i18n.T("app.title"))
	a.win.Resize(fyne.NewSize(780, 600))
	a.win.CenterOnScreen()
	a.buildWidgets()
	a.win.SetMainMenu(a.mainMenu())
	a.win.SetContent(fynetooltip.AddWindowToolTipLayer(a.content(), a.win.Canvas()))
	a.installShortcuts()
	a.updateScanControls()
}

func (a *App) buildWidgets() {
	a.processLabel = &tapLabel{Label: widget.NewLabel(i18n.T("app.no_process")), onTap: a.openProcessList}
	a.foundCount = widget.NewLabel(i18n.Tf("app.found_count", map[string]any{"Count": 0}))
	a.status = widget.NewLabel("")

	a.valueEntry = newToolTipEntry()
	a.valueEntry.SetPlaceHolder(i18n.T("app.value_placeholder"))
	a.valueEntry.OnSubmitted = func(string) { a.scanAction() }

	a.value2Entry = newToolTipEntry()
	a.value2Entry.SetPlaceHolder(i18n.T("app.upper_bound_placeholder"))
	a.value2Entry.OnSubmitted = func(string) { a.scanAction() }

	a.compareEntry = newToolTipEntry()
	a.compareEntry.SetText("==")

	a.scanType = ttwidget.NewSelect(scanTypeLabels(), func(string) { a.updateScanControls() })
	a.scanType.SetSelected(scanTypeLabel(scan.ModeExact))
	a.valueType = ttwidget.NewSelect(valueTypeOptions(), func(label string) {
		if n := customTypeAlignment(label); n > 0 {
			a.alignEntry.SetText(strconv.Itoa(n))
		}
		a.updateValueHint()
	})
	a.valueType.SetSelected(ceValueTypeLabel(a.defaultValueType()))

	a.hexBox = newHintCheck(i18n.T("app.hex"), "scan.hint.hex", func(bool) {})
	a.writable = newHintCheck(i18n.T("app.writable"), "scan.hint.writable", func(bool) {})
	a.writable.SetChecked(a.cfg.Scan.WritableOnly)
	a.speedhack = newHintCheck(i18n.T("app.enable_speedhack"), "scan.hint.speedhack", func(on bool) { a.setSpeedhack(on) })
	a.speedhack.SetChecked(a.cfg.Speedhack.Enabled)
	a.speedScale = newHintEntry("scan.hint.speedhack_scale")
	a.speedScale.SetText(strconv.FormatFloat(a.cfg.Speedhack.Scale, 'g', -1, 64))

	a.alignEntry = newHintEntry("scan.hint.alignment")
	a.alignEntry.SetText(strconv.Itoa(a.cfg.Scan.Alignment))

	a.buildFoundList()
	a.buildCheatTable()
	a.applyHints()
}

func (a *App) defaultValueType() scan.ValueType {
	if t, err := scan.ParseValueType(a.cfg.Scan.ValueType); err == nil {
		return t
	}
	return scan.TypeDword
}

func (a *App) content() fyne.CanvasObject {
	top := container.NewHSplit(a.foundPanel(), a.scanPanel())
	top.SetOffset(0.46)
	body := container.NewVSplit(top, a.cheatPanel())
	body.SetOffset(0.74)
	bar := container.NewBorder(nil, nil, a.processLabel, container.NewHBox(a.status, a.foundCount))
	return container.NewBorder(a.toolbar(), bar, nil, nil, body)
}

func (a *App) toolbar() *widget.Toolbar {
	a.openProcAction = widget.NewToolbarAction(theme.ComputerIcon(), a.openProcessList)
	a.loadAction = widget.NewToolbarAction(theme.FolderOpenIcon(), a.loadTable)
	a.saveAction = widget.NewToolbarAction(theme.DocumentSaveIcon(), a.saveTable)
	a.memViewAction = widget.NewToolbarAction(theme.StorageIcon(), a.openMemoryViewer)
	a.addAddrAction = widget.NewToolbarAction(theme.ContentAddIcon(), a.addAddressDialog)
	a.clearAction = widget.NewToolbarAction(theme.DeleteIcon(), a.clearTable)
	a.settingsAction = widget.NewToolbarAction(theme.SettingsIcon(), a.showSettings)
	return widget.NewToolbar(
		a.openProcAction,
		a.loadAction,
		a.saveAction,
		widget.NewToolbarSeparator(),
		a.memViewAction,
		a.addAddrAction,
		a.clearAction,
		widget.NewToolbarSpacer(),
		a.settingsAction,
	)
}

// updateScanControls enables only the controls that apply in the current
// state, mirroring Cheat Engine's blocked buttons.
func (a *App) updateScanControls() {
	mode := parseCEScanType(a.scanType.Selected)

	if a.scanning {
		if a.scanBtn != nil {
			setEnabled(a.scanBtn, false)
		}
		if a.nextBtn != nil {
			setEnabled(a.nextBtn, false)
		}
		if a.undoBtn != nil {
			setEnabled(a.undoBtn, false)
		}
		if a.stopBtn != nil {
			setEnabled(a.stopBtn, true)
		}
		return
	}
	if a.stopBtn != nil {
		setEnabled(a.stopBtn, false)
	}
	if a.scanBtn != nil {
		setEnabled(a.scanBtn, a.proc != nil)
	}
	if a.nextBtn != nil {
		setEnabled(a.nextBtn, a.session != nil)
	}
	if a.undoBtn != nil {
		setEnabled(a.undoBtn, a.session != nil && a.session.CanUndo())
	}
	if a.valueEntry != nil {
		setEnabled(a.valueEntry, modeNeedsValue(mode))
		a.valueEntry.SetPlaceHolder(valuePlaceholder(mode))
	}
	between := mode == scan.ModeBetween
	if a.andLabel != nil {
		if between {
			a.andLabel.Show()
		} else {
			a.andLabel.Hide()
		}
		a.andLabel.Refresh()
	}
	if a.value2Entry != nil {
		setEnabled(a.value2Entry, between)
		if between {
			a.value2Entry.Show()
		} else {
			a.value2Entry.Hide()
		}
		a.value2Entry.Refresh()
	}
	if a.compareEntry != nil {
		setEnabled(a.compareEntry, mode == scan.ModeExact)
	}

	hasTable := len(a.entries) > 0
	setActionEnabled(a.saveAction, hasTable)
	setActionEnabled(a.saveAsAction, hasTable)
	setActionEnabled(a.memViewAction, a.proc != nil)
	setActionEnabled(a.addAddrAction, a.proc != nil)
	setActionEnabled(a.clearAction, hasTable)
}

type disableable interface {
	Disable()
	Enable()
}

func setEnabled(w disableable, enabled bool) {
	if enabled {
		w.Enable()
	} else {
		w.Disable()
	}
}

func setActionEnabled(action *widget.ToolbarAction, enabled bool) {
	if action == nil {
		return
	}
	if enabled {
		action.Enable()
	} else {
		action.Disable()
	}
}

func (a *App) mainMenu() *fyne.MainMenu {
	openProc := fyne.NewMenuItem(i18n.T("menu.file.open_process"), a.openProcessList)
	openProc.Shortcut = ctrl(fyne.KeyP)
	load := fyne.NewMenuItem(i18n.T("menu.file.load"), a.loadTable)
	load.Shortcut = ctrl(fyne.KeyO)
	save := fyne.NewMenuItem(i18n.T("menu.file.save"), a.saveTable)
	save.Shortcut = ctrl(fyne.KeyS)
	saveAs := fyne.NewMenuItem(i18n.T("menu.file.save_as"), a.saveTableAs)
	saveAs.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl | fyne.KeyModifierAlt}
	saveRes := fyne.NewMenuItem(i18n.T("menu.file.save_scan_results"), a.saveScanResults)
	saveRes.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierAlt | fyne.KeyModifierShift}
	quit := fyne.NewMenuItem(i18n.T("menu.file.quit"), a.fapp.Quit)
	file := fyne.NewMenu(i18n.T("menu.file"),
		openProc,
		fyne.NewMenuItemSeparator(),
		load, save, saveAs,
		fyne.NewMenuItemSeparator(),
		saveRes,
		fyne.NewMenuItemSeparator(),
		quit,
	)

	settings := fyne.NewMenuItem(i18n.T("menu.edit.settings"), a.showSettings)
	edit := fyne.NewMenu(i18n.T("menu.edit"), settings)

	a.themeItems = []*fyne.MenuItem{
		fyne.NewMenuItem(i18n.T("menu.view.light"), func() { a.setTheme(schemeLight) }),
		fyne.NewMenuItem(i18n.T("menu.view.dark"), func() { a.setTheme(schemeDark) }),
		fyne.NewMenuItem(i18n.T("menu.view.system"), func() { a.setTheme(schemeSystem) }),
	}
	a.viewMenu = fyne.NewMenu(i18n.T("menu.view"), a.themeItems...)
	a.updateThemeChecks()

	addAddr := fyne.NewMenuItem(i18n.T("menu.table.add_address"), a.addAddressDialog)
	clear := fyne.NewMenuItem(i18n.T("menu.table.clear"), a.clearTable)
	custom := fyne.NewMenuItem(i18n.T("menu.table.custom_types"), a.showCustomTypes)
	pointer := fyne.NewMenuItem(i18n.T("menu.table.pointer_scan"), a.showPointerScan)
	loadPointer := fyne.NewMenuItem(i18n.T("menu.table.load_pointer_scan"), a.loadPointerScan)
	table := fyne.NewMenu(i18n.T("menu.table"), addAddr, clear, fyne.NewMenuItemSeparator(), pointer, loadPointer, custom)

	speed := fyne.NewMenuItem(i18n.T("menu.tools.speedhack"), a.toggleSpeedhack)
	debuggerItem := fyne.NewMenuItem(i18n.T("menu.tools.debugger"), a.openDebugger)
	dissectItem := fyne.NewMenuItem(i18n.T("menu.tools.dissect"), a.openDissect)
	autoasmItem := fyne.NewMenuItem(i18n.T("menu.tools.auto_assemble"), a.openAutoAssemble)
	tools := fyne.NewMenu(i18n.T("menu.tools"), debuggerItem, dissectItem, autoasmItem, speed)

	about := fyne.NewMenuItem(i18n.T("menu.help.about"), a.showAbout)
	help := fyne.NewMenu(i18n.T("menu.help"), about)

	return fyne.NewMainMenu(file, edit, a.viewMenu, table, tools, help)
}

// setTheme applies and persists a colour scheme, keeping the View menu in sync.
func (a *App) setTheme(m scheme) {
	a.cfg.UI.Theme = m.String()
	a.applyTheme()
	a.updateThemeChecks()
	a.saveConfig()
	a.setStatusText(i18n.Tf("status.theme", map[string]any{"Theme": m.String()}))
}

// updateThemeChecks marks the active scheme in the View > Theme menu.
func (a *App) updateThemeChecks() {
	active := parseScheme(a.cfg.UI.Theme)
	modes := []scheme{schemeLight, schemeDark, schemeSystem}
	for i, item := range a.themeItems {
		item.Checked = active == modes[i]
	}
	if a.viewMenu != nil {
		a.viewMenu.Refresh()
	}
}

func (a *App) installShortcuts() {
	canvas := a.win.Canvas()
	canvas.AddShortcut(ctrl(fyne.KeyM), func(fyne.Shortcut) { a.openMemoryViewer() })
	canvas.AddShortcut(ctrl(fyne.KeyB), func(fyne.Shortcut) { a.browseRow(a.tableSel) })
	canvas.AddShortcut(ctrl(fyne.KeyD), func(fyne.Shortcut) { a.disassembleRow(a.tableSel) })
	canvas.AddShortcut(ctrl(fyne.KeyE), func(fyne.Shortcut) { a.changeValueDialog(a.tableSel) })
	canvas.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierControl | fyne.KeyModifierAlt},
		func(fyne.Shortcut) { a.changeValueBack(a.tableSel) })
}

func ctrl(k fyne.KeyName) fyne.Shortcut {
	return &desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierControl}
}

// pal returns the active palette for manually coloured canvas objects.
func (a *App) pal() palette { return a.th.pal(a.variant()) }

func (a *App) variant() fyne.ThemeVariant {
	if a.fapp == nil {
		return theme.VariantLight
	}
	return a.fapp.Settings().ThemeVariant()
}

func (a *App) setStatus(format string, args ...any) {
	a.status.SetText(fmt.Sprintf(format, args...))
}

// setStatusText sets an already-formatted (typically translated) status line.
func (a *App) setStatusText(text string) {
	a.status.SetText(text)
}

func (a *App) fail(err error) {
	if err == nil {
		return
	}
	dialog.ShowError(err, a.win)
}

func (a *App) isFrozen(addr uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.frozen[addr]
	return ok
}

func (a *App) freezeLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	tick := 0
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
			a.mu.Lock()
			proc := a.proc
			if proc != nil {
				for addr, v := range a.frozen {
					_ = proc.Write(addr, v.Raw)
				}
			}
			a.mu.Unlock()
			tick++
			if tick%10 == 0 {
				fyne.Do(func() {
					if a.resolvePointers() && a.table != nil {
						a.table.Refresh()
					}
				})
			}
		}
	}
}

// tapLabel is a label that runs a callback when tapped.
type tapLabel struct {
	*widget.Label
	onTap func()
}

func (t *tapLabel) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

// monoText creates a monospaced canvas text in the given colour.
func (a *App) monoText(text string) *canvas.Text {
	return a.th.monoText(text, a.pal().text)
}
