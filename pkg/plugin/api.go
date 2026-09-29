package plugin

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/LCRERGO/firstspark/pkg/script"
)

// globals builds the firstspark.* API table a plugin's chunk sees. Functions
// are only installed when their capability is requested and the host supports
// them.
func (p *Plugin) globals() map[string]script.Value {
	fs := script.NewTable()
	set := func(name string, v script.Value) { fs.Set(script.Str(name), v) }

	set("log", script.GoFunc("log", func(args []script.Value) ([]script.Value, error) {
		if p.api.Log != nil {
			p.api.Log(stringArg(args, 0))
		}
		return nil, nil
	}))
	set("print", fs.Get(script.Str("log")))
	set("version", script.Int(APIVersion))

	proc := script.NewTable()
	proc.Set(script.Str("pid"), script.GoFunc("pid", func([]script.Value) ([]script.Value, error) {
		if p.api.PID == nil {
			return []script.Value{script.Int(0)}, nil
		}
		return []script.Value{script.Int(int64(p.api.PID()))}, nil
	}))
	set("process", script.TableVal(proc))

	if (p.api.Read != nil || p.api.Write != nil) && (p.Manifest.Grants(CapMemoryRead) || p.Manifest.Grants(CapMemoryWrite)) {
		mem := script.NewTable()
		if p.Manifest.Grants(CapMemoryRead) && p.api.Read != nil {
			mem.Set(script.Str("read_bytes"), script.GoFunc("read_bytes", p.readBytes))
			mem.Set(script.Str("read_u8"), p.typedRead("read_u8", 1))
			mem.Set(script.Str("read_i32"), p.typedRead("read_i32", 4))
			mem.Set(script.Str("read_u32"), p.typedRead("read_u32", 4))
			mem.Set(script.Str("read_u64"), p.typedRead("read_u64", 8))
			mem.Set(script.Str("read_f32"), p.typedRead("read_f32", 4))
			mem.Set(script.Str("read_f64"), p.typedRead("read_f64", 8))
		}
		if p.Manifest.Grants(CapMemoryWrite) && p.api.Write != nil {
			mem.Set(script.Str("write_bytes"), script.GoFunc("write_bytes", p.writeBytes))
		}
		set("memory", script.TableVal(mem))
	}

	if p.Manifest.Grants(CapUI) && p.api.Show != nil {
		ui := script.NewTable()
		ui.Set(script.Str("notify"), script.GoFunc("notify", func(args []script.Value) ([]script.Value, error) {
			p.api.Show(stringArg(args, 0))
			return nil, nil
		}))
		set("ui", script.TableVal(ui))
	}

	if p.Manifest.Grants(CapScan) && p.api.RegisterType != nil {
		set("register_value_type", script.GoFunc("register_value_type", func(args []script.Value) ([]script.Value, error) {
			name := stringArg(args, 0)
			var spec script.Value
			if len(args) > 1 {
				spec = args[1]
			}
			if err := p.api.RegisterType(name, spec); err != nil {
				return nil, err
			}
			return []script.Value{script.Bool(true)}, nil
		}))
	}

	if p.Manifest.Grants(CapHooking) && p.api.InstallHook != nil {
		hook := script.NewTable()
		hook.Set(script.Str("install"), script.GoFunc("install", func(args []script.Value) ([]script.Value, error) {
			symbol := stringArg(args, 0)
			if len(args) < 2 {
				return nil, fmt.Errorf("hook.install: missing handler bytes")
			}
			code, err := script.ValueBytes(args[1])
			if err != nil {
				return nil, err
			}
			remove, err := p.api.InstallHook(symbol, code)
			if err != nil {
				return nil, err
			}
			if remove != nil {
				p.hooks = append(p.hooks, remove)
			}
			return []script.Value{script.Bool(true)}, nil
		}))
		set("hook", script.TableVal(hook))
	}

	return map[string]script.Value{"firstspark": script.TableVal(fs)}
}

func (p *Plugin) readBytes(args []script.Value) ([]script.Value, error) {
	addr, err := addrArg(args, 0)
	if err != nil {
		return nil, err
	}
	size := int(intArg(args, 1))
	if size <= 0 {
		return nil, fmt.Errorf("read_bytes: size must be positive")
	}
	if size > 1<<20 {
		return nil, fmt.Errorf("read_bytes: size %d exceeds the 1 MiB limit", size)
	}
	b, err := p.api.Read(addr, size)
	if err != nil {
		return nil, err
	}
	return []script.Value{script.BytesValue(b)}, nil
}

func (p *Plugin) writeBytes(args []script.Value) ([]script.Value, error) {
	addr, err := addrArg(args, 0)
	if err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, fmt.Errorf("write_bytes: missing data")
	}
	data, err := script.ValueBytes(args[1])
	if err != nil {
		return nil, err
	}
	if err := p.api.Write(addr, data); err != nil {
		return nil, err
	}
	return []script.Value{script.Bool(true)}, nil
}

func (p *Plugin) typedRead(name string, size int) script.Value {
	return script.GoFunc(name, func(args []script.Value) ([]script.Value, error) {
		addr, err := addrArg(args, 0)
		if err != nil {
			return nil, err
		}
		b, err := p.api.Read(addr, size)
		if err != nil {
			return nil, err
		}
		if len(b) < size {
			return nil, fmt.Errorf("%s: short read", name)
		}
		return []script.Value{decodeNumber(name, b)}, nil
	})
}

func decodeNumber(name string, b []byte) script.Value {
	switch name {
	case "read_u8":
		return script.Int(int64(b[0]))
	case "read_i32":
		return script.Int(int64(int32(binary.LittleEndian.Uint32(b))))
	case "read_u32":
		return script.Int(int64(binary.LittleEndian.Uint32(b)))
	case "read_u64":
		return script.Int(int64(binary.LittleEndian.Uint64(b)))
	case "read_f32":
		return script.Float(float64(math.Float32frombits(binary.LittleEndian.Uint32(b))))
	case "read_f64":
		return script.Float(math.Float64frombits(binary.LittleEndian.Uint64(b)))
	default:
		return script.Nil()
	}
}

func stringArg(args []script.Value, i int) string {
	if i >= len(args) {
		return ""
	}
	return args[i].String()
}

func intArg(args []script.Value, i int) int64 {
	if i >= len(args) || !args[i].IsNumber() {
		return 0
	}
	n, _ := args[i].Number()
	return int64(n)
}

func addrArg(args []script.Value, i int) (uint64, error) {
	if i >= len(args) {
		return 0, fmt.Errorf("missing address")
	}
	v := args[i]
	if v.Kind() == script.KindString {
		s := strings.TrimSpace(v.Str())
		s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
		n, err := strconv.ParseUint(s, 16, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid address %q", v.Str())
		}
		return n, nil
	}
	if v.IsNumber() {
		n, _ := v.Number()
		return uint64(int64(n)), nil
	}
	return 0, fmt.Errorf("invalid address")
}
