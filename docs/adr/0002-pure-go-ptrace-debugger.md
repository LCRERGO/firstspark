# ADR 0002: Pure-Go ptrace debugger with a pluggable backend

## Status

Accepted.

## Context

A debugger can be built on `ptrace(2)` directly or by driving GDB over its MI
protocol. GDB is battle-tested but adds a runtime dependency and requires
parsing MI. The project is intended to be a self-contained Go binary.

## Decision

Define a single `debugger.Backend` interface and implement it with a pure-Go
ptrace backend (`golang.org/x/sys/unix`). Ship a `gdbmi` package that satisfies
the same interface but is not yet implemented, so GDB can be plugged in at
runtime without touching callers.

## Consequences

- No GDB or Python runtime dependency.
- Breakpoints, stepping, registers and remote syscalls are under our control.
- We own the complexity of instruction-level debugging and must handle
  relocation and code-page protection ourselves.
