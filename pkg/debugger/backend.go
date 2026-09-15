// Package debugger defines a pluggable debugging backend. The default
// implementation is a pure-Go ptrace backend; a gdbmi backend can be plugged
// in at runtime without changing callers.
package debugger

import (
	"errors"
	"fmt"
	"syscall"
)

// Backend errors.
var (
	ErrNotSupported  = errors.New("debugger: operation not supported by backend")
	ErrNotAttached   = errors.New("debugger: not attached")
	ErrNoSuchProcess = errors.New("debugger: no such process")
)

// Event describes why a Wait returned.
type Event int

const (
	EventStopped Event = iota
	EventExited
	EventSignaled
)

// StopReason reports the outcome of a Wait.
type StopReason struct {
	Event          Event
	Signal         syscall.Signal
	ExitCode       int
	BreakpointAddr uint64
	HasBreakpoint  bool
	HardwareSlot   int
	HasHardware    bool
}

// Registers is a portable snapshot of the general purpose registers.
type Registers struct {
	RIP, RSP, RBP uint64
	RAX, RBX, RCX uint64
	RDX, RSI, RDI uint64
	R8, R9, R10   uint64
	R11, R12, R13 uint64
	R14, R15      uint64
	RFLAGS        uint64
}

// Backend is the seam between the application and a concrete debugger.
type Backend interface {
	PID() int
	Attach() error
	Detach() error

	Read(addr uint64, size int) ([]byte, error)
	Write(addr uint64, data []byte) error

	Registers() (Registers, error)
	SetRegisters(r Registers) error

	SetBreakpoint(addr uint64) error
	ClearBreakpoint(addr uint64) error

	Step() error
	Continue() error
	Wait() (StopReason, error)

	Close() error
}

// WatchpointBackend is implemented by backends that support hardware data
// watchpoints (the x86 debug registers).
type WatchpointBackend interface {
	// SetWatchpoint arms a watchpoint on addr for size bytes. When writeOnly
	// is false, reads are watched as well.
	SetWatchpoint(addr uint64, size int, writeOnly bool) error
	// ClearWatchpoint disarms the watchpoint on addr.
	ClearWatchpoint(addr uint64) error
	// ClearHardwareStatus clears the sticky debug status register so the next
	// trap is reported.
	ClearHardwareStatus() error
}

// Options configures backend construction.
type Options struct {
	// GDBPath is the gdb executable used by the gdbmi backend.
	GDBPath string
}

// New constructs a backend by name. The empty string selects ptrace.
func New(name string, pid int, opts Options) (Backend, error) {
	switch name {
	case "", "ptrace":
		return NewPtrace(pid)
	case "gdbmi":
		return NewGDBMI(pid, opts)
	default:
		return nil, fmt.Errorf("debugger: unknown backend %q", name)
	}
}
