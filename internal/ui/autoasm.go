//go:build gui

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/pkg/autoasm"
	"github.com/LCRERGO/firstspark/pkg/debugger"
)

const defaultAAScript = `[ENABLE]
// alloc(newmem, 1024)
// label(return)
//
// newmem:
//   nop
// return:
//   ret

[DISABLE]
// dealloc(newmem)`

// openAutoAssemble shows the Auto Assemble window, creating it lazily.
func (a *App) openAutoAssemble() {
	if a.proc == nil {
		a.fail(fmt.Errorf("no process selected"))
		return
	}
	if a.asmWin == nil {
		a.asmWin = a.fapp.NewWindow("Auto Assemble")
		a.asmWin.Resize(fyne.NewSize(700, 560))
		a.buildAutoAssemble()
	}
	a.asmWin.Show()
}

func (a *App) buildAutoAssemble() {
	a.asmEditor = newCodeEditor(nil)
	a.asmEditor.SetText(defaultAAScript)
	a.asmStatus = widget.NewLabel("ready")
	bar := container.NewHBox(
		newHintButton("Execute", "autoasm.hint.execute", a.runAutoAssemble),
		newHintButton("Revert", "autoasm.hint.revert", a.revertAutoAssemble),
	)
	a.asmWin.SetContent(fynetooltip.AddWindowToolTipLayer(container.NewBorder(
		container.NewVBox(bar, a.asmStatus), nil, nil, nil, a.asmEditor), a.asmWin.Canvas()))
}

func (a *App) runAutoAssemble() {
	if a.proc == nil {
		return
	}
	script, err := autoasm.Parse(a.asmEditor.Text())
	if err != nil {
		a.fail(err)
		return
	}
	be, err := debugger.New(a.cfg.Debugger.Backend, a.proc.PID, debugger.Options{GDBPath: a.cfg.Debugger.GDBPath})
	if err != nil {
		a.fail(err)
		return
	}
	exec := autoasm.NewExecutor(a.proc, be, script)
	if err := exec.Apply(true); err != nil {
		a.fail(err)
		return
	}
	a.asmExec = exec
	a.asmStatus.SetText(fmt.Sprintf("applied; %d symbols", len(exec.Symbols())))
}

func (a *App) revertAutoAssemble() {
	if a.asmExec == nil {
		a.setStatus("nothing to revert")
		return
	}
	if err := a.asmExec.Revert(); err != nil {
		a.fail(err)
		return
	}
	a.asmStatus.SetText("reverted")
}
