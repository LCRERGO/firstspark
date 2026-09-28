# ADR 0019: Phased delivery for the feature parity work

## Status

Accepted.

## Context

The remaining work (scripting language, custom types, scanning gaps, pointer
scanner, debugger UI, structure dissect, Auto Assembler) spans several projects
with shared foundations. Doing it in one pass would be unreviewable.

## Decision

Deliver in phases, each committed and measured with `navune analyze .`:

- **Phase 0** — `pkg/combinator` + `pkg/script` (language, tests).
- **Phase 1** — type-system refactor, custom types, scanning gaps (Value
  between, Undo Scan, All, Binary), cheat-table upgrades (pointer records,
  hex/binary display, hotkeys), and the editor widget.
- **Phase 2** — pointer scanner (ADR 0015).
- **Phase 3** — debugger UI and find-accesses/writes (ADR 0016).
- **Phase 4** — structure dissect (ADR 0017).
- **Phase 5** — Auto Assembler (ADR 0018).

The type refactor underpins Phase 1 and later, so it precedes feature work.
Quality hotspots touched by a phase are refactored within it, and the navune
composite index is reported at each checkpoint.

## Consequences

- Reviewable checkpoints, and a quality trend rather than a single measurement.
- Later phases build on a stable type system and language.
