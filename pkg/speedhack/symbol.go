// Package speedhack implements time scaling by inline
// hooking the libc time functions and scaling the values they return.
package speedhack

import (
	"debug/elf"
	"fmt"
	"strings"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// ResolveSymbol returns the runtime address of a dynamic symbol in one of the
// shared objects loaded by pid.
func ResolveSymbol(pid int, name string) (uint64, error) {
	regions, err := mem.Regions(pid)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, r := range regions {
		if !r.FileBacked() {
			continue
		}
		path := strings.TrimSuffix(r.Path, " (deleted)")
		if seen[path] {
			continue
		}
		seen[path] = true

		f, err := elf.Open(path)
		if err != nil {
			continue
		}
		value, ok := lookupDynSym(f, name)
		base := loadBase(regions, path)
		f.Close()
		if ok {
			return base + value, nil
		}
	}
	return 0, fmt.Errorf("speedhack: symbol %q not found", name)
}

func loadBase(regions []mem.Region, path string) uint64 {
	var base uint64
	found := false
	for _, r := range regions {
		if strings.TrimSuffix(r.Path, " (deleted)") != path {
			continue
		}
		if r.Offset != 0 {
			continue
		}
		if !found || r.Start < base {
			base = r.Start
			found = true
		}
	}
	return base
}

func lookupDynSym(f *elf.File, name string) (uint64, bool) {
	syms, err := f.DynamicSymbols()
	if err != nil {
		return 0, false
	}
	for _, s := range syms {
		if s.Name == name && s.Section != elf.SHN_UNDEF {
			return s.Value, true
		}
	}
	return 0, false
}
