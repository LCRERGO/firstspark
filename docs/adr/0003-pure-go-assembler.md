# ADR 0003: Pure-Go disassembler and assembler

## Status

Accepted.

## Context

Cheat Engine lets users type Intel-syntax assembly and patch it into the target.
The usual implementations bind to Capstone and Keystone through cgo, which adds
a C toolchain and system library requirement to every build and complicates
`go install`.

## Decision

Disassemble with the pure-Go `golang.org/x/arch/x86/x86asm`. Write a small,
table-driven Intel-syntax encoder in `pkg/asm` covering the instruction subset
needed for patching and trampolines, with label support via `AssembleProgram`.
Unsupported mnemonics return an error, and a raw-byte path is always available.

## Consequences

- The project builds with no cgo or system libraries for the core engine.
- The assembler is intentionally incomplete and will grow as needed.
- We maintain the encoding logic ourselves.
