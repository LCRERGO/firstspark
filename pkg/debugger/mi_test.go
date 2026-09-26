package debugger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGDB is a scripted MI peer used to exercise the backend without ptrace.
const fakeGDB = `#!/bin/sh
printf '=thread-group-added,id="i1"\n'
printf '*stopped,reason="signal-received",signal-name="SIGSTOP",thread-id="1"\n'
while IFS= read -r line; do
  case "$line" in
    -data-list-register-values*) printf '^done,register-values=[{number="0",value="0x401000"}]\n' ;;
    -data-list-register-names*) printf '^done,register-names=["rip"]\n' ;;
    -data-read-memory-bytes*) printf '^done,memory=[{begin="0x401000",offset="0x0",end="0x401010",contents="deadbeefcafebabe0011223344556677"}]\n' ;;
    -break-insert*) printf '^done,bkpt={number="1",addr="0x401000"}\n' ;;
    -exec-step-instruction*) printf '^running\n'; printf '*stopped,reason="end-stepping-range",addr="0x401001"\n' ;;
    -exec-continue*) printf '^running\n'; printf '*stopped,reason="breakpoint-hit",bkptno="1",addr="0x401000"\n' ;;
    -gdb-exit) printf '^exit\n'; exit 0 ;;
    *) printf '^done\n' ;;
  esac
done
`

func TestGDBMIBackendWithFakeGDB(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-gdb")
	if err := os.WriteFile(script, []byte(fakeGDB), 0o755); err != nil {
		t.Fatal(err)
	}
	be, err := NewGDBMI(1234, Options{GDBPath: script})
	if err != nil {
		t.Fatal(err)
	}
	if err := be.Attach(); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer be.Close()

	regs, err := be.Registers()
	if err != nil || regs.RIP != 0x401000 {
		t.Fatalf("Registers = %+v, %v", regs, err)
	}
	data, err := be.Read(0x401000, 16)
	if err != nil || len(data) != 16 || data[0] != 0xde {
		t.Fatalf("Read = %x, %v", data, err)
	}
	if err := be.Write(0x401000, []byte{0x90}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := be.SetBreakpoint(0x401000); err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	if err := be.Continue(); err != nil {
		t.Fatalf("Continue: %v", err)
	}
	reason, err := be.Wait()
	if err != nil || !reason.HasBreakpoint || reason.BreakpointAddr != 0x401000 {
		t.Fatalf("Wait = %+v, %v", reason, err)
	}
	if err := be.Step(); err != nil {
		t.Fatalf("Step: %v", err)
	}
	reason, err = be.Wait()
	if err != nil || reason.BreakpointAddr != 0x401001 {
		t.Fatalf("step Wait = %+v, %v", reason, err)
	}
	if err := be.ClearBreakpoint(0x401000); err != nil {
		t.Fatalf("ClearBreakpoint: %v", err)
	}
}

func TestParseMIMemory(t *testing.T) {
	rec, ok := parseMILine(`^done,memory=[{begin="0x1000",offset="0x0",end="0x1004",contents="deadbeef"}]`)
	if !ok || rec.class != "done" {
		t.Fatalf("record = %+v ok=%v", rec, ok)
	}
	list, _ := rec.results["memory"].([]any)
	if len(list) != 1 {
		t.Fatalf("memory list = %+v", rec.results["memory"])
	}
	m, _ := list[0].(map[string]any)
	if miString(m, "contents") != "deadbeef" {
		t.Fatalf("contents = %q", miString(m, "contents"))
	}
}

func TestParseMIRegisterValues(t *testing.T) {
	rec, ok := parseMILine(`^done,register-values=[{number="0",value="0x1"},{number="1",value="0x2"}]`)
	if !ok {
		t.Fatal("not parsed")
	}
	list, _ := rec.results["register-values"].([]any)
	if len(list) != 2 {
		t.Fatalf("values = %+v", rec.results["register-values"])
	}
	first, _ := list[0].(map[string]any)
	if miString(first, "number") != "0" || miString(first, "value") != "0x1" {
		t.Fatalf("first = %+v", first)
	}
}

func TestParseMIStreamIsNotARecord(t *testing.T) {
	if _, ok := parseMILine(`~"Reading symbols..."`); ok {
		t.Fatal("stream line should not parse as a record")
	}
	if _, ok := parseMILine(`(gdb) `); ok {
		t.Fatal("prompt should not parse as a record")
	}
}

func TestParseMIErrorAndEscapes(t *testing.T) {
	rec, ok := parseMILine(`^error,msg="Cannot access memory at \"0x0\""`)
	if !ok || rec.class != "error" {
		t.Fatalf("record = %+v", rec)
	}
	if got := miString(rec.results, "msg"); !strings.Contains(got, "0x0") {
		t.Fatalf("msg = %q", got)
	}
}

func TestMIStopReason(t *testing.T) {
	rec, _ := parseMILine(`*stopped,reason="breakpoint-hit",bkptno="1",addr="0x0000000000400000",thread-id="1"`)
	r := miStopReason(rec)
	if r.Event != EventStopped || !r.HasBreakpoint || r.BreakpointAddr != 0x400000 {
		t.Fatalf("stop = %+v", r)
	}
	rec, _ = parseMILine(`*stopped,reason="exited-normally"`)
	if miStopReason(rec).Event != EventExited {
		t.Fatal("expected EventExited")
	}
	rec, _ = parseMILine(`*stopped,reason="end-stepping-range",addr="0x400010"`)
	if r := miStopReason(rec); r.Event != EventStopped || r.BreakpointAddr != 0x400010 {
		t.Fatalf("step stop = %+v", r)
	}
}

func TestSetRegisterByName(t *testing.T) {
	var r Registers
	setRegisterByName(&r, "rax", 0x10)
	setRegisterByName(&r, "RIP", 0x400000)
	setRegisterByName(&r, "r15", 0x99)
	if r.RAX != 0x10 || r.RIP != 0x400000 || r.R15 != 0x99 {
		t.Fatalf("registers = %+v", r)
	}
}

func TestGDBMIAttach(t *testing.T) {
	if _, err := exec.LookPath("gdb"); err != nil {
		t.Skip("gdb not installed")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn target: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	be, err := NewGDBMI(cmd.Process.Pid, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := be.Attach(); err != nil {
		t.Skipf("attach failed (ptrace permissions): %v", err)
	}
	defer be.Close()

	regs, err := be.Registers()
	if err != nil {
		t.Fatalf("Registers: %v", err)
	}
	if regs.RIP == 0 {
		t.Fatal("RIP is zero")
	}
	if _, err := be.Read(regs.RSP, 16); err != nil {
		t.Fatalf("Read: %v", err)
	}
}
