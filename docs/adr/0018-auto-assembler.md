# ADR 0018: Auto Assembler

## Status

Accepted.

## Context

Cheat Engine's Auto Assembler (AA) turns a script into assembled code, allocates
memory, writes the code and optionally hooks a target function. Firstspark
already has an assembler (`pkg/asm`) and trampoline hooking (`pkg/inject`); the
AA language is what is missing.

## Decision

Implement an Auto Assembler on top of `pkg/combinator` and `pkg/asm`, covering
this subset: `alloc`/`dealloc`, `label`, `define`, `db`/`dd`/`dw`, raw
instruction writes, `nop`, `jmp`, `aobscan`/`aobscanmodule`, `registersymbol`,
and `[enable]`/`[disable]` sections.

Deferred: `createthread`, `{$lua}` blocks, structure definitions, and the
broader AA command set.

## Consequences

- The common patch-and-hook workflow is scriptable from the GUI.
- The AA grammar is another consumer of `pkg/combinator`, keeping parsing
  consistent across languages.
