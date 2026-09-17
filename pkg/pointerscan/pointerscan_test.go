package pointerscan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

func testMap() *Pointermap {
	return &Pointermap{
		entries: []entry{
			{value: 0x2100, addrs: []uint64{0x1010}},
			{value: 0x2FF0, addrs: []uint64{0x2100}},
		},
		statics: []StaticRegion{{Start: 0x1000, End: 0x2000, Module: "libtest.so", Base: 0x1000}},
	}
}

func TestScanFindsModuleChain(t *testing.T) {
	chains := testMap().Scan(0x3000, DefaultOptions())
	if len(chains) != 1 {
		t.Fatalf("chains = %+v", chains)
	}
	c := chains[0]
	if c.Module != "libtest.so" || c.Base != 0x1000 {
		t.Fatalf("chain = %+v", c)
	}
	want := []uint64{0x10, 0, 0x10}
	if len(c.Offsets) != len(want) {
		t.Fatalf("offsets = %v", c.Offsets)
	}
	for i := range want {
		if c.Offsets[i] != want[i] {
			t.Fatalf("offsets = %v, want %v", c.Offsets, want)
		}
	}
}

func TestStaticOnly(t *testing.T) {
	pm := &Pointermap{entries: []entry{{value: 0x2FF0, addrs: []uint64{0x2100}}}}
	o := DefaultOptions()
	o.MaxLevel = 1
	o.StaticOnly = true
	if chains := pm.Scan(0x3000, o); len(chains) != 0 {
		t.Fatalf("expected no static chains, got %+v", chains)
	}
	o.StaticOnly = false
	if chains := pm.Scan(0x3000, o); len(chains) != 1 {
		t.Fatalf("expected one absolute chain, got %+v", chains)
	}
}

func TestAlignedFilter(t *testing.T) {
	pm := &Pointermap{entries: []entry{{value: 0x2FF0, addrs: []uint64{0x2105}}}}
	o := DefaultOptions()
	o.MaxLevel = 1
	o.Aligned = true
	if chains := pm.Scan(0x3000, o); len(chains) != 0 {
		t.Fatalf("expected alignment to reject, got %+v", chains)
	}
	o.Aligned = false
	if chains := pm.Scan(0x3000, o); len(chains) != 1 {
		t.Fatalf("expected one chain when unaligned, got %+v", chains)
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.ptr")
	chains := testMap().Scan(0x3000, DefaultOptions())
	if err := Save(path, chains); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(chains) || got[0].Module != chains[0].Module {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestPointermapCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pm.json")
	pm := testMap()
	if err := savePointermap(path, pm); err != nil {
		t.Fatalf("savePointermap: %v", err)
	}
	got, err := loadPointermap(path)
	if err != nil {
		t.Fatalf("loadPointermap: %v", err)
	}
	if len(got.entries) != len(pm.entries) || got.entries[0].value != pm.entries[0].value {
		t.Fatalf("entries = %+v", got.entries)
	}
	if len(got.statics) != 1 || got.statics[0].Module != "libtest.so" {
		t.Fatalf("statics = %+v", got.statics)
	}
}

func TestCachePathKeyedByRegions(t *testing.T) {
	regions := []mem.Region{{Start: 0x1000, End: 0x2000, Path: "/lib/a.so"}}
	other := []mem.Region{{Start: 0x3000, End: 0x4000, Path: "/lib/a.so"}}
	o := BuildOptions{WritableOnly: true, Aligned: true}
	if CachePath(1, regions, o) == CachePath(1, other, o) {
		t.Fatal("cache path should change when the region map changes")
	}
}

func TestBuildPointermapSelf(t *testing.T) {
	p, err := mem.Find(os.Getpid())
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	pm, err := BuildPointermap(p, BuildOptions{WritableOnly: true, Aligned: true, MaxBytes: 8 << 20})
	if err != nil {
		t.Fatalf("BuildPointermap: %v", err)
	}
	if len(pm.entries) == 0 {
		t.Skip("no pointers found in the sampled window")
	}
}
