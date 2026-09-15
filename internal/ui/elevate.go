//go:build gui

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// elevatedEnv marks a process that was already re-launched with pkexec, so the
// prompt is not shown again.
const elevatedEnv = "FIRSTSPARK_ELEVATED"

// isRoot reports whether the process is running as root.
func isRoot() bool { return os.Geteuid() == 0 }

// ptraceRestricted reports whether Yama restricts ptrace to descendants, which
// is what makes attaching to other processes fail for an unprivileged user.
func ptraceRestricted() bool {
	data, err := os.ReadFile("/proc/sys/kernel/yama/ptrace_scope")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) != "0"
}

// maybeAskElevation offers to restart with pkexec when not already root.
func (a *App) maybeAskElevation() {
	if isRoot() || os.Getenv(elevatedEnv) != "" {
		return
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		fyne.Do(func() {
			message := "Firstspark can attach to processes owned by other users only when run as root."
			if ptraceRestricted() {
				message += " ptrace is also restricted to child processes on this system."
			}
			message += "\n\nRestart with elevated privileges (pkexec)?"
			dialog.NewConfirm("Run with elevated privileges?", message, func(ok bool) {
				if !ok {
					return
				}
				if err := relaunchElevated(); err != nil {
					a.fail(err)
					return
				}
				a.fapp.Quit()
			}, a.win).Show()
		})
	}()
}

// relaunchElevated re-executes this binary as root through pkexec, forwarding
// the display environment the new process needs to open a window.
func relaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("elevate: locate executable: %w", err)
	}
	args := []string{"env"}
	for _, key := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"} {
		if v := os.Getenv(key); v != "" {
			args = append(args, key+"="+v)
		}
	}
	args = append(args, elevatedEnv+"=1")
	args = append(args, exe)
	args = append(args, os.Args[1:]...)

	cmd := exec.Command("pkexec", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("elevate: pkexec failed: %w", err)
	}
	return nil
}
