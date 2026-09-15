package autoasm

import (
	"fmt"
	"strconv"

	"golang.org/x/sys/unix"

	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

type allocator interface {
	Mmap(length uint64, prot, flags int) (uint64, error)
}

type protector interface {
	Mprotect(addr, length uint64, prot int) error
}

// Executor applies an Auto Assembler script to a process.
type Executor struct {
	proc    *mem.Process
	be      debugger.Backend
	script  *Script
	symbols map[string]uint64
	written map[uint64][]byte
}

// NewExecutor creates an executor for proc using be to allocate memory.
func NewExecutor(proc *mem.Process, be debugger.Backend, s *Script) *Executor {
	return &Executor{
		proc:    proc,
		be:      be,
		script:  s,
		symbols: map[string]uint64{},
		written: map[uint64][]byte{},
	}
}

// Symbols returns the resolved symbols (allocs and aobscan results).
func (e *Executor) Symbols() map[string]uint64 { return e.symbols }

func (e *Executor) section(enable bool) *Section {
	for i := range e.script.Sections {
		if e.script.Sections[i].Enable == enable {
			return &e.script.Sections[i]
		}
	}
	return nil
}

// Apply processes the [ENABLE] or [DISABLE] section: it allocates memory,
// resolves aobscans, assembles the code and writes it into the target.
func (e *Executor) Apply(enable bool) error {
	s := e.section(enable)
	if s == nil {
		return fmt.Errorf("autoasm: no section")
	}
	alloc, ok := e.be.(allocator)
	if !ok {
		return fmt.Errorf("autoasm: backend cannot allocate memory")
	}
	if err := e.be.Attach(); err != nil {
		return err
	}
	defer e.be.Detach()

	firstAlloc := uint64(0)
	for _, it := range s.Items {
		switch it.Kind {
		case KindAlloc:
			size := uint64(0x1000)
			if len(it.Args) > 1 {
				if n, err := strconv.ParseUint(it.Args[1], 0, 64); err == nil && n > 0 {
					size = n
				}
			}
			addr, err := alloc.Mmap(size, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC,
				unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
			if err != nil {
				return err
			}
			e.symbols[it.Name] = addr
			if firstAlloc == 0 {
				firstAlloc = addr
			}
		case KindDealloc:
			delete(e.symbols, it.Name)
		case KindAOBScan:
			if len(it.Args) < 2 {
				continue
			}
			addr, err := aobScan(e.proc, it.Args[1])
			if err != nil {
				return err
			}
			e.symbols[it.Name] = addr
		}
	}
	return e.emit(s, firstAlloc)
}

// emit assembles and writes each code block at its base address.
func (e *Executor) emit(s *Section, firstAlloc uint64) error {
	blocks := map[uint64]*Section{}
	var order []uint64
	block := func(base uint64) *Section {
		b, ok := blocks[base]
		if !ok {
			b = &Section{Enable: s.Enable}
			blocks[base] = b
			order = append(order, base)
		}
		return b
	}
	cur := uint64(0)
	for _, it := range s.Items {
		switch it.Kind {
		case KindLabel:
			if v, ok := e.symbols[it.Name]; ok {
				cur = v
				continue
			}
			if cur == 0 {
				continue
			}
			block(cur).Items = append(block(cur).Items, it)
		case KindInstruction, KindData:
			if cur == 0 {
				if firstAlloc == 0 {
					return fmt.Errorf("autoasm: no alloc before code")
				}
				cur = firstAlloc
			}
			block(cur).Items = append(block(cur).Items, it)
		}
	}
	for _, base := range order {
		code, _, err := Assemble(blocks[base], base, e.symbols)
		if err != nil {
			return err
		}
		if err := e.write(base, code); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) write(addr uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if _, ok := e.written[addr]; !ok {
		if orig, err := e.proc.Read(addr, len(data)); err == nil {
			e.written[addr] = orig
		}
	}
	if err := e.proc.Write(addr, data); err == nil {
		return nil
	}
	if p, ok := e.be.(protector); ok {
		page := addr &^ 0xFFF
		length := ((addr + uint64(len(data)) - page) + 0xFFF) &^ 0xFFF
		if err := p.Mprotect(page, length, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
			return err
		}
		return e.proc.Write(addr, data)
	}
	return fmt.Errorf("autoasm: write to 0x%x failed", addr)
}

// Revert restores the bytes overwritten by Apply.
func (e *Executor) Revert() error {
	if err := e.be.Attach(); err != nil {
		return err
	}
	defer e.be.Detach()
	for addr, orig := range e.written {
		if err := e.proc.Write(addr, orig); err != nil {
			if p, ok := e.be.(protector); ok {
				page := addr &^ 0xFFF
				length := ((addr + uint64(len(orig)) - page) + 0xFFF) &^ 0xFFF
				if merr := p.Mprotect(page, length, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); merr == nil {
					_ = e.proc.Write(addr, orig)
				}
			}
		}
	}
	e.written = map[uint64][]byte{}
	return nil
}

// aobScan returns the first address matching an array-of-bytes pattern.
func aobScan(p *mem.Process, pattern string) (uint64, error) {
	pat, err := scan.ParseAOB(pattern)
	if err != nil {
		return 0, err
	}
	regions, err := mem.Regions(p.PID)
	if err != nil {
		return 0, err
	}
	for _, r := range regions {
		if !r.Readable() || r.Size() == 0 {
			continue
		}
		window := r.Size()
		if window > 1<<24 {
			window = 1 << 24
		}
		data, rerr := p.Read(r.Start, int(window))
		for i := 0; i+len(pat.Bytes) <= len(data); i++ {
			if pat.Match(data[i:]) {
				return r.Start + uint64(i), nil
			}
		}
		if rerr != nil {
			continue
		}
	}
	return 0, fmt.Errorf("autoasm: pattern not found")
}
