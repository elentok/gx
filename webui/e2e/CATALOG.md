# Web e2e flow catalog

Run with `make web-e2e`. Each flow starts its own fixture server (`testutil/gxfixture`), opens the
`web_url` once for the web cookie, and never reloads the page. The suite fails if a page reloads.
Retries are off. Locators use roles and visible text, plus `data-testid` `pill` and `reason` on
Epics page cards. The helpers `section` and `card` in `fixture.ts` find a section (expanding it) and a card.

Before you write a flow, read "How the fixture behaves" in `skills/gx-ui-check/SKILL.md`: how a fake
worker outcome maps to ticket status, why fixture CLI writes land late, how to set an epic summary,
and "read `test-results/**/error-context.md` first on a locator failure".

Flows keep the numbers of the e2e plan, so a gap in the numbering is a flow not written yet.
Code: `epics.e2e.ts` (flows 1, 2, 6, 7, 8, 54), `pause.e2e.ts` (flows 3, 55, 56), `design.e2e.ts` (flows 4, 5, 9, 10, 11, 12, 13, 14, 42, 43, 44, 45, 46, 50),
`united.e2e.ts` (flows 15, 16, 17, 18, 19, 28, 29, 30, 31, 32, 33, 51, 52), `jump.e2e.ts` (flows 20, 21, 22), `phone.e2e.ts` (flows 23, 24, 41, 53),
`newdesign.e2e.ts` (flows 25, 26, 27, 34, 35, 36, 37, 38, 39, 40), `dirty-trunk.e2e.ts` (flows 57, 58, 59).

## 1. Queue an epic

- **Scenario:** `ready` (one epic, `ready-epic`, not queued)
- **Steps:**
  1. Open the Epics page; `ready-epic` is in Ready to queue.
  2. Click the card's Queue button, with the `/v1/queue/add` call held.
- **Pass:** the card's button is disabled while the call runs. After it, the card leaves Ready to
  queue and shows in In progress, with no navigation and no toast.

## 2. Epic looks right

- **Scenario:** `mixed` (`mixed-epic`: done, open, blocked, draft, cancelled, asking and failing tickets)
- **Steps:**
  1. Open the Epics page; the `mixed-epic` card reads `demo` and `2/7`.
  2. Open `mixed-epic`.
  3. Click Queue on the epic page.
  4. Click "← Epics".
- **Pass:**
  - Before queueing, the epic page shows the ring `2/7` with the Implementation badge and no comet, the
    stats line "Not queued · $0.00 · 0s agent time", the divider "1 done, 1 cancelled", and one card link
    per live ticket: `02 open ready`, `03 blocked after 02`, `04 drafted draft`, `06 asks ready`,
    `07 fails ready`, then the cancelled `05 dropped cancelled`. There is no More stats card yet.
  - After queueing, the stats line starts with "Running" and the comet shows. The fake worker settles:
    the ring reads `4/7`, ticket 06 is expanded with "needs answer", its question and an Answer button
    to the right of its title. Cancelled 05 reads "cancelled". Ticket 07 is expanded in the deadlocked tone with "needs-repair · Run
    failed" in its meta and a Fix it with an agent button. The flow turns recovery off, so 07 stays a
    needs-repair card instead of becoming a waiting parent of an investigate ticket. The Epics page card
    shows `4/7`, a pill and a reason.

## 3. Pause reasons and recovery

- **Scenario:** `paused` (`paused-epic` with one ticket per pause reason, `approve-epic` whose worker
  blocks on a permission prompt). Each row is its own test with its own server; `paused-epic` is
  queued through the CLI so it shows in Needs you.
- **Steps (per recovery row):**
  1. Open the Epics page; the epic's card is in Needs you.
  2. Open the epic from its card, then the ticket from its row.
  3. Take the recovery action.
- **Pass:**
  - The card names the epic's first pause only (`paused-epic`: "Seeded pause: self-reported" and
    "+4 more"; `approve-epic`: "wants to run: rg fixture"). The reason shows on the epic page row
    and on the ticket page; the epic page row's card reads "needs answer" for the Answer rows and
    "needs-repair · <reason>" for the repair rows. Opening a ticket from its card lands on
    `#/t/<address>`. The Repair row's tinted card reads "needs-repair · Run failed" and "Fix it with an
    agent" is the last button, in the card's second row.
  - The action clears the reason from the ticket page without a reload.
- **Rows:**

  | Row | Ticket | Reason | Action |
  | --- | --- | --- | --- |
  | Answer | `paused-epic/03` | Waiting for your answer | type an answer, Send answer |
  | Choose base | `paused-epic/04` | Ambiguous base | pick `01`, Choose base |
  | Retry | `paused-epic/07` | Run failed | Retry in the Recovery actions form |
  | Merge again | `paused-epic/05` | Merge conflict | Merge again; the ticket turns Done |
  | Start over | `paused-epic/08` | Paused by you | Start over, type a Reason in the dialog, confirm |
  | Resume | `paused-epic/08` | Paused by you | the card shows Resume and Start over; Resume |
  | Recommended | `paused-epic/09` | Run failed | the diagnosis shows and only the first button reads "Recommended"; click it |
  | Approve | `approve-epic/01` | Needs approval | Allow once |
  | Repair | `paused-epic/07` | Run failed | none: the card shows "+6 more", the epic page shows three "Fix it with an agent" buttons |

- **Tooltips:** each recovery button carries its tooltip as a `title`: Merge again (05), Resume (08),
  Retry Recommended (09) and Start over (08).
- **Pending:** a held `/v1/tickets/retry` call leaves the Retry button disabled until it answers.

## 4. Start a design session

- **Scenario:** `design` (`design-epic`, a map at Resolve tickets, and its `design-epic-impl` epic)
- **Steps:**
  1. Open the Epics page, then the Design page by its route (`#/d/demo:design-epic`); it shows
     "Start design session". No link reaches a Design page before its session starts.
  2. Click Start design session, then press Chat.
- **Pass:** Before Start the Epics tab is `aria-current="page"` and the header has no "More actions"
  button (a charted design with no session has an empty menu). Progress shows Resolve tickets as
  the current step. After Start the start button is gone and, in the Chat view, the first scripted
  message ("Two questions before the spec.") shows; round one opened in the Questions view.

## 5. Answer a round

- **Scenario:** `design`
- **Steps:**
  1. Start the session. Round one has `Q8` (options) and `Q8b` (yes/no).
  2. Press Chat. Pick SQLite and Yes, then Send, with the `/v1/sessions/prompt` call held.
  3. Click Continue (retried), then press Chat.
- **Pass:**
  - The question dots and titles read `Q8` and `Q8b`.
  - Send is disabled while the call runs.
  - The chat shows the answers as the person's message, so the agent received them.
  - Continue shows after the agent's run-end line.
  - Continue brings a fresh round with `Q9`, which opens in the Questions view; "A fresh round."
    shows after pressing Chat.

## 6. Outside changes show up

- **Scenario:** `ready`
- **Steps:**
  1. Open the Epics page; `ready-epic` is in Ready to queue.
  2. Run `gx server queue add demo:ready-epic` through the CLI against the fixture's server.
- **Pass:** without any action in the page, the card moves to In progress.

## 7. Epic completion

- **Scenario:** `finishing`, started with `--hold` so the last ticket stays running until the flow
  releases it (`fixture.release()`).
- **Steps:**
  1. Open the Epics page; the `finishing-epic` card is in In progress and shows `1/2`.
  2. Release the hold; the last ticket finishes and merges.
- **Pass:** before the release, the running ticket `02`'s card meta starts with "running · ". After it
  the card is in the Finished section (collapsed until opened) as `2/2`; its epic page reads
  "Merged · $0.00 · 0s agent time" with the ring `2/2`, no comet, the divider "2 done" above the
  cards, and a More stats card below them that is `aria-expanded` false, then true after a click.

## 8. Edge states

- **Scenario:** `edge` (project `bare` with no epics, `empty-epic` with no tickets, `blocked-epic`
  with ticket 02 blocked by the unmerged 01)
- **Steps:**
  1. Open the Epics page; the `empty-epic` card is in In progress, reading "Implemented".
  2. Open `empty-epic`.
  3. Click "← Epics" and open `blocked-epic`.
- **Pass:**
  - `empty-epic` reads "No tickets yet" with ring `0/0` and no More stats card.
  - `blocked-epic` shows the cards `02 waiting after 01` and `01 unmerged ready`.
- **Not covered:** the "No epics yet" state of the Epics page. `edge` has epics, so it never shows.

## 9. Design progress to the end

- **Scenario:** `design`
- **Status:** enabled. A `design_step` change publishes a ticket-changed event, so the open page
  refetches Progress without a reload.
- **Steps:** run `gx tickets set demo:design-epic --design-step <step>` for spec, review,
  grill-findings, create-tickets, branch, done.
- **Widths:** 1440 px and 375 px, one test each.
- **Pass:**
  - At 1440 px each step becomes current in turn in the Progress region (Done becomes done) and
    earlier steps read done.
  - At 375 px the steps live in the steps sheet; the step line reads "Resolve tickets · …" before
    Branch and is gone after the last step.
  - On both widths "Open implementation epic →" shows only at Branch and Done, in the Chat region
    within 80 px of its top.

## 10. Auto-advance

- **Scenario:** `design`, with a 3s countdown (`--auto-advance-delay`)
- **Steps:** start the session, answer round one (SQLite, Yes), Send.
- **Pass:** after the run-end line the Composer shows "Continuing in Ns" and Cancel; with no press
  the fresh round (`Q9`) appears in the Questions view; "A fresh round." shows after pressing Chat.

## 11. Auto-advance, cancel

- **Scenario:** `design`, with a 5s countdown
- **Steps:**
  1. Answer round one and Send. During the countdown, click Cancel.
  2. In a second test, open the Note box, type a note and Send it during the countdown instead.
- **Pass:** the countdown disappears, Continue stays, and after the old deadline no fresh round
  has been sent.

## 12. The auto-advance switch

- **Scenario:** `design`, with a 30s countdown
- **Steps:**
  1. The Auto button reads "Auto-advance on" while the agent works.
  2. Open the Auto menu, turn Auto-advance off, answer round one, Send; the run ends.
  3. Turn it on again from the menu.
- **Pass:** with the switch off the run end shows Continue and no countdown; turning it on starts
  "Continuing in Ns".

## 13. Defaulted questions

- **Scenarios:** `design-mixed` (one asked question, two defaulted), `design-defaulted` (all defaulted)
- **Steps:** start the session, press Chat. In the mixed case answer the card Yes and Send.
- **Pass:**
  - Mixed: one card ("Q12 - Keep going?"); Q10 and Q11 show as plain text in the Chat view; the sent answer
    names Q12 only, nothing about Q10 or Q11.
  - All defaulted: one "Q12 - Accept these defaults?" card, the defaults as plain text, and the
    session's Actionable has `grilling` set.

## 14. Working row

- **Scenario:** `design-working` (a plain turn that stays busy for 20s)
- **Steps:**
  1. Start the session while the agent is busy.
  2. Run `gx agent step spec` with the session id in `GX_AGENT_SESSION`.
- **Pass:** the chat shows the spinner row "Working: Resolve tickets · first" (the current ticket's
  title), then "Working: Spec" after the report.

## 15. Redirects

- **Scenario:** `ready`
- **Steps:** set the hash to `#/`, `#/q`, `#/designs` and an unknown path in turn.
- **Pass:** each lands on `#/epics`; the nav has links Epics and Stats, and none named Queue or Designs.

## 16. Needs you

- **Scenario:** `united` (two projects, one card per section and per Needs-you reason, workers held)
- **Steps:** open the Epics page; open and close Ready to design; click around a card; filter.
- **Pass:**
  - Sections read Needs you, Ready to queue, In progress, Ready to design, Finished. Ready to design
    and Finished start collapsed, and the header toggles Ready to design.
  - Each title shows its card count: nine in Needs you (paused, question, permission, deadlock,
    merge-blocked, dirty-trunk, awaiting-merge, Design question, paused one-off), and Finished counts the cards
    on screen.
  - Every Needs-you card has a pill; no other section has one. The stopped Design sits in In
    progress with Continue.
  - A click on a card's project text stays on `#/epics`; its title opens `#/e/demo:paused-epic`.
  - Merge is the only button on the merge-blocked and awaiting-merge cards. The dirty-trunk card
    shows "Stash, merge and restore" and "Merge again", the pill "Merge blocked" and the reason
    "Trunk has local changes", with no file list. Queued, running,
    finished and draft cards have no buttons.
  - A filter shows only the matching card, and "No match" when nothing matches.
- **Not covered:** a Ready to queue card with a blocker (the scenario has none, so its reason is
  unchecked).

## 17. Merge from the card

- **Scenario:** `united`
- **Steps:** click Merge on `awaiting-epic`, then on `blocked-merge-epic`.
- **Pass:** `awaiting-epic` shows the toast "no branch awaiting-epic in ..." (the fixture's epics
  have no git branch). `blocked-merge-epic` shows its result as a second toast. The page stays on
  `#/epics`. A successful merge is not covered.

## 18. Joined card

- **Scenario:** `design`, started with `--hold`
- **Steps:** run `gx tickets set demo:design-epic --design-step create-tickets`, then `branch`, then
  `gx server queue add demo:design-epic-impl`.
- **Pass:**
  - One card, `design-epic`, with no `design-epic-impl` card. Its title opens `#/e/demo:design-epic`.
  - The status reads "Designing" with a percentage in the ring, then "Creating tickets (N so far)..."
    with "…", then "Not started" with `n/m` at Branch, then "Implementing..." once queued. The title
    opens `#/e/demo:design-epic-impl` from Branch on.
  - The time reads a bare duration while Designing, "created ... ago" at Branch, and a bare
    duration once queued.
  - The impl epic page has an "Open design" button and the design epic page an "Open implementation
    epic" button (no arrows); each opens the other's page.

## 19. Follow-ups triage

- **Scenario:** `united`
- **Steps:** open Ready to design; click Triage on the first card; close the dialog; click the title.
- **Pass:** the `follow-ups` card is first and reads "1 bug, 1 idea". The dialog shows
  `/gx-triage-follow-ups --project demo` and a Copy button. The title opens `#/e/demo:follow-ups`.

## 20. Jump menu opens and closes

- **Scenario:** `united`
- **Steps:**
  1. Open `#/e/demo:running-epic`; click the title.
  2. Press Esc; press Ctrl+K.
  3. Type `draft-epic`, then `finished-a`; press Esc.
- **Pass:**
  - The menu shows no count.
  - The title click and Ctrl+K both open the "Jump to epic" dialog with the filter focused; Esc closes it.
  - With no text, Needs you rows show (`paused-epic`) and Ready to design (`draft-epic`) and Finished
    (`finished-a`) rows do not. Typing a name shows its row.

## 21. Jump menu keyboard and picks

- **Scenario:** `united`
- **Steps:**
  1. Open `#/e/demo:running-epic`, open the menu and type `finished`; press Down, then Enter.
  2. Open the menu again and pick `design-live`, then `running-epic`, then the one-off `queued`.
- **Pass:**
  - Down selects the second row and Enter opens its epic page; the menu closes.
  - `design-live` opens `#/d/demo:design-live`, `running-epic` opens `#/e/demo:running-epic`, and the
    one-off opens `#/t/demo:one-offs/02`.
  - On the one-off's ticket page the back link reads "← one-offs" with no count.
- **Not covered:** the `design-epic` Design of the spec text. `united` has no joined card, so the Design
  pick uses `design-live`.

## 22. Back link count

- **Scenario:** `united`
- **Steps:**
  1. Open `#/e/demo:paused-epic`, `#/d/demo:design-live`, then `#/e/demo:running-epic`.
  2. Click the count text of the back link.
  3. Open `#/e/demo:running-epic`; run `gx tickets set demo:paused-epic/01 --status open`.
- **Pass:**
  - The back link reads "← Epics · 8 more need you" on `paused-epic` (itself in Needs you) and
    "· 9 more need you" on the other two pages. A stopped Design is not in Needs you.
  - The title button has no number and no `waiting-count` element exists.
  - Clicking the count text opens `#/epics`.
  - The count drops from 9 to 8 with no reload.

## 23. Phone, Epics page

- **Scenario:** `united`, at 375 px wide (`test.use({ viewport })`)
- **Pass:**
  - Two cards of Needs you share a left edge and the second sits below the first.
  - A card's `time` element is no taller than 1.5 line heights.
  - On `#/d/demo:design-live` the back link reads "← Epics · 9 more" (or the full text) on one line.

## 41. Phone, New Design sheet

- **Scenario:** `ready`, at 375 px wide
- **Steps:** click New Design.
- **Pass:**
  - The dialog is a bottom sheet: 375 px wide, anchored to the bottom, 85% to 91% of the viewport high, with a grab handle (`data-testid="grab-handle"`).
  - Start design is 1.7 to 2.3 times as wide as Discard.
  - Close sits in the header, above the idea box.

## 24. Phone, Design page

- **Scenario:** `design`, at 375 px wide
- **Steps:**
  1. Open `#/d/demo:design-epic` and start the session.
  2. Click the step line, then Close in the sheet. Open it with the ring and press Escape. Open
     it again and tap the backdrop above the sheet.
  3. Press Chat, then Questions in the View toggle.
  4. Press Send with nothing answered, then Escape.
  5. Tap the title; click Close on the sheet.
- **Pass:**
  - The Questions view is up after Start: the first message is hidden and no "Progress" region (segmented bar) is on the page.
  - The step line "Resolve tickets · …" opens the "Design progress steps" sheet with Resolve tickets current; the ring opens it too. Close, Escape and a backdrop tap each close it.
  - No ⤢/⤡ buttons; Chat shows the first message, Questions hides it.
  - The step line sits above the sheet. The Send popover is at most 343 px wide, inside the viewport and overlaps the sheet.
  - Note reads only its icon; the Auto button still shows "Auto-advance" and "Auto-queue".
  - The title opens a "Jump to epic" sheet that fills the screen; Close closes it.

## 25. New Design from an empty page

- **Scenario:** `empty` (one project, no epics)
- **Pass:** the page reads "No epics yet." and offers New Design.

## 26. New Design form

- **Scenario:** `ready`
- **Steps:**
  1. Click New Design (right of the filter); fill the name `fresh-design` and an idea; Start design.
  2. Click "← Epics", open New Design again, press Escape, then click the new card's title.
- **Pass:**
  - The form is a "New Design" dialog with a header row (no project pill in `ready`, which has one
    project) and an idea box taller than half the dialog. The placeholders read `my-epic` and
    "What is the problem? Who hits it? What does done look like?".
  - Start opens `#/d/demo:fresh-design` and `gx server epics` lists it. `gx tickets show
    demo:fresh-design` shows `kind: map`, `design_step: chart` and `## Initial prompt`, and no
    Destination.
  - A reopened modal is empty.
  - The card of the Designing epic opens its epic page, `#/e/demo:fresh-design` (not the Design page).

## 27. Design a not-started Design

- **Scenario:** `united`
- **Steps:** open Ready to design; click Design on the `design-new` card.
- **Pass:** the page is `#/d/demo:design-new` and `gx agents list` names `design-new`.

## 34. New Design text size

- **Scenario:** `ready`
- **Steps:** open New Design.
- **Pass:** the Epic slug and Rough idea fields have a computed font size of 16px.

## 35. New Design project select

- **Scenario:** `united` (projects `demo` and `auto`)
- **Steps:** open New Design with the Epics filter on All projects; press Escape; pick `auto` in the
  Epics "Project" filter; open New Design again.
- **Pass:** the dialog's Project select lists `demo` and `auto`. With All projects and an empty
  draft it shows `demo` (the first); with the filter on `auto` it shows `auto`.

## 36. New Design draft keeps its project

- **Scenario:** `united`
- **Steps:** set the Epics filter to `demo`; open New Design, pick `auto`, type a name; press Escape; open it again.
- **Pass:** the Project select shows `auto`, not the filter's `demo`.

## 37. New Design draft is shared, closing keeps it

- **Scenario:** `ready`
- **Steps:**
  1. Type a name and an idea. Open a second page of the same browser context on `#/epics`.
  2. In the first page close the dialog with ×, reopen, close with Escape, reopen, close with a click on the backdrop, reopen.
- **Pass:**
  - The second page does not open the modal by itself. Its New Design shows the same name and idea.
  - After each way of closing, the reopened dialog still holds the name and idea.

## 38. New Design Discard

- **Scenario:** `ready`
- **Steps:** click Discard with empty fields. Reopen, type a name and an idea, click Discard, click Keep, click Discard twice.
- **Pass:**
  - With empty fields Discard closes the dialog at once.
  - With text it shows "Discard this draft?" with Keep and Discard. Keep returns to the text.
  - The confirming Discard closes the dialog; the next one is empty.

## 39. New Design refused

- **Scenario:** `ready`
- **Steps:** open New Design; name it `ready-epic`; type an idea; Start design.
- **Pass:** an alert in the dialog reads "already has an epic". The name and idea stay and the URL stays `#/epics`.

## 40. gx designs open

- **Scenario:** `ready`
- **Steps:** run `gx designs open cli-design --idea x --project demo --json` twice.
- **Pass:** the first run succeeds and `gx tickets show demo:cli-design` shows `kind: map`. The second returns `epic-exists`.

## 28. Menu: AFK, build text, Paused chip

- **Scenario:** `united` (the flow lives in `united.e2e.ts`)
- **Steps:**
  1. Open the Menu (hamburger, right of the Live dot); click the AFK item.
  2. Run `gx afk` (a flip).
  3. Click the build text; click AFK again; press Esc.
  4. Run `gx server queue pause`.
- **Pass:**
  - The AFK item reads unchecked, then checked after the click, and the menu stays open. `gx afk`
    reports `afk off`, so the server was on, and the item reads unchecked again.
  - The build text (`v...`) shows and does nothing: the menu stays open and the URL does not change.
  - With AFK on and the menu closed, the bar has no AFK.
  - The "Paused" chip shows in the bar.

## 31. Menu keyboard

- **Scenario:** `united`
- **Steps:** focus the Menu button, press Enter, ArrowDown, Enter, then Esc.
- **Pass:** the menu opens; ArrowDown highlights AFK; Enter checks it and the menu stays open (`gx afk`
  reports `afk off`); Esc closes the menu.

## 32. Epics project filter

- **Scenario:** `united` (projects `demo` and `auto`)
- **Steps:**
  1. Read the "Project" select, right of "Filter epics". Pick `demo`; open Ready to design and Finished.
  2. Filter for `awaiting-auto-epic`, an `auto` epic.
  3. Open a second page in the same browser context on `#/epics`.
- **Pass:**
  - The options are All projects, `demo`, `auto`.
  - Every card, the `follow-ups` card included, reads `demo`; none reads `auto`.
  - The `auto` epic gives "No match".
  - The second page reads `demo` in the select.

## 33. No project filter with one project

- **Scenario:** `ready`
- **Pass:** the Epics page has "Filter epics" and no "Project" select.

## 29. Grid columns at desktop width

- **Scenario:** `united`, viewport 1400 px wide.
- **Pass:** two cards of Needs you sit on the same row, side by side.

## 30. Open epic on a deadlock card

- **Scenario:** `united`
- **Steps:** click Open epic on the `deadlock-epic` card.
- **Pass:** the page is `#/e/demo:deadlock-epic`.

The Design page flows below build their Design in the flow (New Design, then `gx agents stop <id>`,
the id being the first column of `gx agents list`), so no scenario has to hold a chart-step map with
an exited session.

## 42. Stopped session card

- **Scenario:** `ready`
- **Steps:** start a Design `stopped-design` from the dialog; go back to the Epics page; stop its session through the CLI.
- **Pass:** the card is in Ready to design, reads "Not started" and has a Design button.

## 43. New session on a stopped Design

- **Scenario:** `ready`
- **Steps:**
  1. Start `retry-design`, stop its session, stay on `#/d/demo:retry-design`; open the ⋯ menu and click "New session".
  2. Start `live-design` and leave it live; open the ⋯ menu, click "New session" and dismiss the browser confirm. Do it again and accept it.
- **Pass:**
  - On the stopped Design the item reads "New session" with the hint "Start a fresh session". No dialog and no Epic slug field appear, and `gx agents list` shows a second session for it.
  - On the live one the hint is "Stop this session and start a fresh one". Cancel starts nothing; OK leaves two sessions, the old one exited and one live.

## 44. Failure banner New session

- **Scenario:** `ready`
- **Steps:** start `ended-design`, stop its session (it ends with no run-end); click "New session" in the `Session failure` region.
- **Pass:** the banner says "Session ended". No dialog appears; `gx agents list` shows a second session.

## 45. Delete design

- **Scenario:** `ready`; the second test uses `design`.
- **Steps:**
  1. Start `delete-me`; open the ⋯ menu: "Delete design" is absent. Stop its session with the page open.
  2. In the ⋯ menu click "Delete design", then Keep, then "Delete design" and Delete (with `/v1/designs/delete` held).
  3. Start `delete-me` again.
  4. On `design`, open `#/d/demo:design-epic`, start the session and open the ⋯ menu.
- **Pass:**
  - "Delete design" appears in the menu with no reload. Keep returns to the menu with the item still there.
  - The confirm Delete is disabled while the call runs.
  - Delete goes to `#/epics`, the card is gone and `gx tickets show demo:delete-me` fails.
  - The second New Design with `delete-me` succeeds.
  - `design-epic` (charted) lists no "Delete design" in the menu.

## 46. gx designs delete

- **Scenario:** `ready`
- **Steps:** `gx designs open cli-del`, stop its session, run `gx designs delete demo:cli-del`; run the same verb on `demo:ready-epic`.
- **Pass:** `gx tickets show demo:cli-del` fails. The verb on `ready-epic` exits non-zero and the epic stays.

## 47. Send popover

- **Scenario:** `design`
- **Steps:**
  1. Start the session and press Send with nothing answered.
  2. Click the `Q8b` row. Press Chat, fold the sheet with ▾, press Send, click the `Q8` row.
  3. Pick SQLite on `Q8`, press Send.
  4. Press Escape. Press Questions, press Send and click the question title (outside the popover). Press Send twice.
  5. Press Send, then Enter on the focused "Send anyway".
- **Pass:**
  - A popover named "Unanswered questions" shows above Send, ends within 2 px of Send's right edge, and the composer keeps its height.
  - It reads "2 questions are unanswered" and "0 of 2 answered", with `Q8` and `Q8b` rows; after the answer, "1 question is unanswered" and "1 of 2 answered".
  - The `Q8b` row closes it, makes `Q8b` the current dot and focuses its first choice. On a folded sheet the `Q8` row opens the sheet at `Q8`.
  - "Send anyway" is focused on open. Escape and a click outside it close it and focus Send. The second Send press closes it and posts nothing.
  - Enter sends the round with `Q8b` as "(unanswered)", with no second dialog and no "Answer them" button.

## 48. Fixed sheet rows

- **Scenario:** `design`, at 1280x600.
- **Steps:**
  1. Start the session, click "📝 Note" on `Q8`.
  2. Scroll the question body to the top, then to its end.
- **Pass:**
  - The note's `rows` is 5.
  - The dots, "0 of 2 answered" and the Prev/Next row keep their positions and stay visible.
  - The body's scroll position changes and the sheet's top edge does not.
  - Run in two tests: the Questions view (the default) and the docked sheet in the Chat view
    (press Chat first).

## 49. Questions and Chat views

- **Scenario:** `design` (and `design-working` for the last test)
- **Steps:**
  1. Start the session. Pick SQLite.
  2. Press Chat, fold and open ▾, press Questions. Press Escape.
  3. In the Questions view, open the Send popover, the jump menu (Ctrl+K), the `Q8b` note modal and the ⋯ menu in turn. Press Escape twice each time.
  4. Type a composer note, switch Chat, Questions, Chat, Questions.
  5. Resize to 1440px, then to 375px; open the steps sheet from the step line and press Escape twice.
  6. In a second test: press Chat, answer both questions, press Send, then Continue.
  7. In a third test on `design-working`: start the session.
- **Pass:**
  - After Start the View toggle reads Questions (pressed) with "0 of 2"; the sheet is taller than half the viewport; "Two questions before the spec." is hidden. After SQLite it reads "1 of 2".
  - In Chat the chat shows and ▾ folds and opens the sheet; in Questions there is no ▾. Escape in Questions shows the chat.
  - With a layer open, the first Escape closes only the layer and the second shows the chat; at 375px the same for the steps sheet.
  - SQLite, `Q8b` as the current dot and the composer note survive two switches. No "Enter focus mode" or "Leave focus mode" button exists.
  - At 1440px the question title is at most 820px wide.
  - Sending from Chat shows the answers in Chat; Continue brings the `Q9` round with Questions pressed.
  - On `design-working` after Start there is no "View" group and no Questions sheet.

## 50. Design epic page

- **Scenario:** `design`
- **Steps:**
  1. Write a long Destination section on `design-epic` with `gx tickets section`; open `#/e/demo:design-epic`.
  2. Click Start design session, press Chat to see the first message; open the epic page again.
- **Pass:**
  - Before the session the ring reads a percentage with `data-tone="muted"`. The summary shows in full under the title.
  - The stats line starts with "Resolve tickets", shows a time ("2m") and has no "$".
  - The open tickets read `01 first implement · ready` and `02 second implement · after 01`. The done ticket `03 timed` shows "2m" and no "$".
  - After the start the ring is `running` and the comet shows. Once `01` is claimed, its meta reads
    "implement · running · <time>".

## 51. Merge from the epic page

- **Scenario:** `united`
- **Steps:** open `#/e/demo:awaiting-epic`, then `#/e/demo:blocked-merge-epic`.
- **Pass:**
  - `awaiting-epic` has a Merge button with `data-tone="done"`, last in the button group.
  - `blocked-merge-epic` shows the notice "Can't merge: needs a rebase" with an `aria-hidden` svg icon.
    The notice holds "Merge again"; the bar has no Merge button.

## 52. Draft impl epic

- **Scenario:** `united`
- **Steps:** open `#/e/demo:publishing-epic-impl`.
- **Pass:** the stats line reads exactly "Publishing tickets · 1 ticket so far". Queue is disabled and its wrapper has the title "Queue unlocks once all tickets are published".

## 53. Phone, epic page

- **Scenario:** `united` at 375 px wide; the other tests use `mixed`.
- **Steps:** open `#/e/demo:awaiting-epic`; open `#/e/demo:mixed-epic` (with a long-titled ticket
  added); open it again with recovery off and click Queue.
- **Pass:**
  - Every button of the bar sits below the stats line and is at least 44 px tall.
  - On a card the meta text ("ready") sits below the title ("open"), indented 54 px. A long title stays
    on one line.
  - Answer on 06 and "Fix it with an agent" on 07 are at least 44 px tall and as wide as their row
    (less the 54 px indent). In 06's card the meta, question and Answer stack in that order, their text
    54 px from the content edge.

## 54. Done list

- **Scenario:** `many-done`
- **Steps:** open `#/e/demo:many-done-epic`; click "Show 2 more"; click "Show fewer".
- **Pass:**
  - The divider reads "5 done" and 3 done cards show, newest first (`merged-e`, `merged-d`, `merged-c`).
  - "Show 2 more" shows 5 cards and the button reads "Show fewer"; clicking goes back to 3.

## 55. Fix it with an agent

- **Scenario:** `paused`, with `paused-epic` queued. Browser clipboard permission granted.
- **Steps:**
  1. Open `paused-epic`; click "Fix it with an agent" on `07`; click Copy; click "Start gx agent".
  2. In a second page on the same server, open the dialog on `07` again; click "Start gx agent".
- **Pass:**
  - The dialog shows `/gx-investigate demo:paused-epic/07 is paused for repair: Run failed`; Copy reads "Copied" and the clipboard holds the prompt.
  - After the start the dialog reads "Agent started. View it in the gx TUI Agents tab" and has no "Start gx agent".
  - `gx agents list` shows one more session, with harness `claude` and dir the trunk checkout of `demo`.
  - The second start shows the toast "an agent is already fixing demo:paused-epic/07" and `gx agents list` has no new session.

## 56. Fix it with an agent, remote control

- **Scenario:** `paused` with config `agents.fix-agent-remote-control=true`, `paused-epic` queued.
- **Steps:** open `paused-epic`; click "Fix it with an agent" on `07`; click "Start gx agent".
- **Pass:** the dialog reads "Agent started. View it in the gx TUI Agents tab or in the Claude app".

## 57. Stats, Grilling section

- **Scenario:** `grilling-stats`; the phone test runs at 375 px wide.
- **Steps:** open `#/stats`; pick range 7d, then 30d; pick project `demo`.
- **Pass:**
  - The "Grilling" section follows "Cost and effort" and precedes "Friction", with cards "Words read" and "Questions asked".
  - At 30d the headlines are 550 and 12, and the small text is total over the 26 non-hatched days (21/day, 0/day).
  - The 4 days before the earliest count are hatched; the empty day after it is a zero bar (no bar, not hatched).
  - At 7d the headlines are 260 and 6, with no hatched days.
  - Project `demo` shows 310 and 7; the 4 hatched days stay the same.
  - At phone width the cards stack in one column with no horizontal scroll.

## 58. Dirty trunk on the Epic page

- **Scenario:** `dirty-trunk` (epic `dirty-epic`; the trunk has an untracked `same.txt` that the
  epic's branch also adds)
- **Steps:** open `#/e/demo:dirty-epic`.
- **Pass:**
  - The stats line reads "Paused · ...".
  - The "Merge blocked" notice reads "Can't merge: Trunk has local changes".
  - Under it the file list (`merge-block-files`) holds one item, `same.txt`.
  - Then "Stash, merge and restore" (with its tooltip) comes before "Merge again", all inside the notice.

## 59. Stash, merge and restore

- **Scenario:** `dirty-trunk`, started with `--hold`
- **Steps:** open `#/e/demo:dirty-epic`; click "Stash, merge and restore".
- **Pass:** a ticket "Resolve the conflicts on main" appears in the epic, the stats line reads
  "Running · ...", the stash button is gone, and the epic never shows "Awaiting merge" or
  "Finished" (the held worker keeps the epic unfinished).

## 60. Merge again on a dirty trunk

- **Scenario:** `dirty-trunk`
- **Steps:** open `#/e/demo:dirty-epic`; click "Merge again" while the trunk is still dirty.
- **Pass:**
  - A warning toast says the changes clash with `same.txt`, with an "Open the paused epic" link to
    the epic.
  - The Epic page stays on "Paused" and shows the fresh file list (`same.txt`).

## 61. Page modes at 760px

- **Scenario:** `mixed` for the Epic page, `design` for the Design page; both at 760 px wide.
- **Steps:** give `mixed-epic` a Summary section and open `#/e/demo:mixed-epic`; click the title.
  Then open `#/d/demo:design-epic` and click the title.
- **Pass:**
  - The ring is 64 px wide. The summary's top is above the ring's bottom. The Queue button's top is
    within one stats-line height of the stats line's top.
  - A card's meta sits on its title's line, right of the title.
  - The ⌘K hint shows, and the "Jump to epic" menu is a popover smaller than the window.
  - On the Design page the "Jump to epic" sheet fills the window (0, 0, 760 x 900).

## 62. Card columns

- **Scenario:** `mixed` at desktop, with a ticket whose title is wider than its card added.
- **Steps:** open `#/e/demo:mixed-epic`.
- **Pass:**
  - The left edges of all live card titles (not done, not cancelled) are equal.
  - The long title shows on one line with `text-overflow: ellipsis`.
  - Each meta's right edge is 16 px inside its card's border.

## 63. Card times

- **Scenario:** `united`
- **Steps:** open the Epics page and expand every section. Run `gx server queue remove demo:queued-epic`, then `gx server queue add demo:queued-epic`.
- **Pass:**
  - Every card but `follow-ups` reads `project · time · status`. Needs-you times read "waiting ...".
  - `running-epic`, `queued-epic`, the queued one-off, `design-live` and `design-stopped` show a bare
    duration. `queued-epic` reads "<1m" after the re-queue.
  - `ready-epic` reads "created ... ago" and its time has a tooltip. `design-new`, `draft-epic` and
    `idle-epic` read "created ... ago".
  - `follow-ups` has no time and reads "demo · 1 bug, 1 idea".
  - The Finished one-offs read "... ago".
