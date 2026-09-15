# AGENTS.md

Guidance for agents working in this repository.

## What this is

Firstspark is a Cheat Engine style memory scanner, debugger and code patcher
for Linux x86-64, written in Go. The engine lives in `pkg/...` and is
UI-agnostic; front-ends live in `internal/...`.

## Build, test, lint

```sh
make                # runnable GUI -> bin/firstspark (CGO + OpenGL/X11 headers)
make build/gui      # same as `make`
make build/headless # headless binary (no CGO, no GUI)
make run            # build the GUI and run it
make test           # unit + integration tests
make lint           # gofmt + go vet (default and gui tags)
make tidy           # go mod tidy
```

GUI prerequisites (Debian/Ubuntu):

```sh
sudo apt install libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev
```

Direct verification commands:

```sh
go build ./...
go build -tags gui ./...
go vet -tags gui ./...
go test ./...
```

Always run the `-tags gui` build and vet when touching `internal/ui`; the
default build ignores those files, so plain `go build ./...` will not catch
GUI compile errors. `gofmt -l internal pkg cmd` must be empty.

## Layout

- `cmd/firstspark` — entrypoint.
- `internal/app` — CLI flags and wiring.
- `internal/ui` — Fyne GUI, gated behind `//go:build gui` (the `!gui` stub is
  `ui_stub.go`). Files: `app.go` (state, window, menus, shortcuts), `theme.go`
  (colours/fonts), `processes.go`, `scan.go`, `results.go`, `memory.go`,
  `settings.go`, `files.go`, `format.go`, `icons.go`, `x11.go`,
  `customtypes.go`, `editor.go`, `pointerscan.go`, `debugger.go`,
  `dissect.go`, `autoasm.go`.
- `pkg/...` — the engine: `mem`, `scan`, `asm`, `debugger`, `inject`,
  `speedhack`, `cheattable`, `config`, `combinator`, `script`, `customtype`,
  `pointerscan`, `dissect`, `autoasm`. Never import `internal/ui` from here.
- `pkg/combinator` — dependency-free parser combinators.
- `pkg/script` — the in-house Lua 5.1.4-compatible subset used by custom value
  types; parses with `pkg/combinator` and compiles to Go closures. Pure Go, so
  it stays in the headless build (ADR 0012).
- `pkg/customtype` — loads user-defined value types from `customtypes.yaml` and
  registers them with `pkg/scan` (ADR 0013).

## Conventions

- The engine must stay UI-agnostic; the GUI is optional and build-tagged.
- Do not add comments unless they explain non-obvious intent.
- Match the surrounding style; run `gofmt`.
- Configuration is YAML under the XDG directories (`pkg/config`).
- The theme maps every `theme.ColorName*` explicitly and falls back to Fyne's
  default theme for unknown names; never return a nil/transparent colour.

## Design decisions

Architecture decisions are recorded as ADRs in `docs/adr/`. Read the relevant
ADR before changing the GUI layout, theme, shortcuts, or icon handling, and add
a new ADR for any decision that changes them. `docs/architecture.md` gives the
high-level picture.

## Notes

- `ui.theme` is `light`, `dark` or `system` (default `light`); it is switchable
  from View ▸ Theme and Edit ▸ Settings.
- `ui.scale` is applied through `FYNE_SCALE` at startup only (Fyne has no
  runtime scale setter), so a scale change takes effect on the next launch.
- Process icons are resolved asynchronously and cached; see ADR 0011.
- Custom value types are documented in `docs/custom-types.md`; edit them from
  the manager (the "…" button beside the Value Type dropdown).
