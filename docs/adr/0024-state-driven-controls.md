# ADR 0024: State-driven control enabling (feature parity)

## Status

Accepted.

## Context

The reference tool greys out the controls that do not apply to the current state:
"Next Scan" and "Undo Scan" are disabled until a first scan has run, the scan
value box is only active for value-based scan types, and toolbar actions such
as Save are disabled when there is nothing to save. Firstspark enabled every
control at all times, which is a visible gap from the reference tool and invites
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

The scan region is also laid out as in the reference tool: the three scan buttons
(First Scan / Next Scan / Undo Scan) at the top, then the scan value with the
Hex checkbox beside it, the Scan Type and Value Type dropdowns, then the
Memory Scan Options. The value box is **dynamic**: its placeholder describes
what the selected scan type expects (`value`, `lower bound`, `delta`,
`not used`), and the second "and" value row only appears for "Value between".

## Consequences

- Controls communicate what is possible, as in the reference tool.
- All enabling logic lives in one function, so new controls are added there.
