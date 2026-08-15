# ADR 0006: Adopt Fyne for the desktop GUI

## Status

Accepted. Supersedes the Gio choice in ADR 0001.

## Context

The original GUI was built on Gio (`gioui.org`), an immediate-mode toolkit.
Recreating a Cheat Engine-style interface — a native menu bar, a data-grid
table with headers, resizable split panes, modal dialogs and a system theme
preference — required hand-rolling each of those widgets. Gio also renders
through Vulkan, which is an unusual runtime dependency for a desktop utility.

Fyne is a retained-mode toolkit with the needed widgets built in
(`MainMenu`, `Toolbar`, `Table`, `HSplit`/`VSplit`, `dialog`), a built-in
light/dark theme that follows the FreeDesktop dark-style preference, and
bundled fonts (Noto Sans, DejaVu Sans Mono). It renders through OpenGL.

## Decision

Replace Gio with Fyne v2 for `internal/ui`. Keep the `gui` build tag: the Fyne
files are tagged `gui`, while `internal/ui/ui_stub.go` is tagged `!gui` and
returns `ErrNotBuilt`, so a plain `go build ./...` stays headless. The
Makefile's default target builds the runnable GUI (`make` == `make build/gui`);
`make build/headless` produces the CLI-only binary. The engine (`pkg/...`)
remains UI-agnostic and is unaffected.

## Consequences

- The GUI toolchain requirement changes from Vulkan headers to CGO plus
  OpenGL/X11/Wayland development headers.
- `internal/ui` is split into several files because the Fyne front-end is
  substantially larger than the single Gio file.
- Fyne is a much larger dependency than Gio and requires CGO, so the GUI
  binary cannot be cross-compiled without a C toolchain.
- The headless CLI build and all engine tests keep working without any
  graphics libraries installed.
