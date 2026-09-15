package customtype

import (
	"path/filepath"
	"testing"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

const moneyScript = `
function bytes_to_value(bytes, address)
	local raw = bytes[1] + bytes[2]*256 + bytes[3]*65536 + bytes[4]*16777216
	return raw / 100
end
function value_to_bytes(value, address)
	local raw = math.floor(value * 100)
	return { raw % 256, math.floor(raw/256) % 256, math.floor(raw/65536) % 256, math.floor(raw/16777216) % 256 }
end`

func TestRegisterAndRoundTrip(t *testing.T) {
	typ, err := Register(Definition{Name: "Money", Size: 4, Kind: "float", Script: moneyScript})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got, ok := scan.LookupType("money"); !ok || got.ID != typ.ID {
		t.Fatalf("LookupType failed: %v", ok)
	}
	v := scan.NewValue(typ.ID, []byte{0x39, 0x30, 0, 0})
	if v.String() != "123.45" {
		t.Fatalf("format = %q", v.String())
	}
	parsed, err := scan.ParseValue(typ.ID, "123.45")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if parsed.String() != "123.45" {
		t.Fatalf("round trip = %q", parsed.String())
	}
}

func TestAlignmentDefaultsToSize(t *testing.T) {
	const firstByte = `function bytes_to_value(bytes) return bytes[1] end`
	typ, err := Register(Definition{Name: "Aligned8", Size: 8, Kind: "int", Alignment: 8, Script: firstByte})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := scan.TypeByID(typ.ID).Alignment; got != 8 {
		t.Fatalf("alignment = %d", got)
	}
	typ2, err := Register(Definition{Name: "AlignedDefault", Size: 3, Kind: "int", Script: firstByte})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := scan.TypeByID(typ2.ID).Alignment; got != 3 {
		t.Fatalf("default alignment = %d", got)
	}
}

func TestRegisterAAType(t *testing.T) {
	def := Definition{
		Name: "AA First Dword", Size: 4, Kind: "int", Mode: "aa",
		Script: "[ENABLE]\nConvertRoutine:\n  mov eax, [rdi]\n  ret\nConvertBackRoutine:\n  mov eax, edi\n  mov [rsi], eax\n  ret\n",
	}
	typ, err := RegisterAA(def)
	if err != nil {
		t.Fatalf("RegisterAA: %v", err)
	}
	v := scan.NewValue(typ.ID, []byte{7, 0, 0, 0})
	if v.String() != "7" {
		t.Fatalf("format = %q", v.String())
	}
	parsed, err := scan.ParseValue(typ.ID, "9")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if parsed.String() != "9" {
		t.Fatalf("round trip = %q", parsed.String())
	}
}

func TestAATypeRejectsFloat(t *testing.T) {
	if _, err := RegisterAA(Definition{Name: "AA Float", Size: 4, Kind: "float", Mode: "aa", Script: "[ENABLE]\nConvertRoutine:\n  ret\n"}); err == nil {
		t.Fatal("expected float AA types to be rejected")
	}
}

func TestReadOnlyType(t *testing.T) {
	typ, err := Register(Definition{
		Name: "ReadOnly", Size: 2, Kind: "int",
		Script: `function bytes_to_value(bytes) return bytes[1] + bytes[2]*256 end`,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := scan.ParseValue(typ.ID, "5"); err == nil {
		t.Fatal("expected a read-only error")
	}
}

func TestRejectsBadScript(t *testing.T) {
	if _, err := Register(Definition{Name: "Bad", Size: 2, Kind: "int", Script: `function bytes_to_value(`}); err == nil {
		t.Fatal("expected a parse error")
	}
	if _, err := Register(Definition{Name: "NoFn", Size: 2, Kind: "int", Script: `local x = 1`}); err == nil {
		t.Fatal("expected a missing-function error")
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "customtypes.yaml")
	defs := []Definition{{Name: "Money", Size: 4, Kind: "float", Script: moneyScript, Description: "cents"}}
	if err := Save(path, defs); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Money" || got[0].Description != "cents" {
		t.Fatalf("round trip = %+v", got)
	}
}
