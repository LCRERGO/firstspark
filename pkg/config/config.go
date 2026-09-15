// Package config loads and saves Firstspark's YAML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration document.
type Config struct {
	Scan      ScanConfig      `yaml:"scan"`
	Debugger  DebuggerConfig  `yaml:"debugger"`
	Speedhack SpeedhackConfig `yaml:"speedhack"`
	UI        UIConfig        `yaml:"ui"`
}

// ScanConfig holds memory scanning defaults.
type ScanConfig struct {
	ValueType     string  `yaml:"value_type"`
	WritableOnly  bool    `yaml:"writable_only"`
	Alignment     int     `yaml:"alignment"`
	SnapshotLimit int64   `yaml:"snapshot_limit"`
	FloatEpsilon  float64 `yaml:"float_epsilon"`
}

// DebuggerConfig selects the debugging backend.
type DebuggerConfig struct {
	Backend string `yaml:"backend"`
	GDBPath string `yaml:"gdb_path"`
}

// SpeedhackConfig holds time scaling defaults.
type SpeedhackConfig struct {
	Enabled bool    `yaml:"enabled"`
	Scale   float64 `yaml:"scale"`
}

// UIConfig holds presentation defaults.
type UIConfig struct {
	ResultLimit  int     `yaml:"result_limit"`
	Theme        string  `yaml:"theme"`
	Scale        float64 `yaml:"scale"`
	FontSize     float64 `yaml:"font_size"`
	ProcessIcons bool    `yaml:"process_icons"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Scan: ScanConfig{
			ValueType:     "dword",
			WritableOnly:  true,
			Alignment:     4,
			SnapshotLimit: 2 << 30,
			FloatEpsilon:  1e-6,
		},
		Debugger:  DebuggerConfig{Backend: "ptrace", GDBPath: "gdb"},
		Speedhack: SpeedhackConfig{Enabled: false, Scale: 1.0},
		UI:        UIConfig{ResultLimit: 1000, Theme: "light", Scale: 1.0, FontSize: 14, ProcessIcons: true},
	}
}

// Load reads a YAML config, falling back to defaults for missing fields.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the configuration as YAML, creating parent directories.
func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	// When running elevated (pkexec), keep the files owned by the invoking
	// user so they stay editable without root.
	chownToOriginalUser(filepath.Dir(path))
	chownToOriginalUser(path)
	return nil
}

func chownToOriginalUser(path string) {
	if os.Geteuid() != 0 {
		return
	}
	uid, err := strconv.Atoi(os.Getenv("FIRSTSPARK_ORIG_UID"))
	if err != nil {
		return
	}
	gid, _ := strconv.Atoi(os.Getenv("FIRSTSPARK_ORIG_GID"))
	_ = os.Chown(path, uid, gid)
}

// Dir returns the configuration directory ($XDG_CONFIG_HOME/firstspark).
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "firstspark")
}

// DefaultPath returns the default configuration file path.
func DefaultPath() string { return filepath.Join(Dir(), "config.yaml") }

// CustomTypesPath returns the default user-defined value types file path.
func CustomTypesPath() string { return filepath.Join(Dir(), "customtypes.yaml") }

// DataDir returns the data directory used for saved scan sessions
// ($XDG_DATA_HOME/firstspark).
func DataDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(base, "firstspark")
}
