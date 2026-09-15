//go:build linux && amd64

package debugger

import (
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRemoteCall(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	time.Sleep(50 * time.Millisecond)

	be, err := NewPtrace(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("NewPtrace: %v", err)
	}
	if err := be.Attach(); err != nil {
		t.Skipf("ptrace attach unavailable: %v", err)
	}
	defer be.Detach()

	pb := be.(*ptraceBackend)
	page, err := pb.Mmap(0x1000, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC,
		unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		t.Fatalf("Mmap: %v", err)
	}
	// mov rax, rdi ; ret
	if err := pb.proc.Write(page, []byte{0x48, 0x89, 0xF8, 0xC3}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	back, err := pb.proc.Read(page, 4)
	if err != nil || len(back) != 4 || back[0] != 0x48 {
		t.Fatalf("routine readback at %#x = % x (%v)", page, back, err)
	}
	got, err := pb.Call(page, []uint64{42})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != 42 {
		t.Fatalf("Call returned %d, want 42", got)
	}
}
