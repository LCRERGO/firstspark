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
editing. They remain available as Cheat Engine's context-menu entries.

`Ctrl+T` (add scan tab) and the Lua shortcuts are intentionally not bound
because the features do not exist.

Global, user-configurable hotkeys (Cheat Engine's Settings ▸ Hotkeys) are a
separate mechanism, described in ADR 0033.

## Consequences

- Muscle memory carries over from Cheat Engine.
- The shortcut set grows with the feature set; adding a feature means adding
  its CE binding.
