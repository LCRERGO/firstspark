//go:build cgo && linux && amd64

// Package jit loads position-independent machine code into the current
// process and calls it with the System V AMD64 calling convention. It backs
// Auto-Assembler-defined custom types (ADR 0023).
package jit

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>

// jit_call6 invokes a function pointer with up to six integer arguments and
// returns RAX, preserving the callee-saved registers across the call.
extern uintptr_t jit_call6(uintptr_t fn, uintptr_t a0, uintptr_t a1,
	uintptr_t a2, uintptr_t a3, uintptr_t a4, uintptr_t a5);

__asm__(
".text\n"
".globl jit_call6\n"
".type jit_call6, @function\n"
"jit_call6:\n"
"  push %rbx\n"
"  push %rbp\n"
"  push %r12\n"
"  push %r13\n"
"  push %r14\n"
"  push %r15\n"
"  sub $8, %rsp\n"
"  mov %rdi, %rax\n"
"  mov %rsi, %rdi\n"
"  mov %rdx, %rsi\n"
"  mov %rcx, %rdx\n"
"  mov %r8, %rcx\n"
"  mov %r9, %r8\n"
"  mov 64(%rsp), %r9\n"
"  call *%rax\n"
"  add $8, %rsp\n"
"  pop %r15\n"
"  pop %r14\n"
"  pop %r13\n"
"  pop %r12\n"
"  pop %rbp\n"
"  pop %rbx\n"
"  ret\n"
".size jit_call6, .-jit_call6\n"
);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Program is executable code loaded into this process together with a scratch
// data buffer.
type Program struct {
	mem    unsafe.Pointer
	size   int
	sealed bool
	buf    unsafe.Pointer
	buflen int
}

// New reserves a code region of codeSize bytes and a data buffer of bufLen
// bytes. The code region is writable until Seal is called.
func New(codeSize, bufLen int) (*Program, error) {
	if codeSize <= 0 {
		return nil, fmt.Errorf("jit: code size must be positive")
	}
	if bufLen <= 0 {
		bufLen = 64
	}
	mem, merr := C.mmap(nil, C.size_t(codeSize), C.PROT_READ|C.PROT_WRITE|C.PROT_EXEC,
		C.MAP_PRIVATE|C.MAP_ANONYMOUS, -1, 0)
	if merr != nil || mem == C.MAP_FAILED {
		return nil, fmt.Errorf("jit: mmap failed")
	}
	buf := C.malloc(C.size_t(bufLen))
	if buf == nil {
		C.munmap(mem, C.size_t(codeSize))
		return nil, fmt.Errorf("jit: malloc failed")
	}
	return &Program{mem: mem, size: codeSize, buf: buf, buflen: bufLen}, nil
}

// Base returns the address the code was loaded at.
func (p *Program) Base() uintptr { return uintptr(p.mem) }

// Entry returns the callable address at an offset into the code region.
func (p *Program) Entry(offset int) uintptr { return uintptr(p.mem) + uintptr(offset) }

// Load copies code into the region.
func (p *Program) Load(code []byte) error {
	if len(code) > p.size {
		return fmt.Errorf("jit: code of %d bytes exceeds region of %d", len(code), p.size)
	}
	dst := unsafe.Slice((*byte)(p.mem), p.size)
	copy(dst, code)
	return nil
}

// Seal makes the code region read-execute only.
func (p *Program) Seal() error {
	if p.sealed {
		return nil
	}
	if C.mprotect(p.mem, C.size_t(p.size), C.PROT_READ|C.PROT_EXEC) != 0 {
		return fmt.Errorf("jit: mprotect failed")
	}
	p.sealed = true
	return nil
}

// SetData copies b into the data buffer.
func (p *Program) SetData(b []byte) {
	n := len(b)
	if n > p.buflen {
		n = p.buflen
	}
	if n == 0 {
		return
	}
	C.memcpy(p.buf, unsafe.Pointer(&b[0]), C.size_t(n))
}

// DataPtr returns the address of the data buffer.
func (p *Program) DataPtr() uintptr { return uintptr(p.buf) }

// DataCopy returns a copy of the first n bytes of the data buffer.
func (p *Program) DataCopy(n int) []byte {
	if n > p.buflen {
		n = p.buflen
	}
	out := make([]byte, n)
	if n > 0 {
		C.memcpy(unsafe.Pointer(&out[0]), p.buf, C.size_t(n))
	}
	return out
}

// Call invokes entry with up to six integer arguments and returns RAX.
func (p *Program) Call(entry uintptr, args ...uintptr) uintptr {
	var a [6]C.uintptr_t
	for i := 0; i < len(args) && i < 6; i++ {
		a[i] = C.uintptr_t(args[i])
	}
	return uintptr(C.jit_call6(C.uintptr_t(entry), a[0], a[1], a[2], a[3], a[4], a[5]))
}

// Close releases the code and data regions.
func (p *Program) Close() {
	if p.buf != nil {
		C.free(p.buf)
		p.buf = nil
	}
	if p.mem != nil {
		C.munmap(p.mem, C.size_t(p.size))
		p.mem = nil
	}
}
