//go:build gui

package ui

import (
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/LCRERGO/firstspark/pkg/mem"
	"github.com/LCRERGO/firstspark/pkg/scan"
)

var memReadBuf = make([]byte, 8)

func TestReadMemoryValue(t *testing.T) {
	p, err := mem.Find(os.Getpid())
	if err != nil {
		t.Skipf("cannot open self: %v", err)
	}
	a := newTestApp(t)
	a.proc = p

	addr := uint64(uintptr(unsafe.Pointer(&memReadBuf[0])))
	if err := p.Write(addr, []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	v, err := a.readMemoryValue(addr, scan.TypeDword)
	if err != nil {
		t.Fatalf("readMemoryValue: %v", err)
	}
	if v.Int64() != 0x04030201 {
		t.Fatalf("value = %#x, want 0x04030201", v.Int64())
	}
	if got := hexOf(v); got != "0x4030201" {
		t.Fatalf("hexOf = %q", got)
	}
	runtime.KeepAlive(memReadBuf)
}

func TestMemValueOptions(t *testing.T) {
	opts := memValueOptions()
	var haveText, haveAOB, haveInt bool
	for _, o := range opts {
		switch {
		case o.text:
			haveText = true
		case o.typ == scan.TypeAOB:
			haveAOB = true
		case o.integer:
			haveInt = true
		}
	}
	if !haveText || !haveAOB || !haveInt {
		t.Fatalf("options missing kinds: %+v", opts)
	}
}
