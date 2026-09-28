# ADR 0001: Engine-first architecture

## Status

Accepted.

## Context

Firstspark targets the same feature surface as the reference tool: scanning, a
debugger, an assembler, code injection and time scaling. We want to publish it
as open source and allow a GUI to evolve without rewriting the engine.

## Decision

Keep all capability in UI-agnostic `pkg/...` packages. `internal/ui` (Gio) and
`internal/app` (CLI) are thin front-ends over the same engine. The engine never
imports the UI.

## Consequences

- The CLI can drive every feature, which makes integration testing possible
  without a display.
- The GUI is optional and build-tagged, so the project compiles on machines
  without the GUI toolchain dependencies.
