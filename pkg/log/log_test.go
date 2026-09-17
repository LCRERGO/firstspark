package log

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
		"unknown": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSetupWritesToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	if err := Setup("debug", path, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	Info("hello", "n", 1)
	Debug("detailed")
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "hello") || !strings.Contains(text, "detailed") {
		t.Errorf("log missing messages: %q", text)
	}
}

func TestSetupRotatesOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	if err := os.WriteFile(path, make([]byte, MaxFileBytes+1), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	if err := Setup("info", path, false); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	Info("after rotation")
	Close()

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected rotated backup: %v", err)
	}
}
