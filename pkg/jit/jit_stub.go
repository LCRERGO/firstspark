//go:build !cgo || !linux || !amd64

package jit

import "fmt"

// Program is unavailable without cgo on linux/amd64.
type Program struct{}

// New reports that the JIT is unavailable.
func New(int, int) (*Program, error) {
	return nil, fmt.Errorf("jit: not supported in this build")
}

// Base is a stub.
func (p *Program) Base() uintptr { return 0 }

// Entry is a stub.
func (p *Program) Entry(int) uintptr { return 0 }

// Load is a stub.
func (p *Program) Load([]byte) error { return fmt.Errorf("jit: not supported in this build") }

// Seal is a stub.
func (p *Program) Seal() error { return fmt.Errorf("jit: not supported in this build") }

// SetData is a stub.
func (p *Program) SetData([]byte) {}

// DataPtr is a stub.
func (p *Program) DataPtr() uintptr { return 0 }

// DataCopy is a stub.
func (p *Program) DataCopy(int) []byte { return nil }

// PtrOff is a stub.
func (p *Program) PtrOff(int) uintptr { return 0 }

// SetBytesOff is a stub.
func (p *Program) SetBytesOff(int, []byte) {}

// CopyOff is a stub.
func (p *Program) CopyOff(int, int) []byte { return nil }

// Call is a stub.
func (p *Program) Call(uintptr, ...uintptr) uintptr { return 0 }

// Err is a stub.
func (p *Program) Err() error { return nil }

// Close is a stub.
func (p *Program) Close() {}
