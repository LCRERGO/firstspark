package mem

import (
	"os"
	"testing"
	"unsafe"
)

func TestRegionPermissions(t *testing.T) {
	r := Region{Start: 0x1000, End: 0x2000, Perms: "rwxp"}
	if !r.Readable() || !r.Writable() || !r.Executable() || !r.Private() || r.Shared() {
		t.Fatalf("rwxp perms = %+v", r)
	}
	s := Region{Start: 0x1000, End: 0x2000, Perms: "r--s"}
	if !s.Readable() || s.Writable() || s.Executable() || s.Private() || !s.Shared() {
		t.Fatalf("r--s perms = %+v", s)
	}
}

func TestRegionString(t *testing.T) {
	r := Region{Start: 0x1000, End: 0x2000, Perms: "r-xp", Offset: 0x10, Dev: "08:01", Inode: 42, Path: "/bin/x"}
	if got := r.String(); got != "1000-2000 r-xp 00000010 08:01 42 /bin/x" {
		t.Fatalf("String = %q", got)
	}
}

func TestRegionFileBacked(t *testing.T) {
	if !(Region{Path: "/bin/x"}).FileBacked() {
		t.Fatal("path should be file-backed")
	}
	if (Region{Path: "[heap]"}).FileBacked() {
		t.Fatal("bracket path is not file-backed")
	}
	if (Region{}).FileBacked() {
		t.Fatal("empty path is not file-backed")
	}
}

func TestModuleBase(t *testing.T) {
	regions := []Region{
		{Start: 0x500000, End: 0x501000, Offset: 0x1000, Path: "/lib/libc.so.6"},
		{Start: 0x400000, End: 0x401000, Offset: 0, Path: "/lib/libc.so.6"},
		{Start: 0x600000, End: 0x601000, Offset: 0, Path: "/bin/game"},
	}
	base, ok := ModuleBase(regions, "libc.so.6")
	if !ok || base != 0x400000 {
		t.Fatalf("ModuleBase = %#x, %v", base, ok)
	}
	if _, ok := ModuleBase(regions, "missing"); ok {
		t.Fatal("expected no module")
	}
}

func TestParseRegionMalformed(t *testing.T) {
	for _, line := range []string{"", "bad", "zzzz-1000 r-xp 0000 00:00 0 /x", "1000 r-xp"} {
		if _, err := ParseRegion(line); err == nil {
			t.Errorf("expected an error for %q", line)
		}
	}
}

func TestFindSelfAndList(t *testing.T) {
	p, err := Find(os.Getpid())
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if p.PID != os.Getpid() {
		t.Fatalf("PID = %d", p.PID)
	}
	if p.Exe == "" {
		t.Error("expected an executable path")
	}
	regions, err := Regions(p.PID)
	if err != nil || len(regions) == 0 {
		t.Fatalf("Regions: %v (%d)", err, len(regions))
	}
	procs, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, q := range procs {
		if q.PID == os.Getpid() {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("self missing from List")
	}
}

func TestReadWriteSelf(t *testing.T) {
	p, err := Find(os.Getpid())
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	buf := make([]byte, 16)
	addr := uint64(uintptr(unsafe.Pointer(&buf[0])))
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	if err := p.Write(addr, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := p.Read(addr, 8)
	if err != nil || string(got) != string(want) {
		t.Fatalf("Read = % x, %v", got, err)
	}
	const magic = 0x1122334455667788
	if err := p.WriteUint64(addr, magic); err != nil {
		t.Fatalf("WriteUint64: %v", err)
	}
	v, err := p.ReadUint64(addr)
	if err != nil || v != magic {
		t.Fatalf("ReadUint64 = %#x, %v", v, err)
	}
	if ptr, err := p.ReadPointer(addr); err != nil || ptr != magic {
		t.Fatalf("ReadPointer = %#x, %v", ptr, err)
	}
}

func TestFindInvalidPID(t *testing.T) {
	if _, err := Find(0); err == nil {
		t.Fatal("expected an error for pid 0")
	}
	if _, err := Find(-1); err == nil {
		t.Fatal("expected an error for a negative pid")
	}
}
