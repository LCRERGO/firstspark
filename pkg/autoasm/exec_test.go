package autoasm

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

func TestModuleMatches(t *testing.T) {
	r := mem.Region{Path: "/usr/lib/libc.so.6"}
	if !moduleMatches(r, "libc.so.6") {
		t.Error("exact basename should match")
	}
	if moduleMatches(r, "libc") {
		t.Error("partial name should not match")
	}
	if !moduleMatches(mem.Region{Path: "/tmp/game (deleted)"}, "game") {
		t.Error("deleted suffix should be trimmed")
	}
	if moduleMatches(mem.Region{Path: "[heap]"}, "heap") {
		t.Error("anonymous regions should not match")
	}
}

func TestParseAOBScanModule(t *testing.T) {
	s, err := Parse("[ENABLE]\naobscanmodule(foo,game.exe,48 8B 05 ?? ?? ?? ??)\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sec := s.Sections[0]
	found := false
	for _, it := range sec.Items {
		if it.Kind == KindAOBScan {
			found = true
			if it.Name != "foo" || len(it.Args) != 3 || it.Args[1] != "game.exe" {
				t.Fatalf("item = %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("aobscanmodule item not parsed")
	}
}

func TestAOBScanModuleUnknown(t *testing.T) {
	p, err := mem.Find(os.Getpid())
	if err != nil {
		t.Skipf("cannot open self: %v", err)
	}
	if _, err := aobScanModule(p, "no-such-module.exe", "48 8B"); err == nil {
		t.Fatal("expected an error for an unknown module")
	}
}

func TestAOBScanModuleSelf(t *testing.T) {
	regions, err := mem.Regions(os.Getpid())
	if err != nil {
		t.Skipf("regions: %v", err)
	}
	var target mem.Region
	for _, r := range regions {
		if r.Readable() && r.Executable() && r.FileBacked() && r.Size() >= 64 {
			target = r
			break
		}
	}
	if target.Start == 0 {
		t.Skip("no executable module region")
	}
	p, err := mem.Find(os.Getpid())
	if err != nil {
		t.Skipf("open self: %v", err)
	}
	data, err := p.Read(target.Start, 128)
	if err != nil || len(data) < 40 {
		t.Skip("cannot read self")
	}
	want := data[16:24]
	pattern := strings.ToUpper(hex.EncodeToString(want))
	if len(pattern) < 4 {
		t.Skip("short sample")
	}
	// space-separated hex pairs
	var parts []string
	for i := 0; i+1 < len(pattern); i += 2 {
		parts = append(parts, pattern[i:i+2])
	}
	addr, err := aobScanModule(p, filepath.Base(target.Path), strings.Join(parts, " "))
	if err != nil {
		t.Fatalf("aobScanModule: %v", err)
	}
	got, err := p.Read(addr, len(want))
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("pattern mismatch at %#x", addr)
	}
}
