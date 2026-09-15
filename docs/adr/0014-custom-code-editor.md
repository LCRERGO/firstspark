# ADR 0014: Custom code editor widget

## Status

Accepted. Extended by ADR 0021.

## Context

Editing Lua custom-type scripts needs syntax highlighting. Fyne has no code
editor, and third-party Fyne editors are immature. We considered tree-sitter
for highlighting, but that would add a second grammar (CGO) that can drift from
the interpreter, and we now own a parser for the language (ADR 0012).

## Decision

Build a `TextGrid`-based editor widget in `internal/ui` that uses the
`pkg/script` tokenizer for highlighting, so the highlighter and the interpreter
share one grammar. It provides syntax highlighting, cursor and selection,
undo/redo, and line numbers.

Bracket matching, auto-indent, find/replace and autocomplete are out of scope
for now.

## Consequences

- No CGO grammar and no grammar drift; highlighting is always consistent with
  what the engine will accept.
- We maintain an editor widget, but only the features scripts actually need.
