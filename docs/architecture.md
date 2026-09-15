# Architecture

Firstspark is split into a UI-agnostic engine (`pkg/...`) and thin front-ends
(`internal/...`). Every capability is reachable from Go without the GUI.

## Layers

```
                 internal/ui (Fyne)     internal/app (CLI)
                        \                    /
                         \                  /
   pkg/scan   pkg/debugger   pkg/inject   pkg/speedhack   pkg/cheattable
        \          |             |             /
         \         |             |            /
                pkg/mem  +  pkg/asm  +  pkg/config
```

- `pkg/mem` — process discovery via `/proc`, memory map parsing, and
  cross-process reads/writes through `process_vm_readv(2)`/`process_vm_writev(2)`.
- `pkg/scan` — the scan engine. `Session.First` performs an initial exact or
  unknown-value scan; `Session.Next` filters the result set using change-based
  predicates. Fixed-width types are compared numerically; strings and AOB
  patterns are matched byte-wise (AOB supports `??` wildcards).
- `pkg/asm` — `Disassemble` wraps `golang.org/x/arch/x86/x86asm`;
  `Assemble`/`AssembleProgram` are a pure-Go Intel-syntax encoder with label
  support, covering the instruction subset used for patching.
- `pkg/debugger` — the `Backend` interface (`Attach`, `Read`/`Write`,
  `Registers`, breakpoints, `Step`/`Continue`/`Wait`) with a pure-Go ptrace
  implementation and a `gdbmi` stub behind the same seam. The ptrace backend
  also exposes `RemoteSyscall`, `Mmap` and `Mprotect`.
- `pkg/inject` — inline trampoline hooking. It plans how many prologue bytes
  must be overwritten, maps a code cave with a remote `mmap`, writes the
  handler plus a jump back, and builds a trampoline that runs the relocated
  original instructions.
- `pkg/speedhack` — resolves libc symbols from the target's ELF files, builds a
  scaling handler with the assembler, and installs it through `pkg/inject`.
- `pkg/config` — YAML configuration under the XDG directories.
- `pkg/cheattable` — Cheat Engine `.CT` (XML) and JSON session import/export.
- `pkg/dissect` — compares a region across several instances and guesses a
  field layout, resolving pointer fields (ADR 0017).
- `pkg/pointerscan` — an N-level pointer scanner over a pointermap (a reverse
  index of pointer values to their addresses, with module-backed addresses
  tagged static). Chains can be saved as `.ptr` files (ADR 0015).
- `pkg/combinator` — hand-rolled parser combinators, shared by `pkg/script`
  and (later) the Auto Assembler.
- `pkg/script` — the Lua 5.1.4-compatible subset used for user-defined value
  types. It parses with `pkg/combinator` and compiles to Go closures so a
  conversion runs without an interpreter loop. Sandboxed: no `io`/`os`/`debug`,
  no `require`, metatables, coroutines or `pcall` (ADR 0012).

## GUI

`internal/ui` is a Fyne front-end, gated behind the `gui` build tag so the
default build stays headless and free of CGO. It mirrors Cheat Engine's window
layout: a separate Process List window, a scan panel and Found list above a
splitter, a cheat table below it, and a separate Memory Viewer with the
disassembler over the hex dump. See ADR 0007.

The package is split into `app.go` (state, window, menus, toolbar, shortcuts),
`theme.go` (the flat cyberpunk theme and embedded display font), `processes.go`,
`scan.go`, `results.go`, `memory.go`, `settings.go`, `files.go` and `format.go`.
Theming and window structure are recorded in ADRs 0006-0010.

## Scanning model

An exact first scan reads each selected region in 4 MiB chunks with an overlap
equal to the value width, so values spanning a chunk boundary are still found.
Unknown-value scans snapshot every aligned address in the writable regions,
bounded by `scan.snapshot_limit`. Subsequent scans re-read the live values and
apply the mode predicate, discarding addresses that no longer match.

## Privilege model

Firstspark runs entirely as the invoking user. Attaching requires the kernel to
allow it (`ptrace_scope=0` or `CAP_SYS_PTRACE`). When permission is denied the
engine returns a diagnostic error and the CLI can re-exec under `sudo`.

## Extensibility

- A new debugger backend only needs to satisfy `debugger.Backend`.
- A pure-Go disassembler/assembler backend can replace `pkg/asm` internals
  without touching callers.
- The engine packages have no dependency on the UI.
