package config

import (
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
}
