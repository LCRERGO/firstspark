# ADR 0027: Asynchronous, cancellable, capped, parallel scanning

## Status

Accepted. The result cap is split into a collection limit and a display limit by
ADR 0054.

## Context

Scans ran **synchronously on Fyne's UI goroutine**, so the window could not
repaint until a scan finished, there was no progress and no way to cancel. On a
large process this looked like an infinite hang. Unknown-value scans were worse:
`consider` appended one `Result` (with a copied `Value`) per candidate and only
stopped at `snapshot_limit` (2 GiB) — hundreds of millions of results, tens of
GB of RAM. `ResultLimit` was applied only after the scan, so the engine still
collected everything. scanmem, by comparison, filters regions, reads in blocks,
polls a stop flag, and reports a 0..1 progress value.

## Decision

Rework the scan engine and its UI driver:

- `Session.First`/`Next` take a `context.Context` and an `onProgress
  func(Progress)` callback (`Progress` = scanned bytes, total bytes, matches).
  They scan into a local slice and only replace the results on success, so a
  cancelled scan discards its work and leaves the previous results untouched.
- Scans run **in parallel** across regions (and across the existing results for
  a next scan), using `runtime.NumCPU()` workers; per-worker results are merged
  in order. Progress counters are atomics, reported by a 100 ms ticker.
- A **result cap** (`Options.MaxResults`, wired to `ui.result_limit`, default
  1000) stops the scan once reached; the cap applies to unknown scans too.
- A **region scope** (`Options.Scope`) chooses *All writable* (default,
  reference-tool-compatible), *Heap + stack + exec + BSS* (scanmem-style, much
  faster), or *All readable*.
- The GUI runs the scan on a background goroutine, shows a **determinate
  progress bar and a status line**, disables the scan buttons, and offers a
  **Stop** button that cancels the context.

## Consequences

- The window stays responsive; long scans show progress and can be cancelled.
- Memory use is bounded by the result cap, so unknown scans are usable.
- Results are deterministic for a given region set, though parallel workers may
  change the order relative to a strictly sequential scan (the cap is applied
  after merging).
- `SnapshotLimit` is no longer the primary guard; the result cap supersedes it.
