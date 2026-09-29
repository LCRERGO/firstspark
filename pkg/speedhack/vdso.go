package speedhack

import (
	"debug/elf"
	"fmt"
	"io"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// Resolve returns the runtime address of a logical time symbol in pid. It
// prefers the per-process vDSO and falls back to libc .dynsym. The bool reports
// whether the address came from the vDSO.
func Resolve(pid int, symbol string) (uint64, bool, error) {
	if addr, ok, err := ResolveVDSO(pid, symbol); err == nil && ok {
		return addr, true, nil
	}
	addr, err := ResolveSymbol(pid, symbol)
	return addr, false, err
}

// ResolveVDSO resolves symbol inside the target's [vdso] mapping.
func ResolveVDSO(pid int, symbol string) (uint64, bool, error) {
	name := vdsoSymbolName(symbol)
	if name == "" {
		return 0, false, nil
	}
	region, ok := findVDSO(pid)
	if !ok {
		return 0, false, nil
	}
	proc, err := mem.Find(pid)
	if err != nil {
		return 0, false, err
	}
	f, err := elf.NewFile(procReaderAt{proc: proc, base: region.Start, size: region.Size()})
	if err != nil {
		return 0, false, fmt.Errorf("speedhack: parse vdso: %w", err)
	}
	defer f.Close()

	tables := []func(*elf.File) ([]elf.Symbol, error){(*elf.File).DynamicSymbols, (*elf.File).Symbols}
	for _, table := range tables {
		syms, err := table(f)
		if err != nil {
			continue
		}
		for _, s := range syms {
			if s.Name == name && s.Section != elf.SHN_UNDEF {
				return region.Start + s.Value, true, nil
			}
		}
	}
	return 0, false, nil
}

// vdsoSymbolName maps a logical name to the kernel's vDSO symbol name.
func vdsoSymbolName(symbol string) string {
	switch symbol {
	case "clock_gettime", "gettimeofday", "time", "clock_getres":
		return "__vdso_" + symbol
	default:
		return ""
	}
}

func findVDSO(pid int) (mem.Region, bool) {
	regions, err := mem.Regions(pid)
	if err != nil {
		return mem.Region{}, false
	}
	for _, r := range regions {
		if r.Path == "[vdso]" {
			return r, true
		}
	}
	return mem.Region{}, false
}

// procReaderAt adapts process memory to io.ReaderAt for debug/elf.
type procReaderAt struct {
	proc *mem.Process
	base uint64
	size uint64
}

func (r procReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || uint64(off) >= r.size {
		return 0, io.EOF
	}
	if max := int(r.size - uint64(off)); len(p) > max {
		p = p[:max]
	}
	data, err := r.proc.Read(r.base+uint64(off), len(p))
	n := copy(p, data)
	if err != nil {
		if n > 0 {
			return n, nil
		}
		return 0, err
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
