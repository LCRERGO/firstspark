//go:build gui

package ui

import (
	"fmt"
	"strings"

	"github.com/LCRERGO/firstspark/pkg/celua"
	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// runLuaTimers fires due script timers. It runs on the UI goroutine.
func (a *App) runLuaTimers() {
	if a.luaRT == nil {
		return
	}
	if err := a.luaRT.RunTimers(); err != nil {
		log.Warn("lua timer failed", "err", err)
	}
}

// luaTable adapts the cheat-table tree to the celua.Table interface.
type luaTable struct{ app *App }

func (t luaTable) Record(desc string) (celua.Record, bool) {
	var found *tableEntry
	t.app.walkEntries(func(e *tableEntry) {
		if found == nil && e.desc == desc {
			found = e
		}
	})
	if found == nil {
		return nil, false
	}
	return luaRecord{t.app, found}, true
}

func (t luaTable) Records() []celua.Record {
	var out []celua.Record
	t.app.walkEntries(func(e *tableEntry) { out = append(out, luaRecord{t.app, e}) })
	return out
}

// Add appends a record created by a script (createMemoryRecord/appendToEntry).
func (t luaTable) Add(desc string, addr uint64, typeName, value string) (celua.Record, error) {
	typ := t.app.defaultValueType()
	if p, err := scan.ParseValueType(typeName); err == nil {
		typ = p
	}
	e := &tableEntry{addr: addr, typ: typ, desc: desc}
	if strings.TrimSpace(value) != "" {
		if v, err := scan.ParseValue(typ, value); err == nil {
			e.value = v
			e.orig = v
		}
	}
	t.app.addRoot(e)
	if t.app.table != nil {
		t.app.table.Refresh()
	}
	return luaRecord{t.app, e}, nil
}

// luaRecord exposes a table entry as a Cheat Engine MemoryRecord.
type luaRecord struct {
	app *App
	e   *tableEntry
}

func (r luaRecord) Description() string { return r.e.desc }
func (r luaRecord) ID() int             { return 0 }
func (r luaRecord) Address() uint64     { return r.e.addr }
func (r luaRecord) TypeName() string    { return r.e.typ.String() }
func (r luaRecord) ValueString() string { return r.e.value.String() }

func (r luaRecord) SetAddress(addr uint64) error {
	r.e.addr = addr
	return nil
}

func (r luaRecord) Active() bool {
	if r.e.script != "" {
		return r.e.scriptExec != nil
	}
	return r.e.frozen
}

func (r luaRecord) SetActive(on bool) error {
	if r.e.script != "" {
		if on {
			r.app.applyEntryScript(r.e)
		} else {
			r.app.disableEntryScript(r.e)
		}
		return nil
	}
	if on != r.e.frozen {
		r.app.toggleFreezeEntry(r.e)
		r.app.table.Refresh()
	}
	return nil
}

func (r luaRecord) SetValueString(s string) error {
	v, err := scan.ParseValue(r.e.typ, s)
	if err != nil {
		return err
	}
	if err := r.app.writeValue(r.e.addr, v); err != nil {
		return err
	}
	r.e.value = v
	return nil
}

// luaRuntime returns the session Lua runtime, creating it lazily. Its globals
// persist across scripts, matching Cheat Engine.
func (a *App) luaRuntime() *celua.Runtime {
	if a.luaRT == nil {
		a.luaRT = a.newLuaRuntime()
	}
	return a.luaRT
}

// newLuaRuntime builds a celua runtime bound to the current app state.
func (a *App) newLuaRuntime() *celua.Runtime {
	return celua.New(celua.Config{
		Proc: a.proc,
		Resolve: func(name string) (uint64, bool) {
			if v, ok := a.symbols[name]; ok {
				return v, true
			}
			if a.proc == nil {
				return 0, false
			}
			regions, err := mem.Regions(a.proc.PID)
			if err != nil {
				return 0, false
			}
			return mem.ModuleBase(regions, name)
		},
		Table: luaTable{a},
		OpenProcess: func(name string) (*mem.Process, error) {
			procs, err := mem.List()
			if err != nil {
				return nil, err
			}
			for i := range procs {
				if procs[i].Name == name {
					p := procs[i]
					a.selectProcessObj(p)
					return &p, nil
				}
			}
			return nil, fmt.Errorf("process %q not found", name)
		},
		Show: func(s string) { a.setStatusText(s) },
		Output: func(s string) {
			a.appendLuaOutput(s)
		},
		Clipboard: func(s string) {
			if a.fapp != nil {
				a.fapp.Clipboard().SetContent(s)
			}
		},
	})
}
