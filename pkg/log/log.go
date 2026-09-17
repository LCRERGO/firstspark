// Package log configures the process-wide structured logger used across the
// engine and the UI. It writes human-readable text to stderr and, when a path
// is configured, to a rotating file so failures can be diagnosed after the
// fact.
package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Level names accepted by ParseLevel.
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// MaxFileBytes is the size at which the log file is rotated once.
const MaxFileBytes = 2 << 20

var (
	mu     sync.Mutex
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	file   *os.File
)

// ParseLevel maps a configuration string to a slog.Level, defaulting to info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn, "warning":
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Setup installs the process logger. When path is non-empty the log is also
// written there, rotating one previous file at MaxFileBytes. When stderr is
// true (headless runs) output also goes to standard error.
func Setup(level string, path string, stderr bool) error {
	var writers []io.Writer
	if stderr {
		writers = append(writers, os.Stderr)
	}
	var opened *os.File
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		rotate(path)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		opened = f
		writers = append(writers, f)
	}
	out := io.Discard
	switch len(writers) {
	case 1:
		out = writers[0]
	default:
		if len(writers) > 1 {
			out = io.MultiWriter(writers...)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
	}
	file = opened
	logger = slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: ParseLevel(level)}))
	return nil
}

// rotate renames an oversized log to a single ".1" backup.
func rotate(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < MaxFileBytes {
		return
	}
	_ = os.Remove(path + ".1")
	_ = os.Rename(path, path+".1")
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
}

// Logger returns the current logger.
func Logger() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	return logger
}

// Debug logs at debug level.
func Debug(msg string, args ...any) { Logger().Debug(msg, args...) }

// Info logs at info level.
func Info(msg string, args ...any) { Logger().Info(msg, args...) }

// Warn logs at warn level.
func Warn(msg string, args ...any) { Logger().Warn(msg, args...) }

// Error logs at error level.
func Error(msg string, args ...any) { Logger().Error(msg, args...) }
