//go:build gui

package ui

import (
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

	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/autoasm"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/customtype"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/dissect"
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
	procHeaderBtns []*widget.Button
	procFilter     *widget.Entry
	procList       *widget.List
	procWin        fyne.Window
	processLabel   *tapLabel

	users     map[int]string
	icons     *iconResolver
	showIcons bool

	viewMenu   *fyne.Menu
	themeItems []*fyne.MenuItem

	session    *scan.Session
	results    []scan.Result
	foundList  *widget.List
	foundSel   int
	foundCount *widget.Label
	status     *widget.Label

	entries  []tableEntry
	table    *widget.Table
	tableSel int

	scanType     *widget.Select
	valueType    *widget.Select
	hexBox       *widget.Check
	valueEntry   *widget.Entry
	value2Entry  *widget.Entry
	compareEntry *widget.Entry
	writable     *widget.Check
	speedhack    *widget.Check
	alignEntry   *widget.Entry
	scanBtn      *widget.Button
	nextBtn      *widget.Button
	undoBtn      *widget.Button
	value2Row    *fyne.Container

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
	a.win = a.fapp.NewWindow("Firstspark")
	a.win.Resize(fyne.NewSize(1100, 760))
	a.win.CenterOnScreen()
	a.buildWidgets()
	a.win.SetMainMenu(a.mainMenu())
	a.win.SetContent(a.content())
	a.installShortcuts()
	a.updateScanControls()
}

func (a *App) buildWidgets() {
	a.processLabel = &tapLabel{Label: widget.NewLabel("No Process Selected"), onTap: a.openProcessList}
	a.foundCount = widget.NewLabel("Found: 0")
	a.status = widget.NewLabel("")

	a.valueEntry = widget.NewEntry()
	a.valueEntry.SetPlaceHolder("value or AOB pattern")
	a.valueEntry.OnSubmitted = func(string) { a.scanAction() }

	a.value2Entry = widget.NewEntry()
	a.value2Entry.SetPlaceHolder("upper bound (Value between)")
	a.value2Entry.OnSubmitted = func(string) { a.scanAction() }

	a.compareEntry = widget.NewEntry()
	a.compareEntry.SetText("==")

	a.scanType = widget.NewSelect(scanTypeOptions, func(string) { a.updateScanControls() })
	a.scanType.SetSelected("Exact value")
	a.valueType = widget.NewSelect(valueTypeOptions(), func(label string) {
		if n := customTypeAlignment(label); n > 0 {
			a.alignEntry.SetText(strconv.Itoa(n))
		}
	})
	a.valueType.SetSelected(ceValueTypeLabel(a.defaultValueType()))

	a.hexBox = widget.NewCheck("Hex", func(bool) {})
	a.writable = widget.NewCheck("Writable", func(bool) {})
	a.writable.SetChecked(a.cfg.Scan.WritableOnly)
	a.speedhack = widget.NewCheck("Enable Speedhack", func(bool) {})
	a.speedhack.SetChecked(a.cfg.Speedhack.Enabled)

	a.alignEntry = widget.NewEntry()
	a.alignEntry.SetText(strconv.Itoa(a.cfg.Scan.Alignment))

	a.buildFoundList()
	a.buildCheatTable()
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
	if a.value2Row != nil {
		if mode == scan.ModeBetween {
			a.value2Row.Show()
		} else {
			a.value2Row.Hide()
		}
		a.value2Row.Refresh()
	}
	if a.value2Entry != nil {
		setEnabled(a.value2Entry, mode == scan.ModeBetween)
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
	openProc := fyne.NewMenuItem("Open Process", a.openProcessList)
	openProc.Shortcut = ctrl(fyne.KeyP)
	load := fyne.NewMenuItem("Load...", a.loadTable)
	load.Shortcut = ctrl(fyne.KeyO)
	save := fyne.NewMenuItem("Save", a.saveTable)
	save.Shortcut = ctrl(fyne.KeyS)
	saveAs := fyne.NewMenuItem("Save As...", a.saveTableAs)
	saveAs.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl | fyne.KeyModifierAlt}
	saveRes := fyne.NewMenuItem("Save Scan Results", a.saveScanResults)
	saveRes.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierAlt | fyne.KeyModifierShift}
	quit := fyne.NewMenuItem("Quit", a.fapp.Quit)
	file := fyne.NewMenu("File",
		openProc,
		fyne.NewMenuItemSeparator(),
		load, save, saveAs,
		fyne.NewMenuItemSeparator(),
		saveRes,
		fyne.NewMenuItemSeparator(),
		quit,
	)

	settings := fyne.NewMenuItem("Settings", a.showSettings)
	edit := fyne.NewMenu("Edit", settings)

	a.themeItems = []*fyne.MenuItem{
		fyne.NewMenuItem("Light", func() { a.setTheme(schemeLight) }),
		fyne.NewMenuItem("Dark", func() { a.setTheme(schemeDark) }),
		fyne.NewMenuItem("System", func() { a.setTheme(schemeSystem) }),
	}
	a.viewMenu = fyne.NewMenu("View", a.themeItems...)
	a.updateThemeChecks()

	addAddr := fyne.NewMenuItem("Add Address Manually", a.addAddressDialog)
	clear := fyne.NewMenuItem("Clear List", a.clearTable)
	custom := fyne.NewMenuItem("Custom Types...", a.showCustomTypes)
	pointer := fyne.NewMenuItem("Pointer Scan...", a.showPointerScan)
	table := fyne.NewMenu("Table", addAddr, clear, fyne.NewMenuItemSeparator(), pointer, custom)

	speed := fyne.NewMenuItem("Speedhack", a.toggleSpeedhack)
	debuggerItem := fyne.NewMenuItem("Debugger", a.openDebugger)
	dissectItem := fyne.NewMenuItem("Dissect Data/Structures", a.openDissect)
	autoasmItem := fyne.NewMenuItem("Auto Assemble", a.openAutoAssemble)
	tools := fyne.NewMenu("Tools", debuggerItem, dissectItem, autoasmItem, speed)

	about := fyne.NewMenuItem("About", a.showAbout)
	help := fyne.NewMenu("Help", about)

	return fyne.NewMainMenu(file, edit, a.viewMenu, table, tools, help)
}

// setTheme applies and persists a colour scheme, keeping the View menu in sync.
func (a *App) setTheme(m scheme) {
	a.cfg.UI.Theme = m.String()
	a.applyTheme()
	a.updateThemeChecks()
	a.saveConfig()
	a.setStatus("theme: %s", m.String())
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
			if a.proc == nil {
				continue
			}
			a.mu.Lock()
			for addr, v := range a.frozen {
				_ = a.proc.Write(addr, v.Raw)
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
