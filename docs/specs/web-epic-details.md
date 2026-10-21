## Problem Statement

The web UI shows an epic only as a Queue card. A person sees the epic name, a progress bar and a few
active or parked tickets. They cannot see what the epic is for, which tickets it holds, what blocks
the waiting ones, how long it has run, or what it has cost. To learn that, they must drop to the TUI
or read ticket files.

## Solution

A read-only epic details page in the web UI, at `#/e/<project:epic>`. A person reaches it from the
epic name on an active Queue card and from the name on a Recently finished row. The page shows the
epic's name, a short summary, a progress bar, one line of stats, and the tree of its tickets. It
updates live while the epic runs.

The page works for loop epics and map epics. It loads any epic address, so a future epics list page
can link to it. Nothing links to a map epic yet.

## User Stories

1. As a person watching the Queue, I want to click the epic name on an active card, so that I can see
   the whole epic and not only its active tickets.
2. As a person who just saw an epic finish, I want to click its name in Recently finished, so that I
   can check what it did and what it cost.
3. As a reader, I want a short summary under the epic name, so that I remember what the epic delivers
   and why.
4. As a reader of an older epic with no summary, I want the summary block hidden, so that the page
   shows no empty gap.
5. As a reader, I want one progress bar split by ticket status, so that I see at a glance how much is
   done, running, waiting for an answer, waiting for other work, or needs attention.
6. As a reader, I want one dim stat sentence under the bar with the epic state, done count, wall-clock
   duration, total cost and total agent time, so that I get the numbers without a stat grid. Cost and
   agent time cover landed tickets only.
7. As a reader, I want no token counts anywhere, so that I am not shown sums of context windows that
   mean nothing.
8. As a reader, I want finished tickets folded into a closed list labelled with their count, so that
   the live work is what I see first.
9. As a reader, I want cancelled tickets to count as done in the bar and in "N of M done", so that the
   page matches the Queue card and the bar fills when the epic finishes.
10. As a reader, I want cancelled tickets in the fold with a muted `cancelled` word, and the fold label
    to say how many were cancelled, so that I can tell them from tickets that landed.
11. As a reader, I want each live ticket as a tinted block in its status tone, so that I see the state
    of each one without reading a word.
12. As a reader, I want each live ticket to show its status and elapsed time on one dim line, plus its
    cost when it has one, so that I know how it is doing.
13. As a reader, I want a blocked ticket to say what blocks it, such as "after 04 and 05", so that I
    know what it waits for.
14. As a reader, I want a blocker from another epic written as `other-epic/06`, so that I can find it
    without a project prefix or a long address.
15. As a reader, I want forks nested under their parent ticket, so that I see where the extra work
    came from.
16. As a reader, I want deep fork chains to stop indenting after two levels, so that rows stay readable
    on a phone.
17. As a reader, I want a done fork in the fold to say `fork of NN`, so that I keep the link to its
    parent even in a flat list.
18. As a reader, I want a live fork whose parent is done to sit at the top level with `fork of NN`, so
    that it does not indent under a row that is hidden in the fold.
19. As a reader, I want a running ticket's elapsed time to tick, so that I see it is alive.
20. As a reader, I want a ticket that needs an answer to show its question and an `Answer →` link to
    the ticket page, so that I can unblock it quickly.
21. As a reader, I want draft tickets shown as live blocks with the word `draft`, so that I know they
    are not queued yet.
22. As a reader, I want needs-repair and error tickets in a `needs attention` segment in the error
    tone, so that I see what is broken. The segment is hidden when empty.
23. As a reader, I want the epic state word printed as the server states it, so that the page and the
    Queue agree.
24. As a reader of a merged epic, I want the state to read `finished`, so that the page does not say
    `awaiting-merge` for work that already merged.
25. As a reader, I want an epic with no tickets to show its name, summary and a dim `No tickets yet`
    line, so that the page is not a broken bar and "0 of 0 done".
26. As a reader of a map epic, I want its Destination as the summary and its decision tickets with
    status only, so that I can follow the map.
27. As a reader of a map epic, I want no state word, badge, elapsed time or cost, so that the page
    shows only what a map has.
28. As a reader, I want the page to update when the epic or one of its tickets changes, so that I need
    no manual reload.
29. As a reader with a mistyped or stale address, I want a clear not-found state, so that I know the
    epic does not exist.
30. As a phone user, I want one column at every width, so that the page reads the same as the Queue.
31. As a desktop user, I want the column capped at 1000px and centered, so that the page matches the
    Queue and ticket pages.
32. As an author running `gx-to-tickets`, I want it to write the `## Summary` of the `-impl` epic, so
    that new epics always have a summary on this page.
33. As an author in an interactive run, I want to see the draft summary in the quiz, so that I can fix
    it before it publishes.

## Implementation Decisions

**Route and entry points**

- The page lives at `#/e/<project:epic>`, next to `#/t/*`. The address holds no slash, but the route
  reads it from the path the same way the ticket route does, so it also accepts any address the list
  page may pass.
- The epic name is a link on an active Queue card and on a Recently finished row. The whole card is
  not a link. One-off rows are tickets and do not link.
- An unknown address, or a 404 from the endpoint, shows a not-found state. The wording is the
  implementer's choice.

**Summary**

- The summary is a `## Summary` section in the epic's `ticket.md` body. There is no schema change and
  no backfill. The page hides the block when the summary is empty.
- `gx-to-tickets` always writes `## Summary` on the `-impl` epic as part of its publish flow, with
  `gx tickets section <project:epic>-impl Summary -`.
- The summary is one paragraph, at most 3 sentences and about 60 words, in B2 English and in
  `GLOSSARY.md` terms. It says what the epic delivers and why, from the user's view. It has no ticket
  numbers, file paths or code names.
- The source is the spec's Problem Statement and Solution, after the review amendments, rewritten and
  not copied. With no spec, it summarizes the plan or the conversation under the same rules.
- With no clear summary, it still writes one, built from whatever the source offers (Problem
  Statement, user stories, ticket titles). It never skips the section, never copies a map Destination,
  and never blocks publishing.
- In an interactive run, the step 5 quiz shows the draft summary for edits. An unattended run
  publishes it as drafted. `gx-design` step 7 needs no change.
- For a map epic, the summary is its Destination.

**Endpoint**

- `GET /v1/epics/detail?address=<project:epic>` returns one payload for loop and map epics, like the
  ticket detail endpoint. An unknown address gives `404`. Without the web cookie it gives `401`, like
  every web route. The status for a missing `address` and the `404` body text are the implementer's
  choice.
- Epic fields are `address`, `kind` (`loop` or `map`), the epic state, `summary`, `done`, `total`,
  `wall_clock`, `agent_time` and `cost_total`. The server computes counts and totals. `kind` is `map`
  when the epic's `ticket.md` frontmatter has `kind: map`, and `loop` otherwise. `wall_clock` runs from
  the first ticket claim to `completed_at`, or to now while the epic runs, and the client ticks it
  while the epic runs. `agent_time` is the sum of the elapsed time of landed tickets. The name of the state
  field is the implementer's choice.
- Rows are a flat list in ticket order. Each row has `address`, `title`, `rendered_status`, `type`,
  `parent`, `elapsed`, `cost`, `claimed_at`, the unresolved blockers (own-epic and cross-epic), and a
  flag for a parked question that needs an answer. The client nests rows by `parent`. Names of any
  extra fields, such as the question text, are the implementer's choice.
- The payload has no token counts anywhere.
- Running tickets: elapsed counts from `claimed_at` and the client ticks it. Cost stays blank until
  the ticket lands, so a live row shows cost only if it has one. `cost_total` and `agent_time` add up
  landed tickets only, and a running epic reports less than it has spent so far.
- Map epics use the same shape with `kind: map`. The summary is the text of the epic's `## Destination`
  section. Rows carry title and status only. Elapsed, cost and totals stay empty, and there is no
  answer flag.
- The route and the payload shape are meant to serve the future epics list page too. This spec
  decides nothing for that page.
- The web mux serves the route. The web allowlist and the "everything else is 404" test list it as
  allowed, and every other server route stays 404. A CLI verb `server epics detail <address>` (with
  `--json`) prints the same payload, for debugging. It sits in the parity table beside
  `server tickets detail`, through a new `apiclient` method.

**Finished state**

- The server adds a `finished` epic state, for an epic with `merged_at` set. An epic with
  `completed_at` but no `merged_at` stays `awaiting-merge`. The existing states stay as they are
  (`awaiting-merge`, `running`, `waiting`, `parked`, `deadlocked`).
- The page prints the server's state as is, in the stat sentence. The badge tone for `finished` is
  the implementer's choice. A map epic shows no state word and no badge.

**Refresh**

- The page refetches on a live event whose address is the epic or one of its tickets. It reuses the existing event connection. There is no timer for refetching.
- The Queue snapshot does not grow.
- The elapsed time of running rows ticks on the client from `claimed_at`. The tick interval is the
  implementer's choice.

**Layout**

- One column at every width, capped at 1000px and centered. The Queue page, the ticket page and the
  epic page use one shared container component. The Queue and ticket pages switch to it, so the
  width is set in one place.
- Top to bottom:
  - The epic name in the condensed heading font. No separate badge row and no project label.
  - The summary, hidden when empty.
  - The progress bar, split by status into done, running (and claimed), needs-answer, waiting
    (blocked, open and draft) and `needs attention` (needs-repair and error, error tone). Cancelled
    joins done. Empty segments are not drawn.
  - One dim stat sentence: `<state> · N of M done · took <wall clock> · <cost> · <agent time> of
    agent time`. It is dimmed. Cost and agent time count landed tickets only. There is no stat grid. N is done plus cancelled and M is every ticket.
    The sentence has no token count. A map epic shows only done and total, with no state word.
  - A native closed `<details>` labelled `✓ N done`, then `· M cancelled` only when M is above 0. N
    counts done tickets only. It is one flat list of done and cancelled tickets, forks included. Each
    row shows its id and title in muted text, then elapsed time and cost dimmer. A cancelled row
    shows a muted `cancelled` word instead of elapsed time, plus the cost if there is one. A fork row
    shows `fork of NN`.
  - Live tickets (everything not done or cancelled) as tinted blocks in ticket order, nested by
    `parent` as described under Nesting.
    Each shows the id in the tone color, the title, and one dim line with the status word, elapsed
    time, plus cost when it has one. A blocked row adds `after 04 and 05`. A row for a fork adds `fork of NN` only when
    its parent is in the fold.
- Rows are plain text. The only link is `Answer →` on a needs-answer row, which goes to the existing
  `#/t/` ticket page. That row also shows its question. No other status shows a question or a link.
- Nesting: live rows nest by `parent` to any depth, with the indent capped at 2 levels. A row indents
  only when its parent is also a live block. A live fork whose parent is in the fold sits at the top
  level.
- Blockers: own-epic blockers are bare numbers. A cross-epic blocker is `epic/NN` with no project
  prefix, no link and no title, so a row reads `after 04 and other-epic/06`.
- Empty epic: the page shows the name, the summary (if any) and one dim line `No tickets yet`. The
  bar, the stat sentence and the fold are hidden. Loop and map epics behave the same.
- Map epic: same layout. Rows show the title and status only, and the stat sentence shows only
  `N of M done`.
- Design reference: the layout prototype on the branch `prototype/web-epic-details-layout` (commit
  `6ae7e6a1`). It still shows token counts. This spec overrides it, so the page has none.
- Names of components, files, CSS and color tokens are the implementer's choice.

## Testing Decisions

- A good test checks behavior from outside: the HTTP response of the endpoint, and what the rendered
  page shows for a given payload. It does not check helper functions or component internals.
- Server seam: the web mux, through the existing web handler test setup. It covers the payload for a
  loop epic and a map epic, the `404` for an unknown address, the `401` without a cookie, `finished` against `awaiting-merge` for `merged_at`, cancelled counted as
  done, and cross-epic blockers in the row data. Prior art is the ticket detail tests in the web
  tests and the web allowlist test.
- Contract seam: the "everything else is 404" test covers the new route.
- Web seam: render the page with a canned detail fetch, through the router like the ticket page tests
  do. It covers the sections above, the empty epic, the map epic, the not-found state, nesting and
  the indent cap, the `fork of NN` cases, the fold label with and without cancelled tickets, the
  `Answer →` link, the Queue, ticket and epic pages all rendering inside the shared container
  component (the 1000px cap itself is not tested), and a refetch when a live event for the epic or one of its tickets arrives. The
  elapsed tick uses fake timers. Prior art is the ticket page and Queue screen tests and the routes
  test.
- Entry links: the Queue screen tests check that an active card and a Recently finished row link to
  the epic page, and that a one-off row does not.
- The `gx-to-tickets` change is a skill text change and has no automated test.

## Out of Scope

- The all-epics list page.
- How the list page reuses the detail route and payload. This spec keeps the route and the payload
  shape open for it and decides nothing.
- The ticket page.
- Actions on the epic page, such as queue, park or launch. The page is read-only.
- A backfill of `## Summary` for existing epics.
- Token counts on the page or in the payload.

## Further Notes

- The generated web types come from the server's wire types, so the new payload types need that
  generation step.
