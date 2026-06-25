package speedhack

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/inject"
)

// DefaultSymbols are the libc entry points hooked for time scaling. Only the
// output getters are implemented; sleep functions are future work.
var DefaultSymbols = []string{
	"clock_gettime",
	"gettimeofday",
}

// funcSpec describes where the time value lives in a hooked function.
type funcSpec struct {
	ptrReg string // register holding the pointer to the time value
}

var specs = map[string]funcSpec{
	"clock_gettime": {"rsi"},
	"gettimeofday":  {"rdi"},
}

// Handler is a generated replacement routine together with the offset of the
// trampoline address placeholder that must be patched after installation.
type Handler struct {
	Code       []byte
	TrampSlot  int
	ScaleRatio [2]int64
}

// BuildHandler generates a routine that calls the trampoline (the original
// function), then multiplies the seconds and sub-second fields of the time
// value by num/den.
func BuildHandler(symbol string, scale float64) (*Handler, error) {
	spec, ok := specs[symbol]
	if !ok {
		return nil, fmt.Errorf("speedhack: no handler for symbol %q", symbol)
	}
	num, den, err := ratio(scale)
	if err != nil {
		return nil, err
	}
	placeholder := "0x1122334455667788"
	program := fmt.Sprintf(`
	push rbx
	push r12
	push r13
	mov rbx, %s
	mov r12, %s
	call r12
	mov r13, rax
	test eax, eax
	jne done
	mov rax, [rbx]
	imul rax, rax, %d
	cqo
	mov rcx, %d
	idiv rcx
	mov [rbx], rax
	mov rax, [rbx+8]
	imul rax, rax, %d
	cqo
	mov rcx, %d
	idiv rcx
	mov [rbx+8], rax
done:
	mov rax, r13
	pop r13
	pop r12
	pop rbx
	ret`, spec.ptrReg, placeholder, num, den, num, den)

	code, _, err := asm.AssembleProgram(program, 0)
	if err != nil {
		return nil, fmt.Errorf("speedhack: assemble handler: %w", err)
	}
	slot := findPlaceholder(code)
	if slot < 0 {
		return nil, fmt.Errorf("speedhack: trampoline placeholder not found")
	}
	return &Handler{Code: code, TrampSlot: slot, ScaleRatio: [2]int64{num, den}}, nil
}

// Hook installs a speedhack handler for symbol in the target process.
func Hook(be debugger.Backend, pid int, symbol string, scale float64) (*inject.Hook, error) {
	addr, err := ResolveSymbol(pid, symbol)
	if err != nil {
		return nil, err
	}
	handler, err := BuildHandler(symbol, scale)
	if err != nil {
		return nil, err
	}
	h, err := inject.Install(be, addr, handler.Code)
	if err != nil {
		return nil, err
	}
	var slot [8]byte
	binary.LittleEndian.PutUint64(slot[:], h.Trampoline())
	if err := h.WriteCave(handler.TrampSlot, slot[:]); err != nil {
		_ = h.Remove()
		return nil, err
	}
	return h, nil
}

// ratio approximates scale as a small fraction num/den.
func ratio(scale float64) (int64, int64, error) {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 0, 0, fmt.Errorf("speedhack: invalid scale %v", scale)
	}
	for den := int64(1); den <= 10000; den++ {
		num := int64(math.Round(scale * float64(den)))
		if num <= 0 {
			continue
		}
		if math.Abs(scale-float64(num)/float64(den)) < 1e-9 {
			return num, den, nil
		}
	}
	return 0, 0, fmt.Errorf("speedhack: cannot approximate scale %v", scale)
}

func findPlaceholder(code []byte) int {
	placeholder := binary.LittleEndian.Uint64([]byte{0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11})
	for i := 0; i+8 <= len(code); i++ {
		if binary.LittleEndian.Uint64(code[i:]) == placeholder {
			return i
		}
	}
	return -1
}
