# ADR-0006: Commit tab removed as standalone tab

**Status**: Accepted

## Context

`TabCommit` existed as a top-level tab (key `4`, `g+c`). Its primary entry point was `nav.Open`
from the log (Enter on a commit row), not direct tab navigation. The direct shortcut (`g+c`)
landed on the last-visited commit with no way to choose a different one from that surface.

## Decision

Remove `TabCommit` as a standalone tab. The log tab becomes a **split view**: commit list on the
left/top, commit detail (the existing commit view model) on the right/bottom. The stash tab uses
the same split pattern.

## Consequences

- `g+c` and key `4` are repurposed: `4` goes to the new stash tab, `g+S` (uppercase) also goes to
  stash. `g+c` is freed.
- The commit view model is reused as the detail panel in both log and stash split views. Its
  action keybindings (amend, reword, push) remain available when focus is in the detail panel.
- `nav.Open(ViewState{Tab: TabCommit, ...})` must be replaced with split-panel focus within the
  log tab.
- All tests referencing `TabCommit` need updating.
