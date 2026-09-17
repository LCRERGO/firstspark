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
