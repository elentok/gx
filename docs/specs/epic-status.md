## Problem Statement

A working Design and a working Implementation look the same in the web UI. Both show the word
"Working" and a blue spinning ring. A person scanning the sidebar or the Epics page cannot tell
whether an agent is designing or building, nor which phase of the build it is in. Two
Implementation states that are real work, Writing PR text and Fixing CI, show a dim, still ring in
the sidebar, so they look idle.

## Solution

A working Design gets a purple ring and a word for what it is doing (Charting, Writing spec,
Reviewing, and so on). A working Implementation keeps the blue ring and gets a word for its phase
(Implementing, Code-reviewing, Verifying, Resolving conflicts, Merging). Writing PR text and Fixing
CI count as working too: blue spinning ring, their own words. This holds in the sidebar rows, the
Epics page cards, and the rings in the Design page and Epic page headers. Everything that is not
working looks as it does today.

## User Stories

1. As a person with several epics, I want a working Design's ring to be purple and a working
   Implementation's ring to stay blue on the Epics page, so that I can tell design work from build
   work at a glance.
2. As a person with the sidebar open, I want the same purple and blue rings on sidebar rows, so that
   the sidebar and the Epics page agree.
3. As a person on a Design page, I want the header ring to be purple with the white comet while the
   Design works, so that I see it is alive without reading the chat.
4. As a person on an Epic page, I want the header ring to be blue with the comet while the epic
   builds, merges, writes PR text or fixes CI, so that the header matches the card.
5. As a person watching a Design, I want its status word to follow the step the agent reports
   (Charting, Researching, Writing spec, Reviewing, Creating tickets, Branching), so that I know what
   the Design is doing right now.
6. As a person watching a Design whose step is unknown, I want the word "Designing", so that the row
   never reads blank or "Working".
7. As a person watching an Implementation, I want "Implementing" for ordinary work, including fix
   tickets, so that the default word says building.
8. As a person watching an Implementation in review, I want "Code-reviewing" while a code-review
   ticket is claimed, so that I know the build reached review.
9. As a person watching an Implementation in verification, I want "Verifying" while a verify ticket
   is claimed, so that I know the epic is being checked.
10. As a person watching an Implementation hit a conflict, I want "Resolving conflicts" while a
    conflict-resolution ticket is claimed, so that I know why progress is slow.
11. As a person watching an Implementation merge, I want "Merging" when only merge tickets are left,
    so that I know the end is near.
12. As a person, I want the latest phase to win when several tickets are claimed (Merging, then
    Verifying, then Code-reviewing, then Resolving conflicts, then Implementing), so that the word
    shows how far the epic has got.
13. As a person whose epic is Writing PR text or Fixing CI, I want a blue spinning ring and the
    words "Writing PR text" or "Fixing CI" in the sidebar and on the card, so that these do not look
    idle.
14. As a person reading a Fixing CI card, I want the card to keep its attempt count text unchanged,
    and the sidebar to show the bare words, so that the card keeps its detail and the sidebar stays
    short.
15. As a person who just pressed Start on a Design, I want the card, sidebar row and headers to spin
    with a Design word while the session starts, so that launching does not look stalled.
16. As a person, I want a Design to count as working while its session is in a turn or a background
    subagent runs, so that the card and the sidebar never disagree.
17. As a person watching tickets get created, I want a Design at Create tickets with a growing
    `-impl` epic to stay working even when its session is idle, with "Creating tickets" in the
    sidebar and "(N so far)" on the card, so that the count keeps showing progress. A Design at
    Create tickets with no running session no longer spins: it reads like any idle Design.
18. As a person whose Design reached Branch, I want the joined card to show the Design state
    (purple ring, "Branching") while its design session works and the `-impl` epic is not running,
    and to follow the Implementation rules (blue ring, build words) once the `-impl` epic runs or
    the map is Done, so that a working Design never looks idle.
19. As a person reading a card, I want working words to end with "..." and sidebar words to have
    none, so that cards read as live and sidebar rows stay compact.
20. As a person sorting the sidebar by Needs you, I want the order to stay as it is today (Needs you
    first, then by name), so that this change does not reorder my sidebar.
21. As a person sorting the sidebar by status, I want one group per working word, placed where
    "Working" sits today, with "Working" among them and all sorted alphabetically, so that I can
    see all reviewing epics together.
22. As a person, I want amber (Needs you), paused, done and grey (idle) rings and their words to look
    as they do today, and the comet to stay white on both kinds, so that only working changes.
23. As a person, I want One-off cards and Follow-ups cards to keep their current words and ring,
    and Chats, Fix agents and Queue agents to keep "Working" and the blue ring, so that this
    change touches only epics.
24. As a person with the page open, I want the words and ring to change when the session starts a
    turn or ends one, without a reload, so that the UI stays live.
25. As a person on a Design page, I want the header ring to follow the working state without a
    reload, so that it does not lag behind the card.

## Implementation Decisions

- One server rule decides whether a Design works: a Design works while any of its live design
  sessions is Starting, or is in a turn (Session status Working, not a background mark), or a
  background subagent runs. Today the sidebar reads the session turn and the card reads only the
  subagent. Both now read the server's rule. The sidebar stops scanning sessions itself for Designs.
- Exception that keeps it working: a Design at Create tickets whose `-impl` epic exists works even
  when its session is idle, and also during a countdown. The card keeps "Creating tickets (N so
  far)..." and its ring shows the placeholder it shows today (no fraction).
- Question, countdown and no-session placements keep priority over working, as they do today: a
  Design waiting on an answer is Needs you (amber), and a counting-down Design keeps its current
  words and ring (except the Create tickets countdown above).
- Visible change (person decided): a Design at Create tickets with no running session no longer
  spins. It shows a grey ring, the word "Designing" with the reason "No design session is
  running", and Continue, like any idle Design, with no "(N so far)" count. Today it spins.
- Joined card at Branch (person decided): while the map is at the Branch step, its design session
  works and the `-impl` epic is not running, the joined card and the sidebar row show the Design
  working state (purple ring, "Branching"). Once the `-impl` epic runs, or the map is Done, they
  follow the `-impl` epic (Implementation rules). "The `-impl` epic runs" means the `-impl` row is
  itself working (Running, Merging, Writing PR text or Fixing CI). The card keeps the `-impl`
  section. `joinImplementation` copies the whole `-impl` row, so the joined row would carry the
  implementation kind and the client could not draw purple: the row therefore carries the kind of
  work (design or implementation) next to the working flag and word, and the client takes the arc
  color from that kind, not from `card_kind`.
- Design word per reported step: Chart "Charting"; Resolve tickets "Researching"; Spec "Writing
  spec"; Review and Grill findings "Reviewing"; Create tickets "Creating tickets"; Branch
  "Branching". Fallback order: the working session's reported step, then the map's own step, then
  "Designing". When several sessions work, the newest by `status_changed_at` gives the reported
  step (see Self-decided design questions).
- Implementation word, first match wins: "Merging" (the existing Merging state: only merge tickets
  left); "Verifying" (a verify ticket claimed); "Code-reviewing" (a code-review ticket claimed);
  "Resolving conflicts" (a conflict-resolution or trunk-conflict ticket claimed); "Implementing" (anything else
  while the epic runs, fix tickets included). A running epic with no claimed ticket reads
  "Implementing".
- Writing PR text and Fixing CI are working states of an Implementation. They keep their words. The
  Epics page card keeps its text as it is today (including the Fixing CI attempt count, with no
  "..."). The sidebar shows "Writing PR text" and "Fixing CI" without the attempt count. The
  working word comes from the state: `EpicListFixingCI` gives "Fixing CI" and
  `EpicListWritingPRText` gives "Writing PR text". `status_words` stays `prstate.Words`, with the
  attempt count.
- Card text rule: the new working words (the Design words, Code-reviewing, Verifying, Resolving
  conflicts, plus "Designing" as the working fallback) end with "..." on cards, like the existing
  "Implementing..." and "Merging...". The sidebar shows the bare word. Existing words keep their
  current text on cards.
- API contract, epic list row: the row carries whether it is working and the bare working word, set
  by the server from the rule above. The client reads these instead of matching on the text of the
  status words (today it matches "Working" and "Creating tickets" prefixes). The server sets the
  flag on every row that spins today, One-off and Follow-ups rows included, and leaves their words
  alone, so `isLive` can read the flag without stopping any ring that spins now. Field names are
  implementer's. For the joined card they follow the Branch rule above, then the `-impl` epic.
- API contract, epic detail response: add a `working` flag for a Design (map) epic, computed from the
  same server rule. `buildMapDetail` passes an empty `designActivity`, but `decorateLive`
  (`server/epics/epicdetail.go`) sets `DesignSection` again from `s.designActivities()` for every
  active map. Set the flag in `decorateLive` next to `DesignSection`, from the same
  `designActivity`. Name is implementer's, given as `working` here.
- Detail freshness: the epic detail is refetched on ticket events, not on session changes. The Design
  page must also refetch it when a live session's state or background mark changes (it already
  refetches when the set of live sessions changes), so the header ring follows the flag.
- Ring colors: the ring component already takes a color token for the arc. A working Design's ring
  (card, sidebar row, jump menu row, Design page header) uses the Design kind color (purple). A
  working Implementation and Writing PR text and Fixing CI use the running tone (blue). The jump
  menu (`webui/src/nav/JumpMenu.tsx`) uses the same arc rule as cards, since it reads `isLive` and
  `ringTone`. The comet stays white. Tone mapping for amber, paused, done and grey does not change.
- Header rings: the Design page ring is purple and live from the detail `working` flag, in place of
  its present session scan. The Epic page ring treats Writing PR text and Fixing CI as running and
  live, whether it takes its tone from the card or from the detail state.
- Sidebar status model: a row's status is Needs you, a working word (blue), or the server word
  (dim). The Needs you sort does not change (Needs you, then name). In the By status sort, all
  working words, "Working" included, share the rank where "Working" ranks today, and each word is
  its own group, sorted alphabetically. Rank comes from the working flag, not from the word: a
  working row groups under its word, an idle row under its server word. The working fallback
  "Designing" and the idle word "Designing" are different rows with different ranks. Chats, Fix agents, Queue agents and other sessions keep
  "Working" and group with each other.
- One-off and Follow-ups cards, and every Queue-tab and agents-sidebar TUI surface, do not change.
- No schema change in the ticket store, so no Migration step.

## Testing Decisions

- A good test checks what a person sees: the words, the ring color and live state, the row order. It
  does not check helper names or field layout.
- Server seam (Go, `server/epics`): table tests of the epic list rows. They cover the working rule
  (Starting, turn, background subagent, idle, countdown and no session at Create tickets with and
  without an `-impl` epic, question, the Branch step with a working session and an idle or running
  `-impl` epic), the Design word for each reported step with both fallbacks and the newest-session
  tie-break, the Implementation word for each claimed-ticket kind (`conflict-resolution` and
  `trunk-conflict` included) and for several at once, a joined row at Branch (working Design with
  an idle and with a running `-impl` epic), and the Writing PR text
  and Fixing CI rows. They also cover the epic detail `working` flag. Prior art: `placedesign_test.go`,
  `designcard_test.go`, `epiccard_test.go`, `epiclist_state_test.go`, `detail_test.go`.
- Web model seam (vitest): sidebar model and ring helpers, fed list rows and sessions. They cover
  status words and tone per row, the arc color (cards, sidebar rows, jump menu rows, and the joined
  row at Branch), the Needs you sort (unchanged), the By status sort (one group per working word,
  "Working" included, alphabetical; a list with a working and an idle "Designing" row), and
  unchanged rows (Chats, Fix agents,
  Queue agents, One-off, Follow-ups). Prior art: `sidebarModel.test.ts`, `EpicCard.test.tsx`,
  `EpicPage.test.tsx`, `DesignPage.test.tsx`.
- Browser seam (Playwright e2e): no new flow. `maxFlows` is 153 and the suite has 153, and
  `gx-playwright-e2es` bans flows for render checks and server rules. The claimed-kind words, the
  sort order and the dots stay in the Go and vitest tests. The live purple-versus-blue checks and
  the turn start and end checks become extra steps in the `bg-progress` flow. Writing PR text has
  no fixture scenario and no browser step: the Go row test and vitest cover it, and it takes the
  same client path as Fixing CI. Everything else is covered at the two seams above.
- No mocks (design decision). The gallery entries that draw cards may get new states for the words;
  they are not a design check.

## Verification

Each entry names the flows to run (catalog names are in `webui/e2e/CATALOG.md`), the checks, and
new steps to write. There is no new flow: the new checks are extra steps in existing flows (see
Steps to add below). The verify ticket runs these against a temporary gx server. Whether a color or
word looks right on screen is judged in that run, not before.

Stories 1 and 2 (ring colors on card and sidebar)
- Run: `bg-progress` flow with the added steps (a working Design and a working Implementation on
  `/epics` and in the sidebar); `epics.e2e.ts` "2. Epic looks right".
- Check: the Design card's and row's arc is the Design purple, the Implementation's is blue, the
  comet is white on both.

Story 3 (Design header ring)
- Run: `bg-progress` flow with the added steps, Design page at 1440px; `design.e2e.ts` "the
  Progress region and the ring show each step in turn".
- Check: with the session in a turn the header ring is purple with the comet.

Story 4 (Epic header ring)
- Run: the vitest `EpicPage` test for a running epic and for Writing PR text and Fixing CI;
  `design.e2e.ts` "Header shape"; the `pr-flow-ci-failing` scenario inside `united.e2e.ts` "16.
  Needs you" for Fixing CI.
- Check: blue ring with the comet in all three; layout unchanged at 1440px and 375px.

Stories 5 and 6 (Design words)
- Run: `bg-progress` flow with the added steps for one step by running UI; the other steps by the
  Go server tests.
- Check: the words match the map for the reported step, then the map's step, then "Designing";
  the card ends with "...", the sidebar does not.

Stories 7 to 12 (Implementation words)
- Run: the Go server table tests and the vitest sidebar and card tests. No browser flow: the word
  for each claimed kind is a server rule.
- Check: for each claimed kind (verify, code-review, conflict-resolution, trunk-conflict, other),
  the row has the expected word; with verify and code-review claimed together the row reads the
  later phase (Verifying); an epic with only a claimed ordinary or fix ticket reads "Implementing";
  merge-only reads "Merging".

Stories 13 and 14 (Writing PR text and Fixing CI)
- Run: the `pr-flow-ci-failing` scenario inside `united.e2e.ts` "16. Needs you" for the Fixing CI
  state; the Go row test and vitest for Writing PR text.
- Check: blue spinning ring in card and sidebar row; the sidebar reads "Fixing CI" with no attempt
  count; the card keeps "Fixing CI (attempt N of M)" and shows no "...".
- Check: GET `/v1/epics` row for the epic has the working flag and the expected words.

Stories 15 and 16 (Starting and the one rule)
- Run: `newdesign.e2e.ts` "27. Design a not-started Design" up to the launch; the `bg-progress`
  scenario (a Working Design session with a subagent).
- Check: while the session starts, card, sidebar row and Design header spin purple with a Design
  word; with a subagent running and the session idle the card and the sidebar both show the word.

Story 17 (Create tickets)
- Run: the Go server table tests (idle, countdown and no session, with and without an `-impl`
  epic) and the vitest card and sidebar tests. No browser flow.
- Check: sidebar "Creating tickets"; card "Creating tickets (N so far)..." with N growing; purple
  spinning ring. A Design at Create tickets with no running session shows a grey ring, "Designing"
  with "No design session is running", Continue and no count (the visible change); a countdown
  with an `-impl` epic stays working.

Story 18 (joined card at Branch)
- Run: the Go server table tests (joined row) and the vitest arc case for the joined row. No
  browser flow: the Auto-advance flow sits in the countdown at Branch, which wins over working.
- Check: at Branch with the session working and the `-impl` epic not running, the card shows a
  purple ring and "Branching". Once the `-impl` epic runs, the ring is blue and the words are
  Implementation words.

Stories 19 to 21 (dots and sorting)
- Run: the vitest sidebar model tests (Needs you sort, By status sort, dots).
- Check: no "..." in any sidebar row; the Needs you sort is Needs you first, then by name, as
  before; in By status, one group per working word ("Working" included) sits where "Working" sat,
  in alphabetical order, after Needs you and ahead of the rest; a working "Designing" row and an
  idle "Designing" row do not share a group by accident.

Story 22 (unchanged states)
- Run: `epics.e2e.ts` flows 2, 6 and 7; `Workspaces` flow; the vitest ring and sidebar tests.
- Check: for each unchanged state (Needs you, paused, done, grey, Ready for review) the ring's tone
  and arc attribute and the words equal the values the same test asserts before the change; the
  comet is white. No screenshots, since those flows take none.

Story 23 (other kinds)
- Run: `Workspaces` flow (Chats, Fix agents), `agents-sidebar.e2e.ts` "73. Agents sidebar"; the
  vitest `EpicCard` and ring tests for One-off and Follow-ups rows.
- Check: One-off and Follow-ups cards keep their words and ring (the Follow-ups arc is teal today);
  Chats, Fix agents and Queue
  agents still read "Working" with a blue ring.

Stories 24 and 25 (live updates)
- Run: `bg-progress` flow with the added steps, using `fixture.controlClaude` (or the `ask`
  helper) to start and end a turn on the Design session, with the page open.
- Check: card, sidebar row and Design header change within the same page, with no reload (the
  suite fails on a reload).

Steps to add (these become implementation tickets)
- Extra steps in the `bg-progress` flow: a working Design whose session reports a settable step,
  a purple-versus-blue check on the card, the sidebar row and the Design header, and a turn start
  and end without reload. Covers stories 1 to 3, 5, 6, 15, 16, 24 and 25.
- No fixture scenario is added for Writing PR text or for claimed verify, code-review or
  conflict-resolution tickets: the Go table tests seed them directly.

## Out of Scope

- The TUI: the Queue tab and the agents sidebar keep their words and colors.
- One-off and Follow-ups cards; Chats, Fix agents and Queue agents.
- Changing the ring for amber, paused, done or idle states, or the comet color.
- A running Design session after the Branch step (it runs in the trunk checkout). Once the
  `-impl` epic runs, or the map is Done, the joined card follows the `-impl` epic.
- Mocks: none exist and none are needed.

## Further Notes

- GLOSSARY: "Session status" keeps Working as the session's word. The epic words here are status
  words of the Epic card, not session statuses. If the implementation adds a term for "working
  word", it goes in the glossary under Epic card.
- Names for the new row fields and the detail `working` flag are implementer's choices; no ticket
  decided them.
- The existing client code that tests the status words text for "Working" and "Creating tickets"
  goes away when the rows carry the working flag and word.

## Self-decided design questions

- Tie-break between several working sessions of one Design: the newest by `status_changed_at` gives
  the reported step. Reason: it is the session the person last saw change, and it needs no new rule.
- Working rank of "Working" in By status: it shares the working rank with the new words and sorts
  A to Z with them. Reason: one rule is easier to explain than a fixed spot for one word.
- `trunk-conflict` reads "Resolving conflicts". Reason: it resolves a conflict like
  `conflict-resolution`, and "Implementing" would hide that.
