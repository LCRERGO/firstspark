# ADR 0039: Script execution and the Lua boundary

## Status

Accepted. S4a and the core of S4b landed; the reference-tool AA/Lua surface grows
incrementally. Relates to ADR 0037 (`.CT` import) and ADR 0018 (Auto
Assembler).

## Context

.CT tables are scripts as much as data: the CK3 table's records are
resolved by `{$lua}`/`[ENABLE]` scripts that run `aobscanmodule`, `alloc`,
`registersymbol` and hooks, then expose symbols like `pSelectedCharacter` that
the records reference. Importing such a table without running its scripts yields
unresolved addresses.

Running a table's scripts executes arbitrary assembly in the target and
arbitrary Lua in-process. Chosen naively this is both a security problem and an
architecture problem: the reference tool exposes Lua through 73 `Lua*.pas` units, and roughly
half bind VCL GUI widgets (`LuaForm`, `LuaButton`, …), plus `LuaInternet`,
`LuaSQL`, `LuaThread` and `LuaD3DHook`. `AGENTS.md` requires `pkg/...` to stay
UI-agnostic, so GUI Lua bindings cannot live in the engine.

## Decision

- **Boundary: the core engine subset.** Implement the reference-tool Lua
  table-object model —
  `AddressList`/`MemoryRecord`/`Memscan`/`Process`, the read/write helpers
  (`readInteger/Qword/Pointer/mem`, `writeInteger/Qword`),
  `createTimer`/`delayedExecute`/`createthread`, and the calls real pointer
  tables use (`getMemoryRecordByDescription`, `findRecord`, `appendToEntry`,
  `setAddress`, `getAddressSafe`, `enableAutoDisable`, `openProcess`,
  `targetIs64Bit`, `showMessage`). Grow the surface only when a table needs it.
- **Explicit non-goals (for now):** the GUI/VCL widget units, `LuaInternet`,
  `LuaSQL`, the D3D hooks, the structure editor and the manual module loader.
- **Full reference-tool Lua parity is an aspirational north star, not a milestone.**
  GUI bindings would be a separate Fyne↔Lua project in `internal/ui`, and the
  integration units would need a sandbox. Neither is committed.
- **Scripts are imported disabled.** Enabling one is an explicit user action
  behind a warning that it runs code in the target and in Firstspark.
- The Auto Assembler directives firstspark already parses (`alloc`, `label`,
  `registersymbol`, `aobscan`, `[ENABLE]`/`[DISABLE]`) are reused; S4a stores a
  script record's source (`Entry.Script`) and runs it on demand from the cheat
  table, merging `Executor.Symbols()` into the address resolver.
- **S4b (core, done):** `pkg/autoasm` gained `aobscanmodule`, `unregistersymbol`,
  label export, and `{$lua}` segmentation (`{$lua}`/`{$asm}`, phase-aware) with
  an `Executor.SetLuaRunner` hook. `pkg/script` gained Go-backed objects
  (`Foreign`, `KindObject`, `GoFunc`), `CompileWithGlobals`, and `Globals`, so an
  engine can expose an API and share one global scope across chunks. `pkg/celua`
  implements the core table scripting subset against `Table`/`Record`
  interfaces: `AddressList`/`MemoryRecord` (fields and `get*/set*` methods),
  `Active`, memory and process helpers, `AobScan`/`AobScanModule`, `writeBytes`,
  `readmem` as a 0-based ByteTable, `getAddressSafe`, `showMessage`,
  `getCEVersion`, `targetIs64Bit`, `openProcess`, process-name helpers,
  `registerCustomTypeAutoAssembler`, `createMemoryRecord`/`appendToEntry`/
  `setAddress` (scripts can add and edit records), real
  `createTimer`/`delayedExecute` (driven by `RunTimers` on the UI tick), and
  no-op thread/GUI surfaces. `internal/ui` bridges the cheat table to `celua`
  via a session runtime whose globals persist, so the CK3 orchestrator runs.
  The scalar helpers were later widened to byte/word/float/double/string reads
  and writes, `getTickCount`/`sleep`, `writeToClipboard` and a
  `findAddressFromDatabase` stub, and the AA parser accepts `globalalloc` and
  `$`-hex alloc sizes. The remaining reference-tool API grows as tables need it.

## Consequences

- Pointer/stat tables become usable without porting the reference tool's UI surface.
- Importing a `.CT` never executes anything on its own; the worst case is an
  unresolved address, not code execution.
- The exact function list is driven by real tables, so the boundary grows
  empirically instead of being designed up front.
- Scripts share one persistent Lua global scope per session, as in the reference tool;
  `pkg/celua` is covered by integration tests that read, write and AOB-scan the
  test process itself, so the API is exercised against a live process.
- A future "open reference-tool tables with GUI scripts" request is a separate project and
  would need its own ADR.
