// Package script implements the Lua 5.1.4-compatible subset used for
// user-defined value types. Scripts are parsed with pkg/combinator and
// compiled to Go closures, so a conversion function runs without an
// interpreter loop.
package script

import "fmt"

// Call invokes a Lua function value. The function keeps its captured
// environment; runtime errors are recovered and returned.
func CallValue(fn Value, args ...Value) (rets []Value, err error) {
	if fn.kind != KindFunction {
		return nil, fmt.Errorf("script: attempt to call a %s value", typeName(fn))
	}
	defer func() {
		if r := recover(); r != nil {
			re, ok := r.(runtimeError)
			if !ok {
				panic(r)
			}
			err = fmt.Errorf("script: %s", re.msg)
		}
	}()
	env := newEnv(nil)
	installBuiltins(env)
	return fn.fn.call(env, args), nil
}

// CompileWithGlobals compiles and runs src with the given globals installed.
// Unlike SetGlobal, the globals are visible while the top-level chunk runs,
// which callers need to provide APIs the chunk references immediately.
func CompileWithGlobals(src string, globals map[string]Value) (*Program, error) {
	chunk, err := parse(src)
	if err != nil {
		return nil, err
	}
	env := newEnv(nil)
	installBuiltins(env)
	for name, v := range globals {
		env.setLocal(name, v)
	}
	p := &Program{globals: env}
	if err := p.run(chunk); err != nil {
		return nil, err
	}
	return p, nil
}

// Program is a compiled script with its globals.
type Program struct {
	globals *Env
}

// Compile parses and runs the top-level chunk of src.
func Compile(src string) (*Program, error) {
	chunk, err := parse(src)
	if err != nil {
		return nil, err
	}
	env := newEnv(nil)
	installBuiltins(env)
	p := &Program{globals: env}
	if err := p.run(chunk); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Program) run(chunk []Stmt) (err error) {
	defer func() {
		if r := recover(); r != nil {
			re, ok := r.(runtimeError)
			if !ok {
				panic(r)
			}
			err = fmt.Errorf("script: %s", re.msg)
		}
	}()
	compileStmts(chunk)(p.globals)
	return nil
}

// Has reports whether a global is defined.
func (p *Program) Has(name string) bool {
	_, ok := p.globals.lookup(name)
	return ok
}

// Global returns a global value, or nil.
func (p *Program) Global(name string) Value { return p.globals.get(name) }

// Globals returns a copy of the program's global scope, for callers that keep
// state across chunks.
func (p *Program) Globals() map[string]Value {
	out := make(map[string]Value, len(p.globals.vars))
	for k, v := range p.globals.vars {
		out[k] = v
	}
	return out
}

// Func returns a global function for direct (non-recovering) calls.
func (p *Program) Func(name string) (*Function, error) {
	v := p.globals.get(name)
	if v.kind != KindFunction {
		return nil, fmt.Errorf("script: %q is not a function", name)
	}
	return v.fn, nil
}

// Env returns the program's global environment, used with Function.Call.
func (p *Program) Env() *Env { return p.globals }

// SetGlobal defines a global value.
func (p *Program) SetGlobal(name string, v Value) { p.globals.setLocal(name, v) }

// Call invokes a global function and returns its results.
func (p *Program) Call(name string, args ...Value) (out []Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			re, ok := r.(runtimeError)
			if !ok {
				panic(r)
			}
			err = fmt.Errorf("script: %s", re.msg)
		}
	}()
	fn := p.globals.get(name)
	if fn.kind != KindFunction {
		return nil, fmt.Errorf("script: %q is not a function", name)
	}
	return fn.fn.call(p.globals, args), nil
}

// BytesValue returns a 1-indexed table of byte integers.
func BytesValue(b []byte) Value {
	t := NewTable()
	for _, x := range b {
		t.Append(Int(int64(x)))
	}
	return TableVal(t)
}

// ValueBytes converts a table of integers into bytes.
func ValueBytes(v Value) ([]byte, error) {
	if v.kind != KindTable {
		return nil, fmt.Errorf("script: expected a table of bytes, got %s", typeName(v))
	}
	arr := v.t.Array()
	out := make([]byte, len(arr))
	for i, e := range arr {
		switch e.kind {
		case KindInt:
			out[i] = byte(e.i)
		case KindFloat:
			out[i] = byte(e.f)
		default:
			return nil, fmt.Errorf("script: byte %d is not a number", i+1)
		}
	}
	return out, nil
}
