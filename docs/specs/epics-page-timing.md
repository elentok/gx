# Epics page timing

Epic: `gx:epics-page-timing`

## Problem Statement

The Epics page shows what each card is doing, but not for how long. A person who looks at the page
cannot tell a run that started five minutes ago from one that has run for two days. They cannot tell
how long a question has waited on them, how old a not-started Design is, or when an epic merged. To
find out, they open the card and read stamps on the epic page, or they guess.

## Solution

Each Epic card gets one time label on the line under its title, between the project and the status
words:

`gx · 2h 5m · Implementing...`

The label depends on the card's section:

| Section | Label | Counts from |
| --- | --- | --- |
| Needs you | `waiting 7m` | the start of the wait the section already sorts by |
| In progress | `2h 5m` | when the work started (see below) |
| Ready to queue | `created 3d ago` | when the epic was created |
| Ready to design | `created 3d ago` | when the epic or Design was created |
| Finished | `5h ago` | when the card merged, else when it finished |

In progress counts from a different start for each kind of card:

- A running, merging or implemented loop epic counts from the epic's first claim (`started_at`).
- A queued epic or one-off counts from its current enqueue.
- A Design counts from its first design session ever, whether a session is live or stopped.
- A running one-off counts from its ticket's claim.

Durations have at most two units:

| Age | Format | Example |
| --- | --- | --- |
| under 1 minute | `<1m` | `<1m`, `created <1m ago` |
| under 1 hour | minutes | `45m` |
| under 1 day | hours and minutes | `2h 5m`, `3h` |
| under 7 days | days and hours | `3d 4h`, `3d` |
| 7 to 29 days | weeks and days | `3w 2d`, `2w` |
| 30 days or more | weeks only | `5w` |

The labels tick while the page is open. A card with no known start shows no label, and its line
reads `project · status` as it does today.

## User Stories

1. As a person on the Epics page, I want each card to show how long it has been in its state, so that I can see stale work at a glance.
2. As a person, I want the time on the line under the title, between the project and the status words, so that the card gains no new row.
3. As a person, I want a Needs-you card to read `waiting 7m`, so that I can see which question or pause has waited longest.
4. As a person, I want a Needs-you card's wait to count from the same start the section sorts by, so that the oldest wait sits first and shows the largest time.
5. As a person, I want a running loop epic to show the time since its first ticket was claimed, so that I can see how long the implementation has taken so far.
6. As a person, I want a merging or implemented epic in In progress to keep counting from its first claim, so that the time reads as the whole implementation time.
7. As a person, I want a queued epic to show how long it has waited in the queue since its current enqueue, so that a remove and re-add starts the count again.
8. As a person, I want a queued one-off to show its own queue time, so that one-offs read like epics.
9. As a person, I want a queue entry written before this change to fall back to the epic's `queued_at`, so that an old queued epic still shows a time.
10. As a person, I want a running one-off to show the time since its ticket was claimed, so that I can see a long-running one-off.
11. As a person, I want a Design in In progress to show the time since its first design session opened, so that I can see how long the Design has been in progress, across sessions.
12. As a person, I want a stopped Design (Continue) to keep the same count, so that stopping a session does not hide the Design's age.
13. As a person, I want a Design that has no design-start stamp to show no time, so that the card never shows a wrong time.
14. As a person, I want a joined card to show the time of the part it shows (the Design until Branch, the `-impl` epic from Branch on), so that the time matches the status words.
15. As a person, I want a Ready-to-queue card to read `created 3d ago`, so that I can see how long it has sat unqueued.
16. As a person, I want a joined card in Ready to queue to count from the `-impl` epic's creation, so that the time shows how long the implementation has waited, not the Design's age.
17. As a person, I want a not-started Design, a draft epic and an epic with no open tickets in Ready to design to read `created 3d ago`, so that I can see old ideas.
18. As a person, I want the Follow-ups card to show no time, so that an inbox with many ages does not show one misleading number.
19. As a person, I want a Finished card to read `5h ago`, so that I can see what landed recently without the status word repeating "merged".
20. As a person, I want a finished Design ("Designed") and a finished one-off to count from their completion, so that every Finished card has a time.
21. As a person, I want durations with at most two units in the agreed format, so that the label stays short.
22. As a person, I want a duration under a minute to read `<1m`, so that a fresh card never shows `0m`.
23. As a person, I want a start time in the future (clock skew) to read `<1m`, so that I never see a negative or odd time.
24. As a person, I want the labels to tick while the page stays open, so that a time does not freeze until the next server event.
25. As a person, I want the time to stay right after a reload that repaints from the page cache, so that a cached card does not show an old time.
26. As a person, I want to hover a label and see the exact start date and time, so that I can check the moment without opening the card.
27. As a person on a phone, I want the line to wrap between parts and never inside the time label, so that the label stays readable at 375 px.
28. As a person, I want a card without a known start to read `project · status` with no extra dot, so that missing data leaves no gap.
29. As an implementer, I want the server to choose the start time and its kind for each card, so that the web client does not derive sections or states.

## Implementation Decisions

- **One time field pair on the card row.** The `/v1/epics` row gains two card fields: a time kind and
  a start time. The kind is one of four literal strings (elapsed, waiting, created, finished). Both
  are empty when the card has no known start. The implementer picks the field names. The kind
  values are string literals in `server/`, so tygo emits them as constants.
- **The server picks the start.** The card-building code that sets section and status words also
  sets the time pair, from the rules in the Solution table. The client only formats and ticks.
- **Needs you** uses the row's existing `waiting_since`, kind waiting.
- **In progress, loop epics.** Running, merging and implemented (auto-merge on) epics use the epic's
  `started_at`, kind elapsed. No `started_at` means no time.
- **In progress, queued.** A queued epic uses the earliest enqueue time among its items still in the
  server queue. When none of its items has an enqueue time, it falls back to the row's `queued_at`.
  A queued one-off uses its own item's enqueue time and has no fallback: the `one-offs` epic's
  `queued_at` belongs to all one-offs, so it says nothing about this one.
- **In progress, one-offs.** A running one-off uses its ticket's `claimed_at`.
- **In progress, Designs.** A live, counting-down, creating-tickets or stopped Design uses the map's
  new design-start stamp. No stamp means no time.
- **Ready to queue** uses `created_at`, kind created. A joined card uses the `-impl` epic's
  `created_at`.
- **Ready to design** uses `created_at` for not-started Designs, draft epics and epics with no open
  tickets. The Follow-ups card gets no time.
- **Finished** uses the existing `finishedAt` rule (merge, else completion), kind finished. This
  covers merged loop epics, finished Designs, joined cards and finished one-offs.
- **Joined cards.** The time follows the part the card shows. Today the join copies the `-impl` row,
  then puts back the map's timestamps. The time pair must come from the `-impl` row from Branch on,
  and from the Design question when the Design wins the card for Needs you.
- **Queue item enqueue time.** Each server-queue item gains an enqueue time, stored in the queue
  state file. The server sets it to now on add, on the merge ticket's auto-enqueue, and for each new
  address in a replace. A replace keeps the time of an address that was already queued. A move keeps
  it. The server ignores any enqueue time a client sends. Items from an older queue file load with no
  time. There is no backfill.
- **Design-start stamp.** A map epic gains a server-stamped frontmatter field: when its first design
  session opened. The server writes it when it creates a session whose Session ticket is the map or
  one of its tickets, and never overwrites it. This covers opening a Design, New Design, Continue and
  auto-advance. The field joins the list of server-stamped epic fields, and the tickets loader reads
  it. Maps designed before this change get no stamp and show no time while in progress. The
  implementer picks the field name and the exact hook.
- **Web formatting.** A new pure formatter turns milliseconds into the duration format in the
  Solution table. It floors each unit and drops a zero second unit (`3h`, not `3h 0m`). A negative
  age counts as zero. The epic page's existing seconds formatter stays as it is.
- **Label words.** elapsed reads `<duration>`, waiting reads `waiting <duration>`, created reads
  `created <duration> ago`, finished reads `<duration> ago`.
- **Card line.** `EpicCard` renders `project · time · status`. The time span uses the dim text
  color, like the project, carries `data-testid="time"`, does not wrap inside itself, and has a
  native `title` with the start as a local date and time. No time means no span and no second dot.
- **Ticking.** The Epics page runs one page-wide clock that ticks every 30 seconds and passes "now"
  to the cards. It runs only while the page is mounted and at least one card has a time. Cards keep
  no timer of their own.
- **Page cache.** The cached rows carry the start stamps, not formatted text, so a repaint from the
  page cache computes fresh labels.
- **Sorting does not change.**
- **Glossary.** The Epic card entry's "project · status" line becomes "project · time · status".

## Testing Decisions

- Test behavior through the highest seams: the `/v1/epics` payload for the server, the rendered
  card for the web client. Do not test helpers that the payload test already covers.
- **Server, `/v1/epics` payload.** Extend the epic-list card tests (prior art: the loop-card,
  design-card, joined-card and section-sort tests). Each case builds a fixture with
  `servertest.WriteTicketWith`, calls the list, and checks the time kind and start per card. Cover
  every section and state: running, merging, implemented, queued epic, queued one-off, old queue
  entry fallback, running one-off, live and stopped Design with and without the stamp, joined card
  before and after Branch, joined Design question, Ready to queue (plain and joined), not-started
  Design, draft, no open tickets, Follow-ups, each Finished kind.
- **Server, queue.** Extend the queue and queue-replace tests: add sets the enqueue time, remove and
  re-add resets it, move keeps it, replace keeps it for kept addresses, and an old queue file loads.
- **Server, design stamp.** Opening a Design writes the stamp once. A second session leaves it
  unchanged. New Design writes it.
- **Tickets loader.** The new map field round-trips through load and stamp.
- **Web formatter.** Table test of every boundary: 59 s, 1 m, 59 m, 1 h, 2 h 0 m, 23 h 59 m, 1 d,
  6 d 23 h, 7 d, 29 d, 30 d, and a negative age.
- **Web card.** Extend `EpicCard.test.tsx` with a fixed "now": each kind's words, no span and no
  extra dot without a time, the `title`, and a tick that moves the label (fake timers).
- **e2e.** One new flow on the `united` scenario checks the labels in a real browser with regular
  expressions, since fixture times are relative to the run.

## Verification

Run against a temporary gx server built from the epic. New flow to write: **61. Card times**
(scenario `united`, in `united.e2e.ts`). Its steps open the Epics page and expand every section. The
`united` scenario must give `design-live` and `design-stopped` a design-start stamp in
`unitedMeta`, and `running-epic` a `started_at`. That scenario change is an implementation ticket.

1. Flow 61: every card except `follow-ups` has a `time` element. Flow 16 still passes.
2. Flow 61: the card line text matches `^demo · .+ · .+$` (or `auto · ...`) and the time sits between
   the project and the status. Flow 23 still passes.
3. Flow 61: each Needs-you card's time matches `^waiting (<1m|\d+m)$`.
4. `gx server epics --json`: the Needs-you rows' start equals their `waiting_since`.
5. Flow 61: `running-epic` matches a bare duration. `gx server epics --json`: its start equals its
   `started_at`.
6. `gx server epics --json`: `awaiting-auto-epic` (In progress, "Implemented") has kind elapsed when
   it has a `started_at`, and no time when it has none.
7. Flow 61: `queued-epic` shows a bare duration. Then run `gx server queue remove demo:queued-epic/01`
   and `gx server queue add demo:queued-epic/01`. `gx server queue list --json` shows a newer enqueue time, and the card
   reads `<1m`.
8. Flow 61: the queued one-off `demo:one-offs/02` shows a bare duration.
9. Server test only (old queue file). No browser check.
10. Server test only (running one-off). `united` has no running one-off.
11. Flow 61: `design-live` shows a bare duration. `gx server epics --json`: its start equals the
    stamp.
12. Flow 61: `design-stopped` (In progress, Continue) shows a bare duration. Flow 42 still passes.
13. Server test only (Design without the stamp).
14. Flow 18 extended: the joined card shows a bare duration while Designing, `created ... ago` at
    Branch, and a bare duration once queued.
15. Flow 61: `ready-epic` matches `^created (<1m|\d+m) ago$`.
16. Flow 18 extended (the Branch step above). `gx server epics --json`: the start equals the
    `design-epic-impl` epic's `created_at`.
17. Flow 61: `design-new`, `draft-epic` and `idle-epic` match `^created .+ ago$`.
18. Flow 61 and flow 19: the `follow-ups` card has no `time` element and its line reads
    `demo · 1 bug, 1 idea`.
19. Flow 61: `finished-a` matches `^\d+w ago$`.
20. Flow 61: the finished one-offs match `^.+ ago$`. Server test covers a finished Design.
21. Web formatter test (all boundaries). No browser check.
22. Web formatter test and flow 61 (fresh cards read `<1m`).
23. Web formatter test (negative age).
24. Web card test with fake timers. No browser check: a real wait of a minute would slow the suite.
25. Flow 61: reload is not allowed in e2e (the suite fails on reload), so this is a web test: a page
    rendered from a cached payload with a later "now" shows the later time.
26. Flow 61: the `ready-epic` time element has a non-empty `title` attribute.
27. Flow 23 extended at 375 px: the time element's height is one line.
28. Web card test: a row with no time renders `project · status`.
29. Server payload tests. No browser check.

## Self-decided design questions

1. **Where does the label go in the line?** As the person asked, between the project and the status:
   `project · time · status`. The status words then name what the time measures.
2. **What words does each section use?** In progress shows a bare duration, because the status words
   ("Implementing...", "Queued #2", "Designing") already say what it counts. Needs you says
   `waiting`, because its status words do not mention the wait. Ready sections say
   `created ... ago`. Finished says only `... ago`, so the line does not read "merged 5h ago ·
   Merged".
3. **When does a running epic start?** At `started_at`, the epic's first claim. This matches the
   Design's "first session ever" rule, and gx already stamps it. Paused time counts, since
   active-time accounting is out of scope.
4. **What do merging and implemented cards in In progress count?** The same `started_at`. There is no
   merge-start stamp, and a short merge does not need its own clock.
5. **What does a queued epic with several queue items count from?** The earliest enqueue among its
   items still in the queue. That is how long the epic has had a place in the queue.
6. **What does a replace do to enqueue times?** It keeps the time of an address that was already
   queued and stamps now for new ones. A replace reorders work. It does not re-queue it.
7. **Does an old one-off queue entry fall back to `queued_at`?** No. The `one-offs` epic's
   `queued_at` is shared by every one-off, so the time would be wrong. It shows no time.
8. **What does a running one-off count from?** Its ticket's `claimed_at`. A one-off has no epic of
   its own.
9. **Which creation time does a joined card use in Ready to queue?** The `-impl` epic's
   `created_at`. The card shows the implementation, and the question is how long it has waited to be
   queued.
10. **Does the Follow-ups card get a time?** No. It is an inbox of tickets with many ages, and one
    number would mislead.
11. **What shows under a minute?** `<1m`, so a fresh card never reads `0m`.
12. **How are units rounded?** Each unit is floored, and a zero second unit is dropped (`3h`,
    `2w`). Flooring never shows more time than has passed.
13. **What about clock skew?** A start in the future counts as zero and reads `<1m`. There is no
    server-clock correction: the browser usually runs on the server's machine, and labels are
    minute-sized.
14. **Does the label tick?** Yes. One page-wide clock ticks every 30 seconds, so a minute label is
    never more than 30 seconds late. It runs only while the page is open and some card has a time,
    to keep idle cost low.
15. **Does the label show the exact time anywhere?** Yes, in a native hover `title`. It adds nothing
    visible to the card.
16. **Do old maps get a backfilled design-start stamp?** No. Session records age out, so a backfill
    could guess wrong. Old in-progress Designs show no time.
17. **Does the existing seconds formatter on the epic page change?** No. The card formatter is new.
    The epic page shows seconds for short agent times, and that is a different job.

## Out of Scope

- The TUI. This covers the web Epics page only.
- Active-time accounting that excludes paused periods.
- Times on the epic page, ticket page or Design page.
- Backfilling enqueue times or design-start stamps.
- Changing the section sort order.

## Further Notes

- ADR 0041 (stats read stamps, not logs) applies: every start comes from a frontmatter stamp or the
  queue state file, never from logs.
- The `united` fixture's Finished epics merged in January 2026, so their labels read in weeks.
