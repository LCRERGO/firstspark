package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/LCRERGO/firstspark/pkg/script"
)

// API is the set of host callbacks the environment offers a plugin. A callback
// may be nil when the matching capability is unavailable; the Lua function is
// then absent rather than failing at call time.
type API struct {
	// PID returns the current target pid, or 0.
	PID func() int
	// Read reads target memory (memory_read).
	Read func(addr uint64, size int) ([]byte, error)
	// Write writes target memory (memory_write).
	Write func(addr uint64, data []byte) error
	// Show displays a message to the user (ui).
	Show func(string)
	// Log writes a plugin message to the log.
	Log func(string)
	// RegisterType registers a runtime value type (scan).
	RegisterType func(name string, spec script.Value) error
	// InstallHook installs an inline hook and returns a remover (hooking).
	InstallHook func(symbol string, handler []byte) (func() error, error)
	// Now overrides the clock used by the API (for tests).
	Now func() time.Time
}

// Plugin is a loaded, sandboxed Lua plugin.
type Plugin struct {
	Manifest *Manifest
	Dir      string
	program  *script.Program
	api      API
	failures int
	hooks    []func() error
}

// Load reads, validates and compiles a plugin from dir. It does not run
// on_load.
func Load(dir string, api API) (*Plugin, error) {
	m, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(filepath.Join(dir, m.Entry))
	if err != nil {
		return nil, fmt.Errorf("plugin %s: read entry: %w", m.ID, err)
	}
	p := &Plugin{Manifest: m, Dir: dir, api: api}
	prog, err := script.CompileWithGlobals(string(src), p.globals())
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", m.ID, err)
	}
	p.program = prog
	return p, nil
}

// Start runs on_load.
func (p *Plugin) Start() error { return p.invoke("on_load", p.context()) }

// Stop runs on_unload and releases any hooks the plugin installed.
func (p *Plugin) Stop() error {
	err := p.invoke("on_unload", p.context())
	for _, remove := range p.hooks {
		_ = remove()
	}
	p.hooks = nil
	return err
}

// Attach runs on_attach.
func (p *Plugin) Attach(pid int) error { return p.invoke("on_attach", script.Int(int64(pid))) }

// Detach runs on_detach.
func (p *Plugin) Detach(pid int) error { return p.invoke("on_detach", script.Int(int64(pid))) }

// Tick runs on_tick.
func (p *Plugin) Tick(ms int64) error { return p.invoke("on_tick", script.Int(ms)) }

// Run calls an arbitrary exported callback, used for declarative menu actions.
func (p *Plugin) Run(name string, args ...script.Value) error { return p.invoke(name, args...) }

// HasAction reports whether the plugin defines the named function.
func (p *Plugin) HasAction(name string) bool {
	return p.program != nil && p.program.Has(name)
}

func (p *Plugin) invoke(name string, args ...script.Value) error {
	if p.program == nil || !p.program.Has(name) {
		return nil
	}
	_, err := p.program.Call(name, args...)
	return err
}

func (p *Plugin) context() script.Value {
	t := script.NewTable()
	t.Set(script.Str("id"), script.Str(p.Manifest.ID))
	t.Set(script.Str("version"), script.Str(p.Manifest.Version))
	t.Set(script.Str("api"), script.Int(int64(p.Manifest.API)))
	return script.TableVal(t)
}

// Failures returns the number of consecutive tick failures.
func (p *Plugin) Failures() int { return p.failures }

func (p *Plugin) recordFailure() { p.failures++ }

func (p *Plugin) resetFailures() { p.failures = 0 }
