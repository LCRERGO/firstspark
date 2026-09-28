# ADR 0033: Configurable global hotkeys

## Status

Accepted. Extends ADR 0009.

## Context

The reference tool's Settings ▸ Hotkeys lets the user bind system-wide key
combinations to scan, speedhack, type-change and process actions. The hotkeys
fire while the target application has focus, which is the point: you can drive
a scan without alt-tabbing into the reference tool. Firstspark only had window-scoped
Fyne shortcuts (ADR 0009), which require Firstspark itself to be focused.

Fyne has no global-hotkey API. On X11, `XGrabKey` on the root window provides
system-wide grabs; under native Wayland a regular client cannot grab keys and
would need the `org.freedesktop.portal.GlobalShortcuts` portal.

## Decision

Add a UI-agnostic `pkg/hotkey` that registers system-wide shortcuts on X11
using `XGrabKey` through the already-vendored `jezek/xgb`/`xgbutil`:

- A dedicated X connection runs `xevent.Main` on its own goroutine; events are
  marshalled to the UI with `fyne.Do`.
- Grabs are issued on the root window for the CapsLock/NumLock variants
  (`keybind.GrabChecked`), and `BadAccess` from another client is reported.
- Combos are `modifiers + one key`: `Ctrl`, `Alt`, `Shift`, `Super` plus a
  letter, digit or F-key. A letter or digit requires at least one modifier;
  function keys may stand alone. `Backspace`/`Delete` clears, `Esc` cancels.
- Repeated activations within 300 ms are ignored (X autorepeat).
- When `DISPLAY` is unset the manager is disabled and the UI says so. Grabs are
  released and the connection closed on quit.

Bindings live in the `hotkeys` map of `config.yaml` (action id → combo) and
default to **all unassigned**, matching the reference tool. `speedhack.delta`
(default 0.5) sets the step for the Speedhack speed +/− actions. A dedicated
**Hotkeys** window (Tools ▸ Hotkeys) lists every action with a press-to-capture
field, a Clear button and a per-action status (`bound`, `duplicate`, or the
grab error).

The actions mirror the reference tool's list, restricted to what Firstspark
implements: toggle speedhack, speedhack +/−, change value type to each type,
new scan / new scan exact / new scan unknown, the next-scan filters, undo and
cancel scan, Debug → Run, pause process (`SIGSTOP`/`SIGCONT`, blocked while the
debugger is attached) and attach to the foreground process
(`_NET_ACTIVE_WINDOW` → `_NET_WM_PID`).

## Consequences

- Hotkeys work while the target is focused, as in the reference tool.
- Unlike the reference tool's non-consuming poll, `XGrabKey` is exclusive: the
  focused application does not receive the key. This is documented and is the
  reason a combo can collide with a game's own bindings.
- Global hotkeys are X11-only for now. Under Wayland the feature degrades to
  "unavailable"; a future ADR can add the GlobalShortcuts portal.
- The built-in window shortcuts of ADR 0009 remain and are separate from the
  configurable global hotkeys, exactly as the reference tool separates its menu
  shortcuts from Settings ▸ Hotkeys.
