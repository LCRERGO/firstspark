package scan

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

func TestCapped(t *testing.T) {
	s := &Session{opts: Options{MaxResults: 2}}
	var m int64
	if s.capped(&m) {
		t.Fatal("should not cap at zero matches")
	}
	atomic.AddInt64(&m, 2)
	if !s.capped(&m) {
		t.Fatal("should cap at the limit")
	}
}

func TestRegionScope(t *testing.T) {
	s := &Session{opts: Options{Scope: ScopeHeapStackExecBSS}}
	cases := []struct {
		path string
		want bool
	}{
		{"[heap]", true},
		{"[stack]", true},
		{"", true},
		{"/usr/lib/libc.so", false},
		{"/app/game", true},
	}
	for _, c := range cases {
		if got := s.regionInScope(mem.Region{Path: c.path}, "/app/game"); got != c.want {
			t.Errorf("regionInScope(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestMatchBetween(t *testing.T) {
	s := &Session{opts: Options{
		Type:    TypeDword,
		Mode:    ModeBetween,
		Value:   Value{Type: TypeDword, Raw: encodeInteger(TypeDword, 10)},
		Value2:  Value{Type: TypeDword, Raw: encodeInteger(TypeDword, 20)},
		Epsilon: 1e-6,
	}}
	cases := []struct {
		v    int64
		want bool
	}{{9, false}, {10, true}, {15, true}, {20, true}, {21, false}}
	for _, c := range cases {
		if got := s.matchBetween(encodeInteger(TypeDword, c.v)); got != c.want {
			t.Errorf("between(%d) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestMatchBetweenReversedBounds(t *testing.T) {
	s := &Session{opts: Options{
		Type:    TypeDword,
		Mode:    ModeBetween,
		Value:   Value{Type: TypeDword, Raw: encodeInteger(TypeDword, 20)},
		Value2:  Value{Type: TypeDword, Raw: encodeInteger(TypeDword, 10)},
		Epsilon: 1e-6,
	}}
	if !s.matchBetween(encodeInteger(TypeDword, 15)) {
		t.Fatal("reversed bounds should still match")
	}
}

func TestMatchAll(t *testing.T) {
	s := &Session{opts: Options{
		Type:  TypeAll,
		Value: Value{Type: TypeAll, Raw: encodeInteger(TypeQword, 0x4240)},
	}}
	raw := []byte{0x40, 0x42, 0, 0, 0, 0, 0, 0}
	got := s.matchAll(raw)
	if len(got) != 3 {
		t.Fatalf("matches = %d, want 3 (word, dword, qword)", len(got))
	}
	if got[0].Type != TypeWord || got[2].Type != TypeQword {
		t.Fatalf("unexpected types: %v, %v", got[0].Type, got[2].Type)
	}
}

func TestUndoRestoresResults(t *testing.T) {
	s := &Session{}
	s.results = []Result{{Addr: 1}, {Addr: 2}}
	s.pushHistory()
	s.results = []Result{{Addr: 1}}
	if !s.CanUndo() {
		t.Fatal("expected an undo step")
	}
	if !s.Undo() {
		t.Fatal("Undo returned false")
	}
	if len(s.results) != 2 || s.results[1].Addr != 2 {
		t.Fatalf("results = %+v", s.results)
	}
	if s.CanUndo() {
		t.Fatal("history should be empty after undo")
	}
}

func TestParseBetweenMode(t *testing.T) {
	if m, err := ParseScanMode("between"); err != nil || m != ModeBetween {
		t.Fatalf("ParseScanMode = %v, %v", m, err)
	}
	if m, err := ParseScanMode("value between"); err != nil || m != ModeBetween {
		t.Fatalf("ParseScanMode = %v, %v", m, err)
	}
}

func TestSessionDelete(t *testing.T) {
	s := &Session{results: []Result{{Addr: 1}, {Addr: 2}, {Addr: 3}}}
	removed := s.Delete(func(r Result) bool { return r.Addr != 2 })
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if len(s.results) != 2 || s.results[0].Addr != 1 || s.results[1].Addr != 3 {
		t.Fatalf("results = %+v", s.results)
	}
}

func TestSplitRegions(t *testing.T) {
	old := regionSplit
	regionSplit = 100
	defer func() { regionSplit = old }()

	regions := []mem.Region{
		{Start: 0x1000, End: 0x1050, Perms: "rw-p"},                   // fits
		{Start: 0x2000, End: 0x2000 + 250, Perms: "r-xp", Path: "/x"}, // 100+100+50
	}
	got := splitRegions(regions)
	if len(got) != 4 {
		t.Fatalf("split into %d regions, want 4", len(got))
	}
	if got[0] != regions[0] {
		t.Fatalf("small region changed: %+v", got[0])
	}
	want := []mem.Region{
		{Start: 0x2000, End: 0x2064, Perms: "r-xp", Path: "/x"},
		{Start: 0x2064, End: 0x20c8, Perms: "r-xp", Path: "/x"},
		{Start: 0x20c8, End: 0x20fa, Perms: "r-xp", Path: "/x"},
	}
	for i, w := range want {
		if got[i+1] != w {
			t.Fatalf("sub-region %d = %+v, want %+v", i, got[i+1], w)
		}
	}
}

func TestSplitRegionsNoSplit(t *testing.T) {
	regions := []mem.Region{{Start: 0, End: 0x1000}}
	if got := splitRegions(regions); len(got) != 1 {
		t.Fatalf("split small region list into %d", len(got))
	}
}

func TestExactIntProbe(t *testing.T) {
	s := NewSession(nil, Options{Type: TypeDword, Mode: ModeExact,
		Value: NewValue(TypeDword, encodeInteger(TypeDword, 5))})
	if _, ok := s.exactIntProbe(); !ok {
		t.Fatal("dword exact should use the fast path")
	}
	s.SetMode(ModeUnknown)
	if _, ok := s.exactIntProbe(); ok {
		t.Fatal("unknown scan should not use the fast path")
	}
	f := NewSession(nil, Options{Type: TypeFloat, Mode: ModeExact, Value: NewValue(TypeFloat, []byte{0, 0, 0, 0})})
	if _, ok := f.exactIntProbe(); ok {
		t.Fatal("float scan should not use the fast path")
	}
}

func TestScanBytesIntMatchesAndCaps(t *testing.T) {
	s := NewSession(nil, Options{Type: TypeDword, Mode: ModeExact, Alignment: 4,
		Value: NewValue(TypeDword, encodeInteger(TypeDword, 0x2a)), MaxResults: 2})
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:], 0x2a)
	binary.LittleEndian.PutUint32(data[8:], 0x2a)
	binary.LittleEndian.PutUint32(data[12:], 0x2a)
	var out []Result
	var matches int64
	if err := s.scanBytes(context.Background(), 0x1000, data, len(data), &out, &matches); err != nil {
		t.Fatalf("scanBytes: %v", err)
	}
	if len(out) != 2 || matches != 2 {
		t.Fatalf("out = %d, matches = %d, want 2/2", len(out), matches)
	}
	if out[0].Addr != 0x1000 || out[1].Addr != 0x1008 {
		t.Fatalf("addresses = %#x, %#x", out[0].Addr, out[1].Addr)
	}
}

func TestDecodeIntRaw(t *testing.T) {
	cases := []struct {
		raw  []byte
		want int64
	}{
		{[]byte{0xff}, -1},
		{[]byte{0xff, 0xff}, -1},
		{[]byte{0, 0, 0, 0x80}, -2147483648},
		{[]byte{1, 0, 0, 0, 0, 0, 0, 0}, 1},
		{[]byte{}, 0},
	}
	for _, c := range cases {
		if got := decodeIntRaw(c.raw); got != c.want {
			t.Errorf("decodeIntRaw(% x) = %d, want %d", c.raw, got, c.want)
		}
	}
}
