# ADR 0016: Debugger UI and find-accesses/writes

## Status

Accepted. Supersedes ADR 0010.

## Context

ADR 0010 deferred the debugger GUI: `pkg/debugger` already provides attach,
registers, breakpoints and stepping, but nothing exposed it. Cheat Engine's
"find out what accesses/writes this address" is a core workflow and requires
the debugger.

## Decision

Expose the debugger in the GUI:

- Hardware watchpoints (DR0-DR3) for "find out what accesses this address" and
  "find out what writes to this address", falling back to software breakpoints
  with single-stepping where hardware watchpoints are unavailable.
- Software INT3 breakpoints, a breakpoint list, a register view, and
  run/step/step-over controls.
- Scope to the main thread first; multi-thread handling is deferred.

Each hit shows the instruction, its disassembly and a register snapshot.

## Consequences

- The debugger and assembler become reachable from the GUI, closing the gap
  ADR 0010 left open.
- Hardware watchpoints are limited to four per thread, as on the CPU.
