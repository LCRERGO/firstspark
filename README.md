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
| GDB/MI backend (attach, memory, registers, breakpoints, step; no remote calls/watchpoints) | implemented |
| Disassembler (pure Go, `x86asm`) | implemented |
| Assembler (pure-Go Intel syntax, common subset) | implemented |
| Inline trampoline hooking + remote `mmap`/`mprotect` | implemented |
| Speedhack (`clock_gettime`, `gettimeofday`), wired to the UI | implemented (experimental) |
| Unrandomizer (constant `rand`/`random`/`rand_r` hooks) | implemented |
| Lua Engine console (`pkg/celua`) | implemented |
| Remote function calls (int/float/double arguments) | implemented |
| Internationalization (go-i18n catalogs, `ui.language`) | implemented |
| PINCE-style auto-attach to a process by name | implemented |
| Fyne desktop GUI | implemented (build tag `gui`) |
| Firstspark `.CT`/JSON/YAML sessions import/export | implemented |
| Cheat Engine `.CT` import/export | partial (tree, expressions, scripts, colour, last state, hotkeys, core Lua; ADR 0037/0046) |

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
make install        # install the GUI binary + manpage (PREFIX=/usr/local)
make install/headless # install the no-CGO binary instead
make test           # unit + integration tests
make test/race      # the same under the race detector
make lint           # gofmt + go vet (default and gui tags)
make fuzz           # fuzz every FuzzXxx target for FUZZTIME (default 15s)
```

The default build produces the Fyne GUI; the headless CLI is available via
`make build/headless` (or the `gui` build tag is simply omitted).

`make install` honours `PREFIX` (default `/usr/local`) and `DESTDIR` for
staged/packaged installs, e.g. `make install PREFIX=$HOME/.local` or
`sudo make install`. It installs the binary to `$PREFIX/bin` and
`firstspark.1` (see [`docs/firstspark.1`](docs/firstspark.1)) to
`$PREFIX/share/man/man1`, so `man firstspark` documents the CLI.

## Usage

### GUI

```sh
bin/firstspark
```

The window follows the Cheat Engine layout: a scan panel and **Found** list at
the top, a **cheat table** below a splitter, and separate **Memory Viewer** and
**Debugger** windows. Open a process (`Ctrl+P`).

- **Scan panel**: phase-aware scan types (exact, bigger/smaller, value-between
  and unknown first; then increased/decreased (+by), changed, unchanged and
  same-as-first) with region filters (writable, executable, copy-on-write,
  scope, region picker and start/stop range), grouped scans, the speedhack and
  the unrandomizer.
- **Found list**: Address / Value / Previous columns, live values, sortable
  headers and a cell menu (change value, add to table, browse, disassemble,
  find writes/accesses, copy, display format). Double-click adds a result to
  the cheat table.
- **Cheat table**: pointer records, bitfields, signed/hex/binary display,
  per-entry colour, hotkeys, groups, move/duplicate, clipboard, freeze-on-tick
  and edit-on-double-click.
- **Memory Viewer** (`Ctrl+M`): continuously scrollable hex with inline byte
  editing, a region browser, disassembly, pointer follow, breakpoints and
  `.bin` dump.
- **Debugger**: register, thread, module, breakpoint, hit, call-stack and trace
  tabs, with conditions.
- **Tools ▸ Lua Engine**: a `print`-capable console; **Edit ▸ Settings** covers
  theme, scale, scan limits, refresh interval, logging and the debugger backend.

### Headless

```sh
bin/firstspark --list                       # list processes
bin/firstspark --pid 1234 --type dword --mode exact --value 1000
bin/firstspark --pid 1234 --type dword --mode unknown --next increased
bin/firstspark --pid 1234 --type dword --mode between --value 0 --value2 100
bin/firstspark --pid 1234 --type grouped --value "4:75 4:* 4:100"
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
internal/i18n         message catalogs and helpers
pkg/mem               /proc introspection + process_vm_readv/writev
pkg/scan              scan engine (types, modes, session, snapshots, grouped)
pkg/asm               x86-64 disassembler + pure-Go Intel assembler
pkg/debugger          Backend interface, ptrace, gdbmi stub, session, remote call
pkg/inject            inline trampoline hooking + remote mmap/mprotect
pkg/speedhack         time-scaling hooks
pkg/unrandomizer      constant rand/random/rand_r hooks
pkg/pointerscan       N-level pointer scanner and pointermap
pkg/dissect           structure dissection
pkg/autoasm           Auto Assembler subset
pkg/script            Lua 5.1.4-compatible subset
pkg/celua             Cheat Engine Lua table-object runtime
pkg/customtype        user-defined value types
pkg/cheattable        .CT / JSON import and export
pkg/config            YAML configuration
pkg/combinator        parser combinators shared by the scripting stack
```

See [`docs/architecture.md`](docs/architecture.md) for details.

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — layering and scan model.
- [`docs/usage.md`](docs/usage.md) — GUI and CLI walkthrough.
- [`docs/custom-types.md`](docs/custom-types.md) — user-defined value types.
- [`docs/glossary.md`](docs/glossary.md) — the project vocabulary.
- [`docs/adr/`](docs/adr/) — architecture decision records.

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
- **Grouped scans**: segments match contiguously at the scan alignment; there
  are no per-segment offsets or gaps yet.
- **Debugger threads**: the ptrace backend traces one thread at a time;
  selecting a thread rebinds the session, and clone/fork following is deferred
  (ADR 0044).
- **Call stack**: a best-effort frame-pointer walk; builds compiled without a
  frame pointer may truncate it.
- **Unrandomizer**: returns a constant only (no counter/pattern mode).
- **RNG hooks** (`speedhack`, `unrandomizer`) and Auto Assembler use the ptrace
  backend directly; they do not use `debugger.backend`.
- **GDB/MI backend**: needs `gdb` on `PATH`; it supports attach, memory,
  registers, breakpoints and stepping, but not remote calls or hardware
  watchpoints (ADR 0047).
- **Cheat Engine tables**: import/export covers the tree, expressions, scripts,
  colour, last state, hotkeys, the core Lua API and preserved unknown elements
  (ADR 0037, ADR 0046); GUI/VCL-script tables are out of scope (ADR 0039).
  Integer, float and string custom-type `ConvertRoutine` conversions are applied
  on both builds — natively via `pkg/jit` under CGO, or interpreted by
  `pkg/aaexec` headless; routines outside that subset or that depend on the
  address argument fall back to raw (ADR 0048).

## License

MIT — see [LICENSE](LICENSE).
