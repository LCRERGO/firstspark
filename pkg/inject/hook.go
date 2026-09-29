// Package inject implements inline trampoline hooking: it overwrites the
// prologue of a target function with a jump into a code cave holding the
// replacement code, and provides a trampoline that runs the relocated original
// instructions before jumping back.
package inject

import (
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/arch/x86/x86asm"
	"golang.org/x/sys/unix"

	"github.com/LCRERGO/firstspark/pkg/debugger"
)

// ErrUnrelocatable is returned when the target prologue contains an
// instruction that cannot be moved to a different address.
var ErrUnrelocatable = errors.New("inject: target prologue cannot be relocated")

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

// deallocator is implemented by backends that can release target memory.
type deallocator interface {
	Munmap(addr, length uint64) error
}

// textPoker is implemented by backends that can write to read-only or special
// mappings, such as the vDSO, where mprotect is not permitted.
type textPoker interface {
	PokeText(addr uint64, data []byte) error
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
		if !relocatable(inst, code[off:off+inst.Len]) {
			return 0, ErrUnrelocatable
		}
		off += inst.Len
	}
	return off, nil
}

// relocatable reports whether relocate can move the instruction from one
// address to another.
func relocatable(inst x86asm.Inst, raw []byte) bool {
	for _, a := range inst.Args {
		switch v := a.(type) {
		case x86asm.Mem:
			if v.Base == x86asm.RIP {
				return findDispField(inst, raw, v) >= 0
			}
		case x86asm.Rel:
			return classifyBranch(raw) != branchUnsupported
		}
	}
	return true
}

type branchKind int

const (
	branchNone branchKind = iota
	branchRel32
	branchJcc8
	branchJmp8
	branchUnsupported
)

// branchKind classifies a relative branch by its opcode.
func classifyBranch(raw []byte) branchKind {
	op, base := opcodeAt(raw)
	switch {
	case op == 0xE8 || op == 0xE9:
		return branchRel32
	case op == 0x0F && base+1 < len(raw) && raw[base+1] >= 0x80 && raw[base+1] <= 0x8F:
		return branchRel32
	case op >= 0x70 && op <= 0x7F:
		return branchJcc8
	case op == 0xEB:
		return branchJmp8
	default:
		return branchUnsupported
	}
}

// opcodeAt returns the opcode byte and its index, skipping legacy prefixes and
// a REX prefix.
func opcodeAt(raw []byte) (byte, int) {
	i := 0
	for i < len(raw) && isLegacyPrefix(raw[i]) {
		i++
	}
	if i < len(raw) && raw[i] >= 0x40 && raw[i] <= 0x4F {
		i++
	}
	if i >= len(raw) {
		return 0, i
	}
	return raw[i], i
}

func isLegacyPrefix(b byte) bool {
	switch b {
	case 0x66, 0x67, 0xF0, 0xF2, 0xF3, 0x2E, 0x36, 0x3E, 0x26, 0x64, 0x65:
		return true
	}
	return false
}

// findDispField locates the 4-byte displacement of a RIP-relative memory
// operand. It searches for the encoded displacement so instructions with a
// trailing immediate are handled too; ambiguity means the instruction is
// refused.
func findDispField(inst x86asm.Inst, raw []byte, m x86asm.Mem) int {
	want := uint32(m.Disp)
	found := -1
	for i := 0; i+4 <= inst.Len; i++ {
		if binary.LittleEndian.Uint32(raw[i:]) == want {
			if found >= 0 {
				return -1
			}
			found = i
		}
	}
	return found
}

// relocate rewrites code so it keeps its meaning when moved from oldAddr to
// newAddr: RIP-relative displacements and relative branches are adjusted, and
// rel8 branches are widened to rel32.
func relocate(code []byte, oldAddr, newAddr uint64) ([]byte, error) {
	out := make([]byte, 0, len(code))
	off := 0
	for off < len(code) {
		inst, err := x86asm.Decode(code[off:], 64)
		if err != nil || inst.Len == 0 || off+inst.Len > len(code) {
			return nil, fmt.Errorf("inject: cannot decode instruction at offset %d", off)
		}
		raw := code[off : off+inst.Len]
		dst := newAddr + uint64(len(out))
		moved, err := relocateInst(inst, raw, oldAddr+uint64(off), dst)
		if err != nil {
			return nil, err
		}
		out = append(out, moved...)
		off += inst.Len
	}
	return out, nil
}

func relocateInst(inst x86asm.Inst, raw []byte, oldAddr, newAddr uint64) ([]byte, error) {
	for _, a := range inst.Args {
		if _, ok := a.(x86asm.Rel); ok {
			return relocateRel(inst, raw, oldAddr, newAddr)
		}
	}
	for _, a := range inst.Args {
		if m, ok := a.(x86asm.Mem); ok && m.Base == x86asm.RIP {
			return relocateRIP(inst, raw, m, oldAddr, newAddr)
		}
	}
	return append([]byte(nil), raw...), nil
}

func relocateRIP(inst x86asm.Inst, raw []byte, m x86asm.Mem, oldAddr, newAddr uint64) ([]byte, error) {
	field := findDispField(inst, raw, m)
	if field < 0 {
		return nil, ErrUnrelocatable
	}
	oldDisp := int64(int32(binary.LittleEndian.Uint32(raw[field:])))
	target := int64(oldAddr) + int64(inst.Len) + oldDisp
	newDisp := target - (int64(newAddr) + int64(inst.Len))
	if !fitsRel32(newDisp) {
		return nil, ErrUnrelocatable
	}
	out := append([]byte(nil), raw...)
	binary.LittleEndian.PutUint32(out[field:], uint32(int32(newDisp)))
	return out, nil
}

func relocateRel(inst x86asm.Inst, raw []byte, oldAddr, newAddr uint64) ([]byte, error) {
	switch classifyBranch(raw) {
	case branchRel32:
		field := inst.Len - 4
		rel := int64(int32(binary.LittleEndian.Uint32(raw[field:])))
		target := int64(oldAddr) + int64(inst.Len) + rel
		newRel := target - (int64(newAddr) + int64(inst.Len))
		if !fitsRel32(newRel) {
			return nil, ErrUnrelocatable
		}
		out := append([]byte(nil), raw...)
		binary.LittleEndian.PutUint32(out[field:], uint32(int32(newRel)))
		return out, nil
	case branchJcc8:
		op, _ := opcodeAt(raw)
		target := int64(oldAddr) + int64(inst.Len) + int64(int8(raw[inst.Len-1]))
		newRel := target - (int64(newAddr) + 6)
		if !fitsRel32(newRel) {
			return nil, ErrUnrelocatable
		}
		return rel32Bytes([]byte{0x0F, 0x80 | (op & 0x0F)}, newRel), nil
	case branchJmp8:
		target := int64(oldAddr) + int64(inst.Len) + int64(int8(raw[inst.Len-1]))
		newRel := target - (int64(newAddr) + 5)
		if !fitsRel32(newRel) {
			return nil, ErrUnrelocatable
		}
		return rel32Bytes([]byte{0xE9}, newRel), nil
	default:
		return nil, ErrUnrelocatable
	}
}

func rel32Bytes(op []byte, rel int64) []byte {
	out := append([]byte(nil), op...)
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(int32(rel)))
	return append(out, b[:]...)
}

func fitsRel32(v int64) bool { return v >= -1<<31 && v <= 1<<31-1 }

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
	installed := false
	defer func() {
		if installed {
			return
		}
		if d, ok := be.(deallocator); ok {
			_ = d.Munmap(cave, caveSize)
		}
	}()

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
	relocated, err := relocate(original, target, tramp)
	if err != nil {
		return nil, err
	}
	trampJump, err := encodeJump(tramp+uint64(len(relocated)), target+uint64(size))
	if err != nil {
		return nil, err
	}
	trampPayload := append(append([]byte{}, relocated...), trampJump...)

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
	installed = true

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

// Remove restores the original prologue using the backend captured at install.
func (h *Hook) Remove() error {
	return h.RemoveWith(h.be)
}

// RemoveWith restores the original prologue using a live backend. The manager
// uses it when the install-time backend is no longer attached.
func (h *Hook) RemoveWith(be debugger.Backend) error {
	if !h.installed {
		return nil
	}
	alloc := h.alloc
	if a, ok := be.(allocator); ok {
		alloc = a
	}
	if err := writeText(alloc, be, h.target, h.original); err != nil {
		return err
	}
	h.installed = false
	return nil
}

// Unmap releases the code cave using be. It is best-effort: backends without
// memory release support are ignored.
func (h *Hook) Unmap(be debugger.Backend) error {
	d, ok := be.(deallocator)
	if !ok {
		return nil
	}
	return d.Munmap(h.cave, h.caveSize)
}

// CloseWith restores the original prologue and releases the code cave using a
// live backend.
func (h *Hook) CloseWith(be debugger.Backend) error {
	removeErr := h.RemoveWith(be)
	unmapErr := h.Unmap(be)
	if removeErr != nil {
		return removeErr
	}
	return unmapErr
}

func writeText(alloc allocator, be debugger.Backend, addr uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if err := be.Write(addr, data); err == nil {
		return nil
	}
	if p, ok := be.(textPoker); ok {
		if err := p.PokeText(addr, data); err == nil {
			return nil
		}
	}
	start := addr &^ 0xFFF
	end := (addr + uint64(len(data)) + 0xFFF) &^ 0xFFF
	if err := alloc.Mprotect(start, end-start, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
		return err
	}
	if err := be.Write(addr, data); err != nil {
		return err
	}
	return alloc.Mprotect(start, end-start, unix.PROT_READ|unix.PROT_EXEC)
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
