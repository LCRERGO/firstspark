# ADR 0040: Found-list feature parity

## Status

Accepted.

## Context

ADR 0007 specified the Found list with columns **Address / Value / Previous**,
but the implementation diverged: `internal/ui/results.go` built a one-column
table whose only cell text was `0x<addr>  <value>`, with no header row, no
previous value, no live refresh and no row actions (no double-click, no
context menu, no sorting). The backing model, `scan.Result`, had a single value
field `Prev` that actually held the value of the latest scan (the comparison
baseline for the next scan), so there was no place to store a previous pass.

The reference tool's scan results update their displayed value as the target's value
changes, distinguish static (module-relative) from dynamic addresses, and let
the user add a result to the address list by double-clicking or through a
right-click menu.

## Decision

- **Model**: `scan.Result` becomes `{Addr, Value, Previous}`. `Value` is the
  value observed by the scan that produced the result and remains the baseline
  for the next comparison. On a `Next` scan the new result is
  `{Value: current, Previous: old.Value}`; a `First` scan leaves `Previous`
  empty. The field rename `Prev` → `Value` is mechanical.
- **Columns**: the Found list shows **Address | Value | Previous** with a
  header row and translated `header.*` titles.
- **Live Value**: a UI tick (~500 ms) re-reads each result and paints the Value
  column from the fresh read. The model's `Value` is never overwritten by these
  reads, so scan comparisons stay correct. The read happens on the UI goroutine
  for up to `ui.result_limit` results.
- **Address rendering**: an address inside a file-backed module region is shown
  as `module+0xoffset` and coloured with the theme's success (green) colour;
  every other address is shown as `0x...` in the normal text colour.
- **Interaction**: double-click adds a result to the cheat table; a single
  click selects it and loads it in the memory viewer; right-click opens a menu
  with Change value, Add to Table, Browse, Disassemble, Find what writes,
  Find what accesses and Delete. Delete removes the selected results from both
  the UI slice and the scan session (`Session.Delete`), so a later Next Scan
  does not bring them back.
- **Display order**: cells are resolved through a `foundOrder` index slice
  (identity today). A future sort can reorder it without touching `a.results`
  or the session, which behaviour-sensitive code (Next Scan, Add to Table,
  Ctrl+E) relies on.
- **Isolation**: `setResults` copies the session's result slice, so the UI can
  delete results without mutating the session's backing array.

## Consequences

- The Found list matches the columns ADR 0007 always claimed, and gains the reference tool's
  live value, static colouring and row actions.
- Sorting is not wired yet; `foundOrder` is the seam for it.
- If live-refreshing many results stutters the UI, the reads can move to a
  background goroutine that publishes a snapshot.
- Static detection depends on the current region map; an address in an
  unloaded or renamed module falls back to an absolute address.
- The absolute address of a static result is not shown in the list; operations
  act on the underlying `Result.Addr`, so this is display-only.
