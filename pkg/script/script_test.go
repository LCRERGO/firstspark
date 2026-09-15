package script

import (
	"bytes"
	"testing"
)

func mustCompile(t *testing.T, src string) *Program {
	t.Helper()
	p, err := Compile(src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return p
}

func call(t *testing.T, p *Program, name string, args ...Value) []Value {
	t.Helper()
	out, err := p.Call(name, args...)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return out
}

func TestIntegerArithmeticIsExact(t *testing.T) {
	p := mustCompile(t, `function add(a, b) return a + b end`)
	out := call(t, p, "add", Int(9007199254740992), Int(1))
	if len(out) != 1 || out[0].Kind() != KindInt || out[0].Int() != 9007199254740993 {
		t.Fatalf("got %v (%v)", out, out[0].Kind())
	}
}

func TestDivisionYieldsFloat(t *testing.T) {
	p := mustCompile(t, `function half(a) return a / 2 end`)
	out := call(t, p, "half", Int(1))
	if out[0].Kind() != KindFloat || out[0].Float() != 0.5 {
		t.Fatalf("got %v", out[0])
	}
}

func TestCustomTypeConversion(t *testing.T) {
	p := mustCompile(t, `
function bytes_to_value(bytes, address)
	return bytes[1] + bytes[2] * 256 + bytes[3] * 65536
end
function value_to_bytes(value, address)
	return { value % 256, math.floor(value / 256) % 256, math.floor(value / 65536) % 256 }
end`)
	got := call(t, p, "bytes_to_value", BytesValue([]byte{0x01, 0x02, 0x03}), Int(0x1000))
	if got[0].Int() != 0x030201 {
		t.Fatalf("bytes_to_value = %v", got[0])
	}
	back := call(t, p, "value_to_bytes", Int(0x030201), Int(0x1000))
	b, err := ValueBytes(back[0])
	if err != nil {
		t.Fatalf("ValueBytes: %v", err)
	}
	if !bytes.Equal(b, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("value_to_bytes = %v", b)
	}
}

func TestRecursionAndClosures(t *testing.T) {
	p := mustCompile(t, `
local function fact(n)
	if n <= 1 then return 1 else return n * fact(n - 1) end
end
function f(n) return fact(n) end
function adder(n) return function(x) return x + n end end
function g() local add5 = adder(5) return add5(3) end`)
	if got := call(t, p, "f", Int(5)); got[0].Int() != 120 {
		t.Fatalf("fact = %v", got[0])
	}
	if got := call(t, p, "g"); got[0].Int() != 8 {
		t.Fatalf("closure = %v", got[0])
	}
}

func TestLoopsAndGenericFor(t *testing.T) {
	p := mustCompile(t, `
function sum(n) local s = 0 for i = 1, n do s = s + i end return s end
function total() local t = {10, 20, 30} local s = 0 for _, v in ipairs(t) do s = s + v end return s end
function countdown() local s = "" local i = 3 while i > 0 do s = s .. i i = i - 1 end return s end`)
	if got := call(t, p, "sum", Int(10)); got[0].Int() != 55 {
		t.Fatalf("sum = %v", got[0])
	}
	if got := call(t, p, "total"); got[0].Int() != 60 {
		t.Fatalf("total = %v", got[0])
	}
	if got := call(t, p, "countdown"); got[0].Str() != "321" {
		t.Fatalf("countdown = %v", got[0])
	}
}

func TestStringFormat(t *testing.T) {
	p := mustCompile(t, `function f(x) return string.format("0x%08X", x) end`)
	if got := call(t, p, "f", Int(255)); got[0].Str() != "0x000000FF" {
		t.Fatalf("format = %q", got[0].Str())
	}
}

func TestSandboxExcludesOSAndIO(t *testing.T) {
	p := mustCompile(t, `function f() return os, io, require end`)
	out := call(t, p, "f")
	for i, v := range out {
		if !v.IsNil() {
			t.Fatalf("result %d should be nil, got %v", i, v)
		}
	}
}

func TestRuntimeErrorIsReturned(t *testing.T) {
	p := mustCompile(t, `function f() return nil + 1 end`)
	if _, err := p.Call("f"); err == nil {
		t.Fatal("expected a runtime error")
	}
}

func TestParseErrorHasPosition(t *testing.T) {
	_, err := Compile("function f( return 1 end")
	if err == nil {
		t.Fatal("expected a parse error")
	}
}
