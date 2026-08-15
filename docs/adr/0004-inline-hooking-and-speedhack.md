# ADR 0004: Inline trampoline hooking for code injection

## Status

Accepted.

## Context

The speedhack and future features need to run code inside a target that is
already running. Options included `LD_PRELOAD` (launch-time only), GOT/PLT
overwrites (only affects calls routed through the GOT), and inline trampoline
hooking (works for any call site, but is the most fragile).

## Decision

Implement inline trampoline hooking in `pkg/inject`. It maps a code cave in the
target with a remote `mmap`, writes the handler plus a jump back, and builds a
trampoline from the relocated original prologue. Hooking refuses to overwrite
prologues containing RIP-relative operands or relative branches.

The speedhack builds a handler that calls the trampoline, scales the returned
`timespec`/`timeval` fields by a rational factor, and returns.

## Consequences

- Works on already-running processes, like Cheat Engine.
- The relocation restriction means some prologues cannot be hooked; the caller
  receives `ErrUnrelocatable`.
- `clock_gettime`/`gettimeofday` served directly through the vDSO bypass the
  hook.
