package customtype

import (
	"errors"
	"fmt"
	"strings"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

// ErrConversionUnsupported reports a Cheat Engine custom type that needs a
// conversion phase Firstspark does not implement yet (ADR 0048); the caller
// should fall back to RegisterRaw.
var ErrConversionUnsupported = errors.New("customtype: conversion not supported")

// CEConversion describes a Cheat Engine custom type's Auto Assembler
// conversion routines.
type CEConversion struct {
	Name               string
	Size               int
	Alignment          int
	CallMethod         bool
	UsesFloat          bool
	UsesString         bool
	MaxStringSize      int
	ConvertRoutine     string
	ConvertBackRoutine string
}

// RegisterCE registers a Cheat Engine custom type, applying its conversion
// through the Auto Assembler/JIT path (ADR 0048): the integer and single-precision
// float cases are supported. String and empty definitions are reported as
// ErrConversionUnsupported so the caller can register them raw.
func RegisterCE(c CEConversion) (*scan.Type, error) {
	if c.UsesString || c.MaxStringSize > 0 {
		return nil, ErrConversionUnsupported
	}
	if c.UsesFloat && c.Size != 4 {
		return nil, ErrConversionUnsupported
	}
	if strings.TrimSpace(c.ConvertRoutine) == "" {
		return nil, ErrConversionUnsupported
	}
	kind := "int"
	if c.UsesFloat {
		kind = "float"
	}
	def := Definition{
		Name:      c.Name,
		Size:      c.Size,
		Kind:      kind,
		Mode:      "aa",
		Alignment: c.Alignment,
		Script:    synthesizeCEScript(c),
	}
	return RegisterAA(def)
}

// synthesizeCEScript wraps the Cheat Engine Windows-x64 routines in a SysV shim
// so RegisterAA (RDI/RSI arguments) can call them. The Cheat Engine address
// argument is passed as 0. The routines precede the shims so the shim's `call`
// is a resolved backward reference.
func synthesizeCEScript(c CEConversion) string {
	back := strings.TrimSpace(c.ConvertBackRoutine)
	var b strings.Builder
	b.WriteString("[ENABLE]\n")
	b.WriteString("__fs_ce_read:\n")
	b.WriteString(strings.TrimSpace(c.ConvertRoutine))
	b.WriteString("\n  ret\n")
	if back != "" {
		b.WriteString("__fs_ce_write:\n")
		b.WriteString(back)
		b.WriteString("\n  ret\n")
	}
	writeCEShim(&b, "ConvertRoutine", "__fs_ce_read", c.CallMethod, false)
	if back != "" {
		writeCEShim(&b, "ConvertBackRoutine", "__fs_ce_write", c.CallMethod, true)
	}
	return b.String()
}

// writeCEShim emits a SysV entry point that maps RDI/RSI to RCX/RDX[/R8] and
// calls the Cheat Engine routine, keeping the stack 16-byte aligned.
func writeCEShim(b *strings.Builder, entry, target string, cdecl, write bool) {
	fmt.Fprintf(b, "%s:\n", entry)
	b.WriteString("  sub rsp, 8\n")
	b.WriteString("  mov rcx, rdi\n")
	switch {
	case write && cdecl:
		b.WriteString("  xor edx, edx\n")
		b.WriteString("  mov r8, rsi\n")
	case write:
		b.WriteString("  mov rdx, rsi\n")
	case cdecl:
		b.WriteString("  xor edx, edx\n")
	}
	fmt.Fprintf(b, "  call %s\n", target)
	b.WriteString("  add rsp, 8\n")
	b.WriteString("  ret\n")
}
