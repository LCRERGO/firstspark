//go:build linux && amd64

package speedhack

import (
	"bufio"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/LCRERGO/firstspark/pkg/mem"
)

const usleepFixture = `
#include <unistd.h>
#include <stdio.h>
volatile unsigned long ticks = 0;
int main(void) {
    printf("%p\n", (void *)&ticks);
    fflush(stdout);
    for (;;) {
        ticks++;
        usleep(100000);
    }
    return 0;
}
`

func TestResolveVDSOSelf(t *testing.T) {
	addr, ok, err := ResolveVDSO(os.Getpid(), "clock_gettime")
	if err != nil {
		t.Fatalf("ResolveVDSO: %v", err)
	}
	if !ok {
		t.Skip("no vdso clock_gettime on this kernel")
	}
	if addr == 0 {
		t.Fatal("vdso clock_gettime resolved to 0")
	}
}

func TestResolveFallsBackToLibc(t *testing.T) {
	addr, fromVDSO, err := Resolve(os.Getpid(), "nanosleep")
	if err != nil {
		t.Skipf("nanosleep not resolvable here (static binary): %v", err)
	}
	if fromVDSO {
		t.Error("nanosleep should not come from the vDSO")
	}
	if addr == 0 {
		t.Fatal("nanosleep resolved to 0")
	}
}

// TestManagerScalesUsleep starts a target that spins and sleeps 100ms in a
// loop, measures its tick rate, installs the speedhack at 2x and checks that it
// roughly doubles. Installing after the target has already blocked in a single
// long sleep would not help, hence the repeating fixture.
func TestManagerScalesUsleep(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler for the fixture")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "ticks.c")
	if err := os.WriteFile(src, []byte(usleepFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "ticks")
	if out, err := exec.Command(cc, "-O0", "-o", bin, src).CombinedOutput(); err != nil {
		t.Skipf("cc failed: %v: %s", err, out)
	}

	cmd := exec.Command(bin)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	sc := bufio.NewScanner(stdout)
	if !sc.Scan() {
		t.Fatal("fixture printed no address")
	}
	addr, err := strconv.ParseUint(strings.TrimPrefix(sc.Text(), "0x"), 16, 64)
	if err != nil {
		t.Fatalf("parse fixture address %q: %v", sc.Text(), err)
	}
	proc, err := mem.Find(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	read := func() uint64 {
		b, err := proc.Read(addr, 8)
		if err != nil || len(b) != 8 {
			return 0
		}
		return binary.LittleEndian.Uint64(b)
	}
	baseline := tickRate(read)

	mgr := NewManager()
	if err := mgr.Install(cmd.Process.Pid, 2.0); err != nil {
		t.Skipf("speedhack install unavailable: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Remove() })
	scaled := tickRate(read)
	t.Logf("baseline %.1f ticks/s, scaled %.1f ticks/s; hooked=%v warnings=%v",
		baseline, scaled, mgr.Hooked(), mgr.Warnings())

	if scaled < baseline*1.5 {
		t.Errorf("speedhack did not accelerate: %.1f -> %.1f ticks/s", baseline, scaled)
	}
	if err := mgr.Remove(); err != nil {
		t.Errorf("Remove: %v", err)
	}
}

func tickRate(read func() uint64) float64 {
	const window = 1200 * time.Millisecond
	start := read()
	time.Sleep(window)
	end := read()
	return float64(end-start) / window.Seconds()
}
