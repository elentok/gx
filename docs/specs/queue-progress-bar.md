# Queue progress bar: one shared `▰▱` bar and a new Queue epic row

## Problem Statement

The Queue tab shows an epic's progress only as text: `● 3 of 5 done` after the epic name. From the
person's side:

- "3 of 5 done" must be read and worked out. A bar can be seen at a glance, and the eye can compare
  many epics down one column.
- The app title row already draws a `▰▱` bar for the whole queue, but the Queue tab itself, where
  the person looks at each epic, has none.
- gx has two bar looks today: the title row's `▰▱` (hand-drawn, because it needs the title row's
  background) and the Claude status line's `■·` (bubbles `progress`, which can't take a background
  style). The same idea looks different in two places.
- The epic status line never shows the **parked** epic state: the Queue rows pass no parked tickets
  into the status logic, so a parked epic reads as "needs attention" or as plain progress.
- Time and cost are uneven. A running epic shows no elapsed time. A done epic shows "took …" but
  not the agent time. In server mode the cost is today's ledger share for the root, not the epic's
  real cost, so it resets at midnight and disagrees with the ticket rows.
- Each epic spends a whole row on "Context window: avg …, max … (N compacts)". The averages are
  rarely useful at the epic level; only the compacts count is.

## Solution

One shared progress-bar component in `ui`, in the `▰▱` design, used in three places: the app title
row (7 cells), the Claude status line (12 cells, switching from `■·` to `▰▱`) and every Queue epic
row (12 cells). Each caller picks its own width and styles.

Every Queue epic gets a new layout around the bar:

```
 queue-progress-bar                           (3/5) ▰▰▰▰▰▰▰▱▱▱▱▱
 Status: ◔ implementing · 2h 14m elapsed · 41m agent · $0.89 · 1 compact

   ✓ Add ui.ProgressBar component                   2m 41K $0.31
```

- **Line 1:** the epic name, then `(done/total)` and the bar, right-aligned to the pane width.
- **Line 2:** `Status:`, the epic state with its icon, then elapsed, agent time, cost and compacts,
  joined by ` · `. Parts that are zero are hidden.
- **Then** a blank line, then the epic's tickets (unchanged).

The bar's filled part takes the state's colour; the rest is `ColorSurface1`. The bar shows on every
epic with at least one ticket, done and parked included.

On a narrow pane every epic moves its bar to its own line under the name (`▰▰▰▱▱▱▱▱▱▱▱▱ (1/4)`), so
all bars stay in one column. On a very narrow pane the bar drops and only `(1/4)` stays.

The parked state now shows, in orange (red when a ticket is needs-repair or errored), with the ids
it waits on (at most 3, then `+N more`). An epic with only drafts left shows `parked — only drafts
left`.

## User Stories

Shared bar

1. As a gx user, I want every progress bar in gx to use the same `▰▱` look, so that gx feels like
   one app.
2. As a gx user, I want a bar to be full only at 100%, so that a full bar never lies about work
   still left.
3. As a gx user, I want an empty bar at 0%, so that "nothing done yet" is clear at a glance.
4. As a gx user, I want the title row's queue bar to look and behave as it does today, so that the
   shared component changes nothing I already rely on there.
5. As a gx user, I want the title row's bar to keep the title row's background colour, so that it
   doesn't look pasted on.
6. As a Claude Code user with the gx status line, I want the context bar in the same `▰▱` look, so
   that it matches gx.
7. As a Claude Code user, I want the status line's bar to keep its green / yellow / orange / red
   colour by token count, so that the warning steps still work.
8. As a Claude Code user, I want the status line's bar to stay 12 cells with its percentage after
   it, so that the line keeps its width and layout.

Queue epic row: line 1

9. As a gx user on the Queue tab, I want a bar on each epic row, so that I can see each epic's
   progress at a glance.
10. As a gx user, I want `(done/total)` next to the bar, so that I also get the exact numbers.
11. As a gx user, I want the counter and bar right-aligned to the pane edge, so that all bars line up
    in one column and are easy to compare.
12. As a gx user, I want the bar on done epics too, so that a full bar confirms the epic finished.
13. As a gx user, I want the bar on parked epics too, so that I see how far a parked epic got.
14. As a gx user, I want no bar on an epic with no tickets, so that an empty bar doesn't suggest
    there is work.
15. As a gx user, I want the bar's fill in the epic's state colour, so that the bar and the Status
    line tell the same story.
16. As a gx user, I want the bar's fill in the default text colour while the epic is queued or
    implementing, so that a healthy epic stays quiet.
17. As a gx user, I want the empty part of the bar in a soft surface colour, so that the filled part
    stands out.

Queue epic row: line 2

18. As a gx user, I want a `Status:` line under the name, so that the epic's state and numbers have
    their own row and don't fight the name for space.
19. As a gx user, I want the state word with an icon (`○ queued`, spinner `implementing`, `⚠ needs
    attention`, `⚠ parked — waiting on …`, `✓ done`), so that I can tell the state without reading
    the numbers, and `○ queued` → spinner `implementing` → `✓ done` reads as one sequence.
20. As a gx user, I want an implementing epic to show the circular progress spinner, so that I see it
    is alive.
21. As a gx user without a Nerd Font, I want the spinner to fall back to ASCII frames, so that it
    never shows a missing-glyph box.
22. As a gx user, I want only the state word and its icon coloured and the rest of the line dim, so
    that the colour points at the one thing that matters.
23. As a gx user, I want "needs attention" in yellow when a ticket errored, needs an answer or needs
    repair while the epic still has other work going, so that I notice a problem early.
24. As a gx user, I want "parked — waiting on <ids>" in orange when the epic has nothing left to run
    but needs-answer, needs-repair or error tickets, so that I know it waits on me and on which
    tickets.
25. As a gx user, I want the parked state in red when any waiting ticket is needs-repair or errored,
    so that a fault stands out from a question.
26. As a gx user, I want a draft ticket not to count as "waiting on" me, so that a ticket parked on
    purpose doesn't look like a request.
27. As a gx user, I want an epic with only drafts left (nothing open, claimed, needs-answer or
    needs-repair) to show `parked — only drafts left` in orange with no ids, so that I see it has
    stopped without being told it waits on a specific ticket.
28. As a gx user, I want a long "waiting on" list to show at most 3 ids then `+N more`, so that the
    line stays short.
29. As a gx user, I want "done" in green, so that finished epics read as finished.
30. As a gx user, I want an epic that has started but runs nothing right now (queue paused, waiting
    for a slot, between tickets) to show `queued` with its elapsed time, so that I see it has been
    going; the title row already tells me about pause or drain.
31. As a gx user, I want the elapsed time of every not-done epic to climb live, at 1-minute
    resolution (`2h 14m`, `<1m` under a minute), so that I see how long it has been going.
32. As a gx user, I want the elapsed time to cost no new timer, so that gx stays cheap while idle; I
    accept that it may be a few minutes stale until the next redraw.
33. As a gx user, I want a done epic's elapsed time fixed at its start-to-completion span, so that
    the number stops when the work stops.
34. As a gx user, I want no elapsed time when the epic has no start time, so that gx never shows a
    guess.
35. As a gx user, I want the agent time (time agents actually spent on its tickets, including live
    time of running tickets), so that I can compare agent work with wall-clock time.
36. As a gx user, I want the epic's cost as the sum of its tickets' costs, in local and server mode
    alike, so that the epic number matches the ticket rows and doesn't reset at midnight.
37. As a gx user, I want the compacts count on the Status line, so that I still see when agents ran
    out of context.
38. As a gx user, I want "1 compact" and "2 compacts" with the right plural, so that the line reads
    naturally.
39. As a gx user, I want zero-valued parts hidden (a never-started epic shows only `○ queued`), so
    that the line has no noise like "0m agent · $0.00".
40. As a gx user, I want no trailing ` · ` when parts are hidden, so that the line looks clean.
41. As a gx user, I want the Status line cut at the pane edge, so that a long "waiting on" list never
    wraps or breaks the layout.
42. As a gx user, I want the old "Context window: avg …, max …" row gone, so that each epic takes no
    more space than it needs.
43. As a gx user, I want a blank line between the Status line and the tickets, so that the epic
    header reads as one block.

Narrow panes

44. As a gx user on a narrow pane, I want every epic to put its bar on its own line when any epic's
    line 1 doesn't fit, so that bars stay in one column and never overlap the name.
45. As a gx user on a narrow pane, I want the counter after the bar (`▰▰▰▱▱▱▱▱▱▱▱▱ (1/4)`) in that
    layout, so that the bar still starts in the same column on every epic.
46. As a gx user on a very narrow pane (about 20 columns), I want the bar dropped and only `(1/4)`
    kept, so that the numbers still show.
47. As a gx user on a narrow pane, I want a name too long for the pane cut at the edge with `…`, as
    the Status line is, so that it never wraps.
48. As a gx user, I want the layout to follow the pane width only, so that resizing gives the same
    result every time.

Unchanged

49. As a gx user, I want the Queue tab header ("Queue · N of M done · implementing…") to stay text
    with no bar, so that the title row's bar isn't repeated right under it.
50. As a gx user, I want ticket rows, selection, folding, search, the epic context menu and the
    plan-error line to work as before, so that only the epic header changes.
51. As a gx user, I want to still select an epic row (the name line) and open its context menu, so
    that epic actions stay reachable.

## Implementation Decisions

Shared component

- New function in `ui`: `ProgressBar(fraction float64, cells int, fill, rest lipgloss.Style) string`.
  It draws `cells` glyphs: `▰` styled with `fill`, then `▱` styled with `rest`.
- Filled cells = `floor(fraction × cells)`, with `fraction` clamped to [0, 1]. The bar is full only
  at fraction 1. Guard float error so exact values (e.g. 7/7, 3/3) give the expected count; callers
  with integer counts may pass `done/total`.
- `cells <= 0` returns an empty string.
- It replaces bubbles `progress` (the status line's only use), because that can't take a background
  style. Once the status line moves over, drop the bubbles `progress` import if nothing else uses it.
- Callers:
  - App title row: 7 cells. Fill = the queue mode colour on the title background, rest = the
    existing "bar rest" title style. Same output as today.
  - Claude status line: 12 cells, `▰▱` instead of `■·`. Fill = the token-count colour. Rest = a
    plain `ColorSurface1` foreground (no background: it prints into Claude Code's terminal). Brackets
    and the percentage text stay.
  - Queue epic rows: 12 cells (see below).

Queue epic header rows

- The epic's header rows in the Queue tree become: separator (blank), **name line**, optional **bar
  line** (narrow layout only), **Status line**, **blank spacer**, optional plan-error line, then the
  tickets. The "context window" node kind goes away. The name line keeps today's role as the epic's
  selectable row; the bar line, Status line and spacer are not selectable.
- Name line (wide layout): one leading space, the epic name in the current epic-header style, padding,
  then `(done/total)`, a space and the 12-cell bar, ending at the pane width. Counts come from
  `Epic.DoneCount()` / `Epic.TotalCount()` (done = terminal, as today). With 0 tickets: no counter,
  no bar.
- Width check: line 1 fits when `1 + name + 1 + counter + 1 + 12 ≤ pane width`. If any epic shown in
  the tab (after the project filter and hide-complete) doesn't fit, every epic uses the narrow
  layout for that render.
- Narrow layout: the name line holds only the name. A name wider than the pane is cut at the pane
  edge with an ANSI-aware cut ending in `…`, like the Status line. A bar line follows: one leading space, the bar,
  a space, `(done/total)`. If the bar line itself doesn't fit (about 20 columns), every epic drops
  the bar and the bar line shows only `(done/total)`.
- Bar colours: fill = the state colour (default text colour for queued and implementing); rest =
  `ColorSurface1`.

Epic state

- One function decides the epic state from the epic, whether it is running and its parked tickets,
  and returns the state, icon, label text and colour. Precedence:
  1. **done** — `Epic.AllDone()`. Green. Icon: the icon set's done icon.
  2. **parked** — the epic is parked (below). Text `parked — waiting on <ids>`: at most 3 ids, then
     ` +N more` (e.g. `parked — waiting on 03, 05, 07 +2 more`). Orange; red when any listed ticket
     is needs-repair or error. When only drafts are left, the text is `parked — only drafts left`,
     orange, with no ids. No `(reattachable)` suffix.
  3. **needs attention** — any ticket renders error, needs-answer or needs-repair. Yellow. Icon:
     warning.
  4. **implementing** — the server reports the epic running. No colour. Icon: the current frame of
     the Queue spinner.
  5. **queued** — anything else, including an epic that has started but runs nothing right now
     (queue paused, waiting for a slot, between tickets). No colour. Icon: `○` (empty circle) in
     both the Nerd and plain icon sets, from a new `IconSet` field (e.g. `EpicQueued`) in
     `ui/icons.go`. With the spinner and the done icon this reads `○ queued` → spinner
     `implementing` → `✓ done`.
- Only the state icon and word take the colour. `Status:` and the parts after it use the dim
  metrics style.
- Mockup glyphs are illustrative; real glyphs come from the icon set (Nerd and plain variants).

Parked tickets

- The Queue rows stop passing `nil` for parked tickets. The client derives them with the same rule
  as the server's all-parked deadlock check (`server/deadlock.go`): the epic is parked when no
  ticket renders open or claimed and some ticket is needs-answer, needs-repair, error or draft
  (done and cancelled tickets don't count).
  - The waiting list is the remaining tickets that render needs-answer, needs-repair or error.
    Drafts are never listed.
  - If the list is empty (only drafts are left), the epic is parked with `only drafts left`.
- One client-side rule for both server and local modes. Reattachability is not shown, and the
  `(reattachable)` suffix goes away from the Queue epic Status line.
- Derived at draw time from the epic, like the rest of the live state, so the entries cache is not
  affected.
- Use "parked" in all new names and text, per ADR 0020 (the existing `StalledTicket` type name is
  out of scope to rename).

Status line values

- Parts, in order, joined by ` · ` with no trailing separator; any part that is zero or unknown is
  hidden:
  - `<elapsed> elapsed`: `now − started_at` in every state except done (live; uses the model's clock
    so tests can fix it); `completed_at − started_at` once done. Hidden when `started_at` is missing.
    1-minute resolution: `2h 14m`, `45m`, and `<1m` under a minute.
    For a done epic with `started_at` but no `completed_at`, fall back to the sum of its tickets'
    elapsed time (today's "took" fallback).
  - `<agent> agent`: the sum of landed tickets' elapsed time plus the live elapsed time of tickets
    running now.
  - Cost: the sum of the epic's tickets' `actual_cost`, everywhere, server mode included. In server
    mode this changes the number from today's spend for the epic (`BudgetStatus.Roots`, read only at
    `ui/tickets/queue_view.go:135`) to the all-time ticket sum. `BudgetStatus.Roots` stays on the
    wire for this epic. The Queue tab header's "today $X" spend is unchanged.
  - `<n> compact` / `<n> compacts`: the sum of the tickets' compactions.
- Elapsed uses the Queue tab's existing wall-clock format (`formatDuration`, e.g. `2h 14m`), except
  that under a minute it shows `<1m` instead of `0m`. Agent time uses the same format. Cost uses the
  existing cost format.
- The Status line is cut at the pane edge (ANSI-aware truncation), never wrapped.

Elapsed refresh

- No new timer or tick. Elapsed is computed at render time and refreshes on any redraw that happens
  for other reasons: the spinner tick while running, store events, key presses, tab switches.
- While idle, elapsed may be stale by a few minutes. This is accepted.

Spinner

- The Queue tab's spinner picks its frames from the Nerd Font setting (circular progress spinner,
  ASCII fallback), instead of always using the Nerd Font frames. This also changes the Queue tab
  header's spinner, which shares it.

Unchanged

- The Queue tab header text gets no bar.
- Ticket rows, their metrics, park-reason subtext, the plan-error line, selection, folding and search
  stay as they are.
- The Tickets tab's epic list and the web UI are untouched.

## Testing Decisions

- Good tests check what the person sees (the rendered text, stripped of ANSI where colour isn't the
  point), not the helpers behind it.
- `ui.ProgressBar`: table test on plain output: 0, 1, fractions just under a cell edge, just under 1
  (never full), exactly 1, out-of-range values (clamped), integer ratios like 7/7 and 1/3, and
  `cells <= 0`. One case checks that `fill` and `rest` styles are applied to the right parts.
- App title row: the existing golden tests and `TestTitleRow_QueueBarDropsFirst` must pass unchanged.
  They are the guard that the shared bar keeps the title row identical.
- Claude status line: update `TestContextProgressValue_RendersBar` to expect `▰`/`▱` and the
  percentage, and add a case that 100% is full and 99% is not.
- Queue tab: test through the Queue model's rendered body at a fixed width (prior art:
  `queue_header_test.go`, e.g. `TestQueueModelEpicHeaderRendersStatusAndContextLines`,
  `epicHeaderLine`, and `queue_park_subtext_test.go`), loading real ticket files. Cover:
  - wide layout: counter and bar end at the pane edge; the bar is full only when all done;
  - no bar and no counter for a 0-ticket epic;
  - narrow switch: one long epic name makes every epic use the bar-on-its-own-line layout, with all
    bars in the same column;
  - very narrow: only `(n/m)` is left;
  - each state's text and colour (queued, implementing, needs attention, parked orange, parked red,
    done), and that only the state word is coloured;
  - parked shows when no ticket is open or claimed and one is needs-answer; drafts are not listed;
  - an error ticket is listed in the waiting list and makes the parked state red;
  - only drafts left shows `parked — only drafts left` in orange with no ids;
  - a waiting list of 5 shows 3 ids then `+2 more`;
  - no `(reattachable)` suffix in either mode;
  - a started epic with no ticket running shows `○ queued` with its elapsed time;
  - Status line parts: a queued epic shows only `queued`; zero parts are hidden; no trailing `·`;
    compact plural; the line is cut at the pane edge;
  - elapsed from a fixed clock in every not-done state, start-to-completion once done, hidden with
    no start; `<1m` under a minute and minute resolution above it;
  - narrow layout: a name wider than the pane is cut with `…`;
  - agent time adds live running time to landed time;
  - cost is the ticket sum in server mode too (replace the ledger-per-root case in
    `queue_server_budget_test.go`);
  - the "Context window" row is gone, and the epic name line is still the selectable epic row.
- Existing tests that read the old text ("took …", "N of M done", "Context window: …") are updated to
  the new layout, not kept alongside it.

## Out of Scope

- A progress bar on the Tickets tab's epic list, or in the web UI (possible follow-ups).
- A segmented bar (done / in progress / parked), with new colours and new count logic.
- A bar in the Queue tab header.
- Renaming `ralphloop.StalledTicket` or other old "stalled" names.
- Changing how the server computes or reports budget. `BudgetStatus.Roots` stays on the wire;
  removing it, now that the TUI no longer reads it, is a follow-up.
- A server-sent per-epic parked flag with reattachability (follow-up). Until then the client derives
  parked itself in both modes and shows no `(reattachable)` suffix.
- A new timer or tick to keep elapsed fresh while idle.

## Further Notes

Assumptions made where the decisions were silent (open to change during implementation):

- Status precedence is done > parked > needs attention > implementing > queued.
- The Claude status line's bar rest colour is `ColorSurface1` (today it is bubbles' default grey).
- Width threshold for dropping the bar is computed (bar line wider than the pane), not a fixed 20.

Settled decisions worth knowing:

- Server-mode epic cost changes meaning: from today's spend for the epic (`BudgetStatus.Roots`) to
  the all-time sum of its tickets' costs. It no longer resets at midnight.
- Elapsed has no timer of its own, so on an idle screen it can lag by a few minutes.
