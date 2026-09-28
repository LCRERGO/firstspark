# ADR 0009: Reference-tool keyboard shortcut parity

## Status

Accepted.

## Context

Users switching from the reference tool expect its keyboard shortcuts. Firstspark
does not implement every reference-tool feature, so binding all of the reference tool's shortcuts would
create dead keys.

## Decision

Bind **the reference tool's exact shortcuts for the actions Firstspark implements**,
and leave shortcuts for unimplemented features unbound:

| Shortcut | Action |
| --- | --- |
| `Ctrl+P` | Open Process List |
| `Ctrl+T` | New scan tab |
| `Ctrl+M` | Open Memory Viewer |
| `Ctrl+O` | Load cheat table |
| `Ctrl+S` | Save cheat table |
| `Ctrl+Alt+S` | Save cheat table as |
| `Alt+Shift+S` | Save scan results |
| `Enter` | First / Next Scan (from the value box) |
| `Ctrl+B` | Browse this memory region |
| `Ctrl+D` | Disassemble this memory region |
| `Ctrl+E` | Change value of selected addresses |
| `Ctrl+Z` | Undo last edit |
| `Ctrl+Alt+E` | Change value back |
| `Ctrl+Enter` | Change description |
| `Ctrl+Alt+H` | Show the selected record as hexadecimal |
| `Ctrl+H` | Assign a per-entry hotkey |
| `Ctrl+Alt+A` | Auto Assemble |
| `Ctrl+Alt+D` | Dissect Data/Structures |
| `Ctrl+1`..`Ctrl+6` | Memory viewer display width |
| `Delete` / `Enter` / `Space` | Delete / change value / freeze the selected record |
| `F5` / `F6` | Find out what accesses / writes the selected address |
| `F9` / `F7` / `F8` / `F5` | Debugger run / step / step over / toggle breakpoint |

The shortcuts are registered in one place (`internal/ui/shortcuts.go`). Fyne
delivers modified keys to the focused widget, so the list also drives a
dispatch used by the text fields and the code editor, and bare keys (Delete,
Enter, Space, F-keys) are handled by the focused table/list widgets.

`Ctrl+C` / `Ctrl+V` / `Ctrl+X` / `Ctrl+A` are not bound globally: Fyne already
handles them inside text fields, and binding them application-wide would break
editing. They remain available as the reference tool's context-menu entries.

`Ctrl+T` (new scan tab) is bound now that scan tabs exist (ADR 0050); the Lua
shortcuts remain intentionally unbound because the feature does not exist.

Global, user-configurable hotkeys (the reference tool's Settings ▸ Hotkeys) are a
separate mechanism, described in ADR 0033.

## Consequences

- Muscle memory carries over from the reference tool.
- The shortcut set grows with the feature set; adding a feature means adding
  its reference-tool binding.
