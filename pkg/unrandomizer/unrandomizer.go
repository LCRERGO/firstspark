// Package unrandomizer replaces a process's libc random functions with a
// constant so randomised values become predictable while scanning.
package unrandomizer

import (
	"fmt"

	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/inject"
	"github.com/LCRERGO/firstspark/pkg/speedhack"
)

// DefaultSymbols are the libc entry points replaced by Hook.
var DefaultSymbols = []string{
	"rand",
	"random",
	"rand_r",
}

// Hook installs a handler for symbol that returns value on every call.
func Hook(be debugger.Backend, pid int, symbol string, value uint64) (*inject.Hook, error) {
	addr, err := speedhack.ResolveSymbol(pid, symbol)
	if err != nil {
		return nil, err
	}
	code, err := buildHandler(value)
	if err != nil {
		return nil, err
	}
	return inject.Install(be, addr, code)
}

// buildHandler assembles a `mov rax, value; ret` routine.
func buildHandler(value uint64) ([]byte, error) {
	program := fmt.Sprintf("mov rax, 0x%x\nret", uint32(value))
	code, _, err := asm.AssembleProgram(program, 0)
	if err != nil {
		return nil, fmt.Errorf("unrandomizer: assemble handler: %w", err)
	}
	return code, nil
}
