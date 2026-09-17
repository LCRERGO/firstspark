# Glossary

Terms used across the codebase, ADRs and UI.

- **AOB** — *array of bytes*: a byte pattern with `??` wildcards used for
  signature scans (`pkg/scan/aob.go`).
- **ASLR bookmark** — a pointer chain whose base is a module plus an offset, so
  it can be re-resolved after a restart even when the module is relocated
  (ADR 0030).
- **Bitfield** — a value that occupies a range of bits inside a wider container;
  edited with read-modify-write so neighbouring bits are preserved (ADR 0030).
- **Cheat table** — the list of tracked addresses, values, pointer chains,
  hotkeys and metadata (`pkg/cheattable`).
- **Code cave** — executable memory allocated in the target to hold a hook
  handler or relocated instructions (`pkg/inject`).
- **Hardware watchpoint** — a data breakpoint implemented with the x86 debug
  registers DR0–DR3 (`pkg/debugger/hardware.go`).
- **Inline hook** — overwriting a function prologue with a jump into a code cave
  (`pkg/inject`).
- **Pointermap** — a reverse index from pointer values to the addresses that
  contain them, used by the pointer scan (`pkg/pointerscan`).
- **Pointer scan** — searching for a chain of pointers, starting at a module or
  static address, that resolves to a target address.
- **PIE** — position-independent executable; its load base moves with ASLR.
- **Region** — one entry of `/proc/<pid>/maps` (`pkg/mem/region.go`).
- **Scan session** — the state of a scan: options, selected regions, results and
  undo history (`pkg/scan/session.go`).
- **Speedhack** — scaling a process's perceived time by hooking the libc time
  functions (`pkg/speedhack`).
- **Track** — observing which instructions write or access an address, via a
  hardware watchpoint (`debugger.Session.Watch`).
- **Watchpoint** — see *Hardware watchpoint*.
