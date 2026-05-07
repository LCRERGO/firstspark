package mem

import "testing"

func TestParseRegion(t *testing.T) {
	line := "55d0e5b0f000-55d0e5b10000 r-xp 00000000 08:01 123456 /usr/bin/demo"
	r, err := ParseRegion(line)
	if err != nil {
		t.Fatalf("ParseRegion: %v", err)
	}
	if r.Start != 0x55d0e5b0f000 || r.End != 0x55d0e5b10000 {
		t.Errorf("bad range %#x-%#x", r.Start, r.End)
	}
	if !r.Readable() || !r.Executable() || r.Writable() {
		t.Errorf("bad perms %q", r.Perms)
	}
	if !r.FileBacked() {
		t.Error("expected file-backed region")
	}
	if r.Size() != 0x1000 {
		t.Errorf("size = %d, want 0x1000", r.Size())
	}
}

func TestParseRegionAnonymous(t *testing.T) {
	line := "7fff00000000-7fff00021000 rw-p 00000000 00:00 0 [stack]"
	r, err := ParseRegion(line)
	if err != nil {
		t.Fatalf("ParseRegion: %v", err)
	}
	if !r.Writable() || r.FileBacked() {
		t.Errorf("unexpected region flags: %+v", r)
	}
	if r.Path != "[stack]" {
		t.Errorf("path = %q", r.Path)
	}
}

func TestRegionFor(t *testing.T) {
	regions := []Region{{Start: 0x1000, End: 0x2000}, {Start: 0x3000, End: 0x4000}}
	if _, ok := RegionFor(regions, 0x1500); !ok {
		t.Error("expected to find region")
	}
	if _, ok := RegionFor(regions, 0x2500); ok {
		t.Error("did not expect to find region")
	}
}
