# Web e2e flow catalog

Run with `make web-e2e`. Each flow starts its own fixture server (`testutil/gxfixture`), opens the
`web_url` once, already signed in: the fixture finishes the web setup and `fixture.ts` adds the
`cookie_name` cookie to the browser. The `web-setup`, `web-login` and `web-logins` scenarios start
signed out or with extra web logins. `nextTotpCode()` returns a code for a new TOTP step, and
`start(scenario, { config: { "server.web.totp": "false" } })` runs without TOTP. It never reloads the page. The suite fails if a page reloads.
The `limits` scenario seeds the limits state: `start("limits", { shape })` takes `fresh` (default),
`stale`, `past-reset` or `one-window`. The `spend` scenario seeds the budget ledger
(`start("spend", { shape })` takes `normal` (default), `short` or `empty`) and holds one fake agent
running; append an assistant usage line to `fixture.agentTranscript` (create its directory first)
and the ledger, polled every second, adds its cost. `fixture.cliInput(input, ...args)` and
`fixture.cliEnvInput(extra, input, ...args)` are `cli`/`cliEnv` with `input` on stdin.
Retries are off. Locators use roles and visible text, plus `data-testid` `pill` and `reason` on
Epics page cards. The helpers `section` and `card` in `fixture.ts` find a section (expanding it) and a card.

Before you write a flow, read "How the fixture behaves" in `skills/gx-ui-check/SKILL.md`: how a fake
worker outcome maps to ticket status, why fixture CLI writes land late, how to set an epic summary,
and "read `test-results/**/error-context.md` first on a locator failure".

Flows keep the numbers of the e2e plan, so a gap in the numbering is a flow not written yet.
Code: `epics.e2e.ts` (flows 1, 2, 6, 7, 8, 54), `pause.e2e.ts` (flows 3, 55, 56), `design.e2e.ts` (flows 4, 5, 9, 10, 11, 12, 13, 14, 42, 43, 44, 45, 46, 50),
`united.e2e.ts` (flows 15, 16, 17, 18, 19, 28, 29, 30, 31, 32, 33, 51, 52), `jump.e2e.ts` (flows 20, 21, 22), `phone.e2e.ts` (flows 23, 24, 41, 53),
`newdesign.e2e.ts` (flows 25, 26, 27, 34, 35, 36, 37, 38, 39, 40), `dirty-trunk.e2e.ts` (flows 57, 58, 59), `design-page.e2e.ts` (flows 71, 72, 73, 74, 75), `disk.e2e.ts` (flows 76, 77, 78), `agents-sidebar.e2e.ts` (flows 72 "Agents tab", 73 "Agents sidebar", 74 "Design sessions toggle", 89, 92, 113), `agents-session.e2e.ts` (flows 75, 77, 80, 81, 82, 83, 103, 104), `agents-composer.e2e.ts` (flows 76 "Send and draft", 78 "Send refused", 79 "Keyboard send", 91, 93, 114), `agents-switcher.e2e.ts` (flow 84), `agents-create.e2e.ts` (flows 85, 86, 87, 88, 90), `codex.e2e.ts` (flows 117, 118, 119, 120, 121, 122), `menu.e2e.ts` (flows 117, 118, 119, 120, 121), `migration.e2e.ts` (flows 161, 163, 168, 169, 170, 171), with shared helpers in `agentsKit.ts`. The numbers 72 to 74 and 117 to 121 are also used by other files, so run these with `-g "^72\. Agents tab"` and so on.

## 1. Queue an epic

- **Scenario:** `ready` (one epic, `ready-epic`, not queued)
- **Steps:**
  1. Open the Epics page; `ready-epic` is in Ready to queue.
  2. Click the card's Queue button, with the `/v1/queue/add` call held.
- **Pass:** the card's button is disabled while the call runs. After it, the card leaves Ready to
  queue and shows in Implementing, with no navigation and no toast.

## 2. Epic looks right

- **Scenario:** `mixed` (`mixed-epic`: done, open, blocked, draft, cancelled, asking and failing tickets)
- **Steps:**
  1. Open the Epics page; the `mixed-epic` card reads `demo` and `2/7`.
  2. Open `mixed-epic`.
  3. Click Queue on the epic page.
  4. Click the top bar's "Epics" link.
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
- **Pass:** without any action in the page, the card moves to Implementing.

## 7. Epic completion

- **Scenario:** `finishing`, started with `--hold` so the last ticket stays running until the flow
  releases it (`fixture.release()`).
- **Steps:**
  1. Open the Epics page; the `finishing-epic` card is in Implementing and shows `1/2`.
  2. Release the hold; the last ticket finishes and merges.
- **Pass:** before the release, the running ticket `02`'s card meta starts with "running · ". After it
  the card is in the Finished section (collapsed until opened) as `2/2`; its epic page reads
  "Merged · $0.00 · 0s agent time" with the ring `2/2`, no comet, the divider "2 done" above the
  cards, and a More stats card below them that is `aria-expanded` false, then true after a click.

## 8. Edge states

- **Scenario:** `edge` (project `bare` with no epics, `empty-epic` with no tickets, `blocked-epic`
  with ticket 02 blocked by the unmerged 01)
- **Steps:**
  1. Open the Epics page; the `empty-epic` card is in Implementing, reading "Implemented".
  2. Open `empty-epic`.
  3. Click the top bar's "Epics" link and open `blocked-epic`.
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
  - Sections read Needs you, Implementing, Ready to queue, Designing, Ready to design, Finished. Ready to design
    and Finished start collapsed, and the header toggles Ready to design.
  - Each title shows its card count: nine in Needs you (paused, question, permission, deadlock,
    merge-blocked, dirty-trunk, awaiting-merge, Design question, paused one-off), and Finished counts the cards
    on screen.
  - Every Needs-you card has a pill; no other section has one. The stopped Design sits in
    Designing with Continue.
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
  - On the one-off's ticket page the top bar's Epics link reads "Epics" with no count.
- **Not covered:** the `design-epic` Design of the spec text. `united` has no joined card, so the Design
  pick uses `design-live`.

## 22. Back link count

- **Scenario:** `united`
- **Steps:**
  1. Open `#/e/demo:paused-epic`, `#/d/demo:design-live`, then `#/e/demo:running-epic`.
  2. Click the count text of the Epics link.
  3. Open `#/e/demo:running-epic`; run `gx tickets set demo:paused-epic/01 --status open`.
- **Pass:**
  - The top bar's Epics link reads "Epics · 8 need you" on `paused-epic` (itself in Needs you) and
    "Epics · 9 need you" on the other two pages. A stopped Design is not in Needs you.
  - The title button has no number and no `waiting-count` element exists.
  - Clicking the count text opens `#/epics`.
  - The count drops from 9 to 8 with no reload.

## 23. Phone, Epics page

- **Scenario:** `united`, at 375 px wide (`test.use({ viewport })`)
- **Pass:**
  - Two cards of Needs you share a left edge and the second sits below the first.
  - A card's `time` element is no taller than 1.5 line heights.
  - On `#/d/demo:design-live` the top bar's Epics link reads "Epics · 9 need you" on one line.

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
  2. Click the top bar's "Epics" link, open New Design again, press Escape, then click the new card's title.
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
  3. Click the Vi mode item (under AFK); run `gx vimode` (a flip).
  4. Click the build text; click AFK again; press Esc.
  5. Run `gx server queue pause`.
- **Pass:**
  - The AFK item reads unchecked, then checked after the click, and the menu stays open. `gx afk`
    reports `afk off`, so the server was on, and the item reads unchecked again.
  - The Vi mode item reads unchecked, then checked after the click, and the menu stays open.
    `gx vimode` reports `vimode off`, so the server was on, and the item reads unchecked again.
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
  3. In the Questions view, open the Send popover, the jump menu (Ctrl+K) and the ⋯ menu in turn. Press Escape twice each time. Then open the `Q8b` note editor, press Escape, and press Done.
  4. Type a composer note, switch Chat, Questions, Chat, Questions.
  5. Resize to 1440px, then to 375px; open the steps sheet from the step line and press Escape twice.
  6. In a second test: press Chat, answer both questions, press Send, then Continue.
  7. In a third test on `design-working`: start the session.
- **Pass:**
  - After Start the View toggle reads Questions (pressed) with "0 of 2"; the sheet is taller than half the viewport; "Two questions before the spec." is hidden. After SQLite it reads "1 of 2".
  - In Chat the chat shows and ▾ folds and opens the sheet; in Questions there is no ▾. Escape in Questions shows the chat.
  - With a layer open, the first Escape closes only the layer and the second shows the chat; at 375px the same for the steps sheet.
  - The note editor ignores Escape and stays open; Done closes it.
  - SQLite, `Q8b` as the current dot and the composer note survive two switches. No "Enter focus mode" or "Leave focus mode" button exists.
  - At 1440px the question title is at most 820px wide.
  - Sending from Chat shows the answers in Chat; Continue brings the `Q9` round with Questions pressed.
  - On `design-working` after Start there is no "View" group and no Questions sheet.

## 50. Design epic page

- **Scenario:** `design`
- **Steps:**
  1. Write a long Destination section on `design-epic` with `gx tickets section`; open `#/e/demo:design-epic`.
  2. Click Start design session, then open the epic page again at once. (Once round one lands the
     session waits for answers and the ring is `waiting`, so the ring is read before that.)
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

## 64. Design ring

- **Scenario:** `design` (`design-working` for the running check)
- **Steps:**
  1. At 390px and at 1440px, open `#/e/demo:design-epic` and read its ring label. Open `#/d/demo:design-epic`.
  2. Click Start design session and wait for round one. Run `gx tickets set demo:design-epic --design-step done`.
  3. On `design-working`, click Start design session.
- **Pass:**
  - The ring holds a Design badge (`role="img"` named "Design"). Its right edge is left of the `h1`, on the same row.
  - The ring label equals the Epic page's. Before Start `data-tone` is `muted`.
  - With round one up it is `waiting` and no comet shows. With the step at Done it is `done`.
  - On `design-working` after Start it is `running` and the comet shows.

## 72. Design actions menu

- **Scenario:** `design` (one live session), `design-two-live`
- **Steps:**
  1. On `design` open `#/d/demo:design-epic`, then click Start design session and open "More actions".
  2. On `design-two-live` open the menu, pick the "Claude Code" row, reopen it and click Stop on "design-epic".
  3. On `design-two-live` open the menu at 375px and at 1440px.
- **Pass:**
  - Before Start there is no "More actions" button. After Start no "New session" or "N live" button
    sits outside the menu. "New session" reads "Stop this session and start a fresh one" and there is
    no "Live sessions" heading.
  - With two sessions the menu shows "Live sessions · 2", a ✓ and `aria-current` on the shown row,
    and Stop only on the other. Picking the other row hides the Questions sheet (that session has no
    round). Stop makes `gx agents list` show one exited session.
  - At 375px the menu is a full-width bottom sheet with a grab handle. At 1440px it is a popover
    with no grab handle: its top is below the ⋯ button's bottom and it overlaps ⋯ horizontally.
  - The ⋯ button is a 44px square with a 1px border. At 1440px its top is above the title's bottom
    (level with the header row, not below it).
- **Note:** the menu's role name is "More actions" (the trigger's name), not "Design actions".

## 73. Note box

- **Scenario:** `design` (at 375px), `design-working`
- **Steps:**
  1. On `design` click Start design session, open "Note on question 8" and click "Open the note in a large editor". Click Done.
  2. Click the composer "Note", then its expand button. Fill the large editor, click Done, answer both questions and click Send.
  3. On `design-working` click Start design session, click "Note" and its expand button.
- **Pass:**
  - The `Q8` note textbox is in the viewport. Its modal is named for `Q8`.
  - The composer "Message" box has `rows` 5. Its modal is "Note on this round". After Done the box holds the typed text, and after Send the chat shows it.
  - On `design-working` (no round) the "Message" box has 5 rows and the expand button, and its modal is "Message to the agent".

## 74. Question top

- **Scenario:** `design` (viewport 1280x600, auto-advance held)
- **Steps:**
  1. Click Start design session. In the Chat view too, open "Note on question 8" so the `Q8` body is taller than the sheet.
  2. Scroll the `question-body` to its end, then move to `Q8b` by its dot. Go back to `Q8` (its dot is named "Question 8, answered" once answered), scroll to the end and repeat with Next →, with a pick (SQLite), and with a Send popover row (`Q8b`).
  3. Run all of it in the Questions view and again docked in the Chat view.
- **Pass:** each time the `Q8b` title's top is within 4px of the `question-body` top, and `Q8b` is the current question. Before each move the `Q8` body had scrolled (`scrollTop` above 0).

## 75. Text size

- **Scenario:** `design` (auto-advance held)
- **Steps:**
  1. Click Start design session. In the Questions view read the computed font size of the `Q8` title, its body ("SQLite or files?"), the first child of the SQLite choice and "0 of 2 answered".
  2. Open the composer "Note", type "prompt size probe", answer both questions and click Send; the chat shows.
  3. Read the font size of the chat message "Two questions before the spec." and the sent prompt. Continue starts a fresh session whose chat holds only the new round, so read them before Continue.
  4. Click Continue until `Q9` shows, click Chat and read the font size of the docked `Q9` title.
- **Pass:** the title is 24px, the body 17px, "n of m answered" 13px (the `xs` token), the choice 16px, the chat message and the prompt 16px, the docked title 18px.
- **Note:** "Earlier messages hidden" is not checked: no scenario shows it (the chat is never truncated), and it uses the same `xs` size as "n of m answered".

## 76. Disk banner and Clean now

- **Scenario:** `disk-low`
- **Steps:**
  1. Set the fake disk's free space to `15G` (under warn, above clean). Open an epic page and a ticket page, then go back to Epics.
  2. Click "Clean now" with `/v1/disk/clean` held, then release it.
  3. Set free to `21G`, then `23G`.
- **Pass:** the strip names "fixture disk" with "Clean now" on all three pages. The button is disabled while the call is held. At 21G the strip shows the new free space and stays. At 23G it is gone.

## 77. Disk hold

- **Scenario:** `disk-low` (workers held)
- **Steps:**
  1. Set clean to `6G`, then free to `5G`. Queue `disk-epic`. Open its page.
  2. Run `gx server tickets explain demo:disk-epic/01`.
  3. Set free to `11G`.
- **Pass:** the strip says new tickets are held. Ticket 01 shows the "disk low" verdict and never reads `running · `. Explain prints "disk low" and the disk name. At 11G the strip stays as a warning without the held text, and ticket 01 reads `running · ` with no reload.

## 78. Clean now fails

- **Scenario:** `disk-low`
- **Steps:** set clean to `fail`, free to `15G`, click "Clean now".
- **Pass:** the strip shows the failure (`failed: …clean.sh`) inline and stays.

## 72. Check config banner

- **Scenario:** `checks-missing`
- **Steps:** open the Epics page and filter to `demo`.
- **Pass:**
  - One "Missing check config" banner reads "demo has no check config. Agents run gx's built-in
    checks until you set it up. gx found 1 command to suggest."
  - It has a "Not now" and a "Set up checks" button. `auto` is not named.

## 73. Check config banner, all projects

- **Scenario:** `checks-missing`
- **Steps:** open the Epics page with the project filter on "All projects".
- **Pass:**
  - One banner reads "2 projects have no check config. Agents run gx's built-in checks until you
    set them up."
  - It lists `auto` and `demo`, each with its own "Set up checks" button, and has one "Not now".

## 74. Not now

- **Scenario:** `checks-missing`
- **Steps:** open the Epics page, watch the requests, click "Not now".
- **Pass:**
  - The banner is gone and the epic cards stay.
  - No request went to `/v1/checks/save`.

## 67. Set up checks, save to repo

- **Scenario:** `checks-missing`, project filter `demo`
- **Steps:** click "Set up checks", open "Show the file", click "Save".
- **Pass:**
  - The dialog "Set up checks · demo" presets Test as "Use command" with `make test`, a
    "Recommended" chip and "Found in Makefile · test". Format, lint and typecheck are on "gx default"
    with "Nothing found" and an empty disabled field. No row suggests the `build` script.
  - "Save to" defaults to committing `.gx/checks.json` on main. The preview shows
    `// .gx/checks.json (committed on main)` and `{"test": {"full": "make test"}}`.
  - After Save the toast reads "Committed .gx/checks.json on main", the dialog and the banner are
    gone, and `gx server checks` reports `demo` as not missing.

## 68. Set up checks, save to project.json and keep gx defaults

- **Scenario:** `checks-missing`, project filter `demo`
- **Steps:** pick "Keep it in gx's project.json" and Save. In a second run, click "Keep gx defaults".
- **Pass:**
  - Each run closes the dialog and removes the banner, and `gx server checks` reports `demo` as
    not missing.
  - "Keep gx defaults" saves `{}`.

## 69. Set up checks, refused commit

- **Scenario:** `checks-missing`, project filter `demo`
- **Steps:** write an uncommitted `.gx/checks.json` into `demo`'s repo, click "Set up checks", then
  "Save" with the repo target.
- **Pass:**
  - The dialog stays open and shows the error near its actions. The banner stays.
  - Trunk has no new commit and the local file is unchanged.

## 70. Phone, Set up checks

- **Scenario:** `checks-missing` at 400 px wide, project filter `demo`
- **Steps:** click "Set up checks".
- **Pass:**
  - Each kind row stacks: the field sits below its label.
  - The dialog is no wider than the viewport and the page has no horizontal scroll.

## 79. Tray states

- **Scenario:** `health`
- **Steps:**
  1. Set free space to `450G/500G`. On the Epics page, the `health-epic` page and its ticket 01 page, set CPU and memory to `cpu=10 mem=10`, `cpu=75 mem=85`, `cpu=95 mem=95`, then `cpu=unavailable mem=10`. Wait for the meters after each set.
  2. Repeat at 1100px and at 390px wide. Never reload.
- **Pass:** the C and M meters read normal, high, critical, then unknown, with fills of 3px, 12px (CPU) or 15px (memory), 15px, then no fill. D stays normal. The same holds on all three pages at both widths.
- **Note:** the fixture fails the whole reading when CPU is unavailable, so memory goes unknown too.

## 80. Held disk in the popover

- **Scenario:** `health`
- **Steps:** set free to `5G`, click the Tray, read the disk entry in the "System health" dialog.
- **Pass:** the D meter is critical. The disk entry is critical, says dispatch is held and points to "Clean now".

## 81. Tray popover

- **Scenario:** `health`
- **Steps:**
  1. Set free to `450G/500G` and click the Tray. Wait for the agent row in the CPU cause list.
  2. Read the header, rows, cause lists, disk entry and footer. Watch the footer count up.
  3. Set `cpu=60 mem=10`.
  4. Press Esc.
  5. Set the width to 390px and open the Tray again.
- **Pass:** the header shows `gx server` and the host. CPU, memory and disk rows show. The CPU and memory rows have no "high at" or "critical at" threshold text. The CPU list links `demo:health-epic/01` to its ticket page, names `go test` and lists "other users". The disk entry names the disk (`fixture disk`). The footer reads "Updated Ns ago" and N grows. Within 5s the CPU row reads "60% busy". Esc closes the popover. At 390px the popover and its rows stay inside the screen and cause names are cut short with an ellipsis.

## 82. Tray offline

- **Scenario:** `health`
- **Steps:** set `cpu=75 mem=85`, open the popover once and close it, stop the fixture server, then open the popover.
- **Pass:** the Tray fades and keeps C and M at high. The popover footer reads "Offline. Values from Ns ago." and the CPU row still shows "75% busy".

## 83. Tab titles

- **Scenario:** `mixed`
- **Steps:**
  1. Open the Epics page.
  2. Click the top bar's Stats link.
  3. Click Epics, open `mixed-epic`.
  4. Open ticket `02` from the epic page.
  5. Set the hash to `#/d/demo:mixed-epic`.
- **Pass:** one smoke case per page. The title is `Epics · gx`; `Stats · gx`; `mixed-epic · Epic · gx`;
  `<ticket 02's title> · Ticket · gx` (the title in the page's h1); `mixed-epic · Design · gx`. The ticket-title edge cases
  (pending, not found, blank title, refetch) live in the unit tests.

## 72. Agents tab

- **Scenario:** `agents`, then `empty`
- **Steps:**
  1. Read the top bar tabs. The count skips Design agents and Queue agents; one Design agent in `agents` raises an Actionable and stays out of it.
  2. Click Agents.
  3. Set the hash to `#/agents/<id>`, then open the same URL in a fresh page of the same context.
  4. Start `empty` and read the tab.
- **Pass:** the tabs read Epics, Agents and Stats. Agents shows the Actionable count from `gx agents list` (named "Agents · N needs you"). The deep link marks the session's row as current, in both pages. The count is hidden on `empty`.

## 73. Agents sidebar

- **Scenario:** `agents`
- **Steps:**
  1. Open `#/agents` and read the groups, rows, marks and "N live".
  2. Hover-read the Actionable mark's tooltip.
  3. Remove the Exited session with `gx agents remove`, then create a session in `demo` with `gx agents create`.
- **Pass:** the groups are `demo`, `other` and `site`, in creation order, with the two design sessions hidden. Each row shows its state mark, plus Unread or Actionable when `gx agents list` says so. The Actionable tooltip is the reason ("needs a decision"). "N live" counts the sessions that have not exited. The removed row leaves and the new row and "N+1 live" appear, with no reload.

## 74. Design sessions toggle

- **Scenario:** `agents`
- **Steps:**
  1. Open `#/agents`; read the toggle and the count.
  2. Check "Design agents", then open `#/agents` in a fresh page.
  3. Uncheck it, then select a design session.
- **Pass:** the "Design agents" toggle starts off with "· 2 hidden" and no design rows. Checking it shows both, and the fresh page has it still checked. With it off, the selected design session keeps its row and the count reads "· 1 hidden".

## 92. Last viewed session

- **Scenario:** `agents`
- **Steps:**
  1. Open `#/agents` with no session viewed.
  2. Open a session, then open `#/agents` in a fresh page.
- **Pass:** the first visit shows "Pick a session". The fresh page redirects to the viewed session and marks its row as current.

## 75. Read a session

- **Scenario:** `agents`
- **Steps:**
  1. Open an idle Claude session; read the header and the chat. Open a design session.
  2. Open the session that has Unread.
  3. Open the Working session.
  4. Open the idle session at 560px high, send eight prompts with `gx agents prompt` and read the newest reply; send a ninth; scroll to the top and send a tenth.
  5. Hold `/v1/sessions/chat` to answer 500, send a prompt, then stop holding and send another.
- **Pass:** the header shows the status mark, the state chip and the directory, and no harness chip for Claude. The chat shows the person message and the reply. A design session shows "Open design page →". Opening the Unread session drops its Unread mark and `gx agents list` shows it read. The Working session shows a plain "Working" row. The chat opens at the newest reply and follows the ninth; after scrolling to the top it stays there for the tenth. While the chat read fails, the chat stays and the new prompt does not show; after the next event it shows both.

## 77. Send while busy

- **Scenario:** `agents`
- **Steps:**
  1. Open the Working session.
  2. Type a draft in the Message box.
- **Pass:** the composer reads "Working. Send turns on when the session is idle." The draft stays in the box and Send is off.

## 76. Send and draft

Run with `-g "76\. Send and draft"`: the number 76 is also used by the disk flow.

- **Scenario:** `agents`
- **Steps:**
  1. Open an idle Claude session at 1280px and type a draft in the Message box.
  2. Open the same session in a new tab of the same browser.
  3. Press Send in the first tab.
- **Pass:** the first tab shows "Draft saved in this browser". The new tab has the draft in the box. After Send the box is empty, the note is gone, the chat shows the text and `gx agents chat <id>` lists it.

## 78. Send refused

Run with `-g "78\. Send refused"`: the number 78 is also used by the disk flow.

- **Scenario:** `agents`
- **Steps:**
  1. Open an idle Claude session. Hold `/v1/sessions/chat` to answer 500 and send a prompt with `gx agents prompt`.
  2. Type a draft, stop holding and press Send.
  3. Type `!ls` and press Send.
- **Pass:** the composer reads "the conversation moved; review it first" in a colour other than the draft note's. The chat reloads and shows the CLI prompt. The draft stays in the box and `gx agents chat` does not list it. The `!ls` send reads "text may not start with !" and keeps `!ls` in the box.

## 79. Keyboard send

Run with `-g "79\. Keyboard send"`: it matches three tests, and the number 79 is also used by the tray flow. The last two are the Design page half.

- **Scenarios:** `agents`; `design` and `design-working` for the Design page.
- **Steps:**
  1. At 390px open an idle Claude session, type text and press Ctrl+Enter, then press Send. Wait for Idle.
  2. At 1280px type a line, press Enter, type a second line, then press Ctrl+Enter.
  3. Design page, `design`: start the session, press Note, press Ctrl+Enter with no question answered, press Escape.
  4. Answer both questions and press Ctrl+Enter. When Continue shows, open Note and press Ctrl+Enter.
  5. Type a message and press Ctrl+Enter.
  6. Design page, `design-working`: start the session, open Note and send Ctrl+Enter to the box.
- **Pass:**
  - At 390px the key sends nothing and the text stays; the button sends.
  - At 1280px Enter adds a newline, no prompt goes out, and Ctrl+Enter sends both lines.
  - With gaps, the key opens the "Unanswered questions" popover with "2 questions are unanswered" and posts nothing.
  - With every question answered, the key posts the round once.
  - While Continue is the main button, the key posts nothing and opens no popover.
  - With text, the main button is Send and the key posts it.
  - While Interrupt is the main button the box is off. The key posts nothing and opens no popover.

## 81. Session without chat

- **Scenario:** `agents`
- **Steps:**
  1. Open the session of the non-Claude harness.
  2. Click Show screen, then press Esc.
- **Pass:** the notice reads "Chat works for Claude sessions only." The composer reads "Sending works for Claude sessions only" and Send is off. Show screen opens the screen dialog and Esc closes it.

## 82. No messages yet

- **Scenario:** `agents`
- **Steps:** open a Claude session whose chat is empty (`gx agents chat`).
- **Pass:** the chat reads "No messages yet" and the Message box shows.

## 80. Failure notices

Run with `-g "80\. Failure"`: it matches two tests, and the number 80 is also used by the tray flow. The second is the crashed half.

- **Scenarios:** `agents-failures`; `agents` for crashed.
- **Steps:**
  1. At 1280px open the stalled session (the fixture's stall threshold is 3s), click Screen, press Esc.
  2. Open the blocked session.
  3. Open the auth session and type a draft.
  4. Open the rate-limited session.
  5. On `agents`, open the crashed session.
- **Pass:**
  - Stalled shows "No progress for 15 min" with only Screen, which opens the screen dialog.
  - Blocked shows "Waiting at a prompt in the terminal" with only Screen.
  - Auth shows "Claude login needed" with only Retry, and Send is on once there is a draft.
  - Rate limit shows "Rate limited" and the reset text, no buttons, and the composer reads "Rate limited. Send turns on when the session is idle." with Send off.
  - Crashed shows "Agent crashed" and the composer reads "Exited. Restore it in gx, Agents tab." There is no Restore or New session button, and Send is off.

## 83. Design session on the Agents page

- **Scenario:** `agents`
- **Steps:**
  1. At 1280px open the design session.
  2. Click "Open design page →".
- **Pass:** the page shows the plain chat and the Message box, with no question sheet, no "Unanswered questions" popover and no Continue button. The link opens `#/d/demo:design-epic`.

## 89. Session not found

- **Scenario:** `agents`
- **Steps:**
  1. At 1280px open `#/agents/no-such-session`.
  2. Open the Exited session, then remove it with `gx agents remove`.
  3. At 390px click "Back to sessions".
- **Pass:** the panel reads "Session not found" and "<label> no longer exists." and the sidebar keeps its rows. At 1280px there is no "Back to sessions" link. After the removal the panel replaces the view, the row leaves, the other rows stay and the URL still names the removed session. At 390px "Back to sessions" opens `#/agents`.

## 84. Session switcher

- **Scenario:** `agents`
- **Steps:**
  1. At 1280px open an idle session. Click the title button, press Esc, then press Ctrl+K.
  2. Filter by a ticket, then by a project dir. Pick the first row.
  3. Press Ctrl+K, then Esc.
  4. Open `#/d/demo:design-epic` and press Ctrl+K.
- **Pass:**
  - Both open the switcher on the filter. The sections are Actionable, Unread and Other, and every session is listed, Design sessions included while the sidebar hides them.
  - The open session carries the "current" tag, and only that one.
  - Filtering by ticket or dir keeps only the matching rows and the "+ New session" row.
  - A pick closes the switcher and opens that session. Esc closes it.
  - On the Design page Ctrl+K opens the "Jump to epic" menu, not the switcher.

## 85. Create session

- **Scenario:** `agents`
- **Steps:**
  1. At 1280px open `#/agents` and press "New session in demo".
  2. Leave the worktree on Project root, type a first prompt and press Create session.
- **Pass:** the form opens with Project `demo` and Worktree "Project root". After Create the form closes, the page opens the new session and its row is selected. The chat shows the prompt and `gx agents list` lists the session in `demo`.

## 86. Create in a new worktree

- **Scenario:** `agents`
- **Steps:**
  1. At 1280px press "New session in demo". Type `bad name` in Worktree, pick "+ Create worktree", press Create session.
  2. Type `fresh-tree`, pick "+ Create worktree", type `/clear` as the first prompt and press Create session.
  3. Replace the prompt with `hello` and press Create session.
- **Pass:** the bad name and the `/` prompt each show a refusal in the form and keep the input. No session is created for either. The last create makes one session whose dir ends in `fresh-tree`, opens it and closes the form.

## 87. Create entry points

- **Scenario:** `agents`
- **Steps:**
  1. At 1280px on `#/agents` press "New session in other".
  2. Open a `site` session. Press "+ New session" in the sidebar, then the same row in the switcher.
  3. Press Ctrl+K, ArrowUp, Enter.
  4. Type a first prompt, close the form with ×, then open `#/agents` in a new tab of the same browser and press "+ New session".
- **Pass:**
  - The group's "+" prefills Project `other`.
  - The sidebar button, the switcher row and the Ctrl+K path each prefill `site`, the selected session's project.
  - Closing keeps the draft: the new tab's form has the prompt and Project `site`. No session is created.

## 88. Phone, Agents page

- **Scenario:** `agents`
- **Steps:**
  1. At 375px open `#/agents`.
  2. Press a session row, then "‹".
  3. Open a session, press its title, then Close.
  4. Press "‹", then the header "+ New session", then ×.
- **Pass:**
  - The list shows alone, with no "Pick a session" panel.
  - A row opens the session without the list; "‹" returns to `#/agents`.
  - The switcher fills the whole screen. Close closes it.
  - The New session form is a sheet with a grab handle, as wide as the screen and ending at its bottom edge.

## 90. Agents offline

- **Scenario:** `agents`
- **Steps:**
  1. At 1280px open an idle Claude session and type a message. Open "New session in demo" and type a first prompt.
  2. Stop the fixture server and wait for the Offline strip.
  3. Close the form.
- **Pass:** the strip reads "Offline, reconnecting…". Create session is off and the form keeps the prompt. The list and chat stay. The composer reads "Offline. Send turns on when gx reconnects.", Send is off and the message is still in the box.

## 91. Removed session draft

- **Scenario:** `agents`
- **Steps:**
  1. Open the Exited session in a second tab, type a message, then close the tab.
  2. Run `gx agents remove` on the session and open `#/`.
  3. Run `gx agents create`, open the new session.
- **Pass:** the stored draft is there after step 1 and gone after step 2. The new session's box is empty, with no "Draft saved in this browser" note. Session ids are random, so the new session cannot reuse the removed label.

## 93. Shared draft

- **Scenario:** `design`, with the auto-advance delay at `1h`.
- **Steps:**
  1. Open `#/d/demo:design-epic`, press Start design session and answer the first question.
  2. Open the same Design page in a new tab.
  3. Open the design session on `#/agents/<id>` in another tab. On the Design page press Note.
  4. Type in the Agents page's box, then in the Design page's Note box.
  5. Answer the other question and press Send answers.
- **Pass:**
  - The new tab shows the first question answered.
  - Text typed on the Agents page shows in the Note box, and the reverse.
  - Send answers sends the text as the note (`gx agents chat <id>` lists it). Both boxes are empty after.

## 94. New Design launches at the strongest tier

- **Scenario:** `design-model-tiers` (demo's worker is `sonnet`, its strongest `opus` at effort `max`).
- **Steps:** open New Design, name it `tier-fresh`, add an idea, Start design.
- **Pass:** the design session's fake claude is launched with `--model opus --effort max`.

## 95. New Agents tab session tiers

- **Scenario:** `design-model-tiers`
- **Steps:**
  1. Open `#/agents`, press + New session.
  2. Read the Tier options, create with the default tier and a first prompt.
  3. Create another with Strongest.
  4. `gx agents stop` the Strongest session, then `gx agents resume` it.
- **Pass:**
  - The Tier options read `Light (haiku)`, `Worker (sonnet)` and `Strongest (opus)`; Worker is selected.
  - The default session launches with `--model sonnet --effort medium`; the Strongest one with `--model opus --effort max`.
  - The resumed session has tier `strongest` and its header reads `opus · max`.

## 96. Session model label

- **Scenario:** `design-model-tiers`
- **Steps:** open each of the four design sessions.
- **Pass:** the header reads `opus · max` (tooltip "Strongest tier") for the sessions at chart, with an unconfirmed destination and with no design step, and `sonnet · medium` (tooltip "Worker tier") for the one at a later step.

## 97. Session model label follows the handoff

- **Scenario:** `charting-handoff`, with the auto-advance delay at `1s`.
- **Steps:** open the `handoff-auto` design session.
- **Pass:** once the handoff restarts it on the worker tier, the header reads `sonnet · medium`.

## 98. Session model label on an interactive session

- **Scenario:** `agents`
- **Steps:** create a session in project `other` from the Agents page, then send it a prompt and wait for the reply.
- **Pass:** the header reads `sonnet · medium` with the tooltip "Worker tier", before and after the reply.

## 99. First-run setup

- **Scenario:** `web-setup`
- **Steps:**
  1. Open `web_url`. Enter a wrong Setup code and continue.
  2. Enter the code from `gx server status`.
  3. Try a short password, then a mismatched pair, then a good one.
  4. On step 3, enter a wrong authenticator code, then the right one from the fixture's wait helper.
- **Pass:** the card shows "step 1 of 3" and the wrong-or-expired error. Step 2 shows "At least 6 characters.", refuses the short and mismatched passwords and goes on with the good one. Step 3 shows the QR, an "Add to authenticator app" link whose `href` starts with `otpauth://totp/` and the key. The wrong code shows the clock error and `gx server status` still prints a setup code. The right code opens the Epics page signed in.

## 100. Setup from the QR link

- **Scenario:** `web-setup`
- **Steps:** open the link `gx server web-url` prints, then finish setup.
- **Pass:** step 1 has the Setup code filled in and the page URL ends in `#/setup` with no code. After setup the page URL ends in `#/epics` (setup sets `#/`, which redirects there).

## 101. Setup and login with TOTP off

- **Scenario:** `web-setup`, with config `server.web.totp=false`.
- **Steps:** enter the Setup code and a good password twice, open Settings, sign out of this device, sign in again.
- **Pass:** the card shows "step 1 of 2" and its button reads "Finish setup", which opens the Epics page. Settings has no "Reset authenticator" row. The login card has no code field and the password alone opens the Epics page.

## 102. Login

- **Scenario:** `web-login`
- **Steps:** set the hash to `#/stats`. Sign in with the right password and a wrong code, then with the right password and the code from the fixture's wait helper.
- **Pass:** the login card shows. The wrong code shows "Wrong password or code". The right pair opens the Stats page at `#/stats`.

## 103. Login rate limit

- **Scenario:** `web-login`
- **Steps:** sign in with a wrong code 5 times, then once more.
- **Pass:** each of the 5 tries shows "Wrong password or code". The next one shows "Too many tries. Try again in N minute(s)." and Sign in is disabled.

## 104. Certificate help

- **Scenario:** `web-login`
- **Steps:** unfold "Browser warns about this page?"; request the profile and the Mac certificate links through the page context.
- **Pass:** the links show only after unfolding. The profile answers `application/x-apple-aspen-config` and the certificate `application/pkix-cert`, both with a body. The fingerprint groups match the `fingerprint:` line of `gx server status`.

## 105. Settings and web logins

- **Scenario:** `web-logins`
- **Steps:** open the menu, click Settings, open Web logins, open a row that is not this device, rename it, go back to the list.
- **Pass:** Settings shows the Security group with "4 devices". Web logins has four rows, one with "This device" and one with "Ends in 20 days unless used". The list shows the new name.

## 106. Sign out another device

- **Scenario:** `web-logins`
- **Steps:** a second browser context signs in through the login page. From the first page, open Web logins, open the second's row, click "Sign out this device" and confirm.
- **Pass:** the second page shows the login page with no reload. `gx server web-logins` no longer lists it.

## 107. Sign out of this device

- **Scenario:** `web-logins`
- **Steps:** open Settings and click "Sign out of this device".
- **Pass:** the login page shows. `gx server web-logins` lists three rows.

## 108. Change password

- **Scenario:** `web-logins`
- **Steps:** open Settings, click "Change password". Enter the current password, a code, and a new password twice. Submit, then open Web logins. In a new browser context, sign in with the new password.
- **Pass:** the page says "The 3 other devices will be signed out." After the change Settings shows and Web logins has one row. The new context signs in.

## 109. Reset authenticator

- **Scenario:** `web-logins`
- **Steps:** open "Reset authenticator", enter the password and a code, click Next, read the key, then go back to Settings. Sign in from a new context with the old secret. Open the page again and click Next, enter a wrong code, then a code from the new key shown. Open Web logins. Sign in from new contexts with a code from the old secret, then from the new one.
- **Pass:** the second step shows the QR code, the "Add to authenticator app" link, the key and the keep note. The old secret still signs in after leaving. The wrong code shows "That code doesn't match." After saving, Web logins has one row, the old secret is refused and the new one signs in.

## 110. Web reset from the CLI

- **Scenario:** `web-logins`
- **Steps:** run `gx server web-reset --yes`. Enter the printed setup code, a password twice and a code from the key shown.
- **Pass:** the open page shows setup step 1 of 3 with no reload. The printed code finishes setup and the Epics page shows.

## 111. CLI revoke

- **Scenario:** `web-logins`
- **Steps:** open Web logins, read this device's id from its row, then run `gx server web-revoke <id>`.
- **Pass:** the open page shows the login page with no reload. `gx server web-logins` no longer lists the id.

## 112. Add the authenticator after a CLI reset

- **Scenario:** `web-login`
- **Steps:** run `gx server web-reset-authenticator`. Open `web_url`. Submit the password without the setup code, then with it. Enter a wrong code, then a code from the key shown. In a new context, sign in with the old secret.
- **Pass:** the CLI prints a setup code and no key. The login card shows a Setup code field above the password. The password alone is refused. The next step shows the QR code, an `otpauth://totp/` link, the key and one code field. A wrong code shows "Wrong password or code". The right code opens the Epics page. The old secret is refused.

## 113. Queue agents switch

- **Scenario:** `queue-agents`
- **Steps:**
  1. Open `#/agents`; read the "Queue agents" and "Design agents" switches.
  2. Check "Queue agents", then open `#/agents` in a fresh page.
  3. Check "Design agents" and read the Agents tab.
- **Pass:** both switches start off with "· 1 hidden" and no rows. Checking "Queue agents" shows the Queue agent's row with only a status mark, and the Design agent stays hidden. The fresh page keeps the choice. The Agents tab reads no "needs you" count with both groups shown.

## 114. Read-only Queue agent

- **Scenario:** `queue-agents`
- **Steps:**
  1. Open `#/agents/<queue agent id>`.
  2. Click "Turn on composer", then Cancel.
  3. Click "Turn on composer", then confirm.
  4. Open the same URL in a fresh page.
- **Pass:** the page reads "gx is driving this agent, so the composer is off." and Send is disabled. The confirm dialog opens with Cancel focused. Cancel keeps it off. The confirm removes the line and the button, and the message box takes text. The fresh page is read-only again.

## 115. Blocked pause on PTY

- **Scenario:** `paused` (its `approve-epic` worker blocks on a permission prompt)
- **Steps:**
  1. Open the Epics page and the `approve-epic` card; wait up to 30 s (the 15 s dwell).
  2. Open ticket 01.
  3. Click "Allow once".
- **Pass:** the epic page reads "needs answer" after the dwell. The ticket page reads "Paused · Agent stuck", and its action card names the Approve, Deny and Retry buttons. "Allow once" clears the pause without a reload.

## 116. Retry on a question menu

- **Scenario:** `blocked-menu` (its worker blocks on a question menu)
- **Steps:**
  1. Open the `menu-epic` card; wait up to 30 s.
  2. Open ticket 01.
  3. Click Retry in "Recovery actions".
- **Pass:** after the dwell the epic page reads "needs answer" and the ticket page reads "Paused · Agent stuck" with no "Allow once". Retry lifts the pause.

## 103. Rate-limit hold

- **Scenario:** `rate-limited` (the `limited-epic` worker hits a Claude rate limit; `waiting-epic` is queued too)
- **Steps:**
  1. Open the `limited-epic` card.
  2. Open the `waiting-epic` page.
  3. Open the Agents page and turn on "Queue agents".
  4. Wait for the reset. The reset is a minute ahead and the hold adds a 60 s buffer, so allow 2 to 3 minutes in all (the flow raises its own timeouts, not the global one).
- **Pass:** the first ticket reads "rate-limited: resets <time>" and is not paused. The second ticket's row carries the rate-limit reason. The Queue agent row shows the rate-limited mark during the hold. After the reset the mark and the second ticket's reason are gone.

## 104. Usage gate and override

- **Scenario:** `usage-gated` (the `busy-epic` worker reports the weekly window at 99% and holds; `gated-epic` is queued too)
- **Steps:**
  1. Open the `gated-epic` page.
  2. Click "Override" on the ticket's verdict line.
- **Pass:** the ticket reads "Claude usage: 99% of the weekly limit, resets <time>" with an Override button. After the click the verdict is gone and the ticket starts, without a reload.

## 117. Default harness setting

- **Scenario:** `default-harness` (demo and dotfiles inherit, gx overrides to Codex; one waiting ticket; Claude, Codex and model-switched Claude agents in the Agents tab). Two more tests start their own fixture: `config: { "agent-runner": "headless" }`, and a Claude model and effort set to empty.
- **Steps:**
  1. Open Settings; the "Agents" row reads "Default: Claude". Open it.
  2. Choose Codex in the Default harness control.
  3. Click Override on `dotfiles`, then Remove override.
  4. On the Agents tab, create a session with "+ New session" (the form has no harness picker).
  5. Open a Claude, the new Codex and the switched session.
- **Pass:** the control shows Claude selected, `gx` has an override chip with Remove override, `demo` and `dotfiles` inherit with Override. The notice reads "Default is now Codex. 1 waiting ticket and the next design step will use it. N running agents keep Claude." The global config has `agents.default-harness` `codex` and keeps `agent-runner` and `server`. `dotfiles` gets the key `agents.default-harness`, then loses it, and keeps its other keys. The new session lists `harness` `codex`. Session headers read harness, tier and `model · effort`; the switched one adds "switched by hand". Under `headless` the page notes "Codex runs on PTY". With no model for the tier a session reads "Worker · CLI default". The herdr note is a vitest check.

## 118. Codex queue rows

- **Scenario:** `codex-queue` (one running Codex ticket, one running Claude ticket, and three waiting: no pin, pinned to Codex, ran before on Claude). A second test starts it with `fakecodex: false`.
- **Steps:**
  1. Open each ticket's epic page and read the row chips.
  2. Switch the Default harness to Codex in Settings, Agents. Raise the `demo` cap by one so a single waiting ticket can claim.
  3. Second test: raise the cap, take the unpinned and ran-before tickets off the queue, open the `pinned` epic, and run `gx server tickets explain demo:pinned/01`.
- **Pass:** the Codex running row reads `Codex` and `Worker`; the unpinned waiting row reads `follows default`; the pinned row reads `Codex` and `pinned`; the ran-before row reads `Claude` and `keeps Claude (ran before)`. After the switch only the unpinned ticket gets a Codex session; the other rows are unchanged. With Codex off PATH the pinned ticket's verdict reads `codex is not on PATH ...` and `explain` shows the same text.

## 119. Codex approval card

- **Scenario:** `codex-blocked` (one ticket queued with agent `codex`; the fake codex worker raises a `PermissionRequest` hook and stays blocked). Two tests, one per answer.
- **Steps:**
  1. Open the epic card and wait up to 30 s (the dwell) for "needs answer".
  2. Open ticket 01; the "Permission request" card shows under "Paused · Agent stuck".
  3. Test 1: click "Allow once". Test 2: fill "Note for Deny (optional)" and click "Deny".
- **Pass:** the pause clears without a reload. The fake's hook record holds one decision: `allow` for the first test, `deny` with the note as its reason for the second.

## 120. Codex rate-limit wait

- **Scenario:** `codex-rate-limited` (`codex-running-epic`: a held Codex ticket whose rollout shows the weekly window at 99%; `codex-waiting-epic`: a Codex ticket queued after the sample is stored; `claude-running-epic`: a held Claude ticket; `max-agents` 3)
- **Steps:**
  1. Open the `codex-waiting-epic` page.
  2. Open the `codex-running-epic` and `claude-running-epic` pages.
  3. Back on the waiting epic, click "Override" on the ticket's verdict line.
- **Pass:** the waiting ticket reads "Codex usage: 99% of the <window> limit, resets <time>" with an Override button. The Codex ticket is not paused and its row starts with "running · "; the Claude ticket's row starts with "running · " and has no verdict. After the click the verdict is gone and the ticket starts, without a reload; the Claude ticket still runs.

## 121. Codex error pause

- **Scenario:** `codex-error` (one ticket queued with agent `codex`; its first turn ends in a model error with no usage-limit signal, and the next prompt finishes it)
- **Steps:**
  1. Open the `codex-error-epic` card and wait up to 30 s (the dwell) for "needs answer".
  2. Open ticket 01.
  3. Click Retry in "Recovery actions".
- **Pass:** the ticket page reads "Paused · Agent stuck" and shows Codex's error text ("The 'gx-no-such-model' model is not supported ..."), with no "Allow once". Retry lifts the pause without a reload.

## 122. Codex design run

- **Scenario:** `codex-design` (`design-epic` with its design session already on Codex, Default harness `codex`)
- **Steps:**
  1. Open the `design-epic` Design page (`#/d/demo:design-epic`).
  2. In the Questions group pick SQLite for Q8 and Yes for Q8b, then click "Send answers".
  3. Click Continue until Q9 shows, then open the Chat view.
  4. Run `gx agents list --json` and `gx server stats --json`.
- **Pass:** the header shows `Codex`, the tier and `<model> · <effort>`. Q8 and Q8b show without a Start button. The chat holds a message starting `$gx-design `. After Continue the round is "Q9 - Keep going?" on the same session; the agent list has one design session with harness `codex`; the grilling question counts sum above zero.

## 117. Limits block

- **Scenario:** `limits` (shape `fresh`: 5h at 34%, weekly at 81%). Further tests start their own fixture: a touch context, and a 320px viewport.
- **Steps:**
  1. Open the menu and read the "Claude limits" block.
  2. Pipe a new statusline payload (55% and 96%, resets later than the seeded ones) into `gx claude statusline`.
  3. Hover the weekly "i" marker; in a touch context tap the 5h marker.
  4. At 320px wide open the menu.
- **Pass:** the title, "5h limit" and "Weekly limit" rows show 34% and 81% with progress bars (`aria-valuenow`); the weekly figure is amber and the 5h one is not; both rows read `resets in \d+[dhm]`. The order is Settings, AFK, the block, the build line. After the pipe the open menu reads 55% and 96% (red) without a reload. The tooltip holds a clock time by hover and by tap. At 320px the menu stays inside the viewport and adds no horizontal scroll (the page itself already scrolls one pixel sideways at that width).

## 118. Limits states

- **Scenario:** `limits`, three tests with shape `stale`, `past-reset` and `one-window`.
- **Steps:** open the menu in each.
- **Pass:** `stale`: both rows show "seen ... ago". `past-reset`: the 5h row reads "waiting" and "reset passed", has no bar and no "seen"; the weekly row is live with a bar. `one-window`: a single row, "Weekly limit".

## 119. Spending block

- **Scenario:** `spend`, tests with shape `normal`, `short` and `empty`, plus a touch context and a 320px viewport on `normal`. The seed is $3.00 for each of the last 40 days and $4.82 today.
- **Steps:**
  1. Open the menu and read the "gx spend (est.)" block.
  2. Hover the "i" marker; in a touch context tap it.
  3. Repeat the read for `short` and `empty`.
  4. At 320px wide open the menu.
- **Pass:** `normal`: Today $4.82; Since Sunday is 4.82 + 3 x days since Sunday; Since the 1st is 4.82 + 3 x (day of month - 1); no "since" note; the order is Settings, AFK, the block, the build line. The tooltip reads "Estimated cost of Claude runs started by gx. Not the bill. Codex runs and Claude sessions outside gx are not counted." by hover and by tap. `short`: Today has no note; each longer row has "since <weekday>, <month> <day>" (today's date) unless its window starts today (Sunday for Since Sunday, the 1st for Since the 1st). `empty`: `$0.00` on all three rows and no notes. At 320px the menu stays inside the viewport and adds no horizontal scroll.

## 120. Menu refresh

- **Scenario:** `spend` (shape `normal`).
- **Steps:**
  1. Open the menu and read the Today amount.
  2. Append a Sonnet usage line (1M input tokens, $3.00) to the fake agent transcript.
  3. Wait for the Today amount to rise, with no reopening and no reload.
  4. Close the menu with Escape, count `/v1/usage/summary` requests, pipe a limits report into `gx claude statusline` and wait 4 seconds.
- **Pass:** Today rises by $3.00 while the menu stays open (the `budget-changed` event is throttled to 60 s; the menu tick is the backstop). After closing, no summary request goes out, even after `limits-changed`.

## 121. Hook report

- **Scenario:** `spend` (shape `normal`).
- **Steps:**
  1. Open the menu: it shows "gx spend (est.)".
  2. Pipe a statusline payload with both windows into `gx claude statusline` against the fixture server.
  3. Run the same payload with `XDG_STATE_HOME` set to an empty directory (no server socket).
- **Pass:** the open menu flips to "Claude limits" with 34% and 81% rows, no reopening. The command prints the same output with and without a server.

## 123. Vi editor: layouts and touch bar

- **Scenario:** `design-working` (the message box with no round). Touch tests run with `test.use(touch)` (`hasTouch`, `isMobile`).
- **Steps:**
  1. Start the design session, click "Note", then "Open the note in a large editor".
  2. Touch at 390 px: read the bar, click Done. Touch at 1280 px: read the modal height.
  3. Mouse at 1280 px: read the modal size, header and footer; click Done.
- **Pass:** a touch device reports `(pointer: coarse)`. At 390 px with touch the bar holds Done only (no pill, no Esc) and Done closes the editor. At 1280 px with touch the bar shows and the modal is at most 680 px high. At 1280 px with a mouse there is no bar; the modal is 860 by 680 with the title, "Saved as you type", Done and no x button; Done closes it.
- **Vi on** (`gx vimode on`, wait for the menu's "Vi mode" item to be checked):
  1. Touch at 390 px: read the bar, the heights of the pill, Esc and Done; click Esc, then Done.
  2. Touch at 1280 px: read the header, bar, footer, Done buttons and status text.
  3. Mouse at 390 px: read the header, footer and buttons.
  4. Mouse at 1280 px: read the mode text position; click the editor, press Esc, then `i`; click Done. With vi off, open the editor and read the status text.
- **Pass (vi on):** at 390 px with touch the bar holds the mode text pill, Esc and Done, each 44 px high; Esc turns INSERT into NORMAL; Done closes. At 1280 px with touch there is the title header and the bar, no "Saved as you type" footer, one Done and one `role="status"` mode text. At 390 px with a mouse there is the title header and a footer with Done and the mode text, no bar and no x button. At 1280 px with a mouse the mode text is left of "Saved as you type", reads INSERT, NORMAL after Esc, INSERT after `i`; with vi off there is no mode text.

## 124. Vi editor: open, close and submit

- **Scenario:** `design-working` (the message box with no round). Vi mode is set with `gx vimode on|off`, then confirmed on the "Vi mode" menu item. The close tests start with `autoAdvanceDelay: "1h"` and wait for the 20 s first turn to end, because a working session disables the field. Two more tests start their own fixture: `agents` (the agent composer) and `design-text` (one free-text question, answer textarea "Answer to question 13").
- **Steps:**
  1. At 390 px, start the design session and click "Note".
  2. Vi on: focus the message box and press Cmd-G (Ctrl-G off a Mac). Again with focus on the page body.
  3. Vi off: press Cmd-G in the field, with a `window` keydown listener that records `defaultPrevented`.
  4. Vi on: fill `hello world`, put the caret after `hello`, press Cmd-G, type `X`.
  5. At 1280 px, vi on: type text, press Escape then `ZZ`. Open again and press Cmd-G. Press the backdrop at 1280 px, then at 390 px.
  6. Vi on and off at 1280 px with a mouse: click Done. In the editor press Escape in insert and in normal mode.
  7. Fill `hello world`, open with the caret after `hello`, press End, then close by Done, Cmd-G, Escape then `ZZ`, and the backdrop.
  8. Type in the editor, close it, press Send and read the `POST /v1/sessions/prompt` body.
  9. On `agents` at 1280 px, fill the composer, open its editor, press Cmd-Enter.
  10. On `design-text` start the design session, open the editor of "Answer to question 13", press Cmd-Enter. At 390 px on `design-working` press Cmd-Enter in the composer's editor.
- **Pass:** at 390 px the expand button shows. Vi on, Cmd-G in the field opens the editor; on the page body it opens nothing. Vi off, Cmd-G opens nothing and the keydown is not cancelled. The editor opens in INSERT and the typed `X` lands at the caret: the editor text reads `helloX world`. `ZZ` in normal mode closes the editor and the field keeps the text; Cmd-G inside the editor closes it. A backdrop press closes at 1280 px and does nothing at 390 px. With a mouse at 1280 px, vi on and off, Done is visible and closes. Escape leaves the editor open in insert and in normal mode. After closing by Done, Cmd-G, `ZZ` or the backdrop the field has focus and its caret is the editor's (after the last character, or on it after `ZZ`, because Escape steps back). The field follows typing in the editor, and Send posts a body holding the text. In the agent composer Cmd-Enter closes the editor and sends the message. On a question's answer editor, and at 390 px in the composer's editor, Cmd-Enter does nothing: the editor stays open and no prompt is posted.

## 125. Vi editor: commands

- **Scenario:** `design-working`, vi mode on (`gx vimode on`, confirmed on the "Vi mode" menu item). Each test waits out the 20 s first turn, then opens the editor with Cmd-G on text it fills in. The caret and text are read from the editor's DOM selection and `.cm-line` rows. The numbers in the flow list (118 in the ticket) were taken, so this flow is 125.
- **Steps:**
  1. Open the editor, press Escape, `v`, Escape, `i`; read the mode text.
  2. On `alpha beta gamma / delta epsilon / zeta` press `l h w e 0 fg j k G gg`; read the caret after each.
  3. At 390 px fill 100 words with no newline, press Escape and `j`; read the cursor box.
  4. On four lines press `5l j`, click the start of line 3, press `j`.
  5. On `one two three / four five` press `x`, `dw`, `yyp`, `u`, Ctrl-R, `iXY` Escape, `.`, `0vld`; read the text after each.
  6. On `abc` press `iX`, `aY`, `oZ` (Escape after each) and `.`; read text and mode.
  7. With touch at 390 px press `lll`, `ge`, `gs`, then `lllv`, `ge`, `gs`; read the caret.
- **Pass:** the mode text reads INSERT, NORMAL, VISUAL, INSERT. The keys move the caret as vim does (`j` and `k` within one character of the column, because they follow the pixel column). In a wrapped paragraph `j` moves the cursor down one line height and the caret stays far from the paragraph end. After a click `j` uses the clicked column. Every edit step gives the text vim gives (`.` repeats `iXY` as `XXYY`; `0vld` deletes two characters). `i`, `a`, `o` enter INSERT, Escape returns to NORMAL, `.` repeats `oZ`. With touch `ge` goes to the line end and `gs` to the line start in normal mode, and in visual mode `ge` and `gs` move the selection head to the line end and start.

## 126. Vi editor: layers and focus

- **Scenario:** `design` (question sheet), `paused` (start-over dialog), `design-working` (message box) and `united` (Triage dialog), all at 1280 px with vi mode on (`gx vimode on`). Touch is used for the Esc button test. The numbers in the flow list (119 in the ticket) were taken, so this flow is 126.
- **Steps:**
  1. On `design`, start the session, answer Q8, open the note of Q8b in the large editor, press Escape.
  2. On `paused` open `#/t/demo:paused-epic/08`, click Start over, open the Reason editor, press the backdrop at (5, 5).
  3. On `design-working` open the editor, press Cmd-K (Ctrl-K), click Done, press Cmd-K.
  4. In the editor type `ab`, press Tab and Shift-Tab six times each, then Tab with focus in the text. Repeat Tab with touch.
  5. On `united` open Ready to design, click Triage on the first card, press Tab and Shift-Tab eight times each.
  6. On `paused` open the start-over dialog, press Tab and Shift-Tab eight times, press Escape. Open it again, open the Reason editor, press Escape.
- **Pass:** Escape in the editor closes neither the editor nor the question sheet. The backdrop press closes only the editor; the start-over dialog stays. Cmd-K over the editor opens no Jump menu; after Done it opens "Jump to epic". Focus stays inside the editor on every Tab and Shift-Tab; with touch the cycle reaches Esc and Done. The text stays `ab` (no tab character). The editor is a dialog named "Message to the agent" with a textbox "Message to the agent text". In the Triage dialog and the start-over dialog focus never leaves. Escape closes the start-over dialog; with the editor open over it, Escape closes neither.

## 127. Vi editor: setting

- **Scenario:** `design-working`. The numbers in the flow list (121 in the ticket) were taken, so this flow is 127. Vi mode is flipped with `gx vimode on|off`.
- **Steps:**
  1. Open the top-right menu; read the "Vi mode" item; click it.
  2. With it on, open a second page of the same browser context on the same URL; open its menu.
  3. Turn vi on, wait out the first turn, fill `hello` with the caret after `he` and press Cmd-G. Read the mode text, text and caret. Run `gx vimode off`; read them again; press Escape and `x`. Run `gx vimode on`; press Escape and `x`.
- **Pass:** the item is a checkbox, unchecked, then `aria-checked` is true after the click. The second page shows it checked. With the editor open the mode text reads INSERT, goes away on `off` and returns on `on`; the text stays `hello` and the caret stays at 2 throughout. With vi off Escape does nothing and `x` types (`hexllo`). With vi on Escape gives NORMAL and `x` deletes (`hllo`). The restart half of the check is a Go test.

The restart flows (128-138, `restart.e2e.ts`; 139 onward, `restart-run.e2e.ts`, which restart for real) run on the `restart-*` scenarios, whose server path is a fake `gx` binary. A flow steers it through `setRestartControl` (`check.json`, `slow`, `old`, `start`, `gx`). `gx` names another gx build for the restart to run, for example `GX_E2E_GX_NEXT`, which reports version `fixture-next`. The shared helpers are in `restart-helpers.ts`. `restart-projects` has two projects, `demo` and `broken`, and no agents; its check file is rewritten to pass unless the flow wants the dry run to fail. `start` = `up` lets the restart bring the server back; the fixture stays up across that restart. A flow that restarts allows one reload (`reloads: 1`).

## 128. Restart server in the menu

- **Scenario:** `restart-projects`
- **Steps:** open the Menu; click "Restart server".
- **Pass:** "Restart server" sits below AFK and above the build text; the click closes the menu and opens the "Restart server" dialog.

## 129. Restart server keyboard

- **Scenario:** `restart-projects`
- **Steps:** focus the Menu button, press Enter, ArrowDown twice, Enter, then Esc.
- **Pass:** the second ArrowDown highlights "Restart server"; Enter opens the dialog; Esc closes it.

## 130. Restart dialog confirms

- **Scenario:** `restart-projects`, check passing with version `fixture-next`, `start` = `up`; one reload allowed.
- **Steps:**
  1. Open the dialog; wait for Restart to enable; press Esc.
  2. Open the dialog again; click Restart.
- **Pass:**
  - After Esc, `gx server status` shows the same `pid:` line: nothing restarted before the click.
  - After the click the dialog reads "Restarting...", then the tab reloads by itself and the pid line has changed.

## 131. Restart dialog checks

- **Scenario:** `restart-projects`, check passing, `slow` = 3 (seconds).
- **Steps:** read the build text in the Menu; open the dialog.
- **Pass:** the dialog reads "Checking..." with Restart disabled, then enables Restart. It shows "Running version: <the menu's build text>" and "Next version: fixture-next".

## 132. Restart dialog, same version

- **Scenario:** `restart-projects`, check version equal to the build text, `start` = `up`; one reload allowed.
- **Steps:** open the dialog; click Restart.
- **Pass:** the dialog says "Versions match" with Restart enabled; the restart completes (reload, new pid).

## 133. Restart dialog, other install path

- **Scenario:** `restart-launchd` (the fake binary sits at a launchd-style path), check version equal to the build text.
- **Steps:** open the dialog.
- **Pass:** the dialog says "Versions match" and Restart is enabled.

## 134. Restart dialog, unreadable version

- **Scenario:** `restart-projects` with `check.json` removed (the binary fails to run), then `restart-missing` (no file at the binary path).
- **Steps:** on each, open the dialog.
- **Pass:** the alert names the failure (`check.json`; "no such file"), Restart is disabled and `gx server status` shows the same pid.

## 135. Restart dialog, bad config

- **Scenario:** `restart-projects`, check with config `ok: false` and an error text.
- **Steps:** open the dialog.
- **Pass:** the alert shows the config error, Restart is disabled and the pid is unchanged.

## 136. Restart dialog, old binary

- **Scenario:** `restart-projects`, control file `old` present (no `restart-check` command).
- **Steps:** open the dialog.
- **Pass:** the alert names `gx server restart`; Restart is disabled.

## 137. Restart dialog, running agents

- **Scenario:** `restart-agents` (held, one running agent), `start` = `up`; one reload allowed.
- **Steps:** open the dialog until its check counts the agent; read it. Rewrite `check.json` with a pending step in `demo`, reopen, read it; click Restart.
- **Pass:** the dialog reads "1 agent is running. Agents keep running through the restart."; with nothing pending it does not read "A migration runs while the agents work."; with a pending step it does; the restart completes (reload, new pid).

## 138. Restart dialog, dry run

- **Scenario:** `restart-projects` (project `broken` fails its dry run), `start` = `up`; one reload allowed.
- **Steps:** open the dialog; click "Restart anyway".
- **Pass:** the alert shows "broken: fixture migration failed" and says the project will be in Migration block after the restart and can be retried from the strip; the button reads "Restart anyway" (no plain "Restart"); the click restarts (reload, new pid). Nothing is saved as a failure by the restart.

## 139. Restart with a failed migration

- **Scenario:** `restart-projects`, `start` = `up`; four reloads allowed. The flow writes a ticket with `status: bogus` into `broken`'s epic, so the migration of `broken` fails when the new server starts, and `provisional_destination` into `demo`'s epic ticket.md.
- **Steps:**
  1. Empty the projects' applied-migrations lists (applied steps do not rerun), then restart; wait for the reload. Each later break empties the lists again.
  2. On the Epics page and on the demo-epic page, read the strip.
  3. Rewrite the bad ticket to valid and click Retry migration.
  4. Break it again and restart; rewrite it to valid and run `gx tickets migrate '<dir from the strip>'`.
  5. Break it again and restart; then fix it and restart again.
- **Pass:**
  - The pid changes and `gx server status` says running although `broken` failed.
  - `demo`'s epic ticket.md now has `destination_unconfirmed` and no `provisional_destination` (rewritten at start).
  - On both pages a strip reads "broken: Migration blocked" with the error, a `gx tickets migrate` command and a Retry migration button.
  - The strip is gone after Retry migration and after the manual migrate, shows again after each failing restart, and is gone after the passing restart.

## 140. Fix agent for a failed migration

- **Scenario:** `restart-projects` with the failed start of `broken` from flow 139; one reload allowed (the restart).
- **Steps:** click "Fix agent" on the strip; Copy; Start; open the Agents tab; in a second page click Fix agent and Start again.
- **Pass:**
  - The dialog shows "The migration of project broken failed with:" and the error; Copy reads "Copied" and the clipboard holds the prompt.
  - A session of project `broken` exists, its dir is `broken`'s trunk checkout, its tier is `strongest` and the launch used `--model opus --effort high`.
  - The Agents tab lists the session.
  - The second start shows the toast "an agent is already fixing broken" and adds no session.

## 141. Restart server, tab reloads

- **Scenario:** `restart-projects`, `start` = `up`, `gx` = the second build (version `fixture-next`); one reload allowed.
- **Steps:** open the dialog; click Restart.
- **Pass:** the dialog reads "Restarting..."; the tab reloads by itself; the menu build text now contains `fixture-next` (it did not before).

## 179. Restart from the web applies pending steps

- **Scenario:** as flow 141 (`gx` = the second build); one reload allowed.
- **Steps:** read `demo`'s list of applied steps; empty it (and `broken`'s) so every step is pending; open the dialog; click Restart; wait for the reload.
- **Pass:** `demo`'s list holds the same names as before (the restarted server applied the pending steps).

## 142. Restart server, other tab

- **Scenario:** as flow 141, plus an agents session in `demo`; two reloads allowed.
- **Steps:** tab A opens the session and types "unsent words" into the composer; tab B restarts the server.
- **Pass:** tab A shows a "New version, reload" strip with a Reload button and keeps "unsent words"; Reload makes the strip go.

## 143. Restart server, server does not return

- **Scenario:** `restart-projects`, `start` unset (the fake refuses, so no new server comes up); one reload allowed; the test timeout is 180 s.
- **Steps:** open the dialog; click Restart; wait for the 60 s timeout; set `start` = `up` and run `gx server restart --continue` by hand.
- **Pass:**
  - After 60 s the dialog says "The server did not return. It may have failed to start." and names the restart log and `gx server start`.
  - The log path shown equals `restart_log` in the check and restart responses.
  - After the hand start the tab reloads by itself.

## 144. Restart server, binary changed

- **Scenario:** `restart-projects`, `start` = `up`; no reload allowed.
- **Steps:** open the dialog; rewrite `check.json` with another version; click Restart.
- **Pass:** the dialog shows "Next version: fixture-newer", does not read "Restarting..." and keeps its Restart button; the pid is unchanged.

## 145. Restart server, two tabs

- **Scenario:** `restart-projects`, `start` unset; three pages with an open dialog; no reload allowed.
- **Steps:** page A clicks Restart; page B clicks Restart right after; the server is stopped; page C clicks Restart.
- **Pass:** B and C both read "Restarting...".

## 146. Restart dialog, launchd binary

- **Scenario:** `restart-launchd`, `check.json` version `launchd-version`.
- **Steps:** open the dialog.
- **Pass:** it reads "Next version: launchd-version" and Restart is enabled.

## 147. Resume on worker

- **Scenario:** `model-mismatch`, one ticket `demo:mismatch-epic/01` paused as `model-mismatch` (ran on `claude-opus-4`, expected `claude-sonnet-4`); no reload allowed.
- **Steps:** open the ticket page; click Resume on worker.
- **Pass:**
  - The page reads "Wrong model" and "Ran on claude-opus-4, expected claude-sonnet-4."
  - Resume on worker is the only recovery button.
  - After the click the pause label is gone.

## 148. Long-running badge

- **Scenario:** `long-running`: ticket 01 carries the long-ticket mark (120 turns), ticket 02 does not.
- **Steps:** open the epic page; open ticket 01; open ticket 02.
- **Pass:**
  - The epic page shows one "Long-running" badge; its tooltip reads "120 assistant turns (threshold N)".
  - Ticket 01's page shows the badge; ticket 02's page shows none.

## 161. Server migrates on start

- **Scenario:** `migrate-pending` (the real server, no flag, a ticket in the old shape in `demo`).
- **Steps:** wait until the handshake stops saying migrating. Read the ticket and `gx tickets show`. Read `demo`'s list file. Kill the bare server and start it again.
- **Pass:** the ticket is in the new shape (`status: paused`). The list has step names, one per line, no repeat. After the second start the server log holds no new migration note and the list is unchanged.

## 163. Per-project list

- **Scenario:** `migrate-pending`, plus a project `other` written into the store with an old-shape ticket and no list (`demo` already has the full list).
- **Steps:** kill the bare server and start it again.
- **Pass:** `other`'s ticket is migrated. Both lists hold the same full set of names, and `demo`'s list is unchanged.

## 168. Crash restart and login start

- **Scenario:** `migrate-pending`, started as a bare `gx server` process (no `start` or `restart` command code, as launchd does).
- **Steps:** wait for the first migration. Add a notes step through inject-step. Kill the process and start it again the same way.
- **Pass:** the first start migrated the ticket. After the second start every list is the earlier list plus the added name, and the server log holds that step's note once per project and no other new note.

## 169. Migrating state holds scheduling

- **Scenario:** `migrate-held` (hold file in the migrate dir; workers held; a ready ticket `demo:ready-epic/01` queued; an old-shape ticket in `old-epic`; `stuck-epic` with its only ticket paused and queued, which is a deadlock once claimed; `merge-epic` with ticket `01` done and `02` queued, whose finish merges the epic).
- **Steps:** read `gx server status --json` and the page. Run `gx server status` and `gx server start`. Create a manual agent with `gx agents create`. Wait 2 s and read the ready ticket, `merge-epic/02`, `stuck-epic`, the server log and the old ticket. Remove the hold file. Then release the workers.
- **Pass:**
  - Status says migrating; the page shows the plain "Migrating tickets" line.
  - `gx server start` exits 0 within 10 s. The manual agent is created and listed although the migration is held.
  - The ready ticket and `merge-epic/02` are not claimed, `stuck-epic` gets no recovery ticket (`02`), the log holds no start-up step and the old ticket is not migrated.
  - After the release the line goes, the log holds `first-rescan`, `recover-merges` and `reclaim-runs` once each in that order, the old ticket is `paused` and the ready ticket is claimed.
  - After the release `stuck-epic` gets its recovery ticket `02` and `merge-epic/02` is claimed. The manual agent is still listed.
  - When the workers are released `merge-epic` is merged, and the manual agent is still listed.

## 170. A failed project is rescanned after the migration

- **Scenario:** `migrate-broken` (project `broken` fails its migration).
- **Steps:** wait until the handshake stops saying migrating. Read the server log and a ticket of `broken`.
- **Pass:** the log holds the three start-up steps once each, and `broken`'s ticket shows (it was scanned although its migration failed).

## 171. Writes wait for the migration

- **Scenario:** `migrate-held`.
- **Steps:** create and open an epic in project `scratch`. Read the old ticket with `gx tickets show`. Start five writes to `demo` without waiting: answer, ask (a log append), epic setting, move and epic creation. After 2.5 s check each, then remove the hold file.
- **Pass:**
  - The `scratch` writes return at once; the read returns the ticket.
  - No `demo` write ends during the hold. The CLI-side writes (epic setting, move, epic creation) print "waiting for the migration of demo"; answer and ask wait in the server and print nothing.
  - After the release every write ends, the old ticket is `paused` and the new epic exists.
- **Not covered:** the write during Retry migration, and a migration with the pause unit (no hook to hold either one).

## 172. A failed step is not recorded, skip notes stay in the log

- **Scenario:** `migrate-pending`.
- **Steps:** wait for the first migration. Inject a passing step and a failing step, restart the bare server. Then inject a step with a skip note and restart again.
- **Pass:**
  - The list holds the passing step and not the failing one; the earlier names are unchanged. The strip of `demo` shows the failure text.
  - After the notes step the list holds its name, the server log holds the note and no strip shows.

## 173. Failed start and Migration block

- **Scenario:** `migrate-broken`.
- **Steps:** wait for the migration to end. Read `gx server status --json`, both projects' tickets and the page.
- **Pass:**
  - The server is running. `demo`'s ticket is claimed; `broken`'s ready ticket is not.
  - The strip reads "broken: Migration blocked" with the error, the `gx tickets migrate` command, "Retry migration" and "Fix agent". It does not say "Paused" or "Missing trunk".

## 174. Retry migration button ends the block

- **Scenario:** `migrate-broken`.
- **Steps:** fix `broken`'s bad ticket on disk. Click "Retry migration". Then inject a failing step and restart the bare server.
- **Pass:** the strip goes with the server pid unchanged and `broken`'s ticket is claimed. After the failing start the strip is back with the new error.

## 175. The retry command by hand ends the block

- **Scenario:** `migrate-broken`.
- **Steps:** fix the ticket. Run the strip's `gx tickets migrate '<dir>'` in a shell.
- **Pass:** the strip goes with the pid unchanged and `broken`'s ticket is claimed.

## 176. The Fix agent route ends the block

- **Scenario:** `migrate-broken`.
- **Steps:** fix the ticket. Click "Fix agent", then "Start gx agent". Run the retry command the way the session would (the fake agent runs none).
- **Pass:** the dialog says "Agent started."; the strip goes with the pid unchanged and `broken`'s ticket is claimed.

## 177. A stale failure record is cleared by the start-up run

- **Scenario:** `migrate-broken`.
- **Steps:** with the strip showing, fix the ticket and restart the bare server.
- **Pass:** the start-up run passes and the strip goes.
- **Not covered:** a click on Retry while a retry runs (no hook to hold a retry; server tests cover the refusal).

## 178. Restart dialog with and without pending steps

- **Scenario:** `restart-projects` (project `broken` has pending steps and fails its dry run).
- **Steps:** open the dialog; close it; rewrite `check.json` with nothing pending and every project ok; open it again.
- **Pass:**
  - With a pending step the alert names "broken: fixture migration failed" and the button reads "Restart anyway".
  - With nothing pending the check answers at once, there is no alert and the button reads "Restart".
  - The line "A migration runs while the agents work." shows only when an agent runs and some project has pending steps (flow 137).
## 147. Command menu opens and filters

- **Scenario:** `agents-commands`, an idle Claude session at 1280px.
- **Steps:**
  1. Type `/` in the Message box.
  2. Type `/co`.
  3. Type `/co`, press Escape; type `/co ` (with a space).
  4. Type `/zzq`.
  5. Type `hello /co`.
- **Pass:**
  - `/` opens the menu. A row shows name, hint, description and source (`clear`, `[name]`, "Clear the conversation", `built-in`), and the footer reads "↑↓ select", "Enter fills", "Cmd/Ctrl+Enter sends" and "Esc closes".
  - `/co` lists the prefix matches (including `compact` and `cost`) first, then the substring matches, alphabetical in each group, with `co` highlighted.
  - Escape and a space close the menu; Escape keeps the text.
  - `/zzq` keeps the menu open with "No command matches /zzq. Cmd/Ctrl+Enter sends it as typed." and no rows; the text stays.
  - `hello /co` opens no menu.

## 148. Command menu keys

- **Scenario:** `agents-commands`, idle Claude sessions at 1280px (a sent slash command keeps its session busy, so the send steps use different sessions) and the Exited session.
- **Steps:**
  1. Type `/comp`; press Down, Up, Down, Enter. Type `/cost`; press Tab.
  2. Type `/mo` and click the "Switch model" row.
  3. Type `/zzq` and press Enter.
  4. Type `/model fast` (menu closed) and press Cmd/Ctrl+Enter. In another session type `/cost` (menu open) and press Cmd/Ctrl+Enter.
  5. Open the Exited session and type `/`.
  6. Turn vi mode on, open the large editor (Cmd/Ctrl+G) in an idle session, type `/co`, press Escape twice, press `0`, then `i`.
- **Pass:**
  - Down and Up move the selected row. Enter fills `/autocompact `, Tab fills `/cost `; neither sends (`gx agents chat` shows no new message).
  - The click fills `/model ` and the box keeps focus.
  - Enter with no match adds a newline and sends nothing.
  - Cmd/Ctrl+Enter posts the typed text, with the menu closed and with it open; the box empties.
  - The Exited session shows no menu.
  - In vi mode, typing in INSERT opens the menu; in NORMAL the menu is closed and `0` opens nothing; `i` returns to INSERT.

## 149. Command list freshness and failure

- **Scenario:** `agents-commands-slow`, `agents-commands-failed`, and `agents-commands` started with `--command-list-age 1s`; an idle Claude session at 1280px.
- **Steps:**
  1. On `agents-commands-slow`, type `/` on the first open.
  2. On `agents-commands-failed`, type `/co`, then type `/cost` and press Cmd/Ctrl+Enter.
  3. On `agents-commands`, type `/`. Write a new skill into the session's directory, switch to another session, wait past the server age, move the page clock on 61 seconds, hold the commands request, return to the first session and type `/`. Release the request.
- **Pass:**
  - The first open shows "Loading commands…" and no rows.
  - The failed list shows the built-ins (`/compact`, `/cost`) and "Skills could not load"; `/cost` still posts and the box empties.
  - A description with `<template>` shows as plain text; the `deploy-notes` skill (disable-model-invocation) is listed.
  - On the reopen the cached rows show at once without the new skill; after the request ends the new skill shows and the old rows are replaced.

## 150. Codex command menu

- **Scenario:** `agents-commands`, an idle Codex session and an idle Claude session at 1280px.
- **Steps:**
  1. In the Codex session type `/`.
  2. Type `$review-s` and press Enter.
  3. Type `use $htm` and press Tab.
  4. In the Claude session type `$re`.
  5. Back in the Codex session type `/gx-triage`.
- **Pass:**
  - `/` lists built-ins (`/status`) and no skills.
  - `$review-s` lists `$review-skill` once; Enter fills `$review-skill `.
  - `use $htm` lists `$html-skill`; Tab fills `use $html-skill ` and closes the menu.
  - `$re` in the Claude session opens no menu and no "No command matches" line.
  - The `/gx-` draft shows "Codex skills start with $. Type $gx-... instead."

## 151. Command menu on the design page and First prompt

- **Scenario:** `agents-commands` at 1280px: the Claude design page `demo:design-auto`, the Codex design page `gx:design-codex`, and the New session form.
- **Steps:**
  1. On the Claude design page open Note and type `/co`, then `/`.
  2. On the Codex design page open Note and type `/`, then `$html`.
  3. In the New session form type `/` in First prompt, choose the `feature` worktree and type `/` again.
  4. Type `/feature-o` and press Enter.
  5. Choose "+ Create worktree" and type `/`.
- **Pass:**
  - The Claude design menu lists `/compact`, `/cost` and `/html-skill`.
  - The Codex design menu lists `/status` and no skill for `/`; `$html` lists `$html-skill`.
  - First prompt lists `/feature-only-skill` for the `feature` worktree only; Enter fills `/feature-only-skill ` and the form stays open.
  - "+ Create worktree" lists the base worktree's commands, without `/feature-only-skill`.

## 152. Phone, command menu

- **Scenario:** `agents-commands` at 390px on the design page `demo:design-auto` and the New session sheet.
- **Steps:**
  1. Open Note and type `/`.
  2. Type `/mo` and tap "Switch model".
  3. Shrink the viewport to 390x420 and type `/`.
  4. Open the New session sheet, type `/` in First prompt and tap the first row.
- **Pass:**
  - Rows take two lines, stay inside 390px and the menu has no footer.
  - The tap fills `/model ` and closes the menu.
  - At 420px high the menu sits inside the viewport.
  - First prompt's menu opens below the field; a tap fills `/name `.

## 153. Send a command

- **Scenario:** `agents-commands` at 1280px. File `agents-commands-send.e2e.ts`; the fake claude's input log is read with `fixture.input`, and `fixture.controlClaude` arms its command output.
- **Steps:**
  1. In an Idle Claude session arm the command output, type `/cost` and press Send. Leave the session and come back (a command that starts no turn changes no status, so the open chat gets no event).
  2. In another Idle Claude session send `/compact foo`.
  3. In the Idle Codex session send `$review-skill`.
  4. Open an Idle session, make its chat read fail, move the chat with a CLI prompt, then send `/clear`. Send `!ls`.
  5. In the New session form send `!ls` as First prompt, then `/cost`.
  6. In the session that holds `/cost` output send `/clear` and type a draft.
  7. Open the Working session and type in the box.
- **Pass:**
  - `/cost` reaches the fake once, shows the pill and the output, and raises no "Send failed".
  - `/compact foo` reaches the fake once and the session goes Working.
  - The Codex prompt `$review-skill` shows in the chat.
  - The stale `/clear` reads "the conversation moved; review it first", stays in the box and never reaches the fake.
  - `!ls` reads "text may not start with !" and stays in the box; First prompt reads "the first prompt may not start with !", keeps it and creates nothing; `/cost` as First prompt creates the session.
  - After `/clear` the old output is gone and the draft stays.
  - The Working session shows "Working. Send turns on when the session is idle." and Send is off.

## 154. Commands in a design session

- **Scenario:** `agents-commands` with `--auto-advance-delay 1h`, design page `demo:design-auto`. A CLI prompt ends the design agent's run, which starts the countdown.
- **Steps:**
  1. With the countdown running send `/clear`, then (after a new countdown) `/compact` from Note.
  2. With the countdown running send a key, then typed text, through the keys route.
  3. Block the session at a dialog and click Close dialog; at a Bash permission and click Deny, then Allow once.
  4. Turn Auto-advance off, end a run and press Continue.
- **Pass:**
  - Each command moves the deadline on and leaves the run-end Continue raised (the server test `DesignSlashCommandStopsTheCountdown` pins this); the page's "Continuing in" line goes away for `/clear`.
  - A key and typed text each stop the countdown and clear the Continue.
  - Close dialog, Deny and Allow once leave the same deadline and "Continuing in" stays.
  - With the countdown off no countdown shows and Continue still works.

## 155. Command output in the chat

- **Scenario:** `agents-commands` at 1280px (the design page with `--auto-advance-delay 1h`). File `agents-commands-output.e2e.ts`; `fixture.controlClaude` arms the output (ANSI codes, 15 lines), `fixture.input` is the fake's input log.
- **Steps:**
  1. In an Idle Claude session arm a 15-line output with ANSI codes and send `/cost`. Leave and come back.
  2. Click "Show all (15 lines)".
  3. In two other Idle Claude sessions send `/model` (empty output) and `/compact` (the "Compacted" line).
  4. In the Idle Codex session send `/status`.
  5. On the design page `demo:design-auto`, after a run ends, send `/cost` with a 14-line output from Note. Leave and come back, then Show all.
- **Pass:**
  - The `/cost` pill shows with a muted monospace block cut at 12 lines; "Show all" expands to every line.
  - No ANSI code and no caveat line show; the fake's input log holds no output text; the session stays read (no actionable reason).
  - `/model`, `/compact` and the Codex `/status` show the pill alone, with no output block.
  - The design page shows the same block, cut at 12 lines, expanded by Show all.

## 156. Key bar and Type field

- **Scenario:** `agents-failures` (the stalled session) and `agents-commands` at 1280px. File `agents-keys.e2e.ts`; the fake claude's input log is `fixture.input`.
- **Steps:**
  1. On the stalled session open Screen from the notice. Type `hello`, then press each key of the bar in order, Ctrl+C last.
  2. Type `!x`.
  3. Through `gx agents keys` send keys and text to a Working, an Idle, a Blocked and a Rate-limited Claude session.
  4. Send a key to the Exited session, `!x` and text with a newline to the Idle one, and a key to the Queue agent. Open the Queue agent's page.
  5. Open the Codex session that is Blocked on an error: read the notice and composer, then in Screen Type `go on` and press Enter.
- **Pass:**
  - Type reaches the fake as `hello` with no Enter; each key reaches it as its own sequence, and every send reads the screen again.
  - `!x` shows "text may not start with !" and reaches nothing.
  - Down, Up and typed text reach the session in all four states.
  - The Exited session refuses with "is exited"; `!x` and the newline are refused; the Queue agent is refused (`session-read-only`) and its page has no Screen button or Type field.
  - The error-blocked notice has Screen only and Send stays off ("Blocked."); Type plus Enter leaves Blocked.
- **Reach:** only a Blocked or stalled session's notice opens Screen on the Agents page, so the other states are driven through the CLI route the buttons call.

## 157. Close dialog

- **Scenario:** `agents-commands` at 1280px (the design page with `--auto-advance-delay 1h`). File `agents-permission.e2e.ts`; `fixture.controlClaude` blocks the session at a question dialog, `fixture.input` is the fake's input log. The test stops the page's `/v1/snapshot` reads after it opened, so the notice outlives the Blocked state.
- **Steps:**
  1. Open an Idle Claude session blocked at a dialog; click Close dialog.
  2. Click Close dialog again on the same notice.
  3. Repeat on the design page `demo:design-auto`.
- **Pass:**
  - The first click sends Esc alone and the session turns Idle.
  - The second click sends nothing and the notice shows a refusal.
  - The design page does the same.

## 158. Allow once and Deny

- **Scenario:** `agents-commands` at 1280px (the design page with `--auto-advance-delay 1h`). File `agents-permission.e2e.ts`; the seeded Blocked sessions cover a Claude prompt gx cannot read, one whose first option is "Yes, and don't ask again", one that ignores Enter, and three Codex ones (hook, screen only, unreadable screen). `fixture.controlClaude` blocks Idle Claude sessions at a readable Bash prompt; `fixture.input` and `fixture.codexDecisions` are the fakes' logs.
- **Steps:**
  1. Block two Idle Claude sessions at a Bash prompt; read the notice, press Allow once on one and Deny on the other.
  2. On the unreadable Claude prompt press Allow once, then Deny.
  3. On the "don't ask again" prompt press Allow once.
  4. On a readable prompt press Down in the Screen key bar, redraw the dialog with the cursor on option 2, close Screen and press Allow once.
  5. On the session that ignores Enter press Allow once.
  6. On the Codex hook session press Allow once. On the Codex screen-only session press Deny. On the Codex unreadable session press Allow once, then Deny.
  7. On the design page block the design session at a readable prompt and press Allow once; then at an unreadable one and press Allow once, then Deny.
- **Pass:**
  - The notice shows the prompt text with Screen, Deny and Allow once; Allow once sends Enter alone, Deny sends Esc alone.
  - An unreadable Claude prompt shows no text; Allow once reads "This request needs a real answer. Use the key bar in Screen." and sends nothing; Deny works.
  - The "don't ask again" prompt and the cursor on option 2 give the same refusal and the fake reads no Enter.
  - The ignored Enter reads "The answer did not reach the session. Check Screen." after the server's 45s grace, and the session stays Blocked.
  - Codex: the hook decision is `allow`; screen-only Deny leaves Blocked; on the unreadable screen Allow once is refused and Deny leaves Blocked.
  - The design page shows the same text, buttons and refusals.
- **Reach:** the fake claude draws no cursor, so step 4 arms the redrawn dialog by hand after Down has reached the fake.

## 159. Exit with /exit

- **Scenario:** `agents-commands` at 1280px (the design page with `--auto-advance-delay 1h`). File `agents-commands-output.e2e.ts`.
- **Steps:**
  1. In an Idle Claude session send `/exit`.
  2. On the design page `demo:design-auto`, after a run ends, send `/exit` from Note.
- **Pass:**
  - The Agents session turns Exited, keeps its chat, and shows "Exited. Restore it in gx, Agents tab." with Send off.
  - The design page keeps its chat and shows Restore and New session.

## 160. Phone, key input and Blocked notices

- **Scenario:** `agents-commands` at 390px. File `agents-keys.e2e.ts`; `fixture.controlClaude` blocks an Idle session at a dialog with a long row, then another at a Bash permission.
- **Steps:**
  1. Open the dialog session: read the notice, then Screen.
  2. Close Screen and open the permission session.
- **Pass:**
  - The page never scrolls sideways; the notices and their buttons (Screen, Close dialog / Deny, Allow once) stay inside 390px.
  - The screen rows scroll sideways.
  - The eight key buttons and Type are at least 44px tall, stay inside 390px and wrap onto more than one line.
