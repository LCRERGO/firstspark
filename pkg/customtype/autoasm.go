package customtype

import (
	"bytes"
	"fmt"
	"math"
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
	prog       *jit.Program
	read       uintptr
	write      uintptr
	size       int
	stringKind bool
	textOff    int
	textSize   int
	mu         sync.Mutex
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
	stringKind := isStringKind(def.Kind)
	textSize := def.MaxStringSize
	if textSize <= 0 {
		textSize = 64
	}
	bufLen := def.Size + 16
	if stringKind {
		bufLen = def.Size + textSize + 1
	}
	prog, err := jit.New(len(probe)+16, bufLen)
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
	p := &aaProgram{
		prog: prog, read: prog.Entry(readOff), size: def.Size,
		stringKind: stringKind, textOff: def.Size, textSize: textSize,
	}
	if off, ok := labelOffset(labels, prog.Base(), "convertbackroutine"); ok {
		p.write = prog.Entry(off)
	}
	return p, nil
}

func isStringKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "string", "text":
		return true
	default:
		return false
	}
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

// readString runs the CE-style three-argument string routine into the text
// buffer and decodes it as a NUL-terminated string.
func (p *aaProgram) readString(raw []byte) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prog.SetData(raw)
	p.prog.SetBytesOff(p.textOff, make([]byte, p.textSize+1))
	p.prog.Call(p.read, p.prog.DataPtr(), 0, p.prog.PtrOff(p.textOff))
	b := p.prog.CopyOff(p.textOff, p.textSize+1)
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// writeString runs the reverse string routine, which writes size bytes at the
// data pointer.
func (p *aaProgram) writeString(s string) ([]byte, error) {
	if p.write == 0 {
		return nil, fmt.Errorf("customtype: type is read-only")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	buf := make([]byte, p.textSize+1)
	copy(buf, s)
	p.prog.SetBytesOff(p.textOff, buf)
	p.prog.SetBytesOff(0, make([]byte, p.size))
	p.prog.Call(p.write, p.prog.PtrOff(p.textOff), 0, p.prog.DataPtr())
	return p.prog.DataCopy(p.size), nil
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
	if kind != scan.KindInt && kind != scan.KindFloat && kind != scan.KindString {
		return nil, fmt.Errorf("customtype: %s: auto-assembler types support only int, float and string", name)
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
	if kind == scan.KindFloat {
		t.Numeric = func(v scan.Value) float64 {
			return float64(math.Float32frombits(uint32(prog.readInt(v.Raw))))
		}
		t.Int64 = func(v scan.Value) int64 {
			return int64(math.Float32frombits(uint32(prog.readInt(v.Raw))))
		}
		t.Format = func(v scan.Value) string {
			return strconv.FormatFloat(float64(math.Float32frombits(uint32(prog.readInt(v.Raw)))), 'g', -1, 32)
		}
		if prog.write != 0 {
			t.Parse = func(input string) (scan.Value, error) {
				f, err := strconv.ParseFloat(strings.TrimSpace(input), 32)
				if err != nil {
					return scan.Value{}, err
				}
				return scan.Value{Type: t.ID, Raw: prog.writeInt(int64(math.Float32bits(float32(f))))}, nil
			}
			t.Encode = func(n int64) []byte { return prog.writeInt(n) }
		}
	} else if kind == scan.KindString {
		t.Text = func(v scan.Value) string { return prog.readString(v.Raw) }
		t.Format = func(v scan.Value) string { return prog.readString(v.Raw) }
		t.Numeric = func(scan.Value) float64 { return 0 }
		t.Int64 = func(scan.Value) int64 { return 0 }
		if prog.write != 0 {
			t.Parse = func(input string) (scan.Value, error) {
				raw, err := prog.writeString(strings.Trim(strings.TrimSpace(input), `"`))
				if err != nil {
					return scan.Value{}, err
				}
				return scan.Value{Type: t.ID, Raw: raw}, nil
			}
		}
	} else {
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
