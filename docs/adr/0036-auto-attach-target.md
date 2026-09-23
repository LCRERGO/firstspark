# ADR 0036: PINCE-style auto-attach to the target process

## Status

Accepted. Relates to ADR 0032 (target death) and ADR 0033 (global hotkeys).

## Context

Firstspark always required the user to pick the target by hand, either from the
Process List (`Ctrl+P`) or the "attach to the foreground process" hotkey. When
the target crashes or is relaunched, `processGone` (ADR 0032) clears the
selection and the user has to choose the new instance manually, then start over.

PINCE solves this differently. It has no "restart" command; instead a poller
(`auto_attach_loop`) runs while nothing is attached and attaches to the first
process whose name matches a configured pattern (a regex, or `;`-separated
substrings with earlier entries taking priority). A relaunched target is picked
up automatically on the next tick. PINCE calls `ptrace` here, but its *effect*
for the user is "the target comes back on its own".

## Decision

Add an opt-in auto-attach poller that mirrors PINCE's matching semantics:

- `process.auto_attach` is a semicolon-separated list of process-name
  substrings, matched in order (earlier entries win, lowest PID within an
  entry), or a single regular expression when `process.auto_attach_regex` is
  set. Matching is case-sensitive, as in PINCE. An empty pattern disables the
  feature; there is no separate enable flag.
- The poller runs every second, and only while no target is selected. It calls
  `mem.List()` and reuses the normal process-selection path, so the scan
  session, region selection and results reset exactly as for a manual choice,
  and the cheat table is kept (addresses are not re-resolved; that is the
  separate "target-restart resilience" idea, still deferred).
- "Attach" here means **selecting the memory target**, not `ptrace`: scanning
  uses `process_vm_readv` and needs no debugger. The ptrace debugger stays a
  separate, explicitly triggered session. Auto-attach therefore does not pause
  the target or demand `CAP_SYS_PTRACE`.
- The pattern is editable in Edit ▸ Settings and applies live through a
  mutex-protected snapshot; an invalid regex is logged and reported in the
  status line once per change, not with a repeating modal. It is a background
  convenience, unlike the startup-only `ui.scale`/language settings.
- Auto-attach is GUI-only (it lives in the `gui`-tagged package).

## Consequences

- A crashed-or-relaunched target is re-selected without user action, which is
  the behavior users expect from PINCE.
- Because the poller only acts while no target is selected, it never overrides a
  manual choice; the user must let the target exit (or use a future "detach"
  action) before it fires again.
- A permissive process-name pattern can select an unintended process; the
  feature is off by default and matching is case-sensitive and ordered, so the
  user can steer it.
- The cheat table is kept but not re-resolved across the restart; pointer
  entries with a module base still refresh on the 500 ms tick, absolute
  addresses do not. Full bookmark recalculation (region + offset) remains
  deferred.
