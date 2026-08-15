# ADR 0007: Cheat Engine-faithful window and panel layout

## Status

Accepted.

## Context

Firstspark deliberately mirrors Cheat Engine (CE). Users coming from CE expect
its window structure: a separate Process List window, a separate Memory Viewer
window, a "Found" results list next to the scan controls, and a cheat table
below them. An earlier design considered docking the process list and merging
the result lists, which would have been simpler but less familiar.

## Decision

Reproduce CE's layout:

- **Main window** — top region (~74% height) holds the process label on the
  left, the Found list (columns Address / Value / Previous, ~46% of the width)
  and the scan panel on the right; a vertical splitter separates it from the
  cheat table (~26%) with columns Active / Description / Address / Type /
  Value.
- **Process List** is a separate window (480x480), opened from the process
  label, the toolbar or `Ctrl+P`. It shows a header row plus rows of
  **Icon / Name / PID / User**, with a filter box. Every non-icon column is
  sortable by clicking its header (click again to reverse); the default sort is
  Name ascending, sorting runs over the filtered set, and the selected process
  is tracked by PID so it survives re-sorting. The user column resolves the
  UID through a cached `/etc/passwd` lookup, falling back to the number.
- **Memory Viewer** is a separate window (801x530) with the disassembler on
  top (~69%) and the hex dump below (~31%).
- The main window has **no status bar**; state is shown by the process label
  and a "Found:" count, and failures use modal dialogs.
- Scan results land in the Found list; the user copies them into the cheat
  table with **Add to Table** (or the equivalent action), where freezing and
  editing happen through the row context menu.

## Consequences

- The layout is familiar to CE users and keeps the two result views distinct.
- More windows to manage than a single-window design, and more code than a
  merged table.
- Split proportions are user-adjustable through the Fyne splitters.
