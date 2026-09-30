# ADR 0054: Decouple collection limit from display limit

## Status

Accepted. Supersedes the single-cap behaviour of ADR 0027.

## Context

ADR 0027 introduced a result cap (`Options.MaxResults`, wired to
`ui.result_limit`, default 1000) to stop unknown-value scans from collecting
hundreds of millions of results. It made the engine stop scanning at the cap
and truncate to it.

That conflated two different concerns. The cap that protects memory is a
*collection* limit; the number of rows a user can usefully scan by eye is a
*display* limit. Because they were one value, a scan that found more than 1000
matches stopped early, and a next scan could only filter those 1000 — the
remaining matches were never seen and could not be narrowed to.

## Decision

- **Split the limits.** `scan.collect_limit` (default 1,000,000) bounds how many
  matches the engine collects and is what stops the scan; `ui.result_limit`
  (default 1000) only caps how many collected results the Found list displays.
- **Next scans filter everything collected.** The scan session keeps every
  collected result; the Found list is a window over it. The display cap is
  re-applied after each scan step, never to the session.
- **Bound the undo history.** `Session.history` now drops the oldest snapshots
  once they exceed a 256 MiB struct budget as well as the 16-step limit, so it
  cannot grow with a large collection limit.
- **Refresh only what is displayed.** The Found list re-reads live values for
  the displayed rows on a background goroutine, not for every collected result.
  Sorting by the live Value column performs a one-time full read of the
  collected set, because that sort must compare everything.
- **The Engine option is renamed** `Options.MaxResults` → `MaxCollected` to say
  what it is.

## Considered Options

- **Keep the single cap** and only raise the default: still conflates the two
  concerns and still makes a next scan miss matches.
- **No collection bound**: re-opens the memory blow-up ADR 0027 fixed.

## Consequences

- A scan can now find far more than the list shows; the count label reports the
  collected total and the displayed number separately.
- Memory is bounded by `scan.collect_limit` (default 1M) rather than
  `ui.result_limit`; raising the collection limit costs memory.
- `ui.result_limit: 0` displays every collected result.
