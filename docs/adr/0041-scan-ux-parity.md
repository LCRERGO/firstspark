# ADR 0041: Scan UX parity

## Status

Accepted.

## Context

The scan panel diverged from the reference tool in several user-facing ways:

- It offered one scan-type list for both scan phases, so choosing a change-based
  filter (for example *Increased value*) and pressing First Scan produced an
  engine error instead of being unavailable (`pkg/scan/session.go`).
- *Bigger than…* and *Smaller than…* were not scan types; the only way to reach
  `>`/`<` was the free-text Compare field, which was enabled only for Exact.
- There was no *Same as first scan* mode because the first-scan value was not
  retained (`Result` held only `Value`/`Previous`).
- Region filtering was limited to a writable default and a coarse scope; the reference tool's
  Executable tri-state, Copy-on-write and Start/Stop range were absent, even
  though `mem.Region.Executable()`/`Private()` existed.
- Grouped scans (`4:75 4:* 4:100`) were unsupported.

## Decision

- **Scan-type list is phase-aware.** `firstScanModes` (Exact, Bigger, Smaller,
  Between, Unknown) and `nextScanModes` (those plus Increased, Increased by,
  Decreased, Decreased by, Changed, Unchanged, Same as first scan) drive the
  dropdown, which swaps when a session starts and when the process changes.
- **Bigger/Smaller are engine modes.** `ModeBigger`/`ModeSmaller` match with a
  fixed `>`/`<`; the Compare control becomes a dropdown (`== != > >= < <=`)
  shown only for Exact, so the advanced operators stay reachable.
- **First value is retained.** `Result` gains `First`, captured by the initial
  scan and carried through every Next. `ModeSameAsFirst` keeps addresses whose
  current value still equals `First`.
- **Memory filters.** `Options` gains `Executable` (Any/Only/Non-executable),
  `CopyOnWrite`, and `Start`/`Stop`. Executable and copy-on-write are applied in
  `regionInScope`; Start/Stop clip each selected region. An explicit per-region
  selection still wins over the scope.
- **Grouped scans.** A new `TypeGrouped` value type with `ParseGrouped` accepts
  space-separated `type:value` segments (1/2/4/8, f, d, s) where `*` is a
  wildcard. Segments match contiguously at the scan alignment; per-segment
  offsets are out of scope for now. The CLI exposes it as
  `--type grouped --value "4:75 4:* 4:100"`.
- **Headless parity.** New modes and filters are reachable from the CLI
  (`--mode bigger|smaller|same as first`, `--exec`, `--cow`, `--start`,
  `--stop`), per the UI-agnostic engine rule.

## Consequences

- The scan panel now mirrors the reference tool's two-phase behaviour and no longer reaches the
  engine's invalid-mode error path.
- `Result` is one `Value` larger; history and undo copy it unchanged.
- Grouped results display as spaced hex bytes, because a stored `Value` cannot
  carry the segment pattern; richer decoding can come later.
- Start/Stop clip regions rather than filtering addresses, so a range that
  spans regions still scans the intersection.
- A follow-up ADR can add per-segment offsets/gaps and a grouped decoder.
