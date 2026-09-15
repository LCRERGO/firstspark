# ADR 0015: Pointer scanner

## Status

Accepted.

## Context

Cheat Engine's pointer scan finds a chain of pointers (a static base plus
offsets) that resolves to a target address. It is one of its most used
features and needs only memory access, not a debugger.

## Decision

Implement an N-level pointer scanner over a pointermap (a reverse index of
pointer values to the addresses holding them, tagging addresses inside loaded
modules as static with a module index and offset).

Defaults follow Cheat Engine: maximum level 5, maximum offset/struct size 2048,
aligned offsets only, static-only off, no-loop on. Results are saved as `.ptr`
files, and a generated pointermap can be cached to disk for fast rescans.

## Consequences

- Pointer scanning becomes available without a debugger.
- Building the pointermap is memory- and time-intensive; caching it to disk is
  what makes repeated scans practical.
