//go:build gui

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// Environment passed to a pkexec re-launch.
const (
	elevatedEnv = "FIRSTSPARK_ELEVATED" // marks an already elevated process
	parentEnv   = "FIRSTSPARK_PARENT_PID"
	origUIDEnv  = "FIRSTSPARK_ORIG_UID"
	origGIDEnv  = "FIRSTSPARK_ORIG_GID"
)

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
			content := widget.NewLabel(elevationMessage())
			content.Wrapping = fyne.TextWrapWord
			d := dialog.NewCustomConfirm("Restart with elevated privileges?",
				"Restart elevated", "Continue unprivileged",
				container.NewPadded(content), func(ok bool) {
					if ok {
						a.restartElevated()
					}
				}, a.win)
			d.Resize(fyne.NewSize(500, 320))
			d.Show()
		})
	}()
}

func elevationMessage() string {
	var b strings.Builder
	b.WriteString("Attaching to processes owned by other users needs root or CAP_SYS_PTRACE.")
	if ptraceRestricted() {
		b.WriteString(" ptrace is also restricted to child processes on this system, so attaching to any other process needs it too.")
	}
	b.WriteString("\n\nRestarting asks for your password with pkexec and runs Firstspark as root, " +
		"keeping your normal configuration and data files. The current window closes once the new one starts.\n\n" +
		"To stay unprivileged, you can instead grant the capability once:\n\n" +
		"    sudo setcap cap_sys_ptrace+ep bin/firstspark")
	return b.String()
}

// restartElevated launches an elevated copy and waits for it in the background
// so that an authentication failure can be reported. The elevated copy closes
// this instance once it has started.
func (a *App) restartElevated() {
	args, err := elevatedArgs()
	if err != nil {
		a.fail(err)
		return
	}
	cmd := exec.Command("pkexec", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		a.fail(fmt.Errorf("elevate: cannot start pkexec: %w", err))
		return
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			fyne.Do(func() { a.fail(fmt.Errorf("elevate: restart did not complete: %w", err)) })
		}
	}()
}

// elevatedArgs builds the pkexec arguments, forwarding the display environment
// the elevated window needs and the user's XDG directories so it reads the
// same configuration.
func elevatedArgs() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("elevate: locate executable: %w", err)
	}
	home := os.Getenv("HOME")
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" && home != "" {
		configHome = filepath.Join(home, ".config")
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" && home != "" {
		dataHome = filepath.Join(home, ".local", "share")
	}

	args := []string{"env"}
	add := func(k, v string) {
		if v != "" {
			args = append(args, k+"="+v)
		}
	}
	add("DISPLAY", os.Getenv("DISPLAY"))
	add("WAYLAND_DISPLAY", os.Getenv("WAYLAND_DISPLAY"))
	add("XAUTHORITY", os.Getenv("XAUTHORITY"))
	add("XDG_RUNTIME_DIR", os.Getenv("XDG_RUNTIME_DIR"))
	add("DBUS_SESSION_BUS_ADDRESS", os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	add("XDG_CONFIG_HOME", configHome)
	add("XDG_DATA_HOME", dataHome)
	add(elevatedEnv, "1")
	add(parentEnv, strconv.Itoa(os.Getpid()))
	add(origUIDEnv, strconv.Itoa(os.Getuid()))
	add(origGIDEnv, strconv.Itoa(os.Getgid()))
	args = append(args, exe)
	args = append(args, os.Args[1:]...)
	return args, nil
}

// closeParentInstance terminates the unprivileged instance that launched this
// elevated copy, once the new window is up.
func closeParentInstance() {
	pidStr := os.Getenv(parentEnv)
	if pidStr == "" {
		return
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return
	}
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}()
}
