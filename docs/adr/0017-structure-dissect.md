# ADR 0017: Structure dissect

## Status

Accepted.

## Context

The reference tool's "Dissect data/structures" compares a memory layout across several
addresses, guesses field types and lets the user follow pointers. It needs only
memory access.

## Decision

Implement a minimal structure dissect: pick a base address and a region size,
compare the same offsets across several instances, guess a field type per
offset from the observed bytes, allow editing a field, and allow following a
pointer field to a new base.

## Consequences

- A layout can be explored and edited without leaving the app.
- Type guessing is heuristic; the user can override the guessed type.
