# ADR 0043: Memory Viewer navigation, editing and dumping

## Status

Accepted.

## Context

The Memory Viewer read a single fixed 256-byte page and rendered it as two
static, non-selectable lists. Find was confined to that window; there was no
scrolling beyond it, no region browser, no byte editing, no way to follow a
pointer, no debugger actions, and no way to dump memory. ADR 0007 only fixed the
window layout.

## Decision

- **Virtualized hex pane.** The hex pane is a `widget.List` whose rows map to
  addresses across the region containing the cursor (a synthetic 64 KiB window is
  used for unmapped addresses). Bytes are read on demand through a page cache
  (`readCached`), so the pane scrolls continuously without pre-reading memory.
  Find searches the whole region in chunks.
- **Disassembly chunk.** The disassembly pane decodes a `memDisasmChunk` window
  anchored just before the cursor and scrolls to the instruction containing it.
- **Region browser.** A **Memory Regions** window lists `/proc/<pid>/maps` with a
  filter, and a toolbar dropdown jumps to a region's base.
- **Byte cursor and editing.** The hex pane tracks a byte cursor; clicking
  selects a byte, Shift-click extends a range, arrow keys and PageUp/PageDown
  move it, and typing two hex digits writes the byte at the cursor immediately.
  The cursor/selection span is coloured by splitting each row into three
  monospace texts laid out edge to edge (`noGapLayout`). Ctrl+C copies the
  selected bytes as spaced hex.
- **Debugger actions and pointer follow.** A context menu on the hex and
  disassembly panes offers Follow Pointer (read a qword and jump when it maps),
  Copy Address, Toggle Breakpoint and find-what-writes/accesses, delegating to
  the existing debugger.
- **Dumping.** File ▸ Save Memory View and Save Selection write raw bytes to a
  `.bin` file.
- The hex pane is a `*memHexList` (a `widget.List` with a byte-cursor `TypedKey`
  and nibble `TypedRune`) so bare keys reach it only while it is focused, leaving
  the address field's typing intact.

## Consequences

- The viewer behaves like Cheat Engine's: continuous scrolling, a region browser,
  inline byte edits, pointer following and a dump.
- `readCached` reads whole 4 KiB pages and caps the cache at 256 pages; the
  per-row reads during scrolling hit the cache after the first page.
- The region list is cached for the dropdown and the browser; it refreshes when
  the viewer is opened and after a Go-to.
- The disassembly pane decodes a bounded chunk rather than the whole region;
  scrolling past it requires moving the cursor (PageUp/PageDown), which is the
  deliberate trade-off for a pure-Go disassembler without a seekable instruction
  index.
