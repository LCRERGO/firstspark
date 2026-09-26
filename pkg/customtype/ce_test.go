package customtype

import (
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

func TestRegisterCEUnsupported(t *testing.T) {
	cases := []CEConversion{
		{UsesFloat: true, Size: 4, ConvertRoutine: "ret"},
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
