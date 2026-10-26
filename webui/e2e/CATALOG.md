# Web e2e flow catalog

Run with `make web-e2e`. Each flow starts its own fixture server (`testutil/gxfixture`), opens the
`web_url` once for the web cookie, and never reloads the page. The suite fails if a page reloads.
Retries are off. Locators use roles and visible text, plus `data-testid` `pill` and `reason` on
Epics page cards. The helpers `section` and `card` in `fixture.ts` find a section (expanding it) and a card.

Flows keep the numbers of the e2e plan, so a gap in the numbering is a flow not written yet.
Code: `epics.e2e.ts` (flows 1, 2, 6, 7, 8, 54), `pause.e2e.ts` (flows 3, 55, 56), `design.e2e.ts` (flows 4, 5, 9, 10, 11, 12, 13, 14, 42, 43, 44, 45, 46, 50),
`united.e2e.ts` (flows 15, 16, 17, 18, 19, 28, 29, 30, 31, 32, 33, 51, 52), `jump.e2e.ts` (flows 20, 21, 22), `phone.e2e.ts` (flows 23, 24, 41, 53),
`newdesign.e2e.ts` (flows 25, 26, 27, 34, 35, 36, 37, 38, 39, 40).

## 1. Queue an epic

- **Scenario:** `ready` (one epic, `ready-epic`, not queued)
- **Steps:**
  1. Open the Epics page; `ready-epic` is in Ready to queue.
  2. Click the card's Queue button.
- **Pass:** the card leaves Ready to queue and shows in In progress, with no navigation.

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
    the ring reads `4/7`, ticket 06 is expanded with "needs-answer · Waiting for your answer", its
    question and an Answer button. Ticket 07 is expanded in the deadlocked tone with "needs-repair · Run
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
    and on the ticket page. Opening a ticket from its card lands on `#/t/<address>`. The Repair row's
    card reads "needs-repair · Run failed".
  - The action clears the reason from the ticket page without a reload.
- **Rows:**

  | Row | Ticket | Reason | Action |
  | --- | --- | --- | --- |
  | Answer | `paused-epic/03` | Waiting for your answer | type an answer, Send answer |
  | Choose base | `paused-epic/04` | Ambiguous base | pick `01`, Choose base |
  | Retry | `paused-epic/06` | Agent stopped without a result | Retry |
  | Approve | `approve-epic/01` | Needs approval | Allow once |
  | Repair | `paused-epic/07` | Run failed | none: the card shows "+4 more", the epic page shows two "Fix it with an agent" buttons and no "Fix it in the TUI" |

- **Not covered yet:** Resolve conflict (`paused-epic/05`, `merge-conflict`). The server offers the
  button but refuses the resume ("ticket 05 is …, not asking").

## 4. Start a design session

- **Scenario:** `design` (`design-epic`, a map at Resolve tickets, and its `design-epic-impl` epic)
- **Steps:**
  1. Open the Epics page, then the Design page by its route (`#/d/demo:design-epic`); it shows
     "Start design session". No link reaches a Design page before its session starts.
  2. Click Start design session.
- **Pass:** The back link reads "← Epics" with no count. Progress shows Resolve tickets as the current step, and the chat shows the first scripted
  message ("Two questions before the spec.") in place of the start button.

## 5. Answer a round

- **Scenario:** `design`
- **Steps:**
  1. Start the session. Round one has `Q8` (options) and `Q8b` (yes/no).
  2. Pick SQLite and Yes, then Send.
  3. Click Continue.
- **Pass:**
  - The question dots and titles read `Q8` and `Q8b`.
  - The chat shows the answers as the person's message, so the agent received them.
  - Continue shows after the agent's run-end line.
  - Continue brings a fresh round with `Q9`.

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
- **Pass:** the card is in the Finished section (collapsed until opened) as `2/2`; its epic page
  reads "Finished · $0.00 · 0s agent time" with the ring `2/2`, no comet, the divider "2 done", and a
  More stats card that is `aria-expanded` false, then true after a click.

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
- **Pass:** each step becomes current in turn (Done becomes done), earlier steps read done, and
  "Open implementation epic →" shows only at Branch and Done.

## 10. Auto-advance

- **Scenario:** `design`, with a 3s countdown (`--auto-advance-delay`)
- **Steps:** start the session, answer round one (SQLite, Yes), Send.
- **Pass:** after the run-end line the Composer shows "Continuing in Ns" and Cancel; with no press
  the fresh round ("A fresh round.", `Q9`) appears.

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
  1. The switch reads "Auto-advance on" while the agent works.
  2. Turn it off, answer round one, Send; the run ends.
  3. Turn it on.
- **Pass:** with the switch off the run end shows Continue and no countdown; turning it on starts
  "Continuing in Ns".

## 13. Defaulted questions

- **Scenarios:** `design-mixed` (one asked question, two defaulted), `design-defaulted` (all defaulted)
- **Steps:** start the session. In the mixed case answer the card Yes and Send.
- **Pass:**
  - Mixed: one card ("Q12 - Keep going?"); Q10 and Q11 show as plain text; the sent answer
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
  - Each title shows its card count: eight in Needs you (paused, question, permission, deadlock,
    merge-blocked, awaiting-merge, Design question, paused one-off), and Finished counts the cards
    on screen.
  - Every Needs-you card has a pill; no other section has one. The stopped Design sits in In
    progress with Continue.
  - A click on a card's project text stays on `#/epics`; its title opens `#/e/demo:paused-epic`.
  - Merge is the only button on the merge-blocked and awaiting-merge cards. Queued, running,
    finished and draft cards have no buttons.
  - A filter shows only the matching card, and "No match" when nothing matches.
- **Not covered:** a Ready to queue card with a blocker (the scenario has none, so its reason is
  unchecked).

## 17. Merge from the card

- **Scenario:** `united`
- **Steps:** click Merge on `awaiting-epic`, then on `blocked-merge-epic`.
- **Pass:** each card shows the server's refusal, and the page stays on `#/epics`. The fixture's
  epics have no git branch, so a successful merge is not covered.

## 18. Joined card

- **Scenario:** `design`, started with `--hold`
- **Steps:** run `gx tickets set demo:design-epic --design-step create-tickets`, then `branch`, then
  `gx server queue add demo:design-epic-impl`.
- **Pass:**
  - One card, `design-epic`, with no `design-epic-impl` card. Its title opens `#/e/demo:design-epic`.
  - The status reads "Designing" with a percentage in the ring, then "Creating tickets (N so far)..."
    with "…", then "Not started" with `n/m` at Branch, then "Implementing..." once queued. The title
    opens `#/e/demo:design-epic-impl` from Branch on.
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
  - The back link reads "← Epics · 7 more need you" on `paused-epic` (itself in Needs you) and
    "· 8 more need you" on the other two pages. A stopped Design is not in Needs you.
  - The title button has no number and no `waiting-count` element exists.
  - Clicking the count text opens `#/epics`.
  - The count drops from 8 to 7 with no reload.

## 23. Phone, Epics page

- **Scenario:** `united`, at 375 px wide (`test.use({ viewport })`)
- **Pass:**
  - Two cards of Needs you share a left edge and the second sits below the first.
  - On `#/d/demo:design-live` the back link reads "← Epics · 8 more" (or the full text) on one line.

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
  2. Click the Progress summary, then Close in the modal. Open it again and press Escape.
  3. Tap the title; click Close on the sheet.
  4. Press Send with nothing answered, then Escape. Press ⤢, then ⤡.
- **Pass:**
  - The composer has no Focus button. The Send popover is at most 343 px wide, inside the viewport and overlaps the sheet.
  - Focus mode keeps the Progress summary above the sheet and hides the messages; ⤡ leaves.
  - The summary opens the "Design progress steps" modal with Resolve tickets current. Close and Escape both close it.
  - Note reads only its icon; the Auto-advance switch has no text.
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
- **Steps:** start `retry-design`, stop its session, stay on `#/d/demo:retry-design`; click the header "New session".
- **Pass:** no dialog and no Epic slug field appear, and `gx agents list` shows a second session for it.

## 44. Failure banner New session

- **Scenario:** `ready`
- **Steps:** start `ended-design`, stop its session (it ends with no run-end); click "New session" in the `Session failure` region.
- **Pass:** the banner says "Session ended". No dialog appears; `gx agents list` shows a second session.

## 45. Delete design

- **Scenario:** `ready`; the second test uses `design`.
- **Steps:**
  1. Start `delete-me`; "Delete design" is absent. Stop its session with the page open.
  2. Click "Delete design", then Keep, then "Delete design" and Delete.
  3. Start `delete-me` again.
  4. On `design`, open `#/d/demo:design-epic`.
- **Pass:**
  - "Delete design" appears with no reload. Keep returns to the page with the button still there.
  - Delete goes to `#/epics`, the card is gone and `gx tickets show demo:delete-me` fails.
  - The second New Design with `delete-me` succeeds.
  - `design-epic` (charted) shows no "Delete design".

## 46. gx designs delete

- **Scenario:** `ready`
- **Steps:** `gx designs open cli-del`, stop its session, run `gx designs delete demo:cli-del`; run the same verb on `demo:ready-epic`.
- **Pass:** `gx tickets show demo:cli-del` fails. The verb on `ready-epic` exits non-zero and the epic stays.

## 47. Send popover

- **Scenario:** `design`
- **Steps:**
  1. Start the session and press Send with nothing answered.
  2. Click the `Q8b` row. Fold the sheet with ▾, press Send, click the `Q8` row.
  3. Pick SQLite on `Q8`, press Send.
  4. Press Escape. Press Send and click the chat. Press Send twice.
  5. Press Send, then Enter on the focused "Send anyway".
- **Pass:**
  - A popover named "Unanswered questions" shows above Send, ends within 2 px of Send's right edge, and the composer keeps its height.
  - It reads "2 questions are unanswered" and "0 of 2 answered", with `Q8` and `Q8b` rows; after the answer, "1 question is unanswered" and "1 of 2 answered".
  - The `Q8b` row closes it, makes `Q8b` the current dot and focuses its first choice. On a folded sheet the `Q8` row opens the sheet at `Q8`.
  - "Send anyway" is focused on open. Escape and a click on the chat close it and focus Send. The second Send press closes it and posts nothing.
  - Enter sends the round with `Q8b` as "(unanswered)", with no second dialog and no "Answer them" button.

## 48. Fixed sheet rows

- **Scenario:** `design`, at 1280x600.
- **Steps:**
  1. Start the session, click "📝 Note" on `Q8`.
  2. Scroll the question body to its end.
- **Pass:**
  - The note's `rows` is 3.
  - The dots, "0 of 2 answered" and the Prev/Next row keep their positions and stay visible.
  - The body's scroll position changes and the sheet's top edge does not.
  - The same checks hold in focus mode (enter with ⤢ first).

## 49. Focus mode

- **Scenario:** `design`
- **Steps:**
  1. Start the session. Press ⤢ (left of ▾), then ⤡.
  2. Enter, open the `Q8` note, focus its field, press Escape.
  3. Enter, press Send with gaps, Escape twice. Repeat with Ctrl+K and the jump menu.
  4. Pick SQLite, type a composer note, enter, leave.
  5. Enter, press ▾; press ⤢ on the folded sheet.
  6. Answer `Q8b`, press Send, then Continue.
- **Pass:**
  - ⤢ sits left of ▾. Entering hides "Two questions before the spec."; the sheet is taller than half the viewport; the title, Progress region and composer stay; the question title is larger than docked; the button reads ⤡.
  - Escape from the note field leaves. With a layer open, the first Escape closes only the Send popover or the jump menu, the second leaves.
  - SQLite, `Q8b` as the current dot and the composer note survive entering and leaving.
  - ▾ folds and leaves; ⤢ on the folded sheet unfolds and enters.
  - Sending leaves with the sent round shown; Continue opens the `Q9` round docked. No "Questions, full screen" dialog appears.

## 50. Design epic page

- **Scenario:** `design`
- **Steps:**
  1. Write a long Destination section on `design-epic` with `gx tickets section`; open `#/e/demo:design-epic`.
  2. Click Start design session; open the epic page again.
- **Pass:**
  - Before the session the ring reads a percentage with `data-tone="muted"`. The summary shows in full under the title.
  - The stats line starts with "Resolve tickets", shows a time ("2m") and has no "$".
  - The open tickets read `01 first implement · ready` and `02 second implement · after 01`. The done ticket `03 timed` shows "2m" and no "$".
  - After the start the ring is `running` and the comet shows.

## 51. Merge from the epic page

- **Scenario:** `united`
- **Steps:** open `#/e/demo:awaiting-epic`, then `#/e/demo:blocked-merge-epic`.
- **Pass:**
  - `awaiting-epic` has a Merge button with `data-tone="done"`, last in the button group.
  - `blocked-merge-epic` shows "needs a rebase" under the bar. Merge is still there, as the retry.

## 52. Draft impl epic

- **Scenario:** `united`
- **Steps:** open `#/e/demo:publishing-epic-impl`.
- **Pass:** the stats line starts with "Publishing tickets". Queue is disabled and its wrapper has the title "Queue unlocks once all tickets are published".

## 53. Phone, epic page

- **Scenario:** `united` at 375 px wide; the second test uses `mixed`.
- **Steps:** open `#/e/demo:awaiting-epic`; open `#/e/demo:mixed-epic`.
- **Pass:**
  - Every button of the bar sits below the stats line and is at least 44 px tall.
  - On a card the meta text ("ready") sits below the title ("open").

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
  - The dialog shows `/gx-investigate demo:paused-epic/07 is needs-repair: Run failed`; Copy reads "Copied" and the clipboard holds the prompt.
  - After the start the dialog reads "Agent started. View it in the gx TUI Agents tab" and has no "Start gx agent".
  - `gx agents list` shows one more session, with harness `claude` and dir the trunk checkout of `demo`.
  - The second dialog reads "an agent is already fixing demo:paused-epic/07" and `gx agents list` has no new session.

## 56. Fix it with an agent, remote control

- **Scenario:** `paused` with config `agents.fix-agent-remote-control=true`, `paused-epic` queued.
- **Steps:** open `paused-epic`; click "Fix it with an agent" on `07`; click "Start gx agent".
- **Pass:** the dialog reads "Agent started. View it in the gx TUI Agents tab or in the Claude app".
