## Problem Statement

The person running gx cannot see how the work goes over time. How many tickets merged this week?
How many tokens and dollars went into design compared with implementation? Do follow-ups pile up
faster than they get done? How many maps wait for a design? How often do tickets pause, and does
the person end those pauses from the web UI or from a terminal? The answers are spread over ticket
frontmatter, run logs, transcripts and git history. Nothing adds them up per day. Some of the
numbers are not recorded at all: tickets have no creation or cancel time, map work has no token
count, cost covers only the last session of a ticket, and a pause leaves no record of how it ended.

The epic page has the same gap. It shows cost and agent time for landed tickets, but not the lines
of code an epic changed, the tokens it used, how long it took from creation to trunk, or how much
friction it hit. A Design's epic page shows no cost or time at all.

## Solution

A read-only **Stats** page in the web UI, at `#/stats`, and a **Stats** section on the epic page.

The Stats page shows per-day charts for the last 7, 30 or 90 days (30 by default), across all
projects or one project. Both filters live in the URL (`#/stats?range=30&project=gx`). The charts
sit in three sections of chart cards:

- **Throughput**: tickets merged, epics merged to trunk, lines of code, follow-ups, and maps waiting
  for design.
- **Cost and effort**: tokens and spend, each split into Design and Implementation.
- **Friction**: pauses by reason, run events, and pause endings by where they ended.

Each card has a title, a headline (the sum over the range, with the daily average under it) and a
legend. A day whose token or spend data is gone shows a gray hatched "no data" bar, not a zero.

To feed the charts, gx stamps new values in ticket and epic frontmatter when work finishes: creation
and cancel times, the address a ticket was created at, tokens and cost summed over every session,
lines of code on a merged epic, and a friction summary. The Stats page reads only these stamps. A
`gx tickets migrate` step fills in past values where the data still exists.

The epic page gets a Stats section below the tickets. A loop epic shows lines, tokens, cost, lead
time and four friction rows. A Design shows tokens, cost and design lead time. Values that are not
stamped yet are left out.

## User Stories

The Stats page

1. As the person running gx, I want a Stats tab in the top bar after Queue, Epics and Designs, so
   that I can reach the charts from any web page.
2. As the person running gx, I want per-day charts across all my projects, so that I see how the
   whole system is doing.
3. As the person running gx, I want a project dropdown with "All projects" and each project, so that
   I can look at one project alone.
4. As the person running gx, I want a 7d / 30d / 90d range picker with 30 days as the default, so
   that I can zoom in on this week or out to the quarter.
5. As the person running gx, I want the project and range in the URL, so that a reload or a shared
   link shows the same view.
6. As the person running gx, I want the range to end today, so that I see today's work as it grows.
7. As the person running gx, I want days to follow the server's local time zone, so that a day on
   the chart is the same day as a budget day.
8. As the person running gx, I want the charts grouped into Throughput, Cost and effort, and
   Friction sections, so that related charts sit together.
9. As a desktop user, I want two columns of chart cards, so that I see many charts at once.
10. As a phone user, I want one column of chart cards, so that each chart is wide enough to read.
11. As the person running gx, I want each card to show the sum over the range as its headline, so
    that I get the total at a glance.
12. As the person running gx, I want the daily average under the headline, counted over days with
    data only, so that missing days do not pull the average down.
13. As the person running gx, I want the headline to say "partial" when any day in the range has no
    data, so that I do not trust an incomplete total.
14. As the person running gx, I want to hover or tap a bar and see the date and each series' value
    (or "no data"), so that I can read exact numbers.
15. As the person running gx, I want a day with no data drawn as a full-height gray hatched bar, so
    that I can tell missing data from a real zero.
16. As the person running gx, I want a zero day drawn as an empty slot on the baseline, so that a
    quiet day looks quiet.
17. As the person running gx, I want the page to refresh when the server reports changes, so that I
    need no manual reload.

Throughput

18. As the person running gx, I want a "Tickets merged" bar chart that counts done tickets by
    `done_at`, so that I see how much work lands each day.
19. As the person running gx, I want map tickets left out of "Tickets merged", so that design
    decisions do not count as merged work.
20. As the person running gx, I want commitless done tickets (one-offs, investigations) in "Tickets
    merged", so that work with no commits still counts.
21. As the person running gx, I want an "Epics merged to trunk" bar chart by `merged_at`, so that I
    see when whole features reached trunk.
22. As the person running gx, I want a "Lines of code" chart with lines added above the baseline and
    lines removed below it, on the day each epic merged, so that I see how much code changed.
23. As the person running gx, I want the "Lines of code" headline to show the two sums ("+N −M")
    with no average, so that merge-day spikes do not make a meaningless average.
24. As the person running gx, I want a "Follow-ups" chart with "created" and "finished" bars side by
    side, so that I see whether follow-ups pile up.
25. As the person running gx, I want the Follow-ups headline to read "N created · M finished", with
    each daily average on the small line, so that the two numbers are never added together.
26. As the person running gx, I want a ticket to stay a follow-up after triage moves it into another
    epic, so that the chart still counts it when it finishes.
27. As the person running gx, I want a follow-up counted as finished on `done_at` or `cancelled_at`,
    so that a closed follow-up and a fixed one both count.
28. As the person running gx, I want a "Waiting for design" line that counts, at the end of each day,
    the maps that were created and not yet completed, so that I see how much design work waits.
29. As the person running gx, I want draft maps left out of "Waiting for design", so that ideas I set
    aside do not count.
30. As the person running gx, I want the "Waiting for design" headline to show the latest day's
    count and no average, so that the headline is the number waiting now.

Cost and effort

31. As the person running gx, I want a "Tokens" chart with stacked Design and Implementation bars,
    so that I see where the tokens go.
32. As the person running gx, I want tokens to sum input, output, cache writes and cache reads, so
    that the number matches what the model processed.
33. As the person running gx, I want subagent tokens included, so that work an agent hands to a
    subagent still counts.
34. As the person running gx, I want Codex sessions to count toward tokens with Codex's own total,
    so that Codex work is not invisible.
35. As the person running gx, I want a "Spend" chart with stacked Design and Implementation bars, so
    that I see where the money goes.
36. As the person running gx, I want spend summed over every session a ticket used, so that a ticket
    with several sessions shows its full cost.
37. As the person running gx, I want Codex sessions to add nothing to spend, so that the chart never
    shows a made-up price.
38. As the person running gx, I want a ticket whose sessions are all Codex to add $0 to spend and
    never make a day "no data", so that "no data" means only missing transcripts.
39. As the person running gx, I want a day to show "no data" only when it had finished items and none
    of them carries the stamp, so that one old ticket does not hide a day of real data. Finished items
    are done tickets and map epics on their stamp day. Cancelled tickets are not finished items, so a
    day with only cancels is zero.
40. As the person running gx, I want a ticket with no session never to make a day "no data", so that
    a ticket finished by hand does not hide the day.
41. As the person running gx, I want a ticket stamped only when every one of its sessions can be
    read, so that a partial value never looks real.
42. As the person running gx, I want tokens and spend counted on the day they were stamped, so that
    each value lands on one day.
43. As the person running gx, I want the charts to sum ticket stamps only, so that work is never
    counted twice through the epic total.
44. As the person running gx, I want design work done after a map ticket closed counted on the day
    the Design completes, so that no design tokens are lost.
45. As the person running gx, I want unlinked interactive sessions left out, so that the charts
    count only work tied to a ticket.
46. As the person running gx, I want every conversation of a linked interactive session counted, not
    only the latest one, so that a design session that ran `/clear` between runs still counts in full.
47. As the person running gx, I want an interactive session linked to an implementation ticket or a
    one-off to count toward that ticket's stamp, so that hands-on work is not lost.
48. As the person running gx, I want a Design that takes longer than 30 days to still get its tokens and
    cost, so that transcript deletion does not erase long designs.
49. As the person running gx, I want past Designs to show "no data", so that a guessed total never
    looks real.
50. As the person running gx, I want a map or map ticket created before conversation recording began
    to stay unstamped, so that a Design that started before the upgrade does not show an undercount.

Friction

51. As the person running gx, I want a "Pauses" chart stacked by the top 5 reasons in the range plus
    "other", so that I see what stops my tickets.
52. As the person running gx, I want the Pauses tooltip to list every reason, so that "other" hides
    nothing.
53. As the person running gx, I want budget kills to count as the `budget-killed` reason, so that
    budget stops show up as friction.
54. As the person running gx, I want a "Run events" chart stacked by merge conflicts, start-overs,
    manual merges, recovery escalations, rate limits and launch failures, so that I see the problems
    gx hit.
55. As the person running gx, I want a "Pause endings" chart stacked by where each pause ended (web,
    TUI, CLI, gx, unknown), so that I see where I end pauses.
56. As the person running gx, I want the Pause endings headline to show the share of person-ended
    pauses that ended in the web UI ("62% web"), so that I can track the goal of ending every pause
    from the web.
57. As the person running gx, I want a median time-to-resume line under that headline, over
    person-ended pauses only, so that I see how fast I respond.
58. As the person running gx, I want friction counted on the day the ticket finished or was
    cancelled, so that the page reads stamps only and stays fast.
59. As the person running gx, I want friction cards never to show "no data", so that a quiet day reads
    as zero.
60. As the person running gx, I want old pauses with no recorded ending to show as "unknown", so that
    old data is not guessed.

The epic page

61. As a reader of a loop epic, I want a Stats section below the tickets, so that the top of the page
    stays as it is.
62. As a reader of a merged loop epic, I want lines added and removed, total tokens, cost and lead
    time (created to merged to trunk), so that I see what the epic took.
63. As a reader of a merged loop epic, I want "Pauses N" with the top reasons as a dim note, "Run
    events N" with counts by type as a note, "Paused time" and "Pause endings N% web", so that I see
    how smooth the run was.
64. As a reader of an unmerged loop epic, I want only "lead time 3d so far", so that the page shows no
    empty rows.
65. As a reader of a Design, I want total tokens, cost and design lead time (map created to completed),
    so that I see what the design took.
66. As a reader of a Design that is not completed, I want only its design lead time so far, so that
    the page shows no empty rows.
67. As a reader, I want cost left out when any session of the epic ran on Codex, so that the page
    never shows a cost that misses part of the work.
68. As a reader, I want every row that has no stamp left out, so that the section never shows a dash.

Data and migration

69. As the person running gx, I want every new ticket stamped with `created_at`, so that charts can
    count when tickets were created.
70. As the person running gx, I want every ticket to record the address it was created at, so that a
    follow-up stays a follow-up after it moves.
71. As the person running gx, I want every cancelled ticket stamped with `cancelled_at`, so that a
    closed follow-up has a finish day.
72. As the person running gx, I want every pause to record where and how it ended, repair pauses
    included, so that the Pause endings chart has data.
73. As the person running gx, I want `gx tickets set <project:epic> --completed-at` to complete a map
    by hand, so that a map built under another name, or dropped, stops waiting for design.
74. As the person running gx, I want loop epics to keep refusing `--completed-at`, so that only the
    server completes a loop epic.
75. As the person running gx, I want one idempotent `gx tickets migrate` step to fill in past values,
    so that the charts have history from the first day.
76. As the person running gx, I want the migrate step never to clear or lower a stamped value, so that
    running it again is safe.
77. As the person running gx, I want the release notes to tell me to run the migrate step soon, so
    that transcripts do not age out before the backfill reads them.
78. As the person running gx, I want the release notes to say that tickets older than the ticket store
    get the store's start day as their creation day, so that I understand the spike on that day.
79. As the person running gx, I want the Stats page to load in under a second on a store ten times
    today's size, so that it stays usable as history grows.
80. As a script author, I want `gx server stats --json` to print the same data as the page, so that I
    can read the stats without a browser.

## Implementation Decisions

**Route, nav and filters**

- The page lives at `#/stats`. The query holds `range` (7, 30 or 90; default 30) and `project` (a
  project name; absent means "All projects"). How the page handles a bad value in the URL is the
  implementer's choice.
- Stats is a top-bar tab. The tab order is Queue, Epics, Designs, Stats. Until the Designs tab exists
  (`docs/specs/webui-design.md`), Stats sits after Epics.
- The project dropdown lists every project in the ticket store. Its order is the implementer's
  choice.
- The page is read-only. It has no write and no alert.

**Endpoint and CLI verb**

- A new `GET` route returns the stats for one range and one project filter. Per ADR 0028 it comes
  with a `gx server stats` verb with `--json`, in the route table and the parity test. The web mux
  allows the route, with the same cookie auth as every other web route. The route and field names,
  the payload shape, and whether the server or the client builds the day buckets are the
  implementer's choice.
- The server computes the stats from stamped frontmatter only, never from run logs or ticket logs
  (ADR 0041). It reads every project's epics, including `.archive/`, `follow-ups-archived` and
  `one-offs-archived`.
- The implementer chooses how to compute it: a frontmatter scan per request, or a per-file cache
  keyed on modification time. There is no precomputed daily table. Target: the page loads in under
  1 second on a store with 10 times today's ~2,600 ticket files.
- Days use the server's local time zone. The range ends with today, which is a partial day.
- The page refetches on server events, debounced like the Epics page. The debounce interval is the
  implementer's choice, and it may be longer than the Epics page's 500 ms because one stats fetch can
  take close to a second. Loading, error and offline states follow the Epics page.

**Layout** (from the approved prototype, branch `prototype/webui-stats-layout`, commit `f49cea56`,
never merged)

- One section per group, in this order: Throughput, Cost and effort, Friction. Each chart is a card
  with a title, a headline and a legend. Two columns on desktop, one on a phone. The page uses the
  shared page container.
- Chart types:
  - Tickets merged, epics merged: bars.
  - Lines of code: added above the baseline, removed below it.
  - Follow-ups: "created" and "finished" bars side by side.
  - Waiting for design: a line.
  - Tokens and spend: stacked bars, Design under Implementation.
  - Pauses, run events, pause endings: stacked bars, one color per series.
- Headline: the sum over the range, with the daily average on a small line under it ("267",
  "8.9/day"). The average counts only days with data. Exceptions:
  - Waiting for design shows the latest day's count and no average.
  - Lines of code shows "+N −M" and no average.
  - Follow-ups shows "N created · M finished", with both daily averages on the small line.
  - Pause endings shows the share of person-ended pauses that ended in the web UI ("62% web"), and
    the median time-to-resume on the small line.
  - The headline adds "partial" when any day in the range has no data.
- No data: a full-height gray hatched bar, with "no data" in the tooltip. A zero day is an empty slot
  on the baseline. Only the tokens and spend cards can have "no data" days.
- Hover or tap shows a tooltip with the date and each series' value, or "no data".
- The chart library or hand-written SVG, the colors (theme tokens only, as the design-system guard
  test requires), number formats and axis labels are the implementer's choice.

**Throughput counting rules**

- Tickets merged: tickets with `status: done` outside map epics, counted on `done_at`. Commitless
  tickets count. Epic `ticket.md` files are not tickets here. A done ticket with no `done_at` is left
  out.
- Epics merged to trunk: epics counted on `merged_at`.
- Lines of code: `lines_added` and `lines_removed` from the epic `ticket.md`, counted on `merged_at`.
- Follow-ups: a ticket is a follow-up when its original address (see below) is in a `follow-ups`
  epic, wherever it lives now. `category` is not used. "created" counts on `created_at`. "finished"
  counts on `done_at` or `cancelled_at`. A move does not finish a follow-up.
- Waiting for design: for each day, the number of map epics whose `created_at` is on or before the
  end of that day and whose `completed_at` is unset or after the end of that day. A map whose current
  status is `draft` is left out of every day.

**Cost and effort counting rules**

- The charts sum ticket stamps only. Epic stamps feed only the epic page.
- Design is map epics and their tickets. Implementation is everything else: loop epics, follow-ups,
  one-offs, code-review, conflict-resolution, merge and investigate tickets.
- A ticket's `total_tokens` and `actual_cost` count on the day they were stamped: `done_at` for a
  ticket. A map epic's remainder (its `total_tokens` and `actual_cost` minus the sum of its tickets'
  stamps) counts as Design on the map epic's stamp day. The remainder holds design work done after a
  map ticket closed and conversations linked to the map epic itself.
- No data: a day shows "no data" on the tokens or spend chart only when it had finished items and
  none of them carries that chart's stamp. A day with no finished items is zero. The rule applies per
  chart, not per series. Finished items are done tickets (on `done_at`) and map epics (on their stamp
  day). Cancelled tickets never get tokens or cost, so they are not finished items.
- An item with no session gets a stamp of zero, so it never makes a day "no data". This holds for live
  stamping. The backfill treats a done ticket with no `session_ids` differently (see the backfill). An item that never
  got a stamp (finished before stamping began, transcripts gone, or a session that could not be read)
  is unstamped. A zero stamp must read differently from no stamp. Today `actual_cost` drops out of
  the file when it is zero, so the implementer adds a way to tell them apart.
- Spend: a ticket whose sessions all ran on Codex stamps `actual_cost` as zero. A mixed ticket stamps
  only its Claude cost.

**Friction counting rules**

- Each card reads the friction summary stamped on tickets and counts it on the ticket's `done_at` or
  `cancelled_at`. A ticket that is still paused shows nothing until it finishes. The Queue tab still
  shows it live.
- Pauses: pause spells per reason. With Pause rounds, one round is one spell. For old data, a spell
  starts at the first `paused` line after the ticket last started an iteration, so repeated old
  `needs-answer` lines count once. The card stacks the top 5 reasons in the range (after the filter)
  plus "other". The tooltip lists every reason. Budget kills count as `budget-killed`. An old line
  with no `kind` counts as "unknown". Legacy names map through `events.ReadLegacy`.
- Run events: merge conflicts (`conflict-hit`), start-overs, manual merges, recovery escalations,
  rate limits (`paused-rate-limit`) and launch failures. Successful auto-recovery, nudges, deadlocks,
  smart-zone pauses, late-finish adoptions and relaunches are left out. Run events that name no ticket
  are not counted. The mapping from event names, old and new, to these series is the implementer's
  choice.
- Pause endings: pause rounds by where they ended: web, TUI, CLI, gx (no person), or unknown. The
  "% web" headline is web divided by web plus TUI plus CLI. The median time-to-resume covers the same
  person-ended pauses, timed from pause start to pause end.
- Early run logs at `<store>/<epic>/run-log.jsonl` (no project directory) count under the `gx`
  project when the migrate step reads them.

**New ticket frontmatter** (all read-only, gx-managed, listed by `gx tickets schema`)

- `created_at`: written by every path that creates a ticket: `tickets add`, `follow-up add`, forks,
  fix tickets a code-review ticket opens, one-off submit, merge and investigate tickets, and any other
  creation path. Epics already have it.
- Original address: the address the ticket was created at, written in the same places as
  `created_at`. `move` and `close` never change it. The field name is the implementer's choice.
  It replaces the `- original: <addr>` Resolution line as the way to learn a ticket's first address.
  Whether that line stays is the implementer's choice.
- `cancelled_at`: written whenever a ticket becomes `cancelled` (`gx server tickets cancel` and its
  descendants, `gx tickets close`, and every other cancel path).
- `total_tokens`: input plus output plus cache writes plus cache reads, over every session of the
  ticket, subagent transcripts (`<session>/subagents/*.jsonl`) included, read in the same pass as
  `actual_cost`. A Codex session adds its own reported total.
- `actual_cost`: changes from the last session only to the sum over every session. This also changes
  the cost the queue view and the ticket page show.
- A friction summary: pauses by reason, run events by type, pause endings by source, total paused
  time, and the durations of person-ended pauses, so the page can compute the median. Stamped when the
  ticket becomes `done` or `cancelled`, computed from its run-log and ticket-log lines. A ticket that
  becomes done again after a new claim gets a new stamp. The YAML shape is the implementer's choice.
- The implementer picks a marker that says a ticket had a Codex session, so the epic page can leave
  out cost.

**When tickets are stamped**

- Implementation tickets stamp `total_tokens` and `actual_cost` when they merge. Commitless tickets
  stamp when they become done. The stamp uses every session in `session_ids`, plus every conversation
  of a linked interactive session recorded against the ticket. A start-over clears
  `session_ids`, so a discarded attempt never counts. Cancelled implementation tickets get no tokens
  or cost.
- A conflict-resolution child records its session in `session_ids`, like every other agent ticket.
  Today the forked child never calls `AppendSessionID`, so none of the 59 done conflict-resolution
  tickets has one. That changes.
- A ticket is stamped only when every one of its sessions can be read. Otherwise it stays unstamped
  and its day can show "no data". This rule holds at merge and in the backfill.
- Map tickets stamp `total_tokens` and `actual_cost` when they become done, from the saved totals of
  the conversations recorded against them. A cancelled map ticket gets no stamp, and the map epic's stamp covers its
  conversations.
- A map epic stamps `total_tokens` and `actual_cost` at `-impl` creation, or at `completed_at` if a
  person writes that first. The stamp sums the saved totals of every conversation recorded against the
  map epic or any of its tickets. It never reads old transcripts, so a Design longer than 30 days still
  stamps.
- A map epic or map ticket gets these stamps only when its `created_at` is after gx began recording
  conversations. Older ones stay unstamped and show "no data". How gx records when recording began is
  the implementer's choice.

**Conversation records for interactive sessions**

- gx records every conversation an interactive session reports (conversation id, dir, harness), not
  only the newest one. This holds for a session linked to any ticket: a map epic, a map ticket, an
  implementation ticket or a one-off. The record outlives session removal. Storage is the
  implementer's choice.
- A conversation counts toward one Session ticket: the latest one the session had while the
  conversation ran. If gx-design links the session to a second map ticket before `/clear`, the second
  ticket gets the whole conversation.
- When a conversation ends (a new conversation starts in the session, or the session ends), gx reads
  its transcript, subagents included, and saves its `total_tokens` and cost in the record. Stamps sum
  these saved totals and never re-read the transcript. A conversation whose transcript could not be
  read has no saved totals, and the ticket that holds it stays unstamped. How gx detects the end is
  the implementer's choice, as long as it reads the transcript before Claude's 30-day deletion.
- Interactive sessions with no Session ticket are not recorded for stats and never count.

**Pause rounds**

- Every pause opens a Pause round in the ticket log, repair pauses too. A repair round has no
  question.
- Every way a pause ends writes the round's end with a `source` (web, tui, cli, gx) and a `how`
  (answer, resume, start-over, manual merge, relaunch, cancel, auto). Today a repair resume leaves no
  trace. That changes.
- How the server learns whether a request came from the web, the TUI or the CLI is the implementer's
  choice.

**Epic stamps**

- When a loop epic merges to trunk, gx stamps on its `ticket.md`:
  - `lines_added` and `lines_removed`, from trunk before the fast-forward to trunk after it.
  - `total_tokens` and `actual_cost`, the sums of its tickets' stamps, but only when every done
    ticket in the epic is stamped. Otherwise the epic stays unstamped and the epic page leaves the
    rows out, so it never shows an undercount.
  - A friction summary, the sum of its tickets' summaries.
- An epic merged by hand gets its lines from the `Ralph-Loop-Ticket` trailers of its commits.
- A map epic's stamps are described under "When tickets are stamped".

**Completing a map by hand**

- `gx tickets set <project:epic> --completed-at <time>` writes `completed_at` on a map epic. It still
  refuses on a loop epic with `CodeServerStamped`. The value format and whether `now` is accepted are
  the implementer's choice.
- Waiting for design ends at `completed_at`, whether `-impl` creation or a person wrote it. Design lead
  time and the map epic's stamp use the same moment.
- `webui-design.md` assumes a person can complete a map by hand. This change gives it that path. That
  spec is not edited here.

**Epic page**

- The epic detail payload gains the stamped values, and the page gets a "Stats" section below the
  tickets. Each row has a label, a value and an optional dim note. The top of the page does not change.
  Field names are the implementer's choice.
- Loop epic rows, each left out when unstamped:
  - Lines: "+3,412 −1,208".
  - Tokens.
  - Cost, left out when any session of the epic ran on Codex.
  - Lead time, from the epic's `created_at` to `merged_at`, with the note "created → merged to trunk".
    Before the merge it reads "3d so far".
  - "Pauses N", with the top reasons as a dim note.
  - "Run events N", with counts by type as a note.
  - "Paused time".
  - "Pause endings N% web".
- Design (map epic) rows: tokens, cost (left out under the same Codex rule), and design lead time from
  the map's `created_at` to its stamp moment (`-impl` creation or `completed_at`, whichever came
  first). Before that it reads "so far".
- This amends `web-epic-details.md`: the payload and page now carry token counts, and a map epic page
  shows cost and time in the Stats section.

**Backfill: one `gx tickets migrate` step**

- One idempotent step, also reachable through `gx server restart --migrate`. It reads transcripts, the
  run logs, the ticket logs, the code repos' git history and the ticket store's git history. It never
  clears or lowers a value that is already stamped. It has a dry run like the other steps.
- `created_at` and the original address: from the store's git history, using the first commit and the
  first path of each ticket file. Tickets that existed before the store get the store's first commit
  day (2026-10-06). The release notes say so.
- `cancelled_at`: from the store's git history, the commit that set `cancelled`.
- Lines of code for past merged epics: from git, using the `Ralph-Loop-Ticket` trailers.
- `total_tokens` and `actual_cost` for implementation tickets: only where every session's transcript
  can still be read. A done ticket with no `session_ids` stays unstamped, not zero, unless it is
  commitless or was done by hand (about 50 older tickets, plus past conflict-resolution tickets, never
  recorded their sessions). Claude deletes transcripts after 30 days by default. An old single-session
  `actual_cost` is rewritten only when every session can be read. Where transcripts are gone, the
  ticket stays unstamped.
- `total_tokens` and `actual_cost` for past merged loop epics: the sums of their tickets' stamps,
  under the same every-done-ticket-stamped rule as at merge.
- Design tokens and cost are not backfilled.
- Friction summaries for past done and cancelled tickets, and for merged loop epics, from the run
  logs. Pause endings in old data are "unknown".
- How the step finds a transcript whose worktree is gone is the implementer's choice.
- The release notes say to run the step soon after upgrading, before more transcripts age out.

## Testing Decisions

- A good test checks behavior from outside: the HTTP response of a route, the frontmatter a lifecycle
  step leaves on disk, what a migrate run writes, and what the page renders for a given payload. It
  does not test helper functions, the cache or component internals.
- Stats endpoint (main seam): the web mux with a canned ticket store, through the existing web handler
  test setup. Table tests cover each counting rule: tickets merged without map tickets and with
  commitless tickets, epics merged, lines on the merge day, follow-ups by original address after a
  move, "finished" on `done_at` and `cancelled_at`, waiting for design with drafts and `completed_at`,
  Design and Implementation split, the map epic remainder on its stamp day, the "no data" rule (mixed
  day, day with no finished items, day with only cancels, ticket with no session, all-Codex ticket,
  unstamped pre-recording map), friction by stamp day, top-5-plus-other, "% web" and the median over person-ended pauses only,
  the project filter, the 7/30/90 ranges, and local-time day buckets. It also covers the `401`
  without a cookie and the allowlist. Prior art: the epic detail and epic list tests in the web tests
  and the web allowlist test.
- CLI parity: the route-table test covers the new route and `gx server stats`.
- Stamping: through the server lifecycle in the `servertest` harness and the ralphloop merge tests,
  with fake transcripts. Cases: `created_at` and the original address on each creation path, `move`
  keeping the original address, `cancelled_at` on each cancel path, tokens and cost summed over every
  session including subagents and Codex, no stamp when one session cannot be read, the friction
  summary at done and at cancel, epic lines, tokens, cost and friction at merge, and map ticket and map
  epic stamps (at `-impl` creation, at a hand-written `completed_at`, and none for a map created before
  recording began), a conflict-resolution child recording its session, and no epic token or cost
  stamp when one done ticket is unstamped. Prior art: the existing merge-metrics tests and epic time backfill tests.
- Pause rounds: each way a pause ends writes `source` and `how`. A repair resume is covered. Prior art:
  the existing ticket log and answer tests.
- Conversation records: an interactive session that reports two conversations keeps both, and the
  record outlives session removal. A conversation that spans two Session tickets counts toward the
  latest. Totals are saved when a conversation ends, and a map epic stamps from saved totals after the
  transcript is deleted. A session linked to an implementation ticket adds to that ticket's stamp.
  Prior art: the server session tests.
- `gx tickets set --completed-at`: writes on a map epic and refuses on a loop epic. Prior art:
  `TestTicketsSet_EpicAddressRefusesServerStampedFields`, which changes to cover the map case.
- Migrate step: a fixture store with git history, run logs and fake transcripts. Covers each backfill,
  a rerun that changes nothing, a stamp that is never lowered, the store import day, tickets whose
  transcripts are gone, a done ticket with no `session_ids` (unstamped unless commitless or done by
  hand), and past epic totals. Prior art: the existing migrate tests.
- Performance: a benchmark over a generated store of about 26,000 ticket files checks the 1-second
  target. Whether it runs in CI is the implementer's choice.
- Web: Vitest renders the Stats page with a canned stats fetch, through the router like the epic page
  tests. Covers the three sections, each card's headline rules, hatched "no data" bars, "partial", the
  tooltip, URL filters, the tab order, and a debounced refetch on a server event. The epic page tests
  cover the Stats section rows for a merged loop epic, an unmerged one, a Design with and without a
  stamp, and the Codex rule for cost. Prior art: the epic page, Epics page and routes tests.

## Out of Scope

- Flow-time charts (design lead time, build lead time, cycle time) on the Stats page.
- Follow-up quality charts (close reasons, time open).
- Renaming `gx tickets close --reason fixed`, which writes `cancelled`.
- Codex pricing. Codex sessions add nothing to spend.
- A backfill of Design tokens and cost.
- Writes, alerts and budget checks on the Stats page. The budget ledger stays the source for budget
  limits only.
- Stats in the TUI.
- Token counts on the ticket page. The ticket page only picks up the new all-session cost.
- Editing `webui-design.md`. It gains the hand-completion path from this spec.

## Further Notes

- Accepted costs of reading stamps only (ADR 0041): spend on cancelled and started-over tickets never
  shows, chart totals do not match the budget ledger, and friction shows on the day a ticket finishes,
  not the day it happened.
- The generated web types come from the server's wire types, so the new payload types need that
  generation step.
- Decisions behind this spec are on the map epic `gx:webui-stats`, tickets 01 to 04 and 14 to 17, and
  its Review findings.
