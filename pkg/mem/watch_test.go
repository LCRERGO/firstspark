package mem

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestNotifyExitOnKill(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Wait()

	p, err := Find(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := p.NotifyExit(ctx)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("NotifyExit did not fire after the process was killed")
	}
}

func TestNotifyExitCancelled(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	p, err := Find(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := p.NotifyExit(ctx)
	cancel()

	select {
	case <-done:
		t.Fatal("NotifyExit closed the channel on cancellation")
	case <-time.After(700 * time.Millisecond):
	}
}

func TestExistsAfterKill(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p, err := Find(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if !p.Exists() {
		t.Fatal("Exists() = false for a running process")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for p.Exists() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if p.Exists() {
		t.Fatal("Exists() = true after the process was killed and reaped")
	}
}
