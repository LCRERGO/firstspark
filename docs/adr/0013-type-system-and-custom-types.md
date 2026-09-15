# ADR 0013: Type descriptor registry and runtime custom types

## Status

Accepted.

## Context

`pkg/scan` modelled value types as a fixed `ValueType` enum, switched on in 25
places in `value.go` and referenced across 12 files. That cannot express a type
defined by the user at runtime, which Cheat Engine supports (a named, fixed-size
type with a bytes<->value conversion).

## Decision

Replace the enum with a `scan.Type` descriptor and a registry:

- Each type provides its name, byte size (or variable), parse, format, encode
  and compare behaviour. Built-ins become descriptors registered at startup;
  the existing enum values remain as stable IDs for `.CT` compatibility.
- A custom type is a descriptor backed by a `pkg/script` function pair:
  `bytes_to_value(bytes[, address])` (required) and
  `value_to_bytes(value[, address])` (optional). Types without the write-back
  function are read-only.
- Comparisons run on the converted value. The type declares a result `kind`
  of `int`, `float` or `string`, mirroring Cheat Engine's float/string
  handling.
- User types are persisted in `customtypes.yaml` (name, size, kind, script,
  alignment, description) under the config directory, and are selectable in
  the GUI and the CLI.

## Consequences

- One code path for built-in and user types; the enum is confined to
  compatibility boundaries.
- Conversion cost is paid per scan candidate; closure compilation (ADR 0012)
  keeps that path fast, and the type is compiled once at load.
- A malformed user script is rejected at load time, not mid-scan.
