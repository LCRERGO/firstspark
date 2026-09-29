# ADR 0053: Reference-tool layout refinements

## Status

Accepted. Refines ADR 0007 (window and panel layout) and ADR 0037/0046 (.CT
fidelity).

## Context

ADR 0007 mirrored the reference tool's window structure, but three details had
drifted from it or were unintuitive:

- The main workspace put the scan controls on the left and the **Found list on
  the right**. The reference tool has the scanned addresses on the left and the
  **memory scan options** on the right, so users reached for controls on the
  wrong side.
- Scan progress lived inside the scan panel, while the reference tool reports it
  in a persistent status bar at the bottom of the window.
- Cheat-table actions (add, change, delete, clear) were only reachable from the
  context menu, with no visible buttons.
- Scan tabs could only be renamed from the Scan menu; the reference tool renames
  a tab by double-clicking it.

## Decision

- **Workspace split** — the Found list occupies the left ~65%, the scan panel
  (memory scan options) the right ~35% (`scanTab.workspace`).
- **Bottom status bar** — the existing bottom bar carries the process label, a
  shared scan-progress line and the status text. The per-tab progress widget is
  that shared line, so progress is visible regardless of scroll position.
- **Grouped scan settings** — the value/scan-type/value-type/compare controls
  and the memory-scan options (writable, alignment, region scope, executable,
  copy-on-write, range) sit in one titled card.
- **Speedhack controls** — the process-wide time hooks follow the reference
  tool's right-hand column: an Unrandomizer toggle, an Enable Speedhack
  checkbox, and (while enabled) a Speed box with an editable value, a
  12-position slider (pause, 0.25×, 0.5×, 1×, 2×, 5×, 10×, 20×, 50×, 100×,
  200×, 500×) and an Apply button. Releasing the slider or pressing Apply
  updates the running hooks in place.
- **Cheat-table actions** — Add Address, Change Value, Delete and Clear are
  buttons under the table, in addition to the context menu.
- **Toolbar grouping** — separators group the process, table-file and tool
  actions.
- **Tab rename by double-click** — Fyne's `DocTabs` exposes no tab-button hook,
  so the scan tabs use a small `tabView` (`internal/ui/tabs.go`) built for this:
  a click selects, a double-click opens the rename prompt (also `F2` and
  Scan ▸ Rename Tab), the trailing `+` creates a tab, and each tab has a close
  button.

## Consequences

- The workspace matches the reference tool's left/right order; ADR 0007's
  description of the split is superseded.
- Only one scan can run at a time per the UI's model, so a single shared
  progress line is sufficient.
- The scan panel no longer contains its own progress bar; it is shorter and
  scrolls less.
