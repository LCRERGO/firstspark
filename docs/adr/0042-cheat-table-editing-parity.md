# ADR 0042: Cheat-table editing parity

## Status

Accepted.

## Context

The cheat table could add, freeze, edit, bite and group records, but several
address-list operations were missing:

- Integer values were always formatted signed, and the reference tool's `<ShowAsSigned>` was
  parsed then dropped, so a 4-byte `0xFFFFFFFF` showed as `-1` even for a table
  written as unsigned.
- A record's type was fixed once added; there was no *Change record type*.
- Groups existed in the model and imported `.CT` files, but there was no way to
  create one in the UI.
- There was no increase/decrease, no reorder, and no copy/paste or duplicate.

## Decision

- **Signed/unsigned display.** `tableEntry` gains `unsigned` (false = signed,
  the previous behaviour) and `cheattable.Entry` gains `ShowAsSigned`, which is
  now honoured on `.CT` import and emitted on export. Decimal formatting uses
  `FormatUint` for unsigned integer types; hex and binary already use the
  unsigned decode. The cell menu toggles *Show as signed* / *Show as unsigned*.
- **Change record type.** The cell menu gains a *Change record type* submenu
  built from the registered value types. Changing the type re-reads the value at
  the same address and clears an incompatible bitfield. `All` and `Grouped` are
  excluded because they do not describe a single record.
- **Group creation.** *Table ▸ New Group* appends an empty group and opens the
  rename form; *Create Group from Selection* wraps the selected top-level
  records under a new group, preserving order and position.
- **Increase/decrease.** The menu prompts once for a delta (default `1`) and
  applies it to every selected numeric leaf; *Change Value Back* joins the menu
  beside Change Value.
- **Ordering.** Move Up/Down/Top/Bottom reorder a record within its sibling
  list. True drag-and-drop is deferred, because Fyne's table has no drag
  support and would need a custom renderer.
- **Clipboard and duplicate.** Ctrl+C copies the selected cheat-table addresses
  (and the context menu copies address/value), Ctrl+V parses pasted lines
  (`0x1234` or `0x1234 - description`) into new records, and *Duplicate Record*
  deep-copies the selection.

## Consequences

- Address-list editing now covers the reference tool's common record operations without a
  redesign of the table widget.
- `ShowAsSigned` written by firstspark is relative to its own default (signed);
  a reference-tool table without the attribute imports as unsigned, matching the reference tool's default.
- Move and duplicate operate on sibling lists and the entry tree, so they reuse
  `rebuildVisible`; no drag state is introduced.
- Group-from-selection wraps only top-level ancestors of the selection, which
  keeps the tree consistent when a selection spans depths.
