package scan

import "testing"

func FuzzParseAOB(f *testing.F) {
	f.Add("48 8B ?? E5")
	f.Add("488B??E5")
	f.Add("0x48,0x8b")
	f.Add("")
	f.Add("zz zz")
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParseAOB(s)
		if err == nil && p != nil {
			_ = p.Match([]byte{0x48, 0x8B, 0x00, 0xE5})
			_ = p.Len()
		}
	})
}

func FuzzParseBinary(f *testing.F) {
	f.Add("1010??11")
	f.Add("1 0 1")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParseBinary(s)
		if err == nil && p != nil {
			_ = p.Match([]byte{0xFF, 0x00})
			_ = FormatBinary(Value{Type: TypeBinary, Raw: p.Bytes, Mask: p.Mask, Bits: p.Bits})
		}
	})
}

func FuzzParseValue(f *testing.F) {
	f.Add("dword", "1000")
	f.Add("aob", "48 8B ?? E5")
	f.Add("double", "10 / 4")
	f.Add("string", `"hello"`)
	f.Fuzz(func(t *testing.T, typeName, input string) {
		typ, err := ParseValueType(typeName)
		if err != nil {
			return
		}
		_, _ = ParseValue(typ, input)
	})
}

func FuzzParseCompareOp(f *testing.F) {
	f.Add("==")
	f.Add(">=")
	f.Add("bogus")
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseCompareOp(s)
	})
}

func FuzzParseScanMode(f *testing.F) {
	f.Add("exact")
	f.Add("increased by")
	f.Add("bogus")
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseScanMode(s)
	})
}
