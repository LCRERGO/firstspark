package cheattable

import (
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// printable strips anything that XML normalises (control characters, spaces and
// tabs) so a round trip is lossless by construction.
func printable(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r > 0x20 && r < 0x7F {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestPropertyTableRoundTrip(t *testing.T) {
	type gen struct {
		Desc, Addr, Type, Value  string
		Hotkey, Display, Pointer string
		Frozen                   bool
		BitSize, BitOffset       int
		BitWidth                 int
		BitSigned                bool
	}
	prop := func(gs []gen) bool {
		if len(gs) > 16 {
			return true
		}
		tbl := &Table{Version: SchemaVersion}
		for i, g := range gs {
			tbl.Entries = append(tbl.Entries, Entry{
				ID:          i + 1,
				Description: printable(g.Desc),
				Address:     printable(g.Addr),
				Type:        printable(g.Type),
				Value:       printable(g.Value),
				Hotkey:      printable(g.Hotkey),
				Display:     printable(g.Display),
				Pointer:     printable(g.Pointer),
				Frozen:      g.Frozen,
				BitSize:     g.BitSize,
				BitOffset:   g.BitOffset,
				BitWidth:    g.BitWidth,
				BitSigned:   g.BitSigned,
			})
		}
		data, err := tbl.Marshal()
		if err != nil {
			return false
		}
		back, err := Parse(data)
		if err != nil {
			return false
		}
		return back.Version == tbl.Version && reflect.DeepEqual(back.Entries, tbl.Entries)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

func TestPropertyPointerChainRoundTrip(t *testing.T) {
	type gen struct {
		Module string
		Base   uint64
		Offset uint64
		// Offsets are bounded so the +/- formatting stays in range.
		A, B, C int32
	}
	prop := func(g gen) bool {
		c := PointerChain{Offsets: []int64{int64(g.A), int64(g.B), int64(g.C)}}
		if m := sanitizeModule(g.Module); m != "" {
			// A module chain carries Module+Offset; an absolute chain carries Base.
			c.Module = m
			c.Offset = g.Offset
		} else {
			c.Base = g.Base
		}
		s := FormatPointerChain(c)
		back, ok := ParsePointerChain(s)
		return ok && back.Module == c.Module && back.Base == c.Base &&
			back.Offset == c.Offset && reflect.DeepEqual(back.Offsets, c.Offsets)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// sanitizeModule keeps only characters that do not collide with the chain
// separators (':' and "+0x").
func sanitizeModule(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '.', r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}
