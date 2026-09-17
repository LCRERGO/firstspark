//go:build linux && amd64

package debugger

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"golang.org/x/sys/unix"
)

// Call invokes fn in the target with up to six integer arguments and returns
// RAX. The target must be attached and stopped; it is left stopped with its
// original registers restored. A scratch page holds a return stub (int3) and a
// private stack, so the target's own stack is not disturbed.
func (b *ptraceBackend) Call(fn uint64, args []uint64) (uint64, error) {
	cargs := make([]CallArg, len(args))
	for i, a := range args {
		cargs[i] = CallArg{Kind: ArgInt, Uint: a}
	}
	res, err := b.CallWithArgs(fn, cargs)
	return res.RAX, err
}

// CallWithArgs invokes fn with typed integer and floating point arguments
// following the System V AMD64 ABI, returning RAX and XMM0.
func (b *ptraceBackend) CallWithArgs(fn uint64, args []CallArg) (CallResult, error) {
	var res CallResult
	var err error
	b.do(func() { res, err = b.callWithArgs(fn, args) })
	return res, err
}

func (b *ptraceBackend) callWithArgs(fn uint64, args []CallArg) (CallResult, error) {
	var res CallResult
	if !b.attached {
		return res, ErrNotAttached
	}
	ngp, nxmm := 0, 0
	for _, a := range args {
		if a.Kind == ArgInt {
			ngp++
		} else {
			nxmm++
		}
	}
	if ngp > 6 {
		return res, fmt.Errorf("debugger: at most 6 integer arguments are supported")
	}
	if nxmm > 8 {
		return res, fmt.Errorf("debugger: at most 8 floating point arguments are supported")
	}

	scratch, err := b.scratchPage()
	if err != nil {
		return res, fmt.Errorf("call: scratch page: %w", err)
	}
	stub := scratch + 0x100
	stackTop := scratch + 0x1000

	if err := b.proc.Write(stub, []byte{0xCC}); err != nil {
		return res, fmt.Errorf("call: write stub: %w", err)
	}
	retSlot := stackTop - 8
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], stub)
	if err := b.proc.Write(retSlot, buf[:]); err != nil {
		return res, fmt.Errorf("call: write return slot: %w", err)
	}

	saved, err := b.registers()
	if err != nil {
		return res, fmt.Errorf("call: read registers: %w", err)
	}
	call := saved
	call.RIP = fn
	call.RSP = retSlot
	// Clearing RAX avoids the kernel restarting an interrupted syscall when we
	// resume at a different instruction.
	call.RAX = 0
	gpVals := []*uint64{&call.RDI, &call.RSI, &call.RDX, &call.RCX, &call.R8, &call.R9}
	gi := 0
	var savedXMM [8][16]byte
	var usedXMM int
	for _, a := range args {
		if a.Kind == ArgInt {
			*gpVals[gi] = a.Uint
			gi++
		}
	}
	if err := b.setRegisters(call); err != nil {
		return res, fmt.Errorf("call: set registers: %w", err)
	}
	for _, a := range args {
		if a.Kind == ArgInt {
			continue
		}
		orig, xerr := b.getXMM(usedXMM)
		if xerr != nil {
			return res, fmt.Errorf("call: read xmm%d: %w", usedXMM, xerr)
		}
		savedXMM[usedXMM] = orig
		var v [16]byte
		if a.Kind == ArgFloat {
			binary.LittleEndian.PutUint32(v[:4], math.Float32bits(float32(a.Float)))
		} else {
			binary.LittleEndian.PutUint64(v[:8], math.Float64bits(a.Float))
		}
		if xerr := b.setXMM(usedXMM, v); xerr != nil {
			return res, fmt.Errorf("call: set xmm%d: %w", usedXMM, xerr)
		}
		usedXMM++
	}
	restoreXMM := func() {
		for i := 0; i < usedXMM; i++ {
			_ = b.setXMM(i, savedXMM[i])
		}
	}
	if check, cerr := b.registers(); cerr == nil && (check.RIP != fn || check.RSP != retSlot) {
		restoreXMM()
		return res, fmt.Errorf("call: register write did not take effect (rip=0x%x rsp=0x%x)", check.RIP, check.RSP)
	}
	if err := b.cont(); err != nil {
		restoreXMM()
		return res, fmt.Errorf("call: continue: %w", err)
	}

	reason, err := b.waitTimeout(2 * time.Second)
	if err != nil {
		b.stopOnTimeout()
		_ = b.setRegisters(saved)
		restoreXMM()
		return res, err
	}
	if reason.Event != EventStopped {
		_ = b.setRegisters(saved)
		restoreXMM()
		return res, fmt.Errorf("call: target did not return (event %d, signal %v)", reason.Event, reason.Signal)
	}
	after, rerr := b.registers()
	xmm0, xerr := b.getXMM(0)
	_ = b.setRegisters(saved)
	restoreXMM()
	if rerr != nil {
		return res, fmt.Errorf("call: read result after stop (signal %v): %w", reason.Signal, rerr)
	}
	if after.RIP != stub+1 {
		return res, fmt.Errorf("call: stopped unexpectedly at 0x%x (signal %v)", after.RIP, reason.Signal)
	}
	res.RAX = after.RAX
	if xerr == nil {
		res.XMM0 = xmm0
	}
	return res, nil
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
