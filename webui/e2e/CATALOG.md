# Web e2e flow catalog

Run with `make web-e2e`. Each flow starts its own fixture server (`testutil/gxfixture`), opens the
`web_url` once for the web cookie, and never reloads the page. The suite fails if a page reloads.
Retries are off. Locators use roles and visible text, plus `data-testid` `pill` and `reason` on
Epics page cards. The helpers `section` and `card` in `fixture.ts` find a section (expanding it) and a card.

Flows keep the numbers of the e2e plan, so a gap in the numbering is a flow not written yet.
Code: `epics.e2e.ts` (flows 1, 2, 6, 7, 8), `pause.e2e.ts` (flow 3), `design.e2e.ts` (flows 4, 5, 9, 10, 11, 12, 13, 14),
`united.e2e.ts` (flows 15, 16, 17, 18, 19), `jump.e2e.ts` (flows 20, 21, 22), `phone.e2e.ts` (flows 23, 24),
`newdesign.e2e.ts` (flows 25, 26, 27).

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
  - Before queueing, the epic page reads "Not queued · 2 of 7 done", folds "1 done · 1 cancelled",
    and shows ticket lines `open`, `blocked · after 02`, `drafted · draft`, `asks`, `fails`.
  - After queueing, the fake worker settles: the epic page reads "Running · 4 of 7 done", ticket 06
    shows "needs-answer" with its question, and the Epics page card shows `4/7`, a pill and a reason.

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
    and on the ticket page.
  - The action clears the reason from the ticket page without a reload.
- **Rows:**

  | Row | Ticket | Reason | Action |
  | --- | --- | --- | --- |
  | Answer | `paused-epic/03` | Waiting for your answer | type an answer, Send answer |
  | Choose base | `paused-epic/04` | Ambiguous base | pick `01`, Choose base |
  | Retry | `paused-epic/06` | Agent stopped without a result | Retry |
  | Approve | `approve-epic/01` | Needs approval | Allow once |
  | Repair | `paused-epic/07` | Run failed | none: the card shows "+4 more", the epic page shows "Fix it in the TUI" twice |

- **Not covered yet:** Resolve conflict (`paused-epic/05`, `merge-conflict`). The server offers the
  button but refuses the resume ("ticket 05 is …, not asking").

## 4. Start a design session

- **Scenario:** `design` (`design-epic`, a map at Resolve tickets, and its `design-epic-impl` epic)
- **Steps:**
  1. Open the Epics page, then the Design page by its route (`#/d/demo:design-epic`); it shows
     "Start design session". No link reaches a Design page before its session starts.
  2. Click Start design session.
- **Pass:** Progress shows Resolve tickets as the current step, and the chat shows the first scripted
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
  reads "2 of 2 done".

## 8. Edge states

- **Scenario:** `edge` (project `bare` with no epics, `empty-epic` with no tickets, `blocked-epic`
  with ticket 02 blocked by the unmerged 01)
- **Steps:**
  1. Open the Epics page; the `empty-epic` card is in In progress, reading "Implemented".
  2. Open `empty-epic`.
  3. Click "← Epics" and open `blocked-epic`.
- **Pass:**
  - `empty-epic` reads "No tickets yet".
  - `blocked-epic` shows `waiting · blocked · after 01` and `unmerged · open`.
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
  - Sections read Needs you, In progress, Ready to queue, Ready to design, Finished. Ready to design
    and Finished start collapsed, and the header toggles Ready to design.
  - Each title shows its card count: nine in Needs you (paused, question, permission, deadlock,
    merge-blocked, awaiting-merge, Design question, stopped Design, paused one-off), and Finished
    counts the cards on screen.
  - Every Needs-you card has a pill; no other section has one.
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
  - The impl epic page links "Open design →" and the design epic page links "Open implementation epic →".

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
- **Not covered:** the `design-epic` Design of the spec text. `united` has no joined card, so the Design
  pick uses `design-live`.

## 22. Jump menu count and back link

- **Scenario:** `united`
- **Steps:** open `#/e/demo:paused-epic`, then `#/e/demo:running-epic`; click "← Epics". Open
  `#/d/demo:design-live`; click "← Epics".
- **Pass:** the title count of other Needs-you cards reads 8 on `paused-epic` (itself in Needs you) and
  9 on the other two pages. "← Epics" opens `#/epics` from the epic page and the Design page.

## 23. Phone, Epics page

- **Scenario:** `united`, at 375 px wide (`test.use({ viewport })`)
- **Pass:** two cards of Needs you share a left edge and the second sits below the first.

## 24. Phone, Design page

- **Scenario:** `design`, at 375 px wide
- **Steps:**
  1. Open `#/d/demo:design-epic` and start the session.
  2. Click the Progress summary, then Close in the modal.
  3. Tap the title; click Close on the sheet.
- **Pass:**
  - The summary opens the "Design progress steps" modal with Resolve tickets current.
  - Note and Focus read only their icons; the Auto-advance switch has no text.
  - The title opens a "Jump to epic" sheet that fills the screen; Close closes it.

## 25. New Design from an empty page

- **Scenario:** `empty` (one project, no epics)
- **Pass:** the page reads "No epics yet." and offers New Design.

## 26. New Design form

- **Scenario:** `ready`
- **Steps:**
  1. Click New Design (right of the filter); fill the name `fresh-design` and an idea; Start design.
  2. Click "← Epics" and the new card's title.
- **Pass:**
  - The form is a "New Design" dialog. Start opens `#/d/demo:fresh-design` and `gx server epics` lists it.
  - The card of the running session opens the same page.

## 27. Design a not-started Design

- **Scenario:** `united`
- **Steps:** open Ready to design; click Design on the `design-new` card.
- **Pass:** the page is `#/d/demo:design-new` and `gx agents list` names `design-new`.
