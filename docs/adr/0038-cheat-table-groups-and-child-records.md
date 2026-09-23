# ADR 0038: Cheat-table groups and child records

## Status

Accepted and implemented. Pulled ahead of the deferred value-changer phases.

## Context

Cheat Engine's address list is a tree: group headers contain child records, and
a child's address is often expressed **relative to its parent** (`+18`,
`+4*$1`). Firstspark's cheat table is a flat `[]tableEntry` rendered by a
`widget.Table`, so it cannot represent a real `.CT` (ADR 0037). Recursive
freeze/set-value also requires the parent/child relationship.

Fyne provides `widget.Tree`, but it renders one label per node and would drop
the Active / Description / Address / Type / Value columns.

## Decision

- Give the cheat-table model a parent/child relationship: an entry may be a
  group header (no address/type) with children; child address expressions are
  resolved against the parent (ADR 0037's expression engine).
- Keep `widget.Table` and render the tree by **projecting the visible nodes into
  a flat row list** with indentation and expand/collapse state (the CE
  approach). Column layout, sorting and cell editing are unchanged.
- Recursive operations (freeze, change value, delete) walk the subtree; a
  changed value applies to children when CE's recursive option is set.
- Extend the `.CT`/JSON schema and `cheattable.Table` to carry the hierarchy.

## Consequences

- Real `.CT` files can be represented and displayed before script execution
  works, with unresolved relative/symbolic addresses shown as such.
- The table gains a visible-row projection layer; selection and the per-row
  value/refresh loops operate on the projection, not `entries` directly.
- The value-changer "recursive set to children" and multi-select work builds on
  this rather than bolting hierarchy onto a flat list later.
