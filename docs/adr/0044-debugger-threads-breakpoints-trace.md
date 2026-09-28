# ADR 0044: Debugger threads, breakpoints, stack and trace

## Status

Accepted.

## Context

The debugger covered attach/detach, continue/step/step-over, registers, remote
calls and hardware watchpoints, but ADR 0016 had deliberately deferred
multi-thread handling, and there was no call stack, no conditional breakpoints,
no breakpoint editing, no trace and no module list. The ptrace backend traces a
single TID.

## Decision

- **Thread list.** The debugger lists `/proc/<pid>/task` and selecting a thread
  rebinds the ptrace session to that TID (detach/close/recreate/re-attach),
  rather than implementing a full multi-tracer. `debuggerTID` defaults to the
  process PID. Full clone/fork-following stays deferred (ADR 0016).
- **Module list.** A tab lists file-backed regions mapped at offset 0 (module
  bases) with base, size and path; selecting one jumps the Memory Viewer there.
  Follow RIP/RSP buttons open the viewer at the current instruction or stack.
- **Conditional breakpoints.** `dbgBreakpoint` carries an `enabled` flag, hit
  count and an optional condition `REGISTER op value` (or `REGISTER op
  REGISTER`). The continue loop evaluates the condition on each breakpoint hit
  and resumes automatically when it is false; an unparsable condition never
  suppresses a stop. Selecting a breakpoint row edits its condition and enabled
  state, and enabled breakpoints are reinstalled on attach.
- **Call stack.** A best-effort frame-pointer walk reads `[RBP]`/`[RBP+8]`,
  bounded to 64 frames and guarded against cycles and non-progressing frames,
  and disassembles each return address. Builds that omit the frame pointer may
  truncate it.
- **Instruction trace.** A bounded single-step trace (step count prompted)
  records RIP and selected registers per step into its own pane; it runs while
  the target is stopped and stops at the requested count or on error.
- **Layout.** The debugger window uses tabs: Registers, Threads, Modules,
  Breakpoints, Hits, Call Stack, Trace.

## Consequences

- The debugger now matches the reference tool's common inspection workflow without a
  multi-thread tracer.
- Selecting a thread loses breakpoints installed in the previous session's
  target memory; enabled breakpoints are reinstalled when the new session
  attaches, and the map is cleared on switch.
- Conditions are intentionally a bounded grammar, not the reference tool's full Lua; memory
  operands and expressions can come later.
- The stack walk and trace are debug-time operations, not hot paths, so their
  per-step reads are acceptable.
