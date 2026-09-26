# Firstspark

A Cheat Engine style memory scanner, debugger and code patcher for Linux,
written in Go.

Firstspark attaches to a running process as an unprivileged user, scans its
memory for values, lets you filter and edit the results, browses and patches
machine code, and scales time inside the target process — all without a Python
or GDB runtime dependency.

## Features

| Area | Status |
| --- | --- |
| Process listing and region enumeration (`/proc`) | implemented |
| Cross-process read/write (`process_vm_readv`/`writev`) | implemented |
| Memory scanner: byte/word/dword/qword/float/double/string/AOB/binary/all/grouped | implemented |
| String encodings: UTF-8/16/32 (LE and BE) | implemented |
| Scan modes: exact, bigger-than, smaller-than, unknown initial, changed, unchanged, increased, decreased, increased-by, decreased-by, value-between, same-as-first | implemented |
| Scan filters: writable, executable, copy-on-write, alignment, region scope and start/stop range | implemented |
| Undo scan | implemented |
| User-defined value types (Lua 5.1 scripts, or Auto Assembler via a local JIT) | implemented |
| Cheat table: pointer records, bitfields, hex/binary/signed display, hotkeys, groups, reorder, copy/paste, ASLR-safe chains | implemented |
| Pointer scanner (N-level, cached pointermap, `.ptr` load/save) | implemented |
| Hex viewer/editor with page scrolling, region browser, byte editing and dump | implemented |
| Debugger backend interface with pure-Go ptrace implementation | implemented |
| Breakpoints, single-step, registers, threads, modules, conditions, call stack, trace | implemented |
| Debugger GUI, hardware watchpoints, find-accesses/writes | implemented |
| Structure dissect (compare instances, guess fields, follow pointers) | implemented |
| Auto Assembler (`alloc`, labels, `db`/`dd`, `aobscan`, enable/disable) | implemented |
| GDB/MI backend | stub (same interface) |
| Disassembler (pure Go, `x86asm`) | implemented |
| Assembler (pure-Go Intel syntax, common subset) | implemented |
| Inline trampoline hooking + remote `mmap`/`mprotect` | implemented |
| Speedhack (`clock_gettime`, `gettimeofday`), wired to the UI | implemented (experimental) |
| Remote function calls (int/float/double arguments) | implemented |
| Internationalization (go-i18n catalogs, `ui.language`) | implemented |
| PINCE-style auto-attach to a process by name | implemented |
| Fyne desktop GUI | implemented (build tag `gui`) |
| Firstspark `.CT`/JSON sessions import/export | implemented |
| Cheat Engine `.CT` import/export | partial (tree, expressions, scripts, core Lua, CE export; ADR 0037) |

## Requirements

- Linux on x86-64
- Go 1.27 or newer
- To attach to other processes, the kernel must permit it: `ptrace_scope=0`,
  `CAP_SYS_PTRACE`, or a non-sandboxed environment. Same-uid attach is enough
  when `kernel.yama.ptrace_scope` is 0.
- For the GUI: CGO plus OpenGL and X11/Wayland development headers.

```sh
sudo apt install libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev
```

## Build

```sh
make                # runnable GUI -> bin/firstspark (requires the headers above)
make build/gui      # same as `make`
make build/headless # headless binary, no CGO or graphics libraries
make run            # build the GUI and run it
make test           # unit + integration tests
```

The default build produces the Fyne GUI; the headless CLI is available via
`make build/headless` (or the `gui` build tag is simply omitted).

## Usage

### GUI

```sh
bin/firstspark
```

The window follows the Cheat Engine layout: a scan panel and **Found** list at
the top, a **cheat table** below a splitter, and a separate **Memory Viewer**
window. Open a process (`Ctrl+P`), choose a scan type and value type, enter a
value and press **First Scan** (`Enter`). Refine with **Next Scan**, then
double-click a found address to add it to the cheat table, where you can freeze
or edit it. The Memory Viewer (`Ctrl+M`) shows the disassembly and hex dump for
the selected address. The theme (light, dark or system) is set in
**Edit → Settings**.

### Headless

```sh
bin/firstspark --list                       # list processes
bin/firstspark --pid 1234 --type dword --mode exact --value 1000
bin/firstspark --pid 1234 --type dword --mode unknown --next increased
bin/firstspark --pid 1234 --type dword --mode exact --value 42 --export run.CT
```

### Configuration

Settings are read from `$XDG_CONFIG_HOME/firstspark/config.yaml`; see
[`configs/config.yaml`](configs/config.yaml) for the defaults. Saved scan
sessions are written to `$XDG_DATA_HOME/firstspark/`. Set
`process.auto_attach` (with `process.auto_attach_regex` for a regular
expression) to re-select a matching target automatically while none is chosen,
so a relaunched process is picked up again; it can also be edited in
**Edit → Settings**.

User-defined value types live in `$XDG_CONFIG_HOME/firstspark/customtypes.yaml`
and are edited from **Table → Custom Types** (or the "…" button beside the
Value Type dropdown). See [`docs/custom-types.md`](docs/custom-types.md) for the
script syntax.

## Architecture

```
cmd/firstspark        entrypoint
internal/app          CLI and wiring
internal/ui           Fyne desktop UI (build tag gui)
pkg/mem               /proc introspection + process_vm_readv/writev
pkg/scan              scan engine (types, modes, snapshots)
pkg/asm               x86-64 disassembler + pure-Go Intel assembler
pkg/debugger          Backend interface, ptrace, gdbmi stub
pkg/inject            inline trampoline hooking
pkg/speedhack         time-scaling hooks
pkg/config            YAML configuration
pkg/cheattable        .CT / JSON import and export
```

See [`docs/architecture.md`](docs/architecture.md) for details.

## Notes and limitations

- **vDSO**: `clock_gettime`/`gettimeofday` may be served by the vDSO; the
  speedhack hooks the libc symbols, so direct vDSO calls bypass it.
- **Assembler**: the pure-Go encoder covers the instruction subset needed for
  patching and trampolines. Unsupported mnemonics return an error; the raw byte
  path is always available.
- **Relocation**: hooking refuses to overwrite a prologue containing
  RIP-relative operands or relative branches.
- **ptrace**: attaching requires a permissive environment; sandboxes that block
  `ptrace(2)` will return `EPERM`.

## License

MIT — see [LICENSE](LICENSE).
