# ADR 0032: Diagnostic logging and target process death

## Status

Accepted.

## Context

Firstspark had no logging at all. Failures were either shown in a modal dialog
or silently discarded, so intermittent problems were impossible to diagnose
after the fact. The most visible one is that the attached target process
sometimes exits abruptly: nothing noticed it, nothing recorded it, and the UI
kept showing the stale process and scan results indefinitely.

## Decision

**Logging.** Add `pkg/log`, a thin wrapper over the standard library's
`log/slog`, with `debug`/`info`/`warn`/`error` levels. It is configured from the
new `log` section of `config.yaml` (`level`, `file`) and the `-log-level` flag.
Output goes to stderr and to a rotating file, by default
`$XDG_STATE_HOME/firstspark/firstspark.log` (one previous file is kept once the
log passes 2 MiB), so the window around a failure survives a restart. Headless
runs also write to stderr.

Engine and UI packages log through this package. Previously swallowed errors
are logged at `debug` or `warn`; the 50 ms freeze loop throttles write failures
to one message per five seconds. Lifecycle events (process selection, attach
and detach, scan start/finish/cancel, speedhack install/remove, auto-assemble
apply/revert, pointer scan, debugger stops) are logged at `info`/`warn`.

**Target death detection.** `pkg/mem` maps `ESRCH` to `ErrNoSuchProcess` and
adds `Process.NotifyExit`, which watches the pid with `pidfd_open` when
available and falls back to polling `/proc`. The UI starts a watcher when a
process is selected and reacts to its exit:

- log a `warn` with the pid, name and, when the debugger was attached, the
  `Wait4` exit code or signal;
- stop the freeze writes, detach and close the debugger session, mark the
  speedhack as no longer applied and log any hook-removal failures;
- clear the selected process, reset the process label and show a status line;
- keep the scan results and cheat table, but stop acting on the dead pid.

Shutdown also releases these resources (`App.shutdown` via Fyne's
`Lifecycle().SetOnStopped`), closing the debugger worker and auto-assemble
backend that previously leaked.

## Consequences

- Intermittent target exits are now timestamped and recorded with a cause when
  one is available, instead of vanishing with the dialog.
- `pkg/mem` gains a dependency on `pidfd_open` (kernel 5.3+), with a portable
  `/proc` fallback.
- The freeze loop, debugger workers and auto-assemble backend no longer outlive
  the target or the application.
