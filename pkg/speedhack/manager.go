package speedhack

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/LCRERGO/firstspark/pkg/debugger"
	"github.com/LCRERGO/firstspark/pkg/inject"
)

// Manager owns the time-scale hooks for one target process. It installs,
// updates and removes them, freezing every thread of the target for the brief
// window in which a prologue is patched.
type Manager struct {
	mu        sync.Mutex
	pid       int
	scale     float64
	hooks     []installedHook
	warnings  []string
	installed bool
}

type installedHook struct {
	name string
	addr uint64
	hook *inject.Hook
}

type hookSpec struct {
	name    string
	addr    uint64
	handler *Handler
}

// NewManager creates an idle manager.
func NewManager() *Manager { return &Manager{} }

// PID returns the target pid, or 0 when not installed.
func (m *Manager) PID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pid
}

// Running reports whether hooks are installed.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.installed
}

// Scale returns the active scale.
func (m *Manager) Scale() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scale
}

// Warnings returns non-fatal messages from the last install, for example a
// symbol that could not be resolved or whose prologue could not be relocated.
func (m *Manager) Warnings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.warnings...)
}

// Hooked returns the names of the symbols currently hooked.
func (m *Manager) Hooked() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.hooks))
	for _, h := range m.hooks {
		names = append(names, h.name)
	}
	return names
}

// Install resolves and hooks the default symbols in pid at the given scale.
func (m *Manager) Install(pid int, scale float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.installed {
		return errors.New("speedhack: already installed")
	}
	return m.installLocked(pid, scale)
}

func (m *Manager) installLocked(pid int, scale float64) error {
	if _, _, err := ratio(scale); err != nil {
		return err
	}
	specs, warnings, err := buildSpecs(pid, scale)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return fmt.Errorf("speedhack: no time functions resolved in pid %d", pid)
	}
	be, err := debugger.NewPtrace(pid)
	if err != nil {
		return err
	}
	if err := be.Attach(); err != nil {
		_ = be.Close()
		return fmt.Errorf("speedhack: attach: %w", err)
	}
	defer be.Close()

	frozen := freezeThreads(pid)
	defer thawThreads(frozen)

	var hooks []installedHook
	for _, s := range specs {
		h, err := InstallHandler(be, s.addr, s.handler)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", s.name, err))
			continue
		}
		hooks = append(hooks, installedHook{name: s.name, addr: s.addr, hook: h})
	}
	if len(hooks) == 0 {
		return fmt.Errorf("speedhack: no time functions could be hooked in pid %d", pid)
	}
	m.pid = pid
	m.scale = scale
	m.hooks = hooks
	m.warnings = warnings
	m.installed = true
	return nil
}

// Remove restores every hooked prologue and releases the code caves.
func (m *Manager) Remove() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeLocked()
}

func (m *Manager) removeLocked() error {
	if !m.installed {
		return nil
	}
	pid := m.pid
	hooks := m.hooks
	m.hooks = nil
	m.installed = false

	be, err := debugger.NewPtrace(pid)
	if err != nil {
		return err
	}
	if err := be.Attach(); err != nil {
		_ = be.Close()
		if errors.Is(err, debugger.ErrNoSuchProcess) {
			return nil
		}
		return fmt.Errorf("speedhack: attach for removal: %w", err)
	}
	defer be.Close()

	frozen := freezeThreads(pid)
	defer thawThreads(frozen)

	var firstErr error
	for _, h := range hooks {
		if err := h.hook.CloseWith(be); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// UpdateScale reinstalls the hooks at a new scale. It is a remove + install so
// a changed rational approximation always fits the cave.
func (m *Manager) UpdateScale(scale float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, _, err := ratio(scale); err != nil {
		return err
	}
	if !m.installed {
		m.scale = scale
		return nil
	}
	pid := m.pid
	if err := m.removeLocked(); err != nil {
		return err
	}
	return m.installLocked(pid, scale)
}

// buildSpecs resolves and builds handlers, skipping symbols that cannot be
// resolved so a single failure does not sink the whole install.
func buildSpecs(pid int, scale float64) ([]hookSpec, []string, error) {
	var specs []hookSpec
	var warnings []string
	for _, name := range DefaultSymbols {
		addr, _, err := Resolve(pid, name)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		handler, err := BuildHandler(name, scale)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		specs = append(specs, hookSpec{name: name, addr: addr, handler: handler})
	}
	return specs, warnings, nil
}

// freezeThreads attaches to every thread except the caller's traced pid so the
// target is stopped for the patch window. Failures are ignored: a thread that
// exits mid-loop simply is not frozen.
func freezeThreads(pid int) []int {
	tids, err := listThreads(pid)
	if err != nil {
		return nil
	}
	var frozen []int
	for _, tid := range tids {
		if tid == pid {
			continue
		}
		if err := unix.PtraceAttach(tid); err != nil {
			continue
		}
		var ws unix.WaitStatus
		if _, err := unix.Wait4(tid, &ws, 0, nil); err != nil {
			_ = unix.PtraceDetach(tid)
			continue
		}
		frozen = append(frozen, tid)
	}
	return frozen
}

func thawThreads(frozen []int) {
	for _, tid := range frozen {
		_ = unix.PtraceDetach(tid)
	}
}

func listThreads(pid int) ([]int, error) {
	entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	if err != nil {
		return nil, err
	}
	tids := make([]int, 0, len(entries))
	for _, e := range entries {
		var tid int
		if _, err := fmt.Sscanf(e.Name(), "%d", &tid); err != nil {
			continue
		}
		tids = append(tids, tid)
	}
	return tids, nil
}
