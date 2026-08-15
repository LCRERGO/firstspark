# ADR 0009: Cheat Engine keyboard shortcut parity

## Status

Accepted.

## Context

Users switching from Cheat Engine expect its keyboard shortcuts. Firstspark
does not implement every CE feature, so binding all of CE's shortcuts would
create dead keys.

## Decision

Bind **Cheat Engine's exact shortcuts for the actions Firstspark implements**,
and leave shortcuts for unimplemented features unbound:

| Shortcut | Action |
| --- | --- |
| `Ctrl+P` | Open Process List |
| `Ctrl+M` | Open Memory Viewer |
| `Ctrl+O` | Load cheat table |
| `Ctrl+S` | Save cheat table |
| `Ctrl+Alt+S` | Save cheat table as |
| `Alt+Shift+S` | Save scan results |
| `Enter` | First / Next Scan (from the value box) |
| `Ctrl+B` | Browse this memory region |
| `Ctrl+D` | Disassemble this memory region |
| `Ctrl+E` | Change value of selected addresses |
| `Ctrl+Alt+E` | Change value back |
| `Delete` | Delete this record (context menu) |

`Ctrl+C` / `Ctrl+V` / `Ctrl+X` / `Ctrl+A` are not bound globally: Fyne already
handles them inside text fields, and binding them application-wide would break
editing. They remain available as Cheat Engine's context-menu entries.

`Ctrl+T` (add scan tab), `Ctrl+Alt+A` (auto assemble), `Ctrl+Alt+D` (dissect),
the Lua shortcuts and the debugger shortcuts (`F5`/`F6`) are intentionally not
bound.

## Consequences

- Muscle memory carries over from Cheat Engine.
- The shortcut set grows with the feature set; adding a feature means adding
  its CE binding.
