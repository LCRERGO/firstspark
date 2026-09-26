package asm

import (
	"fmt"
	"math"
	"strings"
)

// Assemble encodes a single Intel-syntax instruction located at virtual
// address addr. It returns the encoded bytes.
func Assemble(line string, addr uint64) ([]byte, error) {
	line = strings.TrimSpace(stripComment(line))
	if line == "" {
		return nil, nil
	}
	mnem, rest := splitMnemonic(line)
	mnem = strings.ToLower(mnem)

	var ops []operand
	if rest != "" {
		for _, tok := range splitOperands(rest) {
			if tok == "" {
				continue
			}
			op, err := parseOperand(tok)
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
		}
	}
	return encode(mnem, ops, addr)
}

// AssembleBytes encodes a multi-line program, tracking addresses as it goes.
func AssembleBytes(text string, addr uint64) ([]byte, error) {
	var out []byte
	for _, line := range strings.Split(text, "\n") {
		b, err := Assemble(line, addr+uint64(len(out)))
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

type arith struct {
	ext int
	rmR byte
	rRm byte
}

var arithOps = map[string]arith{
	"add": {0, 0x01, 0x03},
	"or":  {1, 0x09, 0x0B},
	"and": {4, 0x21, 0x23},
	"sub": {5, 0x29, 0x2B},
	"xor": {6, 0x31, 0x33},
	"cmp": {7, 0x39, 0x3B},
}

var jccCodes = map[string]byte{
	"jo": 0x0, "jno": 0x1,
	"jb": 0x2, "jc": 0x2, "jnae": 0x2,
	"jae": 0x3, "jnb": 0x3, "jnc": 0x3,
	"je": 0x4, "jz": 0x4,
	"jne": 0x5, "jnz": 0x5,
	"jbe": 0x6, "jna": 0x6,
	"ja": 0x7, "jnbe": 0x7,
	"js": 0x8, "jns": 0x9,
	"jp": 0xA, "jpe": 0xA,
	"jnp": 0xB, "jpo": 0xB,
	"jl": 0xC, "jnge": 0xC,
	"jge": 0xD, "jnl": 0xD,
	"jle": 0xE, "jng": 0xE,
	"jg": 0xF, "jnle": 0xF,
}

// simpleOpcodes are mnemonics with a fixed encoding and no operands.
var simpleOpcodes = map[string][]byte{
	"nop":     {0x90},
	"ret":     {0xC3},
	"leave":   {0xC9},
	"int3":    {0xCC},
	"syscall": {0x0F, 0x05},
	"cdq":     {0x99},
	"cqo":     {0x48, 0x99},
	"pushfq":  {0x9C},
	"popfq":   {0x9D},
}

// incDecExt maps the group-3 mnemonics to their ModRM extension.
var incDecExt = map[string]int{
	"inc": 0, "dec": 1, "not": 2, "neg": 3, "mul": 4, "div": 6, "idiv": 7,
}

// shiftExt maps the group-2 mnemonics to their ModRM extension.
var shiftExt = map[string]int{"shl": 4, "sal": 4, "shr": 5, "sar": 7}

func encode(mnem string, ops []operand, addr uint64) ([]byte, error) {
	if op, ok := simpleOpcodes[mnem]; ok {
		return append([]byte(nil), op...), nil
	}
	if a, ok := arithOps[mnem]; ok {
		return encodeArith(mnem, a, ops)
	}
	if cc, ok := jccCodes[mnem]; ok {
		return encodeJcc(mnem, cc, ops, addr)
	}
	if ext, ok := incDecExt[mnem]; ok {
		return encodeIncDec(mnem, ext, ops)
	}
	if ext, ok := shiftExt[mnem]; ok {
		return encodeShift(shiftName(mnem), ext, ops)
	}
	switch mnem {
	case "mov", "movabs":
		return encodeMov(ops)
	case "lea":
		return encodeLea(ops)
	case "push":
		return encodePush(ops)
	case "pop":
		return encodePop(ops)
	case "jmp":
		return encodeJump("jmp", ops, addr)
	case "call":
		return encodeJump("call", ops, addr)
	case "test":
		return encodeTest(ops)
	case "imul":
		return encodeImul(ops)
	case "movzx":
		return encodeExtend("movzx", 0xB6, 0xB7, ops)
	case "movsx":
		return encodeExtend("movsx", 0xBE, 0xBF, ops)
	default:
		return nil, fmt.Errorf("asm: unsupported instruction %q", mnem)
	}
}

// shiftName normalises "sal" to "shl" for the shared shift encoder.
func shiftName(mnem string) string {
	if mnem == "sal" {
		return "shl"
	}
	return mnem
}

type rmEnc struct {
	rexR, rexX, rexB bool
	forceRex         bool
	noRex            bool
	tail             []byte
}

func encodeRM(regField int, rm operand, size int) (rmEnc, error) {
	var e rmEnc
	e.rexR = regField > 7
	switch rm.kind {
	case kindReg:
		if rm.reg.high8 {
			e.noRex = true
		} else if rm.reg.size == 1 && rm.reg.code&7 >= 4 {
			e.forceRex = true
		}
		e.rexB = rm.reg.code > 7
		e.tail = []byte{0xC0 | byte((regField&7)<<3) | byte(rm.reg.code&7)}
	case kindMem:
		return encodeMem(regField, rm.mem)
	default:
		return e, fmt.Errorf("asm: expected a register or memory operand")
	}
	return e, nil
}

func encodeMem(regField int, m memOperand) (rmEnc, error) {
	var e rmEnc
	e.rexR = regField > 7
	if m.rip {
		tail := []byte{byte((regField&7)<<3) | 0x05}
		tail = append(tail, le32(int32(m.disp))...)
		e.tail = tail
		return e, nil
	}
	if m.base == nil {
		if m.index == nil {
			return e, fmt.Errorf("asm: memory operand has no register")
		}
		sib := byte(scaleBits(m.scale)<<6) | byte((m.index.code&7)<<3) | 0x05
		tail := []byte{byte((regField&7)<<3) | 0x04, sib}
		tail = append(tail, le32(int32(m.disp))...)
		e.rexX = m.index.code > 7
		e.tail = tail
		return e, nil
	}

	base := m.base
	baseLow := base.code & 7
	hasIndex := m.index != nil
	needSIB := hasIndex || baseLow == 4

	mod := 0
	if m.disp != 0 || baseLow == 5 {
		if fitsInt8(m.disp) {
			mod = 1
		} else {
			mod = 2
		}
	}

	var tail []byte
	if !needSIB {
		tail = append(tail, byte(mod<<6)|byte((regField&7)<<3)|byte(baseLow))
	} else {
		idx := byte(4)
		if hasIndex {
			idx = byte(m.index.code & 7)
			e.rexX = m.index.code > 7
		}
		sib := byte(scaleBits(m.scale)<<6) | (idx << 3) | byte(baseLow)
		tail = append(tail, byte(mod<<6)|byte((regField&7)<<3)|0x04, sib)
	}
	e.rexB = base.code > 7
	switch mod {
	case 1:
		tail = append(tail, byte(int8(m.disp)))
	case 2:
		tail = append(tail, le32(int32(m.disp))...)
	}
	e.tail = tail
	return e, nil
}

func emit(size int, e rmEnc, opcode, imm []byte) ([]byte, error) {
	var out []byte
	if size == 2 {
		out = append(out, 0x66)
	}
	rexW := size == 8
	if e.noRex && (rexW || e.rexR || e.rexX || e.rexB || e.forceRex) {
		return nil, fmt.Errorf("asm: high-byte register cannot be used with a REX prefix")
	}
	if rexW || e.rexR || e.rexX || e.rexB || e.forceRex {
		rex := byte(0x40)
		if rexW {
			rex |= 0x08
		}
		if e.rexR {
			rex |= 0x04
		}
		if e.rexX {
			rex |= 0x02
		}
		if e.rexB {
			rex |= 0x01
		}
		out = append(out, rex)
	}
	out = append(out, opcode...)
	out = append(out, e.tail...)
	out = append(out, imm...)
	return out, nil
}

func encodeArith(mnem string, a arith, ops []operand) ([]byte, error) {
	if len(ops) != 2 {
		return nil, fmt.Errorf("asm: %s expects 2 operands", mnem)
	}
	dest, src := ops[0], ops[1]
	size, err := binSize(dest, src)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, fmt.Errorf("asm: %s cannot determine operand size", mnem)
	}

	if src.kind == kindImm {
		e, err := encodeRM(a.ext, dest, size)
		if err != nil {
			return nil, err
		}
		switch {
		case size == 1:
			return emit(size, e, []byte{0x80}, []byte{byte(src.imm)})
		case fitsInt8(src.imm):
			return emit(size, e, []byte{0x83}, []byte{byte(int8(src.imm))})
		default:
			if size == 8 && !fitsInt32(src.imm) {
				return nil, fmt.Errorf("asm: %s immediate does not fit in 32 bits", mnem)
			}
			return emit(size, e, []byte{0x81}, le32(int32(src.imm)))
		}
	}

	if dest.kind == kindReg && src.kind == kindMem {
		e, err := encodeRM(dest.reg.code, src, size)
		if err != nil {
			return nil, err
		}
		return emit(size, e, []byte{a.rRm}, nil)
	}
	if src.kind != kindReg {
		return nil, fmt.Errorf("asm: %s unsupported operand combination", mnem)
	}
	e, err := encodeRM(src.reg.code, dest, size)
	if err != nil {
		return nil, err
	}
	return emit(size, e, []byte{a.rmR}, nil)
}

func encodeMov(ops []operand) ([]byte, error) {
	if len(ops) != 2 {
		return nil, fmt.Errorf("asm: mov expects 2 operands")
	}
	dest, src := ops[0], ops[1]
	size, err := binSize(dest, src)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, fmt.Errorf("asm: mov cannot determine operand size")
	}

	if src.kind == kindImm {
		if dest.kind == kindReg {
			return movRegImm(dest, size, src.imm)
		}
		e, err := encodeRM(0, dest, size)
		if err != nil {
			return nil, err
		}
		if size == 1 {
			return emit(size, e, []byte{0xC6}, []byte{byte(src.imm)})
		}
		if size == 8 && !fitsInt32(src.imm) {
			return nil, fmt.Errorf("asm: mov immediate does not fit in 32 bits")
		}
		return emit(size, e, []byte{0xC7}, le32(int32(src.imm)))
	}

	if dest.kind == kindReg && src.kind == kindMem {
		e, err := encodeRM(dest.reg.code, src, size)
		if err != nil {
			return nil, err
		}
		return emit(size, e, []byte{0x8B}, nil)
	}
	if src.kind != kindReg {
		return nil, fmt.Errorf("asm: mov unsupported operand combination")
	}
	e, err := encodeRM(src.reg.code, dest, size)
	if err != nil {
		return nil, err
	}
	return emit(size, e, []byte{0x89}, nil)
}

func movRegImm(dest operand, size int, imm int64) ([]byte, error) {
	r := dest.reg
	var out []byte
	if size == 2 {
		out = append(out, 0x66)
	}
	needRex := size == 8 || r.code > 7 || (size == 1 && r.code&7 >= 4 && !r.high8)
	if r.high8 && needRex {
		return nil, fmt.Errorf("asm: high-byte register cannot be used with a REX prefix")
	}
	if needRex {
		rex := byte(0x40)
		if size == 8 {
			rex |= 0x08
		}
		if r.code > 7 {
			rex |= 0x01
		}
		out = append(out, rex)
	}
	out = append(out, byte(0xB8+(r.code&7)))
	switch size {
	case 1:
		out = append(out, byte(imm))
	case 2:
		out = append(out, le16(uint16(imm))...)
	case 4:
		out = append(out, le32(int32(imm))...)
	case 8:
		out = append(out, le64(uint64(imm))...)
	}
	return out, nil
}

func encodeLea(ops []operand) ([]byte, error) {
	if len(ops) != 2 || ops[0].kind != kindReg || ops[1].kind != kindMem {
		return nil, fmt.Errorf("asm: lea expects a register and a memory operand")
	}
	size := ops[0].reg.size
	e, err := encodeRM(ops[0].reg.code, ops[1], size)
	if err != nil {
		return nil, err
	}
	return emit(size, e, []byte{0x8D}, nil)
}

func encodePush(ops []operand) ([]byte, error) {
	if len(ops) != 1 {
		return nil, fmt.Errorf("asm: push expects 1 operand")
	}
	op := ops[0]
	switch op.kind {
	case kindReg:
		var out []byte
		if op.reg.code > 7 {
			out = append(out, 0x41)
		}
		return append(out, byte(0x50+(op.reg.code&7))), nil
	case kindImm:
		if fitsInt8(op.imm) {
			return []byte{0x6A, byte(int8(op.imm))}, nil
		}
		return append([]byte{0x68}, le32(int32(op.imm))...), nil
	case kindMem:
		e, err := encodeRM(6, op, 8)
		if err != nil {
			return nil, err
		}
		return emit(8, e, []byte{0xFF}, nil)
	default:
		return nil, fmt.Errorf("asm: unsupported push operand")
	}
}

func encodePop(ops []operand) ([]byte, error) {
	if len(ops) != 1 {
		return nil, fmt.Errorf("asm: pop expects 1 operand")
	}
	op := ops[0]
	if op.kind == kindReg {
		var out []byte
		if op.reg.code > 7 {
			out = append(out, 0x41)
		}
		return append(out, byte(0x58+(op.reg.code&7))), nil
	}
	if op.kind == kindMem {
		e, err := encodeRM(0, op, 8)
		if err != nil {
			return nil, err
		}
		return emit(8, e, []byte{0x8F}, nil)
	}
	return nil, fmt.Errorf("asm: unsupported pop operand")
}

func encodeJump(mnem string, ops []operand, addr uint64) ([]byte, error) {
	if len(ops) != 1 {
		return nil, fmt.Errorf("asm: %s expects 1 operand", mnem)
	}
	op := ops[0]
	ext := byte(4)
	opcode := byte(0xE9)
	if mnem == "call" {
		ext = 2
		opcode = 0xE8
	}
	switch op.kind {
	case kindImm:
		rel := op.imm - int64(addr+5)
		if !fitsInt32(rel) {
			return nil, fmt.Errorf("asm: %s target out of range", mnem)
		}
		return append([]byte{opcode}, le32(int32(rel))...), nil
	case kindReg, kindMem:
		e, err := encodeRM(int(ext), op, 8)
		if err != nil {
			return nil, err
		}
		return emit(8, e, []byte{0xFF}, nil)
	default:
		return nil, fmt.Errorf("asm: unsupported %s operand", mnem)
	}
}

func encodeJcc(mnem string, cc byte, ops []operand, addr uint64) ([]byte, error) {
	if len(ops) != 1 || ops[0].kind != kindImm {
		return nil, fmt.Errorf("asm: %s expects a target address", mnem)
	}
	rel := ops[0].imm - int64(addr+6)
	if !fitsInt32(rel) {
		return nil, fmt.Errorf("asm: %s target out of range", mnem)
	}
	out := []byte{0x0F, 0x80 + cc}
	return append(out, le32(int32(rel))...), nil
}

func encodeIncDec(mnem string, ext int, ops []operand) ([]byte, error) {
	if len(ops) != 1 {
		return nil, fmt.Errorf("asm: %s expects 1 operand", mnem)
	}
	op := ops[0]
	size := operandSize(op)
	if size == 0 {
		return nil, fmt.Errorf("asm: %s requires an explicit size", mnem)
	}
	e, err := encodeRM(ext, op, size)
	if err != nil {
		return nil, err
	}
	if size == 1 {
		return emit(size, e, []byte{0xF6}, nil)
	}
	return emit(size, e, []byte{0xFF}, nil)
}

func encodeShift(mnem string, ext int, ops []operand) ([]byte, error) {
	if len(ops) != 2 || ops[1].kind != kindImm {
		return nil, fmt.Errorf("asm: %s expects a register/memory operand and an immediate count", mnem)
	}
	size := operandSize(ops[0])
	if size == 0 {
		return nil, fmt.Errorf("asm: %s requires an explicit size", mnem)
	}
	e, err := encodeRM(ext, ops[0], size)
	if err != nil {
		return nil, err
	}
	if size == 1 {
		return emit(size, e, []byte{0xC0}, []byte{byte(ops[1].imm)})
	}
	return emit(size, e, []byte{0xC1}, []byte{byte(ops[1].imm)})
}

func encodeTest(ops []operand) ([]byte, error) {
	if len(ops) != 2 {
		return nil, fmt.Errorf("asm: test expects 2 operands")
	}
	dest, src := ops[0], ops[1]
	size, err := binSize(dest, src)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, fmt.Errorf("asm: test cannot determine operand size")
	}
	if src.kind == kindImm {
		e, err := encodeRM(0, dest, size)
		if err != nil {
			return nil, err
		}
		if size == 1 {
			return emit(size, e, []byte{0xF6}, []byte{byte(src.imm)})
		}
		if size == 8 && !fitsInt32(src.imm) {
			return nil, fmt.Errorf("asm: test immediate does not fit in 32 bits")
		}
		return emit(size, e, []byte{0xF7}, le32(int32(src.imm)))
	}
	if src.kind != kindReg {
		return nil, fmt.Errorf("asm: test unsupported operand combination")
	}
	e, err := encodeRM(src.reg.code, dest, size)
	if err != nil {
		return nil, err
	}
	return emit(size, e, []byte{0x85}, nil)
}

func encodeImul(ops []operand) ([]byte, error) {
	switch len(ops) {
	case 2:
		if ops[0].kind != kindReg {
			return nil, fmt.Errorf("asm: imul destination must be a register")
		}
		size := ops[0].reg.size
		e, err := encodeRM(ops[0].reg.code, ops[1], size)
		if err != nil {
			return nil, err
		}
		return emit(size, e, []byte{0x0F, 0xAF}, nil)
	case 3:
		if ops[0].kind != kindReg || ops[2].kind != kindImm {
			return nil, fmt.Errorf("asm: imul expects reg, r/m, imm")
		}
		size := ops[0].reg.size
		e, err := encodeRM(ops[0].reg.code, ops[1], size)
		if err != nil {
			return nil, err
		}
		if fitsInt8(ops[2].imm) {
			return emit(size, e, []byte{0x6B}, []byte{byte(int8(ops[2].imm))})
		}
		if size == 8 && !fitsInt32(ops[2].imm) {
			return nil, fmt.Errorf("asm: imul immediate does not fit in 32 bits")
		}
		return emit(size, e, []byte{0x69}, le32(int32(ops[2].imm)))
	default:
		return nil, fmt.Errorf("asm: imul expects 2 or 3 operands")
	}
}

func encodeExtend(mnem string, opByte, opWord byte, ops []operand) ([]byte, error) {
	if len(ops) != 2 || ops[0].kind != kindReg {
		return nil, fmt.Errorf("asm: %s expects a register destination", mnem)
	}
	src := ops[1]
	var opcode byte
	switch operandSize(src) {
	case 1:
		opcode = opByte
	case 2:
		opcode = opWord
	default:
		return nil, fmt.Errorf("asm: %s source must be a byte or word", mnem)
	}
	size := ops[0].reg.size
	e, err := encodeRM(ops[0].reg.code, src, size)
	if err != nil {
		return nil, err
	}
	return emit(size, e, []byte{0x0F, opcode}, nil)
}

func binSize(a, b operand) (int, error) {
	sa, sb := operandSize(a), operandSize(b)
	if sa != 0 && sb != 0 && sa != sb {
		return 0, fmt.Errorf("asm: operand size mismatch (%d vs %d)", sa, sb)
	}
	if sa != 0 {
		return sa, nil
	}
	return sb, nil
}

func operandSize(o operand) int {
	if o.kind == kindReg {
		return o.reg.size
	}
	if o.hasSize {
		return o.size
	}
	return 0
}

func fitsInt8(v int64) bool { return v >= math.MinInt8 && v <= math.MaxInt8 }

func fitsInt32(v int64) bool { return v >= math.MinInt32 && v <= math.MaxInt32 }

func scaleBits(s int) int {
	switch s {
	case 2:
		return 1
	case 4:
		return 2
	case 8:
		return 3
	default:
		return 0
	}
}

func le16(v uint16) []byte {
	return []byte{byte(v), byte(v >> 8)}
}

func le32(v int32) []byte {
	u := uint32(v)
	return []byte{byte(u), byte(u >> 8), byte(u >> 16), byte(u >> 24)}
}

func le64(v uint64) []byte {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(v >> (8 * i))
	}
	return b
}
