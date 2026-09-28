# ADR 0046: Session formats and .CT fidelity

## Status

Accepted.

## Context

`pkg/cheattable` serialized sessions as XML (`.CT`) or JSON, and its Cheat
Engine import/export dropped several fields the `.CT` schema carries: row
colour, cached last state and hotkeys, and any unmodelled elements. There was
also no YAML form, even though YAML is the project's configuration language.

## Decision

- **YAML sessions.** The `Table`/`Entry` schema gains `yaml` tags, plus
  `MarshalYAML`/`ParseYAML`/`ExportYAML`/`ImportYAML` and a `FormatFor` +
  `LoadAny`/`SaveAs` dispatch by file extension (`.ct`, `.json`, `.yaml`,
  `.yml`; anything else is `.CT`). The GUI Open/Save dialogs and the CLI
  `--export` accept all three.
- **Reference-tool fidelity.** `Entry` gains `Color`, `LastValue`, `LastAddress`,
  `Activated`, `CEHotkeys` and `ExtraElements`.
  - `<Color>` round-trips as an opaque hex string.
  - `<LastState>` round-trips and seeds the value/address and frozen state on
    import.
  - `<Hotkeys>` round-trip; a single-key *Toggle Activation* maps to the
    Firstspark hotkey (VK → `F1`..`F12`/letter/digit), and a Firstspark hotkey
    exports as a reference-tool `Toggle Activation` binding.
  - Unmodelled child elements are preserved as name + text and re-emitted.
  - The reference tool export uses a custom `MarshalXML` so the optional elements and preserved
    extras are written in the reference tool's order.
  - `ConvertRoutine`/`ConvertBackRoutine` are extracted into `CustomTypeDef`,
    but the type is still registered raw: translating arbitrary assembly
    conversions to a Firstspark conversion remains a follow-up (ADR 0037 S5).

## Consequences

- A session can be saved as a human-editable YAML document alongside `.CT` and
  JSON, and the format is chosen by extension.
- .CT tables round-trip colours, last state, hotkeys and unknown
  elements; imported `LastState` gives values for entries that have no `<Value>`.
- Imported colours are rendered in the cheat table's Description column, and a
  frozen Firstspark record exports as an activated reference-tool `LastState`.
- Preserved unknown elements keep only their text content; nested markup is
  flattened because `encoding/xml` cannot re-emit raw markup safely.
- Custom-type assembly conversions are preserved but not applied yet.
