//go:build linux && amd64

package debugger

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/LCRERGO/firstspark/pkg/log"
	"github.com/LCRERGO/firstspark/pkg/mem"
)

// ptrace operations are performed by the specific thread that attached to the
// tracee, and Go migrates goroutines between OS threads. Every operation is
// therefore funnelled through a single worker goroutine pinned to one OS
// thread, which owns the ptrace relationship.
type ptraceBackend struct {
	pid         int
	proc        *mem.Process
	attached    bool
	breakpoints map[uint64]byte
	watchpoints map[uint64]int
	scratch     uint64

	mu            sync.Mutex
	ops           chan func()
	workerStarted bool
	workerStop    chan struct{}
	closed        bool
}

// NewPtrace returns a ptrace based backend for pid.
func NewPtrace(pid int) (Backend, error) {
	p, err := mem.Find(pid)
	if err != nil {
		return nil, err
	}
	return &ptraceBackend{
		pid:         pid,
		proc:        p,
		breakpoints: map[uint64]byte{},
		watchpoints: map[uint64]int{},
	}, nil
}

// do runs f on the dedicated ptrace worker thread.
func (b *ptraceBackend) do(f func()) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	if !b.workerStarted {
		b.ops = make(chan func())
		b.workerStop = make(chan struct{})
		b.workerStarted = true
		go b.worker()
	}
	ops := b.ops
	b.mu.Unlock()

	done := make(chan struct{})
	select {
	case ops <- func() {
		defer close(done)
		f()
	}:
		<-done
	case <-b.workerStop:
	}
}

func (b *ptraceBackend) worker() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for {
		select {
		case f := <-b.ops:
			f()
		case <-b.workerStop:
			return
		}
	}
}

func (b *ptraceBackend) PID() int { return b.pid }

func (b *ptraceBackend) Attach() error {
	var err error
	b.do(func() { err = b.attach() })
	return err
}

func (b *ptraceBackend) attach() error {
	if b.attached {
		return nil
	}
	if err := unix.PtraceAttach(b.pid); err != nil {
		log.Debug("ptrace attach failed", "pid", b.pid, "err", err)
		return translatePtrace(err)
	}
	var ws unix.WaitStatus
	if _, err := unix.Wait4(b.pid, &ws, 0, nil); err != nil {
		log.Debug("ptrace wait after attach failed", "pid", b.pid, "err", err)
		return fmt.Errorf("debugger: wait after attach: %w", err)
	}
	b.attached = true
	log.Debug("ptrace attached", "pid", b.pid)
	return nil
}

func (b *ptraceBackend) Detach() error {
	var err error
	b.do(func() { err = b.detach() })
	return err
}

func (b *ptraceBackend) detach() error {
	if !b.attached {
		return nil
	}
	for addr := range b.breakpoints {
		if err := b.clearBreakpoint(addr); err != nil {
			log.Debug("clearing breakpoint on detach failed", "pid", b.pid, "addr", addr, "err", err)
		}
	}
	for addr := range b.watchpoints {
		if err := b.clearWatchpoint(addr); err != nil {
			log.Debug("clearing watchpoint on detach failed", "pid", b.pid, "addr", addr, "err", err)
		}
	}
	if err := unix.PtraceDetach(b.pid); err != nil {
		log.Debug("ptrace detach failed", "pid", b.pid, "err", err)
		return translatePtrace(err)
	}
	b.attached = false
	log.Debug("ptrace detached", "pid", b.pid)
	return nil
}

// Read reads target memory (process_vm_readv, not ptrace).
func (b *ptraceBackend) Read(addr uint64, size int) ([]byte, error) {
	return b.proc.Read(addr, size)
}

// Write writes target memory (process_vm_writev, not ptrace).
func (b *ptraceBackend) Write(addr uint64, data []byte) error {
	return b.proc.Write(addr, data)
}

func (b *ptraceBackend) Registers() (Registers, error) {
	var regs Registers
	var err error
	b.do(func() { regs, err = b.registers() })
	return regs, err
}

// registers reads the general purpose registers through PTRACE_PEEKUSER.
func (b *ptraceBackend) registers() (Registers, error) {
	rip, err := b.peekUser(userRegRIP)
	if err != nil {
		return Registers{}, err
	}
	get := func(off uintptr) uint64 {
		v, _ := b.peekUser(off)
		return v
	}
	return Registers{
		RIP: rip,
		RSP: get(userRegRSP), RBP: get(userRegRBP),
		RAX: get(userRegRAX), RBX: get(userRegRBX), RCX: get(userRegRCX),
		RDX: get(userRegRDX), RSI: get(userRegRSI), RDI: get(userRegRDI),
		R8: get(userRegR8), R9: get(userRegR9), R10: get(userRegR10),
		R11: get(userRegR11), R12: get(userRegR12), R13: get(userRegR13),
		R14: get(userRegR14), R15: get(userRegR15), RFLAGS: get(userRegRFLAGS),
	}, nil
}

func (b *ptraceBackend) SetRegisters(reg Registers) error {
	var err error
	b.do(func() { err = b.setRegisters(reg) })
	return err
}

// setRegisters writes the general purpose registers through PTRACE_POKEUSER.
func (b *ptraceBackend) setRegisters(reg Registers) error {
	writes := []struct {
		off uintptr
		val uint64
	}{
		{userRegR15, reg.R15}, {userRegR14, reg.R14}, {userRegR13, reg.R13}, {userRegR12, reg.R12},
		{userRegRBP, reg.RBP}, {userRegRBX, reg.RBX}, {userRegR11, reg.R11}, {userRegR10, reg.R10},
		{userRegR9, reg.R9}, {userRegR8, reg.R8}, {userRegRAX, reg.RAX}, {userRegRCX, reg.RCX},
		{userRegRDX, reg.RDX}, {userRegRSI, reg.RSI}, {userRegRDI, reg.RDI},
		{userRegRIP, reg.RIP}, {userRegRFLAGS, reg.RFLAGS}, {userRegRSP, reg.RSP},
	}
	for _, w := range writes {
		if err := b.pokeUser(w.off, w.val); err != nil {
			return err
		}
	}
	return nil
}

func (b *ptraceBackend) SetBreakpoint(addr uint64) error {
	var err error
	b.do(func() { err = b.setBreakpoint(addr) })
	return err
}

func (b *ptraceBackend) setBreakpoint(addr uint64) error {
	if _, ok := b.breakpoints[addr]; ok {
		return nil
	}
	orig, err := b.proc.Read(addr, 1)
	if err != nil {
		return err
	}
	if err := b.writeText(addr, []byte{0xCC}); err != nil {
		return err
	}
	b.breakpoints[addr] = orig[0]
	return nil
}

func (b *ptraceBackend) ClearBreakpoint(addr uint64) error {
	var err error
	b.do(func() { err = b.clearBreakpoint(addr) })
	return err
}

func (b *ptraceBackend) clearBreakpoint(addr uint64) error {
	orig, ok := b.breakpoints[addr]
	if !ok {
		return nil
	}
	if err := b.writeText(addr, []byte{orig}); err != nil {
		return err
	}
	delete(b.breakpoints, addr)
	return nil
}

// writeText writes to a possibly read-only page by temporarily making it
// writable with a remote mprotect.
func (b *ptraceBackend) writeText(addr uint64, data []byte) error {
	if err := b.proc.Write(addr, data); err == nil {
		return nil
	}
	if err := b.PokeText(addr, data); err == nil {
		return nil
	}
	page := addr &^ 0xFFF
	if err := b.mprotect(page, 0x1000, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
		return fmt.Errorf("debugger: make page writable: %w", err)
	}
	if err := b.proc.Write(addr, data); err != nil {
		return err
	}
	return b.mprotect(page, 0x1000, unix.PROT_READ|unix.PROT_EXEC)
}

// PokeText writes to read-only or special mappings (such as the vDSO) with
// PTRACE_POKEDATA, which the kernel permits where mprotect does not.
func (b *ptraceBackend) PokeText(addr uint64, data []byte) error {
	var err error
	b.do(func() { err = b.pokeText(addr, data) })
	return err
}

func (b *ptraceBackend) pokeText(addr uint64, data []byte) error {
	if !b.attached {
		return ErrNotAttached
	}
	if len(data) == 0 {
		return nil
	}
	n, err := unix.PtracePokeData(b.pid, uintptr(addr), data)
	if err != nil {
		return fmt.Errorf("debugger: poke %#x: %w", addr, err)
	}
	if n != len(data) {
		return fmt.Errorf("debugger: poke %#x: short write %d/%d", addr, n, len(data))
	}
	return nil
}

func (b *ptraceBackend) Step() error {
	var err error
	b.do(func() { err = b.step() })
	return err
}

func (b *ptraceBackend) step() error {
	return translatePtrace(unix.PtraceSingleStep(b.pid))
}

func (b *ptraceBackend) Continue() error {
	var err error
	b.do(func() { err = b.cont() })
	return err
}

func (b *ptraceBackend) cont() error {
	return translatePtrace(unix.PtraceCont(b.pid, 0))
}

func (b *ptraceBackend) Wait() (StopReason, error) {
	var reason StopReason
	var err error
	b.do(func() { reason, err = b.wait() })
	return reason, err
}

func (b *ptraceBackend) wait() (StopReason, error) {
	var ws unix.WaitStatus
	_, err := unix.Wait4(b.pid, &ws, 0, nil)
	if err != nil {
		return StopReason{}, fmt.Errorf("debugger: wait: %w", err)
	}
	return b.reason(ws), nil
}

func (b *ptraceBackend) reason(ws unix.WaitStatus) StopReason {
	switch {
	case ws.Exited():
		return StopReason{Event: EventExited, ExitCode: ws.ExitStatus()}
	case ws.Signaled():
		return StopReason{Event: EventSignaled, Signal: ws.Signal()}
	}
	reason := StopReason{Event: EventStopped, Signal: ws.StopSignal()}
	if regs, err := b.registers(); err == nil {
		addr := regs.RIP - 1
		if _, ok := b.breakpoints[addr]; ok {
			reason.BreakpointAddr = addr
			reason.HasBreakpoint = true
		}
	}
	if slot, ok := b.hardwareSlot(); ok {
		reason.HardwareSlot = slot
		reason.HasHardware = true
	}
	return reason
}

// Close detaches and stops the dedicated ptrace worker so its goroutine and
// locked OS thread are released.
func (b *ptraceBackend) Close() error {
	err := b.Detach()
	b.mu.Lock()
	if b.workerStarted {
		close(b.workerStop)
		b.workerStarted = false
	}
	b.closed = true
	b.mu.Unlock()
	return err
}

// RemoteSyscall executes a single syscall inside the traced process. The
// process must be attached and stopped. It returns the value left in RAX.
func (b *ptraceBackend) RemoteSyscall(num uint64, args [6]uint64) (uint64, error) {
	var ret uint64
	var err error
	b.do(func() { ret, err = b.remoteSyscall(num, args) })
	return ret, err
}

func (b *ptraceBackend) remoteSyscall(num uint64, args [6]uint64) (uint64, error) {
	if !b.attached {
		return 0, ErrNotAttached
	}
	gadget, err := b.findSyscallGadget()
	if err != nil {
		return 0, fmt.Errorf("remote syscall: %w", err)
	}
	saved, err := b.registers()
	if err != nil {
		return 0, fmt.Errorf("remote syscall: read registers: %w", err)
	}
	call := saved
	call.RAX = num
	call.RDI, call.RSI, call.RDX = args[0], args[1], args[2]
	call.R10, call.R8, call.R9 = args[3], args[4], args[5]
	call.RIP = gadget
	if err := b.setRegisters(call); err != nil {
		return 0, fmt.Errorf("remote syscall: set registers: %w", err)
	}
	if err := b.step(); err != nil {
		return 0, fmt.Errorf("remote syscall: step: %w", err)
	}
	if _, err := b.wait(); err != nil {
		return 0, fmt.Errorf("remote syscall: wait: %w", err)
	}
	after, err := b.registers()
	if err != nil {
		return 0, fmt.Errorf("remote syscall: read result: %w", err)
	}
	if err := b.setRegisters(saved); err != nil {
		return 0, fmt.Errorf("remote syscall: restore registers: %w", err)
	}
	if int64(after.RAX) < 0 && after.RAX >= 0xFFFFFFFFFFFFF000 {
		return after.RAX, fmt.Errorf("debugger: remote syscall %d failed: %w", num, syscall.Errno(-int64(after.RAX)))
	}
	return after.RAX, nil
}

// Mprotect changes the protection of a memory range in the traced process.
func (b *ptraceBackend) Mprotect(addr, length uint64, prot int) error {
	_, err := b.RemoteSyscall(unix.SYS_MPROTECT, [6]uint64{addr, length, uint64(prot), 0, 0, 0})
	return err
}

func (b *ptraceBackend) mprotect(addr, length uint64, prot int) error {
	_, err := b.remoteSyscall(unix.SYS_MPROTECT, [6]uint64{addr, length, uint64(prot), 0, 0, 0})
	return err
}

// Mmap maps memory in the traced process and returns the resulting address.
func (b *ptraceBackend) Mmap(length uint64, prot, flags int) (uint64, error) {
	var addr uint64
	var err error
	b.do(func() { addr, err = b.mmap(length, prot, flags) })
	return addr, err
}

func (b *ptraceBackend) mmap(length uint64, prot, flags int) (uint64, error) {
	addr, err := b.remoteSyscall(unix.SYS_MMAP, [6]uint64{0, length, uint64(prot), uint64(flags), ^uint64(0), 0})
	if err != nil {
		return 0, err
	}
	return addr, nil
}

// Munmap releases a mapping in the traced process.
func (b *ptraceBackend) Munmap(addr, length uint64) error {
	_, err := b.RemoteSyscall(unix.SYS_MUNMAP, [6]uint64{addr, length, 0, 0, 0, 0})
	return err
}

func (b *ptraceBackend) findSyscallGadget() (uint64, error) {
	regions, err := mem.Regions(b.pid)
	if err != nil {
		return 0, err
	}
	for _, r := range regions {
		if !r.Executable() || r.Size() == 0 {
			continue
		}
		data, err := b.proc.Read(r.Start, int(min(r.Size(), 1<<20)))
		if err != nil && len(data) == 0 {
			continue
		}
		for i := 0; i+1 < len(data); i++ {
			if data[i] == 0x0F && data[i+1] == 0x05 {
				return r.Start + uint64(i), nil
			}
		}
	}
	return 0, errors.New("debugger: no syscall instruction found in target")
}

func translatePtrace(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.ESRCH):
		return ErrNoSuchProcess
	case errors.Is(err, unix.EPERM):
		return mem.ErrPermission
	default:
		return fmt.Errorf("debugger: %w", err)
	}
}
