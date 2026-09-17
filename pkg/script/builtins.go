package script

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

var (
	stdout    io.Writer = io.Discard
	stringLib           = NewTable()
	mathLib             = NewTable()
	tableLib            = NewTable()
)

// SetPrintWriter sets the destination of the Lua print function.
func SetPrintWriter(w io.Writer) { stdout = w }

func installBuiltins(env *Env) {
	base := map[string]func(*Env, []Value) []Value{
		"type":     biType,
		"tostring": biToString,
		"tonumber": biToNumber,
		"print":    biPrint,
		"pairs":    biPairs,
		"ipairs":   biIPairs,
		"next":     biNext,
		"select":   biSelect,
		"assert":   biAssert,
	}
	for name, fn := range base {
		env.setLocal(name, FuncVal(&Function{Name: name, call: fn}))
	}
	installString()
	installMath()
	installTable()
	env.setLocal("string", TableVal(stringLib))
	env.setLocal("math", TableVal(mathLib))
	env.setLocal("table", TableVal(tableLib))
}

func biType(_ *Env, args []Value) []Value {
	if len(args) == 0 {
		return []Value{Str("nil")}
	}
	return []Value{Str(typeName(args[0]))}
}

func biToString(_ *Env, args []Value) []Value {
	if len(args) == 0 {
		return []Value{Str("nil")}
	}
	return []Value{Str(args[0].String())}
}

func biToNumber(_ *Env, args []Value) []Value {
	if len(args) == 0 {
		return []Value{Nil()}
	}
	switch args[0].kind {
	case KindInt, KindFloat:
		return []Value{args[0]}
	case KindString:
		s := strings.TrimSpace(args[0].s)
		if n, err := strconv.ParseInt(s, 0, 64); err == nil {
			return []Value{Int(n)}
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return []Value{Float(f)}
		}
	}
	return []Value{Nil()}
}

func biPrint(_ *Env, args []Value) []Value {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.String()
	}
	fmt.Fprintln(stdout, strings.Join(parts, "\t"))
	return nil
}

func biAssert(_ *Env, args []Value) []Value {
	if len(args) == 0 || !truthy(args[0]) {
		msg := "assertion failed!"
		if len(args) > 1 {
			msg = args[1].String()
		}
		throw("%s", msg)
	}
	return args
}

func biSelect(_ *Env, args []Value) []Value {
	if len(args) == 0 {
		throw("bad argument #1 to 'select'")
	}
	if args[0].kind == KindString && args[0].s == "#" {
		return []Value{Int(int64(len(args) - 1))}
	}
	n := int(toInt(args[0]))
	if n < 0 {
		n = len(args) + n
	}
	if n < 1 {
		throw("bad argument #1 to 'select' (index out of range)")
	}
	if n >= len(args) {
		return nil
	}
	return args[n:]
}

func biNext(_ *Env, args []Value) []Value {
	t := argTable(args, 0)
	var key Value
	if len(args) > 1 {
		key = args[1]
	}
	keys := tableKeys(t)
	if key.IsNil() {
		if len(keys) == 0 {
			return []Value{Nil()}
		}
		return []Value{keys[0], t.Get(keys[0])}
	}
	for i, k := range keys {
		if equal(k, key) {
			if i+1 < len(keys) {
				nk := keys[i+1]
				return []Value{nk, t.Get(nk)}
			}
			return []Value{Nil()}
		}
	}
	return []Value{Nil()}
}

func biPairs(env *Env, args []Value) []Value {
	t := argTable(args, 0)
	next := &Function{Name: "next", call: biNext}
	return []Value{FuncVal(next), TableVal(t), Nil()}
}

func biIPairs(_ *Env, args []Value) []Value {
	t := argTable(args, 0)
	iter := &Function{Name: "ipairs", call: func(_ *Env, a []Value) []Value {
		i := toInt(a[1]) + 1
		v := t.Get(Int(i))
		if v.IsNil() {
			return []Value{Nil()}
		}
		return []Value{Int(i), v}
	}}
	return []Value{FuncVal(iter), TableVal(t), Int(0)}
}

func tableKeys(t *Table) []Value {
	keys := make([]Value, 0, len(t.arr)+len(t.hash))
	for i := range t.arr {
		keys = append(keys, Int(int64(i+1)))
	}
	for k := range t.hash {
		keys = append(keys, k)
	}
	return keys
}

func installString() {
	reg := func(name string, fn func(*Env, []Value) []Value) {
		stringLib.Set(Str(name), FuncVal(&Function{Name: "string." + name, call: fn}))
	}
	reg("len", func(_ *Env, a []Value) []Value { return []Value{Int(int64(len(argString(a, 0))))} })
	reg("upper", func(_ *Env, a []Value) []Value { return []Value{Str(strings.ToUpper(argString(a, 0)))} })
	reg("lower", func(_ *Env, a []Value) []Value { return []Value{Str(strings.ToLower(argString(a, 0)))} })
	reg("reverse", func(_ *Env, a []Value) []Value {
		r := []rune(argString(a, 0))
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return []Value{Str(string(r))}
	})
	reg("rep", func(_ *Env, a []Value) []Value {
		s := argString(a, 0)
		n := int(argInt(a, 1))
		if n <= 0 || s == "" {
			return []Value{Str("")}
		}
		if n > (1<<31)/len(s) {
			throw("string.rep result too large")
		}
		return []Value{Str(strings.Repeat(s, n))}
	})
	reg("sub", func(_ *Env, a []Value) []Value {
		r := []rune(argString(a, 0))
		i := int(argInt(a, 1))
		j := len(r)
		if len(a) > 2 {
			j = int(argInt(a, 2))
		}
		if i < 0 {
			i = len(r) + i + 1
		}
		if j < 0 {
			j = len(r) + j + 1
		}
		if i < 1 {
			i = 1
		}
		if j > len(r) {
			j = len(r)
		}
		if i > j {
			return []Value{Str("")}
		}
		return []Value{Str(string(r[i-1 : j]))}
	})
	reg("byte", func(_ *Env, a []Value) []Value {
		s := argString(a, 0)
		i := int64(1)
		if len(a) > 1 {
			i = argInt(a, 1)
		}
		j := i
		if len(a) > 2 {
			j = argInt(a, 2)
		}
		if i < 0 {
			i = int64(len(s)) + i + 1
		}
		if j < 0 {
			j = int64(len(s)) + j + 1
		}
		var out []Value
		for k := i; k <= j && k <= int64(len(s)); k++ {
			if k >= 1 {
				out = append(out, Int(int64(s[k-1])))
			}
		}
		return out
	})
	reg("char", func(_ *Env, a []Value) []Value {
		b := make([]byte, len(a))
		for i, v := range a {
			b[i] = byte(toInt(v))
		}
		return []Value{Str(string(b))}
	})
	reg("format", func(_ *Env, a []Value) []Value {
		if len(a) == 0 {
			return []Value{Str("")}
		}
		return []Value{Str(luaFormat(argString(a, 0), a[1:]))}
	})
}

func luaFormat(format string, args []Value) string {
	var b strings.Builder
	argi := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		i++
		if i >= len(format) {
			b.WriteByte('%')
			break
		}
		if format[i] == '%' {
			b.WriteByte('%')
			continue
		}
		start := i
		for i < len(format) && strings.IndexByte("-+ #0.123456789", format[i]) >= 0 {
			i++
		}
		if i >= len(format) {
			b.WriteString("%" + format[start:])
			break
		}
		verb := format[i]
		spec := format[start:i]
		var arg Value
		if argi < len(args) {
			arg = args[argi]
			argi++
		}
		switch verb {
		case 'd', 'i', 'u':
			b.WriteString(fmt.Sprintf("%"+spec+"d", toInt(arg)))
		case 'x', 'X', 'o':
			b.WriteString(fmt.Sprintf("%"+spec+string(verb), toInt(arg)))
		case 'f', 'e', 'E', 'g', 'G':
			b.WriteString(fmt.Sprintf("%"+spec+string(verb), toNumber(arg)))
		case 's':
			b.WriteString(fmt.Sprintf("%"+spec+"s", arg.String()))
		case 'q':
			b.WriteString(strconv.Quote(arg.String()))
		case 'c':
			b.WriteByte(byte(toInt(arg)))
		default:
			b.WriteString("%" + spec + string(verb))
		}
	}
	return b.String()
}

func installMath() {
	reg := func(name string, fn func(*Env, []Value) []Value) {
		mathLib.Set(Str(name), FuncVal(&Function{Name: "math." + name, call: fn}))
	}
	unary := func(f func(float64) float64) func(*Env, []Value) []Value {
		return func(_ *Env, a []Value) []Value { return []Value{numberValue(f(toNumber(argValue(a, 0))))} }
	}
	reg("floor", unary(math.Floor))
	reg("ceil", unary(math.Ceil))
	reg("abs", unary(math.Abs))
	reg("sqrt", unary(math.Sqrt))
	reg("fmod", func(_ *Env, a []Value) []Value {
		return []Value{numberValue(math.Mod(toNumber(argValue(a, 0)), toNumber(argValue(a, 1))))}
	})
	reg("mod", func(_ *Env, a []Value) []Value {
		return []Value{numberValue(math.Mod(toNumber(argValue(a, 0)), toNumber(argValue(a, 1))))}
	})
	reg("pow", func(_ *Env, a []Value) []Value {
		return []Value{Float(math.Pow(toNumber(argValue(a, 0)), toNumber(argValue(a, 1))))}
	})
	reg("min", func(_ *Env, a []Value) []Value {
		best := toNumber(argValue(a, 0))
		for _, v := range a[1:] {
			if n := toNumber(v); n < best {
				best = n
			}
		}
		return []Value{numberValue(best)}
	})
	reg("max", func(_ *Env, a []Value) []Value {
		best := toNumber(argValue(a, 0))
		for _, v := range a[1:] {
			if n := toNumber(v); n > best {
				best = n
			}
		}
		return []Value{numberValue(best)}
	})
	mathLib.Set(Str("pi"), Float(math.Pi))
	mathLib.Set(Str("huge"), Float(math.Inf(1)))
	mathLib.Set(Str("hugeint"), Int(math.MaxInt64))
}

func installTable() {
	reg := func(name string, fn func(*Env, []Value) []Value) {
		tableLib.Set(Str(name), FuncVal(&Function{Name: "table." + name, call: fn}))
	}
	reg("insert", func(_ *Env, a []Value) []Value {
		t := argTable(a, 0)
		if len(a) == 2 {
			t.Set(Int(int64(t.Len()+1)), a[1])
			return nil
		}
		pos := int(toInt(a[1]))
		val := a[2]
		for i := t.Len(); i >= pos; i-- {
			t.Set(Int(int64(i+1)), t.Get(Int(int64(i))))
		}
		t.Set(Int(int64(pos)), val)
		return nil
	})
	reg("remove", func(_ *Env, a []Value) []Value {
		t := argTable(a, 0)
		pos := t.Len()
		if len(a) > 1 {
			pos = int(toInt(a[1]))
		}
		if pos < 1 || pos > t.Len() {
			return []Value{Nil()}
		}
		v := t.Get(Int(int64(pos)))
		for i := pos; i < t.Len(); i++ {
			t.Set(Int(int64(i)), t.Get(Int(int64(i+1))))
		}
		t.Set(Int(int64(t.Len())), Nil())
		return []Value{v}
	})
	reg("concat", func(_ *Env, a []Value) []Value {
		t := argTable(a, 0)
		sep := ""
		if len(a) > 1 && a[1].kind == KindString {
			sep = a[1].s
		}
		parts := make([]string, t.Len())
		for i := range parts {
			parts[i] = t.Get(Int(int64(i + 1))).String()
		}
		return []Value{Str(strings.Join(parts, sep))}
	})
}

func argValue(a []Value, i int) Value {
	if i >= len(a) {
		throw("missing argument #%d", i+1)
	}
	return a[i]
}

func argString(a []Value, i int) string {
	v := argValue(a, i)
	if v.kind == KindString {
		return v.s
	}
	if v.IsNumber() {
		return v.String()
	}
	throw("bad argument #%d (string expected, got %s)", i+1, typeName(v))
	return ""
}

func argInt(a []Value, i int) int64 { return toInt(argValue(a, i)) }

func argTable(a []Value, i int) *Table {
	v := argValue(a, i)
	if v.kind != KindTable {
		throw("bad argument #%d (table expected, got %s)", i+1, typeName(v))
	}
	return v.t
}

func toInt(v Value) int64 {
	switch v.kind {
	case KindInt:
		return v.i
	case KindFloat:
		return int64(v.f)
	case KindString:
		if n, err := strconv.ParseInt(strings.TrimSpace(v.s), 0, 64); err == nil {
			return n
		}
	}
	throw("number expected, got %s", typeName(v))
	return 0
}
