package customtype

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/LCRERGO/firstspark/pkg/scan"
)

func TestRegisterCEInteger(t *testing.T) {
	conv := CEConversion{
		Name:               "fs_ce_incr",
		Size:               4,
		Alignment:          4,
		ConvertRoutine:     "mov eax, [rcx]\nadd eax, 1",
		ConvertBackRoutine: "mov eax, ecx\nsub eax, 1\nmov [rdx], eax",
	}
	typ, err := RegisterCE(conv)
	if err != nil {
		if strings.Contains(err.Error(), "not supported in this build") {
			t.Skip("jit unavailable in this build")
		}
		t.Fatalf("RegisterCE: %v", err)
	}
	if got := typ.Int64(scan.Value{Type: typ.ID, Raw: []byte{9, 0, 0, 0}}); got != 10 {
		t.Fatalf("Int64 = %d, want 10", got)
	}
	back, err := typ.Parse("10")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(back.Raw) != 4 || back.Raw[0] != 9 {
		t.Fatalf("Parse back = %v, want 9,0,0,0", back.Raw)
	}
}

func TestRegisterCEFloat(t *testing.T) {
	// Identity routines: the raw bits pass through, and the type interprets
	// them as a single-precision float.
	conv := CEConversion{
		Name:               "fs_ce_float",
		Size:               4,
		Alignment:          4,
		UsesFloat:          true,
		ConvertRoutine:     "mov eax, [rcx]",
		ConvertBackRoutine: "mov eax, ecx\nmov [rdx], eax",
	}
	typ, err := RegisterCE(conv)
	if err != nil {
		if strings.Contains(err.Error(), "not supported in this build") {
			t.Skip("jit unavailable in this build")
		}
		t.Fatalf("RegisterCE: %v", err)
	}
	if typ.Kind != scan.KindFloat {
		t.Fatalf("kind = %v, want float", typ.Kind)
	}
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint32(raw, math.Float32bits(1.5))
	if got := typ.Numeric(scan.Value{Type: typ.ID, Raw: raw}); got != 1.5 {
		t.Fatalf("Numeric = %v, want 1.5", got)
	}
	if got := typ.Format(scan.Value{Type: typ.ID, Raw: raw}); got != "1.5" {
		t.Fatalf("Format = %q, want 1.5", got)
	}
	parsed, err := typ.Parse("2.5")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := binary.LittleEndian.Uint32(parsed.Raw); got != math.Float32bits(2.5) {
		t.Fatalf("Parse bits = %#x, want %#x", got, math.Float32bits(2.5))
	}
}

func TestRegisterCEString(t *testing.T) {
	// A 4-byte string type whose routine copies the four bytes through, with a
	// NUL-terminated output buffer on read.
	conv := CEConversion{
		Name:               "fs_ce_str4",
		Size:               4,
		MaxStringSize:      8,
		UsesString:         true,
		ConvertRoutine:     "mov eax, [rcx]\nmov [r8], eax",
		ConvertBackRoutine: "mov eax, [rcx]\nmov [r8], eax",
	}
	typ, err := RegisterCE(conv)
	if err != nil {
		if strings.Contains(err.Error(), "not supported in this build") {
			t.Skip("jit unavailable in this build")
		}
		t.Fatalf("RegisterCE: %v", err)
	}
	if typ.Kind != scan.KindString {
		t.Fatalf("kind = %v, want string", typ.Kind)
	}
	if got := typ.Text(scan.Value{Type: typ.ID, Raw: []byte("ABCD")}); got != "ABCD" {
		t.Fatalf("Text = %q, want ABCD", got)
	}
	if got := typ.Format(scan.Value{Type: typ.ID, Raw: []byte("abcd")}); got != "abcd" {
		t.Fatalf("Format = %q, want abcd", got)
	}
	parsed, err := typ.Parse("WXYZ")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if string(parsed.Raw) != "WXYZ" {
		t.Fatalf("Parse = %q, want WXYZ", parsed.Raw)
	}
}

func TestRegisterCEInterpreter(t *testing.T) {
	// Force the pure-Go interpreter to cover the CGO-free path (ADR 0048 P4).
	forceInterpreter = true
	defer func() { forceInterpreter = false }()

	conv := CEConversion{
		Name:               "fs_ce_incr_interp",
		Size:               4,
		ConvertRoutine:     "mov eax, [rcx]\nadd eax, 1",
		ConvertBackRoutine: "mov eax, ecx\nsub eax, 1\nmov [rdx], eax",
	}
	typ, err := RegisterCE(conv)
	if err != nil {
		t.Fatalf("RegisterCE: %v", err)
	}
	if got := typ.Int64(scan.Value{Type: typ.ID, Raw: []byte{9, 0, 0, 0}}); got != 10 {
		t.Fatalf("Int64 = %d, want 10", got)
	}
	back, err := typ.Parse("10")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(back.Raw) != 4 || back.Raw[0] != 9 {
		t.Fatalf("Parse back = %v, want 9,0,0,0", back.Raw)
	}
}

func TestRegisterCEUnsupported(t *testing.T) {
	cases := []CEConversion{
		{UsesFloat: true, Size: 8, ConvertRoutine: "ret"},
		{UsesString: true, Size: 4, ConvertRoutine: "ret"},
		{MaxStringSize: 32, Size: 4, ConvertRoutine: "ret"},
		{Size: 4, ConvertRoutine: ""},
	}
	for i, c := range cases {
		if _, err := RegisterCE(c); err == nil {
			t.Fatalf("case %d: expected an error", i)
		}
	}
}
