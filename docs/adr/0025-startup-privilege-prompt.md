# ADR 0025: Startup privilege prompt via pkexec

## Status

Accepted. Amends ADR 0005.

## Context

Attaching to processes owned by other users, and attaching at all when Yama's
`ptrace_scope` is not 0, requires root or `CAP_SYS_PTRACE`. ADR 0005 chose to
stay unprivileged and only re-exec under `sudo` on request, which left users to
discover the permission problem themselves. Running the GUI as root is awkward
because the elevated process must keep `DISPLAY`/`WAYLAND_DISPLAY`/`XAUTHORITY`
to open a window, and `sudo` needs a terminal.

## Decision

On every GUI startup, when the process is not already root and was not launched
by a previous elevation, show a dialog offering to restart with **`pkexec`**.
If the user accepts, re-exec the binary through `pkexec`, forwarding `DISPLAY`,
`WAYLAND_DISPLAY`, `XAUTHORITY`, `XDG_RUNTIME_DIR` and
`DBUS_SESSION_BUS_ADDRESS`, and set `FIRSTSPARK_ELEVATED=1` so the new process
does not prompt again. Declining continues unprivileged.

The prompt is shown at every startup (as requested) and there is no persistent
"don't ask again". No setuid binary or privileged helper is introduced.

## Consequences

- Users can elevate in one click instead of diagnosing ptrace permissions.
- A root GUI needs the forwarded display environment; under Wayland the
  elevated process may still be unable to open a window, in which case the user
  should run unprivileged with `setcap cap_sys_ptrace+ep` instead.
- Running the GUI as root remains the user's explicit choice, never automatic.
