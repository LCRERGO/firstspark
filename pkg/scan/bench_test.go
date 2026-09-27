package scan

import (
	"context"
	"testing"
)

func benchSession(mode ScanMode, typ ValueType, value string) *Session {
	v, _ := ParseValue(typ, value)
	opts := DefaultOptions()
	opts.Type = typ
	opts.Mode = mode
	opts.Alignment = 4
	opts.Value = v
	return NewSession(nil, opts)
}

func benchData() []byte {
	const n = 1 << 20
	data := make([]byte, n)
	for i := 0; i+4 <= n; i += 4096 {
		data[i], data[i+1], data[i+2], data[i+3] = 0xEF, 0xBE, 0xAD, 0xDE
	}
	return data
}

func BenchmarkScanBytesExact(b *testing.B) {
	s := benchSession(ModeExact, TypeDword, "0xDEADBEEF")
	data := benchData()
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	var matches int64
	for i := 0; i < b.N; i++ {
		var out []Result
		matches = 0
		if err := s.scanBytes(context.Background(), 0x1000, data, len(data), &out, &matches); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanBytesUnknown(b *testing.B) {
	s := benchSession(ModeUnknown, TypeDword, "0")
	data := benchData()
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	var matches int64
	for i := 0; i < b.N; i++ {
		out := make([]Result, 0, len(data)/4)
		matches = 0
		if err := s.scanBytes(context.Background(), 0x1000, data, len(data), &out, &matches); err != nil {
			b.Fatal(err)
		}
	}
}
