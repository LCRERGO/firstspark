# ADR 0053: Reference-tool layout refinements

## Status

Accepted. Refines ADR 0007 (window and panel layout) and ADR 0037/0046 (.CT
fidelity).

## Context

ADR 0007 mirrored the reference tool's window structure, but three details had
drifted from it or were unintuitive:

- The main workspace put the **Found list on the left** and the scan controls on
  the right. The reference tool has the scan controls on the left and the Found
  list on the right, so users reached for controls on the wrong side.
- Scan progress lived inside the scan panel, while the reference tool reports it
  in a persistent status bar at the bottom of the window.
- Cheat-table actions (add, change, delete, clear) were only reachable from the
  context menu, with no visible buttons.

## Decision

- **Workspace split** — scan controls occupy the left ~35%, the Found list the
  right ~65% (`scanTab.workspace`).
- **Bottom status bar** — the existing bottom bar carries the process label, a
  shared scan-progress line and the status text. The per-tab progress widget is
  that shared line, so progress is visible regardless of scroll position.
- **Grouped scan settings** — the value/scan-type/value-type/compare controls
  and the memory-scan options (writable, alignment, region scope, executable,
  copy-on-write, range) sit in one titled card.
- **Cheat-table actions** — Add Address, Change Value, Delete and Clear are
  buttons under the table, in addition to the context menu.
- **Toolbar grouping** — separators group the process, table-file and tool
  actions.

## Consequences

- The workspace matches the reference tool's left/right order; ADR 0007's
  description of the split is superseded.
- Only one scan can run at a time per the UI's model, so a single shared
  progress line is sufficient.
- The scan panel no longer contains its own progress bar; it is shorter and
  scrolls less.
