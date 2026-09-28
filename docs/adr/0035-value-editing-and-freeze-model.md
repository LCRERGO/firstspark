# ADR 0035: Value editing, live values and the freeze model

## Status

Accepted.

## Context

The cheat table showed the value last written and froze a snapshot captured at
freeze time. Editing a frozen row was therefore silently reverted by the 50 ms
freeze loop, values changed by the target were never shown, and deleting a row
or re-resolving a pointer left stale freeze entries behind. The reference tool instead
shows the live memory value and keeps a per-record frozen value that a manual
change updates.

## Decision

- Every cheat-table row re-reads its value on the existing 500 ms UI tick, so
  the Value column shows the *live value*.
- Frozen state moves onto `tableEntry` (`frozen`, `frozenValue`), mirroring
  the reference tool's per-record `Active`/`FrozenValue`. The 50 ms writer goroutine
  writes a mutex-protected snapshot rebuilt when freeze state, frozen value or
  address changes, so delete, reorder and pointer re-resolution stay correct by
  construction.
- Enabling freeze reads memory fresh, writes it once and stores it as the
  frozen value; disabling leaves the last written value.
- Change value writes once, updates `frozenValue` when the row is frozen, and
  captures `UndoValue` from the live read. `Ctrl+Z` restores `UndoValue` and
  moves the frozen value when the entry is frozen.
- `Ctrl+E` changes the value of the selection in whichever list is active. The
  cheat table is the default, preserving its original target; selecting a Found
  scan result makes that list active. A Found edit writes memory once and
  updates the row, and never touches a cheat-table entry for the same address.
  This removes the earlier deliberate deviation (ADR 0007) where the Found list
  was read-only.
- Deferred: allow-increase/allow-decrease freeze modes, recursive child sets,
  value expressions, read-only custom types, and dissect/Auto-Assembler writes
  moving a frozen value. The cheat-table schema is unchanged: `Frozen` still
  seeds `frozenValue` from the saved value on load.

## Consequences

- Editing a frozen row now behaves like the reference tool: the edit becomes the value
  that is held.
- The Value column reflects the target process, so a failed or overridden freeze
  is visible instead of hidden.
- Frozen state survives row deletion, reordering and pointer re-resolution.
- Found-list edits are independent of the cheat table: a frozen row at the same
  address keeps its frozen value and overwrites the edit on the next tick.
