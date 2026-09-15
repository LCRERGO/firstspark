# ADR 0026: Collapsible process tree

## Status

Accepted.

## Context

Cheat Engine's process list is flat, which makes it hard to tell which process
spawned which and to focus on a subtree (a game and its helpers, a shell and its
children). Other tools (htop, pstree) show a parent/child tree with collapsible
nodes.

## Decision

- `pkg/mem.Process` gains `PPID`, parsed from `/proc/<pid>/stat` (after the last
  `)` so the `comm` field's spaces and parentheses do not confuse the parser).
- The Process List window gains a **Tree** checkbox. In tree mode the list is
  the process hierarchy: children are indented under their parent and a
  disclosure triangle (`▸`/`▾`) collapses or expands a node. Siblings sort by
  the active column and direction; filtering keeps every match and its
  ancestors so matches retain their context.
- Nodes default to expanded and expansion state is kept per session in memory;
  it is not persisted. Flat mode is unchanged.

## Consequences

- Users can navigate process families and collapse noise without losing the
  flat, sortable view.
- The tree is rebuilt from `/proc` on each refresh; expansion is remembered by
  PID, so it survives a refresh but not a restart.
