package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Default()
	cfg.Scan.Alignment = 8
	cfg.UI.ResultLimit = 250
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Scan.Alignment != 8 || got.UI.ResultLimit != 250 {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Debugger.Backend != "ptrace" {
		t.Errorf("expected default backend, got %q", got.Debugger.Backend)
	}
	if got.UI.Theme != "cyberpunk" || got.UI.ThemeVariant != "light" {
		t.Errorf("expected cyberpunk/light defaults, got %q/%q", got.UI.Theme, got.UI.ThemeVariant)
	}
}

func TestLoadMigratesLegacyTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  theme: dark\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.UI.Theme != "cyberpunk" || got.UI.ThemeVariant != "dark" {
		t.Errorf("migration = %q/%q, want cyberpunk/dark", got.UI.Theme, got.UI.ThemeVariant)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  theme: system\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	first, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := first.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	second, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if second.UI != first.UI {
		t.Errorf("migration not stable: %+v then %+v", first.UI, second.UI)
	}
}

func TestUnknownFamilyIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  theme: mytheme\n  theme_variant: dark\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.UI.Theme != "mytheme" {
		t.Errorf("Theme = %q, want mytheme (config should not validate families)", got.UI.Theme)
	}
}

func TestLoadKeepsFamilyTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  theme: nord\n  theme_variant: system\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.UI.Theme != "nord" || got.UI.ThemeVariant != "system" {
		t.Errorf("got %q/%q, want nord/system", got.UI.Theme, got.UI.ThemeVariant)
	}
}

func TestXDGPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xc")
	t.Setenv("XDG_DATA_HOME", "/tmp/xd")
	t.Setenv("XDG_STATE_HOME", "/tmp/xs")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xk")
	if got := Dir(); got != "/tmp/xc/firstspark" {
		t.Fatalf("Dir = %q", got)
	}
	if got := DefaultPath(); got != "/tmp/xc/firstspark/config.yaml" {
		t.Fatalf("DefaultPath = %q", got)
	}
	if got := CustomTypesPath(); got != "/tmp/xc/firstspark/customtypes.yaml" {
		t.Fatalf("CustomTypesPath = %q", got)
	}
	if got := DataDir(); got != "/tmp/xd/firstspark" {
		t.Fatalf("DataDir = %q", got)
	}
	if got := StateDir(); got != "/tmp/xs/firstspark" {
		t.Fatalf("StateDir = %q", got)
	}
	if got := LogPath(); got != "/tmp/xs/firstspark/firstspark.log" {
		t.Fatalf("LogPath = %q", got)
	}
	if got := CacheDir(); got != "/tmp/xk/firstspark" {
		t.Fatalf("CacheDir = %q", got)
	}
}

func TestRoundTripNewFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Default()
	cfg.UI.RefreshMS = 250
	cfg.Scan.FloatEpsilon = 0.5
	cfg.Scan.SnapshotLimit = 4096
	cfg.Debugger.Backend = "gdbmi"
	cfg.Debugger.GDBPath = "/usr/bin/gdb"
	cfg.Process.AutoAttach = "game;server"
	cfg.Process.AutoAttachRegex = true
	cfg.Log.Level = "debug"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if back.UI.RefreshMS != 250 || back.Scan.FloatEpsilon != 0.5 || back.Scan.SnapshotLimit != 4096 {
		t.Fatalf("scan/ui fields lost: %+v", back)
	}
	if back.Debugger.Backend != "gdbmi" || back.Debugger.GDBPath != "/usr/bin/gdb" {
		t.Fatalf("debugger fields lost: %+v", back.Debugger)
	}
	if back.Process.AutoAttach != "game;server" || !back.Process.AutoAttachRegex {
		t.Fatalf("process fields lost: %+v", back.Process)
	}
	if back.Log.Level != "debug" {
		t.Fatalf("log level = %q", back.Log.Level)
	}
}

func TestMigrateUI(t *testing.T) {
	ui := UIConfig{Theme: "dark"}
	migrateUI(&ui)
	if ui.Theme != "cyberpunk" || ui.ThemeVariant != "dark" {
		t.Fatalf("migrate dark = %+v", ui)
	}
	ui = UIConfig{}
	migrateUI(&ui)
	if ui.Theme != "cyberpunk" || ui.ThemeVariant != "light" {
		t.Fatalf("migrate empty = %+v", ui)
	}
	ui = UIConfig{Theme: "nord", ThemeVariant: "dark"}
	migrateUI(&ui)
	if ui.Theme != "nord" || ui.ThemeVariant != "dark" {
		t.Fatalf("migrate kept = %+v", ui)
	}
}

func TestFirstEnvInt(t *testing.T) {
	t.Setenv("FS_TEST_A", "")
	t.Setenv("FS_TEST_B", "7")
	if n, ok := firstEnvInt("FS_TEST_A", "FS_TEST_B"); !ok || n != 7 {
		t.Fatalf("firstEnvInt = %d, %v", n, ok)
	}
	if _, ok := firstEnvInt("FS_TEST_A"); ok {
		t.Fatal("expected no value")
	}
}
