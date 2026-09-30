# Firstspark

Firstspark is a memory scanner, debugger and code patcher. This file is the
project glossary: the canonical names for its domain concepts and the words to
avoid for them.

## Language

**First scan**:
The initial pass that searches the target's regions for addresses matching a
value or scan mode, producing the collected results.

**Next scan**:
A filtering pass over the *collected* results that keeps only the addresses
whose live value still matches the chosen mode.

**Scan session**:
The state of one scan workspace: its collected results, the last scan step's
snapshot for undo, and the options a next scan uses.

**Collected result**:
A match the engine found and retained for a scan session. A next scan filters
collected results, whether or not they are currently displayed.

**Displayed result**:
A collected result the Found list is currently showing. The Found list shows
only the first *display limit* results of the current order, but every
collected result stays available to a next scan.

**Collection limit**:
The cap on how many collected results a scan may produce (`scan.collect_limit`).
Reaching it stops the scan. Zero means unlimited.

**Display limit**:
The cap on how many collected results the Found list shows at once
(`ui.result_limit`). It never changes what was collected or what a next scan
filters. Zero means unlimited.

**Found list**:
The per-tab list of displayed results for a scan session.

**Cheat table**:
The shared table of addresses the user has pinned, independent of any scan
session.
_Avoid_: address list
