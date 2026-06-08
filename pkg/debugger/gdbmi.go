package debugger

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
)

// gdbmiBackend is a placeholder for a GDB/MI backend. It is wired through the
// same Backend interface so it can be dropped in at runtime, but the MI
// protocol implementation is future work.
type gdbmiBackend struct {
	pid     int
	gdbPath string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
}

// NewGDBMI returns a gdbmi backend. It currently reports ErrNotSupported for
// every operation beyond construction.
func NewGDBMI(pid int, opts Options) (Backend, error) {
	path := opts.GDBPath
	if path == "" {
		path = "gdb"
	}
	if _, err := exec.LookPath(path); err != nil {
		return nil, fmt.Errorf("debugger: gdbmi backend: %w", err)
	}
	return &gdbmiBackend{pid: pid, gdbPath: path}, nil
}

func (b *gdbmiBackend) PID() int { return b.pid }

func (b *gdbmiBackend) Attach() error { return ErrNotSupported }
func (b *gdbmiBackend) Detach() error { return ErrNotSupported }

func (b *gdbmiBackend) Read(uint64, int) ([]byte, error) { return nil, ErrNotSupported }
func (b *gdbmiBackend) Write(uint64, []byte) error       { return ErrNotSupported }

func (b *gdbmiBackend) Registers() (Registers, error) { return Registers{}, ErrNotSupported }
func (b *gdbmiBackend) SetRegisters(Registers) error  { return ErrNotSupported }
func (b *gdbmiBackend) SetBreakpoint(uint64) error    { return ErrNotSupported }
func (b *gdbmiBackend) ClearBreakpoint(uint64) error  { return ErrNotSupported }
func (b *gdbmiBackend) Step() error                   { return ErrNotSupported }
func (b *gdbmiBackend) Continue() error               { return ErrNotSupported }
func (b *gdbmiBackend) Wait() (StopReason, error)     { return StopReason{}, ErrNotSupported }
func (b *gdbmiBackend) Close() error                  { return nil }
