package script

import "testing"

type counter struct{ n int64 }

func (c *counter) Index(key Value) (Value, bool) {
	switch key.Str() {
	case "value":
		return Int(c.n), true
	case "add":
		return GoFunc("add", func(args []Value) ([]Value, error) {
			if len(args) >= 2 {
				c.n += args[1].Int()
			}
			return []Value{Int(c.n)}, nil
		}), true
	}
	return Nil(), false
}

func (c *counter) SetIndex(key, value Value) error {
	if key.Str() == "value" {
		c.n = value.Int()
	}
	return nil
}

func TestForeignObject(t *testing.T) {
	c := &counter{}
	p, err := CompileWithGlobals(`
obj.value = 5
obj:add(10)
result = obj.value
`, map[string]Value{"obj": ObjectVal(c)})
	if err != nil {
		t.Fatalf("CompileWithGlobals: %v", err)
	}
	if got := p.Global("result").Int(); got != 15 {
		t.Fatalf("result = %d, want 15", got)
	}
	if c.n != 15 {
		t.Fatalf("object state = %d, want 15", c.n)
	}
}

func TestForeignUnknownMemberErrors(t *testing.T) {
	_, err := CompileWithGlobals(`x = obj.missing`, map[string]Value{"obj": ObjectVal(&counter{})})
	if err == nil {
		t.Fatal("expected an error for an unknown member")
	}
}

func TestGoFuncErrorPropagates(t *testing.T) {
	_, err := CompileWithGlobals(`boom()`, map[string]Value{
		"boom": GoFunc("boom", func([]Value) ([]Value, error) {
			return nil, errTest
		}),
	})
	if err == nil {
		t.Fatal("expected the Go error to propagate")
	}
}

type testError struct{}

func (testError) Error() string { return "boom" }

var errTest = testError{}
