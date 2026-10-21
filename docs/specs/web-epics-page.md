## Problem Statement

The web UI shows only epics that are in the Queue. A person who has just run `/gx-design` cannot see
the new implementation epic from a phone, so they cannot inspect it and queue it. They cannot see
which maps wait to be designed, how far a map being designed has come, or which epics finished
recently. They cannot search epic history by name, and they cannot tell how many follow-ups wait.
For all of this they must use the TUI or read ticket files.

## Solution

A read-only-first "Epics page" in the web UI at `#/epics`, next to the Queue. A shared top bar on
every page links `Queue` and `Epics`. The Epics page lists epics in sections by lifecycle: Ready to
queue, Designing, Waiting to design and Recently finished. Each epic is in exactly one section.
Queued and running epics stay on the Queue page. A search box finds any epic by name, old ones
included. A muted line in the header shows the follow-up counts and links to the `follow-ups` epic
pages. A `Queue` button on a Ready to queue row (and on the epic page) queues the epic. This is the
first write action in the web UI. The page updates live.

## User Stories

1. As a person who just designed an epic, I want to see it under Ready to queue, so that I can
   inspect it before I queue it.
2. As a person on a phone, I want a `Queue` button on a Ready to queue row, so that I can start the
   epic without a computer.
3. As a person who clicked `Queue`, I want the button to say it is working, so that I do not click
   twice.
4. As a person who clicked `Queue`, I want the row to leave Ready to queue once the epic is queued,
   so that the list stays true.
5. As a person whose `Queue` click was refused, I want the reason inline next to the button, so that
   I know why and the row stays.
6. As a person, I want no confirm step and no undo on `Queue`, so that queuing is one tap. Taking an
   epic back stays on the Queue page.
7. As a person looking at an epic page that is ready to queue, I want the same `Queue` button there,
   so that I can queue after reading the tickets.
8. As a person looking at any other epic page, I want no `Queue` button, so that I cannot queue a
   map, a running epic or a finished one.
9. As a person, I want a Ready to queue epic that waits for another epic to say `after <epic-name>`
   with a link, so that I know what blocks it while `Queue` still works.
10. As a person, I want only unmerged epics counted as blockers, so that old merged work does not
    clutter the note.
11. As a person, I want Ready to queue sorted newest first, so that the epic I just designed is on
    top.
12. As a person, I want one-offs, follow-ups and archived epics never listed, and draft epics shown
    only in search, so that the sections show only epics I can act on.
13. As a person, I want maps that wait for design in their own section, so that I see what to design
    next.
14. As a person, I want Waiting to design sorted A to Z, so that I can find a map by name.
15. As a person, I want a map I have started in a Designing section with `N/M` tickets closed, so
    that I see its progress.
16. As a person, I want a Designing row to show `active 2h ago` and sort newest first, so that the
    map I worked on last is on top.
17. As a person, I want a Recently finished section with the last 10 merged epics and finished
    maps mixed, so that finished work does not vanish.
18. As a person, I want a `map` marker on a finished map, so that I tell it from an implementation
    epic.
19. As a person, I want each row to be one line with the name, then dim `project · N/M · created 2h
    ago`, so that the page is dense and calm.
20. As a person, I want the time wording to match the section (`created`, `active`, `finished`), so
    that the time means something.
21. As a person, I want only the epic name to be a link to the epic page, so that I do not open a
    page by mistake while I aim for `Queue`.
22. As a person, I want empty sections hidden, so that the page shows no empty gaps.
23. As a new user with no epics, I want "No epics yet." and a hint to run `/gx-design`, so that I know
    what to do.
24. As a person, I want one search box that matches the epic name as a case-insensitive substring,
    so that I can find any epic.
25. As a person who types in the search box, I want one flat result list that also covers old
    finished, queued and running epics, so that nothing is hidden.
26. As a person reading search results, I want a state badge first on each row, so that I know where
    the epic is in its life.
27. As a person who finds nothing, I want a "No match" empty state, so that I know the search ran.
28. As a person who clears the search, I want the sections back, so that I return to the overview.
29. As a person, I want a muted "Follow-ups 35 · gx 31 · blog 4" line, so that I see how many
    follow-ups wait without a list.
30. As a person with one project, I want just "Follow-ups 31", so that the line stays short.
31. As a person, I want each project name in that line to link to its `follow-ups` epic page, so
    that I can open them.
32. As a person with no follow-ups, I want no follow-ups line, so that the header stays clean.
33. As a person, I want the page to update by itself on every server event, so that I need no
    reload.
34. As a person typing in the search box or scrolled down, I want a refetch not to reset my search
    text or scroll position.
35. As a person on first load, I want a centered spinner, so that the page does not look empty or broken.
36. As a person whose server link drops, I want the last list to stay on screen, so that a blip does
    not blank the page. The Offline dot tells me it may be stale.
37. As a person whose first load fails, I want "Could not load epics" and a `Retry` button, so that I
    can recover without a reload.
38. As a person, I want a top bar with `Queue` and `Epics` on every page, so that I move between
    them in one tap.
39. As a person, I want the current top bar link highlighted, so that I know where I am.
40. As a person, I want the build string, the Paused or Draining chip and the Live dot in the top
    bar, so that they stay visible on every page.
41. As a person on an epic or ticket page, I want a back link, so that I return to where I came
    from.
42. As a person who opened an epic or ticket page by a direct link or reload, I want the back link to
    go to the parent (`← Queue` or `← <epic name>`), so that I am not stuck.
43. As a person with an old bookmark, I want `#/` and unknown hashes to open the Queue, so that links
    do not break.
44. As the server, I stamp a map's `completed_at` when its `<map>-impl` epic is created, so that the
    page knows the map is done being designed (map decision 01).
45. As a developer with existing maps, I run `gx tickets migrate` to backfill `completed_at`, so
    that old maps land in the right section (map decision 01).
46. As a script author, I want a `gx server epics --json` verb, so that I can read the same data the
    page reads (map decision 04).
47. As a person, I want the page on one column capped at 1000px like the Queue, so that it reads the
    same on a phone and a desktop.

## Implementation Decisions

**Navigation and routes**

- The Epics page is `#/epics`. The Queue moves to `#/q`. `#/` and any unknown hash redirect to
  `#/q`. Existing `#/e/<address>` and `#/t/<address>` links stay.
- A shared top bar sits on every page, inside the page container. It holds `Queue` and `Epics`
  links, then the build string, the Paused or Draining chip and the Live dot. These three leave the
  Queue title row. The tabs are larger and uppercase, and the active tab has a tone underline. On
  epic and ticket pages no tab is highlighted.
- The Queue page and the Epics page have no h1. The highlighted tab names the page. Epic and ticket
  pages keep their own titles.
- The back link on the epic and ticket pages is `← Back` and uses router history when the page was
  reached inside the app. After a direct link or reload it goes to a fixed parent: `← Queue`
  (`#/q`) on the epic page and `← <epic name>` on the ticket page.
- The Queue page and the epic page change only here: the top bar, the `#/q` route and the epic page
  `Queue` button. What the Queue page shows does not change.

**Section rules** (the server computes the state, the client groups)

| Epic condition | Where it shows |
|---|---|
| Loop epic, open, not queued, not merged | Ready to queue, newest first |
| Loop epic, draft | Hidden; search only |
| Loop epic queued, running, awaiting merge, parked or deadlocked | Queue page; search only, with a state badge |
| Loop epic merged | Recently finished, by `merged_at` |
| Map without `completed_at`, with a claimed or done ticket | Designing, by `last_activity_at`, newest first |
| Map without `completed_at`, nothing claimed or done | Waiting to design, A to Z |
| Map with `completed_at` | Recently finished, with a `map` marker, by `completed_at` |
| `one-offs`, `follow-ups`, `*-archived` epics | Never listed |

- Recently finished is the last 10 rows in total, both kinds mixed, newest first by `merged_at` or
  `completed_at` (confirmed with the person). Older epics show only through search.
- Sort orders the map never set were confirmed with the person: Waiting to design A to Z, Recently
  finished newest first, flat search results A to Z.
- A draft is a loop-epic state only. Map tickets are never parked, so a map has no draft state.
  Drafts show only in search, with a `Draft` badge. `*-archived` epics are never listed, not even in
  search.
- A Ready to queue row is always queueable: ready means open, unqueued, unmerged, with an open
  ticket. An open, unqueued loop epic with no open ticket is in no section. It shows only in search,
  without a `Queue` button. The implementer picks its state name and badge label.
- A map with every ticket closed but no `completed_at` stays in Designing.
- A Ready to queue epic that is blocked by another unmerged epic stays in Ready to queue.

**Row content**

- One layout in every section: the epic name (a link to `#/e/<address>`), then dim, dot-separated
  parts `project · N/M · <time>`. The `Queue` button follows the text directly, not at the far right.
- The time part reads `created 2h ago` (Ready to queue, Waiting to design), `active 2h ago`
  (Designing, from `last_activity_at`) and `finished 2h ago` (Recently finished).
- A finished map adds a dim `map` part. Ready to queue and Designing rows have no other marker.
- A blocked Ready to queue row adds `after <epic-name>` after the time, dim, with each name linked and
  names comma-separated. Only unmerged blockers count.
- Search results use the same row, with a state badge first. The badge covers every server state,
  and a `map` marker for maps. The label is the `state` name in sentence case ("Ready to queue",
  "Designing", "Waiting to design", "Finished", "Queued", "Running", "Awaiting merge", "Parked",
  "Deadlocked", "Draft"). Tones (the existing names in `ui/tone.ts`): Ready to queue `running`,
  Designing and Waiting to design `muted`, Finished `done`, Queued `running`, Running `running`,
  Awaiting merge `waiting`, Parked `parked`, Deadlocked `deadlocked`, Draft `muted`.
- Search results are one flat list sorted A to Z. Each row shows its project.
- There is no project filter and no project grouping.
- Each section starts with a tone badge: Ready to queue, Designing, Waiting to design, Recently
  finished. The badge shows no count. The section order is the one just listed.
- Empty states: every section empty gives "No epics yet." with a hint to run `/gx-design`. A search
  with no match gives an `EmptyState` titled "No match".

**Header**

- One row: a narrow search box on the left, one muted follow-ups line on the right: "Follow-ups 35 ·
  gx 31 · blog 4". Each project name links to that project's `follow-ups` epic page. With one project
  the line is "Follow-ups N". With no open follow-ups the line is hidden. No individual follow-up
  is listed.
- The count is the number of `draft` and `open` tickets in the project's `follow-ups` epic (confirmed
  with the person). Closed follow-ups do not count. Projects with zero, or with no `follow-ups`
  epic, are left out.

**Search**

- Case-insensitive substring on the epic name, over all epics the endpoint returns. While the box has
  text the sections are replaced by the flat result list. Clearing the box brings the sections back.
  The client runs the search and the 10-row cap.

**Queue action**

- `POST /v1/queue/add` joins the web mux and needs the state token, like the other web writes. The
  CLI verb is the existing `gx server queue add`, so ADR 0028 needs no new verb for it.
- `queue add` also accepts an epic address (`project:epic`). It writes one queue entry for the epic,
  with no expansion into tickets, since the scheduler already runs per epic. Ticket addresses keep
  working.
- Refusals stay results, as in ADR 0028: `map-epic` and `already-queued`, plus a new reason for an
  epic with no open ticket. The new reason is `no-open-tickets`, kebab-case like `map-epic` and `already-queued`; clients switch on it (ADR 0028).
- The `Queue` button shows on each Ready to queue row and on the epic page, only when the epic is
  queueable. It is never shown for a map. It is disabled only while the request runs, and its label
  shows a pending state. Success shows no toast. A refusal shows the server's reason inline beside
  the button, and the row stays. There is no confirm and no undo.
- After a successful `Queue` click the page refetches at once, without waiting for an event. The
  epic page needs to know whether the epic is queueable. The epic detail payload gets a `queueable`
  field, with no new route. It is true for any open, unqueued, unmerged loop epic with an open
  ticket, the same rule as the list.
- `Queue` uses a new shared `Button` component with variants `solid`, `tinted`, `outline` and
  `ghost`, tones, and sizes `sm` and `md`. `Queue` is `tinted` with the `running` tone. The failed
  first fetch `Retry` is a `ghost` button. The Gallery gets entries for `Button` and `TopBar` (Queue
  active, Epics active, none active, Paused, Draining, Offline, no build string). Any other new
  shared component gets its own Gallery entry too; the implementer decides which ones exist.

**Data source**

- New route `GET /v1/epics` on the web mux, with the CLI verb `gx server epics --json` as a thin
  client through a new `apiclient` method (ADR 0028). The snapshot does not change.
- The server returns every listable epic: all but one-offs, follow-ups and `*-archived`. Each row
  carries `address`, `project`, `name`, `kind` (`loop` or `map`), a computed `state`, `done`,
  `total`, `completed_at`, `merged_at`, `queued`, `queueable`, `blocked_by_epics` and
  `last_activity_at`, plus `created_at`, the epic's own `created_at` (loop epics and maps alike). See
  "Ticket and map data changes".
- `state` is one of `ready`, `designing`, `waiting-to-design`, `finished`, `queued`, `running`,
  `awaiting-merge`, `parked`, `deadlocked` and `draft`. Epics in `queued`, `running`, `awaiting-merge`
  and `parked` and `deadlocked` have no page section and show only in search.
- `blocked_by_epics` lists only unmerged blockers. The server computes it.
- `last_activity_at` is the newest `claimed_at` or `done_at` across the map's tickets.
- The response also holds `follow_ups`, a list of `{project, open}`.
- Without the web cookie the route gives `401`, like every web route. The web allowlist and the
  "everything else is 404" test list the new routes, and every other server route stays 404.
- The generated web types come from the server's wire types, so the new payload types need the
  generation step.

**Ticket and map data changes**

- A map gets a `completed_at`, stamped when its `<map>-impl` epic is created. `gx tickets migrate`
  backfills it from the impl epic's creation time, when that epic exists.
- Tickets get two new read-only fields, `claimed_at` and `done_at`. They are stamped on every claim and
  every done, for loop and map tickets. The existing `finished_at` is renamed to
  `iteration_finished_at`. `gx tickets migrate` rewrites existing tickets and backfills the new
  fields.
- An epic gets a `created_at` field, stamped when the epic is created. `gx tickets migrate` backfills
  it from the epic file's creation time. Tickets get no `created_at`.
- `gx server epics` is a parent command today; it becomes runnable itself and takes `--json`.
- `queue.json` holds ticket addresses today. One entry for an epic address (`project:epic`) is new,
  so the implementer checks every consumer of the queue list (scheduler, epic summaries, root
  removal).
- Before the rename, the implementer audits every reader of `finished_at` (TUI, loop, server, tests)
  and updates them, since the rename goes past the web UI.

**Refresh and fetch states**

- The page refetches `GET /v1/epics` on every server event, debounced to one fetch per burst of about
  500 ms. It does no address matching, since the page spans many epics. It reuses the existing event
  connection.
- First load shows a centered spinner (`role="status"`, "Loading epics") under the real top bar and search
  box, with no section badges until data arrives.
- A failed refetch with data on screen keeps the last list. The Offline dot says it may be stale, and
  the list refetches on reconnect.
- A failed first fetch shows the inline line "Could not load epics" and a `Retry` button. It also
  retries on the next event or reconnect. It does not stay a spinner.
- A refetch never resets the search text or the scroll position.

**Layout**

- One column in the shared page container, capped at 1000px and centered, in the Queue tab's visual
  language (condensed type, tone colors, `Badge`) but without cards.
- Design reference: the throwaway prototype on the branch `prototype/web-epics-page`. This spec
  overrides it where they differ.
- Names of components, files, CSS and color tokens are the implementer's choice.

## Testing Decisions

- A good test checks behavior from outside: the HTTP response of an endpoint, the result of a CLI
  verb, and what a rendered page shows for a given payload. It does not check helper functions or
  component internals.
- Server seam: the web mux, through the existing web handler test setup. It covers the `GET
  /v1/epics` payload for each section rule (one case per row of the table, including drafts,
  one-offs, follow-ups and archived epics left out), `blocked_by_epics` with merged blockers
  ignored, the follow-up counts, `last_activity_at`, the `401` without a cookie, and `POST
  /v1/queue/add` with an epic address (success, `map-epic`, `already-queued`, no open ticket, and
  the state token). Prior art is the ticket detail and queue write tests in the web tests and the
  web allowlist test.
- Contract seam: the "everything else is 404" test and the route-versus-CLI parity test cover the
  new route and verb.
- Web seam: render the page through the router with a canned `/v1/epics` fetch, like the ticket page
  tests. It covers the sections and their order, empty sections hidden, row parts and time wording,
  the `after <epic-name>` note, the header follow-ups line (one project, several, none), search
  (case-insensitive substring, flat list with state badges, "No match", clearing), the `Queue`
  button (pending, refusal inline, immediate refetch), the spinner, the failed first fetch with
  `Retry`, the kept list on a failed refetch, and a debounced refetch on a server event that keeps
  the search text. Fake timers cover the debounce. Scroll position is not tested in jsdom; the test
  checks only that the search text and the rendered rows stay.
- Router and nav tests: `#/` and unknown hashes redirect to `#/q`, the top bar highlights the
  current page, and the back link uses history or the fixed parent. Prior art is the routes test.
- Epic page: the `Queue` button shows only for a queueable epic and never for a map.
- Data changes: tests for the `completed_at` stamp on `-impl` creation, the `claimed_at` and
  `done_at` stamps, the `iteration_finished_at` rename, and `gx tickets migrate` backfills. Prior art
  is the existing migrate tests.

## Out of Scope

- A TUI epics page.
- Changing what the Queue page shows.
- Listing individual follow-ups on the Epics page.
- Editing, archiving or deleting epics from the web UI.
- One-off history. It needs special handling and is filed as `gx:follow-ups/36`.
- TUI epic-only queuing (remove the checkboxes and single-ticket queue actions), filed as
  `gx:follow-ups/37`.

## Further Notes

- The epic details spec (`web-epic-details.md`) left the list page out of scope and kept its route and
  payload open for this page. The epic page is no longer purely read-only: it gains the `Queue`
  button described above. `web-epic-details.md` carries an amendment saying so.
- The map's Destination said the Queue and epic pages change only where they link to Epics. This
  spec widens that on purpose: the `#/q` route, the shared top bar (which takes the build string,
  Paused chip and Live dot out of the Queue title row), and the epic page `Queue` button.
