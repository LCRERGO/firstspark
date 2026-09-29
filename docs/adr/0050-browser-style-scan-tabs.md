# ADR 0050: Browser-style scan tabs

## Status

Accepted. The tab container is a custom `tabView` as of ADR 0053, which adds
double-click rename; the lifecycle and semantics below are unchanged.

## Context

The reference tool's scanner supports **multiple scan tabs**, each an independent scan,
so a user can search for health, ammo and money side by side instead of
restarting one scan at a time. Firstspark had exactly one scan: `internal/ui`
held a single `session`, `results` slice and `foundList` on `App`, and
`scanPanel()`/`buildFoundList()` wrote directly into those fields. There was no
way to keep two scans alive at once.

## Decision

- **A *scan tab* owns its Found list, its full set of scan controls and one
  `scan.Session`** (options, results, undo history). The cheat table is
  **shared** across tabs: *Add to Table* always appends to the one address list.
- **Placement.** Tabs sit in the main window's upper region, above a thin shared
  **speed strip** (Speedhack and Unrandomizer, which hook the process, not a
  scan) that sits at the bottom of that region, just above the cheat table, like
  the reference tool. The cheat table and status bar stay global. The tab container is
  the custom `tabView` of ADR 0053 (Fyne's `DocTabs` exposes no tab-button
  hook).
- **Lifecycle (browser semantics).** `+` adds a tab, `×` closes one, `Ctrl+T`
  opens, `Ctrl+W` closes, `Ctrl+Tab`/`Ctrl+Shift+Tab` cycle; tabs are renamed by
  double-clicking the tab (or `F2`, or Scan ▸ Rename Tab);
  tabs auto-name `Scan 1`, `Scan 2`, …; there is always at least one tab;
  closing a tab that holds results asks for confirmation.
- **Per-tab scan state.** Value, value type, scan type, compare, hex,
  alignment, region scope/selection, executable/COW, Start/Stop range and the
  undo history are all per tab.
- **Concurrency and refresh.** One scan per tab; a scan may run in a background
  tab while another tab is active, and switching tabs never cancels it. The
  live Found refresh runs for the **active tab only** (plus once on becoming
  active). Cheat-table refresh and freezing stay global.
- **Process changes.** Selecting a new process clears every tab's session and
  results but keeps the tab count and names; the target dying leaves results in
  place as before; *New table* collapses back to a single fresh tab.
- **Compare tabs.** Two tabs' result sets can be compared **by address**:
  only-in-this (`A−B`), only-in-other (`B−A`), in-both (`A∩B`), in-exactly-one
  (`A△B`). The outcome lands in a **new** tab seeded with the copied rows and
  capped by `ui.result_limit`; the sources are untouched. Compare is offered
  only when both tabs share a value type, so the result tab can read live
  values and continue next scans. The address algebra is a pure `pkg/scan`
  helper plus a results-seeding session constructor, keeping the engine
  UI-agnostic.
- **Persistence.** Tabs are session-scoped: nothing is written to `.CT`/session
  files or `pkg/config`.

## Consequences

- Parallel scans become possible, matching the reference tool.
- The GUI's single `session`/`results`/`foundList`/`scanPanel` state must be
  refactored into a per-tab controller; those builders can no longer assign to
  `App` fields.
- ADR 0009 reserved `Ctrl+T` for "add scan tab"; it is now bound. `Ctrl+W` and
  `Ctrl+Tab` are browser-style additions with no reference-tool equivalent.
- A tab can originate from a scan or from a scan-tab compare.
- Renaming is available from the tab itself (double-click), `F2` and the Scan
  menu, via the custom `tabView` (ADR 0053).
- `pkg/scan` gains a compare helper and a results-seeding constructor; the scan
  hot path is unchanged.
