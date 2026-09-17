// Package config loads and saves Firstspark's YAML configuration.
package config

import (
	"fmt"
	"os"
	"os/user"
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
	Log       LogConfig       `yaml:"log"`
	// Hotkeys maps an action id (for example "speedhack.toggle") to a combo
	// such as "Ctrl+Alt+S" or "F5". Unassigned actions are absent.
	Hotkeys map[string]string `yaml:"hotkeys"`
}

// LogConfig controls diagnostic logging.
type LogConfig struct {
	// Level is one of debug, info, warn or error.
	Level string `yaml:"level"`
	// File overrides the default log path when non-empty.
	File string `yaml:"file"`
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
	// Delta is how much the speedhack +/- hotkeys change the scale.
	Delta float64 `yaml:"delta"`
}

// UIConfig holds presentation defaults.
type UIConfig struct {
	ResultLimit int `yaml:"result_limit"`
	// Theme is the palette family: cyberpunk, nord, dracula or tokyo-night.
	Theme string `yaml:"theme"`
	// ThemeVariant is light, dark or system.
	ThemeVariant string  `yaml:"theme_variant"`
	Language     string  `yaml:"language"`
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
		Speedhack: SpeedhackConfig{Enabled: false, Scale: 1.0, Delta: 0.5},
		UI: UIConfig{
			ResultLimit: 1000, Theme: "cyberpunk", ThemeVariant: "light",
			Language: "en", Scale: 1.0, FontSize: 14, ProcessIcons: true,
		},
		Log: LogConfig{Level: "info"},
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
	migrateUI(&cfg.UI)
	return cfg, nil
}

// migrateUI upgrades the pre-family ui.theme values (light, dark, system) to
// the cyberpunk family plus a variant, and fills in empty fields.
func migrateUI(ui *UIConfig) {
	switch ui.Theme {
	case "light", "dark", "system":
		ui.ThemeVariant = ui.Theme
		ui.Theme = "cyberpunk"
	case "":
		ui.Theme = "cyberpunk"
	}
	if ui.ThemeVariant == "" {
		ui.ThemeVariant = "light"
	}
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
	uid, ok := firstEnvInt("FIRSTSPARK_ORIG_UID", "SUDO_UID")
	if !ok {
		return
	}
	gid, _ := firstEnvInt("FIRSTSPARK_ORIG_GID", "SUDO_GID")
	_ = os.Chown(path, uid, gid)
}

func firstEnvInt(keys ...string) (int, bool) {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// Dir returns the configuration directory ($XDG_CONFIG_HOME/firstspark). When
// run elevated it uses the invoking user's directory, not root's.
func Dir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".config")
	}
	return filepath.Join(base, "firstspark")
}

// DefaultPath returns the default configuration file path.
func DefaultPath() string { return filepath.Join(Dir(), "config.yaml") }

// CustomTypesPath returns the default user-defined value types file path.
func CustomTypesPath() string { return filepath.Join(Dir(), "customtypes.yaml") }

// CacheDir returns the cache directory ($XDG_CACHE_HOME/firstspark).
func CacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".cache")
	}
	return filepath.Join(base, "firstspark")
}

// StateDir returns the state directory ($XDG_STATE_HOME/firstspark), used for
// logs and other runtime state.
func StateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".local", "state")
	}
	return filepath.Join(base, "firstspark")
}

// LogPath returns the default log file path.
func LogPath() string { return filepath.Join(StateDir(), "firstspark.log") }

// DataDir returns the data directory used for saved scan sessions
// ($XDG_DATA_HOME/firstspark).
func DataDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".local", "share")
	}
	return filepath.Join(base, "firstspark")
}

// homeDir returns the invoking user's home directory, resolving sudo's
// SUDO_USER when running as root so configuration stays the user's.
func homeDir() string {
	if os.Geteuid() == 0 {
		if name := os.Getenv("SUDO_USER"); name != "" && name != "root" {
			if u, err := user.Lookup(name); err == nil && u.HomeDir != "" {
				return u.HomeDir
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return os.Getenv("HOME")
}
