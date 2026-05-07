package mem

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/sys/unix"
)

// Errors returned by cross-process memory operations.
var (
	ErrUnmapped   = errors.New("mem: address not mapped")
	ErrPermission = errors.New("mem: permission denied (check ptrace_scope / CAP_SYS_PTRACE)")
)

// maxChunk bounds a single process_vm_readv/writev call.
const maxChunk = 8 * 1024 * 1024

// Read copies size bytes from the process address space starting at addr.
// A short read caused by an unmapped page is reported as ErrUnmapped along
// with the bytes read so far.
func (p *Process) Read(addr uint64, size int) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("mem: negative size %d", size)
	}
	out := make([]byte, size)
	done := 0
	for done < size {
		n := min(size-done, maxChunk)
		got, err := p.readv(addr+uint64(done), out[done:done+n])
		if got < 0 {
			got = 0
		}
		done += got
		if err != nil {
			return out[:done], err
		}
		if got == 0 {
			break
		}
	}
	if done != size {
		return out[:done], io.ErrUnexpectedEOF
	}
	return out, nil
}

// Write copies data into the process address space at addr.
func (p *Process) Write(addr uint64, data []byte) error {
	done := 0
	for done < len(data) {
		n := min(len(data)-done, maxChunk)
		written, err := p.writev(addr+uint64(done), data[done:done+n])
		if written < 0 {
			written = 0
		}
		done += written
		if err != nil {
			return err
		}
		if written == 0 {
			break
		}
	}
	if done != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func (p *Process) readv(addr uint64, buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	local := []unix.Iovec{{Base: &buf[0], Len: uint64(len(buf))}}
	remote := []unix.RemoteIovec{{Base: uintptr(addr), Len: len(buf)}}
	n, err := unix.ProcessVMReadv(p.PID, local, remote, 0)
	if err != nil {
		return n, translate(err)
	}
	return n, nil
}

func (p *Process) writev(addr uint64, buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	local := []unix.Iovec{{Base: &buf[0], Len: uint64(len(buf))}}
	remote := []unix.RemoteIovec{{Base: uintptr(addr), Len: len(buf)}}
	n, err := unix.ProcessVMWritev(p.PID, local, remote, 0)
	if err != nil {
		return n, translate(err)
	}
	return n, nil
}

func translate(err error) error {
	switch {
	case errors.Is(err, unix.EFAULT):
		return ErrUnmapped
	case errors.Is(err, unix.EPERM):
		return ErrPermission
	default:
		return fmt.Errorf("mem: %w", err)
	}
}

// ReadUint64 reads a little-endian uint64 from addr.
func (p *Process) ReadUint64(addr uint64) (uint64, error) {
	b, err := p.Read(addr, 8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

// ReadPointer reads a native (64-bit) pointer from addr.
func (p *Process) ReadPointer(addr uint64) (uint64, error) { return p.ReadUint64(addr) }

// WriteUint64 writes a little-endian uint64 to addr.
func (p *Process) WriteUint64(addr, v uint64) error {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	return p.Write(addr, b[:])
}
