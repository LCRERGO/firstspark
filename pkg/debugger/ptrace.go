//go:build linux && amd64

package debugger

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

type ptraceBackend struct {
	pid         int
	proc        *mem.Process
	attached    bool
	breakpoints map[uint64]byte
	watchpoints map[uint64]int
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

func (b *ptraceBackend) PID() int { return b.pid }

func (b *ptraceBackend) Attach() error {
	if b.attached {
		return nil
	}
	if err := unix.PtraceAttach(b.pid); err != nil {
		return translatePtrace(err)
	}
	var ws unix.WaitStatus
	if _, err := unix.Wait4(b.pid, &ws, 0, nil); err != nil {
		return fmt.Errorf("debugger: wait after attach: %w", err)
	}
	b.attached = true
	return nil
}

func (b *ptraceBackend) Detach() error {
	if !b.attached {
		return nil
	}
	for addr := range b.breakpoints {
		_ = b.ClearBreakpoint(addr)
	}
	for addr := range b.watchpoints {
		_ = b.ClearWatchpoint(addr)
	}
	if err := unix.PtraceDetach(b.pid); err != nil {
		return translatePtrace(err)
	}
	b.attached = false
	return nil
}

func (b *ptraceBackend) Read(addr uint64, size int) ([]byte, error) {
	return b.proc.Read(addr, size)
}

func (b *ptraceBackend) Write(addr uint64, data []byte) error {
	return b.proc.Write(addr, data)
}

func (b *ptraceBackend) Registers() (Registers, error) {
	var r unix.PtraceRegs
	if err := unix.PtraceGetRegs(b.pid, &r); err != nil {
		return Registers{}, translatePtrace(err)
	}
	return Registers{
		RIP: r.Rip, RSP: r.Rsp, RBP: r.Rbp,
		RAX: r.Rax, RBX: r.Rbx, RCX: r.Rcx,
		RDX: r.Rdx, RSI: r.Rsi, RDI: r.Rdi,
		R8: r.R8, R9: r.R9, R10: r.R10,
		R11: r.R11, R12: r.R12, R13: r.R13,
		R14: r.R14, R15: r.R15, RFLAGS: r.Eflags,
	}, nil
}

func (b *ptraceBackend) SetRegisters(reg Registers) error {
	r := unix.PtraceRegs{
		Rip: reg.RIP, Rsp: reg.RSP, Rbp: reg.RBP,
		Rax: reg.RAX, Rbx: reg.RBX, Rcx: reg.RCX,
		Rdx: reg.RDX, Rsi: reg.RSI, Rdi: reg.RDI,
		R8: reg.R8, R9: reg.R9, R10: reg.R10,
		R11: reg.R11, R12: reg.R12, R13: reg.R13,
		R14: reg.R14, R15: reg.R15, Eflags: reg.RFLAGS,
	}
	if err := unix.PtraceSetRegs(b.pid, &r); err != nil {
		return translatePtrace(err)
	}
	return nil
}

func (b *ptraceBackend) SetBreakpoint(addr uint64) error {
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
	page := addr &^ 0xFFF
	if err := b.Mprotect(page, 0x1000, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
		return fmt.Errorf("debugger: make page writable: %w", err)
	}
	if err := b.proc.Write(addr, data); err != nil {
		return err
	}
	return b.Mprotect(page, 0x1000, unix.PROT_READ|unix.PROT_EXEC)
}

func (b *ptraceBackend) Step() error {
	return translatePtrace(unix.PtraceSingleStep(b.pid))
}

func (b *ptraceBackend) Continue() error {
	return translatePtrace(unix.PtraceCont(b.pid, 0))
}

func (b *ptraceBackend) Wait() (StopReason, error) {
	var ws unix.WaitStatus
	_, err := unix.Wait4(b.pid, &ws, 0, nil)
	if err != nil {
		return StopReason{}, fmt.Errorf("debugger: wait: %w", err)
	}
	switch {
	case ws.Exited():
		return StopReason{Event: EventExited, ExitCode: ws.ExitStatus()}, nil
	case ws.Signaled():
		return StopReason{Event: EventSignaled, Signal: ws.Signal()}, nil
	}
	reason := StopReason{Event: EventStopped, Signal: ws.StopSignal()}
	if regs, err := b.Registers(); err == nil {
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
	return reason, nil
}

func (b *ptraceBackend) Close() error { return b.Detach() }

// RemoteSyscall executes a single syscall inside the traced process. The
// process must be attached and stopped. It returns the value left in RAX.
func (b *ptraceBackend) RemoteSyscall(num uint64, args [6]uint64) (uint64, error) {
	if !b.attached {
		return 0, ErrNotAttached
	}
	gadget, err := b.findSyscallGadget()
	if err != nil {
		return 0, err
	}
	saved, err := b.Registers()
	if err != nil {
		return 0, err
	}
	call := saved
	call.RAX = num
	call.RDI, call.RSI, call.RDX = args[0], args[1], args[2]
	call.R10, call.R8, call.R9 = args[3], args[4], args[5]
	call.RIP = gadget
	if err := b.SetRegisters(call); err != nil {
		return 0, err
	}
	if err := b.Step(); err != nil {
		return 0, err
	}
	if _, err := b.Wait(); err != nil {
		return 0, err
	}
	after, err := b.Registers()
	if err != nil {
		return 0, err
	}
	if err := b.SetRegisters(saved); err != nil {
		return 0, err
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

// Mmap maps memory in the traced process and returns the resulting address.
func (b *ptraceBackend) Mmap(length uint64, prot, flags int) (uint64, error) {
	addr, err := b.RemoteSyscall(unix.SYS_MMAP, [6]uint64{0, length, uint64(prot), uint64(flags), ^uint64(0), 0})
	if err != nil {
		return 0, err
	}
	return addr, nil
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
