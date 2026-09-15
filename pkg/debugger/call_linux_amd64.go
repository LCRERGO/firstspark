//go:build linux && amd64

package debugger

import (
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// Call invokes fn in the target with up to six integer arguments and returns
// RAX. The target must be attached and stopped; it is left stopped with its
// original registers restored. A scratch page holds a return stub (int3) and a
// private stack, so the target's own stack is not disturbed.
func (b *ptraceBackend) Call(fn uint64, args []uint64) (uint64, error) {
	var ret uint64
	var err error
	b.do(func() { ret, err = b.call(fn, args) })
	return ret, err
}

func (b *ptraceBackend) call(fn uint64, args []uint64) (uint64, error) {
	if !b.attached {
		return 0, ErrNotAttached
	}
	if len(args) > 6 {
		return 0, fmt.Errorf("debugger: at most 6 arguments are supported")
	}
	scratch, err := b.scratchPage()
	if err != nil {
		return 0, fmt.Errorf("call: scratch page: %w", err)
	}
	stub := scratch + 0x100
	stackTop := scratch + 0x1000

	if err := b.proc.Write(stub, []byte{0xCC}); err != nil {
		return 0, fmt.Errorf("call: write stub: %w", err)
	}
	retSlot := stackTop - 8
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stub)
	if err := b.proc.Write(retSlot, buf[:]); err != nil {
		return 0, fmt.Errorf("call: write return slot: %w", err)
	}

	saved, err := b.registers()
	if err != nil {
		return 0, fmt.Errorf("call: read registers: %w", err)
	}
	call := saved
	call.RIP = fn
	call.RSP = retSlot
	// Clearing RAX avoids the kernel restarting an interrupted syscall when we
	// resume at a different instruction.
	call.RAX = 0
	if len(args) > 0 {
		call.RDI = args[0]
	}
	if len(args) > 1 {
		call.RSI = args[1]
	}
	if len(args) > 2 {
		call.RDX = args[2]
	}
	if len(args) > 3 {
		call.RCX = args[3]
	}
	if len(args) > 4 {
		call.R8 = args[4]
	}
	if len(args) > 5 {
		call.R9 = args[5]
	}
	if err := b.setRegisters(call); err != nil {
		return 0, fmt.Errorf("call: set registers: %w", err)
	}
	if check, cerr := b.registers(); cerr == nil && (check.RIP != fn || check.RSP != retSlot) {
		return 0, fmt.Errorf("call: register write did not take effect (rip=0x%x rsp=0x%x)", check.RIP, check.RSP)
	}
	if err := b.cont(); err != nil {
		return 0, fmt.Errorf("call: continue: %w", err)
	}

	reason, err := b.waitTimeout(2 * time.Second)
	if err != nil {
		b.stopOnTimeout()
		_ = b.setRegisters(saved)
		return 0, err
	}
	if reason.Event != EventStopped {
		_ = b.setRegisters(saved)
		return 0, fmt.Errorf("call: target did not return (event %d, signal %v)", reason.Event, reason.Signal)
	}
	after, rerr := b.registers()
	_ = b.setRegisters(saved)
	if rerr != nil {
		return 0, fmt.Errorf("call: read result after stop (signal %v): %w", reason.Signal, rerr)
	}
	if after.RIP != stub+1 {
		return 0, fmt.Errorf("call: stopped unexpectedly at 0x%x (signal %v)", after.RIP, reason.Signal)
	}
	return after.RAX, nil
}

func (b *ptraceBackend) scratchPage() (uint64, error) {
	if b.scratch != 0 {
		return b.scratch, nil
	}
	addr, err := b.mmap(0x1000, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		return 0, err
	}
	b.scratch = addr
	return addr, nil
}

// waitTimeout waits for a stop, giving up after d.
func (b *ptraceBackend) waitTimeout(d time.Duration) (StopReason, error) {
	deadline := time.Now().Add(d)
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(b.pid, &ws, unix.WNOHANG, nil)
		if err != nil {
			return StopReason{}, fmt.Errorf("debugger: wait: %w", err)
		}
		if pid == b.pid {
			return b.reason(ws), nil
		}
		if time.Now().After(deadline) {
			return StopReason{}, fmt.Errorf("debugger: remote call timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

// stopOnTimeout interrupts a target that failed to return.
func (b *ptraceBackend) stopOnTimeout() {
	if err := unix.Kill(b.pid, unix.SIGSTOP); err != nil {
		return
	}
	var ws unix.WaitStatus
	_, _ = unix.Wait4(b.pid, &ws, 0, nil)
}
