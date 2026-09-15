# ADR 0024: State-driven control enabling (Cheat Engine parity)

## Status

Accepted.

## Context

Cheat Engine greys out the controls that do not apply to the current state:
"Next Scan" and "Undo Scan" are disabled until a first scan has run, the scan
value box is only active for value-based scan types, and toolbar actions such
as Save are disabled when there is nothing to save. Firstspark enabled every
control at all times, which is a visible gap from Cheat Engine and invites
clicks that can only fail.

## Decision

Drive the enabled state of the scan controls and toolbar actions from the
application state in one place (`App.updateScanControls`):

- **First Scan** — enabled when a process is selected.
- **Next Scan** — enabled when a scan session exists.
- **Undo Scan** — enabled when the session has an undo step.
- **Scan value / upper bound** — enabled only for the scan types that use them
  (`and` only for "Value between").
- **Compare** — enabled only for exact scans.
- **Toolbar** — Save/Save As/Clear List require a non-empty cheat table;
  Memory View and Add Address Manually require a selected process.

The function runs after every state change (process selected, scan, undo,
scan-type change, table edit/load/clear).

The scan region is also laid out as in Cheat Engine: the scan value with the
Hex checkbox beside it, the second value on an "and" row, the Scan Type and
Value Type dropdowns, the three scan buttons, then the Memory Scan Options.

## Consequences

- Controls communicate what is possible, as in Cheat Engine.
- All enabling logic lives in one function, so new controls are added there.
