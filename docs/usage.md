# Usage

## Building

```sh
make build/headless  # headless (no CGO)
make build/gui       # Fyne GUI (needs OpenGL/X11 dev headers)
make                 # same as make build/gui
```

## Finding a value

1. Start a target, e.g. the bundled fixture:

   ```sh
   make fixtures
   ./test/target
   ```

2. In the GUI, select the process, set **Type** to `dword`, **Mode** to
   `exact`, **Value** to `1000`, and press **First Scan**.

3. Change the value in the target and press **Next Scan** with mode
   `changed` (or `increased`) to narrow the results.

4. Select a result to view its bytes and disassembly, press **Freeze** to hold
   the value, and **Export** to save the session.

## Headless

```sh
# List processes
firstspark --list

# Exact scan for a dword
firstspark --pid "$(pgrep -n target)" --type dword --mode exact --value 1000

# Unknown-value scan followed by an "increased" filter
firstspark --pid 1234 --type dword --mode unknown --next increased

# Export the results as a Cheat Engine table
firstspark --pid 1234 --type dword --mode exact --value 42 --export run.CT
```

## Value types

`byte`, `word`, `dword`, `qword`, `float`, `double`, `string`, `aob`,
`binary`, plus the `utf16le`, `utf16be`, `utf32le` and `utf32be` string
encodings. Custom types loaded from `customtypes.yaml` also appear here.

AOB patterns accept spaces or compact hex and `??` wildcards, e.g.
`48 8B ?? E5` or `488B??E5`.

Numeric scan values may be a single Lua expression (the `pkg/script` subset),
e.g. `360 * (10 ^ 6)` or `math.floor(10 / 3)`. Integer types require an
integral result; power is `^`. Most controls in the GUI show a hint on hover,
including the scan value/compare/type fields (with the AOB wildcard syntax),
the process list, memory viewer, debugger, dissect, custom type and
auto-assemble windows.

## Scan modes

`exact`, `unknown`, `changed`, `unchanged`, `increased`, `decreased`,
`increased by`, `decreased by`, `between`.

`exact`, `increased by`, `decreased by` and `between` take a value; the others
do not.

## Configuration

`$XDG_CONFIG_HOME/firstspark/config.yaml`:

```yaml
scan:
  value_type: dword
  writable_only: true
  alignment: 4
  snapshot_limit: 2147483648
  float_epsilon: 0.000001
debugger:
  backend: ptrace
  gdb_path: gdb
speedhack:
  enabled: false
  scale: 1.0
  delta: 0.5
ui:
  result_limit: 1000
  theme: dark
  language: en
log:
  level: info
  file: ""
hotkeys:
  speedhack.toggle: Ctrl+Alt+K
```

`ui.language` selects an embedded catalog from `internal/i18n/locales`; the
change applies on the next launch. `ui.theme` accepts `light`, `dark` or
`system`.

## Global hotkeys

Tools ▸ Hotkeys binds system-wide shortcuts (X11 only) that fire while the
target has focus. A combo is modifiers plus one key; a letter or digit needs a
modifier (`Ctrl+Alt+K`), while function keys may stand alone (`F8`). Bindings
are stored under `hotkeys:` in the config file and default to unassigned. The
same window reports conflicts, for example when another program already owns a
combo. Under native Wayland global hotkeys are unavailable and the window says
so.

## Logging

Diagnostics go to `$XDG_STATE_HOME/firstspark/firstspark.log`
(`~/.local/state/firstspark/firstspark.log` by default) and to stderr. Set
`log.level` to `debug`, `info`, `warn` or `error`, or pass `-log-level` on the
command line. One previous log file is kept. The log records the target
process lifecycle, scans, speedhack and debugger actions, and any process that
exits while selected.

## Troubleshooting

- **`permission denied (check ptrace_scope / CAP_SYS_PTRACE)`** — the kernel is
  restricting `ptrace`. Set `sudo sysctl kernel.yama.ptrace_scope=0` or grant
  `CAP_SYS_PTRACE`, and avoid sandboxes that block `ptrace(2)`.
- **`GUI support not built in`** — rebuild with `make gui` after installing the
  development headers listed in the README.
