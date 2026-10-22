## Problem Statement

The web UI ticket page at `#/t/<address>` shows only what a person must act on: the open question,
the answer form, the permission card and the button that sends a Paused ticket back to work. It
does not say what the ticket is, what it waits for, what forked from it, what it cost, or how its
runs went. Most ticket rows in the web UI do not link to it at all. To understand a ticket, a
person must open its markdown file or drop to the TUI.

## Solution

The `#/t/<address>` page grows into the one page for a ticket. It shows a header (title, status
badge, type, epic), the action card when there is something to act on, the ticket body as
markdown, the related tickets (blockers, parent, forks), one line of stats and the run history. It
updates live while the epic runs.

The page is read-only. It adds no new actions. The actions that exist today (answer, the
permission card and the action by pause reason) work as before, inside the action card.

Every row in the web UI that shows a ticket links to its page. The page renders map decision
tickets too, and broken (Error) tickets get a working page instead of an endless spinner.

The spec uses the words of `pause-terminology.md`. Its implementation waits for
`pause-terminology-impl` to merge.

## User Stories

1. As a person on the epic page, I want every ticket row to link to its ticket page, so that I can
   read any ticket, not only one that asks a question.
2. As a person on the epic page, I want the done and cancelled rows in the "✓ N done" fold to link
   too, so that I can check what a finished ticket did.
3. As a person watching the Queue, I want running, Paused and Answered ticket lines on an epic card
   to link to their ticket page, so that I can see what an agent is working on.
4. As a person watching the Queue, I want a one-off row in Recently finished to link to its ticket
   page, so that I can check the result of a one-off.
5. As a person, I want "Answer →" and "Review →" to stay as a second link next to the title, so
   that I still see at a glance that a ticket needs me.
6. As a person following a map epic, I want every decision ticket row, open or done, to link to its
   page, so that I can read the question and its resolution.
7. As a reader, I want the ticket title as the page heading and a status badge under it, so that I
   know at once what the ticket is and where it stands.
8. As a reader of a Paused ticket, I want the badge to read Paused with the reason label, such as
   "Paused · Merge conflict", so that I know why it stopped without reading further.
9. As a reader of a running ticket, I want a spinning arc after the badge, so that I see an agent is
   working on it.
10. As a reader, I want one dim stat line with the type, elapsed time, cost and context window, so
    that I get the numbers without a stat grid.
11. As a reader of a running ticket, I want its elapsed time to tick, so that I see it is alive.
12. As a reader of a ticket that has not merged yet, I want cost and context left out instead of a
    placeholder, so that the line shows only real numbers.
13. As a reader of a ticket never claimed, I want the stat line to show only its type, so that I am
    not shown zeros.
14. As a reader, I want the epic name in the header to link to the epic page, so that I can get back
    to the epic after hopping from ticket to ticket.
15. As a person who must act, I want the action card first, right under the header, so that I see
    what is asked of me before anything else.
16. As a person answering a question, I want the question, the chat, the answer form and the
    "Earlier rounds (N)" fold inside the action card, so that the flow works as it does today.
17. As a person, I want the action card on a running ticket whose agent hit a permission prompt, so
    that "Review →" always opens a page with the card to review.
18. As a person, I want the pause reason text shown inside the action card, next to the action by
    reason (Answer, Choose base, Retry, Resume, …), so that I read why it stopped next to the
    button that sends it back to work.
19. As a person who just sent an answer or an action, I want its result to stay on screen during
    this visit, so that I know it worked.
20. As a person who answered a question, I want the action card to keep the question and my answer
    until the agent picks the ticket up, so that the "Answered" link from the Queue opens something
    meaningful.
21. As a reader of a done ticket, I want no old questions on the page, so that the page shows the
    ticket and not its past conversation.
22. As a reader, I want the ticket body rendered as markdown, whole and unfolded, so that I can read
    it without opening the file.
23. As a reader, I want checkboxes, tables and strikethrough in the body rendered, so that task lists
    and tables read as the author wrote them.
24. As a reader, I want a body link to a sibling ticket file to open that ticket's page, so that I
    can follow references between tickets.
25. As a reader, I want other relative links shown as plain text, so that I am not sent to a dead
    link.
26. As a reader, I want web links in the body to open in a new tab, so that I keep the ticket page.
27. As a reader of a Paused ticket, I want the `## Paused` text left out of the Description, so that
    I do not read it twice.
28. As a reader, I want a Related section with one line for each blocker, the parent and each fork,
    with a short reason such as "blocks this · done", "forked from · done" or "fork · open", so
    that I see what this ticket waits for and where its extra work went.
29. As a reader, I want each Related line to link to that ticket's page, a blocker in another epic
    too, so that I can follow the chain.
30. As a reader, I want a blocker from another epic written as `other-epic/NN`, so that I can tell
    it from a blocker in this epic.
31. As a reader, I want a Runs section with one line per run (start, duration, outcome, and whether
    it was a new session or the same session), so that I see how the ticket's work went.
32. As a reader, I want a run that ended in a pause to show "paused" with the reason label, so that
    I can tell an asked question from a crash.
33. As a reader, I want a run whose agent went away with no logged end to show "agent lost", so that
    I know the run did not finish normally.
34. As a reader, I want a run that a person relaunched to show "relaunched", so that I can tell it
    from a lost agent.
35. As a reader, I want the last run to read "running" and tick only while the ticket is claimed,
    so that the page never claims a dead agent is working.
35a. As a reader of a ticket whose pause was answered while its agent was still alive, I want the
    work after the answer on its own run line ending in its real outcome, so that a later merge
    shows on a line and the pause line stays as it happened.
35b. As a reader of a cancelled ticket whose agent was stopped, I want its last run to show
    "cancelled", so that I can tell a cancel from a lost agent.
36. As a reader, I want the page to use plain words and never say claim, reclaim or reopen, so that
    I need no gx internals to read it.
37. As a reader of a map decision ticket, I want its Resolution first, then its Question, with no
    stat line and no Runs section, so that the page shows what a map ticket has.
38. As a reader of a map decision ticket, I want the body's Resolution and Question headings not
    repeated, so that the page has no duplicate sections.
39. As a reader, I want empty sections hidden, so that the page has no empty headings.
40. As a reader of a ticket with nothing to show, I want one dim `Nothing here yet` line, so that
    the page does not look broken.
41. As a reader of a broken ticket, I want its page to load with an "Error" badge and an action card
    that says what is wrong and where the file is, so that I know which file to fix.
42. As a reader of a ticket whose file does not parse, I want the card to say the file's format is
    invalid and show the parser's error, so that I know what to fix.
43. As a person with a mistyped or stale address, I want "Ticket not found" and a back link to the
    epic, so that I know the ticket does not exist.
44. As a reader, I want the page to update when this ticket or another ticket in its epic changes,
    so that I need no manual reload.
45. As a phone user, I want the page in one column like the epic page, so that it reads the same on
    every page.
46. As a person on the dark theme, I want the ticket page buttons readable, so that I can act on a
    ticket at night.

## Implementation Decisions

**Route and links in**

- The page stays at `#/t/<address>`. The ticket's state decides what shows, not the URL.
- On every `TicketLine` that has an address, the title links to `#/t/<address>`. "Answer →" and
  "Review →" stay as a second link to the same page.
- Links in:
  - Epic page: every live row and every done and cancelled row in the "✓ N done" fold. Error rows
    link too.
  - Queue epic cards: running, Paused and Answered lines.
  - Recently finished: one-off rows (their address is the ticket's). Epic rows keep linking to the
    epic page.
  - Map epic pages: every decision ticket row, open or done.
- A Paused line on the Queue keeps its current repair hint text.

**Layout**

- Design reference: variant D of the layout prototype on the branch
  `prototype/web-ticket-details-layout` (opened with `gallery.html?proto=ticket`). Where this spec
  and the prototype differ, this spec wins: the prototype still uses the old words and shows a cost
  per run.
- The page uses the epic page's frame: top bar, back link, condensed h1 with the ticket title. The
  back link stays as it is today.
- Shared components are extracted and used by both the epic page and the ticket page: a page header
  (back link, h1, optional line under it), a stat line, a section heading (moved out of the Epics
  page) and a detail page wrapper (offline strip, the shared page container, the connected top bar).
  Their names are the implementer's choice.
- Header line: the status badge, then the dim stat line `type · elapsed · cost · context`. The epic
  name sits in the header line and links to `#/e/<project:epic>`.
  - A Paused ticket's badge reads Paused with the reason label, such as "Paused · Merge conflict".
    The labels and tones are the ones `pause-terminology-impl` builds for the web views.
  - A running ticket shows the `ui/arc` spinner right after the badge.
  - The `Arc` component changes everywhere: a half-circle trail (top and right borders) and one turn
    every 1.05s.
- Then, top to bottom: the action card (when it shows), and the sections. Each section has a badge
  heading inside an h2, as on the Epics page. The order is Resolution (map tickets only), Related,
  Description (called Question on a map ticket), Runs.
- The ticket page's buttons (the answer form, the permission card, the action forms by reason and
  the collapsible fold) use `ui/button` instead of Chakra's `Button`, which draws unreadable text on
  the dark theme.

**Action card**

- The action card (see `GLOSSARY.md`) holds everything on the page that asks a person to act. It
  shows when the ticket has any of:
  - an open question (the question, its chat and the answer form);
  - a pending permission prompt, also on a running ticket;
  - pause reason text;
  - an action by pause reason;
  - the result of an answer or action sent from this page visit;
  - an Answered state (the ticket waits for the agent to pick it up), which keeps the last question
    and the person's answer;
  - an Error status (see Broken tickets).
- Otherwise there is no action card. "Parked" in its old sense is not the trigger.
- The "Earlier rounds (N)" fold shows only inside the action card. A ticket without an action card
  shows no old questions. `gx server tickets history` still has them.
- The action card reuses the action forms that `pause-terminology-impl` ticket 13 builds (Answer,
  Choose base, Retry, Resume, …) and their result text. This spec designs no buttons. It decides only
  where they sit and when the card shows.
- Pause reason text: the text under the body's `## Paused` heading shows in the action card in the
  alarm color, next to the action. A self-reported pause shows its question, as today. Whether the
  server or the client splits the `## Paused` text from the body is the implementer's choice.
- A draft has no action, so a draft gets no action card unless one of the cases above applies.

**Body**

- Description is the whole body, nothing folded, `## Comments` included. Its own `##` headings
  render smaller than the page's section headings.
- Description leaves out the `## Paused` section, which shows in the action card.
- Park rounds never reach the body. The server builds them from the ticket log.
- On a map ticket, Description is headed "Question". It leaves out the body's `## Resolution`
  (shown in its own section) and the body's `## Question` heading.
- The shared `Markdown` component adds `remark-gfm`: checkboxes, tables and strikethrough render.
  Checkboxes are read-only. The epic summary uses the same component and gains GFM too.
- Links in the body:
  - A relative link to a sibling ticket file (`issues/NN-slug.md` or `NN-slug.md`) points to that
    ticket's page.
  - Any other relative link renders as plain text.
  - `http(s)` links open in a new tab.

**Related**

- One `TicketLine` per blocker (`blocked_by`), the parent and each fork child, each with a short
  reason and the ticket's status word in the new words, such as "blocks this · done",
  "forked from · done" or "fork · open". The exact reason wording is the implementer's choice.
- A blocker in another epic is written `other-epic/NN`, with no project prefix, as on the epic
  page.
- Each line links to that ticket's page, a blocker in another epic too.

**Runs**

- One line per run, oldest first, as in the prototype. A line holds the start time, the duration
  (ticking while it runs), the outcome, and how the run started: "new session" for a cold start, or
  "same session" when the run reused the agent's last session. The page never prints "resumed" for
  this. The run log keeps its `cold-start` and `resumed` values; only the page label differs.
- No cost per run (gx does not record one). No branch.
- A run starts at an agent start (`iteration-started`) and normally ends at the next merge, pause or
  start over. Conflicts, recoveries, nudges and smart-zone pauses do not show as lines.
- Outcomes use the pause-terminology words. A run that ended in a pause shows "paused" with the
  reason label ("paused · <reason label>"). The other outcomes (merged, merged with no commits,
  started over) follow pause-terminology. Their exact wording is the implementer's choice.
- Runs with no normal end:
  - **"agent lost"**: the server restarted, found the agent gone with no commits and no edits, and
    put the ticket back in the queue (`claim-reopened`). The line explains this in plain words,
    such as "agent gone after server restart, no work to keep, back in the queue". The run ends at
    that event.
  - **"agent lost"**: the log just stops and a new agent start follows. The run ends at that next
    start.
  - **"relaunched"**: a person relaunched the ticket (`human-relaunch` between two runs). The old run
    ends at the relaunch.
  - **"cancelled"**: a cancel stopped the agent (`--stop`, and always for a one-off). Cancel writes
    no run-log line, so the server ends the run at the cancel, taking the time from the status
    write or the cancel event.
  - The last run with no end shows "running" (ticking) only while the ticket is claimed. Otherwise
    it shows "agent lost" with no duration.
- Work after a pause with no new agent start gets its own line:
  - An answer delivered to a live agent claims the ticket again without an `iteration-started`.
    The pause line keeps its "paused · <reason label>" outcome. A new "same session" line starts
    when the ticket is claimed again and ends at the next merge, pause or start over.
  - A late-finish adoption (a ticket paused with no commits, then merged when its agent finished
    late) works the same way: a new line from the late finish to the merge.
  - A reclaim writes no run-log line, so the run continues on the same line.
- The page never says claim, reclaim or reopen.

**Map decision tickets**

- A map ticket has no stat line and no Runs section.
- A closed map ticket shows Resolution first, then Question. An open one starts with Question.
- Map tickets are never Paused. The action card rules above still apply.

**Empty states**

- Description (empty body), Runs (never claimed), Related (no blockers, parent or forks) and
  Resolution (a closed map ticket with no resolution text) are hidden when empty: no heading and no
  placeholder.
- The stat line shows only the values it has. A never-claimed ticket shows its type alone.
- When every section is hidden and there is no action card, the page shows one dim line,
  `Nothing here yet`, under the header, like the epic page's `No tickets yet`.

**Not found and broken tickets**

- An unknown address shows "Ticket not found: `<address>` does not exist", with the back link to the
  epic in the address. No redirect and no search for a moved ticket.
- An Error ticket gets a working page. The server sends a small payload instead of a 500.
- Every Error ticket shows an action card with the error reason in the alarm color and the ticket's
  file path, and no buttons. The reason is the one the server already computes: read error, graph
  error, `blocked_by` error, or "unrecognized status X".
- A file that fails to parse: the card says the ticket file's format is invalid, followed by the
  parser's own error text. The h1 is the title the loader derives from the file name. The header
  shows the "Error" badge and the epic link. The stat line and every section are hidden. No raw
  file text.
- A ticket that parses but has a graph or `blocked_by` error shows the full page as usual, plus the
  action card with the reason.

**Payload**

- `GET /v1/tickets/detail` grows. The payload type keeps the name `TicketDetail`. It carries
  everything the page shows: header fields (type, epic), the body markdown, the related tickets
  (blockers, parent, fork children, each with address, title and status), the stats (elapsed,
  `claimed_at`, cost, context window) and the run lines, built by the server from the run log.
- It also carries the pause reason, its label, the pause reason text and a flag that says the
  ticket is in a map epic. The server builds the label, so the web shows the same words as the
  summaries. The header badge, the "paused · <label>" run outcome, the action card and the map
  ticket layout read these.
- An address that fails to parse returns `404`, the same as an unknown address, so the page shows
  "Ticket not found" instead of hanging on "Loading ticket".
- For an Error ticket it carries the status, the derived title, the error reason, the parser error
  when there is one, and the file path.
- `GET /v1/tickets/history` stays raw, for the CLI.
- Cost and context come from the ticket's merge stamps and are empty until the ticket merges.
- The field names, the run outcome values on the wire and whether the server or the client splits
  the body are the implementer's choice.
- The generated web types are regenerated from the server's wire types. The CLI verb
  `server tickets detail` prints the grown payload, with no extra work beyond the shared type.

**Refresh**

- The page refetches on any live event for a ticket in the same epic (this ticket, its blockers in
  the epic, its fork children), using the existing event connection. A blocker in another epic does
  not refresh the page live. There is no refetch timer.
- A running ticket's elapsed time and its running run line tick on the client from `claimed_at`, as
  epic rows do. The tick interval is the implementer's choice.

**Words**

- The page, its components and its tests use the pause-terminology words: Paused, pause reason,
  pause reason text, action card, merge, start over, Resume only for a pause a person chose. No new
  web text uses a form of "park", "land" or "unpark".
- Names of components, files, CSS, color tokens and payload fields are the implementer's choice.

## Testing Decisions

- A good test checks behavior from outside: the HTTP response of the endpoint, and what the
  rendered page shows for a given payload. It does not check helper functions or component
  internals.
- **Server seam: the web mux**, through the existing web handler test setup. It covers the grown
  payload for a loop ticket and a map ticket, related tickets including a cross-epic blocker, the
  run lines for each case (normal end, pause with reason, `claim-reopened`, a stopped log followed by
  a new start, `human-relaunch`, a cancel that stopped the agent, a pause answered to a live agent
  then merged, a late-finish adoption, an open last run while claimed and while not claimed, new
  session and same session), the pause reason, label and text and the map-epic flag, the small
  payload for a file that fails to parse and for a graph error, and the `404` for an unknown address
  and for a malformed one. Prior art: the ticket detail tests and the epic detail tests in the
  server package.
- **Web seam: the ticket page** rendered through the router with a canned detail fetch, like the
  existing ticket page and route tests. It covers the header (badge, reason label, arc, stat line,
  epic link), the action card cases and its absence, "Earlier rounds" only inside the card, the
  sections and their order, the map ticket layout, the hidden empty sections and
  `Nothing here yet`, the not-found state (a malformed address too), both Error layouts, the body link rules and GFM, and a
  refetch when a live event for a ticket in the same epic arrives. The elapsed tick uses fake
  timers. Prior art: the ticket page tests (answer form, answer result, permission card, reading),
  the routes test and the markdown test.
- **Links in**: the Queue screen tests check the links on epic card lines and on one-off Recently
  finished rows. The epic page route tests check links on live, fold, Error and map rows. The
  `TicketLine` test checks the title link and the second "Answer →" or "Review →" link.
- **Shared components**: the Epics page tests and the routes test keep passing after the pages
  move to the shared header, stat line, section heading and detail page components. The `Arc` test
  covers the new trail and speed only if the existing test already checks them.

## Out of Scope

- New actions: pause, cancel, relaunch, queue, editing the body or fields.
- The agent's live terminal output or transcript.
- Previous and next ticket navigation.
- Branch, worktree and `resolved_base`.
- Cost per run.
- Live refresh for a blocker in another epic.
- Designing the action buttons themselves (`pause-terminology-impl` ticket 13 builds them).

## Further Notes

- `web-ticket-details-impl` waits for `pause-terminology-impl` to merge. It builds on the Paused
  status, the pause reason labels, the action forms by reason and the regenerated web types from
  that epic.
- The map is `gx:web-ticket-details`. Its decision tickets 01 to 09 hold the full reasoning.
