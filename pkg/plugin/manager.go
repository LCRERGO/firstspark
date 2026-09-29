package plugin

import (
	"fmt"
	"sync"
)

// maxTickFailures is how many consecutive on_tick errors disable a plugin.
const maxTickFailures = 5

// Manager discovers and runs the enabled plugins under a directory.
type Manager struct {
	mu      sync.Mutex
	dir     string
	api     API
	gui     bool
	enabled []*Plugin
	errors  []string
}

// NewManager creates a manager rooted at dir.
func NewManager(dir string, api API) *Manager {
	return &Manager{dir: dir, api: api}
}

// Load discovers plugins and starts the enabled ones. gui reports whether the
// GUI is available; plugins requiring ui are skipped when it is not.
func (m *Manager) Load(enabled []string, gui bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gui = gui
	dirs, err := Discover(m.dir)
	if err != nil {
		m.errors = append(m.errors, err.Error())
		return
	}
	want := map[string]bool{}
	for _, id := range enabled {
		want[id] = true
	}
	for _, dir := range dirs {
		mf, err := LoadManifest(dir)
		if err != nil {
			m.errors = append(m.errors, err.Error())
			continue
		}
		if !want[mf.ID] {
			continue
		}
		if mf.RequiresUI() && !gui {
			continue
		}
		p, err := Load(dir, m.api)
		if err != nil {
			m.errors = append(m.errors, err.Error())
			continue
		}
		if err := p.Start(); err != nil {
			m.errors = append(m.errors, fmt.Sprintf("%s: on_load: %v", mf.ID, err))
			continue
		}
		m.enabled = append(m.enabled, p)
	}
}

// Info describes a discovered plugin.
type Info struct {
	Manifest Manifest
	Enabled  bool
}

// List returns every discovered manifest, marking the enabled ones.
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked()
}

func (m *Manager) listLocked() []Info {
	dirs, _ := Discover(m.dir)
	active := map[string]bool{}
	for _, p := range m.enabled {
		active[p.Manifest.ID] = true
	}
	out := make([]Info, 0, len(dirs))
	for _, dir := range dirs {
		mf, err := LoadManifest(dir)
		if err != nil {
			continue
		}
		out = append(out, Info{Manifest: *mf, Enabled: active[mf.ID]})
	}
	return out
}

// Plugins returns the running plugins.
func (m *Manager) Plugins() []*Plugin {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*Plugin(nil), m.enabled...)
}

// Attach notifies running plugins that pid was selected.
func (m *Manager) Attach(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.enabled {
		if err := p.Attach(pid); err != nil {
			m.errors = append(m.errors, fmt.Sprintf("%s: on_attach: %v", p.Manifest.ID, err))
		}
	}
}

// Detach notifies running plugins that the target is gone.
func (m *Manager) Detach(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.enabled {
		if err := p.Detach(pid); err != nil {
			m.errors = append(m.errors, fmt.Sprintf("%s: on_detach: %v", p.Manifest.ID, err))
		}
	}
}

// Tick runs on_tick on every plugin, disabling one after repeated failures.
func (m *Manager) Tick(ms int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := make([]*Plugin, 0, len(m.enabled))
	for _, p := range m.enabled {
		if err := p.Tick(ms); err != nil {
			p.recordFailure()
			m.errors = append(m.errors, fmt.Sprintf("%s: on_tick: %v", p.Manifest.ID, err))
			if p.Failures() >= maxTickFailures {
				_ = p.Stop()
				m.errors = append(m.errors, fmt.Sprintf("%s: disabled after %d failures", p.Manifest.ID, p.Failures()))
				continue
			}
		} else {
			p.resetFailures()
		}
		kept = append(kept, p)
	}
	m.enabled = kept
}

// MenuContribution is a declarative menu item contributed by a plugin.
type MenuContribution struct {
	PluginID   string
	PluginName string
	Item       MenuItem
}

// Menu returns the menu items of the running plugins.
func (m *Manager) Menu() []MenuContribution {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []MenuContribution
	for _, p := range m.enabled {
		for _, item := range p.Manifest.Menu {
			out = append(out, MenuContribution{PluginID: p.Manifest.ID, PluginName: p.Manifest.Name, Item: item})
		}
	}
	return out
}

// Run invokes an exported plugin action, for a declarative menu item or hotkey.
func (m *Manager) Run(pluginID, action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.enabled {
		if p.Manifest.ID != pluginID {
			continue
		}
		if !p.HasAction(action) {
			return fmt.Errorf("plugin %s: no action %q", pluginID, action)
		}
		return p.Run(action)
	}
	return fmt.Errorf("plugin %s is not running", pluginID)
}

// Errors returns the load and runtime errors seen so far.
func (m *Manager) Errors() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.errors...)
}

// Close stops every running plugin.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.enabled {
		_ = p.Stop()
	}
	m.enabled = nil
}
