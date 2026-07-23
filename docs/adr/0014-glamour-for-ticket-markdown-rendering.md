# ADR 0014 — `glamour` renders ticket/map markdown verbatim

## Status
Accepted

## Context

The tickets tab (`.scratch/gx-tickets/spec.md`) previews the selected ticket's or wayfinder map's
markdown file body in a side-by-side preview panel. The first prototype round hand-parsed the file
into its conventional `## Question`/`## Answer`/`## Comments` sections and rendered each with
existing `ui.Style*` lipgloss helpers — consistent with how the rest of this app renders content
(every other panel in `gx` is hand-styled lipgloss, no markdown renderer exists in the module
graph today).

That approach broke down for two reasons surfaced during the prototype's reaction round:

1. **Section conventions aren't guaranteed.** `.scratch/` ticket files are free-form markdown by
   convention, not by schema (see `docs/agents/issue-tracker.md`) — a `map.md` has a completely
   different section shape (Destination/Notes/Decisions so far/Not yet specified/Out of scope) than
   a ticket (Question/Answer/Comments), and either can contain arbitrary nested markdown (lists,
   code spans, links) the hand-parser would need to special-case indefinitely.
2. **Hand-parsing loses information.** Anything inside a section beyond flat text — nested bullets,
   inline code, emphasis — would render as raw markdown syntax unless the parser grew its own
   mini markdown-to-lipgloss renderer, which is what a markdown-rendering library already is.

## Decision

Render the raw file body verbatim through `charm.land/glamour/v2` (the library behind `glow`),
using a custom `glamour.StyleConfig` built from this app's existing Catppuccin Mocha palette
constants rather than one of glamour's built-in presets — so ticket/map bodies read as part of the
same visual system as the rest of the app (no background blocks on headings; bold+color only,
matching this app's spare look) instead of glamour's default styling. Only the header line (icon +
number + title, since the title lives in the filename, not the file body) and the metadata line
(status/type/blocked-by) are still synthesized outside glamour; everything else is
`glamour.Render` output, unmodified.

Confirmed version-compatible with this repo's existing `charm.land/lipgloss/v2` before adopting it.

## Considered Options

- **Hand-parsed sections** (the original prototype approach) — rejected: assumes a section schema
  `.scratch/` files don't actually guarantee, and would eventually need to reimplement markdown
  rendering piecemeal (lists, code spans, emphasis) to avoid showing raw syntax.
- **Raw text dump, no rendering** — rejected: shows literal `##`/`-`/backtick syntax to the user,
  which is exactly the illegible-syntax problem this panel exists to solve.
- **`glamour` with a built-in preset style** (e.g. `dark`) — rejected: would visually clash with
  the rest of the app's specific Catppuccin Mocha palette and spare, no-background-block heading
  style; a custom `StyleConfig` costs little more than picking a preset.

## Consequences

- This is the first markdown-rendering dependency in the module graph — worth knowing before
  reaching for hand-styled lipgloss the next time a raw markdown file needs displaying somewhere
  else in the app; `glamour` (with this app's Catppuccin `StyleConfig`) is now the precedent.
- The custom `StyleConfig` is a new artifact to maintain in parallel with `ui/styles.go`'s existing
  `ui.Style*` constants — a palette change there should be cross-checked against the glamour style
  mapping too, since they're two independent places encoding the same color-to-role decisions.
- Ticket/map bodies are rendered as-authored; there is no way to selectively suppress or reformat
  part of a file's content without editing the file itself (consistent with this tab being
  read-only over ticket state — see the spec's "Out of scope").
