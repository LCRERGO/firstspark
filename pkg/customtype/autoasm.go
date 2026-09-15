package customtype

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/LCRERGO/firstspark/pkg/autoasm"
	"github.com/LCRERGO/firstspark/pkg/jit"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

// aaProgram is a compiled Auto Assembler conversion pair loaded into this
// process.
type aaProgram struct {
	prog  *jit.Program
	read  uintptr
	write uintptr
	size  int
	mu    sync.Mutex
}

func buildAA(def Definition) (*aaProgram, error) {
	script, err := autoasm.Parse(strings.Trim(def.Script, "\n\r"))
	if err != nil {
		return nil, err
	}
	var section *autoasm.Section
	for i := range script.Sections {
		if script.Sections[i].Enable {
			section = &script.Sections[i]
			break
		}
	}
	if section == nil {
		return nil, fmt.Errorf("no [ENABLE] section")
	}
	probe, _, err := autoasm.Assemble(section, 0, nil)
	if err != nil {
		return nil, err
	}
	prog, err := jit.New(len(probe)+16, def.Size+16)
	if err != nil {
		return nil, err
	}
	code, labels, err := autoasm.Assemble(section, uint64(prog.Base()), nil)
	if err != nil {
		prog.Close()
		return nil, err
	}
	if err := prog.Load(code); err != nil {
		prog.Close()
		return nil, err
	}
	if err := prog.Seal(); err != nil {
		prog.Close()
		return nil, err
	}
	readOff, ok := labelOffset(labels, prog.Base(), "convertroutine")
	if !ok {
		prog.Close()
		return nil, fmt.Errorf("script defines no ConvertRoutine")
	}
	p := &aaProgram{prog: prog, read: prog.Entry(readOff), size: def.Size}
	if off, ok := labelOffset(labels, prog.Base(), "convertbackroutine"); ok {
		p.write = prog.Entry(off)
	}
	return p, nil
}

func (p *aaProgram) readInt(raw []byte) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prog.SetData(raw)
	return int64(p.prog.Call(p.read, p.prog.DataPtr()))
}

func (p *aaProgram) writeInt(n int64) []byte {
	if p.write == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prog.Call(p.write, uintptr(n), p.prog.DataPtr())
	return p.prog.DataCopy(p.size)
}

func (p *aaProgram) Close() { p.prog.Close() }

// RegisterAA compiles an Auto Assembler script whose [ENABLE] section defines
// a `ConvertRoutine` label (bytes pointer in RDI, value in RAX) and an
// optional `ConvertBackRoutine` label (value in RDI, output pointer in RSI),
// loads the code into this process and registers a type backed by it
// (ADR 0023).
func RegisterAA(def Definition) (*scan.Type, error) {
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return nil, fmt.Errorf("customtype: name is required")
	}
	if def.Size <= 0 {
		return nil, fmt.Errorf("customtype: %s: size must be positive", name)
	}
	kind, err := parseKind(def.Kind)
	if err != nil {
		return nil, fmt.Errorf("customtype: %s: %w", name, err)
	}
	if kind != scan.KindInt {
		return nil, fmt.Errorf("customtype: %s: auto-assembler types support only kind int", name)
	}
	prog, err := buildAA(def)
	if err != nil {
		return nil, fmt.Errorf("customtype: %s: %w", name, err)
	}
	alignment := def.Alignment
	if alignment <= 0 {
		alignment = def.Size
	}
	t := &scan.Type{
		ID:        scan.NextTypeID(),
		Name:      strings.ToLower(name),
		Label:     name,
		Size:      def.Size,
		Alignment: alignment,
		Kind:      kind,
	}
	t.Int64 = func(v scan.Value) int64 { return prog.readInt(v.Raw) }
	t.Numeric = func(v scan.Value) float64 { return float64(prog.readInt(v.Raw)) }
	t.Format = func(v scan.Value) string { return strconv.FormatInt(prog.readInt(v.Raw), 10) }
	if prog.write != 0 {
		t.Parse = func(input string) (scan.Value, error) {
			n, err := strconv.ParseInt(strings.TrimSpace(input), 0, 64)
			if err != nil {
				return scan.Value{}, err
			}
			return scan.Value{Type: t.ID, Raw: prog.writeInt(n)}, nil
		}
		t.Encode = func(n int64) []byte { return prog.writeInt(n) }
	}
	scan.RegisterType(t)
	return t, nil
}

// Validate compiles a definition without registering it.
func Validate(def Definition) error {
	if isAA(def) {
		prog, err := buildAA(def)
		if err != nil {
			return err
		}
		prog.Close()
		return nil
	}
	return validateLua(def)
}

// AATest converts data through an Auto Assembler definition's routines and
// returns the value and the write-back round trip.
func AATest(def Definition, data []byte) (int64, []byte, error) {
	prog, err := buildAA(def)
	if err != nil {
		return 0, nil, err
	}
	defer prog.Close()
	value := prog.readInt(data)
	var back []byte
	if prog.write != 0 {
		back = prog.writeInt(value)
	}
	return value, back, nil
}

func isAA(def Definition) bool {
	m := strings.ToLower(strings.TrimSpace(def.Mode))
	return m == "aa" || m == "autoassembler"
}

func labelOffset(labels map[string]uint64, base uintptr, name string) (int, bool) {
	for k, v := range labels {
		if strings.EqualFold(k, name) {
			return int(v - uint64(base)), true
		}
	}
	return 0, false
}
