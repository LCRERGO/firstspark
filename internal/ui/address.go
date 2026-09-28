//go:build gui

package ui

import (
	"fmt"

	"github.com/LCRERGO/firstspark/pkg/address"
	"github.com/LCRERGO/firstspark/pkg/mem"
)

// symbolResolver resolves address names: script symbols first,
// then module load bases.
type symbolResolver struct {
	symbols map[string]uint64
	regions []mem.Region
}

func (r symbolResolver) ResolveName(name string) (uint64, bool) {
	if v, ok := r.symbols[name]; ok {
		return v, true
	}
	if len(r.regions) > 0 {
		if base, ok := mem.ModuleBase(r.regions, name); ok {
			return base, true
		}
	}
	return 0, false
}

// resolveExpression resolves an entry's stored address expression against
// its parent and the symbol/module tables, then applies the offset chain.
func (a *App) resolveExpression(e *tableEntry, r symbolResolver) (uint64, error) {
	parent, hasParent := uint64(0), false
	if e.parent != nil && e.parent.addr != 0 {
		parent, hasParent = e.parent.addr, true
	}
	addr, err := address.Eval(e.expr, parent, hasParent, r)
	if err != nil {
		return 0, err
	}
	for _, off := range e.exprOffsets {
		if a.proc == nil {
			return 0, fmt.Errorf("no process")
		}
		v, err := address.EvalOffset(off, r)
		if err != nil {
			return 0, err
		}
		ptr, err := a.proc.ReadUint64(addr)
		if err != nil {
			return 0, err
		}
		addr = ptr + uint64(v)
	}
	return addr, nil
}
