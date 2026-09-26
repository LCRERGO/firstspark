//go:build gui

package ui

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/mem"
)

var dbgRegNames = []string{
	"RIP", "RSP", "RBP", "RAX", "RBX", "RCX", "RDX", "RSI", "RDI",
	"R8", "R9", "R10", "R11", "R12", "R13", "R14", "R15", "RFLAGS",
}

// openDebugger shows the Debugger window, creating it lazily.
func (a *App) openDebugger() {
	if a.proc == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.no_process")))
		return
	}
	if a.dbgWin == nil {
		a.dbgWin = a.fapp.NewWindow(i18n.T("debugger.title"))
		a.dbgWin.Resize(fyne.NewSize(680, 660))
		a.buildDebugger()
	}
	if a.dbgTID == 0 {
		a.dbgTID = a.proc.PID
	}
	a.refreshThreads()
	a.refreshModules()
	a.dbgWin.Show()
	a.dbgWin.Canvas().Focus(a.dbgRegs)
}

// listThreads returns the TIDs of a process's threads.
func listThreads(pid int) []int {
	entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	if err != nil {
		return nil
	}
	out := make([]int, 0, len(entries))
	for _, e := range entries {
		if tid, err := strconv.Atoi(e.Name()); err == nil {
			out = append(out, tid)
		}
	}
	sort.Ints(out)
	return out
}

// threadName reads a thread's command name.
func threadName(pid, tid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/comm", pid, tid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (a *App) refreshThreads() {
	a.dbgThreads = nil
	if a.proc != nil {
		a.dbgThreads = listThreads(a.proc.PID)
	}
	if a.dbgThreadList != nil {
		a.dbgThreadList.Refresh()
	}
}

func (a *App) refreshModules() {
	a.dbgModules = nil
	if a.proc != nil {
		if regions, err := mem.Regions(a.proc.PID); err == nil {
			for _, r := range regions {
				if r.FileBacked() && r.Offset == 0 {
					a.dbgModules = append(a.dbgModules, r)
				}
			}
		}
	}
	if a.dbgModuleList != nil {
		a.dbgModuleList.Refresh()
	}
}

// selectThread rebinds the debugger to another thread, re-attaching when it was
// already attached.
func (a *App) selectThread(tid int) {
	if tid == a.dbgTID {
		return
	}
	wasAttached := a.dbgAttached
	if a.dbgSession != nil {
		_ = a.dbgSession.Detach()
		_ = a.dbgSession.Close()
		a.dbgSession = nil
		a.dbgAttached = false
	}
	a.dbgTID = tid
	a.dbgBreakpoints = map[uint64]bool{}
	a.refreshBreakpointList()
	a.refreshThreads()
	a.dbgStatus.SetText(i18n.Tf("debugger.thread_selected", map[string]any{"TID": tid}))
	if wasAttached {
		a.debuggerAttach()
	}
}

// followRegister opens the Memory Viewer at RIP or RSP.
func (a *App) followRegister(rip bool) {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.attach_first")))
		return
	}
	regs, err := a.dbgSession.Registers()
	if err != nil {
		a.fail(err)
		return
	}
	addr := regs.RSP
	if rip {
		addr = regs.RIP
	}
	a.openMemoryViewer()
	a.loadMemory(addr)
}

func (a *App) buildDebugger() {
	a.dbgStatus = widget.NewLabel(i18n.T("debugger.not_attached"))
	a.dbgRegVals = make([]string, len(dbgRegNames))
	a.dbgAddrEntry = newHintEntry("debugger.hint.address")
	a.dbgAddrEntry.SetPlaceHolder(i18n.T("debugger.address_placeholder"))
	a.dbgBreakpoints = map[uint64]bool{}
	a.dbgWatchpoints = map[uint64]int{}
	a.dbgWatchWrite = map[uint64]bool{}

	a.dbgRegs = a.newDebuggerList(
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
	a.dbgHits = a.newDebuggerList(
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
	a.dbgBPList = a.newDebuggerList(
		func() int { return len(a.dbgBPLabels) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.dbgBPLabels) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = a.dbgBPLabels[id]
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.dbgThreadList = a.newDebuggerList(
		func() int { return len(a.dbgThreads) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.dbgThreads) {
				t.Text = ""
				t.Refresh()
				return
			}
			tid := a.dbgThreads[id]
			mark := "  "
			if tid == a.dbgTID {
				mark = "* "
			}
			name := ""
			if a.proc != nil {
				name = threadName(a.proc.PID, tid)
			}
			t.Text = fmt.Sprintf("%s%d  %s", mark, tid, name)
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.dbgThreadList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(a.dbgThreads) {
			a.selectThread(a.dbgThreads[id])
		}
	}
	a.dbgModuleList = a.newDebuggerList(
		func() int { return len(a.dbgModules) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.dbgModules) {
				t.Text = ""
				t.Refresh()
				return
			}
			r := a.dbgModules[id]
			t.Text = fmt.Sprintf("0x%012x  %10s  %s", r.Start, humanBytes(r.Size()), r.Path)
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.dbgModuleList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(a.dbgModules) {
			a.openMemoryViewer()
			a.loadMemory(a.dbgModules[id].Start)
		}
	}
	a.dbgStackList = a.newDebuggerList(
		func() int { return len(a.dbgStack) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.dbgStack) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = a.dbgStack[id]
			t.Color = a.pal().text
			t.Refresh()
		},
	)
	a.dbgTraceList = a.newDebuggerList(
		func() int { return len(a.dbgTrace) },
		func() fyne.CanvasObject { return a.monoText("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			t := o.(*canvas.Text)
			if id < 0 || id >= len(a.dbgTrace) {
				t.Text = ""
				t.Refresh()
				return
			}
			t.Text = a.dbgTrace[id]
			t.Color = a.pal().text
			t.Refresh()
		},
	)

	controls := container.NewHBox(
		newHintButton(i18n.T("debugger.attach"), "debugger.hint.attach", a.debuggerAttach),
		newHintButton(i18n.T("debugger.detach"), "debugger.hint.detach", a.debuggerDetach),
		newHintButton(i18n.T("debugger.continue"), "debugger.hint.continue", a.debuggerContinue),
		newHintButton(i18n.T("debugger.step"), "debugger.hint.step", a.debuggerStep),
		newHintButton(i18n.T("debugger.step_over"), "debugger.hint.step_over", a.debuggerStepOver),
		newHintButton(i18n.T("debugger.call"), "debugger.hint.call", a.debuggerCallDialog),
	)
	watch := container.NewHBox(
		a.dbgAddrEntry,
		newHintButton(i18n.T("debugger.toggle_breakpoint"), "debugger.hint.toggle_breakpoint", a.debuggerToggleBreakpoint),
		newHintButton(i18n.T("debugger.find_writes"), "debugger.hint.find_writes", func() { a.debuggerWatch(true) }),
		newHintButton(i18n.T("debugger.find_accesses"), "debugger.hint.find_accesses", func() { a.debuggerWatch(false) }),
		newHintButton(i18n.T("debugger.stop_watch"), "debugger.hint.stop_watch", a.debuggerStopWatch),
	)
	follow := container.NewHBox(
		newHintButton(i18n.T("debugger.follow_rip"), "debugger.hint.follow_rip", func() { a.followRegister(true) }),
		newHintButton(i18n.T("debugger.follow_rsp"), "debugger.hint.follow_rsp", func() { a.followRegister(false) }),
		newHintButton(i18n.T("debugger.refresh"), "debugger.hint.refresh", a.debuggerRefresh),
	)
	a.dbgRegEdit = newHintEntry("debugger.hint.register_edit")
	a.dbgRegEdit.SetPlaceHolder(i18n.T("debugger.register_edit_placeholder"))
	register := container.NewHBox(
		a.dbgRegEdit,
		newHintButton(i18n.T("debugger.set_register"), "debugger.hint.set_register", a.debuggerSetRegister),
	)
	top := container.NewVBox(container.NewHScroll(controls), container.NewHScroll(watch), follow, register, a.dbgStatus)
	tabs := container.NewAppTabs(
		container.NewTabItem(i18n.T("debugger.tab.registers"), a.dbgRegs),
		container.NewTabItem(i18n.T("debugger.tab.threads"), a.dbgThreadList),
		container.NewTabItem(i18n.T("debugger.tab.modules"), a.dbgModuleList),
		container.NewTabItem(i18n.T("debugger.tab.breakpoints"), a.dbgBPList),
		container.NewTabItem(i18n.T("debugger.tab.hits"), a.dbgHits),
		container.NewTabItem(i18n.T("debugger.tab.stack"), a.dbgStackList),
		container.NewTabItem(i18n.T("debugger.tab.trace"), a.dbgTraceList),
	)
	a.dbgWin.SetContent(fynetooltip.AddWindowToolTipLayer(container.NewBorder(top, nil, nil, nil, tabs), a.dbgWin.Canvas()))
}

// dbgList is a list that also handles the debugger's bare function keys, which
// Fyne delivers to the focused widget instead of the shortcut system.
type dbgList struct {
	widget.List
	app *App
}

func (a *App) newDebuggerList(length func() int, create func() fyne.CanvasObject, update func(widget.ListItemID, fyne.CanvasObject)) *dbgList {
	l := &dbgList{app: a}
	l.Length = length
	l.CreateItem = create
	l.UpdateItem = update
	l.ExtendBaseWidget(l)
	return l
}

func (l *dbgList) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyF9:
		l.app.debuggerContinue()
		return
	case fyne.KeyF7:
		l.app.debuggerStep()
		return
	case fyne.KeyF8:
		l.app.debuggerStepOver()
		return
	case fyne.KeyF5:
		l.app.debuggerToggleBreakpoint()
		return
	}
	l.List.TypedKey(ev)
}

func (a *App) ensureDebuggerSession() bool {
	if a.dbgSession != nil {
		return true
	}
	tid := a.debuggerTID()
	if tid <= 0 {
		a.fail(fmt.Errorf("%s", i18n.T("error.no_process")))
		return false
	}
	s, err := debugger.NewSession(a.cfg.Debugger.Backend, tid, debugger.Options{GDBPath: a.cfg.Debugger.GDBPath})
	if err != nil {
		a.fail(err)
		return false
	}
	a.dbgSession = s
	return true
}

// debuggerTID returns the thread the debugger is bound to.
func (a *App) debuggerTID() int {
	if a.dbgTID != 0 {
		return a.dbgTID
	}
	if a.proc != nil {
		return a.proc.PID
	}
	return 0
}

func (a *App) debuggerAttach() {
	if !a.ensureDebuggerSession() {
		return
	}
	if err := a.dbgSession.Attach(); err != nil {
		a.fail(err)
		return
	}
	a.dbgAttached = true
	log.Info("debugger attached", "tid", a.debuggerTID())
	a.dbgStatus.SetText(i18n.Tf("debugger.attached_to", map[string]any{"PID": a.debuggerTID()}))
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
	a.dbgAttached = false
	log.Info("debugger detached", "tid", a.debuggerTID())
	a.dbgStatus.SetText(i18n.T("debugger.detached"))
}

func (a *App) debuggerContinue() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.attach_first")))
		return
	}
	pid := a.proc.PID
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
		switch reason.Event {
		case debugger.EventExited:
			log.Warn("target exited while debugging", "pid", pid, "code", reason.ExitCode)
		case debugger.EventSignaled:
			log.Warn("target killed by signal while debugging", "pid", pid, "signal", reason.Signal)
		}
		fyne.Do(func() {
			a.dbgStatus.SetText(describeStop(reason))
			if reason.Event != debugger.EventStopped {
				a.processGone(pid)
				return
			}
			a.debuggerRefresh()
		})
	}()
}

func (a *App) debuggerStep() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.attach_first")))
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
			a.dbgStatus.SetText(i18n.T("debugger.stepped"))
			a.debuggerRefresh()
		})
	}()
}

// debuggerStepOver runs a call to its return, or single-steps otherwise.
func (a *App) debuggerStepOver() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.attach_first")))
		return
	}
	go func() {
		regs, err := a.dbgSession.Registers()
		if err != nil {
			fyne.Do(func() { a.fail(err) })
			return
		}
		raw, _ := a.dbgSession.Read(regs.RIP, 16)
		ins := asm.Disassemble(raw, regs.RIP)
		isCall := len(ins) > 0 && strings.HasPrefix(ins[0].Text, "call")
		if !isCall {
			if err := a.dbgSession.Step(); err != nil {
				fyne.Do(func() { a.fail(err) })
				return
			}
			if _, err := a.dbgSession.Wait(); err != nil {
				fyne.Do(func() { a.fail(err) })
				return
			}
			fyne.Do(func() { a.dbgStatus.SetText(i18n.T("debugger.stepped")); a.debuggerRefresh() })
			return
		}
		retRaw, err := a.dbgSession.Read(regs.RSP, 8)
		if err != nil || len(retRaw) < 8 {
			fyne.Do(func() { a.fail(fmt.Errorf("%s", i18n.T("debugger.step_over_failed"))) })
			return
		}
		retAddr := binary.LittleEndian.Uint64(retRaw)
		if err := a.dbgSession.SetBreakpoint(retAddr); err != nil {
			fyne.Do(func() { a.fail(err) })
			return
		}
		if err := a.dbgSession.Continue(); err != nil {
			_ = a.dbgSession.ClearBreakpoint(retAddr)
			fyne.Do(func() { a.fail(err) })
			return
		}
		if _, err := a.dbgSession.Wait(); err != nil {
			_ = a.dbgSession.ClearBreakpoint(retAddr)
			fyne.Do(func() { a.fail(err) })
			return
		}
		_ = a.dbgSession.ClearBreakpoint(retAddr)
		fyne.Do(func() {
			a.dbgStatus.SetText(i18n.T("debugger.stepped_over"))
			a.debuggerRefresh()
		})
	}()
}

func (a *App) debuggerToggleBreakpoint() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.attach_first")))
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
		a.dbgStatus.SetText(i18n.Tf("debugger.cleared_breakpoint", map[string]any{"Addr": fmt.Sprintf("%x", addr)}))
		a.refreshBreakpointList()
		return
	}
	if err := a.dbgSession.SetBreakpoint(addr); err != nil {
		a.fail(err)
		return
	}
	a.dbgBreakpoints[addr] = true
	a.dbgStatus.SetText(i18n.Tf("debugger.breakpoint", map[string]any{"Addr": fmt.Sprintf("%x", addr)}))
	a.refreshBreakpointList()
}

func (a *App) debuggerWatch(writeOnly bool) {
	if a.proc == nil {
		return
	}
	if !a.ensureDebuggerSession() {
		return
	}
	if a.dbgStop != nil {
		a.setStatusText(i18n.T("debugger.watch_running"))
		return
	}
	if !a.dbgSession.SupportsWatchpoints() {
		a.fail(fmt.Errorf("%s", i18n.T("debugger.no_watchpoints")))
		return
	}
	addr, err := parseAddress(a.dbgAddrEntry.Text)
	if err != nil {
		a.fail(err)
		return
	}
	stop := make(chan struct{})
	a.dbgStop = stop
	mode := i18n.T("debugger.watch_writes")
	if !writeOnly {
		mode = i18n.T("debugger.watch_accesses")
	}
	a.dbgWatchpoints[addr] = 4
	a.dbgWatchWrite[addr] = writeOnly
	a.refreshBreakpointList()
	a.dbgStatus.SetText(i18n.Tf("debugger.watching", map[string]any{"Mode": mode, "Addr": fmt.Sprintf("%x", addr)}))
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
				a.dbgStatus.SetText(i18n.T("debugger.watch_stopped"))
			}
			delete(a.dbgWatchpoints, addr)
			delete(a.dbgWatchWrite, addr)
			a.refreshBreakpointList()
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

// debuggerSetRegister applies a "NAME=value" register edit.
func (a *App) debuggerSetRegister() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.attach_first")))
		return
	}
	regs, err := a.dbgSession.Registers()
	if err != nil {
		a.fail(err)
		return
	}
	updated, err := applyRegisterEdit(regs, a.dbgRegEdit.Text)
	if err != nil {
		a.fail(err)
		return
	}
	if err := a.dbgSession.SetRegisters(updated); err != nil {
		a.fail(err)
		return
	}
	a.dbgRegEdit.SetText("")
	a.debuggerRefresh()
}

// debuggerCallDialog invokes a function in the target with typed arguments.
func (a *App) debuggerCallDialog() {
	if a.dbgSession == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.attach_first")))
		return
	}
	addr := widget.NewEntry()
	addr.SetPlaceHolder(i18n.T("debugger.address_placeholder"))
	args := widget.NewMultiLineEntry()
	args.SetPlaceHolder(i18n.T("debugger.call_args_placeholder"))
	ret := widget.NewSelect([]string{"int", "float", "double"}, nil)
	ret.SetSelected("int")
	d := dialog.NewForm(i18n.T("debugger.call"), i18n.T("action.apply"), i18n.T("action.cancel"),
		[]*widget.FormItem{
			widget.NewFormItem(i18n.T("debugger.call_address"), addr),
			widget.NewFormItem(i18n.T("debugger.call_args"), args),
			widget.NewFormItem(i18n.T("debugger.call_return"), ret),
		},
		func(ok bool) {
			if !ok {
				return
			}
			fn, err := parseAddress(addr.Text)
			if err != nil {
				a.fail(err)
				return
			}
			parsed, err := parseCallArgs(args.Text)
			if err != nil {
				a.fail(err)
				return
			}
			res, err := a.dbgSession.Call(fn, parsed)
			if err != nil {
				a.fail(err)
				return
			}
			var out string
			switch ret.Selected {
			case "float":
				out = strconv.FormatFloat(float64(res.Float()), 'g', -1, 32)
			case "double":
				out = strconv.FormatFloat(res.Double(), 'g', -1, 64)
			default:
				out = fmt.Sprintf("0x%x", res.RAX)
			}
			a.dbgStatus.SetText(i18n.Tf("debugger.call_result", map[string]any{"Result": out}))
			dialog.ShowInformation(i18n.T("debugger.call"), out, a.dbgWin)
		}, a.dbgWin)
	d.Resize(fyne.NewSize(460, 320))
	d.Show()
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
	a.refreshThreads()
	a.refreshModules()
}

func (a *App) refreshBreakpointList() {
	labels := make([]string, 0, len(a.dbgBreakpoints)+len(a.dbgWatchpoints))
	for addr := range a.dbgBreakpoints {
		labels = append(labels, fmt.Sprintf("BP  0x%x", addr))
	}
	for addr, size := range a.dbgWatchpoints {
		mode := "r/w"
		if a.dbgWatchWrite[addr] {
			mode = "w"
		}
		labels = append(labels, fmt.Sprintf("WP  0x%x  %d bytes  %s", addr, size, mode))
	}
	sort.Strings(labels)
	a.dbgBPLabels = labels
	if a.dbgBPList != nil {
		a.dbgBPList.Refresh()
	}
}

// applyRegisterEdit applies a "NAME=value" edit to a register snapshot.
func applyRegisterEdit(regs debugger.Registers, text string) (debugger.Registers, error) {
	parts := strings.SplitN(text, "=", 2)
	if len(parts) != 2 {
		return regs, fmt.Errorf("%s", i18n.T("error.register_edit"))
	}
	name := strings.ToUpper(strings.TrimSpace(parts[0]))
	v, err := parseUintLoose(parts[1])
	if err != nil {
		return regs, fmt.Errorf("%s", i18n.T("error.register_edit"))
	}
	switch name {
	case "RIP":
		regs.RIP = v
	case "RSP":
		regs.RSP = v
	case "RBP":
		regs.RBP = v
	case "RAX":
		regs.RAX = v
	case "RBX":
		regs.RBX = v
	case "RCX":
		regs.RCX = v
	case "RDX":
		regs.RDX = v
	case "RSI":
		regs.RSI = v
	case "RDI":
		regs.RDI = v
	case "R8":
		regs.R8 = v
	case "R9":
		regs.R9 = v
	case "R10":
		regs.R10 = v
	case "R11":
		regs.R11 = v
	case "R12":
		regs.R12 = v
	case "R13":
		regs.R13 = v
	case "R14":
		regs.R14 = v
	case "R15":
		regs.R15 = v
	case "RFLAGS":
		regs.RFLAGS = v
	default:
		return regs, fmt.Errorf("%s", i18n.T("error.register_edit"))
	}
	return regs, nil
}

// parseCallArgs parses lines of "kind:value" (kind defaults to int).
func parseCallArgs(text string) ([]debugger.CallArg, error) {
	var args []debugger.CallArg
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kind, val := "int", line
		if i := strings.IndexByte(line, ':'); i >= 0 {
			kind = strings.ToLower(strings.TrimSpace(line[:i]))
			val = strings.TrimSpace(line[i+1:])
		}
		switch kind {
		case "int", "ptr", "pointer":
			n, err := parseUintLoose(val)
			if err != nil {
				return nil, fmt.Errorf("%s", i18n.Tf("error.invalid_argument", map[string]any{"Arg": line}))
			}
			args = append(args, debugger.CallArg{Kind: debugger.ArgInt, Uint: n})
		case "float":
			f, err := strconv.ParseFloat(val, 32)
			if err != nil {
				return nil, fmt.Errorf("%s", i18n.Tf("error.invalid_argument", map[string]any{"Arg": line}))
			}
			args = append(args, debugger.CallArg{Kind: debugger.ArgFloat, Float: f})
		case "double":
			f, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return nil, fmt.Errorf("%s", i18n.Tf("error.invalid_argument", map[string]any{"Arg": line}))
			}
			args = append(args, debugger.CallArg{Kind: debugger.ArgDouble, Float: f})
		default:
			return nil, fmt.Errorf("%s", i18n.Tf("error.invalid_argument", map[string]any{"Arg": line}))
		}
	}
	return args, nil
}

func parseUintLoose(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseUint(s, 0, 64); err == nil {
		return n, nil
	}
	return strconv.ParseUint(s, 16, 64)
}

func describeStop(reason debugger.StopReason) string {
	switch reason.Event {
	case debugger.EventExited:
		return i18n.Tf("debugger.exited", map[string]any{"Code": reason.ExitCode})
	case debugger.EventSignaled:
		return i18n.Tf("debugger.signaled", map[string]any{"Signal": reason.Signal})
	default:
		if reason.HasHardware {
			return i18n.Tf("debugger.watchpoint_hit", map[string]any{"Slot": reason.HardwareSlot})
		}
		if reason.HasBreakpoint {
			return i18n.Tf("debugger.breakpoint_hit", map[string]any{"Addr": fmt.Sprintf("%x", reason.BreakpointAddr)})
		}
		return i18n.Tf("debugger.stopped", map[string]any{"Signal": reason.Signal})
	}
}

// findWhatWrites opens the debugger and watches a cheat-table row.
func (a *App) findWhatWrites(row int, writeOnly bool) {
	if row < 0 || row >= len(a.entries) || a.entries[row].group || a.entries[row].expr != "" {
		return
	}
	a.findWhatWritesAddr(a.entries[row].addr, writeOnly)
}

// findWhatWritesAddr opens the debugger and watches an arbitrary address.
func (a *App) findWhatWritesAddr(addr uint64, writeOnly bool) {
	a.openDebugger()
	a.dbgAddrEntry.SetText(fmt.Sprintf("0x%x", addr))
	a.debuggerWatch(writeOnly)
}
