//go:build gui

package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/pkg/dissect"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// openDissect shows the structure dissect window, creating it lazily.
func (a *App) openDissect() {
	if a.proc == nil {
		a.fail(fmt.Errorf("no process selected"))
		return
	}
	if a.dissectWin == nil {
		a.dissectWin = a.fapp.NewWindow("Dissect Data/Structures")
		a.dissectWin.Resize(fyne.NewSize(720, 560))
		a.buildDissect()
	}
	a.dissectWin.Show()
}

func (a *App) buildDissect() {
	a.dissectStatus = widget.NewLabel("no data")
	a.dissectBaseEntry = newHintEntry("dissect.hint.base")
	a.dissectBaseEntry.SetPlaceHolder("0x1234")
	switch {
	case a.tableSel >= 0 && a.tableSel < len(a.entries) &&
		!a.entries[a.tableSel].group && a.entries[a.tableSel].expr == "":
		a.dissectBaseEntry.SetText(fmt.Sprintf("0x%x", a.entries[a.tableSel].addr))
	case a.tab() != nil && len(a.tab().results) > 0:
		a.dissectBaseEntry.SetText(fmt.Sprintf("0x%x", a.tab().results[0].Addr))
	}
	a.dissectSizeEntry = newHintEntry("dissect.hint.size")
	a.dissectSizeEntry.SetText("128")
	a.dissectInstEntry = newHintEntry("dissect.hint.instance")
	a.dissectInstEntry.SetPlaceHolder("instance address")
	a.dissectSel = -1

	a.dissectTable = widget.NewTable(
		func() (int, int) { return len(a.dissectFields), 2 + len(a.dissectBases) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.TableCellID, o fyne.CanvasObject) { a.updateDissectCell(id, o) },
	)
	a.dissectTable.ShowHeaderRow = true
	a.dissectTable.CreateHeader = func() fyne.CanvasObject { return a.monoText("") }
	a.dissectTable.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		t := o.(*canvas.Text)
		t.Text = a.dissectHeader(id.Col)
		t.Color = a.pal().primary
		t.Refresh()
	}
	a.dissectTable.SetColumnWidth(0, 90)
	a.dissectTable.SetColumnWidth(1, 90)
	a.dissectTable.OnSelected = func(id widget.TableCellID) { a.dissectSel = id.Row }

	bar := container.NewHBox(
		widget.NewLabel("Base"), a.dissectBaseEntry,
		widget.NewLabel("Size"), a.dissectSizeEntry,
		newHintButton("Dissect", "dissect.hint.dissect", a.runDissect),
	)
	inst := container.NewHBox(
		widget.NewLabel("Instance"), a.dissectInstEntry,
		newHintButton("Add Instance", "dissect.hint.add_instance", a.addDissectInstance),
	)
	footer := container.NewHBox(
		newHintButton("Edit Value...", "dissect.hint.edit", a.dissectEdit),
		newHintButton("Follow Pointer", "dissect.hint.follow", a.dissectFollow),
	)
	a.dissectWin.SetContent(fynetooltip.AddWindowToolTipLayer(container.NewBorder(
		container.NewVBox(bar, inst, a.dissectStatus), footer, nil, nil, a.dissectTable), a.dissectWin.Canvas()))
}

func (a *App) dissectHeader(col int) string {
	switch col {
	case 0:
		return "Offset"
	case 1:
		return "Type"
	default:
		i := col - 2
		if i < len(a.dissectBases) {
			return fmt.Sprintf("0x%x", a.dissectBases[i])
		}
		return ""
	}
}

func (a *App) updateDissectCell(id widget.TableCellID, o fyne.CanvasObject) {
	t := o.(*canvas.Text)
	if id.Row < 0 || id.Row >= len(a.dissectFields) {
		t.Text = ""
		t.Refresh()
		return
	}
	f := a.dissectFields[id.Row]
	switch id.Col {
	case 0:
		t.Text = fmt.Sprintf("+0x%x", f.Offset)
	case 1:
		t.Text = f.Kind.String()
	default:
		t.Text = f.Format(id.Col - 2)
	}
	t.Color = a.pal().text
	t.Refresh()
}

func (a *App) runDissect() {
	if a.proc == nil {
		return
	}
	base, err := parseAddress(a.dissectBaseEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	size, err := strconv.Atoi(a.dissectSizeEntry.Text)
	if err != nil || size <= 0 {
		a.fail(fmt.Errorf("invalid size"))
		return
	}
	r, err := dissect.AddressRange(a.proc.PID)
	if err != nil {
		a.fail(err)
		return
	}
	bases := append([]uint64{base}, a.dissectBases...)
	fields, err := dissect.Dissect(a.proc, base, size, bases, r)
	if err != nil {
		a.fail(err)
		return
	}
	a.dissectBase = base
	a.dissectBases = bases
	a.dissectFields = fields
	a.dissectSel = -1
	a.dissectTable.Refresh()
	a.dissectStatus.SetText(fmt.Sprintf("%d fields across %d instance(s)", len(fields), len(bases)))
}

func (a *App) addDissectInstance() {
	addr, err := parseAddress(a.dissectInstEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	for _, b := range a.dissectBases {
		if b == addr {
			a.setStatus("instance already added")
			return
		}
	}
	a.dissectBases = append(a.dissectBases, addr)
	a.runDissect()
}

func (a *App) dissectEdit() {
	if a.dissectSel < 0 || a.dissectSel >= len(a.dissectFields) {
		a.setStatus("select a field first")
		return
	}
	f := a.dissectFields[a.dissectSel]
	entry := widget.NewEntry()
	entry.SetText(f.Format(0))
	d := dialog.NewForm("Edit Field", "Apply", "Cancel",
		[]*widget.FormItem{widget.NewFormItem(f.Kind.String(), entry)},
		func(ok bool) {
			if !ok {
				return
			}
			v, err := scan.ParseValue(f.Kind.ScanType(), entry.Text)
			if err != nil {
				a.fail(err)
				return
			}
			if a.proc == nil {
				return
			}
			if err := a.proc.Write(a.dissectBase+uint64(f.Offset), v.Raw); err != nil {
				a.fail(err)
				return
			}
			a.runDissect()
		}, a.dissectWin)
	d.Resize(fyne.NewSize(360, 180))
	d.Show()
}

func (a *App) dissectFollow() {
	if a.dissectSel < 0 || a.dissectSel >= len(a.dissectFields) {
		a.setStatus("select a field first")
		return
	}
	f := a.dissectFields[a.dissectSel]
	if f.Kind != dissect.KindPointer || len(f.Targets) == 0 {
		a.fail(fmt.Errorf("the selected field is not a pointer"))
		return
	}
	a.dissectBaseEntry.SetText(fmt.Sprintf("0x%x", f.Targets[0]))
	a.runDissect()
}
