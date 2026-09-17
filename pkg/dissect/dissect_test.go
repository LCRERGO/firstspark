package dissect

import (
	"encoding/binary"
	"math"
	"testing"
)

func guessAll(buf []byte, r Range) []Field {
	var out []Field
	for off := 0; off < len(buf); {
		f := guess(off, [][]byte{buf}, r)
		out = append(out, f)
		off += f.Size
	}
	return out
}

func TestGuessLayout(t *testing.T) {
	buf := make([]byte, 17)
	binary.LittleEndian.PutUint64(buf[0:], 0x10000)
	binary.LittleEndian.PutUint32(buf[8:], math.Float32bits(1.5))
	binary.LittleEndian.PutUint32(buf[12:], 42)
	buf[16] = 7

	fields := guessAll(buf, Range{Min: 0x1000, Max: 0x20000})
	if len(fields) != 4 {
		t.Fatalf("fields = %+v", fields)
	}
	want := []struct {
		off  int
		kind Kind
	}{{0, KindPointer}, {8, KindFloat}, {12, KindDword}, {16, KindByte}}
	for i, w := range want {
		if fields[i].Offset != w.off || fields[i].Kind != w.kind {
			t.Fatalf("field %d = %+v, want offset %d kind %v", i, fields[i], w.off, w.kind)
		}
	}
}

func TestFormat(t *testing.T) {
	buf := make([]byte, 17)
	binary.LittleEndian.PutUint64(buf[0:], 0x10000)
	binary.LittleEndian.PutUint32(buf[8:], math.Float32bits(1.5))
	binary.LittleEndian.PutUint32(buf[12:], 42)
	buf[16] = 7
	fields := guessAll(buf, Range{Min: 0x1000, Max: 0x20000})
	got := []string{fields[0].Format(0), fields[1].Format(0), fields[2].Format(0), fields[3].Format(0)}
	want := []string{"0x10000", "1.5", "42", "7"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("format[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFormatShortRead(t *testing.T) {
	for _, kind := range []Kind{KindPointer, KindDouble, KindQword, KindFloat, KindDword, KindWord, KindByte, KindBytes} {
		f := Field{Kind: kind, Size: 8, Values: [][]byte{nil}}
		if got := f.Format(0); got != "" {
			t.Errorf("Format(%v) with short read = %q, want empty", kind, got)
		}
	}
}

func TestScanTypeMapping(t *testing.T) {
	if KindPointer.ScanType() != 8 && KindPointer.ScanType().String() != "qword" {
		t.Fatalf("pointer scan type = %v", KindPointer.ScanType())
	}
	if KindDword.ScanType().String() != "dword" {
		t.Fatalf("dword scan type = %v", KindDword.ScanType())
	}
}
