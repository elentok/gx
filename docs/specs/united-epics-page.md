# United epics page

Source: map epic `gx:united-epics-page` (decisions in its tickets 01 to 12). Prototypes:
branch `prototype/united-epics-page` (throwaway, never merged). Research:
`docs/research/united-epics-page-data.md` on branch `research/united-epics-page-data`.

> **Update 2026-10-29:** the "In progress" section was split in two. **Implementing** (running, queued,
> merging and awaiting-merge epics, and one-offs) now sits below "Needs you". **Designing** (live,
> counting-down and stopped Designs) sits between "Ready to queue" and "Ready to design". The section
> keys are `implementing` and `designing`; `in-progress` is gone. Read "In progress" below as one of
> these two, by card kind.

## Problem Statement

The web UI looks cluttered. Epic work is split over three tabs. The Queue tab shows what runs, the
Epics tab lists every epic, and the Designs tab holds the map epics and their chat. A person who
wants to know "what needs me now" has to visit all three. The Queue's "Recently finished" list and
the Epics list read as a blob of small text. A Design and its `<epic>-impl` epic show as two
unrelated things, so the person cannot see one piece of work from idea to merge. Buttons look like
small labels, not like buttons. The Design page's composer floats in a page that scrolls as a whole,
and the person loses the questions while they read the chat.

## Solution

One **Epics** tab replaces the Queue, Epics and Designs tabs. It is the home page at `/epics`. The
old routes `/q` and `/designs` redirect to it.

The page is a grid of epic cards in five sections: Needs you, In progress, Ready to queue, Ready to
design (collapsed) and Finished (collapsed). A text filter sits above them. Each card has a progress
ring, the title, and a "project · status" line in the kind's color. A Design and its `-impl` epic
share one card, which moves from Designing to Creating tickets to Implementing. A card that needs
the person has a state pill, a short reason, and at most two buttons. The text is larger and less
dense.

The shared Button gets three roles (main, secondary, ghost) and two sizes, and it looks like a
button. The Design page fills the window. The chat scrolls between the title and a bottom area that
holds the questions card and a one-line composer. The Design page and the epic page get a jump menu
on the title (⌘K) to switch epics, and a "← Epics" link above it. A "New Design" button on the Epics
page starts a new Design.

The server sends every card fact in `/v1/epics`, one row per card, so the page reads one payload.

## User Stories

### The Epics tab

1. As a person, I want one Epics tab in place of the Queue, Epics and Designs tabs, so that I look in one place for all epic work.
2. As a person, I want `/epics` to be the home page, and `/`, `/q` and `/designs` to open it, so that old links and bookmarks still work.
3. As a person, I want the top bar to show only the Epics and Stats tabs, so that the nav matches the pages that exist.
4. As a person, I want the AFK toggle and the "Paused" queue chip in the top bar on the Epics page, so that I keep the controls the Queue tab had.
5. As a person, I want the cards in five sections in a fixed order (Needs you, In progress, Ready to queue, Ready to design, Finished), so that the work that waits on me comes first.
6. As a person, I want Ready to design and Finished collapsed at first, with a toggle, so that the page stays short.
7. As a person, I want each section title to show a dot in its tone and a count of the cards on screen, so that I see how much sits in each section.
8. As a person, I want a text filter above the sections that matches the card title, so that I find an epic fast.
9. As a person, I want a filter match to open a collapsed section, and sections with no match to hide while I type, so that I see every hit.
10. As a person, I want the filter to also find finished epics older than the Finished cap, so that I can reach old work.
11. As a person, I want one list across all projects with the project on each card, so that I do not switch projects.
12. As a person, I want the grid to fit as many 360 px cards per row as the screen allows, and one column on a phone, so that the page works on any screen.
13. As a person, I want an empty state with a "New Design" button when there are no cards, so that I know how to start.
14. As a person, I want "No match" when the filter finds nothing, so that I know the filter is the reason the page is empty.

### Sections and their cards

15. As a person, I want paused, deadlocked and merge-blocked epics, epics awaiting merge in a project with auto-merge off, loop epics with a ticket question or permission waiting, and Designs that wait on me in Needs you, oldest wait first, so that I clear the oldest blocker first.
16. As a person, I want running epics first, then queued epics in queue order, then Designs by last activity in In progress, so that I see what moves now.
17. As a person, I want an epic awaiting merge in a project with auto-merge on to stay in In progress, so that a merge the server makes in seconds does not ask for me.
18. As a person, I want ready epics in Ready to queue, newest first, so that the latest plan is on top.
19. As a person, I want one follow-ups card per project with open follow-ups first in Ready to design, then Designs not yet started, draft epics and epics with no open tickets by name, so that all unplanned work sits together.
20. As a person, I want Finished to show the last `server.recently-finished` epics and one-offs (older done epics stay reachable through the filter), newest first, so that I see what landed lately.
21. As a person, I want one card per one-off ticket in the section that matches its state, so that a one-off shows next to the epics.
67. As a person, I want a Design whose first session runs before its map exists to show as a Design card titled with the session's epic name, following the normal Design rules (In progress with Open while the session runs, Needs you with Answer while it waits on an answer), to become the normal Design card once the map exists, and to go away if the session ends with no map, so that a new Design shows from its first minute.

### The epic card

22. As a person, I want each card to show a 64 px progress ring with a kind icon, a 24 px title and a "project · status" line in the kind's color, so that I read the card at a glance.
23. As a person, I want the ring to show the percentage for a Design, n/m for an Implementation, "…" while tickets are being created, and an empty ring for a one-off, so that the ring always means progress.
24. As a person, I want the status words to say whether the card is a Design or an Implementation, so that I know which part runs.
25. As a person, I want a running, merging or ticket-creating card's ring to show a moving comet, still under reduced motion, so that live work stands out without a spinner.
26. As a person, I want a state pill and a dim reason only on Needs-you cards, and a reason on other cards only when there is one, so that quiet cards stay quiet.
27. As a person, I want a Needs-you card's border tinted in its state's color, so that it stands out in a mixed grid.
28. As a person, I want only the title to be a link, so that buttons inside the card work with mouse and keyboard.
29. As a person, I want a card's buttons in a footer pinned to its bottom, so that footers line up across a row.
30. As a person, I want the follow-ups card to show its counts ("5 bugs, 2 ideas") and link to the project's follow-ups epic, so that I see the backlog size.

### The joined card

31. As a person, I want a Design and its `-impl` epic in the same project to share one card titled with the Design name, so that I follow one piece of work from idea to merge.
32. As a person, I want the joined card to show "Designing" with a percentage until Create tickets, then "Creating tickets (N so far)...", then "Implementing..." with n/m from Branch on, so that one ring tells the whole story.
33. As a person, I want the active part to set the joined card's state and section, and Needs you to win when either part needs me, so that a waiting part never hides.
34. As a person, I want the joined card's title to open the active part's epic page, so that the link goes where the work is.
35. As a person, I want an `-impl` epic whose Design is gone to show as a plain loop epic card under its own name, and a Design with no `-impl` epic to show only its design part, so that broken pairs still show.
36. As a person, I want the epic page of a Design and of its `-impl` epic to link to each other, so that I move between the two parts.

### Card actions

37. As a person, I want **Design** on a Design that has not started (its map is at Chart, no map ticket is done, and no session is live), which starts a session and opens the Design page, so that I start designing in one click.
38. As a person, I want **Answer** on a Design that waits on an answer and **Open** on a Design whose session runs, both opening the Design page, so that I reach the chat.
39. As a person, I want a stopped Design (any Design that is not started, not done, and has no live session) to sit at the end of In progress with "No design session is running" and **Continue**, which starts a session and opens the Design page, so that a stopped Design does not stall unseen. It is not in Needs you: no agent waits on me, the work is only paused.
40. As a person, I want a Design in the auto-advance countdown to stay in In progress with "Next run starts in Ns" as its reason, so that I do not act on something that resumes by itself.
41. As a person, I want **Answer** for a ticket question or permission, **Open ticket** for a paused ticket and **Open epic** for a deadlocked epic, so that I reach the page where I act.
42. As a person, I want **Queue** on a ready card and **Merge** on merge-blocked and awaiting-merge cards to act in place, so that I do not leave the page.
43. As a person, I want a refused Queue or Merge to show its reason next to the button, so that I know why it failed.
44. As a person, I want queued, running, finished and draft cards to have no buttons, so that the page shows only what needs me.
45. As a person, I want **Triage** on a follow-ups card to open a dialog with `/gx-triage-follow-ups --project <project>` and a copy button, so that I can start triage until a triage screen exists.
46. As a person, I want the skill named `gx-triage-follow-ups`, so that its name matches the card's action.

### New Design

47. As a person, I want a **New Design** button to the right of the filter that opens the new-Design form in a dialog, so that I can start a Design without a Designs tab.
48. As a person, I want Start in that dialog to open the new Design's page, so that I land in its chat.

### Buttons

49. As a person, I want every shared Button to look like a button: sentence case, 8 px corners, md 40 px by default and sm 32 px for dense rows, so that I see what I can click.
50. As a person, I want three button roles (main, secondary, ghost), so that the main action stands out and the rest step back.

### The Design page

51. As a person, I want the Design page to fill the window with the chat scrolling on its own between the title and the bottom area, so that the questions and composer never scroll away.
52. As a person, I want the questions card and a one-line composer under the chat, taking at most half the screen before the question scrolls, so that the chat stays readable.
53. As a person, I want Auto-advance on the left of the composer and Note, Focus and Send on the right, so that the main action sits where I expect it.
54. As a person, I want no tool-call lines in the chat and only a "Working: <step> · <ticket>" row while the agent works, so that the chat stays about the conversation.
55. As a person, I want the title row to hold only the Design name and the Sessions control, with Progress as the right column, so that the page header stays calm.
56. As a phone user, I want Progress as a segment bar and one line, with a tap opening the full list in a modal, so that the page content does not jump.
57. As a phone user, I want Note and Focus icon-only and the word "Auto-advance" hidden, so that the composer stays on one line.

### Switching epics

58. As a person, I want a "← Epics" link above the title on the Design page and the epic page, so that I can go back home.
59. As a person, I want the title on both pages to open a jump menu (also ⌘K or Ctrl+K) with a filter and the Epics sections as compact rows, so that I switch epics without going home.
60. As a person, I want the jump menu to hide Ready to design and Finished until I type, so that the default list is short.
61. As a person, I want arrow keys and Enter in the jump menu, and Esc or a click outside to close it, so that I can switch with the keyboard.
62. As a person, I want a pick to open a Design on its Design page and any other card where its title points, so that I land on the useful page.
63. As a person, I want the title to show a count of the other Needs-you cards, so that I know more work waits on me.
64. As a phone user, I want a tap on the title to open the jump menu as a full-screen sheet with a close button, so that it fits a small screen.

### CLI and live updates

65. As a person, I want the page and the jump menu to update 500 ms after server events and on reconnect, so that I see changes without a reload.
66. As a person, I want `gx server epics` to print one line per card, with the card's section, so that the CLI shows what the page shows.

## Implementation Decisions

### Server: one row per card

- `/v1/epics` sends one row per card. Today's facts stay, and the row adds the facts that lived only
  in the snapshot: pause and deadlock reasons, ticket questions and permissions, design session marks
  (live, waiting on an answer, failed, auto-advance countdown deadline), Designs with no map yet, and
  one-offs (queued, running, paused, asking, and finished within the `server.recently-finished` cap).
- One Go function joins a Design with its `<map>-impl` epic in the same project and decides the
  card's kind, state, section, status words, ring progress, reason and actions. The web client does
  not derive sections. The implementer picks the row shape and its field names. A joined row must
  still carry both addresses, so that the epic pages can link to each other.
- Card kinds: Design, Implementation, One-off, Follow-ups. A joined card is a Design until Branch,
  then an Implementation.
- Section rules (ticket 02 and 04):
  - Needs you: paused, deadlocked, merge-blocked; awaiting merge with auto-merge off; a loop epic or
    one-off with a ticket question or permission waiting; a Design whose session waits on an answer.
    Sort by the oldest wait first. The wait starts when the ticket paused or asked,
    when the merge failed, when the epic reached awaiting merge, or when the Design session posted its
    question. With no such time, use the last activity.
  - In progress: running, queued (not started), merging, awaiting merge with auto-merge on, Designs
    with a live session (including the auto-advance countdown and a Design with no map yet), joined
    cards in Creating tickets, stopped Designs.
    Running first, then queued in queue order, then live Designs by last activity, then stopped
    Designs by last activity.
  - Ready to queue: ready epics, newest first.
  - Ready to design: follow-ups cards first (one per project with open follow-ups), then Designs not
    started, draft epics and epics with no open tickets, by name.
  - Finished: `/v1/epics` carries every done epic, and only the one-offs within the
    `server.recently-finished` cap, newest first. The page shows the first
    `server.recently-finished` cards until the person types a filter; the filter and the jump menu
    search all of them.
- Not started and stopped Designs: a Design is Not started when its map is at Chart, no map ticket
  is done, and no session is live (today's `mapNotStarted` rule). Any other Design that is not done
  and has no live session is Stopped. Session history plays no part: session records can be deleted,
  and a Design worked on from a terminal has none.
- The project's `follow-ups` epic never gets a plain card. It shows only as the follow-ups card, and
  not at all when it has no open follow-ups.
- Joined card progress (ticket 03): Designing total = 7 steps (Chart to Branch) + map tickets − 1;
  done = steps passed + closed map tickets. The percentage can go down. Creating tickets counts the
  live tickets in the `-impl` epic. From Branch on, progress is the `-impl` epic's n/m. The card does
  not wait for the map's done status.
- Status words (prototype `united-epics-page/05`, round 6):
  - Design: "Designing", "Creating tickets (N so far)...", "Not started",
    "Designed" (done Design with no `-impl` epic; the card sits in Finished).
  - Implementation: "Implementing...", "Merging...", "Implemented" (awaiting merge), "Queued #N",
    "Not started", "Merged" for Finished.
  - Other: "One-off", "Draft", "No open tickets", the follow-ups counts ("5 bugs, 2 ideas";
    follow-ups with no category count as "N other").
- State pill words (Needs you only): Question (a Design or a ticket waiting on an answer),
  Permission, Paused, Deadlocked, Merge blocked, Awaiting merge.
- Reason text: the implementer picks the words. Examples from the prototype: "3 questions on the
  Destination", "Auto-advance is off", "06 Retry the launch: zero commits after the iteration",
  "05 and 07 each wait on the other", "Conflicts with main in server/land.go", "Auto-merge is off for
  blog", "Wants to run: pnpm add -D @types/safari", "Starts after agent-runner-phase2", "Next run
  starts in 12s". A countdown reason ticks on the client from the deadline.
- When several tickets of one epic are paused or asking, the implementer picks which one the reason
  and the button name (oldest wait is the natural pick).
- Queue mode stays in the snapshot. Queued cards say "Queued #N" whatever the mode. The top bar's
  "Paused" chip shows a paused queue.
- A Design with no map yet gets a card keyed by its session's epic name, titled with that name. It
  follows the normal Design rules: In progress with Open while the session runs, Needs you with
  Answer while it waits on an answer. Once the map exists, the row becomes the normal Design card.
  When its session ends, fails or is removed with no map, the card goes away. It has no epic page:
  its title opens `/d/<project>:<epic>`. The implementer picks the key.

### Card actions (ticket 04 and 12)

| Card | Section | Buttons | What it does |
| --- | --- | --- | --- |
| Design, not started | Ready to design | Design | POST open-design, then open `/d/<epic>` |
| Design, session waits on an answer | Needs you | Answer | open `/d/<epic>` |
| Design, session runs | In progress | Open | open `/d/<epic>` |
| Design, no session, not done | In progress | Continue | POST open-design, then open `/d/<epic>` |
| Design, auto-advance countdown | In progress | none | |
| Loop epic or one-off, ticket question or permission | Needs you | Answer | open the ticket page |
| Loop epic or one-off, paused ticket | Needs you | Open ticket | open the ticket page |
| Deadlocked epic | Needs you | Open epic | open the epic page |
| Ready | Ready to queue | Queue | POST in place |
| Merge-blocked, awaiting merge with auto-merge off | Needs you | Merge | POST in place |
| Follow-ups | Ready to design | Triage | open the triage dialog |
| Queued, running, merging, finished, draft, no open tickets | any | none | |

- No card has two buttons today. The card still allows two (main, then secondary) for later use.
  Merge-blocked cards have no Open epic button; the title link covers it (ticket 12, overrides the
  prototype).
- A refused write shows its reason inline next to the button, as `EpicActionButton` does today.
- Only the title is a link. Targets: the active part's epic page for epics and joined cards, the
  ticket page for a one-off, the project's `follow-ups` epic page for a follow-ups card.
- The triage dialog says "Triage UI not yet implemented, for now run:" with
  `/gx-triage-follow-ups --project <project>` and a copy-to-clipboard button. The header follow-ups
  counts line goes away.
- The skill `gx-resolve-follow-ups` becomes `gx-triage-follow-ups`, with every mention updated.

### Web: the Epics page

- New Epics page at `/epics` replaces `QueueScreen`, today's `EpicsPage` and the Designs tab
  (`/designs` without an address and the `Picker` sidebar). `/`, `/q`, `/designs` and unknown hashes
  redirect to `/epics`. The top bar tabs are Epics and Stats.
- The page reads only `/v1/epics` through the existing `useFetched` hook: fetch on mount, refetch
  500 ms after events settle and on reconnect, no polling. It repaints from a `sessionStorage` copy
  after a reload and shows the centered `Spinner` before the first data.
- Layout (prototype variant A11d with the `comet-tail` ring):
  - Grid `repeat(auto-fill, minmax(min(100%, 360px), 1fr))`, 20 px gaps.
  - Section title: 24 px condensed, sentence case, 10 px tone dot, count at 18 px dim. Ready to
    design and Finished start collapsed with a ▸/▾ toggle.
  - Filter: 40 px tall, 16 px text, max 320 px wide, matches the card title. The New Design button
    (main role) sits to its right.
  - Card: 16 px corners, 20 px padding, no dividers, a flex column. Header: 64 px ring (5 px stroke,
    15 px condensed tabular label inside, 26 px kind icon badge on its bottom right), title 24 px
    condensed 600, status line 15 px with the project dim, then " · ", then the status in the kind
    color. Body (Needs you or a reason): the pill (sentence case, 7 px dot, 14 px, 26 px tall, state
    fg on bg) and the reason (14 px dim, two lines max). Footer only with buttons, right-aligned,
    `margin-top: auto`. Buttons wrap when they do not fit.
  - Needs-you border: `color-mix(in srgb, <state fg> 40%, <border>)`.
  - Kind colors as theme tokens next to the state tones: Design violet `#c4a7ff`, Implementation
    teal `#7ad8d0`, One-off pink `#f0a6ca`, Follow-ups grey `#b4bcc8`, each with a bg near 0.13
    alpha. Icons: pen (Design), `</>` (Implementation), bolt (One-off), inbox (Follow-ups).
  - Ring motion: a white comet with a fading tail (10 segments, opacity rising to the head, 16% of
    the ring) goes round once every 1.6 s, over fill and track. It runs on running, merging and
    Creating tickets cards. `prefers-reduced-motion` keeps it still.
- The empty state shows the New Design button. The filter's empty result shows "No match".
- The implementer picks the component split (card, ring, section, filter, triage dialog).

### New Design (ticket 12)

- The New Design button opens today's `NewDesignForm` (epic name, rough idea, project select) in a
  dialog. Start posts open-design with `new_design`, then opens `/d/<project>:<epic>`. Cancel closes
  the dialog. The jump menu has no New Design entry.
- The Design page keeps the form for "New session" through its Sessions control.

### Shared Button (ticket 06, style A)

- Three roles replace the four variants: main (today's `solid` and `tinted`: solid tone fill, text in
  the surface color, hover `brightness(1.12)`), secondary (today's `outline`: `surface.raised` fill,
  1 px `border`, `text` color, hover border `text.dim`), ghost (no fill or border, tone text, muted
  tone uses `text.muted`, hover fills the tone bg).
- Sizes: sm 32 px tall, 12 px side padding, 14 px text; md (default) 40 px, 16 px, 15 px.
- 8 px corners, 1 px border, 6 px icon gap. Saira, weight 600, sentence case, normal letter spacing.
- Focus: 2 px outline in the tone, 2 px offset. Pressed: moves down 1 px. Disabled: 50% opacity.
- Every page that uses Button gets the new look. Composer buttons that pass `height="auto"` may drop
  it; the implementer decides.
- Light mode is out of scope.

### Design page (ticket 07, layout A; ticket 10)

- The page fills `100dvh`: top bar, title row, then the chat column and the 300 px Progress column.
- The title row holds the "← Epics" link above the title, the title as the jump menu button, and the
  Sessions control on the right. No state label.
- The chat scrolls alone and opens at the newest message. No tool-call lines. While the agent works,
  a spinner row reads "Working: <step> · <ticket>".
- The bottom area sits under the chat in the page flow, at most 50% of the screen height. Past that
  only the question part scrolls. The questions card has dots, "N of M answered" and a fold toggle,
  the current question, then Prev and Next, all with 12 px corners (text boxes and the note box too).
- The composer bar is one line. Left: a real Auto-advance toggle with its word, then the countdown
  and Cancel while one runs. Right: the next ticket's name (before Continue), Note and Focus
  (secondary), then Send. Interrupt replaces Send while the agent works; Continue replaces it after a
  run-end line. Note opens the note box above the bar.
- Progress marks are 16 px icon boxes so labels line up.
- Phone: Note and Focus are icon-only, "Auto-advance" hides, the 50% cap stays. Progress is a
  segment bar plus one line under the title; a tap opens the full list in a modal.
- The Designs `Picker` and `MobilePicker` go away.

### Jump menu (ticket 08 and 10)

- On the Design page and the epic page. The title is a button: the name, ▾, a count of the other
  Needs-you cards, and a "⌘K" hint (hidden on a phone).
- Click or ⌘K/Ctrl+K opens a menu under the title, about 440 px wide (screen minus 32 px on a
  phone), list scrolling past 60% of the screen height. Esc or a click outside closes it.
- A filter on top, focused on open. Rows grouped by section: 26 px ring in the kind color with the
  kind icon, the name, and "project · status" (the pill word on Needs-you rows). The current epic is
  marked. Ready to design and Finished show only while typing; typing searches every section. Up and
  Down move the cursor, Enter opens.
- A pick opens a Design card on `/d/<epic>`. Any other card opens its title link target.
- On a phone, a tap on the title opens a full-screen sheet with the filter, compact rows and a close
  button.
- It reads the same `/v1/epics` data and hook as the Epics page.

### Epic page

- "← Epics" link above the title replaces "← Queue". The title becomes the jump menu button.
- The epic page of a Design links to its `-impl` epic page, and the `-impl` epic page links to the
  Design's epic page. The implementer picks the link label and place.
- No new type scale for the epic page (out of scope).

### CLI (ticket 12)

- `gx server epics` prints one tab-separated line per card: the active part's address (the Design
  until Branch, then the `-impl` epic; the ticket address for a one-off; the follow-ups epic for a
  follow-ups card; `<project>:<epic>` for a Design with no map), the card kind, the state, done/total, `queueable=`, and a new section column with one of `needs-you`, `in-progress`, `ready-to-queue`,
  `ready-to-design`, `finished`.
- `gx server epics` prints only the capped Finished cards; a flag (the implementer names it) prints
  all of them.
- `gx server epics --json` prints the rows as the route sends them.
- `gx server epics detail` is unchanged.

## Testing Decisions

- A good test drives the public seam and checks what a caller sees: the JSON of `/v1/epics`, the
  CLI output, and the rendered page by role and visible text. It never checks internal helpers.
- Seam 1, server: the `/v1/epics` route, through `servertest` fixtures
  (`servertest.WriteTicketWith`) and its session helpers (`WithTestHolder`, `WithChatHarness`,
  `WithAutoAdvanceDelay`) for the design session marks (live, waiting on an answer, countdown,
  stopped). One table test per section rule, per Needs-you reason, per joined
  card phase (Designing percentage, Creating tickets count, Implementing n/m), per broken pair, and
  per action. Prior art: `server/epiclist_test.go`, `server/mappicker_test.go`,
  `server/mapprogress_test.go`.
- Seam 2, CLI: `printEpicList` output for each card kind. Prior art: `cmd/server_parity_test.go`
  and the existing `gx server epics` tests.
- Seam 3, web: page components with an injected fetcher, through `make web-test` (Vitest). Prior
  art: `epics/EpicsPage.test.tsx`, `queue/QueueScreen.test.tsx`, `design/DesignPage.test.tsx`,
  `routes.test.tsx`, `ui/top-bar/TopBar.test.tsx`.
- Seam 4, browser: the web e2e flow catalog (`webui/e2e/CATALOG.md`, `make web-e2e`) against
  `gxfixture` servers.
- Port flows 1 to 8 to the Epics page. They keep their numbers. Only the nav steps change, except:
  flow 7 checks the Finished section, and flow 8 shows `empty-epic` in Ready to design.
- New flows: Needs you, Joined card, Jump menu, Redirects, Phone (the first phone-size flow), New
  Design, Follow-ups triage. New fixture scenario with one card per section and per Needs-you
  reason; the implementer names it. The joined card reuses `design` (`design-epic` and
  `design-epic-impl`).
- Gallery: add the epic card (each state, Needs you with buttons, the joined card), the ring (still
  and animated) and the jump menu rows. Update Button to three roles and two sizes. Update the TopBar
  cases for the new tabs.
- The Go e2e tests in `e2e/` cover the TUI and stay as they are.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture`). "The new scenario" is the
fixture scenario with one card per section and per Needs-you reason. Catalog flows are in
`webui/e2e/CATALOG.md`. Look checks use `gx-ui-check` against the fixture.

1. Story 1: ported flow 1 (Queue an epic) runs from the Epics page; the nav has no Queue or Designs tab.
2. Story 2: new flow "Redirects": `#/`, `#/q`, `#/designs` and an unknown hash land on `#/epics`.
3. Story 3: new flow "Redirects" checks the nav reads Epics and Stats; gallery TopBar cases show the same.
4. Story 4: the new scenario with the queue paused (`gx server queue pause` or the scenario's mode): the Epics page top bar shows "Paused" and the AFK toggle; clicking AFK flips `gx server` AFK state.
5. Story 5: new flow "Needs you" on the new scenario: section titles show in order Needs you, In progress, Ready to queue, Ready to design, Finished.
6. Story 6: new flow "Needs you": Ready to design and Finished start collapsed; the toggle opens them.
7. Story 7: new flow "Needs you": each section title shows the count of its cards; the Finished count is the cards on screen (the cap with no filter, the matches while typing).
8. Story 8: ported flow 2 (Epic looks right): typing `mixed` in the filter leaves only `mixed-epic`.
9. Story 9: ported flow 8 (Edge states): typing `empty` opens Ready to design and shows `empty-epic`; other sections hide.
10. Story 10: new scenario with more finished epics than `server.recently-finished`: an old one is missing from Finished and shows after typing its name; `/v1/epics` carries it; the jump menu finds it by name; `gx server epics` leaves it out, and with the all-finished flag prints it. A scenario with a few hundred finished epics loads the Epics page and filters it with no visible lag.
11. Story 11: the new scenario spans two projects; `gx server epics` lists cards from both; each card shows its project.
12. Story 12: new flow "Phone": at 375 px the grid is one column; a `gx-ui-check` look at desktop width shows several columns.
13. Story 13: new flow "New Design": a fixture with no cards shows the empty state with a New Design button.
14. Story 14: new flow "Needs you": a filter with no match shows "No match".
15. Story 15: new flow "Needs you": a paused ticket, a merge-blocked epic, an awaiting-merge epic with auto-merge off, a ticket question, a permission and a Design question each show in Needs you; `gx server epics` prints them with section `needs-you` in oldest-wait order.
16. Story 16: `gx server epics` on the new scenario prints In progress cards running first, then queued in queue order, then Designs.
17. Story 17: the new scenario with auto-merge on: `gx server epics` shows the awaiting-merge card in In progress before the server merges it.
18. Story 18: `gx server epics` prints Ready to queue cards newest first; ported flow 1 shows `ready-epic` there.
19. Story 19: new flow "Follow-ups triage": the follow-ups card is first in Ready to design; `gx server epics` lists it before Designs, drafts and no-open-tickets epics, which sort by name. Server table tests: the `follow-ups` epic gets no plain card, and a project with no open follow-ups gets no follow-ups card.
20. Story 20: ported flow 7 (Epic completion): `finishing-epic` shows in Finished as `2/2` "Merged" after the release.
21. Story 21: the new scenario has a queued, a paused and a finished one-off: each shows as its own card in In progress, Needs you and Finished; `gx server epics` prints each with kind One-off and its ticket address.
22. Story 22: gallery epic card cases; ported flow 2 shows `mixed-epic` with "demo · Implementing..." and `2/7` in the ring.
23. Story 23: gallery ring cases (percentage, n/m, "…", empty); new flow "Joined card" reads the ring label in each phase.
24. Story 24: new flow "Joined card" checks the status words "Designing", "Creating tickets (N so far)...", "Implementing...".
25. Story 25: gallery animated ring case; a `gx-ui-check` look with reduced motion on shows a still ring.
26. Story 26: new flow "Needs you": pills and reasons show only on Needs-you cards; the Ready to queue card with a blocker shows its reason and no pill.
27. Story 27: gallery Needs-you case; a `gx-ui-check` look shows the tinted border.
28. Story 28: new flow "Needs you": clicking a card outside its title and buttons does not navigate; the title opens the epic page.
29. Story 29: gallery case with two cards in a row, one with a footer: the footer sits at the card bottom.
30. Story 30: new flow "Follow-ups triage": the card reads the counts, and its title opens the project's `follow-ups` epic page.
31. Story 31: new flow "Joined card" (scenario `design`): one card titled `design-epic`; no separate `design-epic-impl` card; `gx server epics` prints one line for the pair.
32. Story 32: new flow "Joined card": `gx tickets set demo:design-epic --design-step create-tickets`, then `branch`; the status moves from Designing to Creating tickets to Implementing.... Server table test checks the percentage formula at Resolve tickets, 4 map tickets, 2 closed: 3 of 10 = 30%.
33. Story 33: server table test: a joined card with a Design question and a queued `-impl` sits in Needs you; at Done with a queued `-impl` it sits in In progress.
34. Story 34: new flow "Joined card": before Branch the title opens `#/e/demo:design-epic`, after Branch `#/e/demo:design-epic-impl`.
35. Story 35: server table tests for an `-impl` epic with no Design and a Design with no `-impl`; `gx server epics` prints the first under its `-impl` name.
36. Story 36: new flow "Joined card": the `design-epic` epic page links to `design-epic-impl`, and back.
37. Story 37: new flow "New Design": open Ready to design, click Design on a not-started Design; it opens `#/d/<epic>` and `gx agents list` shows a design session.
38. Story 38: ported flow 4 (Start a design session) starts from the Epics page; with the session running the card shows Open; with a round waiting it shows Answer; each opens the Design page.
39. Story 39: server table tests: a Design at Chart with no done map ticket and no live session is Not started in Ready to design; a Design at Spec with no live session is Stopped in In progress (originally Needs you). New scenario Design with no session at Spec: the card is in In progress with Continue; Continue opens the Design page and `gx agents list` shows a session.
40. Story 40: flow 10 (Auto-advance) plus a look at the Epics page during the countdown: the card is in In progress with "Next run starts in Ns".
41. Story 41: ported flow 3 (Pause reasons): each paused card on the Epics page shows Answer or Open ticket and opens the ticket page; the new scenario's deadlocked epic shows Open epic.
42. Story 42: ported flow 1 (Queue in place); new flow "Needs you" clicks Merge on the merge-blocked and awaiting-merge cards; `gx server epics` shows the change; both cards show only Merge.
43. Story 43: web test: a refused Queue shows the server's reason next to the button.
44. Story 44: new flow "Needs you": queued, running, finished and draft cards have no buttons.
45. Story 45: new flow "Follow-ups triage": Triage opens the dialog with `/gx-triage-follow-ups --project demo` and a copy button.
46. Story 46: `rg gx-resolve-follow-ups` in the repo and skills finds nothing; the `gx-triage-follow-ups` skill exists.
47. Story 47: new flow "New Design": the button sits right of the filter and opens the form in a dialog.
48. Story 48: new flow "New Design": fill the name and idea, Start; the page is `#/d/demo:<name>` and `gx server epics` lists the new Design.
49. Story 49: gallery Button cases at sm and md; a `gx-ui-check` look on the epic page shows the new buttons.
50. Story 50: gallery Button cases for main, secondary and ghost in each tone.
51. Story 51: flows 5 and 13 on a long round: the chat scrolls while the composer stays at the bottom.
52. Story 52: flow 13 (Defaulted questions) with a long round: the bottom area stops at half the screen and the question scrolls.
53. Story 53: flow 12 (auto-advance switch) finds the switch on the left; flow 11 finds Note on the right.
54. Story 54: flow 14 (Working row): the spinner row shows and no tool-call line appears.
55. Story 55: flow 4: the title row shows the Design name and Sessions, no state label; Progress is the right column.
56. Story 56: new flow "Phone": the Progress segment bar opens a modal with the full list.
57. Story 57: new flow "Phone": Note and Focus are icon-only and "Auto-advance" is hidden.
58. Story 58: new flow "Jump menu": "← Epics" on the Design page and the epic page opens `#/epics`.
59. Story 59: new flow "Jump menu": the title click and ⌘K both open the menu with the filter focused.
60. Story 60: new flow "Jump menu": Ready to design and Finished rows appear only after typing.
61. Story 61: new flow "Jump menu": Down, Enter opens the next row; Esc closes the menu.
62. Story 62: new flow "Jump menu": picking `design-epic` opens `#/d/demo:design-epic`; picking a loop epic opens `#/e/...`; picking a one-off opens its ticket page.
63. Story 63: new flow "Jump menu" on the new scenario: the title shows the count of the other Needs-you cards.
64. Story 64: new flow "Phone": a tap on the title opens a full-screen sheet; the close button closes it.
65. Story 65: ported flow 6 (Outside changes): `gx server queue add demo:ready-epic` moves the card to In progress with no reload.
66. Story 66: `gx server epics` on the new scenario prints one line per card with address, kind, state, done/total, `queueable=` and section; `gx server epics --json` prints the rows; exit code 0.
67. Story 67: server table test: a design session running for an epic with no map gives one In progress Design card titled with the session's epic name; while the session waits on an answer the card is in Needs you with Answer; after the map is written, the same card is the normal Design card; when the session ends with no map, the card is gone. New flow "New Design": the card's title opens `#/d/<project>:<epic>`.

New flows and scenarios to write (implementation tickets): Needs you, Joined card, Jump menu,
Redirects, Phone, New Design, Follow-ups triage, and the fixture scenario with one card per section
and per Needs-you reason (two projects, one-offs, a follow-ups epic, auto-merge on and off, a
deadlock, a stopped Design, more finished epics than the cap).

## Out of Scope

- The TUI. It keeps its own Queue tab, and the Go e2e tests in `e2e/` stay.
- A new type scale for the whole web UI (ticket page, epic page, stats).
- Folding the design chat into the epic page.
- A real triage screen for follow-ups (follow-up `gx:follow-ups/04`).
- Light mode for the new Button.
- Pause and Remove from queue on the card. They stay on the epic page and in the TUI.
- A queue-paused mark on queued cards.

## Further Notes

- The prototypes on `prototype/united-epics-page` are throwaway. Run `pnpm dev` in `webui/` and open
  `#/epics?variant=A11d&ring=comet-tail`, `#/buttons-prototype?button=A` and
  `#/d-prototype?variant=A&case=long&cap=50&button=A&switcher=C` to see the chosen look. Where a
  prototype and this spec differ (the merge-blocked card's Open epic button), the spec wins.
- `GLOSSARY.md` gains "Epic card" and the card kinds if the implementer adds them; "Design" keeps its
  meaning.
