// Package customtype loads user-defined value types from customtypes.yaml and
// registers them with the scan type registry. Each type is backed by a Lua
// 5.1.4-compatible script compiled by pkg/script (ADR 0012, ADR 0013).
package customtype

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/LCRERGO/firstspark/pkg/scan"
	"github.com/LCRERGO/firstspark/pkg/script"
)

// Definition is one user-defined value type.
type Definition struct {
	Name        string `yaml:"name"`
	Size        int    `yaml:"size"`
	Kind        string `yaml:"kind"`
	Mode        string `yaml:"mode,omitempty"` // "lua" (default) or "aa"
	Script      string `yaml:"script"`
	Alignment   int    `yaml:"alignment,omitempty"`
	Description string `yaml:"description,omitempty"`
	// MaxStringSize is the conversion buffer for a string Auto Assembler type.
	MaxStringSize int `yaml:"max_string_size,omitempty" json:"max_string_size,omitempty"`
}

type document struct {
	Types []Definition `yaml:"types"`
}

// Load reads definitions from path. A missing file yields no types.
func Load(path string) ([]Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("customtype: read %s: %w", path, err)
	}
	var doc document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("customtype: parse %s: %w", path, err)
	}
	return doc.Types, nil
}

// Save writes definitions to path. Scripts are trimmed of surrounding blank
// lines because yaml.v3 cannot re-parse a block scalar that starts with one.
func Save(path string, defs []Definition) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("customtype: create dir: %w", err)
	}
	for i := range defs {
		defs[i].Script = strings.Trim(defs[i].Script, "\n\r")
	}
	data, err := yaml.Marshal(document{Types: defs})
	if err != nil {
		return fmt.Errorf("customtype: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("customtype: write %s: %w", path, err)
	}
	return nil
}

// RegisterRaw registers a passthrough integer type of the given size without a
// conversion script. It is used for Cheat Engine custom types whose Auto
// Assembler conversion routine cannot be translated yet (ADR 0037 S5): the
// width and name are preserved, the value is shown and edited raw.
func RegisterRaw(name string, size int) (*scan.Type, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("customtype: name is required")
	}
	if size != 1 && size != 2 && size != 4 && size != 8 {
		return nil, fmt.Errorf("customtype: %s: unsupported size %d", name, size)
	}
	decode := func(raw []byte) int64 {
		switch len(raw) {
		case 1:
			return int64(int8(raw[0]))
		case 2:
			return int64(int16(binary.LittleEndian.Uint16(raw)))
		case 4:
			return int64(int32(binary.LittleEndian.Uint32(raw)))
		case 8:
			return int64(binary.LittleEndian.Uint64(raw))
		}
		return 0
	}
	encode := func(n int64) []byte {
		b := make([]byte, size)
		switch size {
		case 1:
			b[0] = byte(n)
		case 2:
			binary.LittleEndian.PutUint16(b, uint16(n))
		case 4:
			binary.LittleEndian.PutUint32(b, uint32(n))
		case 8:
			binary.LittleEndian.PutUint64(b, uint64(n))
		}
		return b
	}
	t := &scan.Type{
		ID:        scan.NextTypeID(),
		Name:      strings.ToLower(name),
		Label:     name,
		Size:      size,
		Alignment: size,
		Kind:      scan.KindInt,
	}
	t.Int64 = func(v scan.Value) int64 { return decode(v.Raw) }
	t.Numeric = func(v scan.Value) float64 { return float64(decode(v.Raw)) }
	t.Format = func(v scan.Value) string { return strconv.FormatInt(decode(v.Raw), 10) }
	t.Parse = func(input string) (scan.Value, error) {
		n, err := strconv.ParseInt(strings.TrimSpace(input), 0, 64)
		if err != nil {
			return scan.Value{}, err
		}
		return scan.Value{Type: t.ID, Raw: encode(n)}, nil
	}
	t.Encode = encode
	scan.RegisterType(t)
	return t, nil
}

// Register compiles a definition and registers it as a scan type.
func Register(def Definition) (*scan.Type, error) {
	if strings.EqualFold(strings.TrimSpace(def.Mode), "aa") || strings.EqualFold(strings.TrimSpace(def.Mode), "autoassembler") {
		return RegisterAA(def)
	}
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
	prog, err := script.Compile(strings.Trim(def.Script, "\n\r"))
	if err != nil {
		return nil, fmt.Errorf("customtype: %s: %w", name, err)
	}
	readFn, err := prog.Func("bytes_to_value")
	if err != nil {
		return nil, fmt.Errorf("customtype: %s: %w", name, err)
	}
	writeFn, _ := prog.Func("value_to_bytes")

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
	t.Format = func(v scan.Value) string { return formatValue(prog, readFn, kind, v) }
	t.Text = func(v scan.Value) string { return readString(prog, readFn, v) }
	t.Numeric = func(v scan.Value) float64 { return readNumber(prog, readFn, v) }
	t.Int64 = func(v scan.Value) int64 { return int64(readNumber(prog, readFn, v)) }
	t.Parse = func(input string) (scan.Value, error) {
		if writeFn == nil {
			return scan.Value{}, fmt.Errorf("customtype: %s is read-only", name)
		}
		raw, err := encodeInput(prog, writeFn, kind, input)
		if err != nil {
			return scan.Value{}, err
		}
		return scan.Value{Type: t.ID, Raw: raw}, nil
	}
	t.Encode = func(n int64) []byte {
		if writeFn == nil {
			return nil
		}
		out := writeFn.Call(prog.Env(), script.Int(n), script.Int(0))
		if len(out) == 0 {
			return nil
		}
		b, err := script.ValueBytes(out[0])
		if err != nil {
			return nil
		}
		return b
	}
	if err := validate(prog, readFn, def.Size); err != nil {
		return nil, fmt.Errorf("customtype: %s: %w", name, err)
	}
	scan.RegisterType(t)
	return t, nil
}

func validateLua(def Definition) error {
	prog, err := script.Compile(strings.Trim(def.Script, "\n\r"))
	if err != nil {
		return err
	}
	if _, err := prog.Func("bytes_to_value"); err != nil {
		return err
	}
	return nil
}

// RegisterAll registers every definition, stopping at the first error.
func RegisterAll(defs []Definition) ([]*scan.Type, error) {
	var out []*scan.Type
	for _, d := range defs {
		t, err := Register(d)
		if err != nil {
			return out, err
		}
		out = append(out, t)
	}
	return out, nil
}

// LoadAndRegister loads path and registers all of its types.
func LoadAndRegister(path string) ([]*scan.Type, error) {
	defs, err := Load(path)
	if err != nil {
		return nil, err
	}
	return RegisterAll(defs)
}

func parseKind(s string) (scan.Kind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "int", "integer":
		return scan.KindInt, nil
	case "float", "double":
		return scan.KindFloat, nil
	case "string", "text":
		return scan.KindString, nil
	default:
		return scan.KindInt, fmt.Errorf("unknown kind %q", s)
	}
}

func decode(prog *script.Program, fn *script.Function, raw []byte) script.Value {
	out := fn.Call(prog.Env(), script.BytesValue(raw), script.Int(0))
	if len(out) == 0 {
		return script.Nil()
	}
	return out[0]
}

func readNumber(prog *script.Program, fn *script.Function, v scan.Value) float64 {
	if f, ok := decode(prog, fn, v.Raw).Number(); ok {
		return f
	}
	return 0
}

func readString(prog *script.Program, fn *script.Function, v scan.Value) string {
	return decode(prog, fn, v.Raw).String()
}

func formatValue(prog *script.Program, fn *script.Function, kind scan.Kind, v scan.Value) string {
	r := decode(prog, fn, v.Raw)
	switch kind {
	case scan.KindFloat:
		if f, ok := r.Number(); ok {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
	case scan.KindString:
		return r.String()
	default:
		if f, ok := r.Number(); ok {
			return strconv.FormatInt(int64(f), 10)
		}
	}
	return r.String()
}

func encodeInput(prog *script.Program, fn *script.Function, kind scan.Kind, input string) ([]byte, error) {
	var arg script.Value
	switch kind {
	case scan.KindString:
		arg = script.Str(strings.Trim(strings.TrimSpace(input), `"`))
	case scan.KindFloat:
		f, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
		if err != nil {
			return nil, err
		}
		arg = script.Float(f)
	default:
		n, err := strconv.ParseInt(strings.TrimSpace(input), 0, 64)
		if err != nil {
			return nil, err
		}
		arg = script.Int(n)
	}
	out := fn.Call(prog.Env(), arg, script.Int(0))
	if len(out) == 0 {
		return nil, fmt.Errorf("value_to_bytes returned no value")
	}
	return script.ValueBytes(out[0])
}

func validate(prog *script.Program, readFn *script.Function, size int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("script error: %v", r)
		}
	}()
	readFn.Call(prog.Env(), script.BytesValue(make([]byte, size)), script.Int(0))
	return nil
}
