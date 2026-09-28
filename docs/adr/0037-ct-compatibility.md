# ADR 0037: .CT compatibility

## Status

Accepted. Staged work; S0–S6 landed. Relates to ADR 0038 (groups) and ADR 0039
(script execution).

## Context

`pkg/cheattable` serialised firstspark's own *attribute* schema
(`<CheatEntry ID= Description= Address= Type= Value/>`) and claimed the reference tool
`.CT` import/export in the README. Real .CT files use an
*element*-based schema (`<ID>`, `<Description>`, `<VariableType>`, `<Address>`,
`<Offsets>`, nested `<CheatEntries>`, `<AssemblerScript>`, …) and store no
values — only type/length metadata; values are read from the target. Parsing a
real `.CT` therefore produced a handful of empty rows and no error, silently
corrupting the table.

A representative CK3 table (1108 entries) is also structurally beyond the flat
model: 166 group headers, 55 Auto Assembler scripts, 1012 addresses of which
983 are parent-relative (`+18`, `+4*$1`), 0 absolute, and 29 are symbols defined
by the scripts.

## Decision

Commit to `.CT` compatibility as a dependency-ordered program:

- **S0 (done)** — `Parse` detected `CheatEngineTableVersion` and returned an
  error instead of blank rows.
- **S1 (done)** — the element-schema parser (`pkg/cheattable/ce.go`): nested
  entries, per-type extras (`Length`, `Unicode`, `CodePage`, `ZeroTerminate`,
  `ByteLength`, `ShowAsSigned`, `ShowAsHex`), `<Offsets>`, `<GroupHeader>`,
  `<CustomType>`, `<AssemblerScript>`. It converts absolute addresses,
  `module+offset` chains and signed-hex offsets, maps the reference-tool variable types, and
  records skipped entries in `Table.Stats` by reason. Addresses it cannot yet
  resolve (symbolic, parent-relative) are kept as `Entry.Expr` text.
- **S2 (done)** — groups/child records and tree rendering (ADR 0038). The GUI
  cheat table is a tree projected into visible rows with expand/collapse,
  recursive freeze/delete, and store/save of the hierarchy.
- Without a process or script execution, the CK3 reference file now imports
  completely: 1108 entries (166 group headers, 55 Auto Assembler scripts with
  their source preserved, 887 leaves), 980 carrying an unresolved address
  expression and 1003 nested under a parent; nothing is skipped.
- **S3 (done)** — `pkg/address` evaluates address expressions (`$`/`0x`/bare
  hex, `+ - * /`, parentheses, symbols, modules, parent-relative). `Entry`
  stores the raw expression and offset list, and the UI resolves them live each
  tick against the symbol/module tables, caching the address. Symbols themselves
  arrive with script execution (S4), so symbol-rooted entries resolve only after
  S4; module-rooted and parent-relative entries resolve now.
- **S4a (done)** — script records are preserved (`Entry.Script`) and can be run
  explicitly from the row menu after a warning; the script's symbols are merged
  into the resolver. **S4b core (done)** adds `aobscanmodule`, label export,
  `{$lua}` segmentation and `pkg/celua`'s core table scripting API
  (`AddressList`/`MemoryRecord`, memory/process helpers, `openProcess`, …) with a
  UI bridge, so the CK3 orchestrator evaluates; the remaining reference-tool AA/Lua surface
  grows incrementally (ADR 0039).
- **S5 (done, bounded)** — `registerCustomTypeAutoAssembler` blocks in a
  table's scripts are parsed for their `TypeName` and `ByteSize`, and
  `pkg/customtype.RegisterRaw` registers a passthrough integer type of that
  width (the `Custom` entries keep their name and size). The reference tool's x86
  `ConvertRoutine`/`ConvertBackRoutine` use a different calling convention than
  firstspark's AA types (ADR 0023), so the conversion (scaling, dates) is not
  applied yet; that translation remains a follow-up.
- **S6 (done)** — `Table.MarshalCE`/`ExportCE` write a real reference-tool
  element document (types mapped back, pointer chains split into
  address/offsets, groups and scripts preserved); `File ▸ Save as .CT
  Table…` uses it. A `MarshalCE` → `Parse` round trip is covered by a test.

Records store the **address expression text** and resolve it live against the
symbol table and module map, caching the last result. This is what lets scripts
register symbols after import, and it makes S6 a lossless export. Addresses are
*not* frozen to absolute values at import.

## Consequences

- Importing a reference-tool file is a staged capability: it now preserves the full tree
  and resolves expressions; only script execution and symbol extraction remain
  (S4).
- The data model gains expression-based addresses and a tree, which are also
  prerequisites for recursive freeze/set and for the deferred value-changer
  phases; the symbol table lands with S4.
- Until S4, script-driven tables (like the CK3 one) import structurally but
  their symbol-rooted addresses do not resolve.
- Full reference-tool behavior is not claimed until S6; the README must not claim
  reference-tool import/export until then.
