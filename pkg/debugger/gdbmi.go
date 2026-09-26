package debugger

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// gdbmiBackend implements Backend over GDB's Machine Interface (mi2). It drives
// a gdb child process: attach/detach, memory reads and writes, registers,
// software breakpoints, stepping, continue and stop reporting. Remote calls and
// hardware watchpoints are not supported (they stay ptrace-only).
type gdbmiBackend struct {
	pid      int
	gdbPath  string
	attached bool

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	respCh chan miRecord
	stopCh chan miRecord
	done   chan struct{}
	names  map[int]string
	breaks map[uint64]int
}

// NewGDBMI returns a gdbmi backend. Attach starts the gdb child process.
func NewGDBMI(pid int, opts Options) (Backend, error) {
	path := opts.GDBPath
	if path == "" {
		path = "gdb"
	}
	if _, err := exec.LookPath(path); err != nil {
		return nil, fmt.Errorf("debugger: gdbmi backend: %w", err)
	}
	return &gdbmiBackend{pid: pid, gdbPath: path, breaks: map[uint64]int{}}, nil
}

func (b *gdbmiBackend) PID() int { return b.pid }

func (b *gdbmiBackend) Attach() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd != nil {
		return nil
	}
	cmd := exec.Command(b.gdbPath, "--interpreter=mi2", "-q", "-p", strconv.Itoa(b.pid))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	b.cmd = cmd
	b.stdin = stdin
	b.stdout = bufio.NewReader(stdout)
	b.respCh = make(chan miRecord, 1)
	b.stopCh = make(chan miRecord, 16)
	b.done = make(chan struct{})
	if err := cmd.Start(); err != nil {
		b.cmd = nil
		return fmt.Errorf("debugger: start gdb: %w", err)
	}
	go b.readLoop()
	// Sync with gdb so Attach does not return before it is responsive.
	if _, err := b.send("-gdb-set confirm off"); err != nil {
		_ = b.closeLocked()
		return err
	}
	if _, err := b.send("-gdb-set pagination off"); err != nil {
		_ = b.closeLocked()
		return err
	}
	// Probe the inferior: registers are only available once the attach stop has
	// been processed, so this both confirms the attach and warms nothing else.
	if _, err := b.send("-data-list-register-values x"); err != nil {
		_ = b.closeLocked()
		return fmt.Errorf("debugger: gdbmi: attach failed: %w", err)
	}
	b.attached = true
	return nil
}

// Detach releases the target but keeps the gdb process alive.
func (b *gdbmiBackend) Detach() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return nil
	}
	b.attached = false
	_, err := b.send("-target-detach")
	return err
}

func (b *gdbmiBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeLocked()
}

func (b *gdbmiBackend) closeLocked() error {
	if b.cmd == nil {
		return nil
	}
	if b.attached {
		b.attached = false
		_, _ = b.send("-gdb-exit")
	}
	if b.stdin != nil {
		_ = b.stdin.Close()
	}
	_ = b.cmd.Process.Kill()
	_ = b.cmd.Wait()
	b.cmd = nil
	b.stdin = nil
	b.stdout = nil
	return nil
}

func (b *gdbmiBackend) readLoop() {
	defer close(b.done)
	for {
		line, err := b.stdout.ReadString('\n')
		if len(line) > 0 {
			b.handleLine(strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			return
		}
	}
}

func (b *gdbmiBackend) handleLine(line string) {
	if line == "" || strings.HasPrefix(line, "(gdb)") {
		return
	}
	switch line[0] {
	case '~', '@', '&':
		return // stream records
	}
	rec, ok := parseMILine(line)
	if !ok {
		return
	}
	switch rec.kind {
	case '^':
		select {
		case b.respCh <- rec:
		default:
		}
	case '*':
		if rec.class == "stopped" {
			select {
			case b.stopCh <- rec:
			case <-b.done:
			}
		}
	}
}

// send writes a command and waits for its result record. Callers hold b.mu.
func (b *gdbmiBackend) send(cmd string) (miRecord, error) {
	if b.stdin == nil {
		return miRecord{}, ErrNotAttached
	}
	for {
		select {
		case <-b.respCh:
		default:
			goto drained
		}
	}
drained:
	if _, err := io.WriteString(b.stdin, cmd+"\n"); err != nil {
		return miRecord{}, fmt.Errorf("debugger: gdbmi write: %w", err)
	}
	select {
	case rec := <-b.respCh:
		if rec.class == "error" {
			return rec, fmt.Errorf("debugger: gdbmi %s: %s", cmd, miString(rec.results, "msg"))
		}
		return rec, nil
	case <-b.done:
		return miRecord{}, fmt.Errorf("debugger: gdbmi: gdb exited")
	case <-time.After(30 * time.Second):
		return miRecord{}, fmt.Errorf("debugger: gdbmi timeout waiting for %q", cmd)
	}
}

func (b *gdbmiBackend) Read(addr uint64, size int) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return nil, ErrNotAttached
	}
	res, err := b.send(fmt.Sprintf("-data-read-memory-bytes 0x%x %d", addr, size))
	if err != nil {
		return nil, err
	}
	list, _ := res.results["memory"].([]any)
	if len(list) == 0 {
		return nil, fmt.Errorf("debugger: gdbmi: no memory at 0x%x", addr)
	}
	m, _ := list[0].(map[string]any)
	data, derr := hex.DecodeString(miString(m, "contents"))
	if derr != nil {
		return nil, fmt.Errorf("debugger: gdbmi decode memory: %w", derr)
	}
	return data, nil
}

func (b *gdbmiBackend) Write(addr uint64, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return ErrNotAttached
	}
	_, err := b.send(fmt.Sprintf("-data-write-memory-bytes 0x%x %s", addr, hex.EncodeToString(data)))
	return err
}

func (b *gdbmiBackend) Registers() (Registers, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return Registers{}, ErrNotAttached
	}
	names, err := b.registerNames()
	if err != nil {
		return Registers{}, err
	}
	res, err := b.send("-data-list-register-values x")
	if err != nil {
		return Registers{}, err
	}
	list, _ := res.results["register-values"].([]any)
	var regs Registers
	for _, item := range list {
		t, ok := item.(map[string]any)
		if !ok {
			continue
		}
		num, _ := strconv.Atoi(miString(t, "number"))
		setRegisterByName(&regs, names[num], miHex(miString(t, "value")))
	}
	return regs, nil
}

func (b *gdbmiBackend) registerNames() (map[int]string, error) {
	if b.names != nil {
		return b.names, nil
	}
	res, err := b.send("-data-list-register-names")
	if err != nil {
		return nil, err
	}
	list, _ := res.results["register-names"].([]any)
	m := map[int]string{}
	for i, v := range list {
		if s, ok := v.(string); ok {
			m[i] = s
		}
	}
	b.names = m
	return m, nil
}

// SetRegisters writes each register with -gdb-set, ignoring registers the
// target does not have.
func (b *gdbmiBackend) SetRegisters(r Registers) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return ErrNotAttached
	}
	values := registerPairs(r)
	for _, p := range values {
		_, _ = b.send(fmt.Sprintf("-gdb-set $%s=0x%x", p.name, p.value))
	}
	return nil
}

func (b *gdbmiBackend) SetBreakpoint(addr uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return ErrNotAttached
	}
	res, err := b.send(fmt.Sprintf("-break-insert *0x%x", addr))
	if err != nil {
		return err
	}
	bk, _ := res.results["bkpt"].(map[string]any)
	num, _ := strconv.Atoi(miString(bk, "number"))
	if num > 0 {
		b.breaks[addr] = num
	}
	return nil
}

func (b *gdbmiBackend) ClearBreakpoint(addr uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return ErrNotAttached
	}
	num, ok := b.breaks[addr]
	if !ok {
		return nil
	}
	if _, err := b.send(fmt.Sprintf("-break-delete %d", num)); err != nil {
		return err
	}
	delete(b.breaks, addr)
	return nil
}

func (b *gdbmiBackend) Step() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return ErrNotAttached
	}
	_, err := b.send("-exec-step-instruction")
	return err
}

func (b *gdbmiBackend) Continue() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.attached {
		return ErrNotAttached
	}
	// Drop any stale stop (for example the attach stop) before resuming.
	for {
		select {
		case <-b.stopCh:
		default:
			goto drained
		}
	}
drained:
	_, err := b.send("-exec-continue")
	return err
}

func (b *gdbmiBackend) Wait() (StopReason, error) {
	if b.done == nil {
		return StopReason{}, ErrNotAttached
	}
	select {
	case rec := <-b.stopCh:
		return miStopReason(rec), nil
	case <-b.done:
		return StopReason{}, fmt.Errorf("debugger: gdbmi: gdb exited")
	}
}

func miStopReason(rec miRecord) StopReason {
	reason := miString(rec.results, "reason")
	switch reason {
	case "exited-normally":
		return StopReason{Event: EventExited}
	case "exited":
		code, _ := strconv.Atoi(miString(rec.results, "exit-code"))
		return StopReason{Event: EventExited, ExitCode: code}
	case "signal-received":
		return StopReason{Event: EventSignaled, Signal: miSignal(miString(rec.results, "signal-name"))}
	case "breakpoint-hit":
		return StopReason{Event: EventStopped, HasBreakpoint: true, BreakpointAddr: miHex(miString(rec.results, "addr"))}
	default:
		r := StopReason{Event: EventStopped}
		if addr := miString(rec.results, "addr"); addr != "" {
			r.BreakpointAddr = miHex(addr)
		}
		return r
	}
}

// miHex parses an MI hex value like "0x55" or "55".
func miHex(s string) uint64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	n, _ := strconv.ParseUint(s, 16, 64)
	return n
}

func miSignal(name string) syscall.Signal {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SIGINT":
		return syscall.SIGINT
	case "SIGILL":
		return syscall.SIGILL
	case "SIGABRT":
		return syscall.SIGABRT
	case "SIGFPE":
		return syscall.SIGFPE
	case "SIGSEGV":
		return syscall.SIGSEGV
	case "SIGTERM":
		return syscall.SIGTERM
	default:
		return syscall.SIGTRAP
	}
}

type registerPair struct {
	name  string
	value uint64
}

// registerPairs orders a snapshot like the ptrace backend.
func registerPairs(r Registers) []registerPair {
	return []registerPair{
		{"rip", r.RIP}, {"rsp", r.RSP}, {"rbp", r.RBP}, {"rax", r.RAX},
		{"rbx", r.RBX}, {"rcx", r.RCX}, {"rdx", r.RDX}, {"rsi", r.RSI},
		{"rdi", r.RDI}, {"r8", r.R8}, {"r9", r.R9}, {"r10", r.R10},
		{"r11", r.R11}, {"r12", r.R12}, {"r13", r.R13}, {"r14", r.R14},
		{"r15", r.R15}, {"eflags", r.RFLAGS},
	}
}

// setRegisterByName writes one register field from a gdb register name.
func setRegisterByName(r *Registers, name string, v uint64) {
	switch strings.ToLower(name) {
	case "rip", "eip", "pc":
		r.RIP = v
	case "rsp", "esp", "sp":
		r.RSP = v
	case "rbp", "ebp", "bp":
		r.RBP = v
	case "rax", "eax":
		r.RAX = v
	case "rbx", "ebx":
		r.RBX = v
	case "rcx", "ecx":
		r.RCX = v
	case "rdx", "edx":
		r.RDX = v
	case "rsi", "esi":
		r.RSI = v
	case "rdi", "edi":
		r.RDI = v
	case "r8":
		r.R8 = v
	case "r9":
		r.R9 = v
	case "r10":
		r.R10 = v
	case "r11":
		r.R11 = v
	case "r12":
		r.R12 = v
	case "r13":
		r.R13 = v
	case "r14":
		r.R14 = v
	case "r15":
		r.R15 = v
	case "eflags", "rflags", "flags":
		r.RFLAGS = v
	}
}
