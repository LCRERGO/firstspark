# ADR 0049: Scan performance

## Status

Accepted.

## Context

Scanning is the hottest path in the tool. Profiling the engine showed four
costs that dominated large scans:

- every candidate in a first scan allocated a copy of its bytes (`matchExact`
  called `NewValue`), so an exact scan over a gigabyte made hundreds of
  millions of small allocations;
- the type registry lock (`TypeByID`) was taken per candidate;
- a next scan read each result with its own `process_vm_readv`, so filtering a
  million results issued a million syscalls;
- a single large region was scanned by one worker, so a big heap did not use
  the other cores.

## Decision

- **Cache derived state.** `Session` caches the resolved `*Type`, the candidate
  width and any AOB/Binary patterns when the options change, so the per-candidate
  path never takes the registry lock.
- **Compare without copying.** Candidate comparison aliases the read buffer
  (`Value{Type, Raw}`); `NewValue` (which copies) is only used when a match is
  stored. The match counter is accumulated per chunk, not with an atomic per
  candidate.
- **Specialized integer loop.** Exact/Bigger/Smaller scans over the builtin
  integer types decode each candidate directly and compare against a
  predecoded target, avoiding the generic dispatch.
- **Batched next scans.** `filterResults` visits results in address order and
  re-reads them in 64 KiB windows, so one read covers many results. The kept
  results are reassembled in the original order and `Previous`/`Value` are
  updated as before.
- **Region splitting.** Regions larger than 32 MiB are tiled into sub-regions so
  the existing worker pool parallelises within them. Chunk reads always ask for
  the width-overlap bytes past the chunk end, so a value spanning a chunk or
  sub-region boundary is still matched.
- Benchmarks (`BenchmarkScanBytesExact`, `BenchmarkScanBytesUnknown`) and
  integration tests for the next scan and for split regions guard the behaviour.

## Consequences

- An exact dword scan went from ~0.2 GB/s to ~2 GB/s per core in the
  benchmark, with the per-candidate allocations (previously one per candidate)
  removed; a next scan issues one syscall per window instead of one per result.
- A first scan's result order over split regions is interleaved by worker (it
  was already per-region); a next scan preserves the input order.
- Capping to `MaxResults` in a first scan is now checked per chunk, so the
  intermediate result set can overshoot the cap slightly before it is truncated.
- The specialized loop covers builtin integer types only; custom, grouped,
  string, AOB and floating-point types use the general path.
