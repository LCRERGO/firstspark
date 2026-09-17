package scan

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/LCRERGO/firstspark/pkg/script"
)

const (
	maxInt64AsFloat = float64(1 << 63)
	minInt64AsFloat = -float64(1 << 63)
)

// parseIntInput parses a plain integer literal, falling back to a single Lua
// expression when the literal does not parse (for example "360 * (10 ^ 6)").
func parseIntInput(s string) (int64, error) {
	if n, err := parseInteger(s); err == nil {
		return n, nil
	}
	v, err := evalExpression(s)
	if err != nil {
		return 0, err
	}
	return expressionInt64(v)
}

// parseFloatInput parses a plain float literal, falling back to a single Lua
// expression when the literal does not parse.
func parseFloatInput(s string) (float64, error) {
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return f, nil
	}
	v, err := evalExpression(s)
	if err != nil {
		return 0, err
	}
	return expressionFloat64(v)
}

// evalExpression evaluates input as a single Lua expression. It is wrapped in
// a function so the scripting language's expression syntax (arithmetic,
// parentheses, math.* calls) is available without exposing a statement chunk.
func evalExpression(input string) (script.Value, error) {
	src := "function __firstspark_value() return (" + input + ") end"
	prog, err := script.Compile(src)
	if err != nil {
		return script.Nil(), err
	}
	out, err := prog.Call("__firstspark_value")
	if err != nil {
		return script.Nil(), err
	}
	if len(out) == 0 {
		return script.Nil(), fmt.Errorf("scan: expression %q produced no value", input)
	}
	return out[0], nil
}

func expressionInt64(v script.Value) (int64, error) {
	switch v.Kind() {
	case script.KindInt:
		return v.Int(), nil
	case script.KindFloat:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
			return 0, fmt.Errorf("scan: expression result %s is not an integer", v.String())
		}
		if f < minInt64AsFloat || f >= maxInt64AsFloat {
			return 0, fmt.Errorf("scan: expression result %s overflows int64", v.String())
		}
		return int64(f), nil
	default:
		return 0, fmt.Errorf("scan: expression result %s is not a number", v.String())
	}
}

func expressionFloat64(v script.Value) (float64, error) {
	f, ok := v.Number()
	if !ok {
		return 0, fmt.Errorf("scan: expression result %s is not a number", v.String())
	}
	return f, nil
}
