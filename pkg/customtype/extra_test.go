package customtype

import (
	"testing"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

func TestRegisterLuaStringReadOnly(t *testing.T) {
	typ, err := Register(Definition{
		Name: "Lua Str", Size: 2, Kind: "string",
		Script: `function bytes_to_value(bytes) return "hi" end`,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := typ.Format(scan.Value{Type: typ.ID, Raw: []byte{0, 0}}); got != "hi" {
		t.Fatalf("Format = %q", got)
	}
	if got := typ.Text(scan.Value{Type: typ.ID, Raw: []byte{0, 0}}); got != "hi" {
		t.Fatalf("Text = %q", got)
	}
	if _, err := typ.Parse("z"); err == nil {
		t.Fatal("expected a read-only error")
	}
}

func TestValidate(t *testing.T) {
	ok := Definition{Name: "v", Size: 4, Kind: "int", Mode: "aa",
		Script: "[ENABLE]\nConvertRoutine:\n  mov eax, [rdi]\n  ret\n"}
	if err := Validate(ok); err != nil {
		t.Fatalf("Validate(int): %v", err)
	}
	bad := Definition{Name: "v2", Size: 4, Kind: "int", Mode: "aa",
		Script: "[ENABLE]\nConvertRoutine:\n  definitely_not_an_opcode\n  ret\n"}
	if err := Validate(bad); err == nil {
		t.Fatal("expected an error for an invalid script")
	}
	lua := Definition{Name: "v3", Size: 1, Kind: "int",
		Script: `function bytes_to_value(bytes) return bytes[1] end`}
	if err := Validate(lua); err != nil {
		t.Fatalf("Validate(lua): %v", err)
	}
}

func TestAATest(t *testing.T) {
	def := Definition{Name: "at", Size: 4, Kind: "int", Mode: "aa",
		Script: "[ENABLE]\nConvertRoutine:\n  mov eax, [rdi]\n  ret\nConvertBackRoutine:\n  mov eax, edi\n  mov [rsi], eax\n  ret\n"}
	v, back, err := AATest(def, []byte{5, 0, 0, 0})
	if err != nil {
		t.Fatalf("AATest: %v", err)
	}
	if v != 5 || len(back) != 4 || back[0] != 5 {
		t.Fatalf("AATest = %d, % x", v, back)
	}
}

func TestRegisterRawRejects(t *testing.T) {
	if _, err := RegisterRaw("", 4); err == nil {
		t.Fatal("expected an error for an empty name")
	}
	if _, err := RegisterRaw("x", 3); err == nil {
		t.Fatal("expected an error for an unsupported size")
	}
	typ, err := RegisterRaw("Raw Byte", 1)
	if err != nil {
		t.Fatalf("RegisterRaw: %v", err)
	}
	if got := typ.Int64(scan.Value{Type: typ.ID, Raw: []byte{0xff}}); got != -1 {
		t.Fatalf("Int64 = %d, want -1", got)
	}
	if got := typ.Format(scan.Value{Type: typ.ID, Raw: []byte{0xff}}); got != "-1" {
		t.Fatalf("Format = %q", got)
	}
}

func TestRegisterRawRejectsOutOfRange(t *testing.T) {
	typ, err := RegisterRaw("Raw Word", 2)
	if err != nil {
		t.Fatalf("RegisterRaw: %v", err)
	}
	if _, err := typ.Parse("65536"); err == nil {
		t.Fatal("expected an out-of-range error")
	}
	if _, err := typ.Parse("65535"); err != nil {
		t.Fatalf("boundary value rejected: %v", err)
	}
}
