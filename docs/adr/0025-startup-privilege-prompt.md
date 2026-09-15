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
The dialog explains why privileges help, offers "Restart elevated" / "Continue
unprivileged", and shows the `setcap cap_sys_ptrace+ep` alternative.

If the user accepts, the binary is re-executed through `pkexec`, forwarding:

- the display environment (`DISPLAY`, `WAYLAND_DISPLAY`, `XAUTHORITY`,
  `XDG_RUNTIME_DIR`, `DBUS_SESSION_BUS_ADDRESS`), so the elevated window opens;
- `XDG_CONFIG_HOME` / `XDG_DATA_HOME` (defaulted from `HOME`), so the elevated
  process reads and writes **the user's** configuration, not root's;
- `FIRSTSPARK_ORIG_UID` / `FIRSTSPARK_ORIG_GID`, so saved configuration files are
  chowned back to the invoking user;
- `FIRSTSPARK_PARENT_PID` and `FIRSTSPARK_ELEVATED=1`.

The elevated instance terminates the original one (`SIGTERM` to the parent PID)
once its window is up, and does not prompt again. `pkexec` is waited on in the
background so an authentication failure is reported on the original window
instead of closing it. Declining continues unprivileged.

The prompt is shown at every startup (as requested) and there is no persistent
"don't ask again". No setuid binary or privileged helper is introduced.

## Consequences

- Users can elevate in one click instead of diagnosing ptrace permissions.
- A root GUI needs the forwarded display environment; under Wayland the
  elevated process may still be unable to open a window, in which case the user
  should run unprivileged with `setcap cap_sys_ptrace+ep` instead.
- Running the GUI as root remains the user's explicit choice, never automatic.
