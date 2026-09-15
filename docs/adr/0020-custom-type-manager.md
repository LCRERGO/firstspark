# ADR 0020: Custom-type manager and test panel

## Status

Accepted.

## Context

Cheat Engine manages custom types from the Value Type dropdown: define (Lua or
Auto Assembler), edit, delete. The type's metadata is really a script
convention rather than a dialog. Firstspark's first cut only offered a
create/replace dialog — no list, no edit, no delete, and no way to check that a
script works before scanning.

## Decision

Replace the dialog with a **manager window** opened from a **"…" button beside
the Value Type dropdown** (and the Table menu). The manager lists the types in
`customtypes.yaml`, and can add, edit and delete them:

- Delete rewrites `customtypes.yaml` and calls a new `scan.UnregisterType`, so
  a deleted type disappears from the registry. Table entries still referencing
  it fall back to displaying raw bytes (the existing behaviour for an unknown
  type ID).
- The editor form holds name, byte size, kind, alignment, description and the
  script, plus a **Test** panel: convert a hex byte string to a value, read N
  bytes from a chosen target address and convert them, and check the
  `value_to_bytes` round-trip.
- Selecting a custom type sets the scan panel's alignment from the type's
  `alignment` (Cheat Engine's `preferedAlignment`).

Auto-Assembler-defined custom types are **not** implemented: they require
native conversion routines executed against the target, which is a separate,
risky subsystem. Lua-backed types cover the same conversions.

## Consequences

- Feature parity with Cheat Engine's create/edit/delete flow, plus a test
  facility Cheat Engine lacks in its UI.
- The manager is the single place types are edited; the old dialog is removed.
- Deleting a type is unconditional, as in Cheat Engine.
