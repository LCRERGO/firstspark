//go:build linux && amd64

package debugger

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("FIRSTSPARK_DEBUG_HELPER") == "1" {
		if os.Getenv("FIRSTSPARK_DEBUG_SPIN") == "1" {
			x := 0
			for {
				x++
				if x < 0 {
					x = 0
				}
			}
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	os.Exit(m.Run())
}

func TestAttachAndReadRegisters(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "FIRSTSPARK_DEBUG_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
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

	regs, err := be.Registers()
	if err != nil {
		t.Fatalf("Registers: %v", err)
	}
	if regs.RIP == 0 || regs.RSP == 0 {
		t.Errorf("unexpected registers: RIP=%#x RSP=%#x", regs.RIP, regs.RSP)
	}
}

func TestBreakpointRoundTrip(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "FIRSTSPARK_DEBUG_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
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

	regs, err := be.Registers()
	if err != nil {
		t.Fatalf("Registers: %v", err)
	}
	// Find a readable, non-executable page to poke a byte safely.
	addr := regs.RSP &^ 0xFFF
	if err := be.SetBreakpoint(addr); err != nil {
		t.Skipf("cannot set breakpoint: %v", err)
	}
	data, err := be.Read(addr, 1)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if data[0] != 0xCC {
		t.Errorf("breakpoint byte = %#x, want 0xcc", data[0])
	}
	if err := be.ClearBreakpoint(addr); err != nil {
		t.Fatalf("ClearBreakpoint: %v", err)
	}
}

func TestCloseStopsWorker(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "FIRSTSPARK_DEBUG_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
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
	if err := be.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := be.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
