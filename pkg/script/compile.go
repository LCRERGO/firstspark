package script

import (
	"math"
	"strconv"
	"strings"
)

const varargName = "$varargs"

type flow uint8

const (
	flowNormal flow = iota
	flowBreak
	flowReturn
)

type exprFn func(*Env) Value
type multiFn func(*Env) []Value
type stmtFn func(*Env) (flow, []Value)
type assigner func(*Env, Value)

func compileExpr(e Expr) exprFn {
	switch e := e.(type) {
	case *NilLit:
		return func(*Env) Value { return Nil() }
	case *BoolLit:
		v := e.Value
		return func(*Env) Value { return Bool(v) }
	case *IntLit:
		v := e.Value
		return func(*Env) Value { return Int(v) }
	case *FloatLit:
		v := e.Value
		return func(*Env) Value { return Float(v) }
	case *StrLit:
		v := e.Value
		return func(*Env) Value { return Str(v) }
	case *Vararg:
		return func(env *Env) Value { return env.get(varargName) }
	case *Name:
		name := e.Name
		return func(env *Env) Value { return env.get(name) }
	case *Index:
		x := compileExpr(e.X)
		k := compileExpr(e.Key)
		return func(env *Env) Value { return index(x(env), k(env)) }
	case *Call:
		return compileCall(e)
	case *Unop:
		x := compileExpr(e.X)
		op := e.Op
		return func(env *Env) Value { return unop(op, x(env)) }
	case *Binop:
		return compileBinop(e)
	case *TableLit:
		return compileTable(e)
	case *FuncLit:
		return compileFuncLit(e)
	default:
		throw("unknown expression")
	}
	return nil
}

func compileMulti(e Expr) multiFn {
	switch e := e.(type) {
	case *Call:
		return compileCallMulti(e)
	case *Vararg:
		return func(env *Env) []Value { return tableValues(env.get(varargName)) }
	default:
		f := compileExpr(e)
		return func(env *Env) []Value { return []Value{f(env)} }
	}
}

func compileValues(exprs []Expr) multiFn {
	if len(exprs) == 0 {
		return func(*Env) []Value { return nil }
	}
	fns := make([]multiFn, len(exprs))
	for i, e := range exprs {
		if i == len(exprs)-1 {
			fns[i] = compileMulti(e)
		} else {
			f := compileExpr(e)
			fns[i] = func(env *Env) []Value { return []Value{f(env)} }
		}
	}
	return func(env *Env) []Value {
		var out []Value
		for _, f := range fns {
			out = append(out, f(env)...)
		}
		return out
	}
}

func compileCall(c *Call) exprFn {
	m := compileCallMulti(c)
	return func(env *Env) Value { return first(m(env)) }
}

func compileCallMulti(c *Call) multiFn {
	if c.Method != "" {
		idx, ok := c.Fn.(*Index)
		if !ok {
			throw("malformed method call")
		}
		base := compileExpr(idx.X)
		argFn := compileValues(c.Args)
		method := c.Method
		return func(env *Env) []Value {
			self := base(env)
			fn := methodLookup(self, method)
			args := append([]Value{self}, argFn(env)...)
			return callValue(fn, env, args)
		}
	}
	fnFn := compileExpr(c.Fn)
	argFn := compileValues(c.Args)
	return func(env *Env) []Value { return callValue(fnFn(env), env, argFn(env)) }
}

func callValue(fn Value, env *Env, args []Value) []Value {
	if fn.kind != KindFunction {
		throw("attempt to call a %s value", typeName(fn))
	}
	return fn.fn.call(env, args)
}

func first(vs []Value) Value {
	if len(vs) == 0 {
		return Nil()
	}
	return vs[0]
}

func tableValues(v Value) []Value {
	if v.kind == KindTable {
		return v.t.Array()
	}
	return nil
}

func compileTable(e *TableLit) exprFn {
	fields := make([]struct {
		key, val exprFn
	}, len(e.Fields))
	for i, f := range e.Fields {
		var key exprFn
		if f.Key != nil {
			key = compileExpr(f.Key)
		}
		fields[i] = struct {
			key, val exprFn
		}{key: key, val: compileExpr(f.Value)}
	}
	return func(env *Env) Value {
		t := NewTable()
		for _, f := range fields {
			if f.key == nil {
				t.Append(f.val(env))
			} else {
				t.Set(f.key(env), f.val(env))
			}
		}
		return TableVal(t)
	}
}

func compileFuncLit(fn *FuncLit) exprFn {
	params := fn.Params
	vararg := fn.Vararg
	body := compileStmts(fn.Body)
	return func(env *Env) Value {
		closure := env
		f := &Function{
			call: func(_ *Env, args []Value) []Value {
				local := newEnv(closure)
				for i, p := range params {
					if i < len(args) {
						local.setLocal(p, args[i])
					} else {
						local.setLocal(p, Nil())
					}
				}
				if vararg {
					t := NewTable()
					for i := len(params); i < len(args); i++ {
						t.Append(args[i])
					}
					local.setLocal(varargName, TableVal(t))
				}
				fl, rets := body(local)
				if fl == flowReturn {
					return rets
				}
				return nil
			},
		}
		return FuncVal(f)
	}
}

func compileAssigner(t Expr) assigner {
	switch t := t.(type) {
	case *Name:
		name := t.Name
		return func(env *Env, v Value) { env.set(name, v) }
	case *Index:
		objFn := compileExpr(t.X)
		keyFn := compileExpr(t.Key)
		return func(env *Env, v Value) { setIndex(objFn(env), keyFn(env), v) }
	default:
		throw("invalid assignment target")
	}
	return nil
}

func compileStmts(stmts []Stmt) stmtFn {
	fns := make([]stmtFn, len(stmts))
	for i, s := range stmts {
		fns[i] = compileStmt(s)
	}
	return func(env *Env) (flow, []Value) {
		for _, f := range fns {
			fl, rets := f(env)
			if fl != flowNormal {
				return fl, rets
			}
		}
		return flowNormal, nil
	}
}

func compileStmt(s Stmt) stmtFn {
	switch s := s.(type) {
	case *LocalStmt:
		names := s.Names
		valFn := compileValues(s.Exprs)
		return func(env *Env) (flow, []Value) {
			vals := valFn(env)
			for i, n := range names {
				if i < len(vals) {
					env.setLocal(n, vals[i])
				} else {
					env.setLocal(n, Nil())
				}
			}
			return flowNormal, nil
		}
	case *AssignStmt:
		targets := make([]assigner, len(s.Targets))
		for i, t := range s.Targets {
			targets[i] = compileAssigner(t)
		}
		valFn := compileValues(s.Exprs)
		return func(env *Env) (flow, []Value) {
			vals := valFn(env)
			for i, a := range targets {
				if i < len(vals) {
					a(env, vals[i])
				} else {
					a(env, Nil())
				}
			}
			return flowNormal, nil
		}
	case *LocalFuncStmt:
		name := s.Name
		fnFn := compileFuncLit(s.Fn)
		return func(env *Env) (flow, []Value) {
			env.setLocal(name, Nil())
			env.setLocal(name, fnFn(env))
			return flowNormal, nil
		}
	case *FuncStmt:
		a := compileAssigner(s.Target)
		fnFn := compileFuncLit(s.Fn)
		return func(env *Env) (flow, []Value) {
			a(env, fnFn(env))
			return flowNormal, nil
		}
	case *CallStmt:
		call, ok := s.Call.(*Call)
		if !ok {
			throw("malformed call statement")
		}
		fn := compileCallMulti(call)
		return func(env *Env) (flow, []Value) {
			fn(env)
			return flowNormal, nil
		}
	case *DoStmt:
		body := compileStmts(s.Body)
		return func(env *Env) (flow, []Value) { return body(newEnv(env)) }
	case *IfStmt:
		return compileIf(s)
	case *WhileStmt:
		cond := compileExpr(s.Cond)
		body := compileStmts(s.Body)
		return func(env *Env) (flow, []Value) {
			for truthy(cond(env)) {
				fl, rets := body(newEnv(env))
				if fl == flowBreak {
					break
				}
				if fl == flowReturn {
					return fl, rets
				}
			}
			return flowNormal, nil
		}
	case *RepeatStmt:
		body := compileStmts(s.Body)
		cond := compileExpr(s.Cond)
		return func(env *Env) (flow, []Value) {
			for {
				iter := newEnv(env)
				fl, rets := body(iter)
				if fl == flowBreak {
					break
				}
				if fl == flowReturn {
					return fl, rets
				}
				if truthy(cond(iter)) {
					break
				}
			}
			return flowNormal, nil
		}
	case *NumForStmt:
		startFn := compileExpr(s.Start)
		limitFn := compileExpr(s.Limit)
		var stepFn exprFn
		if s.Step != nil {
			stepFn = compileExpr(s.Step)
		}
		body := compileStmts(s.Body)
		name := s.Name
		return func(env *Env) (flow, []Value) {
			start := toNumber(startFn(env))
			limit := toNumber(limitFn(env))
			step := 1.0
			if stepFn != nil {
				step = toNumber(stepFn(env))
			}
			loop := newEnv(env)
			if step == 0 {
				throw("'for' step is zero")
			}
			for i := start; (step > 0 && i <= limit) || (step < 0 && i >= limit); i += step {
				loop.setLocal(name, numberValue(i))
				fl, rets := body(newEnv(loop))
				if fl == flowBreak {
					break
				}
				if fl == flowReturn {
					return fl, rets
				}
			}
			return flowNormal, nil
		}
	case *GenForStmt:
		exprFn := compileValues(s.Exprs)
		body := compileStmts(s.Body)
		names := s.Names
		return func(env *Env) (flow, []Value) {
			vals := exprFn(env)
			var iter, state, control Value
			if len(vals) > 0 {
				iter = vals[0]
			}
			if len(vals) > 1 {
				state = vals[1]
			}
			if len(vals) > 2 {
				control = vals[2]
			}
			for {
				rets := callValue(iter, env, []Value{state, control})
				if len(rets) == 0 || rets[0].IsNil() {
					break
				}
				control = rets[0]
				loop := newEnv(env)
				for i, n := range names {
					if i < len(rets) {
						loop.setLocal(n, rets[i])
					} else {
						loop.setLocal(n, Nil())
					}
				}
				fl, out := body(loop)
				if fl == flowBreak {
					break
				}
				if fl == flowReturn {
					return fl, out
				}
			}
			return flowNormal, nil
		}
	case *ReturnStmt:
		valFn := compileValues(s.Exprs)
		return func(env *Env) (flow, []Value) { return flowReturn, valFn(env) }
	case *BreakStmt:
		return func(*Env) (flow, []Value) { return flowBreak, nil }
	default:
		throw("unknown statement")
	}
	return nil
}

func compileIf(s *IfStmt) stmtFn {
	cond := compileExpr(s.Cond)
	then := compileStmts(s.Then)
	type branch struct {
		cond exprFn
		body stmtFn
	}
	branches := make([]branch, len(s.ElseIfs))
	for i, ei := range s.ElseIfs {
		branches[i] = branch{cond: compileExpr(ei.Cond), body: compileStmts(ei.Body)}
	}
	els := compileStmts(s.Else)
	return func(env *Env) (flow, []Value) {
		if truthy(cond(env)) {
			return then(newEnv(env))
		}
		for _, b := range branches {
			if truthy(b.cond(env)) {
				return b.body(newEnv(env))
			}
		}
		if els != nil {
			return els(newEnv(env))
		}
		return flowNormal, nil
	}
}

func compileBinop(e *Binop) exprFn {
	switch e.Op {
	case "and":
		l := compileExpr(e.L)
		r := compileExpr(e.R)
		return func(env *Env) Value {
			lv := l(env)
			if !truthy(lv) {
				return lv
			}
			return r(env)
		}
	case "or":
		l := compileExpr(e.L)
		r := compileExpr(e.R)
		return func(env *Env) Value {
			lv := l(env)
			if truthy(lv) {
				return lv
			}
			return r(env)
		}
	}
	l := compileExpr(e.L)
	r := compileExpr(e.R)
	op := e.Op
	return func(env *Env) Value { return binop(op, l(env), r(env)) }
}

func truthy(v Value) bool { return !(v.kind == KindNil || (v.kind == KindBool && !v.b)) }

func typeName(v Value) string {
	switch v.kind {
	case KindNil:
		return "nil"
	case KindBool:
		return "boolean"
	case KindInt, KindFloat:
		return "number"
	case KindString:
		return "string"
	case KindTable:
		return "table"
	case KindFunction:
		return "function"
	case KindObject:
		return "userdata"
	default:
		return "?"
	}
}

func index(obj, key Value) Value {
	if obj.kind == KindTable {
		return obj.t.Get(key)
	}
	if obj.kind == KindObject {
		if v, ok := obj.obj.Index(key); ok {
			return v
		}
		throw("attempt to index a %s value", typeName(obj))
	}
	throw("attempt to index a %s value", typeName(obj))
	return Nil()
}

func setIndex(obj, key, v Value) {
	switch obj.kind {
	case KindTable:
		obj.t.Set(key, v)
	case KindObject:
		if err := obj.obj.SetIndex(key, v); err != nil {
			throw("%s", err.Error())
		}
	default:
		throw("attempt to index a %s value", typeName(obj))
	}
}

func methodLookup(self Value, name string) Value {
	switch self.kind {
	case KindTable:
		return self.t.Get(Str(name))
	case KindObject:
		if fn, ok := self.obj.Index(Str(name)); ok {
			return fn
		}
	case KindString:
		if fn := stringLib.Get(Str(name)); !fn.IsNil() {
			return fn
		}
	}
	throw("attempt to index a %s value", typeName(self))
	return Nil()
}

func numberValue(f float64) Value {
	if f == math.Trunc(f) && f >= math.MinInt64 && f <= math.MaxInt64 {
		return Int(int64(f))
	}
	return Float(f)
}

func toNumber(v Value) float64 {
	switch v.kind {
	case KindInt:
		return float64(v.i)
	case KindFloat:
		return v.f
	case KindString:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v.s), 64); err == nil {
			return f
		}
	}
	throw("attempt to perform arithmetic on a %s value", typeName(v))
	return 0
}

func binop(op string, a, b Value) Value {
	switch op {
	case "+", "-", "*":
		if a.kind == KindInt && b.kind == KindInt {
			var r int64
			var ok bool
			switch op {
			case "+":
				r, ok = addInt(a.i, b.i)
			case "-":
				r, ok = subInt(a.i, b.i)
			case "*":
				r, ok = mulInt(a.i, b.i)
			}
			if ok {
				return Int(r)
			}
		}
		x, y := toNumber(a), toNumber(b)
		switch op {
		case "+":
			return Float(x + y)
		case "-":
			return Float(x - y)
		default:
			return Float(x * y)
		}
	case "/":
		return Float(toNumber(a) / toNumber(b))
	case "^":
		return Float(math.Pow(toNumber(a), toNumber(b)))
	case "%":
		if a.kind == KindInt && b.kind == KindInt && b.i != 0 {
			r := a.i % b.i
			if r != 0 && (r < 0) != (b.i < 0) {
				r += b.i
			}
			return Int(r)
		}
		x, y := toNumber(a), toNumber(b)
		return Float(x - math.Floor(x/y)*y)
	case "..":
		return Str(concat(a) + concat(b))
	case "==":
		return Bool(equal(a, b))
	case "~=":
		return Bool(!equal(a, b))
	case "<", "<=", ">", ">=":
		return Bool(compare(op, a, b))
	default:
		throw("unknown operator %s", op)
	}
	return Nil()
}

func concat(v Value) string {
	switch v.kind {
	case KindString:
		return v.s
	case KindInt, KindFloat:
		return v.String()
	}
	throw("attempt to concatenate a %s value", typeName(v))
	return ""
}

func equal(a, b Value) bool {
	if a.IsNumber() && b.IsNumber() {
		return toNumber(a) == toNumber(b)
	}
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case KindNil:
		return true
	case KindBool:
		return a.b == b.b
	case KindString:
		return a.s == b.s
	case KindTable:
		return a.t == b.t
	case KindFunction:
		return a.fn == b.fn
	}
	return false
}

func compare(op string, a, b Value) bool {
	if a.IsNumber() && b.IsNumber() {
		x, y := toNumber(a), toNumber(b)
		switch op {
		case "<":
			return x < y
		case "<=":
			return x <= y
		case ">":
			return x > y
		default:
			return x >= y
		}
	}
	if a.kind == KindString && b.kind == KindString {
		switch op {
		case "<":
			return a.s < b.s
		case "<=":
			return a.s <= b.s
		case ">":
			return a.s > b.s
		default:
			return a.s >= b.s
		}
	}
	throw("attempt to compare %s with %s", typeName(a), typeName(b))
	return false
}

func unop(op string, x Value) Value {
	switch op {
	case "-":
		if x.kind == KindInt {
			if x.i == math.MinInt64 {
				return Float(-float64(x.i))
			}
			return Int(-x.i)
		}
		if x.kind == KindFloat {
			return Float(-x.f)
		}
		return Float(-toNumber(x))
	case "not":
		return Bool(!truthy(x))
	case "#":
		switch x.kind {
		case KindString:
			return Int(int64(len(x.s)))
		case KindTable:
			return Int(int64(x.t.Len()))
		}
		throw("attempt to get length of a %s value", typeName(x))
	}
	throw("unknown unary operator %s", op)
	return Nil()
}

func addInt(a, b int64) (int64, bool) {
	r := a + b
	if (a > 0 && b > 0 && r < 0) || (a < 0 && b < 0 && r >= 0) {
		return 0, false
	}
	return r, true
}

func subInt(a, b int64) (int64, bool) {
	r := a - b
	if (b < 0 && r < a) || (b > 0 && r > a) {
		return 0, false
	}
	return r, true
}

func mulInt(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	r := a * b
	if r/b != a {
		return 0, false
	}
	return r, true
}
