package debugger

import "github.com/LCRERGO/firstspark/pkg/asm"

// Session wraps a Backend with higher-level controls for the GUI.
type Session struct {
	backend Backend
	watch   WatchpointBackend
}

// NewSession constructs a backend by name and wraps it.
func NewSession(name string, pid int, opts Options) (*Session, error) {
	b, err := New(name, pid, opts)
	if err != nil {
		return nil, err
	}
	s := &Session{backend: b}
	if w, ok := b.(WatchpointBackend); ok {
		s.watch = w
	}
	return s, nil
}

// PID returns the target process id.
func (s *Session) PID() int { return s.backend.PID() }

// SupportsWatchpoints reports whether hardware watchpoints are available.
func (s *Session) SupportsWatchpoints() bool { return s.watch != nil }

// Attach attaches to the target.
func (s *Session) Attach() error { return s.backend.Attach() }

// Detach detaches from the target.
func (s *Session) Detach() error { return s.backend.Detach() }

// Close releases the backend.
func (s *Session) Close() error { return s.backend.Close() }

// Registers reads the target's registers.
func (s *Session) Registers() (Registers, error) { return s.backend.Registers() }

// SetRegisters writes the target's registers.
func (s *Session) SetRegisters(r Registers) error { return s.backend.SetRegisters(r) }

// Read reads target memory.
func (s *Session) Read(addr uint64, size int) ([]byte, error) { return s.backend.Read(addr, size) }

// Write writes target memory.
func (s *Session) Write(addr uint64, data []byte) error { return s.backend.Write(addr, data) }

// Continue resumes the target.
func (s *Session) Continue() error { return s.backend.Continue() }

// Step single-steps the target.
func (s *Session) Step() error { return s.backend.Step() }

// Call invokes a function in the target with typed arguments. It returns
// ErrNotSupported when the backend cannot call into the target.
func (s *Session) Call(fn uint64, args []CallArg) (CallResult, error) {
	fc, ok := s.backend.(FloatCaller)
	if !ok {
		return CallResult{}, ErrNotSupported
	}
	return fc.CallWithArgs(fn, args)
}

// Wait blocks until the target stops.
func (s *Session) Wait() (StopReason, error) { return s.backend.Wait() }

// SetBreakpoint installs a software breakpoint.
func (s *Session) SetBreakpoint(addr uint64) error { return s.backend.SetBreakpoint(addr) }

// ClearBreakpoint removes a software breakpoint.
func (s *Session) ClearBreakpoint(addr uint64) error { return s.backend.ClearBreakpoint(addr) }

// SetWatchpoint arms a hardware watchpoint.
func (s *Session) SetWatchpoint(addr uint64, size int, writeOnly bool) error {
	if s.watch == nil {
		return ErrNotSupported
	}
	return s.watch.SetWatchpoint(addr, size, writeOnly)
}

// ClearWatchpoint disarms a hardware watchpoint.
func (s *Session) ClearWatchpoint(addr uint64) error {
	if s.watch == nil {
		return ErrNotSupported
	}
	return s.watch.ClearWatchpoint(addr)
}

// Hit describes one watchpoint stop.
type Hit struct {
	Address     uint64
	RIP         uint64
	Registers   Registers
	Instruction string
	Bytes       []byte
}

// Watch reports up to maxHits stops of addr, attaching and detaching around
// the loop. It blocks, so run it on its own goroutine; close stop to cancel.
func (s *Session) Watch(addr uint64, size int, writeOnly bool, maxHits int, stop <-chan struct{}, report func(Hit)) error {
	if s.watch == nil {
		return ErrNotSupported
	}
	if err := s.backend.Attach(); err != nil {
		return err
	}
	if err := s.watch.SetWatchpoint(addr, size, writeOnly); err != nil {
		_ = s.backend.Detach()
		return err
	}
	defer func() {
		_ = s.watch.ClearWatchpoint(addr)
		_ = s.backend.Detach()
	}()

	hits := 0
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		if err := s.backend.Continue(); err != nil {
			return err
		}
		reason, err := s.backend.Wait()
		if err != nil {
			return err
		}
		if reason.Event != EventStopped {
			return nil
		}
		if !reason.HasHardware {
			continue
		}
		regs, err := s.backend.Registers()
		if err != nil {
			return err
		}
		raw, _ := s.backend.Read(regs.RIP, 16)
		hit := Hit{Address: addr, RIP: regs.RIP, Registers: regs, Bytes: raw}
		if len(raw) > 0 {
			if ins := asm.Disassemble(raw, regs.RIP); len(ins) > 0 {
				hit.Instruction = ins[0].Text
			}
		}
		report(hit)
		hits++
		if maxHits > 0 && hits >= maxHits {
			return nil
		}
		_ = s.watch.ClearHardwareStatus()
		if err := s.backend.Step(); err != nil {
			return err
		}
		if _, err := s.backend.Wait(); err != nil {
			return err
		}
	}
}
