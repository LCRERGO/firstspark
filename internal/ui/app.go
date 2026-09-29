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
	"github.com/LCRERGO/firstspark/pkg/celua"
	"github.com/LCRERGO/firstspark/pkg/cheattable"
	"github.com/LCRERGO/firstspark/pkg/config"
	"github.com/LCRERGO/firstspark/pkg/customtype"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/dissect"
	"github.com/LCRERGO/firstspark/pkg/hotkey"
	"github.com/LCRERGO/firstspark/pkg/inject"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/plugin"
	"github.com/LCRERGO/firstspark/pkg/scan"
	"github.com/LCRERGO/firstspark/pkg/speedhack"
)

// tableEntry is one row of the cheat table. Entries form a tree: a group holds
// children and has no address or value, and a child's address may be expressed
// relative to its parent.
type tableEntry struct {
	addr        uint64
	typ         scan.ValueType
	desc        string
	value       scan.Value
	orig        scan.Value
	frozen      bool
	frozenValue scan.Value
	undoValue   scan.Value
	hasUndo     bool
	pointer     *pointerChain
	bit         *bitSpec
	display     displayFormat
	unsigned    bool
	hotkey      fyne.KeyName
	group       bool
	expanded    bool
	children    []*tableEntry
	parent      *tableEntry
	depth       int
	expr        string
	exprOffsets []string
	script      string
	scriptExec  *autoasm.Executor
	scriptBE    debugger.Backend
	color       string
	ceHotkeys   []cheattable.CEHotkey
	extra       []cheattable.RawElement
}

// activePanel identifies which result list the focused shortcuts act on.
// The cheat table is the default, preserving Ctrl+E's original target.
type activePanel int

const (
	panelCheatTable activePanel = iota
	panelFound
)

// App is the root UI state.
type App struct {
	cfg  config.Config
	fapp fyne.App
	win  fyne.Window
	th   *cyberTheme

	procs          []mem.Process
	proc           *mem.Process
	symbols        map[string]uint64
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

	viewMenu        *fyne.Menu
	themeChoices    []themeChoice
	themeSystemItem *fyne.MenuItem

	status *widget.Label

	tabs       []*scanTab
	activeTab  int
	tabSeq     int
	tabsWidget *container.DocTabs

	entries         []*tableEntry
	entryRoots      []*tableEntry
	table           *cheatTable
	tableSel        int
	tableMulti      map[*tableEntry]bool
	clickMod        fyne.KeyModifier
	lastTap         time.Time
	lastTapRow      int
	lastTapCol      int
	activePanel     activePanel
	hotkeyShortcuts map[uint64]fyne.Shortcut

	speedhack    *ttwidget.Check
	speedScale   *toolTipEntry
	speedMgr     *speedhack.Manager
	speedApplied bool
	plugins      *plugin.Manager
	unrandom     *ttwidget.Check
	unrandomVal  *toolTipEntry
	unrandomHook []*inject.Hook
	unrandomOn   bool

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
	dbgRegs        *dbgList
	dbgHits        *dbgList
	dbgRegVals     []string
	dbgHitLabels   []string
	dbgAddrEntry   *toolTipEntry
	dbgStatus      *widget.Label
	dbgStop        chan struct{}
	dbgBreakpoints map[uint64]*dbgBreakpoint
	dbgWatchpoints map[uint64]int
	dbgWatchWrite  map[uint64]bool
	dbgBPList      *dbgList
	dbgBPLabels    []string
	dbgBPAddrs     []uint64
	dbgBPWatch     []bool
	dbgRegEdit     *toolTipEntry
	dbgTID         int
	dbgAttached    bool
	dbgThreads     []int
	dbgThreadList  *dbgList
	dbgModules     []mem.Region
	dbgModuleList  *dbgList
	dbgStack       []string
	dbgStackAddrs  []uint64
	dbgStackList   *dbgList
	dbgTrace       []string
	dbgTraceList   *dbgList

	asmWin     fyne.Window
	asmEditor  *codeEditor
	asmStatus  *widget.Label
	asmExec    *autoasm.Executor
	asmBackend debugger.Backend

	luaRT      *celua.Runtime
	luaWin     fyne.Window
	luaLines   []string
	luaList    *widget.List
	luaInput   *luaInput
	luaHistory []string
	luaHistPos int

	ctWin       fyne.Window
	ctList      *widget.List
	ctDefs      []customtype.Definition
	ctSel       int
	ctID        scan.ValueType
	ctName      *toolTipEntry
	ctMode      *ttwidget.Select
	ctSize      *toolTipEntry
	ctKind      *ttwidget.Select
	ctAlign     *toolTipEntry
	ctDesc      *toolTipEntry
	ctEditor    *codeEditor
	ctStatus    *widget.Label
	ctTestBytes *toolTipEntry
	ctTestAddr  *toolTipEntry
	ctTestOut   *widget.Label

	dissectWin       fyne.Window
	dissectTable     *widget.Table
	dissectFields    []dissect.Field
	dissectBases     []uint64
	dissectBase      uint64
	dissectSel       int
	dissectBaseEntry *toolTipEntry
	dissectSizeEntry *toolTipEntry
	dissectInstEntry *toolTipEntry
	dissectStatus    *widget.Label

	memWin       fyne.Window
	disasm       []asm.Instruction
	disasmList   *widget.List
	hexList      *memHexList
	memAddrEntry *toolTipEntry
	memType      *ttwidget.Select
	searchPat    []byte
	searchMask   []byte
	searchNext   uint64

	memCur          uint64
	memSelStart     uint64
	memSelEnd       uint64
	memSelActive    bool
	memNibbleHigh   bool
	memPending      byte
	memViewStart    uint64
	memViewLen      int
	memRegion       mem.Region
	memRegionRows   int
	memPageCache    map[uint64][]byte
	disasmBase      uint64
	regionSelect    *ttwidget.Select
	regionSelecting bool
	regionsWin      fyne.Window
	memRegions      []mem.Region
	regionsView     []mem.Region
	regionFilter    *toolTipEntry
	regionsList     *widget.List

	mu              sync.Mutex
	freezeTargets   map[uint64]scan.Value
	stop            chan struct{}
	err             error
	procWatchCancel context.CancelFunc

	autoMu      sync.Mutex
	autoPattern string
	autoRegex   bool

	hotkeys            *hotkey.Manager
	hkWin              fyne.Window
	hotkeyStatus       map[string]string
	hotkeyStatusLabels map[string]*widget.Label
	paused             bool
}

// Run opens the application window and blocks until it is closed.
func Run(cfg config.Config) error {
	// Fyne reads the interface scale from the environment at startup; there is
	// no runtime setter, so a scale change takes effect on the next launch.
	if cfg.UI.Scale > 0 && cfg.UI.Scale != 1 {
		os.Setenv("FYNE_SCALE", strconv.FormatFloat(cfg.UI.Scale, 'g', -1, 64))
	}
	a := &App{
		cfg:           cfg,
		freezeTargets: map[uint64]scan.Value{},
		stop:          make(chan struct{}),
		tableSel:      -1,
		procSortCol:   0,
		procSortAsc:   true,
		showIcons:     cfg.UI.ProcessIcons,
		expanded:      map[int]bool{},
		treeMode:      true,
	}
	a.icons = newIconResolver()
	a.fapp = app.NewWithID("com.firstspark.app")
	a.setAutoAttach(cfg.Process.AutoAttach, cfg.Process.AutoAttachRegex)
	a.th = newTheme(parseFamily(cfg.UI.Theme), parseVariant(cfg.UI.ThemeVariant), cfg.UI.FontSize)
	a.fapp.Settings().SetTheme(a.th)
	loadErr := func() error {
		_, err := customtype.LoadAndRegister(config.CustomTypesPath())
		return err
	}()
	a.plugins = plugin.NewManager(resolvePluginDir(cfg), plugin.API{
		PID:   func() int { return a.targetPID() },
		Read:  func(addr uint64, size int) ([]byte, error) { return a.pluginRead(addr, size) },
		Write: func(addr uint64, data []byte) error { return a.pluginWrite(addr, data) },
		Show:  func(s string) { a.pluginNotify(s) },
		Log:   func(s string) { log.Info("plugin", "msg", s) },
	})
	a.plugins.Load(cfg.Plugins.Enabled, true)
	a.build()
	if loadErr != nil {
		a.fail(loadErr)
	}
	a.fapp.Lifecycle().SetOnStopped(a.shutdown)
	a.setupHotkeys()
	a.refreshProcesses()
	go a.freezeLoop()
	go a.autoAttachLoop()
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
	a.processLabel = newTapLabel(i18n.T("app.no_process"), a.openProcessList)
	a.status = widget.NewLabel("")

	a.speedhack = newHintCheck(i18n.T("app.enable_speedhack"), "scan.hint.speedhack", func(on bool) { a.setSpeedhack(on) })
	a.speedhack.SetChecked(a.cfg.Speedhack.Enabled)
	a.speedScale = newHintEntry("scan.hint.speedhack_scale")
	a.speedScale.SetText(strconv.FormatFloat(a.cfg.Speedhack.Scale, 'g', -1, 64))
	a.unrandom = newHintCheck(i18n.T("scan.unrandomizer"), "scan.hint.unrandomizer", func(on bool) { a.setUnrandomizer(on) })
	a.unrandomVal = newHintEntry("scan.hint.unrandomizer_value")
	a.unrandomVal.SetText("0")

	a.buildCheatTable()
	a.buildTabs()
}

// buildTabs creates the document-tab container and its first scan tab.
func (a *App) buildTabs() {
	a.tabsWidget = container.NewDocTabs()
	a.tabsWidget.CreateTab = func() *container.TabItem { return a.createTab() }
	a.tabsWidget.CloseIntercept = func(item *container.TabItem) { a.closeScanTab(item) }
	a.tabsWidget.OnSelected = func(item *container.TabItem) { a.onTabSelected(item) }
	a.tabSeq = 1
	first := a.createTab()
	a.tabsWidget.Append(first)
	a.tabsWidget.SelectIndex(0)
}

// tab returns the active scan tab, or nil before the UI is built.
func (a *App) tab() *scanTab {
	if a.activeTab < 0 || a.activeTab >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.activeTab]
}

func (a *App) defaultValueType() scan.ValueType {
	if t, err := scan.ParseValueType(a.cfg.Scan.ValueType); err == nil {
		return t
	}
	return scan.TypeDword
}

func (a *App) content() fyne.CanvasObject {
	workspace := container.NewBorder(a.processStrip(), nil, nil, nil, a.tabsWidget)
	body := container.NewVSplit(workspace, a.cheatPanel())
	body.SetOffset(0.74)
	bar := container.NewBorder(nil, nil, a.processLabel, a.status)
	return container.NewBorder(a.toolbar(), bar, nil, nil, body)
}

// processStrip holds the process-wide toggles shared by every scan tab.
func (a *App) processStrip() fyne.CanvasObject {
	return container.NewHBox(
		a.speedhack,
		widget.NewLabel(i18n.T("scan.speedhack_scale")),
		container.NewGridWrap(fyne.NewSize(70, 34), a.speedScale),
		widget.NewSeparator(),
		a.unrandom,
		widget.NewLabel(i18n.T("scan.unrandomizer_value")),
		container.NewGridWrap(fyne.NewSize(90, 34), a.unrandomVal),
	)
}

// updateScanControls refreshes the active tab's scan controls.
func (a *App) updateScanControls() {
	if t := a.tab(); t != nil {
		t.updateScanControls()
		return
	}
	a.updateTableActions()
}

// updateScanTypeOptions swaps the active tab's scan-type list.
func (a *App) updateScanTypeOptions() {
	if t := a.tab(); t != nil {
		t.updateScanTypeOptions()
	}
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
// state, mirroring the reference tool's blocked buttons.
func (a *scanTab) updateScanControls() {
	mode := parseCEScanType(a.scanType.Selected)
	if a.scanning {
		a.setScanButtons(false, false, false, true)
		return
	}
	a.setScanButtons(a.proc != nil, a.session != nil, a.session != nil && a.session.CanUndo(), false)
	a.updateValueControls(mode)
	a.updateTableActions()
}

// setScanButtons enables the scan button set.
func (a *scanTab) setScanButtons(canScan, canNext, canUndo, canStop bool) {
	if a.scanBtn != nil {
		setEnabled(a.scanBtn, canScan)
	}
	if a.nextBtn != nil {
		setEnabled(a.nextBtn, canNext)
	}
	if a.undoBtn != nil {
		setEnabled(a.undoBtn, canUndo)
	}
	if a.stopBtn != nil {
		setEnabled(a.stopBtn, canStop)
	}
}

// updateValueControls enables and shows the value controls for a scan mode.
func (a *scanTab) updateValueControls(mode scan.ScanMode) {
	if a.valueEntry != nil {
		setEnabled(a.valueEntry, modeNeedsValue(mode))
		a.valueEntry.SetPlaceHolder(valuePlaceholder(mode))
	}
	between := mode == scan.ModeBetween
	if a.andLabel != nil {
		setVisible(a.andLabel, between)
	}
	if a.value2Entry != nil {
		setEnabled(a.value2Entry, between)
		setVisible(a.value2Entry, between)
	}
	if a.compareSelect != nil {
		setEnabled(a.compareSelect, mode == scan.ModeExact)
	}
}

// updateTableActions enables the toolbar actions that depend on app state.
func (a *App) updateTableActions() {
	hasTable := len(a.entryRoots) > 0
	setActionEnabled(a.saveAction, hasTable)
	setActionEnabled(a.saveAsAction, hasTable)
	setActionEnabled(a.memViewAction, a.proc != nil)
	setActionEnabled(a.addAddrAction, a.proc != nil)
	setActionEnabled(a.clearAction, hasTable)
}

func setVisible(o fyne.CanvasObject, visible bool) {
	if o == nil {
		return
	}
	if visible {
		o.Show()
	} else {
		o.Hide()
	}
	o.Refresh()
}

// updateScanTypeOptions swaps the Scan Type list between the first-scan and
// next-scan sets, keeping the current choice when it is still available.
func (a *scanTab) updateScanTypeOptions() {
	if a.scanType == nil {
		return
	}
	options := scanTypeLabelsFor(a.session != nil)
	current := a.scanType.Selected
	a.scanType.Options = options
	a.scanType.Refresh()
	for _, o := range options {
		if o == current {
			return
		}
	}
	a.scanType.SetSelected(scanTypeLabel(scan.ModeExact))
	a.updateScanControls()
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
	newTable := fyne.NewMenuItem(i18n.T("menu.file.new"), a.newTable)
	newTable.Shortcut = ctrl(fyne.KeyN)
	load := fyne.NewMenuItem(i18n.T("menu.file.load"), a.loadTable)
	load.Shortcut = ctrl(fyne.KeyO)
	save := fyne.NewMenuItem(i18n.T("menu.file.save"), a.saveTable)
	save.Shortcut = ctrl(fyne.KeyS)
	saveAs := fyne.NewMenuItem(i18n.T("menu.file.save_as"), a.saveTableAs)
	saveAs.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl | fyne.KeyModifierAlt}
	saveCE := fyne.NewMenuItem(i18n.T("menu.file.save_ce"), a.saveTableAsCE)
	saveRes := fyne.NewMenuItem(i18n.T("menu.file.save_scan_results"), a.saveScanResults)
	saveRes.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierAlt | fyne.KeyModifierShift}
	quit := fyne.NewMenuItem(i18n.T("menu.file.quit"), a.fapp.Quit)
	file := fyne.NewMenu(i18n.T("menu.file"),
		openProc,
		newTable,
		fyne.NewMenuItemSeparator(),
		load, save, saveAs, saveCE,
		fyne.NewMenuItemSeparator(),
		saveRes,
		fyne.NewMenuItemSeparator(),
		quit,
	)

	settings := fyne.NewMenuItem(i18n.T("menu.edit.settings"), a.showSettings)
	edit := fyne.NewMenu(i18n.T("menu.edit"), settings)

	var themeItems []*fyne.MenuItem
	for _, fam := range families {
		fam := fam
		var variantItems []*fyne.MenuItem
		for _, vr := range []variant{variantLight, variantDark} {
			vr := vr
			item := fyne.NewMenuItem(variantLabel(vr), func() { a.setTheme(fam, vr) })
			a.themeChoices = append(a.themeChoices, themeChoice{fam: fam, variant: vr, item: item})
			variantItems = append(variantItems, item)
		}
		famItem := fyne.NewMenuItem(familyLabel(fam), nil)
		famItem.ChildMenu = fyne.NewMenu("", variantItems...)
		themeItems = append(themeItems, famItem)
	}
	// System is a single option that keeps the current family and follows the
	// desktop light/dark preference.
	a.themeSystemItem = fyne.NewMenuItem(variantLabel(variantSystem),
		func() { a.setTheme(parseFamily(a.cfg.UI.Theme), variantSystem) })
	themeItems = append(themeItems, fyne.NewMenuItemSeparator(), a.themeSystemItem)
	themeItem := fyne.NewMenuItem(i18n.T("menu.view.theme"), nil)
	themeItem.ChildMenu = fyne.NewMenu("", themeItems...)
	a.viewMenu = fyne.NewMenu(i18n.T("menu.view"), themeItem)
	a.updateThemeChecks()

	addAddr := fyne.NewMenuItem(i18n.T("menu.table.add_address"), a.addAddressDialog)
	newGroupItem := fyne.NewMenuItem(i18n.T("menu.table.new_group"), a.newGroup)
	clear := fyne.NewMenuItem(i18n.T("menu.table.clear"), a.clearTable)
	custom := fyne.NewMenuItem(i18n.T("menu.table.custom_types"), a.showCustomTypes)
	pointer := fyne.NewMenuItem(i18n.T("menu.table.pointer_scan"), a.showPointerScan)
	loadPointer := fyne.NewMenuItem(i18n.T("menu.table.load_pointer_scan"), a.loadPointerScan)
	table := fyne.NewMenu(i18n.T("menu.table"), addAddr, newGroupItem, clear, fyne.NewMenuItemSeparator(), pointer, loadPointer, custom)

	speed := fyne.NewMenuItem(i18n.T("menu.tools.speedhack"), a.toggleSpeedhack)
	hotkeysItem := fyne.NewMenuItem(i18n.T("menu.tools.hotkeys"), a.showHotkeys)
	debuggerItem := fyne.NewMenuItem(i18n.T("menu.tools.debugger"), a.openDebugger)
	dissectItem := fyne.NewMenuItem(i18n.T("menu.tools.dissect"), a.openDissect)
	dissectItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyD, Modifier: fyne.KeyModifierControl | fyne.KeyModifierAlt}
	autoasmItem := fyne.NewMenuItem(i18n.T("menu.tools.auto_assemble"), a.openAutoAssemble)
	autoasmItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyA, Modifier: fyne.KeyModifierControl | fyne.KeyModifierAlt}
	luaItem := fyne.NewMenuItem(i18n.T("menu.tools.lua"), a.openLuaConsole)
	managePluginsItem := fyne.NewMenuItem(i18n.T("menu.tools.plugins"), a.showPlugins)
	toolsItems := []*fyne.MenuItem{debuggerItem, dissectItem, autoasmItem, luaItem, speed, hotkeysItem,
		fyne.NewMenuItemSeparator(), managePluginsItem}
	if contribs := a.pluginMenuContribs(); len(contribs) > 0 {
		var actions []*fyne.MenuItem
		for _, c := range contribs {
			c := c
			actions = append(actions, fyne.NewMenuItem(c.Item.Label, func() { a.runPluginAction(c.PluginID, c.Item.Action) }))
		}
		sub := fyne.NewMenuItem(i18n.T("menu.tools.plugin_actions"), nil)
		sub.ChildMenu = fyne.NewMenu("", actions...)
		toolsItems = append(toolsItems, sub)
	}
	tools := fyne.NewMenu(i18n.T("menu.tools"), toolsItems...)

	about := fyne.NewMenuItem(i18n.T("menu.help.about"), a.showAbout)
	help := fyne.NewMenu(i18n.T("menu.help"), about)

	newTab := fyne.NewMenuItem(i18n.T("menu.scan.new_tab"), a.addScanTab)
	newTab.Shortcut = ctrl(fyne.KeyT)
	closeTab := fyne.NewMenuItem(i18n.T("menu.scan.close_tab"), func() {
		if o := a.tabsWidget.Selected(); o != nil {
			a.closeScanTab(o)
		}
	})
	closeTab.Shortcut = ctrl(fyne.KeyW)
	renameTab := fyne.NewMenuItem(i18n.T("menu.scan.rename_tab"), a.renameScanTab)
	nextTab := fyne.NewMenuItem(i18n.T("menu.scan.next_tab"), func() { a.cycleTab(1) })
	nextTab.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyTab, Modifier: fyne.KeyModifierControl}
	prevTab := fyne.NewMenuItem(i18n.T("menu.scan.previous_tab"), func() { a.cycleTab(-1) })
	prevTab.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyTab, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}
	compare := fyne.NewMenuItem(i18n.T("menu.scan.compare_tabs"), a.compareTabs)
	scanMenu := fyne.NewMenu(i18n.T("menu.scan"),
		newTab, closeTab, renameTab,
		fyne.NewMenuItemSeparator(),
		nextTab, prevTab,
		fyne.NewMenuItemSeparator(),
		compare,
	)

	return fyne.NewMainMenu(file, edit, a.viewMenu, scanMenu, table, tools, help)
}

// themeChoice ties a View menu item to a family and variant.
type themeChoice struct {
	fam     family
	variant variant
	item    *fyne.MenuItem
}

// setTheme applies and persists a palette family and variant, keeping the View
// menu in sync.
func (a *App) setTheme(fam family, vr variant) {
	a.cfg.UI.Theme = string(fam)
	a.cfg.UI.ThemeVariant = vr.String()
	a.applyTheme()
	a.updateThemeChecks()
	a.saveConfig()
	a.setStatusText(i18n.Tf("status.theme", map[string]any{
		"Theme": familyLabel(fam) + " " + variantLabel(vr),
	}))
}

// updateThemeChecks marks the active family and variant in the View > Theme
// menu.
func (a *App) updateThemeChecks() {
	activeFam := parseFamily(a.cfg.UI.Theme)
	activeVariant := parseVariant(a.cfg.UI.ThemeVariant)
	system := activeVariant == variantSystem
	for _, c := range a.themeChoices {
		c.item.Checked = !system && c.fam == activeFam && c.variant == activeVariant
	}
	if a.themeSystemItem != nil {
		a.themeSystemItem.Checked = system
	}
	if a.viewMenu != nil {
		a.viewMenu.Refresh()
	}
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

// syncFreezeTargets rebuilds the writer goroutine's view of the frozen
// entries. It must run on the UI goroutine because it reads the entry tree.
func (a *App) syncFreezeTargets() {
	targets := make(map[uint64]scan.Value)
	a.walkEntries(func(e *tableEntry) {
		if e.frozen && !e.group && e.expr == "" {
			targets[e.addr] = e.frozenValue
		}
	})
	a.mu.Lock()
	a.freezeTargets = targets
	a.mu.Unlock()
}

// shutdown releases everything bound to the target and stops the background
// loops when the application quits.
func (a *App) shutdown() {
	a.removeSpeedhack()
	a.speedApplied = false
	a.stopProcessWatch()
	if a.stop != nil {
		close(a.stop)
		a.stop = nil
	}
	if a.dbgSession != nil {
		if err := a.dbgSession.Detach(); err != nil {
			log.Debug("detach on shutdown failed", "err", err)
		}
		_ = a.dbgSession.Close()
		a.dbgSession = nil
	}
	if a.asmBackend != nil {
		_ = a.asmBackend.Close()
		a.asmBackend = nil
	}
	if a.hotkeys != nil {
		a.hotkeys.Close()
		a.hotkeys = nil
	}
	if a.plugins != nil {
		a.plugins.Close()
		a.plugins = nil
	}
	log.Info("firstspark stopped")
}

func (a *App) freezeLoop() {
	const freezeTick = 50 * time.Millisecond
	every := a.refreshTickEvery(int(freezeTick / time.Millisecond))
	ticker := time.NewTicker(freezeTick)
	defer ticker.Stop()
	tick := 0
	var lastWriteErr time.Time
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
			a.writeFrozen(&lastWriteErr)
			tick++
			if tick%every == 0 {
				fyne.Do(a.refreshUI)
			}
		}
	}
}

// refreshTickEvery converts the configured refresh interval into a number of
// freeze ticks, clamped to at least one and defaulting to 500 ms.
func (a *App) refreshTickEvery(tickMS int) int {
	if tickMS < 1 {
		tickMS = 50
	}
	refresh := a.cfg.UI.RefreshMS
	if refresh < tickMS {
		refresh = 500
	}
	every := refresh / tickMS
	if every < 1 {
		every = 1
	}
	return every
}

// writeFrozen rewrites every frozen target under the state lock.
func (a *App) writeFrozen(lastWriteErr *time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	proc := a.proc
	if proc == nil {
		return
	}
	for addr, v := range a.freezeTargets {
		if err := proc.Write(addr, v.Raw); err != nil {
			if time.Since(*lastWriteErr) > 5*time.Second {
				log.Warn("freeze write failed", "pid", proc.PID, "addr", fmt.Sprintf("0x%x", addr), "err", err)
				*lastWriteErr = time.Now()
			}
		}
	}
}

// refreshUI re-reads the cheat table and the active tab's Found values on the
// UI goroutine.
func (a *App) refreshUI() {
	changed := a.refreshEntries()
	if t := a.tab(); t != nil {
		t.refreshFoundValues()
		t.refreshFound()
	}
	a.syncFreezeTargets()
	a.runLuaTimers()
	if a.plugins != nil {
		a.plugins.Tick(int64(a.cfg.UI.RefreshMS))
	}
	if changed && a.table != nil {
		a.table.Refresh()
	}
}

// tapLabel is a label that runs a callback when tapped.
type tapLabel struct {
	*widget.Label
	ttwidget.ToolTipWidgetExtend
	onTap func()
}

func newTapLabel(text string, onTap func()) *tapLabel {
	t := &tapLabel{Label: widget.NewLabel(text), onTap: onTap}
	t.ExtendBaseWidget(t)
	t.SetToolTip(i18n.T("app.hint.process_label"))
	return t
}

func (t *tapLabel) ExtendBaseWidget(wid fyne.Widget) {
	t.ExtendToolTipWidget(wid)
	t.Label.ExtendBaseWidget(wid)
}

func (t *tapLabel) MouseIn(e *desktop.MouseEvent)    { t.ToolTipWidgetExtend.MouseIn(e) }
func (t *tapLabel) MouseMoved(e *desktop.MouseEvent) { t.ToolTipWidgetExtend.MouseMoved(e) }
func (t *tapLabel) MouseOut()                        { t.ToolTipWidgetExtend.MouseOut() }

func (t *tapLabel) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

// monoText creates a monospaced canvas text in the given colour.
func (a *App) monoText(text string) *canvas.Text {
	return a.th.monoText(text, a.pal().text)
}
