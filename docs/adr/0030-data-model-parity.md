# ADR 0030: Cheat-table schema, bitfields, string encodings and scan regions

## Status

Accepted.

## Context

The cheat table serialized only ID, description, address, type and value, so
pointer chains, hotkeys, display format and frozen state were lost on save. The
value-type set stopped at UTF-8 strings, there was no bitfield type, and a scan
could only use one of three fixed region scopes.

## Decision

- **Cheat-table schema**: `cheattable.Entry` gains `Hotkey`, `Display`,
  `Frozen`, `Encoding`, `Pointer`, and the bitfield fields `BitSize`,
  `BitOffset`, `BitWidth`, `BitSigned`; `Table` gains a `Version` attribute
  (currently `2`). Old files load unchanged because the new attributes are
  optional. `FormatPointerChain`/`ParsePointerChain` encode a chain as
  `module+0xoffset:+0x10,-0x8`. Pointer entries with a module are re-resolved
  against the current process's region map, so chains survive ASLR/PIE; entries
  without a module keep their absolute address.
- **Bitfield**: a table-only edit/display type configured by bit offset, width
  (1..64) and signedness. Reads and writes are read-modify-write so neighbouring
  bits are preserved. It is not a scannable type.
- **String encodings**: `utf16le`, `utf16be`, `utf32le` and `utf32be` are
  registered as separate variable-width string types, encoded on parse and
  decoded for display.
- **Scan regions**: `scan.Options.Regions` can carry an explicit region list.
  The new *Scan Regions* dialog lets the user include or exclude individual
  regions; the selection is per session (cleared on process change) and forces
  the scope to *All readable*.

## Consequences

- Cheat tables round-trip GUI-only state and remain backward compatible.
- ASLR-safe pointer entries depend on the module name; a renamed or unloaded
  module falls back to the stored absolute address only when no module is set.
- Bitfield freezing writes the whole captured container, matching the value at
  freeze time.
- Region selection is not persisted; a future ADR can move it into the session.
