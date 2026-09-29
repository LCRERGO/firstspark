// Package plugin loads sandboxed Lua plugins that extend Firstspark through a
// versioned firstspark.* API. Plugins cannot import Go; the host exposes only
// the capabilities a plugin's manifest requests. See ADR 0052.
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// APIVersion is the plugin API version this build implements.
const APIVersion = 1

// Capability names a group of host functions a plugin must be granted.
type Capability string

const (
	CapMemoryRead  Capability = "memory_read"
	CapMemoryWrite Capability = "memory_write"
	CapHooking     Capability = "hooking"
	CapScan        Capability = "scan"
	CapUI          Capability = "ui"
)

var knownCapabilities = map[Capability]bool{
	CapMemoryRead:  true,
	CapMemoryWrite: true,
	CapHooking:     true,
	CapScan:        true,
	CapUI:          true,
}

// MenuItem is a declarative UI contribution.
type MenuItem struct {
	Label    string `yaml:"label"`
	Action   string `yaml:"action"`
	Shortcut string `yaml:"shortcut"`
}

// Hotkey is a declarative global-hotkey contribution.
type Hotkey struct {
	Action string `yaml:"action"`
	Combo  string `yaml:"combo"`
}

// Manifest is the parsed plugin.yaml.
type Manifest struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Version     string       `yaml:"version"`
	API         int          `yaml:"api"`
	Entry       string       `yaml:"entry"`
	Description string       `yaml:"description"`
	Author      string       `yaml:"author"`
	Permissions []Capability `yaml:"permissions"`
	Requires    []string     `yaml:"requires"`
	Menu        []MenuItem   `yaml:"menu"`
	Hotkeys     []Hotkey     `yaml:"hotkeys"`
}

// Validate fills defaults and checks the manifest.
func (m *Manifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("plugin: manifest id is required")
	}
	if m.API == 0 {
		m.API = APIVersion
	}
	if m.API != APIVersion {
		return fmt.Errorf("plugin %s: api %d is not supported (want %d)", m.ID, m.API, APIVersion)
	}
	if m.Entry == "" {
		m.Entry = "init.lua"
	}
	for _, p := range m.Permissions {
		if !knownCapabilities[p] {
			return fmt.Errorf("plugin %s: unknown capability %q", m.ID, p)
		}
	}
	return nil
}

// Grants reports whether the manifest requests cap.
func (m *Manifest) Grants(cap Capability) bool {
	for _, p := range m.Permissions {
		if p == cap {
			return true
		}
	}
	return false
}

// RequiresUI reports whether the plugin only works under the GUI.
func (m *Manifest) RequiresUI() bool {
	for _, r := range m.Requires {
		if strings.EqualFold(r, "ui") {
			return true
		}
	}
	return false
}

// LoadManifest reads and validates plugin.yaml from dir.
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("plugin: parse manifest in %s: %w", dir, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Discover returns the plugin directories under root that contain a
// plugin.yaml, sorted by path.
func Discover(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "plugin.yaml")); err == nil {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}
