// Package aaexec interprets the Auto Assembler subset used by custom-type
// conversion routines so the CGO-free headless build can apply conversions
// without pkg/jit (ADR 0048 P4). It is a bounded x86-64 interpreter: any
// unsupported instruction or addressing form fails Compile rather than
// executing incorrectly.
package aaexec

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/LCRERGO/firstspark/pkg/autoasm"
)

const (
	opReg = iota
	opImm
	opMem
	opLabel
)

const baseAddr = 0x1000_0000

// maxSteps bounds execution to catch runaway loops.
const maxSteps = 1_000_000

// supportedOps is the instruction set Compile accepts.
var supportedOps = func() map[string]bool {
	ops := []string{
		"nop", "ret", "call", "jmp", "mov", "movzx", "movsx", "lea",
		"push", "pop", "add", "sub", "and", "or", "xor", "cmp", "test",
		"inc", "dec", "neg", "not", "imul", "shl", "shr", "sar",
		"cdq", "cqo", "idiv", "div",
	}
	m := map[string]bool{}
	for _, op := range ops {
		m[op] = true
	}
	for op := range jccConditions {
		m[op] = true
	}
	return m
}()

type operand struct {
	kind  int
	reg   int
	size  int
	imm   int64
	base  int
	index int
	scale int
	disp  int64
	label string
}

type instruction struct {
	label string // non-empty for a label line
	op    string
	args  []operand
	raw   string
}

// Program is a compiled conversion routine.
type Program struct {
	code           []instruction
	labels         map[string]int
	entries        map[string]uintptr
	mem            []byte
	regs           [16]uint64
	zf, sf, of, cf bool
	pc             int
	calls          []int
	done           bool
	err            error

	valueSize int
	textOff   int
	stackTop  uint64
}

// Compile builds a program from an [ENABLE] section's items. It returns entry
// points for the labels the caller is interested in.
func Compile(items []autoasm.Item, memSize, valueSize int) (*Program, error) {
	if memSize <= 0 {
		memSize = valueSize + 4096
	}
	p := &Program{
		labels:    map[string]int{},
		entries:   map[string]uintptr{},
		mem:       make([]byte, memSize),
		valueSize: valueSize,
		stackTop:  baseAddr + uint64(memSize),
	}
	for _, it := range items {
		switch it.Kind {
		case autoasm.KindLabel:
			p.labels[strings.ToLower(it.Name)] = len(p.code)
			p.code = append(p.code, instruction{label: it.Name})
		case autoasm.KindInstruction:
			in, err := parseInstruction(it.Text)
			if err != nil {
				return nil, err
			}
			if !supportedOps[in.op] {
				return nil, fmt.Errorf("aaexec: unsupported instruction %q", strings.TrimSpace(it.Text))
			}
			p.code = append(p.code, in)
		case autoasm.KindAlloc, autoasm.KindDealloc, autoasm.KindDefine,
			autoasm.KindRegisterSymbol, autoasm.KindUnregisterSymbol:
			// metadata, ignored
		default:
			return nil, fmt.Errorf("aaexec: unsupported directive on line %d", it.Line)
		}
	}
	for name, pc := range p.labels {
		p.entries[name] = uintptr(pc + 1) // 1-based token, distinct from memory
	}
	return p, nil
}

// Entry returns the call token for a label.
func (p *Program) Entry(name string) (uintptr, bool) {
	e, ok := p.entries[strings.ToLower(name)]
	return e, ok
}

// PtrOff returns the address of an offset into the conversion buffer.
func (p *Program) PtrOff(off int) uintptr { return baseAddr + uintptr(off) }

// SetData copies b into the value region at offset 0.
func (p *Program) SetData(b []byte) { p.SetBytesOff(0, b) }

// SetBytesOff copies b into the buffer at off.
func (p *Program) SetBytesOff(off int, b []byte) {
	if off < 0 || off > len(p.mem) {
		return
	}
	copy(p.mem[off:], b)
}

// DataPtr returns the value region address.
func (p *Program) DataPtr() uintptr { return baseAddr }

// DataCopy returns a copy of the first n bytes.
func (p *Program) DataCopy(n int) []byte { return p.CopyOff(0, n) }

// CopyOff returns a copy of n bytes at off.
func (p *Program) CopyOff(off, n int) []byte {
	if off < 0 || n <= 0 || off >= len(p.mem) {
		return nil
	}
	end := off + n
	if end > len(p.mem) {
		end = len(p.mem)
	}
	out := make([]byte, end-off)
	copy(out, p.mem[off:end])
	return out
}

// Err reports the last execution error.
func (p *Program) Err() error { return p.err }

// Close releases the program.
func (p *Program) Close() { p.mem = nil; p.code = nil }

// Call runs the routine at entry with up to six SysV arguments and returns RAX.
func (p *Program) Call(entry uintptr, args ...uintptr) uintptr {
	p.reset()
	if entry == 0 || int(entry) > len(p.code) {
		p.err = fmt.Errorf("aaexec: bad entry %d", entry)
		return 0
	}
	p.regs[7] = uint64(argsAt(args, 0)) // RDI
	p.regs[6] = uint64(argsAt(args, 1)) // RSI
	p.regs[2] = uint64(argsAt(args, 2)) // RDX
	p.regs[1] = uint64(argsAt(args, 3)) // RCX
	p.regs[8] = uint64(argsAt(args, 4)) // R8
	p.regs[9] = uint64(argsAt(args, 5)) // R9
	p.regs[4] = p.stackTop              // RSP
	p.pc = int(entry) - 1
	for steps := 0; p.pc >= 0 && p.pc < len(p.code); steps++ {
		if steps > maxSteps {
			p.err = fmt.Errorf("aaexec: step limit exceeded")
			return 0
		}
		in := p.code[p.pc]
		if in.label != "" {
			p.pc++
			continue
		}
		if err := p.exec(in); err != nil {
			p.err = err
			return 0
		}
		if p.done {
			break
		}
	}
	return uintptr(p.regs[0])
}

func (p *Program) reset() {
	p.regs = [16]uint64{}
	p.zf, p.sf, p.of, p.cf = false, false, false, false
	p.pc = 0
	p.calls = p.calls[:0]
	p.done = false
	p.err = nil
}

func argsAt(args []uintptr, i int) uintptr {
	if i < len(args) {
		return args[i]
	}
	return 0
}

func (p *Program) exec(in instruction) error {
	switch in.op {
	case "nop":
		p.pc++
	case "ret":
		if len(p.calls) == 0 {
			p.done = true
			return nil
		}
		p.pc = p.calls[len(p.calls)-1]
		p.calls = p.calls[:len(p.calls)-1]
	case "call":
		p.calls = append(p.calls, p.pc+1)
		return p.jump(in.args[0])
	case "jmp":
		return p.jump(in.args[0])
	case "mov":
		return p.execMov(in.args)
	case "movzx", "movsx":
		return p.execExtend(in.op, in.args)
	case "lea":
		addr, err := p.effAddr(in.args[1])
		if err != nil {
			return err
		}
		return p.set(in.args[0], in.args[0].size, addr)
	case "push":
		return p.execPush(in.args[0])
	case "pop":
		return p.execPop(in.args[0])
	case "add", "sub", "and", "or", "xor", "cmp", "test":
		return p.execArith(in.op, in.args)
	case "inc", "dec", "neg", "not":
		return p.execUnary(in.op, in.args)
	case "imul":
		return p.execImul(in.args)
	case "shl", "shr", "sar":
		return p.execShift(in.op, in.args)
	case "cdq":
		if (p.regs[0]>>31)&1 == 1 {
			p.regs[2] = 0xffffffff
		} else {
			p.regs[2] = 0
		}
		p.pc++
	case "cqo":
		if (p.regs[0]>>63)&1 == 1 {
			p.regs[2] = math.MaxUint64
		} else {
			p.regs[2] = 0
		}
		p.pc++
	case "idiv", "div":
		return p.execDiv(in.op, in.args[0])
	default:
		if cond, ok := jccConditions[in.op]; ok {
			if cond(p) {
				return p.jump(in.args[0])
			}
			p.pc++
			return nil
		}
		return fmt.Errorf("aaexec: unsupported instruction %q", in.raw)
	}
	return nil
}

func (p *Program) jump(target operand) error {
	pc, ok := p.labels[strings.ToLower(target.label)]
	if !ok {
		return fmt.Errorf("aaexec: unknown label %q", target.label)
	}
	p.pc = pc
	return nil
}

func (p *Program) execMov(args []operand) error {
	if len(args) != 2 {
		return fmt.Errorf("aaexec: mov needs two operands")
	}
	dst, src := args[0], args[1]
	size := p.sizeOf(dst, src)
	v, err := p.value(src, size)
	if err != nil {
		return err
	}
	if err := p.set(dst, size, v); err != nil {
		return err
	}
	p.pc++
	return nil
}

func (p *Program) execExtend(op string, args []operand) error {
	if len(args) != 2 {
		return fmt.Errorf("aaexec: %s needs two operands", op)
	}
	dst, src := args[0], args[1]
	v, err := p.value(src, src.size)
	if err != nil {
		return err
	}
	if op == "movsx" {
		v = signExtend(v, src.size)
	}
	if err := p.set(dst, dst.size, v); err != nil {
		return err
	}
	p.pc++
	return nil
}

func (p *Program) execPush(o operand) error {
	v, err := p.value(o, 8)
	if err != nil {
		return err
	}
	p.regs[4] -= 8
	if err := p.writeMem(p.regs[4], 8, v); err != nil {
		return err
	}
	p.pc++
	return nil
}

func (p *Program) execPop(o operand) error {
	v, err := p.readMem(p.regs[4], 8)
	if err != nil {
		return err
	}
	p.regs[4] += 8
	if err := p.set(o, 8, v); err != nil {
		return err
	}
	p.pc++
	return nil
}

func (p *Program) execArith(op string, args []operand) error {
	if len(args) != 2 {
		return fmt.Errorf("aaexec: %s needs two operands", op)
	}
	dst, src := args[0], args[1]
	size := p.sizeOf(dst, src)
	w := uint(size * 8)
	mask := maskFor(size)
	a, err := p.value(dst, size)
	if err != nil {
		return err
	}
	b, err := p.value(src, size)
	if err != nil {
		return err
	}
	var r uint64
	switch op {
	case "add":
		r = (a + b) & mask
		p.cf = r < a
		p.setFlags(r, size)
		p.of = ((a^r)&(b^r))>>(w-1) == 1
	case "sub", "cmp":
		r = (a - b) & mask
		p.cf = a < b
		p.setFlags(r, size)
		p.of = ((a^b)&(a^r))>>(w-1) == 1
	case "and", "test":
		r = (a & b) & mask
		p.cf, p.of = false, false
		p.setFlags(r, size)
	case "or":
		r = (a | b) & mask
		p.cf, p.of = false, false
		p.setFlags(r, size)
	case "xor":
		r = (a ^ b) & mask
		p.cf, p.of = false, false
		p.setFlags(r, size)
	}
	if op != "cmp" && op != "test" {
		if err := p.set(dst, size, r); err != nil {
			return err
		}
	}
	p.pc++
	return nil
}

func (p *Program) execUnary(op string, args []operand) error {
	if len(args) != 1 {
		return fmt.Errorf("aaexec: %s needs one operand", op)
	}
	dst := args[0]
	size := dst.size
	mask := maskFor(size)
	a, err := p.value(dst, size)
	if err != nil {
		return err
	}
	var r uint64
	switch op {
	case "inc":
		r = (a + 1) & mask
		p.setFlags(r, size)
	case "dec":
		r = (a - 1) & mask
		p.setFlags(r, size)
	case "neg":
		r = (-a) & mask
		p.setFlags(r, size)
		p.cf = a != 0
	case "not":
		r = (^a) & mask
	}
	if err := p.set(dst, size, r); err != nil {
		return err
	}
	p.pc++
	return nil
}

func (p *Program) execImul(args []operand) error {
	switch len(args) {
	case 2:
		size := p.sizeOf(args[0], args[1])
		a, err := p.value(args[0], size)
		if err != nil {
			return err
		}
		b, err := p.value(args[1], size)
		if err != nil {
			return err
		}
		return p.storeProduct(args[0], size, a*b)
	case 3:
		size := args[0].size
		b, err := p.value(args[1], size)
		if err != nil {
			return err
		}
		c, err := p.value(args[2], size)
		if err != nil {
			return err
		}
		return p.storeProduct(args[0], size, b*c)
	default:
		return fmt.Errorf("aaexec: imul needs two or three operands")
	}
}

func (p *Program) storeProduct(dst operand, size int, r uint64) error {
	mask := maskFor(size)
	r &= mask
	p.setFlags(r, size)
	if err := p.set(dst, size, r); err != nil {
		return err
	}
	p.pc++
	return nil
}

func (p *Program) execShift(op string, args []operand) error {
	if len(args) != 2 {
		return fmt.Errorf("aaexec: %s needs two operands", op)
	}
	dst, src := args[0], args[1]
	size := dst.size
	mask := maskFor(size)
	w := size * 8
	a, err := p.value(dst, size)
	if err != nil {
		return err
	}
	count, err := p.value(src, 1)
	if err != nil {
		return err
	}
	count &= uint64(w - 1)
	var r uint64
	switch op {
	case "shl":
		r = (a << count) & mask
		if count > 0 {
			p.cf = (a>>(w-int(count)))&1 == 1
		}
	case "shr":
		r = (a >> count) & mask
		if count > 0 {
			p.cf = (a>>(count-1))&1 == 1
		}
	case "sar":
		signed := int64(signExtend(a, size))
		r = uint64(signed>>count) & mask
		if count > 0 {
			p.cf = uint64(signed>>(count-1))&1 == 1
		}
	}
	p.setFlags(r, size)
	if err := p.set(dst, size, r); err != nil {
		return err
	}
	p.pc++
	return nil
}

func (p *Program) execDiv(op string, o operand) error {
	size := o.size
	divisor, err := p.value(o, size)
	if err != nil {
		return err
	}
	if divisor == 0 {
		return fmt.Errorf("aaexec: division by zero")
	}
	switch size {
	case 4:
		dividend := (uint64(p.regs[2]&0xffffffff) << 32) | (p.regs[0] & 0xffffffff)
		var q, r uint64
		if op == "idiv" {
			d := int64(int32(divisor))
			qn := int64(int32(dividend)) / d
			r = uint64(int64(int32(dividend)) % d)
			q = uint64(int32(qn))
		} else {
			q = dividend / divisor
			r = dividend % divisor
		}
		p.regs[0] = (p.regs[0] &^ 0xffffffff) | (q & 0xffffffff)
		p.regs[2] = (p.regs[2] &^ 0xffffffff) | (r & 0xffffffff)
	case 8:
		if op == "idiv" {
			d := int64(divisor)
			q := int64(p.regs[0]) / d
			r := int64(p.regs[0]) % d
			p.regs[0] = uint64(q)
			p.regs[2] = uint64(r)
		} else {
			q := p.regs[0] / divisor
			r := p.regs[0] % divisor
			p.regs[0] = q
			p.regs[2] = r
		}
	default:
		return fmt.Errorf("aaexec: %s size %d unsupported", op, size)
	}
	p.pc++
	return nil
}

func (p *Program) setFlags(r uint64, size int) {
	w := uint(size * 8)
	p.zf = r == 0
	p.sf = (r>>(w-1))&1 == 1
}

func (p *Program) value(o operand, size int) (uint64, error) {
	switch o.kind {
	case opReg:
		return p.regs[o.reg] & maskFor(o.size), nil
	case opImm:
		return uint64(o.imm) & maskFor(sizeOr(size, 8)), nil
	case opMem:
		addr, err := p.effAddr(o)
		if err != nil {
			return 0, err
		}
		return p.readMem(addr, size)
	default:
		return 0, fmt.Errorf("aaexec: invalid operand")
	}
}

func (p *Program) set(o operand, size int, v uint64) error {
	switch o.kind {
	case opReg:
		switch o.size {
		case 8:
			p.regs[o.reg] = v
		case 4:
			p.regs[o.reg] = v & 0xffffffff
		case 2:
			p.regs[o.reg] = (p.regs[o.reg] &^ 0xffff) | (v & 0xffff)
		case 1:
			p.regs[o.reg] = (p.regs[o.reg] &^ 0xff) | (v & 0xff)
		}
		return nil
	case opMem:
		addr, err := p.effAddr(o)
		if err != nil {
			return err
		}
		return p.writeMem(addr, size, v)
	default:
		return fmt.Errorf("aaexec: cannot write operand")
	}
}

func (p *Program) effAddr(o operand) (uint64, error) {
	if o.kind != opMem {
		return 0, fmt.Errorf("aaexec: not a memory operand")
	}
	addr := uint64(o.base)
	if addr != 0 {
		addr = p.regs[o.base]
	}
	if o.index >= 0 {
		addr += p.regs[o.index] * uint64(o.scale)
	}
	return uint64(int64(addr) + o.disp), nil
}

func (p *Program) readMem(addr uint64, size int) (uint64, error) {
	if size <= 0 || size > 8 {
		return 0, fmt.Errorf("aaexec: bad read size %d", size)
	}
	off := addr - baseAddr
	if addr < baseAddr || off+uint64(size) > uint64(len(p.mem)) {
		return 0, fmt.Errorf("aaexec: read outside buffer at %#x", addr)
	}
	var v uint64
	for i := 0; i < size; i++ {
		v |= uint64(p.mem[off+uint64(i)]) << (8 * uint(i))
	}
	return v, nil
}

func (p *Program) writeMem(addr uint64, size int, v uint64) error {
	if size <= 0 || size > 8 {
		return fmt.Errorf("aaexec: bad write size %d", size)
	}
	off := addr - baseAddr
	if addr < baseAddr || off+uint64(size) > uint64(len(p.mem)) {
		return fmt.Errorf("aaexec: write outside buffer at %#x", addr)
	}
	for i := 0; i < size; i++ {
		p.mem[off+uint64(i)] = byte(v >> (8 * uint(i)))
	}
	return nil
}

func (p *Program) sizeOf(a, b operand) int {
	if a.kind == opReg {
		return a.size
	}
	if a.kind == opMem && a.size > 0 {
		return a.size
	}
	if b.kind == opReg {
		return b.size
	}
	if b.kind == opMem && b.size > 0 {
		return b.size
	}
	return 4
}

func sizeOr(size, fallback int) int {
	if size > 0 {
		return size
	}
	return fallback
}

func maskFor(size int) uint64 {
	if size >= 8 || size <= 0 {
		return math.MaxUint64
	}
	return (uint64(1) << (8 * uint(size))) - 1
}

func signExtend(v uint64, size int) uint64 {
	switch size {
	case 1:
		return uint64(int64(int8(v)))
	case 2:
		return uint64(int64(int16(v)))
	case 4:
		return uint64(int64(int32(v)))
	default:
		return v
	}
}

var jccConditions = map[string]func(*Program) bool{
	"jz": func(p *Program) bool { return p.zf }, "je": func(p *Program) bool { return p.zf },
	"jnz": func(p *Program) bool { return !p.zf }, "jne": func(p *Program) bool { return !p.zf },
	"js": func(p *Program) bool { return p.sf }, "jns": func(p *Program) bool { return !p.sf },
	"jo": func(p *Program) bool { return p.of }, "jno": func(p *Program) bool { return !p.of },
	"jc": func(p *Program) bool { return p.cf }, "jb": func(p *Program) bool { return p.cf },
	"jnae": func(p *Program) bool { return p.cf },
	"jnc":  func(p *Program) bool { return !p.cf }, "jae": func(p *Program) bool { return !p.cf },
	"jnb": func(p *Program) bool { return !p.cf },
	"jbe": func(p *Program) bool { return p.cf || p.zf }, "jna": func(p *Program) bool { return p.cf || p.zf },
	"ja": func(p *Program) bool { return !p.cf && !p.zf }, "jnbe": func(p *Program) bool { return !p.cf && !p.zf },
	"jl": func(p *Program) bool { return p.sf != p.of }, "jnge": func(p *Program) bool { return p.sf != p.of },
	"jge": func(p *Program) bool { return p.sf == p.of }, "jnl": func(p *Program) bool { return p.sf == p.of },
	"jle": func(p *Program) bool { return p.zf || p.sf != p.of },
	"jg":  func(p *Program) bool { return !p.zf && p.sf == p.of },
}

// parseInstruction parses one assembly line.
func parseInstruction(text string) (instruction, error) {
	text = strings.TrimSpace(text)
	in := instruction{raw: text}
	i := 0
	for i < len(text) && !isSpace(text[i]) {
		i++
	}
	in.op = strings.ToLower(text[:i])
	rest := strings.TrimSpace(text[i:])
	if rest == "" {
		return in, nil
	}
	for _, part := range splitOperands(rest) {
		o, err := parseOperand(part)
		if err != nil {
			return instruction{}, fmt.Errorf("aaexec: %q: %w", text, err)
		}
		in.args = append(in.args, o)
	}
	return in, nil
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' }

func splitOperands(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

func parseOperand(s string) (operand, error) {
	s = strings.TrimSpace(s)
	o := operand{base: -1, index: -1}
	for {
		lower := strings.ToLower(s)
		switch {
		case strings.HasPrefix(lower, "byte ptr "):
			o.size = 1
			s = s[len("byte ptr "):]
		case strings.HasPrefix(lower, "word ptr "):
			o.size = 2
			s = s[len("word ptr "):]
		case strings.HasPrefix(lower, "dword ptr "):
			o.size = 4
			s = s[len("dword ptr "):]
		case strings.HasPrefix(lower, "qword ptr "):
			o.size = 8
			s = s[len("qword ptr "):]
		case strings.HasPrefix(lower, "byte "):
			o.size = 1
			s = s[len("byte "):]
		case strings.HasPrefix(lower, "word "):
			o.size = 2
			s = s[len("word "):]
		case strings.HasPrefix(lower, "dword "):
			o.size = 4
			s = s[len("dword "):]
		case strings.HasPrefix(lower, "qword "):
			o.size = 8
			s = s[len("qword "):]
		default:
			goto done
		}
	}
done:
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		o.kind = opMem
		return parseMem(o, s[1:len(s)-1])
	}
	if idx, size, ok := lookupReg(s); ok {
		o.kind = opReg
		o.reg = idx
		o.size = size
		return o, nil
	}
	if n, err := parseNumber(s); err == nil {
		o.kind = opImm
		o.imm = n
		o.size = sizeOr(o.size, 8)
		return o, nil
	}
	o.kind = opLabel
	o.label = s
	return o, nil
}

func parseMem(o operand, expr string) (operand, error) {
	terms := splitTerms(expr)
	for _, t := range terms {
		t = strings.TrimSpace(t)
		t = strings.TrimPrefix(t, "+")
		if t == "" {
			continue
		}
		if i := strings.IndexByte(t, '*'); i >= 0 {
			idx, _, ok := lookupReg(strings.TrimSpace(t[:i]))
			if !ok {
				return o, fmt.Errorf("aaexec: bad index %q", t)
			}
			scale, err := strconv.Atoi(strings.TrimSpace(t[i+1:]))
			if err != nil {
				return o, fmt.Errorf("aaexec: bad scale %q", t)
			}
			o.index = idx
			o.scale = scale
			continue
		}
		if idx, _, ok := lookupReg(t); ok {
			if o.base < 0 {
				o.base = idx
			} else {
				o.index = idx
				o.scale = 1
			}
			continue
		}
		if n, err := parseNumber(t); err == nil {
			o.disp += n
			continue
		}
		return o, fmt.Errorf("aaexec: bad memory term %q", t)
	}
	return o, nil
}

func splitTerms(s string) []string {
	var out []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] == '+' || s[i] == '-' {
			out = append(out, s[start:i])
			start = i
		}
	}
	out = append(out, s[start:])
	return out
}

func parseNumber(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	return strconv.ParseInt(s, 0, 64)
}

func lookupReg(name string) (int, int, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if r, ok := registers[name]; ok {
		return r.idx, r.size, true
	}
	return 0, 0, false
}

type regInfo struct {
	idx  int
	size int
}

var registers = buildRegisters()

func buildRegisters() map[string]regInfo {
	m := map[string]regInfo{
		"al": {0, 1}, "ax": {0, 2}, "eax": {0, 4}, "rax": {0, 8},
		"cl": {1, 1}, "cx": {1, 2}, "ecx": {1, 4}, "rcx": {1, 8},
		"dl": {2, 1}, "dx": {2, 2}, "edx": {2, 4}, "rdx": {2, 8},
		"bl": {3, 1}, "bx": {3, 2}, "ebx": {3, 4}, "rbx": {3, 8},
		"spl": {4, 1}, "sp": {4, 2}, "esp": {4, 4}, "rsp": {4, 8},
		"bpl": {5, 1}, "bp": {5, 2}, "ebp": {5, 4}, "rbp": {5, 8},
		"sil": {6, 1}, "si": {6, 2}, "esi": {6, 4}, "rsi": {6, 8},
		"dil": {7, 1}, "di": {7, 2}, "edi": {7, 4}, "rdi": {7, 8},
	}
	suffix := []struct {
		s string
		n int
	}{{"b", 1}, {"w", 2}, {"d", 4}, {"", 8}}
	for i := 8; i <= 15; i++ {
		base := "r" + strconv.Itoa(i)
		for _, sfx := range suffix {
			m[base+sfx.s] = regInfo{i, sfx.n}
		}
	}
	return m
}
