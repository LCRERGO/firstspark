//go:build gui

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

const (
	// memPageSize is the read/cache granularity of the memory viewer.
	memPageSize = 0x1000
	// memFallbackWindow is the virtual range shown for an unmapped address.
	memFallbackWindow = 0x10000
	// memDisasmChunk is how many bytes are disassembled around the cursor.
	memDisasmChunk = 0x800
	// memSearchChunk bounds a single Find pass.
	memSearchChunk = 1 << 20
)

// memTypeOption pairs a memory-viewer display width with its translation key.
type memTypeOption struct {
	key string
	typ scan.ValueType
}

var memTypeOptions = []memTypeOption{
	{"memory.type.byte", scan.TypeByte},
	{"memory.type.2bytes", scan.TypeWord},
	{"memory.type.4bytes", scan.TypeDword},
	{"memory.type.8bytes", scan.TypeQword},
	{"memory.type.float", scan.TypeFloat},
	{"memory.type.double", scan.TypeDouble},
}

func memTypeLabels() []string {
	out := make([]string, len(memTypeOptions))
	for i, o := range memTypeOptions {
		out[i] = i18n.T(o.key)
	}
	return out
}

func memTypeLabel(t scan.ValueType) string {
	for _, o := range memTypeOptions {
		if o.typ == t {
			return i18n.T(o.key)
		}
	}
	return i18n.T("memory.type.4bytes")
}

func parseMemType(label string) scan.ValueType {
	for _, o := range memTypeOptions {
		if i18n.T(o.key) == label {
			return o.typ
		}
	}
	return scan.TypeDword
}

// openMemoryViewer shows the separate Memory Viewer window, creating it lazily.
func (a *App) openMemoryViewer() {
	if a.memWin == nil {
		a.memWin = a.fapp.NewWindow(i18n.T("memory.title"))
		a.memWin.Resize(fyne.NewSize(801, 530))
		a.buildMemoryViewer()
	}
	a.refreshMemRegions()
	a.memWin.Show()
}

func (a *App) buildMemoryViewer() {
	a.memAddrEntry = newHintEntry("memory.hint.address")
	a.memAddrEntry.SetText("0x0")
	a.memAddrEntry.OnSubmitted = func(string) { a.goToAddress() }

	a.memType = newHintSelect(memTypeLabels(), "memory.hint.display", func(string) { a.reloadMemory() })
	a.memType.SetSelected(memTypeLabel(scan.TypeDword))
	a.regionSelect = newHintSelect(a.regionLabels(), "memory.hint.region", func(string) { a.jumpToRegion() })

	bar := container.NewHBox(
		widget.NewLabel(i18n.T("memory.address")), a.memAddrEntry,
		newHintButton(i18n.T("memory.go"), "memory.hint.go", a.goToAddress),
		widget.NewSeparator(),
		widget.NewLabel(i18n.T("memory.display")), a.memType,
		newHintButton(i18n.T("memory.find"), "memory.hint.find", a.findDialog),
		newHintButton(i18n.T("memory.change_value"), "memory.hint.change_value", a.changeMemoryValue),
		widget.NewSeparator(),
		widget.NewLabel(i18n.T("memory.region")), a.regionSelect,
		newHintButton(i18n.T("memory.regions"), "memory.hint.regions", a.openMemoryRegions),
	)

	a.disasmList = widget.NewList(
		func() int { return len(a.disasm) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) { a.updateDisasmRow(id, o) },
	)
	a.disasmList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(a.disasm) {
			a.setCursor(a.disasm[id].Addr)
		}
	}
	a.hexList = widget.NewList(
		func() int { return a.memRegionRows },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) { a.updateHexRow(id, o) },
	)
	a.hexList.OnSelected = func(id widget.ListItemID) {
		a.setCursor(a.memRegion.Start + uint64(id)*16)
		a.setDisasmAt(a.memCur)
	}

	split := container.NewVSplit(a.disasmList, a.hexList)
	split.SetOffset(0.69)
	a.memWin.SetContent(fynetooltip.AddWindowToolTipLayer(container.NewBorder(bar, nil, nil, nil, split), a.memWin.Canvas()))
	viewItems := make([]*fyne.MenuItem, len(memTypeOptions))
	for i, o := range memTypeOptions {
		opt := o
		viewItems[i] = fyne.NewMenuItem(i18n.T(opt.key), func() { a.memType.SetSelected(i18n.T(opt.key)) })
	}
	viewItems = append(viewItems,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("menu.view.memory_regions"), a.openMemoryRegions),
	)
	a.memWin.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu(i18n.T("menu.file"), fyne.NewMenuItem(i18n.T("menu.file.close"), func() { a.memWin.Hide() })),
		fyne.NewMenu(i18n.T("menu.search"),
			fyne.NewMenuItem(i18n.T("menu.search.find"), a.findDialog),
			fyne.NewMenuItem(i18n.T("menu.search.find_next"), a.findNext),
		),
		fyne.NewMenu(i18n.T("menu.view"), viewItems...),
	))
	a.installMemoryShortcuts()
}

func (a *App) installMemoryShortcuts() {
	c := a.memWin.Canvas()
	c.AddShortcut(ctrl(fyne.KeyG), func(fyne.Shortcut) { c.Focus(a.memAddrEntry) })
	c.AddShortcut(ctrl(fyne.KeyF), func(fyne.Shortcut) { a.findDialog() })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF3}, func(fyne.Shortcut) { a.findNext() })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageUp}, func(fyne.Shortcut) { a.memPage(-1) })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageDown}, func(fyne.Shortcut) { a.memPage(1) })
	// Cheat Engine selects the display width with Ctrl+1..0.
	keys := []fyne.KeyName{fyne.Key1, fyne.Key2, fyne.Key3, fyne.Key4, fyne.Key5, fyne.Key6}
	for i, k := range keys {
		if i >= len(memTypeOptions) {
			break
		}
		label := i18n.T(memTypeOptions[i].key)
		c.AddShortcut(ctrl(k), func(fyne.Shortcut) { a.memType.SetSelected(label) })
	}
}

func (a *App) goToAddress() {
	addr, err := parseAddress(a.memAddrEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	a.loadMemory(addr)
}

// loadMemory selects the region containing addr and anchors the viewer there.
func (a *App) loadMemory(addr uint64) {
	if a.proc == nil {
		a.setStatusText(i18n.T("error.no_process"))
		return
	}
	a.memRegion = a.regionFor(addr)
	a.memRegionRows = int((a.memRegion.End - a.memRegion.Start) / 16)
	if a.memRegionRows < 1 {
		a.memRegionRows = 1
	}
	a.memPageCache = map[uint64][]byte{}
	a.refreshRegionSelect()
	a.setCursor(addr)
	if a.hexList != nil {
		a.hexList.Refresh()
		a.scrollHexTo(addr)
	}
	a.setDisasmAt(addr)
}

// regionFor returns the mapped region containing addr, or a synthetic window,
// and refreshes the cached region list.
func (a *App) regionFor(addr uint64) mem.Region {
	if a.proc != nil {
		if regions, err := mem.Regions(a.proc.PID); err == nil {
			a.memRegions = regions
			if r, ok := mem.RegionFor(regions, addr); ok && r.End > r.Start {
				return r
			}
		}
	}
	base := addr &^ (memFallbackWindow - 1)
	return mem.Region{Start: base, End: base + memFallbackWindow}
}

// setCursor records the current address and updates the address field.
func (a *App) setCursor(addr uint64) {
	a.memCur = addr
	if a.memAddrEntry != nil {
		a.memAddrEntry.SetText(fmt.Sprintf("0x%x", addr))
	}
}

func (a *App) reloadMemory() {
	if a.proc == nil {
		return
	}
	a.memPageCache = map[uint64][]byte{}
	if a.hexList != nil {
		a.hexList.Refresh()
	}
}

// memPage moves the cursor by one viewer page.
func (a *App) memPage(dir int) {
	const step = 0x100
	if dir < 0 {
		if a.memCur < step {
			return
		}
		a.loadMemory(a.memCur - step)
		return
	}
	a.loadMemory(a.memCur + step)
}

func (a *App) scrollHexTo(addr uint64) {
	if a.hexList == nil || addr < a.memRegion.Start {
		return
	}
	row := int((addr - a.memRegion.Start) / 16)
	if row < 0 {
		row = 0
	}
	a.hexList.ScrollTo(row)
}

// readCached reads n bytes at addr, caching whole pages.
func (a *App) readCached(addr uint64, n int) []byte {
	if a.proc == nil || n <= 0 {
		return nil
	}
	out := make([]byte, n)
	got := 0
	for got < n {
		cur := addr + uint64(got)
		page := cur &^ (memPageSize - 1)
		data, ok := a.memPageCache[page]
		if !ok {
			raw, err := a.proc.Read(page, memPageSize)
			if err != nil || len(raw) == 0 {
				break
			}
			data = raw
			if a.memPageCache == nil {
				a.memPageCache = map[uint64][]byte{}
			}
			if len(a.memPageCache) > 256 {
				a.memPageCache = map[uint64][]byte{}
			}
			a.memPageCache[page] = data
		}
		off := int(cur - page)
		if off >= len(data) {
			break
		}
		got += copy(out[got:], data[off:])
	}
	return out[:got]
}

func (a *App) updateHexRow(id widget.ListItemID, o fyne.CanvasObject) {
	t := o.(*canvas.Text)
	if id < 0 || id >= a.memRegionRows {
		t.Text = ""
		t.Refresh()
		return
	}
	addr := a.memRegion.Start + uint64(id)*16
	t.Text = a.formatHexRow(addr, a.readCached(addr, 16))
	t.Color = a.pal().text
	t.Refresh()
}

// formatHexRow renders one 16-byte row: address, hex bytes, ASCII and a decoded
// value column.
func (a *App) formatHexRow(addr uint64, data []byte) string {
	typ := scan.TypeDword
	if a.memType != nil {
		typ = parseMemType(a.memType.Selected)
	}
	w := typ.Size()
	if w <= 0 {
		w = 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%016x  ", addr)
	for j := 0; j < 16; j++ {
		if j < len(data) {
			fmt.Fprintf(&b, "%02x ", data[j])
		} else {
			b.WriteString("   ")
		}
	}
	b.WriteByte(' ')
	for j := 0; j < len(data) && j < 16; j++ {
		c := data[j]
		if c >= 0x20 && c < 0x7f {
			b.WriteByte(c)
		} else {
			b.WriteByte('.')
		}
	}
	if w <= len(data) {
		v := scan.NewValue(typ, data[:w])
		fmt.Fprintf(&b, "  = %s", v.String())
	}
	return b.String()
}

func (a *App) updateDisasmRow(id widget.ListItemID, o fyne.CanvasObject) {
	t := o.(*canvas.Text)
	if id < 0 || id >= len(a.disasm) {
		t.Text = ""
		t.Refresh()
		return
	}
	ins := a.disasm[id]
	t.Text = fmt.Sprintf("%016x  %-24x  %s", ins.Addr, ins.Bytes, ins.Text)
	t.Color = a.pal().text
	t.Refresh()
}

// setDisasmAt disassembles a chunk around addr and scrolls to it.
func (a *App) setDisasmAt(addr uint64) {
	start := addr
	if addr >= 0x40 {
		start = addr - 0x40
	}
	data := a.readCached(start, memDisasmChunk)
	a.disasmBase = start
	a.disasm = asm.Disassemble(data, start)
	if a.disasmList != nil {
		a.disasmList.Refresh()
		a.scrollDisasmTo(addr)
	}
}

func (a *App) scrollDisasmTo(addr uint64) {
	if a.disasmList == nil {
		return
	}
	for i, ins := range a.disasm {
		if addr >= ins.Addr && addr < ins.Addr+uint64(ins.Len) {
			a.disasmList.ScrollTo(i)
			return
		}
	}
}

// regionLabels returns the labels of the cached regions for the dropdown.
func (a *App) regionLabels() []string {
	out := make([]string, 0, len(a.memRegions))
	for _, r := range a.memRegions {
		out = append(out, regionLabel(r))
	}
	return out
}

func regionLabel(r mem.Region) string {
	name := r.Path
	if name == "" {
		name = "[anon]"
	}
	return fmt.Sprintf("0x%x-%0x %s %s", r.Start, r.End, r.Perms, name)
}

func (a *App) refreshRegionSelect() {
	if a.regionSelect == nil {
		return
	}
	labels := a.regionLabels()
	if len(labels) == 0 {
		return
	}
	a.regionSelect.Options = labels
	a.regionSelect.Refresh()
	current := regionLabel(a.memRegion)
	for i, l := range labels {
		if l == current {
			a.regionSelecting = true
			a.regionSelect.SetSelectedIndex(i)
			a.regionSelecting = false
			return
		}
	}
}

// jumpToRegion loads the region selected in the toolbar dropdown.
func (a *App) jumpToRegion() {
	if a.regionSelecting || a.regionSelect == nil || a.regionSelect.Selected == "" {
		return
	}
	for _, r := range a.memRegions {
		if regionLabel(r) == a.regionSelect.Selected {
			a.loadMemory(r.Start)
			return
		}
	}
}

// openMemoryRegions shows the Memory Regions browser.
func (a *App) openMemoryRegions() {
	if a.proc == nil {
		a.setStatusText(i18n.T("error.no_process"))
		return
	}
	if a.regionsWin == nil {
		a.regionsWin = a.fapp.NewWindow(i18n.T("regions.memory_title"))
		a.regionsWin.Resize(fyne.NewSize(760, 480))
		a.buildRegionsWindow()
	}
	a.refreshMemRegions()
	a.regionsWin.Show()
}

func (a *App) buildRegionsWindow() {
	a.regionFilter = newHintEntry("regions.hint.filter")
	a.regionFilter.SetPlaceHolder(i18n.T("regions.filter_placeholder"))
	a.regionFilter.OnChanged = func(string) { a.applyRegionFilter() }
	a.regionsList = widget.NewList(
		func() int { return len(a.regionsView) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.regionsView) {
				t.Text = ""
				t.Refresh()
				return
			}
			r := a.regionsView[id]
			name := r.Path
			if name == "" {
				name = "[anon]"
			}
			t.Text = fmt.Sprintf("%016x-%016x %-4s %10s  %s", r.Start, r.End, r.Perms, humanBytes(r.Size()), name)
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.regionsList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(a.regionsView) {
			a.openMemoryViewer()
			a.loadMemory(a.regionsView[id].Start)
		}
	}
	top := container.NewBorder(nil, nil, widget.NewLabel(i18n.T("regions.filter_label")), nil, a.regionFilter)
	a.regionsWin.SetContent(fynetooltip.AddWindowToolTipLayer(container.NewBorder(top, nil, nil, nil, a.regionsList), a.regionsWin.Canvas()))
	a.regionsWin.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu(i18n.T("menu.file"), fyne.NewMenuItem(i18n.T("menu.file.close"), func() { a.regionsWin.Hide() })),
	))
}

// refreshMemRegions reloads /proc/<pid>/maps for the viewer and browser.
func (a *App) refreshMemRegions() {
	if a.proc == nil {
		a.memRegions = nil
	} else if regions, err := mem.Regions(a.proc.PID); err == nil {
		a.memRegions = regions
	}
	a.applyRegionFilter()
	a.refreshRegionSelect()
}

// applyRegionFilter recomputes the filtered region view.
func (a *App) applyRegionFilter() {
	q := ""
	if a.regionFilter != nil {
		q = strings.ToLower(strings.TrimSpace(a.regionFilter.Text))
	}
	if q == "" {
		a.regionsView = a.memRegions
	} else {
		a.regionsView = make([]mem.Region, 0, len(a.memRegions))
		for _, r := range a.memRegions {
			if strings.Contains(strings.ToLower(r.Path), q) || strings.Contains(strings.ToLower(r.Perms), q) {
				a.regionsView = append(a.regionsView, r)
			}
		}
	}
	if a.regionsList != nil {
		a.regionsList.Refresh()
	}
}

func (a *App) findDialog() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(i18n.T("placeholder.aob_pattern"))
	d := dialog.NewForm(i18n.T("dialog.find.title"), i18n.T("action.find"), i18n.T("action.cancel"),
		[]*widget.FormItem{widget.NewFormItem(i18n.T("field.aob_pattern"), entry)},
		func(ok bool) {
			if !ok {
				return
			}
			p, err := scan.ParseAOB(entry.Text)
			if err != nil {
				a.fail(err)
				return
			}
			a.searchPat = p.Bytes
			a.searchMask = p.Mask
			a.searchNext = a.memCur
			a.findNext()
		}, a.memWin)
	d.Resize(fyne.NewSize(420, 180))
	d.Show()
}

// findNext searches the current region from searchNext for the pattern.
func (a *App) findNext() {
	if len(a.searchPat) == 0 {
		a.setStatusText(i18n.T("status.no_search_pattern"))
		return
	}
	pos := a.searchNext
	if pos < a.memRegion.Start || pos >= a.memRegion.End {
		pos = a.memRegion.Start
	}
	for pos < a.memRegion.End {
		n := int(min(uint64(memSearchChunk), a.memRegion.End-pos))
		data := a.readCached(pos, n)
		if len(data) == 0 {
			break
		}
		for i := 0; i+len(a.searchPat) <= len(data); i++ {
			if matchAt(data, i, a.searchPat, a.searchMask) {
				addr := pos + uint64(i)
				a.searchNext = addr + 1
				a.loadMemory(addr)
				a.setStatusText(i18n.Tf("status.found_at", map[string]any{"Addr": fmt.Sprintf("%x", addr)}))
				return
			}
		}
		pos += uint64(len(data))
	}
	a.searchNext = pos
	a.setStatusText(i18n.T("status.pattern_not_found"))
}

func matchAt(data []byte, off int, pat, mask []byte) bool {
	for k := range pat {
		if len(mask) > k && mask[k] == 0 {
			continue
		}
		if data[off+k] != pat[k] {
			return false
		}
	}
	return true
}

// memValueOption is one entry of the Change value form's type list.
type memValueOption struct {
	label   string
	typ     scan.ValueType
	integer bool
	text    bool
}

func memValueOptions() []memValueOption {
	return []memValueOption{
		{i18n.T("memory.type.byte"), scan.TypeByte, true, false},
		{i18n.T("memory.type.2bytes"), scan.TypeWord, true, false},
		{i18n.T("memory.type.4bytes"), scan.TypeDword, true, false},
		{i18n.T("memory.type.8bytes"), scan.TypeQword, true, false},
		{i18n.T("memory.type.float"), scan.TypeFloat, false, false},
		{i18n.T("memory.type.double"), scan.TypeDouble, false, false},
		{i18n.T("memory.type.text"), scan.TypeString, false, true},
		{i18n.T("memory.type.aob"), scan.TypeAOB, false, false},
	}
}

// readMemoryValue reads a value of the given type at addr. Variable-width
// types use a default window (64 bytes for text, 8 for AOB).
func (a *App) readMemoryValue(addr uint64, typ scan.ValueType) (scan.Value, error) {
	if a.proc == nil {
		return scan.Value{}, fmt.Errorf("%s", i18n.T("error.no_process"))
	}
	size := typ.Size()
	if size <= 0 {
		if typ == scan.TypeAOB {
			size = 8
		} else {
			size = 64
		}
	}
	raw, err := a.proc.Read(addr, size)
	if err != nil {
		return scan.Value{}, err
	}
	return scan.NewValue(typ, raw), nil
}

// changeMemoryValue opens Cheat Engine's Memory Viewer change-value form: a
// value field, a type selector and a Hexadecimal/Unicode checkbox that depends
// on the type. It reads the address fresh when the type changes and writes on
// OK (ADR 0037 value-changer phase 2).
func (a *App) changeMemoryValue() {
	if a.proc == nil {
		a.setStatusText(i18n.T("error.no_process"))
		return
	}
	addr, err := parseAddress(a.memAddrEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	opts := memValueOptions()
	labels := make([]string, len(opts))
	for i, o := range opts {
		labels[i] = o.label
	}
	valueEntry := widget.NewEntry()
	typeSel := widget.NewSelect(labels, nil)
	typeSel.SetSelected(labels[2])
	check := widget.NewCheck(i18n.T("memory.hexadecimal"), nil)

	selected := func() memValueOption {
		for _, o := range opts {
			if o.label == typeSel.Selected {
				return o
			}
		}
		return opts[2]
	}
	effectiveType := func() scan.ValueType {
		o := selected()
		if o.text && check.Checked {
			return scan.TypeUTF16LE
		}
		return o.typ
	}
	refresh := func() {
		o := selected()
		switch {
		case o.integer:
			check.SetText(i18n.T("memory.hexadecimal"))
			check.Enable()
		case o.text:
			check.SetText(i18n.T("memory.unicode"))
			check.Enable()
		default:
			check.SetText(i18n.T("memory.hexadecimal"))
			check.Disable()
		}
		v, rerr := a.readMemoryValue(addr, effectiveType())
		if rerr != nil {
			valueEntry.SetText("")
			return
		}
		if o.integer && check.Checked {
			valueEntry.SetText(hexOf(v))
		} else {
			valueEntry.SetText(v.String())
		}
	}
	typeSel.OnChanged = func(string) { refresh() }
	check.OnChanged = func(bool) { refresh() }
	refresh()

	form := widget.NewForm(
		widget.NewFormItem(i18n.T("field.value"), valueEntry),
		widget.NewFormItem(i18n.T("field.type"), typeSel),
		widget.NewFormItem("", check),
	)
	d := dialog.NewCustomConfirm(i18n.T("dialog.change_value.title"), i18n.T("action.apply"), i18n.T("action.cancel"),
		form, func(ok bool) {
			if !ok || a.proc == nil {
				return
			}
			v, perr := scan.ParseValue(effectiveType(), valueEntry.Text)
			if perr != nil {
				a.fail(perr)
				return
			}
			if werr := a.proc.Write(addr, v.Raw); werr != nil {
				a.fail(werr)
				return
			}
			a.loadMemory(addr)
			a.setStatusText(i18n.Tf("status.memory_changed", map[string]any{"Addr": fmt.Sprintf("%x", addr)}))
		}, a.memWin)
	d.Resize(fyne.NewSize(440, 280))
	d.Show()
}
