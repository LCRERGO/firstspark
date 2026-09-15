//go:build linux && amd64

package debugger

import (
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
)

// Offsets of the debug registers within the x86-64 struct user.
const (
	dr0Offset = 848
	dr6Offset = 896
	dr7Offset = 904
)

func drOffset(slot int) uintptr { return dr0Offset + uintptr(slot)*8 }

// SetWatchpoint arms one of the four hardware watchpoints on addr.
func (b *ptraceBackend) SetWatchpoint(addr uint64, size int, writeOnly bool) error {
	if !b.attached {
		return ErrNotAttached
	}
	dr7, err := b.peekUser(dr7Offset)
	if err != nil {
		return err
	}
	slot := -1
	for i := 0; i < 4; i++ {
		if dr7&(1<<(2*i)) == 0 {
			slot = i
			break
		}
	}
	if slot < 0 {
		return errors.New("debugger: all hardware watchpoints are in use")
	}
	if err := b.pokeUser(drOffset(slot), addr); err != nil {
		return err
	}
	rw := uint64(1) // write only
	if !writeOnly {
		rw = 3 // read or write
	}
	dr7 &^= uint64(0xF) << (16 + 4*slot)
	dr7 |= (rw << (16 + 4*slot)) | (lengthBits(size) << (18 + 4*slot))
	dr7 |= 1 << (2 * slot)
	if err := b.pokeUser(dr7Offset, dr7); err != nil {
		return err
	}
	b.watchpoints[addr] = slot
	return nil
}

// ClearWatchpoint disarms the watchpoint on addr.
func (b *ptraceBackend) ClearWatchpoint(addr uint64) error {
	slot, ok := b.watchpoints[addr]
	if !ok {
		return nil
	}
	dr7, err := b.peekUser(dr7Offset)
	if err != nil {
		return err
	}
	dr7 &^= 1 << (2 * slot)
	dr7 &^= uint64(0xF) << (16 + 4*slot)
	if err := b.pokeUser(dr7Offset, dr7); err != nil {
		return err
	}
	_ = b.pokeUser(drOffset(slot), 0)
	delete(b.watchpoints, addr)
	return nil
}

// ClearHardwareStatus clears the sticky debug status register.
func (b *ptraceBackend) ClearHardwareStatus() error {
	return b.pokeUser(dr6Offset, 0)
}

func (b *ptraceBackend) hardwareSlot() (int, bool) {
	dr6, err := b.peekUser(dr6Offset)
	if err != nil {
		return 0, false
	}
	for i := 0; i < 4; i++ {
		if dr6&(1<<i) != 0 {
			return i, true
		}
	}
	return 0, false
}

func (b *ptraceBackend) peekUser(offset uintptr) (uint64, error) {
	var buf [8]byte
	if _, err := unix.PtracePeekUser(b.pid, offset, buf[:]); err != nil {
		return 0, translatePtrace(err)
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

func (b *ptraceBackend) pokeUser(offset uintptr, value uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	if _, err := unix.PtracePokeUser(b.pid, offset, buf[:]); err != nil {
		return translatePtrace(err)
	}
	return nil
}

// lengthBits encodes a watchpoint length for the DR7 LEN field.
func lengthBits(size int) uint64 {
	switch size {
	case 2:
		return 1
	case 8:
		return 2
	case 4:
		return 3
	default:
		return 0
	}
}
