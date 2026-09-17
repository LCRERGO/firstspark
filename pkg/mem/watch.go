package mem

import (
	"context"
	"time"

	"golang.org/x/sys/unix"
)

// watchInterval is how often the /proc fallback polls for process exit.
const watchInterval = 500 * time.Millisecond

// NotifyExit returns a channel that is closed when the process exits. It uses
// pidfd_open when the kernel and permissions allow it, falling back to polling
// /proc. Cancelling ctx stops the watcher without closing the channel, so a
// caller that changes target must check the pid it is still watching.
func (p *Process) NotifyExit(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if p == nil || p.PID <= 0 {
		close(done)
		return done
	}
	go func() {
		if p.watchPidfd(ctx) {
			close(done)
		}
	}()
	return done
}

// watchPidfd returns true when the process exited and false when the watcher
// was cancelled or pidfd is unavailable.
func (p *Process) watchPidfd(ctx context.Context) bool {
	fd, err := unix.PidfdOpen(p.PID, 0)
	if err != nil {
		return p.watchProc(ctx)
	}
	defer unix.Close(fd)
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, 500)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false
		}
		if n > 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
	}
}

// watchProc is the /proc polling fallback used when pidfd_open is denied.
func (p *Process) watchProc(ctx context.Context) bool {
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if !p.Exists() {
				return true
			}
		}
	}
}
