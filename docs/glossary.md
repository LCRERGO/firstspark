# Glossary

Terms used across the codebase, ADRs and UI.

- **Address expression** — a address written as text (`+18`,
  `module+0x10`, `pSelectedCharacter`) and resolved live against the parent
  record, module map and symbol table, rather than stored as an absolute number
  (ADR 0037).
- **AOB** — *array of bytes*: a byte pattern with `??` wildcards used for
  signature scans (`pkg/scan/aob.go`).
- **ASLR bookmark** — a pointer chain whose base is a module plus an offset, so
  it can be re-resolved after a restart even when the module is relocated
  (ADR 0030).
- **Auto-attach** — selecting the first process whose name matches the
  configured pattern while no target is chosen, so a restarted target comes
  back by itself (ADR 0036). It selects the memory target; it does not attach
  the ptrace debugger.
- **Bigger than / Smaller than** — first- and next-scan modes that keep values
  strictly above or below the scan value (ADR 0041).
- **Bitfield** — a value that occupies a range of bits inside a wider container;
  edited with read-modify-write so neighbouring bits are preserved (ADR 0030).
- **Byte cursor** — the selected byte in the Memory Viewer; hex-nibble typing
  writes it and Shift-click extends a selection (ADR 0043).
- **Call stack** — the best-effort frame-pointer unwind shown in the debugger,
  with each return address disassembled (ADR 0044).
- **Reference-tool core Lua API** — the bounded subset of the reference tool's Lua table-object
  model Firstspark implements for scripts (`AddressList`, `MemoryRecord`,
  `Memscan`, `Process`, memory helpers, timers); GUI and OS-integration units
  are non-goals (ADR 0039).
- **Reference-tool custom type conversion** — the assembly `ConvertRoutine` /
  `ConvertBackRoutine` a custom type defines; Firstspark can run
  the integer case through the JIT behind a SysV shim (ADR 0048).
- **.CT table** — the reference tool's XML format: an element-based
  schema with a nested entry tree, per-type metadata and embedded Auto
  Assembler/Lua scripts. Distinct from firstspark's own attribute-based schema
  (ADR 0037).
- **Child record** — an address-list entry that lives under a parent (usually a
  group header) and may express its address relative to it (ADR 0038).
- **Change value** — the one-shot manual edit of a value (`Ctrl+E`). On a
  cheat-table entry it also updates the frozen value and undo value; on a Found
  scan result it writes memory once and updates the row (ADR 0035). It accepts
  `(description)` references and `value`/`oldvalue` expressions. The Memory
  Viewer has its own Change Value form with a type selector.
- **Cheat table** — the list of tracked addresses, values, pointer chains,
  hotkeys and metadata (`pkg/cheattable`).
- **Code cave** — executable memory allocated in the target to hold a hook
  handler or relocated instructions (`pkg/inject`).
- **Conditional breakpoint** — a software breakpoint with an optional
  `REGISTER op value` condition; a false condition resumes automatically
  (ADR 0044).
- **Copy-on-write region** — a private mapping (`p` in `/proc/<pid>/maps`);
  the scan's *Copy on write* filter keeps only these (ADR 0041).
- **Debug thread** — the TID the debugger is bound to; selecting another thread
  rebinds and re-attaches the ptrace session (ADR 0044).
- **Duplicate record** — a deep copy of a cheat-table record (and its subtree)
  appended as a new root; created from the context menu (ADR 0042).
- **Executable filter** — the scan option that keeps only executable regions,
  only non-executable regions, or any (ADR 0041).
- **First value** — the value an address held during the initial scan, kept on
  the result so *Same as first scan* can compare against it (ADR 0041).
- **Follow pointer** — reading a qword at the Memory Viewer cursor and jumping to
  the address it holds when that address is mapped (ADR 0043).
- **Found list** — the scan-results list beside the scan panel, with columns
  Address / Value / Previous. Its Value column is re-read live and its address
  column shows a module-relative `module+0xoffset` for static addresses
  (ADR 0040).
- **Freeze (Active)** — holding a cheat-table entry at its frozen value by
  rewriting it on a 50 ms timer; the Active column shows it.
- **Frozen value** — the value a frozen entry is held at. Set by freezing (from
  a fresh read) and by Change value; the live read never touches it.
- **Group header** — a cheat-table entry that holds no address or value and
  exists only to parent child records in the tree (ADR 0038).
- **Grouped scan** — a value type matching a contiguous sequence of typed
  values, written `4:75 4:* 4:100`, where `*` is a wildcard (ADR 0041).
- **Hardware watchpoint** — a data breakpoint implemented with the x86 debug
  registers DR0–DR3 (`pkg/debugger/hardware.go`).
- **Inline hook** — overwriting a function prologue with a jump into a code cave
  (`pkg/inject`).
- **Instruction trace** — a bounded single-step log of RIP and registers in the
  debugger (ADR 0044).
- **Live value** — the value last read from the target process, shown in the
  cheat table's Value column and the Found list's Value column; every cheat-table
  row is re-read on the 500 ms UI tick, as are the Found results (ADR 0040).
- **Lua Engine** — the console window that evaluates Lua chunks against the
  shared `pkg/celua` runtime; `print` writes to its output log (ADR 0045).
- **Memory Regions browser** — the Memory Viewer window that lists the process
  memory map and jumps to a region (ADR 0043).
- **MI (Machine Interface)** — GDB's machine-readable protocol; the `gdbmi`
  debugger backend drives a gdb child process over it (ADR 0047).
- **Module** — a file-backed region mapped at offset 0; the debugger lists these
  as load bases (ADR 0044).
- **Pointermap** — a reverse index from pointer values to the addresses that
  contain them, used by the pointer scan (`pkg/pointerscan`).
- **Pointer scan** — searching for a chain of pointers, starting at a module or
  static address, that resolves to a target address.
- **PIE** — position-independent executable; its load base moves with ASLR.
- **Previous value** — the value a Found-list address held in the scan before
  the latest one; empty after a first scan (ADR 0040).
- **Record type** — the value type of a cheat-table entry; it can be changed in
  place, re-reading the value at the same address (ADR 0042).
- **Region** — one entry of `/proc/<pid>/maps` (`pkg/mem/region.go`).
- **Same as first scan** — a next-scan mode that keeps addresses whose current
  value still equals their first-scan value (ADR 0041).
- **Scan range** — the optional Start/Stop address bounds that clip the scanned
  regions (ADR 0041).
- **Scan session** — the state of a scan: options, selected regions, results and
  undo history (`pkg/scan/session.go`).
- **Scan tab** — a named, independent scanner workspace in the main window; it
  owns its Found list, scan controls and one *scan session*, while the cheat
  table is shared across tabs. Created and closed browser-style (ADR 0050).
- **Scan-tab compare** — an address-based set operation between two *scan tabs*
  (only in one, only in the other, in both, in exactly one) that produces a new
  scan tab; requires both tabs to share a value type (ADR 0050).
- **Signed/unsigned display** — whether an integer cheat-table entry is
  formatted as signed; toggled per record and persisted in `.CT`/JSON
  (ADR 0042).
- **Speedhack** — scaling a process's perceived time by hooking the libc time
  functions (`pkg/speedhack`).
- **Static address** — an address inside a file-backed module region, shown as
  `module+0xoffset` and coloured green in the Found list; any other address is
  dynamic (ADR 0040).
- **Symbol** — a named address registered by an Auto Assembler/Lua script (for
  example `pSelectedCharacter`); cheat-table address expressions resolve against
  the symbol table (ADR 0037).
- **Track** — observing which instructions write or access an address, via a
  hardware watchpoint (`debugger.Session.Watch`).
- **Undo value** — the value captured just before the last manual Change value;
  `Ctrl+Z` restores it and moves the frozen value when the entry is frozen.
- **Unrandomizer** — hooking libc `rand`/`random`/`rand_r` to return a constant
  so randomised values become predictable (ADR 0045).
- **Watchpoint** — see *Hardware watchpoint*.
