//go:build gui

package ui

import (
	"fmt"

	"fyne.io/fyne/v2/dialog"

	"github.com/LCRERGO/firstspark/internal/i18n"
	"github.com/LCRERGO/firstspark/pkg/autoasm"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/log"
)

// runEntryScript confirms and then enables a table script. Enabling runs it
// against the target and registers its symbols (ADR 0039).
func (a *App) runEntryScript(e *tableEntry) {
	if e == nil || e.script == "" {
		return
	}
	if a.proc == nil {
		a.fail(fmt.Errorf("%s", i18n.T("error.no_process")))
		return
	}
	d := dialog.NewConfirm(i18n.T("dialog.run_script_title"), i18n.T("dialog.run_script_body"),
		func(ok bool) {
			if ok {
				a.applyEntryScript(e)
			}
		}, a.win)
	d.Show()
}

func (a *App) applyEntryScript(e *tableEntry) {
	script, err := autoasm.Parse(e.script)
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
	rt := a.luaRuntime()
	rt.SetProc(a.proc)
	exec.SetLuaRunner(rt.Eval)
	if err := exec.Apply(true); err != nil {
		_ = be.Close()
		log.Warn("table script apply failed", "pid", a.proc.PID, "err", err)
		a.fail(err)
		return
	}
	e.scriptExec = exec
	e.scriptBE = be
	if a.symbols == nil {
		a.symbols = map[string]uint64{}
	}
	for name, addr := range exec.Symbols() {
		a.symbols[name] = addr
	}
	a.refreshEntries()
	a.table.Refresh()
	a.setStatusText(i18n.Tf("status.script_applied", map[string]any{"Name": e.desc}))
}

func (a *App) disableEntryScript(e *tableEntry) {
	if e == nil || e.scriptExec == nil {
		return
	}
	if err := e.scriptExec.Revert(); err != nil {
		log.Warn("table script revert failed", "err", err)
		a.fail(err)
	}
	if e.scriptBE != nil {
		_ = e.scriptBE.Close()
	}
	e.scriptExec = nil
	e.scriptBE = nil
	a.refreshEntries()
	a.table.Refresh()
	a.setStatusText(i18n.Tf("status.script_reverted", map[string]any{"Name": e.desc}))
}
