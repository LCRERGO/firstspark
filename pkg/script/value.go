package script

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Kind is the runtime type of a Value.
type Kind uint8

const (
	KindNil Kind = iota
	KindBool
	KindInt
	KindFloat
	KindString
	KindTable
	KindFunction
)

// Value is a Lua value. It is comparable so it can be used as a table key.
type Value struct {
	kind Kind
	b    bool
	i    int64
	f    float64
	s    string
	t    *Table
	fn   *Function
}

// Nil returns the nil value.
func Nil() Value { return Value{kind: KindNil} }

// Bool returns a boolean value.
func Bool(b bool) Value { return Value{kind: KindBool, b: b} }

// Int returns an integer value.
func Int(i int64) Value { return Value{kind: KindInt, i: i} }

// Float returns a float value.
func Float(f float64) Value { return Value{kind: KindFloat, f: f} }

// Str returns a string value.
func Str(s string) Value { return Value{kind: KindString, s: s} }

// TableVal wraps a table as a value.
func TableVal(t *Table) Value { return Value{kind: KindTable, t: t} }

// FuncVal wraps a function as a value.
func FuncVal(fn *Function) Value { return Value{kind: KindFunction, fn: fn} }

// Kind reports the value's runtime type.
func (v Value) Kind() Kind { return v.kind }

// IsNil reports whether v is nil.
func (v Value) IsNil() bool { return v.kind == KindNil }

// Bool returns the boolean payload.
func (v Value) Bool() bool { return v.b }

// Int returns the integer payload.
func (v Value) Int() int64 { return v.i }

// Float returns the float payload.
func (v Value) Float() float64 { return v.f }

// Str returns the string payload.
func (v Value) Str() string { return v.s }

// Table returns the table payload.
func (v Value) Table() *Table { return v.t }

// Function returns the function payload.
func (v Value) Function() *Function { return v.fn }

// Number returns the numeric value as a float64.
func (v Value) Number() (float64, bool) {
	switch v.kind {
	case KindInt:
		return float64(v.i), true
	case KindFloat:
		return v.f, true
	default:
		return 0, false
	}
}

// IsNumber reports whether v is an integer or float.
func (v Value) IsNumber() bool { return v.kind == KindInt || v.kind == KindFloat }

// String renders v the way Lua's tostring does.
func (v Value) String() string {
	switch v.kind {
	case KindNil:
		return "nil"
	case KindBool:
		if v.b {
			return "true"
		}
		return "false"
	case KindInt:
		return strconv.FormatInt(v.i, 10)
	case KindFloat:
		return formatFloat(v.f)
	case KindString:
		return v.s
	case KindTable:
		return fmt.Sprintf("table: %p", v.t)
	case KindFunction:
		return fmt.Sprintf("function: %p", v.fn)
	default:
		return "?"
	}
}

func formatFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	s := strconv.FormatFloat(f, 'g', 14, 64)
	if !strings.ContainsAny(s, ".eEni") {
		s += ".0"
	}
	return s
}

// Table is a Lua table with an array part and a keyed part.
type Table struct {
	arr  []Value
	hash map[Value]Value
}

// NewTable creates an empty table.
func NewTable() *Table { return &Table{} }

// Get returns the value at k, or nil.
func (t *Table) Get(k Value) Value {
	if k.kind == KindInt && k.i >= 1 && k.i <= int64(len(t.arr)) {
		return t.arr[k.i-1]
	}
	if t.hash == nil {
		return Nil()
	}
	if v, ok := t.hash[k]; ok {
		return v
	}
	return Nil()
}

// Set assigns v at k.
func (t *Table) Set(k, v Value) {
	if k.kind == KindInt && k.i >= 1 {
		if k.i <= int64(len(t.arr)) {
			t.arr[k.i-1] = v
			return
		}
		if k.i == int64(len(t.arr))+1 && !v.IsNil() {
			t.arr = append(t.arr, v)
			return
		}
	}
	if t.hash == nil {
		t.hash = map[Value]Value{}
	}
	t.hash[k] = v
}

// Append adds v after the last array element.
func (t *Table) Append(v Value) { t.arr = append(t.arr, v) }

// Len returns the number of array elements.
func (t *Table) Len() int { return len(t.arr) }

// Array returns the array part.
func (t *Table) Array() []Value { return t.arr }

// Function is a compiled callable.
type Function struct {
	Name string
	call func(env *Env, args []Value) []Value
}

// Call invokes the function.
func (f *Function) Call(env *Env, args ...Value) []Value { return f.call(env, args) }

// Env is a lexical scope chain.
type Env struct {
	vars   map[string]Value
	parent *Env
}

func newEnv(parent *Env) *Env {
	return &Env{vars: map[string]Value{}, parent: parent}
}

func (e *Env) lookup(name string) (Value, bool) {
	for env := e; env != nil; env = env.parent {
		if v, ok := env.vars[name]; ok {
			return v, true
		}
	}
	return Nil(), false
}

func (e *Env) get(name string) Value {
	v, _ := e.lookup(name)
	return v
}

func (e *Env) set(name string, v Value) {
	for env := e; env != nil; env = env.parent {
		if _, ok := env.vars[name]; ok {
			env.vars[name] = v
			return
		}
	}
	e.root().vars[name] = v
}

func (e *Env) root() *Env {
	for env := e; env.parent != nil; env = env.parent {
		e = env
	}
	return e
}

func (e *Env) setLocal(name string, v Value) { e.vars[name] = v }

// runtimeError unwinds a Lua runtime error to the call boundary.
type runtimeError struct{ msg string }

func (e runtimeError) Error() string { return e.msg }

func throw(format string, args ...any) {
	panic(runtimeError{msg: fmt.Sprintf(format, args...)})
}
