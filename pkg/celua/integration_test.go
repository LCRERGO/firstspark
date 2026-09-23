package celua

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

// selfBuf is package-level so its address is a stable heap allocation for the
// process_vm_readv/writev round trip.
var selfBuf = make([]byte, 8)

// TestEvalAgainstSelf exercises the memory/process API against the test
// process itself, which needs no privileges beyond reading your own memory.
func TestEvalAgainstSelf(t *testing.T) {
	p, err := mem.Find(os.Getpid())
	if err != nil {
		t.Skipf("cannot open self: %v", err)
	}
	r := New(Config{Proc: p})

	addr := uintptr(unsafe.Pointer(&selfBuf[0]))
	chunk := `
pid = getOpenedProcessID()
nm = getProcessNameFromProcessID(pid)
pid2 = getProcessIDFromProcessName(nm)
writeBytes(ADDR, {1, 2, 3, 4, 5, 6, 7, 8})
ival = readInteger(ADDR)
qval = readQword(ADDR)
bytes = readBytes(ADDR, 8)
ok = true
`
	chunk = strings.ReplaceAll(chunk, "ADDR", u64str(uint64(addr)))
	if err := r.Eval(chunk); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got := binary.LittleEndian.Uint32(selfBuf); got != 0x04030201 {
		t.Fatalf("writeBytes/readInteger = %#x", got)
	}
	if got := binary.LittleEndian.Uint64(selfBuf); got != 0x0807060504030201 {
		t.Fatalf("readQword bytes = %#x", got)
	}
	runtime.KeepAlive(selfBuf)
}

func TestEvalAobScanSelf(t *testing.T) {
	p, err := mem.Find(os.Getpid())
	if err != nil {
		t.Skipf("cannot open self: %v", err)
	}
	regions, err := mem.Regions(os.Getpid())
	if err != nil {
		t.Skipf("regions: %v", err)
	}
	var target mem.Region
	for _, reg := range regions {
		if reg.Readable() && reg.Executable() && reg.FileBacked() && reg.Size() >= 128 {
			target = reg
			break
		}
	}
	if target.Start == 0 {
		t.Skip("no executable module region")
	}
	data, err := p.Read(target.Start, 128)
	if err != nil || len(data) < 40 {
		t.Skip("cannot read self")
	}
	want := data[16:24]
	var parts []string
	for _, b := range want {
		parts = append(parts, hexByte(b))
	}
	module := filepath.Base(target.Path)
	r := New(Config{Proc: p})
	if err := r.Eval(`found = AobScanModule("` + module + `", "` + strings.Join(parts, " ") + `")`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
}

func u64str(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func hexByte(b byte) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[b>>4], digits[b&0xf]})
}
