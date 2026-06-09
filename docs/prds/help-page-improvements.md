# Help Page Improvements

## Problem Statement

The keybindings help modal (`?`) wastes screen space and is hard to use. It renders every binding in
a single column, so on a wide terminal most of the screen is empty while the list runs off the
bottom — and there is no visual indication that the content is scrollable or that more bindings
exist below the fold. Chords are displayed with a misleading separator (`g/l`), which reads as "press
g **or** l" when it actually means "press g **then** l". The four number keys that switch tabs
(`1`/`2`/`3`/`4`) are absent from the list entirely. And there is no way to find a binding by name or
key — the user must eyeball the whole list.

## Solution

Rework the help modal so it:

- lays bindings out in **2–3 responsive columns**, keeping each section intact, and widens toward the
  screen so the space is used;
- shows a **scrollbar gutter** down the right edge whenever the content overflows;
- renders chords as a **sequence** (`gl`, not `g/l`), freeing `/` to mean *alternatives*;
- shows the tab-switch actions on **one merged row per action** listing both accelerators
  (`1/gw  worktrees tab`);
- supports **live filtering**: press `/`, type, and the list narrows to matching bindings; `esc`
  clears the filter (a second `esc` closes help), `enter` keeps the narrowed list and returns focus
  to scrolling.

Filtering is delivered as a new shared **Filter** component (`ui/filter`), deliberately distinct from
the existing **Search** component — see ADR 0011 and the CONTEXT.md "Find: Search and Filter" entry.

## User Stories

1. As a gx user on a wide terminal, I want the help modal to lay bindings out in multiple columns, so
   that I can see most of the keybindings at once instead of a narrow strip down the left.
2. As a gx user on a narrow terminal, I want the help modal to fall back to fewer columns (down to
   one), so that bindings stay readable instead of being crushed.
3. As a gx user, I want each help section (App, Navigation, Search, …) to stay together in one
   column, so that a heading is never separated from its bindings.
4. As a gx user reading the help list, I want the sections to flow column-major (down the first
   column, then the next), so that the existing alphabetical section order still reads naturally.
5. As a gx user whose help content is taller than the modal, I want a scrollbar gutter on the right
   edge, so that I can tell at a glance that there is more to scroll and roughly where I am.
6. As a gx user whose help content fits entirely, I want no scrollbar shown, so that the chrome only
   appears when it is meaningful.
7. As a gx user, I want chords shown as `gl`, `gw`, `to`, so that the display matches how I actually
   type them (one key after another) instead of implying a choice between keys.
8. As a gx user, I want `/` in a key display to mean "either of these keys", so that merged rows like
   `1/gw` read correctly as "press 1 or gw".
9. As a gx user, I want the number keys `1`/`2`/`3`/`4` for switching tabs to appear in the help, so
   that I can discover them without reading the source.
10. As a gx user, I want each tab-switch action shown once with both its number key and its chord
    (`1/gw  worktrees tab`), so that the list documents both accelerators without repeating the
    label.
11. As a gx user, I want the number key shown before the chord (`1/gw`, not `gw/1`), so that the
    simplest accelerator is the most prominent.
12. As a gx user, I want to press `/` inside the help modal to start filtering, so that I can find a
    binding without scanning the whole list.
13. As a gx user filtering the help, I want the list to narrow as I type, so that I immediately see
    only the bindings that match.
14. As a gx user filtering the help, I want my query matched against both the binding's keys and its
    description, so that typing `push` or `gl` both find what I expect.
15. As a gx user filtering the help, I want sections with no matching bindings to disappear, so that
    the narrowed list has no empty headings.
16. As a gx user filtering the help, I want the number of matches shown (e.g. "12 matches" / "no
    matches"), so that I know whether my query found anything.
17. As a gx user typing a filter query, I want keys like `q` and `?` to type into the input rather
    than close the modal, so that I can type any query freely.
18. As a gx user filtering the help, I want `enter` to keep the narrowed list and move focus back to
    scrolling, so that I can read and scroll the filtered results.
19. As a gx user filtering the help, I want `esc` to clear the filter and return to the full list
    while keeping help open, so that abandoning a search does not also close the modal.
20. As a gx user with no active filter, I want `esc` to close the help modal, so that exiting help
    stays a single keystroke when I am not searching.
21. As a gx user, I want a `/ filter` hint in the help footer when not filtering, so that the
    capability is discoverable.
22. As a gx user, I want filtering available in the help on every tab, so that the behavior is
    consistent regardless of which screen I opened help from.
23. As a gx developer, I want the new scrollbar to be a reusable helper, so that the output modal and
    commit info header can adopt the same affordance later.
24. As a gx developer, I want filtering to be a reusable `ui/filter` component, so that the file tree
    and log views can adopt narrow-the-list filtering later.

## Implementation Decisions

### Chord display (`keys.Binding.Keys()`)
- `Keys()` joins `Seq` with `""` instead of `"/"`, so `["g","l"]` renders `gl`. The optional
  `Display` override is unchanged.
- `/` is reclaimed as the **alternatives** separator, used when merging twin bindings (below).
- The only other caller, the chord overlay, only ever shows single-key completions, so the change is
  safe there.

### Tab bindings in help (`help.BuildSections` + `app.Bindings()`)
- Root cause of the missing `1`–`4`: `app.Bindings()` registers the number keys with the **same
  `BindingID`** as their `gw`/`gl`/`gs`/`gS` chord twins, and `BuildSections` dedupes by
  `BindingID`, silently dropping the second one.
- `BuildSections` changes from *dedup-and-drop* to *merge*: bindings that share a `BindingID` within
  a category collapse into one entry whose key display is their sequences joined by `/`
  (alternatives). This is a general rule — any future alias pair (e.g. `j` + `down`) would likewise
  collapse to `j/down`.
- `app.Bindings()` registers the number bindings **before** their chord twins so the merged display
  is number-first (`1/gw`).

### Scrollbar (`ui.RenderScrollbar`)
- New pure helper in `ui/`: given the viewport `height`, the content `total` line count, the
  `visible` line count, and the scroll `offset`, it returns a `height`-line column with a track glyph
  and a proportional thumb. Returns `""` when `total <= visible`.
- Rendered to the right of the help body inside the modal frame when the content overflows.
- Built generic so the output modal and commit info header can adopt it; those adoptions are **not**
  part of this work.

### Column layout (`ui/help`)
- The single-column `RenderView` is replaced by a column packer. Each section renders as a block
  (heading + `  key  title` rows) and stays intact.
- Column count is responsive: `cols = clamp(modalWidth / targetColWidth, 1, 3)`.
- Whole section-blocks are distributed **column-major**: break to the next column when the
  accumulated height crosses `ceil(totalHeight / cols)`; a section is never split and a heading never
  orphaned at a column bottom.
- Columns are padded to equal width and joined horizontally.
- The help modal width cap is raised (today `containerWidth*2/3`, max 104) toward
  `containerWidth - margin`, retaining a `MIN_WIDTH` fallback for narrow terminals.
- The packed multi-column block feeds the viewport; the scrollbar uses the block's total height vs
  the viewport height.

### Filter component (`ui/filter`)
- New component, **separate** from `ui/search` (ADR 0011). It carries only the query, an input box,
  and an active/inactive mode — no match positions, no match cursor.
- Interface (mirrors `ui/search`'s host-facing shape minus the match engine):
  `Start()`, `Clear()`, `Query()`, `HasQuery()`, `IsActive()`, `InputFocused()`, `SetWidth()`,
  `View()` (renders the one-line input bar), and `Update(msg) (Model, tea.Cmd, Result)`.
- `Update` emits `FilterChangedMsg{Query}`; `/` activates, `esc` clears and deactivates, `enter`
  keeps the query and defocuses the input.
- The **host** owns the predicate. `ui/filter` knows nothing about bindings.

### Help/filter integration (`help.Model`)
- `help.Model` embeds the `ui/filter` model, so all screens inherit filtering with no per-screen
  wiring (help is already a shared per-screen model built via `help.BuildSections`).
- When `filter.InputFocused()`, key events route to the filter first, so help's own close keys
  (`q`/`?`/`esc`/`enter`) do not fire while typing.
- On query change, help recomputes the visible sections: a binding survives when the query
  (case-insensitive substring) matches its `Keys()` **or** its `Title`; sections with no surviving
  bindings are dropped; columns are re-packed; viewport scroll resets to top.
- The filter input renders as a **top input bar** inside the help body when active. The match count
  (`N matches` / `no matches`) renders in the modal `RightTitle`. When inactive, the footer shows a
  `/ filter` hint.
- `esc` semantics: with the filter active, `esc` clears it and stays in help; with no active filter,
  `esc` closes help.

## Testing Decisions

Good tests here exercise **external behavior** through public interfaces, not internal layout
plumbing. Prefer pure-function and component-level tests over rendered-pixel assertions; where output
is asserted, strip ANSI and check for the presence/order of plain text and structural properties
(column count, row merges) rather than exact spacing.

- **`ui/filter`** — component tests modeled on `ui/search/model_test.go`: initial state, `Start`
  enters input mode, query updates flow through `Update` and emit `FilterChangedMsg`, `esc` clears
  the query and deactivates, `enter` keeps the query and defocuses, `InputFocused()` reflects mode.
- **`ui.RenderScrollbar`** — table-driven pure-function tests: returns `""` when content fits; thumb
  size and position are proportional to `visible`/`total` and `offset`; edge cases at top, at bottom,
  and at very small heights produce a sane, in-bounds thumb.
- **`help.BuildSections` merge** — extends `ui/help/help_test.go`: twins sharing a `BindingID` within
  a category merge into one `/`-joined row (`1/gw`); distinct `BindingID`s stay as separate rows;
  ordering reflects registration order (number-first).
- **Column packer + `Keys()`** — packer tests assert sections are kept whole, the column count
  responds to width (3 wide, 1 narrow), and order is column-major; `keys` tests assert `Keys()`
  joins a chord with `""` (`gl`) and still honors a `Display` override.

Filter-on-help end-to-end behavior (esc clears then closes, `q` types while focused) can be covered
by a `help` Update-level test that feeds key messages and asserts `IsOpen` / filter state, in the
style of the existing `TestHelpCloseKeys`.

## Out of Scope

- Adopting `ui.RenderScrollbar` in the output modal or commit info header (only the help modal uses
  it here).
- Adopting `ui/filter` in the file tree or log views (the component is built shared, but only the
  help modal consumes it here).
- Fuzzy/subsequence matching for the filter — matching is case-insensitive substring for now.
- Any change to `ui/search`'s highlight-and-jump behavior or its existing consumers.
- Mouse interaction with the scrollbar (drag-to-scroll); the gutter is an indicator only.
- Reordering or recategorizing bindings beyond the twin-merge and the number-first ordering.

## Further Notes

- Decisions were reached in a grilling session; see `docs/plans/help-page-improvements.md` for the
  task checklist, `CONTEXT.md` "Find: Search and Filter" for the sharpened glossary, and ADR 0011 for
  why `ui/filter` is a separate component from `ui/search`.
- Sequencing: land the two shared-code changes first (`keys.Binding.Keys()` and the `BuildSections`
  merge + `app.Bindings()` reorder) and run the suite, since they ripple beyond the help modal —
  then the scrollbar, the column layout, and finally `ui/filter` + integration.
