package scan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// helperBuf is held live so the GC never frees the target buffer.
var helperBuf []byte

func TestMain(m *testing.M) {
	if os.Getenv("FIRSTSPARK_HELPER") == "1" {
		runHelper()
		return
	}
	os.Exit(m.Run())
}

func runHelper() {
	const magic = 0xDEADBEEF
	helperBuf = make([]byte, 1<<20)
	for i := 0; i+4 <= len(helperBuf); i += 4 {
		binary.LittleEndian.PutUint32(helperBuf[i:], magic)
	}
	fmt.Println(uintptr(unsafePointer(&helperBuf[0])))
	for {
		time.Sleep(time.Hour)
	}
}

func TestExactScanFindsKnownValue(t *testing.T) {
	addr, pid := startHelper(t)
	proc, err := mem.Find(pid)
	if err != nil {
		t.Fatalf("mem.Find: %v", err)
	}
	regions, err := mem.Regions(pid)
	if err != nil {
		t.Fatalf("mem.Regions: %v", err)
	}
	region, ok := mem.RegionFor(regions, addr)
	if !ok {
		t.Fatalf("no region for %#x", addr)
	}

	opts := DefaultOptions()
	opts.Type = TypeDword
	opts.Mode = ModeExact
	opts.Alignment = 4
	opts.WritableOnly = false
	opts.Regions = []mem.Region{region}
	opts.Value, err = ParseValue(TypeDword, "0xDEADBEEF")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}

	session := NewSession(proc, opts)
	if err := session.First(context.Background(), nil); err != nil {
		t.Fatalf("First: %v", err)
	}
	if session.Count() == 0 {
		t.Fatal("expected scan results")
	}
	if session.Results()[0].Addr != addr {
		t.Errorf("first result = %#x, want %#x", session.Results()[0].Addr, addr)
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	addr, pid := startHelper(t)
	proc, err := mem.Find(pid)
	if err != nil {
		t.Fatalf("mem.Find: %v", err)
	}
	orig, err := proc.Read(addr, 4)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := []byte{0x78, 0x56, 0x34, 0x12}
	if err := proc.Write(addr, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := proc.Read(addr, 4)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("read back % x, want % x", got, want)
	}
	_ = proc.Write(addr, orig)
}

func TestNextScanFilters(t *testing.T) {
	addr, pid := startHelper(t)
	proc, err := mem.Find(pid)
	if err != nil {
		t.Fatalf("mem.Find: %v", err)
	}
	regions, err := mem.Regions(pid)
	if err != nil {
		t.Fatalf("mem.Regions: %v", err)
	}
	region, ok := mem.RegionFor(regions, addr)
	if !ok {
		t.Fatalf("no region for %#x", addr)
	}
	opts := DefaultOptions()
	opts.Type = TypeDword
	opts.Mode = ModeExact
	opts.Alignment = 4
	opts.WritableOnly = false
	opts.Regions = []mem.Region{region}
	opts.Value, err = ParseValue(TypeDword, "0xDEADBEEF")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	s := NewSession(proc, opts)
	if err := s.First(context.Background(), nil); err != nil {
		t.Fatalf("First: %v", err)
	}
	first := s.Count()
	if first == 0 {
		t.Fatal("expected scan results")
	}

	// Unchanged: the batched next scan keeps everything and records Previous.
	s.SetMode(ModeUnchanged)
	if err := s.Next(context.Background(), nil); err != nil {
		t.Fatalf("Next unchanged: %v", err)
	}
	if s.Count() != first {
		t.Fatalf("unchanged kept %d of %d", s.Count(), first)
	}
	for _, r := range s.Results() {
		if len(r.Previous.Raw) == 0 {
			t.Fatal("Previous was not recorded by the next scan")
		}
		if !bytes.Equal(r.Previous.Raw, r.Value.Raw) {
			t.Fatalf("unchanged value differs: % x vs % x", r.Previous.Raw, r.Value.Raw)
		}
	}

	// Changed: the values are static, so nothing survives.
	s.SetMode(ModeChanged)
	if err := s.Next(context.Background(), nil); err != nil {
		t.Fatalf("Next changed: %v", err)
	}
	if s.Count() != 0 {
		t.Fatalf("changed kept %d results, want 0", s.Count())
	}
}

func TestSplitRegionFindsValue(t *testing.T) {
	addr, pid := startHelper(t)
	proc, err := mem.Find(pid)
	if err != nil {
		t.Fatalf("mem.Find: %v", err)
	}
	regions, err := mem.Regions(pid)
	if err != nil {
		t.Fatalf("mem.Regions: %v", err)
	}
	region, ok := mem.RegionFor(regions, addr)
	if !ok {
		t.Fatalf("no region for %#x", addr)
	}
	old := regionSplit
	regionSplit = 4096
	defer func() { regionSplit = old }()

	opts := DefaultOptions()
	opts.Type = TypeDword
	opts.Mode = ModeExact
	opts.Alignment = 4
	opts.WritableOnly = false
	opts.Regions = []mem.Region{region}
	opts.Value, err = ParseValue(TypeDword, "0xDEADBEEF")
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	s := NewSession(proc, opts)
	if err := s.First(context.Background(), nil); err != nil {
		t.Fatalf("First: %v", err)
	}
	for _, r := range s.Results() {
		if r.Addr == addr {
			return
		}
	}
	t.Fatalf("value at %#x lost after splitting the region", addr)
}

func startHelper(t *testing.T) (uint64, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "FIRSTSPARK_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	sc := bufio.NewScanner(stdout)
	if !sc.Scan() {
		t.Fatalf("helper produced no address: %v", sc.Err())
	}
	addr, err := strconv.ParseUint(sc.Text(), 10, 64)
	if err != nil {
		t.Fatalf("parse address %q: %v", sc.Text(), err)
	}
	return addr, cmd.Process.Pid
}
