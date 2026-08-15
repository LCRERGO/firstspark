# ADR 0011: Per-process icons from the icon theme, with an optional X11 path

## Status

Accepted.

## Context

The process list should show a small icon per process, like a task manager.
There is no standard way to get an icon for a Linux process: the icon is not
stored in the executable, and the X11 `_NET_WM_ICON` property only exists for
windows and only under an X server. Firstspark runs on Wayland by default,
where `_NET_WM_ICON` is unavailable for native clients.

## Decision

Resolve icons in a dedicated `internal/ui` component with this priority, first
hit wins:

1. **X11 `_NET_WM_ICON`** for the process's window (via `github.com/jezek/xgbutil`),
   choosing the icon closest to 32px. Only attempted when `DISPLAY` is set.
2. **`WM_CLASS` → `.desktop` `StartupWMClass`** → `Icon=`.
3. **Executable basename → `.desktop` `Exec=`** basename. Wine entries (under
   `~/.local/share/applications/wine/Programs/`) are matched this way; wrapper
   binaries such as `env`, `wine`, `flatpak`, `steam` and `sh` are ignored.
4. **Steam/Proton**: read `SteamAppId` from `/proc/PID/environ` and resolve
   `steam_icon_<appid>` through the icon theme, then Steam's `games/<appid>.png`.
5. A generic application icon.

The `.desktop` index and the icon-theme chain are built once and cached; icons
are resolved **asynchronously** on a background goroutine and cached per PID, so
opening the Process List never blocks the UI. Resolution can be disabled with
the `ui.process_icons` setting.

`github.com/jezek/xgbutil` + `github.com/jezek/xgb` (BSD-3, pure Go, no CGO)
provide the X11 path. The X11 client connects lazily and silently disables
itself when there is no display.

## Consequences

- Icons appear for most desktop applications on both X11 and Wayland, plus
  Wine and Steam/Proton programs.
- Two new pure-Go dependencies are added to the GUI build only; the headless
  build is unaffected.
- Icon resolution is heuristic: processes with no `.desktop` entry and no
  window fall back to the generic icon, which is the expected case for daemons.
- No new dependency on the icon theme's own libraries; a small subset of the
  Icon Theme specification is implemented in `internal/ui/icons.go`.
