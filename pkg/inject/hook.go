// Package inject implements inline trampoline hooking: it overwrites the
// prologue of a target function with a jump into a code cave holding the
// replacement code, and provides a trampoline that runs the relocated original
// instructions before jumping back.
package inject

import (
	"errors"
	"fmt"

	"golang.org/x/arch/x86/x86asm"
	"golang.org/x/sys/unix"

	"github.com/LCRERGO/firstspark/pkg/debugger"
)

// ErrUnrelocatable is returned when the target prologue contains instructions
// that cannot be moved to a different address.
var ErrUnrelocatable = errors.New("inject: target prologue contains rip-relative or relative branch instructions")

const (
	relJumpLen = 5  // E9 rel32
	absJumpLen = 14 // FF 25 00000000 <addr64>
)

// allocator is implemented by backends that can map executable memory in the
// target process.
type allocator interface {
	Mmap(length uint64, prot, flags int) (uint64, error)
	Mprotect(addr, length uint64, prot int) error
}

// Hook is an installed inline hook.
type Hook struct {
	be         debugger.Backend
	alloc      allocator
	target     uint64
	original   []byte
	size       int
	cave       uint64
	caveSize   uint64
	trampoline uint64
	installed  bool
}

// Target returns the hooked address.
func (h *Hook) Target() uint64 { return h.target }

// Cave returns the address of the code cave.
func (h *Hook) Cave() uint64 { return h.cave }

// Trampoline returns the address of the relocated original instructions.
// Calling it invokes the original function body.
func (h *Hook) Trampoline() uint64 { return h.trampoline }

// Original returns the overwritten prologue bytes.
func (h *Hook) Original() []byte { return h.original }

// PlanSize returns the number of bytes that must be overwritten at the start
// of code so that a jump of minLen bytes fits. It fails if any of the covered
// instructions cannot be relocated.
func PlanSize(code []byte, minLen int) (int, error) {
	off := 0
	for off < minLen {
		if off >= len(code) {
			return 0, errors.New("inject: not enough bytes to overwrite")
		}
		inst, err := x86asm.Decode(code[off:], 64)
		if err != nil || inst.Len == 0 || off+inst.Len > len(code) {
			return 0, fmt.Errorf("inject: cannot decode instruction at offset %d", off)
		}
		if !relocatable(inst) {
			return 0, ErrUnrelocatable
		}
		off += inst.Len
	}
	return off, nil
}

func relocatable(inst x86asm.Inst) bool {
	for _, a := range inst.Args {
		switch v := a.(type) {
		case x86asm.Mem:
			if v.Base == x86asm.RIP {
				return false
			}
		case x86asm.Rel:
			return false
		}
	}
	return true
}

// Install overwrites target with a jump to handler. handler must eventually
// transfer control back to the instruction after the overwritten prologue,
// typically via Hook.Trampoline. The code cave is mapped read/write/execute.
func Install(be debugger.Backend, target uint64, handler []byte) (*Hook, error) {
	alloc, ok := be.(allocator)
	if !ok {
		return nil, errors.New("inject: backend cannot allocate executable memory")
	}

	probe, err := be.Read(target, 64)
	if err != nil && len(probe) < absJumpLen {
		return nil, fmt.Errorf("inject: read target prologue: %w", err)
	}

	size, err := PlanSize(probe, relJumpLen)
	if err != nil {
		return nil, err
	}

	caveSize := uint64(len(handler) + 4*absJumpLen + 16)
	cave, err := alloc.Mmap(caveSize, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		return nil, fmt.Errorf("inject: map code cave: %w", err)
	}

	jumpToCave, err := encodeRelJump(target, cave)
	if err != nil {
		// The cave is out of rel32 range; overwrite more bytes for an
		// absolute jump.
		size, err = PlanSize(probe, absJumpLen)
		if err != nil {
			return nil, err
		}
		jumpToCave = encodeAbsJump(cave)
	}

	original := append([]byte{}, probe[:size]...)

	// Code cave layout: handler | jump back to target+size.
	handlerAddr := cave
	jumpBack, err := encodeJump(handlerAddr+uint64(len(handler)), target+uint64(size))
	if err != nil {
		return nil, err
	}
	cavePayload := append(append([]byte{}, handler...), jumpBack...)

	// Trampoline layout: relocated original | jump back to target+size.
	tramp := alignUp(cave+uint64(len(cavePayload)), 16)
	trampJump, err := encodeJump(tramp+uint64(len(original)), target+uint64(size))
	if err != nil {
		return nil, err
	}
	trampPayload := append(append([]byte{}, original...), trampJump...)

	if err := be.Write(cave, cavePayload); err != nil {
		return nil, fmt.Errorf("inject: write code cave: %w", err)
	}
	if err := be.Write(tramp, trampPayload); err != nil {
		return nil, fmt.Errorf("inject: write trampoline: %w", err)
	}

	patch := padTo(jumpToCave, size)
	if err := writeText(alloc, be, target, patch); err != nil {
		return nil, fmt.Errorf("inject: patch target: %w", err)
	}

	return &Hook{
		be:         be,
		alloc:      alloc,
		target:     target,
		original:   original,
		size:       size,
		cave:       cave,
		caveSize:   caveSize,
		trampoline: tramp,
		installed:  true,
	}, nil
}

// WriteCave writes data at an offset inside the code cave. It is used to
// patch placeholders (such as a trampoline address) after installation.
func (h *Hook) WriteCave(offset int, data []byte) error {
	if offset < 0 || uint64(offset+len(data)) > h.caveSize {
		return fmt.Errorf("inject: cave write out of range")
	}
	return h.be.Write(h.cave+uint64(offset), data)
}

// Remove restores the original prologue.
func (h *Hook) Remove() error {
	if !h.installed {
		return nil
	}
	if err := writeText(h.alloc, h.be, h.target, h.original); err != nil {
		return err
	}
	h.installed = false
	return nil
}

func writeText(alloc allocator, be debugger.Backend, addr uint64, data []byte) error {
	if err := be.Write(addr, data); err == nil {
		return nil
	}
	page := addr &^ 0xFFF
	if err := alloc.Mprotect(page, 0x1000, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
		return err
	}
	if err := be.Write(addr, data); err != nil {
		return err
	}
	return alloc.Mprotect(page, 0x1000, unix.PROT_READ|unix.PROT_EXEC)
}

// encodeRelJump builds a 5-byte relative jump from `from` to `to`. It fails
// when the target is outside the rel32 range, so callers can plan for an
// absolute jump instead.
func encodeRelJump(from, to uint64) ([]byte, error) {
	rel := int64(to) - int64(from+relJumpLen)
	if rel < -1<<31 || rel >= 1<<31 {
		return nil, errors.New("inject: jump target out of relative range")
	}
	out := []byte{0xE9}
	out = append(out, byte(rel), byte(rel>>8), byte(rel>>16), byte(rel>>24))
	return out, nil
}

// encodeAbsJump builds a 14-byte absolute indirect jump to `to`.
func encodeAbsJump(to uint64) []byte {
	out := []byte{0xFF, 0x25, 0x00, 0x00, 0x00, 0x00}
	for i := 0; i < 8; i++ {
		out = append(out, byte(to>>(8*i)))
	}
	return out
}

// encodeJump builds a jump from address `from` to address `to`, preferring a
// 5-byte relative jump and falling back to a 14-byte absolute jump.
func encodeJump(from, to uint64) ([]byte, error) {
	if b, err := encodeRelJump(from, to); err == nil {
		return b, nil
	}
	return encodeAbsJump(to), nil
}

func padTo(b []byte, size int) []byte {
	if len(b) >= size {
		return b
	}
	out := append([]byte{}, b...)
	for len(out) < size {
		out = append(out, 0x90)
	}
	return out
}

func alignUp(v, align uint64) uint64 {
	if align == 0 {
		return v
	}
	return (v + align - 1) &^ (align - 1)
}
