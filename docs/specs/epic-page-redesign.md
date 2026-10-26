# Epic page redesign

## Problem Statement

The epic page in the web UI looks cluttered next to the new Epics page.

- The header stacks its actions as separate lines: a link to the Design or the -impl epic, the
  land-block reason, a disabled Queue with "Still publishing tickets", the design session button,
  Queue, Merge and the summary. Each one sits on its own row.
- Progress is a flat segmented bar, while the Epics page shows a ProgressRing. The two pages
  look like they come from different apps.
- Tickets are plain text lines, so one ticket is hard to tell from the next. Done tickets fold into
  a closed "✓ N done" list that shows none of them.
- The Stats section is a long label and value table at the bottom, always open.
- A Design (map epic) shows only "n of m done" in its header: no time, no cost, and no time per
  ticket.
- A needs-repair ticket says "Fix it in the TUI". The person must switch to a terminal and start
  an investigation by hand.

## Solution

The page uses one quiet layout for loop epics and Designs (mock C of the approved mocks):

- **Header.** The back link, then a ProgressRing (64px, 52px on phones) with the kind badge, the
  title (still the jump menu) and the full summary beside it. On phones the summary sits under the
  title.
- **Bar.** Under a hairline, one bar: a dim stats line on the left and the action buttons on the
  right. On phones the buttons wrap to their own row under the stats line, 44px tall.
- **Notice.** A land-block reason shows as a full-width line under the bar in the paused tone.
- **Ticket cards.** Each live ticket is a slim one-line card that links to its ticket page. The
  card has a status dot, the id, the title and a dim meta text. A needs-you card and a needs-repair
  card stay expanded.
- **Done tickets.** A divider ("N done, M cancelled") follows the live cards. Under it are the 3
  latest done tickets and "Show N more".
- **More stats.** A collapsed card at the bottom with a one-line preview. It opens to the full
  stats.
- **Fix it with an agent.** A needs-repair card has a "Fix it with an agent" button. It opens a
  modal with a `/gx-investigate` prompt, a Copy button and a "Start gx agent" button that starts a
  gx agent session with that prompt.

## User Stories

Header and bar:

1. As a person on an epic page, I want a ProgressRing with the kind badge in the header, so that
   the page matches the epic's card on the Epics page.
2. As a person on a loop epic, I want the ring to read `n/m`, so that I see how many tickets are
   done.
3. As a person on a Design, I want the ring to read a percent, stay grey with no live design
   session, show the moving comet only while the design agent is working, show the waiting tone
   with no motion while it waits for my answer, and turn green when done, so that the comet always
   means an agent is working and the waiting colour means it is my turn.
4. As a person on a running epic, I want the ring to show the moving comet, so that I can see work
   is live.
5. As a person on an epic page, I want the title to stay the jump menu, so that I can still switch
   epics from the title.
6. As a person on an epic page, I want the full summary beside the title, so that I read what the
   epic is for without a toggle.
7. As a person on a loop epic, I want one dim stats line with the state word, the time, the cost
   and the agent time, so that the numbers I check most sit in one place.
8. As a person on a Design, I want the stats line to start with the current design step name, then
   the design time and cost, so that I see where the Design stands.
9. As a person on a Design, I want the design time to be the sum of each design ticket's
   claim-to-close time, plus the claimed ticket's time so far while a design session runs, so that
   it counts the time spent designing and never counts up for a stopped session.
10. As a person on a Design whose cost is unknown, I want the cost left off the stats line, so that
    I never read a wrong $0.00.
11. As a person on an epic page, I want "Open design" or "Open implementation epic" as a button in
    the bar, so that every action looks the same.
12. As a person on an epic page, I want link buttons first and the action button (Queue, Merge or
    the design session button) on the far right, so that the main action is always in the same
    place.
13. As a person on an awaiting-merge epic, I want a green Merge button, so that the finish action
    stands out.
14. As a person on a merge-blocked epic, I want the land-block reason as a full-width notice line
    under the bar, in the paused tone, with Merge hidden, so that I see why it stopped.
15. As a person on a draft -impl epic, I want the stats line to start with "Publishing tickets" and
    a disabled Queue button with the tooltip "Queue unlocks once all tickets are published", so
    that I know why I cannot queue yet.
16. As a person on a phone, I want the buttons on their own row under the stats line, 44px tall, so
    that I can tap them.

Ticket cards:

17. As a person on an epic page, I want each live ticket in a slim one-line card, so that one
    ticket is easy to tell from the next.
18. As a person on an epic page, I want a whole card to open its ticket page, so that I do not hunt
    for a small link.
19. As a person on a loop epic, I want a card's meta to show its status, time, cost, blockers and
    fork note, so that I keep the facts the old detail line gave me.
20. As a person on a Design, I want a design ticket's meta to show its type and state
    ("prototype · ready", "grilling · after 04"), and a done one only its time, so that I never
    see a per-ticket cost that the data cannot back.
21. As a person watching a running ticket, I want a small spinning ring as its dot and its time
    counting up, so that I see it is live.
22. As a person with a ticket question, I want the needs-you card to stay expanded with the
    question and an Answer button, so that I can answer without searching.
23. As a person on an epic with forks, I want a fork's card indented under its parent, so that I
    see where it came from.
24. As a person on a phone, I want a card's meta under its title, so that the title has room.
25. As a person on an epic with a paused ticket, I want the pause reason in the card's meta and no
    extra "Open →" link, so that the card stays one line.
26. As a person on an epic with a failed (error) ticket, I want a one-line card with its status in
    the meta, so that it looks like other live tickets.

Done tickets:

27. As a person on an epic page, I want a divider reading "N done, M cancelled" after the live
    cards, so that finished work sits apart.
28. As a person on an epic page, I want the 3 latest done tickets under the divider, so that I see
    what just finished.
29. As a person on an epic with more done tickets, I want "Show N more" to show them all and then
    become "Show fewer", so that I can open and close the full list.
30. As a person on a finished epic, I want the divider at the top as the heading when no ticket is
    open, so that the page does not start with an empty list.
31. As a person on an epic with a cancelled ticket, I want it muted with "cancelled" as its meta,
    so that I can tell it from a merged one.

More stats:

32. As a person on an epic page, I want a collapsed "More stats" card at the bottom with a one-line
    preview, so that the details stay out of the way until I want them.
33. As a person on a loop epic, I want the preview to read lines, then tokens, then lead time, so
    that I see the size of the work at a glance.
34. As a person on a Design, I want the preview to read tokens, then lead time, so that it shows
    only stats gx records.
35. As a person opening "More stats", I want every stat the old Stats section showed, so that
    nothing is lost.
36. As a person on an epic with no stats, I want no "More stats" card, so that the page shows
    nothing empty.

Empty and loading states:

37. As a person on an epic with no tickets, I want "No tickets yet" where the cards go and a ring
    at 0/0 or 0%, so that the page says plainly that nothing is there.
38. As a person opening an epic page, I want the centered spinner until data loads and "Epic not
    found" for a bad address, so that those states work as before.
39. As a person reloading an epic page, I want the page to repaint from its cached copy, so that
    the layout does not flash.

Fix it with an agent:

40. As a person on an epic with a needs-repair ticket, I want its card expanded in the deadlocked
    tone with its reason and a "Fix it with an agent" button, so that I can start the repair from
    the web.
41. As a person who clicked "Fix it with an agent", I want a modal that shows the prompt
    `/gx-investigate <ticket address> is needs-repair: <reason>`, so that I see what an agent will
    get.
42. As a person in that modal, I want a Copy button, so that I can paste the prompt into a session
    of my own.
43. As a person in that modal, I want "Start gx agent" to start a gx agent session with the prompt
    in the project's main checkout with the project's default harness, so that the investigation
    starts with one click.
44. As a person who started the agent, I want the modal to read "Agent started. View it in the gx
    TUI Agents tab", so that I know where to find it.
45. As a person with remote control turned on in config, I want the Claude session to start with
    `--remote-control <ticket address>` and the modal to add "or in the Claude app", so that I can
    follow the session from my phone.
46. As a person whose start failed, I want the server's error in the modal and Copy still there, so
    that I can fall back to a manual session.
47. As a person using the CLI, I want a verb that starts the same fix agent for a ticket address,
    so that the CLI can do what the web route does.
48. As a person who clicks "Start gx agent" while an agent already fixes that ticket (from another
    tab, a reload or the CLI), I want the refusal "an agent is already fixing <address>", so that
    two agents never work on the same repair.

## Implementation Decisions

### Page structure (web)

- The epic page keeps its route, its data hooks (`useEpicDetail`, `useEpicList`), the page cache,
  the spinner and the not-found state. Everything below the back link changes.
- The header has new parts: the ring with its badge, the title column, the bar, the notice line,
  the ticket card list, the done group and the More stats card. The implementer picks the
  component names and how to split them. The segmented bar and the old `StatLine` use leave this
  page. The segmented bar component stays if other pages use it.
- **Ring.** Reuse `ProgressRing`. It gets a third size for 52px on phones. The implementer picks
  the name and stroke.
  - Kind: when the joined card's active address is this page's epic, the card's kind (this keeps
    the follow-ups badge). Otherwise from `detail.kind`: `map` is Design, `loop` is
    Implementation. A Design and its -impl epic share one joined card on the Epics page, so the
    card's kind can name the partner.
  - Tone and live: find the epic's card in the already loaded epic list (through `addresses`, as
    today). Use the card's `ringTone` and `isLive` only when the card's active address is this
    page's epic. Otherwise, and until the list loads, the page maps its own detail, because the
    list has no row for the hidden half of a joined pair:
    - a loop epic: `detail.state` running or merging is running with the comet, a finished epic is
      done, anything else is muted;
    - a Design: the rule under "Design tone", done when its Done step is done. Before Branch the joined
    card is the Design, so the -impl page uses its own rule. From Branch on the joined card is the
    -impl epic, so the Design page uses its own rule.
  - Loop label `done/total`, fill `done/total`.
  - Design label `NN%`, fill from a new `ring_progress` field on the epic detail payload. The
    server fills it with the same `designPercent` rule the Design card uses, 100 when the Design
    is done.
  - Design tone: the page's own rule, first match wins. Today's `ringTone` keeps a Design grey while
    its session runs, so the page does not use it for a Design. A follow-up asks the Epics page
    Design card to adopt the same rule.
    1. Done (the Done step is done): done tone, no comet.
    2. Waiting for the person: the Design is in Needs you, or a live design session has an
       Actionable (a grilling round or a run-end line). Waiting tone, no comet.
    3. Working: a live design session whose status is starting or working, or the Design is
       creating tickets. Running tone with the comet.
    4. A live design session that is idle, blocked or rate limited with no Actionable: running
       tone, no comet.
    5. Otherwise (no session, or only Exited ones): muted, no comet.

    "Live" means `liveSessions(address, sessions)`, as in the design time.
- **Title column.** The jump-menu h1 stays. The full summary renders as `Markdown` under the title,
  with no clamp. On desktop the column sits beside the ring. On phones the summary moves under the
  ring and title row.
- **Stats line, loop epic.** Parts joined by " · ", empty parts dropped:
  1. The state word, coloured by its tone. "Not queued" for a waiting epic with no queue item, as
     today. "Publishing tickets" for a draft -impl epic.
  2. The wall clock. A running epic shows the plain duration that counts up as today. Any other
     epic with a wall clock shows "took X".
  3. `cost_total`.
  4. "X agent time".

  "n of m done" leaves the line because the ring shows it.
- **Stats line, Design.**
  1. The current step's label from `steps`, the step whose state is `current`. A done Design shows
     the Done step's label.
  2. The design time: the sum of the rows' claim-to-close seconds, plus the claimed row's time
     since `claimed_at` only while this Design has a live design session
     (`liveSessions(address, sessions).length > 0`; `hasDesignSession` also counts Exited sessions
     and is not used here). A claimed row with no live design session adds nothing and does not
     count up. The time is left out when no row has
     a time.
  3. `stats.cost`, left out when absent.
- **Buttons.** All actions are `Button`s in one group, in this order:
  - Link buttons first. The partner button reads "Open design" or "Open implementation epic". It
    is neutral, has no arrow and navigates to the partner's epic page.
  - The action button last, on the far right. It is one of these:
    - Queue (blue). Disabled with the tooltip "Queue unlocks once all tickets are published" on a
      draft -impl epic.
    - Merge (done tone, green).
    - The design session button: "Start design session" (blue) or "Open design session" (running
      tone), with today's rules.
  - The buttons wrap to their own row on phones, 44px tall.
  - The "Still publishing tickets" text goes.
- **Notice line.** When the epic is paused with a `land_block_reason`, the reason shows under the
  bar, full width, in the paused tone. It wraps. Merge stays hidden, as `isMergeable` already
  rules.

### Ticket cards (web)

- The live rows keep today's order and nesting (`nestLive`, indent per generation up to
  `MAX_INDENT`). The tone, dot and bucket mapping in the epic `rows` module stay the single source.
  The dot is a filled tone dot, a small spinning ring for running, and a check for done.
- Whole-card link: a one-line card is an anchor to `ticketHref`. An expanded card (needs-you or
  needs-repair) cannot wrap its own button in an anchor. Its title is the link and the button sits
  in the second row. The implementer picks the markup, but each card has one link to the ticket
  page.
- **Meta text, loop rows.** Today's `detailLine` parts, with two word changes taken from the
  approved mocks:
  - An open row with no blockers reads "ready".
  - A blocked row reads "after NN" with no status word.

  The other statuses keep their rendered word, the pause reason, time, cost and fork note. The
  "Open →" link of a paused row goes, because the card is the link.
- **Meta text, Design rows.** The ticket type, then the state, joined by " · ". The state is
  "ready", "after NN" for a blocked row, or the rendered word otherwise. A done Design row shows
  its claim-to-close time only, never a cost, and no time when it is under one minute (a
  spec-round ticket is claimed and closed in the same second). The row still counts toward the
  stats-line sum. A claimed Design row counts up only while a design session runs.
- **Needs-you card.** It stays expanded. The second row holds the question and an Answer button
  that opens the ticket page, as the Answer link does today.
- **Needs-repair card.** It stays expanded in the deadlocked tone. The meta shows the status and the
  pause reason. The second row holds a "Fix it with an agent" button. `REPAIR_HINT` ("Fix it in the
  TUI") is removed from the code, because only the epic page uses it.
- **Error card.** A row with status `error` stays a one-line card in the deadlocked tone, with its
  status in the meta.
- **Phones.** The meta moves under the title.

### Done group (web)

- The divider reads "N done" or "N done, M cancelled" and sits between the live cards and the done
  cards. With no live rows it sits at the top as the heading.
- The done rows sort by close time, newest first, with no fork indent. A fork whose parent is drawn
  elsewhere keeps the "fork of NN" meta. The group shows the first 3. "Show N more" reveals the
  rest and then reads "Show fewer". Neither state is remembered across reloads.
- A cancelled row is muted with the meta "cancelled". A merged loop row shows its time and cost. A
  done Design row shows its time.

### More stats (web)

- A full-width collapsed card (a button with `aria-expanded`) reading "More stats", with a chevron
  and a right-aligned dim preview. Clicking it opens the stats, and clicking it again closes them.
  Collapsed is the default. The open state is not remembered.
- Preview, empty parts dropped:
  - Loop: "+A −R lines · T tokens · lead time X".
  - Design: "T tokens · lead time X".
- The open card shows every row `statRows` returns today, with the same labels, values and notes,
  laid out as the open-stats mock (a grid of label, value and note). The implementer picks the
  column count per width.
- No card when `statRows` is empty, as today.

### Empty state (web)

- With `total === 0`, "No tickets yet" shows where the cards go. The ring reads `0/0` (loop) or
  `0%` (Design). The divider and done group are absent. More stats follows its usual rule.

### Fix it with an agent (web, server, config)

- **Modal.** "Fix it with an agent" opens a dialog. It shows the prompt
  `/gx-investigate <ticket address> is needs-repair: <reason>`, with `<reason>` being the row's
  pause reason label. When the row has no reason, the prompt ends at "is needs-repair". The dialog
  has a Copy button, which reuses the copy logic in `TriageDialog` (clipboard, then the textarea
  fallback, then select the text), and a "Start gx agent" button.
- **Start.** "Start gx agent" calls a new web write route that takes the ticket address and the
  web state token. The implementer picks the route path and the CLI verb name. The server:
  - builds the prompt itself from the ticket's current reason,
  - refuses a ticket that is not needs-repair,
  - finds the project's trunk checkout (`tickets.TrunkCheckout`) and refuses when there is none,
  - refuses when the ticket has an open `type: investigate` child (the one recovery opens for a
    repair pause that needs judgment). The message names the child's address. This check reads
    the raw status and runs before the needs-repair check, because the rendered status of such a
    ticket is "waiting for children". The page never shows the button in that state (the card is
    not a needs-repair card), so the check guards the CLI and races.
  - refuses with "an agent is already fixing <address>" when a live session already has this
    ticket's session name. This covers a reload, a second tab and the CLI. The name check and the
    session save happen as one step under `metaMu` (for example, a "unique name" option on the
    create request), so two starts at the same moment create one session.
  - calls the existing `createSession` with `Dir` set to that checkout, no `Worktree`, the
    project's default harness (`settings.chatHarness()`, as design sessions use), `Prompt` set to
    the prompt, and a session name made from the ticket address. The name maps one-to-one to the
    address, so the duplicate check above can find it. The implementer picks the exact form.
  - `Ticket` stays empty, because a Session ticket is a map epic or map ticket by definition.
    Setting it would record the investigation's conversation cost on the loop ticket.
- **Remote control.** A new boolean config key, default false. The implementer picks the key name
  and its place in the config file. When it is on and the harness is Claude, the fix-agent session
  starts with `--remote-control <ticket address>`. The flag goes into the session's launch args.
  Other harnesses ignore the setting. The key applies only to fix-agent sessions. The server reads
  it from the live settings when each session starts, so a config reload applies it to the next
  start.
  - The remote-control name is recorded in the session meta when the session starts. Every
    launch and every resume (a server restart, or Resume in the TUI) adds `--remote-control` from
    the meta, not from settings, because `startSessionHolder` rebuilds the Claude args on each
    start and would drop it otherwise. A session started with the key off never gains the flag.
- **Result.** The route returns the usual `SessionResult` and also says whether remote control was
  applied. The implementer picks the field. On success the dialog text becomes "Agent started. View
  it in the gx TUI Agents tab", plus " or in the Claude app" when remote control was applied, and
  "Start gx agent" goes away. On a refusal or error the dialog shows the server's message, and both
  Copy and "Start gx agent" stay.
- **Route chores.** The new route follows "Adding a server route" in `.ai/index.md`: `routeTable`,
  a CLI verb with a `routeVerbs` entry, `webWrites`, and the `TestWeb_EverythingElseIs404`
  allowlist. The CLI verb prints the started session's id or the refusal message.

### Server payload changes

- Map epic rows gain `type`, `blockers` (the unresolved ones, as loop rows have), `claimed_at` for
  the claimed row, and `elapsed` for a done row, meaning DoneAt minus ClaimedAt in seconds, absent
  when either is unset.
- Every row gains its close time (DoneAt or CancelledAt) for done-group sorting. The implementer
  picks the field name, or the server sorts the done rows itself.
- The epic detail gains `ring_progress` for map epics, as above.
- Regenerate `webui/src/generated/server.ts` with `make web-types` and check the diff for `any`.

## Testing Decisions

- Good tests check what a person sees or what a route returns. They do not check component
  internals, CSS values or helper call order.
- **Web page tests.** Extend `EpicPage.test.tsx`. It renders through `appRoutes` with fake
  `epic` and `epicList` fetchers and a fake connection. Assert on roles and visible text:
  - the ring label,
  - the stats line text,
  - button names and order,
  - the notice line,
  - card links,
  - meta text,
  - the divider text,
  - "Show N more" and "Show fewer",
  - the More stats preview and its open rows,
  - "No tickets yet",
  - the fix-agent dialog: prompt text, Copy, and the start result texts with a fake start call.

  The tests that look for the "Open design →" and "Open implementation epic →" links change to
  the new button names.
- **Pure helpers.** The stats line parts, meta text, design time sum, done ordering and preview
  text get table tests next to the existing `rows` and `stats` helpers, if they stay pure
  functions.
- **Server tests.** Extend `server/epicdetail_test.go` for the new map row fields, the close time
  and `ring_progress`. Add route tests for the fix-agent route with `servertest` fixtures
  (`servertest.WriteTicketWith`):
  - a needs-repair ticket starts a session in the trunk checkout with the built prompt and no
    Session ticket,
  - a ticket that is not needs-repair is refused,
  - a project with no trunk checkout is refused,
  - a second start while a live session has the ticket's session name is refused with
    "an agent is already fixing <address>",
  - a ticket with an open investigate child is refused with a message naming the child,
  - two starts at once for the same ticket create one session,
  - a resume of a fix-agent session started with remote control on keeps `--remote-control`,
  - with the config key on and the Claude harness the launch args carry
    `--remote-control <address>`, with it off they do not, and a non-Claude harness never gets it.

  The `server_parity_test` and web 404 allowlist tests cover the route chores.
- **Gallery.** Update the epic page entries in `webui/src/gallery/registry.tsx` for running,
  finished, Design, draft, merge-blocked, empty and needs-repair states, so a person can look at
  each.
- **E2e.** Flows 2, 3, 7, 8 and 18 read epic page text that changes. Their pass lines and code
  change with this epic. New flows are listed under Verification.
- **Fixture support.** The fix-agent flows need fixture work, as its own implementation ticket:
  - `fakeclaude` accepts `--remote-control <name>` instead of exiting on an unknown flag.
  - `fakeclaude` treats a `/gx-investigate ...` prompt as a plain slash prompt, not as a worker
    prompt for a ticket address.
  - `gxfixture` scenarios gain a way to set a config key, so flow 45 can turn remote control on.

## Verification

Flows refer to `webui/e2e/CATALOG.md`. Look checks use `gx-ui-check` against a `gxfixture`
scenario at desktop width and at 375px. "Updated flow N" means the catalog flow whose pass lines
this epic rewrites. "New flow" means a flow to write as an implementation ticket.

1. Story 1: updated flow 2 on `mixed`: the epic page header has a ring with the Implementation
   badge. On `design`, the `design-epic` page shows the Design badge.
2. Story 2: updated flow 2: the ring reads `2/7` before queueing and `4/7` after.
3. Story 3: new flow "Design epic page" on `design`: before a session the ring reads a percent and
   is muted (`data-tone="muted"`). After Start design session, while the fake agent works, it reads
   `data-tone="running"` with the comet. Page tests, one per rule under "Design tone": done; Needs
   you and a live session with an Actionable both show the waiting tone with no comet; a working
   or starting session and creating tickets show the comet; an idle live session with no
   Actionable is running with no comet; no session or an Exited one is muted. A Design page whose joined
   card shows the -impl epic, and an -impl page whose joined card still shows the Design, each use
   their own kind and tone, through the fallback mappings (loop running, merging, finished, other;
   Design with a live, an Exited and no session). The follow-ups epic keeps its card's badge.
4. Story 4: updated flow 2: after queueing, the running `mixed-epic` ring shows the comet. Updated
   flow 7: after the release the ring reads `2/2`, done tone, no comet.
5. Story 5: flow 20 (Jump menu opens and closes) passes unchanged.
6. Story 6: new flow "Design epic page": the summary text of `design-epic` shows under the title in
   full. A `gx-ui-check` look at 375px shows it under the ring row.
7. Story 7: updated flow 2: the stats line reads "Not queued · $0.00 · 0s agent time" before
   queueing (the parts the fixture stamps) and starts with "Running" after.
8. Story 8: new flow "Design epic page": the stats line starts with "Resolve tickets".
9. Story 9: new flow "Design epic page" on a `design` scenario extended with one done design ticket
   that has claim and close stamps over one minute apart: that card's meta and the stats line show
   its time. Server test: a map row's `elapsed` equals DoneAt minus ClaimedAt. Page tests: a done
   row under one minute shows no time on its card but counts in the sum, and a claimed row adds no
   time when the Design has no session or only an Exited one.
10. Story 10: new flow "Design epic page": the `design` fixture has no stamped cost, so the stats
    line has no "$". Server test: `stats.cost` is absent when a ticket had a Codex session.
11. Story 11: updated flow 18: the impl epic page has a button "Open design" and the Design page
    has a button "Open implementation epic". Each opens the other page.
12. Story 12: updated flow 18 on `design-epic-impl` once queueable: "Open design" comes before
    Queue in the bar's button group.
13. Story 13: new flow "Merge from the epic page" on `united`: `#/e/demo:awaiting-epic` shows a
    Merge button with `data-tone="done"` last in the group.
14. Story 14: same flow on `#/e/demo:blocked-merge-epic`: the notice line shows the land-block
    reason under the bar and there is no Merge button.
15. Story 15: new flow "Draft impl epic" on `united` (`draft-epic`'s -impl, or a scenario addition
    with a draft -impl epic): the stats line starts with "Publishing tickets", Queue is disabled,
    and hovering it shows "Queue unlocks once all tickets are published".
16. Story 16: new flow "Phone, epic page" at 375px on `united`, `#/e/demo:awaiting-epic`: the
    buttons sit below the stats line and each is at least 44px tall.
17. Story 17: updated flow 2: each live ticket is a card (one link per ticket) with its id and
    title on one line.
18. Story 18: updated flow 3: opening the ticket from its card lands on `#/t/<address>`.
19. Story 19: updated flow 2: card metas read `ready`, `after 02`, `drafted · draft`, `asks` and
    `fails`, as their rendered words.
20. Story 20: new flow "Design epic page": open design tickets show their type and "ready" or
    "after NN". A done one shows a time and no "$".
21. Story 21: updated flow 7 with `--hold`: before the release the running card has the spinning
    dot (`aria-hidden` svg inside the card) and a time that grows between two reads.
22. Story 22: updated flow 2: after queueing, ticket 06's card is expanded with its question and an
    Answer button that opens the ticket page.
23. Story 23: page test with a fork row: its card has `data-indent="1"`. No fixture scenario has
    forks, so no e2e flow is needed.
24. Story 24: new flow "Phone, epic page" on `mixed`: a card's meta sits below its title (its
    bounding box top is below the title's bottom).
25. Story 25: updated flow 3 on `paused-epic`: the "Ambiguous base" and "Agent stopped without a
    result" reasons show in their cards' meta, and the page has no "Open →" link.
26. Story 26: updated flow 2: the `fails` ticket is one line, in the deadlocked tone, with its
    status in the meta.
27. Story 27: updated flow 2: the divider reads "1 done, 1 cancelled".
28. Story 28: new flow "Done list" on a new scenario `many-done` (an epic with 5 merged tickets
    with different close times and 1 open): 3 done cards show, newest first.
29. Story 29: same flow: "Show 2 more" shows all 5 and then reads "Show fewer". Clicking it goes
    back to 3.
30. Story 30: updated flow 7: after the release the divider is the first item in the ticket list.
31. Story 31: updated flow 2: the cancelled card is muted and its meta reads "cancelled".
32. Story 32: updated flow 2: a "More stats" button with `aria-expanded="false"` is the last item.
    Clicking it sets `aria-expanded="true"` and shows the stat rows.
33. Story 33: page test with lines, tokens and lead time stamped: the preview reads
    "+L −R lines · T tokens · lead time X".
34. Story 34: page test for a map epic with tokens and lead time: the preview reads
    "T tokens · lead time X", with no session or step count.
35. Story 35: page test: the open card shows Lines, Tokens, Cost, Lead time, Pauses, Run events,
    Paused time and Pause endings with today's values and notes.
36. Story 36: updated flow 8: `empty-epic` (no stats stamped) has no "More stats" button.
37. Story 37: updated flow 8: `empty-epic` reads "No tickets yet" and its ring reads `0/0`. Page
    test: an empty map epic's ring reads `0%`.
38. Story 38: page tests: the spinner shows before data, and "Epic not found" shows for an unknown
    address.
39. Story 39: page test with a seeded page cache: the redesigned header renders before the fetch
    resolves.
40. Story 40: updated flow 3, Repair row: `paused-epic/07`'s card is expanded in the deadlocked
    tone with "Run failed" and a "Fix it with an agent" button. The page has no "Fix it in the
    TUI" text.
41. Story 41: new flow "Fix it with an agent" on `paused`: clicking the button opens a dialog
    showing `/gx-investigate demo:paused-epic/07 is needs-repair: Run failed`.
42. Story 42: same flow: Copy puts that text on the clipboard (granted clipboard permission) or
    selects it.
43. Story 43: same flow: "Start gx agent" starts a session. `gx agents list` then shows one more
    session with dir set to the trunk checkout of `demo` and harness `claude`. Server route test:
    the session has the prompt and no Session ticket.
44. Story 44: same flow: the dialog reads "Agent started. View it in the gx TUI Agents tab" and has
    no "Start gx agent" button.
45. Story 45: new flow "Fix it with an agent, remote control" on `paused` with the new config key
    set to true in the fixture config: the dialog reads "...Agents tab or in the Claude app".
    Server test: the launch args carry `--remote-control demo:paused-epic/07` for Claude only.
46. Story 46: server route test: a non-needs-repair ticket and a project with no trunk checkout are
    refused with a message. Page test with a fake refusal: the dialog shows the message, and Copy
    and "Start gx agent" stay.
47. Story 47: run the new CLI verb on `paused` with `demo:paused-epic/07`: it prints a session id
    and exits 0. With `demo:paused-epic/03` (needs-answer) it prints the refusal and exits non-zero.
    `TestRouteTableHasCLIVerbs` passes.
48. Story 48: same flow as story 43: reload the page, open the dialog again and click "Start gx
    agent": the dialog shows "an agent is already fixing demo:paused-epic/07" and `gx agents list`
    has no new session. Server route test covers the CLI path.
    Server route test: two starts for the same ticket run at once and create one session.
    Server route test: a paused ticket with an open `type: investigate` child is refused with a
    message naming the child (no story; a server guard).

## Out of Scope

- The TUI epic view.
- Any change to the Epics page or its cards beyond what the shared helpers already do.
- "Approve it in the TUI" on permission cards. It stays as it is.
- Remote control for design sessions or any session other than "Fix it with an agent".
- A session count for Designs.
- The Epics page Design card's tone. It keeps today's rule; a follow-up covers matching it.

## Self-decided design questions

1. **How does the web start the fix agent, given that `POST /v1/sessions/create` is not a web
   write?** Choice: a new narrow web route that takes the ticket address. The server builds the
   prompt and checkout and then calls the existing `createSession`. Reason: the browser does not
   know the checkout path, and a generic create on the web would let a page start any prompt in
   any directory.
2. **Does the fix-agent session carry the ticket as its Session ticket?** Choice: no. Reason: the
   glossary defines a Session ticket as a map epic or map ticket, and setting it would record the
   investigation's cost on the loop ticket.
3. **Does the remote-control key apply to all Claude sessions?** Choice: only to fix-agent
   sessions. Reason: the session name is the ticket address, and widening it would change design
   and chat sessions, which nobody asked for.
4. **What happens to "Start gx agent" after a successful start?** Choice: it goes away, and Copy
   stays. Reason: a second click would start a duplicate agent. The server also refuses a
   duplicate (story 48).
5. **Are done tickets nested by fork?** Choice: no. The done group is flat, newest first, and keeps
   "fork of NN". Reason: indenting conflicts with time order and the 3-card cut.
6. **Which words do open and blocked rows show?** Choice: "ready" and "after NN", on loop epics
   and Designs, as in the approved mocks. Reason: the mocks were approved with these words, and
   one wording for both kinds keeps the layout shared.
7. **Where do the ring's tone and badge come from?** Choice: the badge from the page's own
   `detail.kind`. The tone and comet from the joined card only when it shows this page's epic,
   else from the same helpers on the page's own state. Reason: a Design and its -impl epic share
   one card, so copying it whole gave each page the partner's badge and tone.
8. **How full is a done Design's ring?** Choice: 100%, in the done tone. Reason: the Design card
   does the same.
9. **Are the More stats and "Show N more" states kept across reloads?** Choice: no. Reason: both
   are per-visit views, and the page opens calm by default.

## Further Notes

- Mocks: https://claude.ai/artifact/J4FpZtrjBHQMqJRcCoeJGX (rows "Combined: C + H3 + S1",
  "Combined at phone width" and "Bar states").
- Map decisions: `gx:epic-page-redesign` tickets 01 to 04.
- The design ring rule mirrors the Design card on the Epics page. When that card's rule changes,
  this page follows through the shared helpers.
