//go:build gui

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/LCRERGO/firstspark/pkg/debugger"
)

var dbgRegNames = []string{
	"RIP", "RSP", "RBP", "RAX", "RBX", "RCX", "RDX", "RSI", "RDI",
	"R8", "R9", "R10", "R11", "R12", "R13", "R14", "R15", "RFLAGS",
}

// openDebugger shows the Debugger window, creating it lazily.
func (a *App) openDebugger() {
	if a.proc == nil {
		a.fail(fmt.Errorf("no process selected"))
		return
	}
	if a.dbgWin == nil {
		a.dbgWin = a.fapp.NewWindow("Debugger")
		a.dbgWin.Resize(fyne.NewSize(660, 560))
		a.buildDebugger()
	}
	a.dbgWin.Show()
}

func (a *App) buildDebugger() {
	a.dbgStatus = widget.NewLabel("not attached")
	a.dbgRegVals = make([]string, len(dbgRegNames))
	a.dbgAddrEntry = widget.NewEntry()
	a.dbgAddrEntry.SetPlaceHolder("0x1234")
	a.dbgBreakpoints = map[uint64]bool{}

	a.dbgRegs = widget.NewList(
		func() int { return len(dbgRegNames) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(dbgRegNames) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = fmt.Sprintf("%-7s %s", dbgRegNames[id], a.dbgRegVals[id])
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.dbgHits = widget.NewList(
		func() int { return len(a.dbgHitLabels) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.dbgHitLabels) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = a.dbgHitLabels[id]
			t.Color = a.pal().text
			t.Refresh()
		},
	)

	controls := container.NewHBox(
		widget.NewButton("Attach", a.debuggerAttach),
		widget.NewButton("Detach", a.debuggerDetach),
		widget.NewButton("Continue", a.debuggerContinue),
		widget.NewButton("Step", a.debuggerStep),
	)
	watch := container.NewHBox(
		a.dbgAddrEntry,
		widget.NewButton("Toggle Breakpoint", a.debuggerToggleBreakpoint),
		widget.NewButton("Find Writes", func() { a.debuggerWatch(true) }),
		widget.NewButton("Find Accesses", func() { a.debuggerWatch(false) }),
		widget.NewButton("Stop Watch", a.debuggerStopWatch),
	)
	top := container.NewVBox(controls, watch, a.dbgStatus)
	a.dbgWin.SetContent(container.NewBorder(top, nil, nil, nil,
		container.NewVSplit(a.dbgRegs, a.dbgHits)))
}

func (a *App) ensureDebuggerSession() bool {
	if a.dbgSession != nil {
		return true
	}
	s, err := debugger.NewSession(a.cfg.Debugger.Backend, a.proc.PID, debugger.Options{GDBPath: a.cfg.Debugger.GDBPath})
	if err != nil {
		a.fail(err)
		return false
	}
	a.dbgSession = s
	return true
}

func (a *App) debuggerAttach() {
	if !a.ensureDebuggerSession() {
		return
	}
	if err := a.dbgSession.Attach(); err != nil {
		a.fail(err)
		return
	}
	a.dbgStatus.SetText(fmt.Sprintf("attached to %d", a.proc.PID))
	a.debuggerRefresh()
}

func (a *App) debuggerDetach() {
	if a.dbgSession == nil {
		return
	}
	if err := a.dbgSession.Detach(); err != nil {
		a.fail(err)
		return
	}
	a.dbgStatus.SetText("detached")
}

func (a *App) debuggerContinue() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("attach first"))
		return
	}
	go func() {
		if err := a.dbgSession.Continue(); err != nil {
			fyne.Do(func() { a.fail(err) })
			return
		}
		reason, err := a.dbgSession.Wait()
		if err != nil {
			fyne.Do(func() { a.fail(err) })
			return
		}
		fyne.Do(func() {
			a.dbgStatus.SetText(describeStop(reason))
			a.debuggerRefresh()
		})
	}()
}

func (a *App) debuggerStep() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("attach first"))
		return
	}
	go func() {
		if err := a.dbgSession.Step(); err != nil {
			fyne.Do(func() { a.fail(err) })
			return
		}
		if _, err := a.dbgSession.Wait(); err != nil {
			fyne.Do(func() { a.fail(err) })
			return
		}
		fyne.Do(func() {
			a.dbgStatus.SetText("stepped")
			a.debuggerRefresh()
		})
	}()
}

func (a *App) debuggerToggleBreakpoint() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("attach first"))
		return
	}
	addr, err := parseAddress(a.dbgAddrEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	if a.dbgBreakpoints[addr] {
		if err := a.dbgSession.ClearBreakpoint(addr); err != nil {
			a.fail(err)
			return
		}
		delete(a.dbgBreakpoints, addr)
		a.dbgStatus.SetText(fmt.Sprintf("cleared breakpoint at 0x%x", addr))
		return
	}
	if err := a.dbgSession.SetBreakpoint(addr); err != nil {
		a.fail(err)
		return
	}
	a.dbgBreakpoints[addr] = true
	a.dbgStatus.SetText(fmt.Sprintf("breakpoint at 0x%x", addr))
}

func (a *App) debuggerWatch(writeOnly bool) {
	if a.proc == nil {
		return
	}
	if !a.ensureDebuggerSession() {
		return
	}
	if a.dbgStop != nil {
		a.setStatus("a watch is already running")
		return
	}
	if !a.dbgSession.SupportsWatchpoints() {
		a.fail(fmt.Errorf("this backend does not support hardware watchpoints"))
		return
	}
	addr, err := parseAddress(a.dbgAddrEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	stop := make(chan struct{})
	a.dbgStop = stop
	mode := "writes to"
	if !writeOnly {
		mode = "accesses of"
	}
	a.dbgStatus.SetText(fmt.Sprintf("watching %s 0x%x", mode, addr))
	go func() {
		err := a.dbgSession.Watch(addr, 4, writeOnly, 50, stop, func(h debugger.Hit) {
			line := fmt.Sprintf("0x%x  %s", h.RIP, h.Instruction)
			fyne.Do(func() {
				a.dbgHitLabels = append(a.dbgHitLabels, line)
				if a.dbgHits != nil {
					a.dbgHits.Refresh()
				}
			})
		})
		fyne.Do(func() {
			if err != nil {
				a.fail(err)
			}
			if a.dbgStatus != nil {
				a.dbgStatus.SetText("watch stopped")
			}
			a.dbgStop = nil
		})
	}()
}

func (a *App) debuggerStopWatch() {
	if a.dbgStop != nil {
		close(a.dbgStop)
		a.dbgStop = nil
	}
}

func (a *App) debuggerRefresh() {
	if a.dbgSession == nil {
		return
	}
	regs, err := a.dbgSession.Registers()
	if err != nil {
		return
	}
	a.dbgRegVals = []string{
		fmt.Sprintf("0x%016x", regs.RIP), fmt.Sprintf("0x%016x", regs.RSP), fmt.Sprintf("0x%016x", regs.RBP),
		fmt.Sprintf("0x%016x", regs.RAX), fmt.Sprintf("0x%016x", regs.RBX), fmt.Sprintf("0x%016x", regs.RCX),
		fmt.Sprintf("0x%016x", regs.RDX), fmt.Sprintf("0x%016x", regs.RSI), fmt.Sprintf("0x%016x", regs.RDI),
		fmt.Sprintf("0x%016x", regs.R8), fmt.Sprintf("0x%016x", regs.R9), fmt.Sprintf("0x%016x", regs.R10),
		fmt.Sprintf("0x%016x", regs.R11), fmt.Sprintf("0x%016x", regs.R12), fmt.Sprintf("0x%016x", regs.R13),
		fmt.Sprintf("0x%016x", regs.R14), fmt.Sprintf("0x%016x", regs.R15), fmt.Sprintf("0x%016x", regs.RFLAGS),
	}
	if a.dbgRegs != nil {
		a.dbgRegs.Refresh()
	}
}

func describeStop(reason debugger.StopReason) string {
	switch reason.Event {
	case debugger.EventExited:
		return fmt.Sprintf("exited with code %d", reason.ExitCode)
	case debugger.EventSignaled:
		return fmt.Sprintf("killed by signal %v", reason.Signal)
	default:
		if reason.HasHardware {
			return fmt.Sprintf("hardware watchpoint %d hit", reason.HardwareSlot)
		}
		if reason.HasBreakpoint {
			return fmt.Sprintf("breakpoint at 0x%x", reason.BreakpointAddr)
		}
		return fmt.Sprintf("stopped (signal %v)", reason.Signal)
	}
}

// findWhatWrites opens the debugger and watches a cheat-table row.
func (a *App) findWhatWrites(row int, writeOnly bool) {
	if row < 0 || row >= len(a.entries) {
		return
	}
	addr := a.entries[row].addr
	a.openDebugger()
	a.dbgAddrEntry.SetText(fmt.Sprintf("0x%x", addr))
	a.debuggerWatch(writeOnly)
}
