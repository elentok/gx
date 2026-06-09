# ADR 0011 — Filter is a separate component from Search

## Status
Accepted

## Context

The keybindings help modal (`ui/help`) needed a way to find a binding by name or key. The codebase
already has `ui/search`, used by the diff and file-tree views. The obvious move was to reuse it.

But `ui/search` implements one specific interaction: *highlight-and-jump*. The content stays fully
visible, every match is highlighted in place, and `n`/`N` walk a match cursor through the viewport
(`Match{ViewportRow, DataIndex}`). That model assumes a long single-column stream where the user
wants to stay oriented and step between occurrences.

The help modal is the opposite shape: a short reference list rendered in **2–3 columns** (see the
help-page improvements plan). Two things break when `ui/search` meets that shape:

1. **Jump order is undefined in a grid.** With multiple columns there is no natural linear order for
   "next match" — match #1 top-left, match #2 top-right, where does `n` send the viewport? The
   match-cursor model that is intuitive in one column becomes disorienting exactly *because* of the
   columns the help page wants.
2. **The useful interaction is narrowing, not stepping.** The only reason to search a keybindings
   reference is "show me the binding for X." Hiding non-matches and re-flowing the remainder answers
   that directly; a highlighted cursor over a still-full list does not.

These are two genuinely different interaction concepts, not two configurations of one. See CONTEXT.md
"Find: Search and Filter".

## Decision

Introduce `ui/filter` as a **separate component beside `ui/search`**, not an extension of it.

- `ui/filter` carries only the query, the input box, and an active/inactive mode, and emits
  `FilterChangedMsg`. It has no match positions and no match cursor.
- The host owns the matching predicate (help matches a binding's key *and* title) and re-renders its
  own narrowed, re-flowed content. This mirrors how `ui/search` already leaves match computation to
  the host — the split of responsibility is the same, only the payload differs (query vs. match
  positions).
- `ui/search` is left unchanged for its highlight-and-jump consumers (diff, file tree).

## Considered Options

- **Extend `ui/search` with a filter mode.** Rejected: it would fuse two interaction models behind
  one type, and `ui/search`'s match-cursor machinery (`ViewportRow`/`DataIndex`, `n`/`N`,
  `jumpToMatch`) is dead weight — or actively misleading — under filtering.
- **Inline the filtering in `ui/help` only.** Rejected: filtering is wanted next for the file tree
  and log, so the narrowing interaction belongs in the shared layer (per the design-system "add it
  to the shared layer first" rule), not duplicated per screen.

## Consequences

- The codebase now has two find components, and the choice between them is a real decision: use
  **Search** for highlight-and-jump over a visible stream, **Filter** for narrow-the-list. The
  CONTEXT.md glossary entry exists to keep them from being conflated.
- Future filterable views (file tree, log) compose `ui/filter` and supply their own predicate.
- If a view ever wants both — narrow *and* step through what remains — that is a new requirement, not
  a reason to merge the components.
