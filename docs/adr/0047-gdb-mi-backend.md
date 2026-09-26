# ADR 0047: GDB/MI debugger backend

## Status

Accepted.

## Context

`pkg/debugger` defines a `Backend` seam with a pure-Go ptrace implementation
and a `gdbmi` backend that satisfied the interface but returned
`ErrNotSupported` for every operation (ADR 0002). The Settings dialog exposed
the backend choice, so selecting `gdbmi` produced a debugger that could not do
anything.

## Decision

Implement the `Backend` over GDB's Machine Interface (`mi2`):

- **Process model.** `Attach` spawns `gdb --interpreter=mi2 -q -p <pid>`, sets
  `confirm`/`pagination` off and probes `-data-list-register-values` so the
  attach stop is processed before returning. `Detach` sends `-target-detach`;
  `Close` sends `-gdb-exit` and reaps the child.
- **Reader and sync.** A goroutine reads stdout and parses MI records
  (`pkg/debugger/mi.go`): `^` result records are matched to the single in-flight
  command, `*stopped` events are queued for `Wait`, and stream/status/notify
  records are ignored. `Continue` drains stale stops before resuming.
- **Operations.** Memory via `-data-read-memory-bytes`/`-data-write-memory-bytes`,
  registers via `-data-list-register-names` + `-data-list-register-values x`,
  `SetRegisters` via `-gdb-set`, breakpoints via `-break-insert *addr` /
  `-break-delete` (tracking gdb's breakpoint numbers), and
  `-exec-step-instruction` / `-exec-continue`. `Wait` maps `*stopped` reasons
  (breakpoint hit, step end, signal, exit) to `StopReason`.
- **Out of scope.** Remote calls and hardware watchpoints are not implemented for
  gdbmi; `Session.SupportsWatchpoints` stays false and the hooking features
  (speedhack, unrandomizer, Auto Assembler) continue to construct ptrace
  explicitly.

## Consequences

- The debugger works with either backend; gdbmi needs `gdb` on `PATH` (or
  `debugger.gdb_path`).
- Commands are serialized and one result is matched per command; a result that
  arrives with no waiter is dropped. Async `*stopped` events are queued.
- `SetRegisters` issues one `-gdb-set` per register and ignores registers the
  target does not have.
- Tests cover the MI parser and the full backend against a scripted fake gdb;
  the real-gdb integration test skips when ptrace is not permitted.
