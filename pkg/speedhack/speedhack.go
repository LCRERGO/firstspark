package speedhack

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/LCRERGO/firstspark/pkg/asm"
	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/inject"
)

// DefaultSymbols are the logical time functions hooked by default. Each is
// resolved from the target's vDSO first, then from libc .dynsym.
//
// Only the lowest-level functions are hooked. glibc's nanosleep, usleep and
// sleep all funnel through clock_nanosleep (nanosleep is a thin wrapper and
// __nanosleep aliases it), so hooking clock_nanosleep alone scales every
// sleep exactly once; hooking the wrappers as well would double-scale.
var DefaultSymbols = []string{
	"clock_gettime",
	"gettimeofday",
	"time",
	"clock_nanosleep",
}

const placeholder = "0x1122334455667788"

const (
	unitNano  = 1000000000
	unitMicro = 1000000
)

// Handler is a generated replacement routine together with the offset of the
// trampoline address placeholder that must be patched after installation.
type Handler struct {
	Code       []byte
	TrampSlot  int
	ScaleRatio [2]int64
}

// BuildHandler generates a routine for symbol that scales the value it returns
// (time getters) or the duration it sleeps (sleep functions) by num/den.
func BuildHandler(symbol string, scale float64) (*Handler, error) {
	num, den, err := ratio(scale)
	if err != nil {
		return nil, err
	}
	program, err := programFor(symbol, num, den)
	if err != nil {
		return nil, err
	}
	code, _, err := asm.AssembleProgram(program, 0)
	if err != nil {
		return nil, fmt.Errorf("speedhack: assemble %s handler: %w", symbol, err)
	}
	slot := findPlaceholder(code)
	if slot < 0 {
		return nil, fmt.Errorf("speedhack: %s handler has no trampoline placeholder", symbol)
	}
	return &Handler{Code: code, TrampSlot: slot, ScaleRatio: [2]int64{num, den}}, nil
}

// programFor returns the assembly for symbol. Time getters scale their output
// by num/den; sleep functions divide the requested duration by num/den.
func programFor(symbol string, num, den int64) (string, error) {
	switch symbol {
	case "clock_gettime":
		return getterProgram("rsi", unitNano, num, den)
	case "gettimeofday":
		return getterProgram("rdi", unitMicro, num, den)
	case "time":
		return timeProgram(num, den)
	case "nanosleep":
		return nanosleepProgram("rdi", "rsi", num, den)
	case "clock_nanosleep":
		return nanosleepProgram("rdx", "rcx", num, den)
	case "usleep":
		return usleepProgram(num, den)
	case "sleep":
		return sleepProgram(num, den)
	default:
		return "", fmt.Errorf("speedhack: no handler for symbol %q", symbol)
	}
}

// getterProgram scales a { tv_sec, tv_sub } struct in place. ptrReg holds the
// struct pointer and unit is 1e9 for timespec or 1e6 for timeval.
func getterProgram(ptrReg string, unit, num, den int64) (string, error) {
	if num <= 0 || num > maxImm32 {
		return "", fmt.Errorf("speedhack: scale numerator %d does not fit an immediate", num)
	}
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
	mov r10, rax
	mov rax, [rbx+8]
	imul rax, rax, %d
	cqo
	idiv rcx
	cqo
	mov rcx, %d
	idiv rcx
	add r10, rax
	mov [rbx], r10
	mov [rbx+8], rdx
done:
	mov rax, r13
	pop r13
	pop r12
	pop rbx
	ret`, ptrReg, placeholder, num, den, num, unit)
	return program, nil
}

// timeProgram scales the time_t returned by time and stored through rdi.
func timeProgram(num, den int64) (string, error) {
	if num <= 0 || num > maxImm32 {
		return "", fmt.Errorf("speedhack: scale numerator %d does not fit an immediate", num)
	}
	program := fmt.Sprintf(`
	push rbx
	push r12
	mov rbx, rdi
	mov r12, %s
	call r12
	imul rax, rax, %d
	cqo
	mov rcx, %d
	idiv rcx
	test rbx, rbx
	jz done
	mov [rbx], rax
done:
	pop r12
	pop rbx
	ret`, placeholder, num, den)
	return program, nil
}

// nanosleepProgram scales a timespec request by num/den before the call and
// scales a non-null remaining time back up afterwards. reqReg/remReg select the
// argument registers (nanosleep: rdi/rsi, clock_nanosleep: rdx/rcx).
func nanosleepProgram(reqReg, remReg string, num, den int64) (string, error) {
	program := fmt.Sprintf(`
	push rbx
	push rbp
	push r12
	push r13
	push r14
	push r15
	mov rbx, %s
	mov r14, %s
	mov r15, [rbx]
	mov r13, [rbx+8]
	mov rax, r15
	imul rax, rax, %d
	add rax, r13
	mov rcx, %d
	imul rax, rcx
	cqo
	mov rcx, %d
	idiv rcx
	cqo
	mov rcx, %d
	idiv rcx
	mov [rbx], rax
	mov [rbx+8], rdx
	mov %s, rbx
	mov %s, r14
	mov r12, %s
	call r12
	mov rbp, rax
	mov [rbx], r15
	mov [rbx+8], r13
	test r14, r14
	jz done
	mov rax, [r14]
	mov r10, [r14+8]
	mov rcx, %d
	imul rax, rcx
	cqo
	mov rcx, %d
	idiv rcx
	mov r8, rax
	mov rax, r10
	mov rcx, %d
	imul rax, rcx
	cqo
	mov rcx, %d
	idiv rcx
	cqo
	mov rcx, %d
	idiv rcx
	add r8, rax
	mov [r14], r8
	mov [r14+8], rdx
done:
	mov rax, rbp
	pop r15
	pop r14
	pop r13
	pop r12
	pop rbp
	pop rbx
	ret`, reqReg, remReg, unitNano, den, num, unitNano,
		reqReg, remReg, placeholder, num, den, num, den, unitNano)
	return program, nil
}

// usleepProgram divides the microsecond argument by num/den.
func usleepProgram(num, den int64) (string, error) {
	program := fmt.Sprintf(`
	push r12
	mov rax, rdi
	mov rcx, %d
	imul rax, rcx
	cqo
	mov rcx, %d
	idiv rcx
	mov rdi, rax
	mov r12, %s
	call r12
	pop r12
	ret`, den, num, placeholder)
	return program, nil
}

// sleepProgram divides the seconds argument by num/den and scales an unslept
// return value back up.
func sleepProgram(num, den int64) (string, error) {
	program := fmt.Sprintf(`
	push r12
	mov rax, rdi
	mov rcx, %d
	imul rax, rcx
	cqo
	mov rcx, %d
	idiv rcx
	mov rdi, rax
	mov r12, %s
	call r12
	mov rcx, %d
	imul rax, rcx
	cqo
	mov rcx, %d
	idiv rcx
	pop r12
	ret`, den, num, placeholder, num, den)
	return program, nil
}

// Hook installs a speedhack handler for the logical symbol in the target
// process, resolving it from the vDSO when possible.
func Hook(be debugger.Backend, pid int, symbol string, scale float64) (*inject.Hook, error) {
	addr, _, err := Resolve(pid, symbol)
	if err != nil {
		return nil, err
	}
	handler, err := BuildHandler(symbol, scale)
	if err != nil {
		return nil, err
	}
	return InstallHandler(be, addr, handler)
}

// InstallHandler installs an already-built handler at addr.
func InstallHandler(be debugger.Backend, addr uint64, handler *Handler) (*inject.Hook, error) {
	h, err := inject.Install(be, addr, handler.Code)
	if err != nil {
		return nil, err
	}
	var slot [8]byte
	binary.LittleEndian.PutUint64(slot[:], h.Trampoline())
	if err := h.WriteCave(handler.TrampSlot, slot[:]); err != nil {
		_ = h.Remove()
		_ = h.Unmap(be)
		return nil, err
	}
	return h, nil
}

const maxImm32 = 1<<31 - 1

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
	want := binary.LittleEndian.Uint64([]byte{0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11})
	for i := 0; i+8 <= len(code); i++ {
		if binary.LittleEndian.Uint64(code[i:]) == want {
			return i
		}
	}
	return -1
}
