# ADR 0045: Tools and settings parity

## Status

Accepted.

## Context

The A–F roadmap left a few reference-tool tools and settings surfaces unimplemented:
there was no Lua console (although `pkg/celua` already backed table scripts),
no "New table", the refresh cadence was hardcoded, several config keys had no
UI, and there was no Unrandomizer. ADR 0010 had declared reference-tool "Table Extras /
Advanced Options" out of scope.

## Decision

- **Lua Engine console.** `celua.Config` gains `Output func(string)` and the
  runtime registers a `print` global that renders arguments tab-separated and
  forwards them to `Output` (falling back to `Show`). A **Lua Engine** window
  shows an output log and an input entry with Up/Down command history and a
  Clear button; chunks are evaluated against the shared runtime, so globals
  persist between the console and table scripts. The console runs on the UI
  goroutine because the runtime is not thread-safe.
- **New cheat table.** File ▸ New (`Ctrl+N`) clears the cheat table, scan
  session/results and symbols, with a confirmation when the table is not empty.
  Clearing also unbinds the table's hotkeys, fixing a leak in Clear List.
- **Settings.** The Settings dialog gains the UI **refresh interval**
  (`ui.refresh_ms`, clamped to ≥ 50 ms and used for the cheat-table/Found
  refresh tick), **snapshot limit**, **float epsilon**, **log level**, and the
  **debugger backend** and **gdb path**. Log level and backend take effect on
  the next launch / next session.
- **Unrandomizer.** A new `pkg/unrandomizer` resolves libc `rand`, `random` and
  `rand_r` with the speedhack symbol resolver and installs a `mov rax, value;
  ret` handler through `pkg/inject`, returning a constant. It is toggled from
  the scan panel with a value field, mirroring the speedhack.
- **Out of scope.** The reference tool's Table Extras / Advanced Options remain out of scope
  (ADR 0010), as does the gdbmi backend (still a stub).

## Consequences

- The Lua console makes the existing in-process Lua runtime interactively
  usable without changing how table scripts run.
- New/refresh/settings close the remaining menu and configuration gaps.
- The unrandomizer is constant-return only; a counter/pattern mode would be a
  follow-up.
- `ui.refresh_ms` trades CPU for responsiveness but does not change the 50 ms
  freeze write cadence.
