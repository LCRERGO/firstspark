# ADR 0052: Sandboxed-Lua plugins with declared capabilities

## Status

Accepted.

## Context

Users need to extend Firstspark. The obvious route on Linux is Go's `plugin`
package, but it requires CGO, locks every plugin to the host's exact Go version,
dependency versions and build tags, and is unavailable in the `CGO_ENABLED=0`
headless build that the project treats as a hard constraint. Native plugins
would also mean arbitrary code execution inside the debugger. Firstspark
already has a sandboxed Lua subset (`pkg/script`, ADR 0012), a reference-tool
Lua runtime (`pkg/celua`), and a runtime value-type registry
(`scan.RegisterType`).

## Decision

A plugin is a sandboxed Lua bundle in `$XDG_DATA_HOME/firstspark/plugins/<id>/`
with a `plugin.yaml` manifest and an entry file. The manifest declares `id`,
`name`, `version`, an integer plugin-API version, the entry file, requested
capabilities and optional declarative menu/hotkey contributions. A new
`pkg/plugin` package owns the host, a versioned `firstspark.*` API and the
capability registry; `pkg/celua` stays the console/table runtime and is not
overloaded. Plugins load headless as well as under the GUI.

Capabilities are all-or-nothing per plugin: enabling a plugin grants the set its
manifest requests, and an update that grows the set re-prompts. The v1 set is
`memory_read`, `memory_write`, `hooking`, `scan` and `ui`. Engine extension
points are the value-type registry, a hook-symbol registry and the UI
action/menu registry; hooks install through the safe manager of ADR 0051 rather
than being written by the plugin. UI contributions are declarative (menu items,
hotkeys) so plugin code never runs Fyne; a plugin requiring `ui` is skipped in
the headless build.

## Consequences

- Plugins never break the headless/CGO-free build and can never import Go.
- The plugin ABI is a Lua API version, not a Go symbol ABI: portable, but capped
  at what the host exposes.
- A malicious plugin is bounded by its granted capabilities, but a granted
  `memory_write` or `hooking` plugin is still powerful; enabling is explicit and
  visible.
- A runaway callback cannot crash the host but can stall the UI thread, so
  consecutive `on_tick` failures auto-disable the plugin.
