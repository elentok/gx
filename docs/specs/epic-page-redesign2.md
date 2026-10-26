# Epic page redesign 2

## Problem Statement

The `epic-page-redesign` epic shipped a new Epic page in the web UI, but the page does not look like
the approved mocks. A side-by-side check found 48 differences (D1 to D48 in
`docs/research/epic-page-redesign2-differences.md`). The biggest ones:

- The page switches to its wide layout at 992px. A 760px window gets the phone layout, while the
  mock shows the desktop layout there.
- Ticket cards, done cards and the More stats card have no card styling. A ticket is a bare link
  with a coloured title.
- The card meta follows the title instead of sitting at the card's right end. The id has no fixed
  width, so the titles do not line up.
- The meta prints raw status words such as "claimed · 3m" and "needs-answer · Waiting for your
  answer".
- The land-block notice is plain red text with no box, no icon and no "Can't merge" prefix.
- The back link, the summary, the stats line and the done divider use brighter text and other sizes
  than the mocks.

## Solution

The Epic page takes the mocks' look. Decisions made after the mocks still win on behaviour.

- The shared page container gets two page modes. "Narrow" pages (Epic page, Ticket page) are one
  column and switch to the wide layout at 640px. "Wide" pages (Design page) have a sidebar and
  switch at 992px, as today. Every part that reads the breakpoint follows its page's mode.
- The back link, the header, the stats line and the bar take the mock sizes and colours.
- Every live ticket is a card. The id is a fixed-width monospace column, the title is white and cut
  with an ellipsis, and the meta sits at the card's right end. Running and needs-answer cards use the
  mock wording ("running · 4m · $0.62", "needs answer").
- Needs-answer and needs-repair cards are tinted red cards. Done and cancelled tickets are
  outline-only cards.
- The land-block notice is a tinted box with an info icon and "Can't merge: <reason>". The
  merge-blocked buttons and the dirty-trunk file list sit inside it.
- A merged epic reads "Merged" on the Epic page.
- The page frame, the progress ring and every button style stay as built.

## User Stories

Page modes and the back link:

1. As a person on the Epic page in a window 640px or wider, I want the wide layout (64px ring,
   summary beside the title, buttons right of the stats line, card meta on the title's line), so
   that a mid-size window shows the desktop layout from the mocks.
2. As a person on the Ticket page, I want it to switch layouts at 640px like the Epic page, so that
   the two one-column pages agree at every width.
3. As a person on the Design page, I want its sidebar layout to keep switching at 992px, so that
   the sidebar and the composer still have room.
4. As a person using the jump menu, I want it to open as a popover or as a full-screen sheet by its
   page's mode, so that the menu matches the layout around it.
5. As a person on the Epic, Ticket or Design page, I want the back link in 14px dim text with 28px
   below it (24px on phones), so that it reads as quiet navigation, as in the mocks.

Header:

6. As a person on the Epic page, I want the ring and the title block centred on each other with an
   18px gap (14px on phones), so that the header looks balanced.
7. As a person on the Epic page, I want the title at 30px (24px on phones) with a little more line
   height, so that long names fit on a phone.
8. As a person on the Epic page, I want the summary in 15px muted text, 4px under the title (a full
   row 12px under the ring row on phones), so that it reads as support text under the name.

Stats bar:

9. As a person on the Epic page, I want a faint hairline above the bar with 14px of space on each
   side (12px on phones), so that the bar sits apart without a heavy line.
10. As a person on the Epic page, I want the stats line in dim grey, so that the numbers do not
    compete with the title.
11. As a person on a Design or a draft -impl epic, I want the step word ("Resolve tickets",
    "Publishing tickets") one step brighter than the rest of the line, so that I see the state
    first.
12. As a person on a draft -impl epic, I want the stats line to read "Publishing tickets · N
    tickets so far" with no cost or agent time, so that I see how far the publishing got.
13. As a person on a merged epic, I want the state word to read "Merged", so that the Epic page
    agrees with the Epics page cards.

Land-block notice:

14. As a person on a merge-blocked epic, I want the reason in a tinted red box with an info icon,
    reading "Can't merge: <reason>", 14px under the bar, so that I see at once why the merge
    stopped.
15. As a person on a merge-blocked epic, I want the recovery buttons inside the notice and no Merge
    button in the bar, so that the fix sits next to the reason.
16. As a person on an epic blocked by local trunk changes, I want the notice to read "Can't merge:
    Trunk has local changes", then list every clashing file, then offer "Stash, merge and restore"
    and "Merge again", so that I can decide with the facts in front of me.

Ticket cards:

17. As a person on the Epic page, I want each live ticket in a raised card with a faint border and
    rounded corners, so that one ticket is easy to tell from the next.
18. As a person watching a running ticket, I want its card border in faint blue, so that the live
    ticket stands out.
19. As a person scanning the list, I want the id in small dim monospace in a fixed-width column, so
    that all titles start at the same place.
20. As a person scanning the list, I want live titles in white on one line, cut with an ellipsis,
    with only the dot carrying the colour, so that the list stays calm.
21. As a person on the Epic page in the wide layout, I want the meta in small dim text at the card's
    right end, so that the title has the space.
22. As a person with a running or needs-answer ticket, I want its meta in the ticket's colour, so
    that the state reads at the right end too.
23. As a person with a running ticket, I want its meta to read "running · <time> · <cost>", with
    empty parts dropped and "fork of NN" at the end, so that it uses one plain word.
24. As a person on a Design, I want a running design ticket's meta to say "running" too, so that
    both kinds use the same word.
25. As a person with a ticket question, I want its meta to read only "needs answer", so that the
    question below it carries the detail.
26. As a person scanning the list, I want an 8px status dot, and a 10px spinning ring with a faint
    track for a running ticket, so that the marks match the mocks.
27. As a person on the Epic page, I want 8px between cards and 28px between the bar and the list,
    so that the list reads as one group.
28. As a person on an epic with forks, I want a fork's card indented 28px (20px on phones) and
    narrowed by the same amount, so that I see where it came from.
29. As a person with a ticket question, I want the card tinted red, the title a little bolder and a
    second row indented under the title with the question filling it and Answer on the right, so
    that the question is easy to find.
30. As a person with a needs-repair ticket, I want the same tinted red card with "Fix it with an
    agent" at the second row's end, so that both "needs you" cards look alike.
31. As a person on a phone, I want Answer and "Fix it with an agent" full width and 44px tall, so
    that I can tap them.

Done and cancelled tickets:

32. As a person on the Epic page, I want the done divider in small dim text with faint lines, so
    that it does not compete with the live cards.
33. As a person on the Epic page, I want each done ticket in an outline-only card with a muted title,
    a dim monospace id and the meta at the right end, so that finished work looks finished.
34. As a person on the Epic page, I want a plain green check stroke as the done mark, so that it
    matches the mock.
35. As a person on an epic with a cancelled ticket, I want an outline-only card with a muted dot, a
    dim title and "cancelled" as its meta, so that I can tell it from a merged one.

More stats:

36. As a person on the Epic page, I want the More stats card raised with a faint border and rounded
    corners, 28px under the list, so that it matches the ticket cards.
37. As a person on the Epic page, I want a 16px dim chevron, a 15px medium "More stats" label and a
    13px dim preview at the right, so that the card reads like the mock.
38. As a person on a phone, I want "More stats" on the top row and the preview under it, so that
    neither wraps into a narrow column.

Phone layout:

39. As a person on a phone, I want each card on two lines (title line, then the meta indented under
    the title) with the title cut by an ellipsis, so that long titles do not push the meta away.
40. As a person on a phone with a needs-answer or needs-repair ticket, I want the meta, the
    question and the button on their own rows, indented under the title, so that each part has
    room.
41. As a person on a phone, I want done and cancelled cards on two lines with their outline, so
    that they match the live cards.

## Implementation Decisions

### Scope and sources

- The web Epic page, the shared page container, `useWide`, the back link, the page header and the
  jump menu change. No server or CLI change.
- Look values come from `docs/research/epic-page-redesign2-differences.md` (the mock column) and the
  mock artifact https://claude.ai/artifact/J4FpZtrjBHQMqJRcCoeJGX (rows "Combined: C + H3 + S1",
  "Combined at phone width", "Bar states" option a). Use the research doc first. Read the mock when
  a value is missing.
- The mocks win on look. Later decisions win on behaviour: the "Fix it with an agent" button, the
  Design ring tone rule, hidden design times under 1 minute, the jump menu title, and the
  rendered-word metas of other statuses.
- This epic needs the whole `recover-merge-error-impl` epic first (`docs/specs/recover-merge-error.md`).
  That epic adds the epic detail `actions` list, the merge-blocked buttons, the dirty-trunk file list
  and the shared write hook. This epic places and styles them.

### Kept as built

These differences stay as they are. They need no ticket:

- D2: the top bar, the page width and the page padding.
- D6: the jump menu caret and the ⌘K hint.
- D8: the kind badge.
- D9: the comet shape.
- D10: the ring tone taken from the joined card.
- D14: "Awaiting merge" in waiting-yellow.
- D16 to D19, D23 and D35: every button style on every page. No new `Button` variants. This covers
  Answer, "Fix it with an agent", "Show N more" and the disabled Queue.
- D40: the More stats preview wording.
- D44: the publishing bar's button order.

### Page modes

- The shared page container takes a page mode: "narrow" or "wide". The implementer picks the prop
  name and how it reaches the components (a context is the obvious fit).
  - "Narrow": one column, wide layout from 640px (40em). The Epic page and the Ticket page.
  - "Wide": a sidebar, wide layout from 992px (62em). The Design page.
  - The Epics page and the Stats page have no part that reads the breakpoint today. The implementer
    picks their mode.
- `useWide` answers by the page's mode. The gallery's forced override still wins.
- Every part that reads the breakpoint follows the mode: the jump menu (popover or sheet, ⌘K hint),
  the back link's count text, the Epic page header and bar, the cards and More stats. Chakra `md`
  props on the Epic page (the card meta wrap) go away, so one breakpoint drives the page. The
  implementer picks how the cards get the flag (the hook or a prop).
- The Design page's own `useWide` readers (the composer, the New Design sheet, the auto-advance bar)
  keep 992px through the "wide" mode.

### Back link and header

- The back link is 14px in `text.dim` (#a3acba). The space under it is 28px in the wide layout and
  24px on phones. This applies on the Epic, Ticket and Design pages. The waiting count part keeps its
  13px waiting tone.
- The header row with a lead (the Epic page ring) centres its items, with an 18px gap (14px on
  phones).
- The jump menu title is 30px with line height 1.1 in the wide layout and 24px with line height 1.15
  on phones, with `overflow-wrap: anywhere`. It is one component, so the Design page title takes the
  same sizes by its own mode. The one-line clamp, caret and ⌘K hint stay.
- The summary is 15px, line height 1.5, `text.muted` (#c9d0da), 4px under the title. On phones it is
  a full-width row 12px under the ring row.

### Stats bar

- Hairline 1px #252c36. Space above the bar and `padding-top` are 14px each (12px on phones).
- The stats line is 14px `text.dim`, line height normal.
- State word colours:
  - Loop state words keep their tone colours. "Awaiting merge" stays waiting-yellow.
  - The Design step word and "Publishing tickets" have no tone. They render in `text.muted`, one step
    brighter than the line. The implementer picks how a stat part marks this.
- A draft -impl epic's line has two parts: "Publishing tickets", then "N tickets so far".
  - N counts every row of the epic except cancelled ones.
  - One ticket reads "1 ticket so far". At 0 the part is dropped.
  - The cost, the wall clock and the agent time leave this line.
- The web label for a finished epic becomes "Merged". `epicBadge` serves only the Epic page, so the
  Epics page is untouched. Its cards already read "Merged" from the server. The comment that says
  the labels match the TUI's `epicStateLabels` changes to name the gap: the TUI still says
  "Finished", tracked in gx:follow-ups/46.

### Land-block notice

- It shows when the epic is paused with a land-block reason, as built.
- Box: background rgba(255,112,102,0.10), radius 10px, padding 10px 14px, gap 10px, 14px under the
  bar. A 16px info-circle icon in #ff7066 on the left. Text 14px, line height 1.5, #ffb3ad.
- Text: "Can't merge: <reason>".
- `dirty-trunk`:
  - The reason reads "Trunk has local changes", the same short reason as the Epics page card. It
    does not use the capped five-file reason text.
  - Under it, the full clashing file list from the epic detail as a plain list inside the box.
  - Then the buttons "Stash, merge and restore" and "Merge again", in that order.
- Other merge blocks show the buttons from `detail.actions` (for example "Merge again") under the
  text.
- The notice buttons use the styles and the write hook that `recover-merge-error-impl` built. On
  phones they are 44px tall, like the bar buttons.
- The bar shows no Merge button while the epic has a land block. The notice's buttons replace it.
  This retires the `isMergeable` "Merge as retry" path from the first redesign.

### Ticket cards

- New colours #252c36 (card border, hairline, divider lines), #1d232c (done card border) and #ffb3ad
  (notice text) have no theme token yet. The implementer picks whether to add tokens and their
  names.
- Live card: background #141920, 1px #252c36 border, radius 12px, height 48px, padding 0 16px, gap
  12px. A running card's border is rgba(108,180,255,0.40). The whole card stays one link.
- Row parts in order:
  - The mark: an 8px dot placed directly in the row. A running ticket gets a 10px SVG ring with a
    30% arc on a rgba(108,180,255,0.3) track. The implementer picks whether to extend `Arc` or draw a
    card-local spinner.
  - The id: ui-monospace 12px `text.dim`, fixed width 28px (26px on phones).
  - The title: white, one line, ellipsis.
  - The meta: 13px `text.dim`, `white-space: nowrap`, at the card's right end.
- Meta colour: running cards in the running tone (#6cb4ff). Needs-answer and needs-repair cards in
  the paused tone (#ff7066).
- Meta wording (the `cardMeta` helper):
  - Running loop row: "running", the time, the cost, then "fork of NN". Empty parts drop.
  - Running Design row: the type, then "running", then the time while a design session is live. For
    example "implement · running · 3m".
  - Needs-answer row, either kind: only "needs answer".
  - Every other status keeps its rendered word and pause reason, as built. Needs-repair still reads
    "needs-repair · Run failed".
- Gaps: 8px between cards, 28px between the bar (or the notice) and the list.
- Fork indent: 28px wide, 20px on phones. The card narrows by the same amount.
- Expanded card (needs-answer and needs-repair):
  - Background rgba(255,112,102,0.07), 1px rgba(255,112,102,0.35) border, radius 12px, padding
    12px 16px, gap 10px. The title is weight 500.
  - The second row is indented 60px. The question is 14px `text.muted` and fills the row. Answer or
    "Fix it with an agent" sits at the row's right end.
  - The buttons keep their built style. On phones they are full width and 44px tall.
- Phone card: two lines, padding 10px 14px, row gap 2px, line gap 10px. The title ellipsizes. The
  meta (13px `text.dim`) sits on the second line, indented 54px. An expanded phone card puts the
  meta, the question and the button on their own rows, each indented 54px.

### Done and cancelled cards

- Divider: label 13px `text.dim`, lines 1px #252c36, margin 16px 0 4px. When it heads the list the
  top margin is 0.
- Done card: no fill, 1px #1d232c border, radius 12px, height 44px, padding 0 16px. The title is 14px
  `text.muted`, the id is the monospace column, and the meta sits at the right end in 13px
  `text.dim`.
- Done mark: a plain 12px check stroke, #55d996, stroke width 2.2, no circle. `StatusIcon` elsewhere
  is unchanged.
- Cancelled card: the done card's border and no fill, a muted dot, the title in `text.dim`, meta
  "cancelled".
- Phone done and cancelled cards: two lines, padding 9px 14px, the meta indented 54px.
- "Show N more" stays the built button (D35).

### More stats

- Card: background #141920, 1px #252c36 border, radius 12px, padding 14px 16px, gap 14px, 28px
  above it.
- A 16px SVG chevron with stroke `text.dim` replaces the "▸"/"▾" glyphs. It still turns with the
  open state.
- "More stats" is 15px weight 500. The preview is 13px `text.dim`, right-aligned.
- Phones: two rows. The chevron and "More stats" on top, the preview under them with a 28px left pad
  and a 4px gap. Padding 12px 14px.

### Gallery

- The gallery's Epic page entries gain a cancelled card, a running fork and a `dirty-trunk` notice
  with a file list, beside the existing states.

## Testing Decisions

- Good tests check what a person sees: roles, visible text, element order and, in e2e, bounding
  boxes. Page tests do not check CSS values. Exact look values are checked with `gx-ui-check`
  (`getComputedStyle`) against a `gxfixture` scenario.
- **Pure helpers.** Extend `rows.test.ts` with table tests for `cardMeta` (running loop row with and
  without cost, running fork whose parent is done, running Design row with and without a live
  session, needs-answer row on both kinds, needs-repair row) and for the loop stat parts (publishing
  with 0, 1 and 7 counted rows, cancelled rows left out, the "Merged" word).
- **Page tests.** Extend `EpicPage.test.tsx` (renders through `appRoutes` with fake fetchers):
  - the publishing line text,
  - "Merged" on a finished epic,
  - the notice text, its file list and its buttons for `dirty-trunk` and for `needs-rebase`,
  - no Merge button in the bar on a land-blocked epic,
  - the meta texts on cards,
  - the cancelled card's meta.
- **Page mode.** A hook test for `useWide` with a fake `matchMedia`: under a narrow page it answers
  by the 40em query, under a wide page by the 62em query, and the gallery override wins. The e2e flow
  "Page modes at 760px" checks the real layout.
- **E2e.** Flows 2, 7, 51, 52 and 53 read text or layout that changes. Their pass lines and code
  change with this epic. New flows are listed under Verification.
- **Prior art.** `rows.test.ts`, `EpicPage.test.tsx`, `nav/BackLink.test.tsx`, and the phone flows in
  `webui/e2e/phone.e2e.ts`.

## Verification

Flows refer to `webui/e2e/CATALOG.md`. "Look check" means `gx-ui-check` on the named scenario, reading
`getComputedStyle` or bounding boxes at the named width. Desktop is 1024px, mid is 760px, phone is
375px. "Updated flow N" means the catalog flow whose pass lines this epic rewrites. "New flow" means a
flow to write as an implementation ticket.

1. Story 1: new flow "Page modes at 760px" on `mixed` at 760px: on `#/e/demo:mixed-epic` the ring
   is 64px wide, the summary's top is above the ring's bottom, and the Queue button's top is within
   the stats line's height (same row). A card's meta sits on the title's line.
2. Story 2: hook test: a narrow page answers by the 40em query and a wide page by the 62em query.
   Page test: the Ticket page renders in the "narrow" page mode.
3. Story 3: same flow on `design` at 760px: on `#/d/demo:design-epic` tapping the title opens the
   "Jump to epic" sheet that fills the screen (the phone layout). Flow 24 passes unchanged. Hook
   test: a wide page answers by the 62em query.
4. Story 4: same flow: on the Epic page at 760px the jump menu opens as a popover and the ⌘K hint
   shows. Flows 20 and 21 pass unchanged.
5. Story 5: look check on `united` at desktop and phone on `#/e/demo:awaiting-epic`,
   `#/t/demo:paused-epic/01` and `#/d/demo:design-live`: the back link is 14px, colour #a3acba,
   and the gap to the next element is 28px (24px at phone). Flow 22 passes unchanged.
6. Story 6: look check on `mixed`, `#/e/demo:mixed-epic`: the header row has `align-items: center`,
   gap 18px at desktop and 14px at phone.
7. Story 7: look check on the same page: the h1 is 30px with line height 33px at desktop, and 24px
   with line height 1.15 at phone. Same check on `#/d/demo:design-epic` at both widths.
8. Story 8: look check on `design`, `#/e/demo:design-epic` after flow 50's step 1: the summary is
   15px, line height 1.5, #c9d0da, 4px under the title at desktop. At phone its top is 12px below
   the ring's bottom and it spans the content width.
9. Story 9: look check on `mixed`: the bar's `border-top` is 1px #252c36, `padding-top` is 14px and
   the gap above is 14px at desktop, 12px each at phone.
10. Story 10: look check on `mixed`: the stats line colour is #a3acba.
11. Story 11: look check: on `design`, `#/e/demo:design-epic`, the "Resolve tickets" span is
    #c9d0da. On `united`, `#/e/demo:publishing-epic-impl`, the "Publishing tickets" span is
    #c9d0da.
12. Story 12: updated flow 52: the stats line reads exactly "Publishing tickets · 1 ticket so far".
    Table tests cover 0 (part dropped), 7 and cancelled rows.
13. Story 13: updated flow 7: after the release the epic page reads "Merged · $0.00 · 0s agent
    time". Page test: a finished epic's stats line starts with "Merged".
14. Story 14: updated flow 51: `blocked-merge-epic` shows a notice reading "Can't merge: needs a
    rebase" with an info icon (`aria-hidden` svg) inside it. Look check: background
    rgba(255,112,102,0.1), radius 10px, text #ffb3ad, 14px under the bar.
15. Story 15: updated flow 51: on `blocked-merge-epic` the bar has no Merge button, and the notice
    holds the "Merge again" button. `awaiting-epic` still has Merge last in the bar. Page test: a
    land-blocked epic with `actions` renders those buttons inside the notice region.
16. Story 16: extend recover-merge-error's flow "Dirty trunk on the Epic page" (scenario
    `dirty-trunk`): the notice reads "Can't merge: Trunk has local changes", then every clashing
    file as a list item, then "Stash, merge and restore" before "Merge again", all inside the
    notice. Page test with a 7-file list shows all 7 files.
17. Story 17: look check on `mixed`: a live card has background #141920, border 1px #252c36, radius
    12px and height 48px. Updated flow 2: each live ticket is still one link.
18. Story 18: updated flow 7 with `--hold`: before the release the running card's border colour is
    rgba(108,180,255,0.4) (look check on the same state).
19. Story 19: new flow "Card columns" on `mixed` at desktop: the left edges of all live card titles
    are equal. Look check: the id is monospace 12px #a3acba.
20. Story 20: same flow: a title wider than the card shows on one line (its box height is one line)
    and its `text-overflow` is ellipsis. Look check: a live title is #ffffff.
21. Story 21: same flow at desktop: each meta's right edge equals its card's right edge minus 16px.
    Look check: the meta is 13px #a3acba.
22. Story 22: look check: on flow 7's held running card the meta is #6cb4ff. On `mixed` after
    queueing, ticket 06's meta is #ff7066.
23. Story 23: updated flow 7 with `--hold`: the running card's meta starts with "running · ". Table
    tests: "running · 3m · $0.40", "running · 3m" with no cost, and "running · 3m · $0.40 · fork of
    07" when the parent is done.
24. Story 24: updated flow 50: after the start, the claimed design ticket's meta reads "<type> ·
    running" and gains a time. Table test: without a live session it reads "<type> · running" with
    no time.
25. Story 25: updated flow 2: after queueing, ticket 06's meta reads "needs answer". Updated flow 3,
    Answer row: the `paused-epic/03` card reads "needs answer". The Repair row still reads
    "needs-repair · Run failed".
26. Story 26: look check on `mixed`: an open card's dot is 8px. On flow 7's held running card the
    mark is a 10px svg with a track circle and an arc.
27. Story 27: look check on `mixed`: cards are 8px apart and the first card's top is 28px below the
    bar's bottom.
28. Story 28: page test with a fork row: its card has `data-indent="1"`. Look check on `mixed` with
    recovery left on (as in the research doc): fork `07a`'s card is 28px right of its parent and 28px
    narrower at desktop, 20px at phone.
29. Story 29: updated flow 2: after queueing, ticket 06's card holds the question and an Answer
    button in its second row, with Answer to the right of the question. Look check: background
    rgba(255,112,102,0.07), border rgba(255,112,102,0.35), title weight 500, the second row's left
    edge 60px right of the card's content edge.
30. Story 30: updated flow 3, Repair row: `paused-epic/07`'s card has the same tinted background as
    the needs-answer card, and "Fix it with an agent" is the last item of its second row. Flow 55
    passes unchanged.
31. Story 31: updated flow 53 on `mixed` at phone, with recovery turned off before queueing (as in
    flow 2): after queueing, Answer on 06 and "Fix it with an
    agent" on 07 are each at least 44px tall and as wide as their row.
32. Story 32: look check on `mixed`: the divider label is 13px #a3acba and its lines are #252c36.
    Updated flow 7: the divider is still the first item of the list.
33. Story 33: look check on `many-done`: a done card has a transparent background, border 1px
    #1d232c, radius 12px, height 44px, a 14px #c9d0da title and its meta at the right end. Flow 54
    passes unchanged.
34. Story 34: look check on `many-done`: the done mark is a 12px svg with one path and no circle,
    stroke #55d996.
35. Story 35: updated flow 2: the cancelled card `05` meta reads "cancelled". Look check: no fill,
    border #1d232c, title #a3acba, dot in the muted tone.
36. Story 36: updated flow 7: the More stats card is still the last item. Look check: background
    #141920, border 1px #252c36, radius 12px, 28px above it.
37. Story 37: look check on flow 7's finished page: the chevron is a 16px svg, "More stats" is 15px
    weight 500, the preview is 13px #a3acba and right-aligned.
38. Story 38: look check on `finishing` after the release (flow 7's end state) at phone: "More stats" is one line high and the preview's top is below its bottom.
39. Story 39: updated flow 53: on `mixed` at phone a card's meta sits below its title (kept) and is
    indented 54px from the card's content edge. A long title stays one line.
40. Story 40: updated flow 53 on `mixed` at phone, recovery off, after queueing: in ticket 06's card the meta, the
    question and Answer are stacked in that order, each 54px from the card's content edge.
41. Story 41: look check on `many-done` at phone: a done card has two lines, padding 9px 14px and
    border #1d232c.

## Out of Scope

- The TUI epic view. The TUI keeps "Finished" for a merged epic (gx:follow-ups/46).
- The Epics page cards, except that they already read "Merged" from the server.
- Button styles on any page (D16 to D19, D23, D35). No new `Button` variants.
- The page frame: top bar, content width and padding (D2).
- The recovery verbs, the epic `actions` list, the dirty-trunk file list data and the write hook.
  `recover-merge-error-impl` builds them.
- New features that are not in the mocks.

## Further Notes

- Mocks: https://claude.ai/artifact/J4FpZtrjBHQMqJRcCoeJGX.
- Comparison: https://claude.ai/artifact/5in4w5gjubBndWLztWNm3L and
  `docs/research/epic-page-redesign2-differences.md`. Each story maps to differences there: stories
  1 to 4 to D1, 5 to D3, 6 to D4 and D45, 7 to D5, 8 to D7, 9 to D11, 10 to D12, 11 to D13, 12 to
  D15, 14 to D41 and D42, 15 and 16 to D43, 17 to D20, 18 to D21, 19 to D27, 20 to D28, 21 to D24,
  22 to D25, 23 to 25 to D26, 26 to D29, 27 to D30, 28 to D31, 29 and 30 to D22, 31 to D47, 32 to
  D32, 33 to D33, 34 to D34, 36 to D36, 37 to D37 and D38, 38 to D39, 39 to D46, 40 to D47, 41 to
  D48.
- Map decisions: gx:epic-page-redesign2 tickets 01 to 03. Earlier spec: `docs/specs/epic-page-redesign.md`.
- `epic-page-redesign2-impl` is blocked on the whole `recover-merge-error-impl` epic.
