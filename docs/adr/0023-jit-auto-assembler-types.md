# ADR 0023: Local JIT for Auto-Assembler custom types

## Status

Accepted.

## Context

The reference tool lets a custom type be defined by an Auto Assembler script whose
conversion routines are native code, so the scan loop runs at native speed.
Reproducing that by calling the routines *in the target* is not viable: a scan
converts every candidate address, and each remote call is a stop-resume cycle
(ADR 0022). The reference tool's routines run in its own process.

## Decision

Assemble the script's `[ENABLE]` section with `pkg/asm` and load the resulting
machine code into **this** process with a new `pkg/jit` package:

- `pkg/jit` (build-tagged `cgo && linux && amd64`, with a `!cgo` stub) `mmap`s a
  writable region, copies the code, then `mprotect`s it read-execute (W^X), and
  provides a C scratch buffer. Calls go through a small assembly trampoline that
  preserves the callee-saved registers, so a routine that ignores the ABI cannot
  corrupt the Go runtime.
- `pkg/customtype` assembles the section twice: once at address 0 for its size,
  then at the real JIT address so any absolute references are correct.
- Convention for the script: a `ConvertRoutine` label (pointer to the bytes in
  RDI, value in RAX) and an optional `ConvertBackRoutine` label (value in RDI,
  output pointer in RSI). Only `kind: int` is supported; float and string AA
  types are rejected.

Because the routines run locally, AA types are registered at startup like Lua
types and need no target process.

## Consequences

- AA-defined types convert at native speed inside scans.
- A malformed script runs arbitrary native code in the Firstspark process and
  can crash it; this is the same trust model as the reference tool's Auto Assembler.
- The JIT needs CGO, so the headless (CGO-free) build returns "not supported"
  for AA types; Lua types work everywhere.
