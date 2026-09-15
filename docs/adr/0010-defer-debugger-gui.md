# ADR 0010: Defer the debugger GUI; bound the GUI feature scope

## Status

Superseded by ADR 0016.

## Context

The engine already provides a full debugger (`pkg/debugger`: attach, register
access, breakpoints, step/continue/wait) and a speedhack, but no UI. Building a
debugger front-end (register panes, breakpoint list, disassembly stepping,
thread list) is a project in itself. The GUI rework also risks scope creep into
Lua, auto-assembly, structure dissection and plugins — all Cheat Engine
features Firstspark does not have.

## Decision

The GUI implements this feature set:

- Process List, memory scanning with CE-style controls, the Found list and the
  cheat table, freezing, value editing, Add Address Manually, Memory Viewer
  (hex + disassembler), Settings, Speedhack toggle, and cheat-table
  load/save/export (`.CT` XML and `.json`).

Explicitly **out of scope** for this work: the debugger UI, Lua engine,
auto-assemble, structure dissection, plugins, languages, D3D, scan tabs,
pointer scan, unrandomizer, undo scan, and CE's Table Extras / Advanced Options.

## Consequences

- The GUI ships as a coherent scanner/debugger-lite front-end without
  half-finished panes.
- The `F5`/`F6` access/write shortcuts stay unbound until the debugger UI
  exists; the debugger and assembler remain fully usable from the CLI and from
  Go.
