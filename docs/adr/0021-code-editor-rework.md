# ADR 0021: Code editor rework

## Status

Accepted. Extends ADR 0014.

## Context

The first `TextGrid` editor (ADR 0014) had syntax highlighting, a gutter and
undo/redo, but no selection, no clipboard, no scrolling, no find/replace and no
error feedback — below Cheat Engine's Auto Assembler editor, which offers line
numbers, bracket matching, auto-indent, block indent, find/replace, bookmarks
and a current-line highlight.

## Decision

Rework the shared editor widget (`internal/ui/editor.go`) to add:

- **Core editing**: mouse-drag and shift+arrow selection, cut/copy/paste/select
  all through the system clipboard, scrolling, undo/redo, and
  Ctrl+Home/End, Page Up/Down, delete-word/line.
- **Code editing**: auto-indent on newline, bracket matching and highlight,
  current-line highlight, comment toggle, block indent/unindent, go-to-line,
  find/replace (inline bar) and a line:column status bar.
- **Feedback**: parse errors shown as gutter markers and a status line, updated
  by a Check action.
- **Language modes**: the editor highlights Lua (via `pkg/script.Tokenize`) or
  Auto Assembler (via a new `pkg/autoasm.Tokenize`) depending on the script
  being edited, so one widget serves both windows.

Advanced features (autocomplete, code folding, multi-caret, bookmarks, zoom)
remain out of scope.

## Consequences

- The custom-type and Auto Assembler editors share one improved widget.
- We maintain an editor rather than a text area, but only the features scripts
  need, and the highlighter stays consistent with the parsers.
