# ADR 0031: Expression scan values and hover hints

## Status

Accepted.

## Context

Two usability gaps existed in the scan panel:

- The scan value box accepted only a literal (decimal, `0x` hex, or a float).
  Entering a computed value such as `360 * (10 ^ 6)` failed, even though the
  in-house scripting language (`pkg/script`, ADR 0012) already evaluates
  arithmetic with precedence, parentheses and `math.*` builtins.
- The Scan Value and Compare controls gave no explanation of what they accept,
  and "Array of Bytes" was never described in the UI, so the wildcard pattern
  syntax was undiscoverable. Fyne v2.8.1 has no built-in tooltip widget.

## Decision

**Expressions in numeric scan values.** `pkg/scan` evaluates a value that does
not parse as a literal as a single Lua expression, reusing `pkg/script`. The
expression is wrapped as `function __firstspark_value() return (<input>) end`
so only an expression is available, not a statement chunk. This applies to the
built-in numeric types (`Byte`, `2/4/8 Bytes`, `Float`, `Double`, `All`) and to
both value boxes (the lower and upper bounds of "Value between"). Text, AOB,
binary, UTF and user-defined types keep their existing parsers. Integer types
require an exactly integral result; float types accept any number. The scripting
language stays the Lua 5.1.4 subset of ADR 0012 (no `**`, no bitwise operators);
power is `^`. When the Hex box is ticked it only rewrites a bare hex literal, so
`0xFF + 1` is evaluated as an expression rather than corrupted into `0x0xFF + 1`.

**Hover hints.** The GUI uses `github.com/dweymouth/fyne-tooltip` v0.4.0 (the
only maintained Fyne tooltip library; BSD-3-Clause, pure Go). Every window is
wrapped with `fynetooltip.AddWindowToolTipLayer` (the main window plus the
process list, memory viewer, debugger, dissect, custom type and auto-assemble
windows), because the library needs one layer per window. Fyne's buttons,
checks and selects are replaced with the library's `ttwidget.*` drop-ins, a
small `toolTipEntry` wrapper adds hover support to entries (the library has no
entry drop-in), and custom widgets (`tapLabel`, `treeToggle`) embed
`ttwidget.ToolTipWidgetExtend`. Helper constructors (`newHintEntry`,
`newHintButton`, `newHintCheck`, `newHintSelect`) keep the call sites short.
Hints are translated and live in `en.json`. The Scan Value hint is dynamic: it
describes the syntax of the currently selected value type (notably the AOB
wildcard pattern), while the Compare hint lists the accepted operators.

Coverage extends across the app: the scan options, process list (filter, tree,
sort headers, disclosure triangles), memory viewer, debugger (execution,
watchpoints, register editing), dissect, custom type manager, auto-assemble
window, code editor toolbar, and the main results/cheat-table controls.

Two surfaces remain unhinted because the library cannot reach them:

- **Dialogs** (`dialog.NewCustom*`): the tooltip layer is attached to the
  underlying `*widget.PopUp`, which Fyne does not expose, so Settings, Pointer
  Scan, Regions, Add Address, bitfield, hotkey and the call-function dialogs
  have no hints. Converting them to windows would enable hints.
- **Icon-only toolbar actions** (`widget.ToolbarAction`): the toolbar renders a
  private, non-extensible button. Hints would require rebuilding the toolbar
  from `ttwidget.Button`s.

**AOB compact patterns.** `ParseAOB` now splits an unseparated run of hex digits
and wildcards, so `488B??E5` parses as its documentation always claimed.

## Consequences

- `pkg/scan` depends on `pkg/script` (and transitively `pkg/combinator`); both
  are pure Go and remain in the headless build.
- Expression evaluation compiles a small Lua chunk per parse; this happens once
  per scan, not per byte, so the cost is negligible.
- Dialogs are not wrapped with a tooltip layer because the library cannot reach
  them; a dialog opened while a hint is visible may log a benign missing-layer
  error from the library.
- New controls should attach hints through `newHint*` helpers, `setHint`, or the
  dynamic value hint (`updateValueHint`). New windows must be wrapped with
  `AddWindowToolTipLayer` or their hints will not appear.
