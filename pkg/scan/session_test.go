package scan

import (
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
