# Epic dependency UX

## Problem Statement

An epic can wait on another epic. Its `blocked_by` names an unmerged epic. gx shows that epic as stuck, and it shows it three different ways:

- The Epic page says "Paused".
- The Epics page card says "Deadlocked".
- The sidebar says "Needs you".

None of this is true. The epic is waiting, and it will start by itself once the blocker merges. The scheduler also treats it as a deadlock: it logs a Deadlocked event and starts a recovery agent for work that is not broken. A person who wants to add or remove such a dependency has only the CLI (`gx tickets epic set --blocked-by`). The local tracker doc does not describe epic-level `blocked_by` either.

## Solution

The server gets one new epic state, **Waiting** (waiting on another epic). Every surface reads it: the Epic page, the Epics page card, the sidebar, the TUI Queue tab, the Needs you counts and the scheduler. The state names the blocker epics. It is never Paused, Deadlocked or Needs you.

An epic is Waiting only when it has an unreleased blocker epic (its own epic-level `blocked_by`), nothing is running, no ticket is Paused and there is no real cycle inside the epic.

This effort also removes cross-epic ticket blockers. A ticket's `blocked_by` names only tickets of its own epic, and an epic's `blocked_by` names only whole epics. Waiting then has one source and the state rule stays simple.

The Epic page gets a "Blocked by" row of chips under the header (one chip per unreleased own `blocked_by` entry), with a "+ Blocker" picker. It adds and removes blockers and writes the same field as `gx tickets epic set --blocked-by`. A server refusal (a cycle) shows inside the picker.

## User Stories

1. As a person with an epic blocked by an unmerged epic, I want the Epic page header to say "Waiting on <blocker>", so that I know the epic is not stuck.
2. As a person, I want the Epics page card of that epic to say the same words, so that the card and the page agree.
3. As a person, I want the sidebar row to say "Waiting", so that the epic does not look like it needs me.
4. As a person, I want the sidebar row tooltip to name the blocker, so that I can see what the epic waits on without opening it.
5. As a person, I want the TUI Queue tab to say "Waiting on <blocker>", so that the terminal agrees with the web.
6. As a person, I want the epic to never read Paused, Deadlocked or Needs you while it only waits, so that these words keep meaning "look at this".
7. As a person with several blockers, I want the header and the card to read "Waiting on A and 1 more", so that the line stays short.
8. As a TUI user with several blockers, I want "Waiting on A +1 more", so that the narrow row keeps the TUI's own "+N more" form.
9. As a person, I want Waiting to come only from an unmerged epic-level blocker, so that there is one kind of cross-epic dependency and one rule to learn.
10. As a person, I want the blocker named by its bare epic name, so that the line is short.
11. As a person, I want to click the blocker name on the Epic page and land on that epic, so that I can check on it.
12. As a person, I want a dim hint line, "<blocker> is not queued. This epic stays put until it is merged.", when a blocker is not queued, so that I know why nothing moves. The hint always uses this form with the blocker's own state label.
13. As a person, I want the hint to say "<blocker> is <its own state label>. This epic stays put until it is merged." (for example "is Paused", "is Deadlocked", "is Merge blocked") in the Paused text tone when a blocker is stuck, so that I know to go and fix the blocker.
14. As a person, I want no hint while every blocker is moving, so that the page stays quiet.
15. As a TUI user, I want the suffix "(not queued)" or the blocker's own state label, such as "(Paused)" or "(Merge blocked)", on the status line only when the blocker is not moving, so that the row stays one line.
16. As a person, I want a waiting epic never to appear in Needs you, even when its blocker is stuck, so that the Needs you list is only things I must do. The stuck blocker shows in Needs you on its own.
17. As a person, I want a queued waiting epic to sit in Implementing and an unqueued one in Ready to queue, so that the Epics page has no new section to learn.
18. As a person, I want a queued waiting epic to keep its queue place and use no agent slot, so that it starts by itself once the blocker merges.
19. As a person, I want the scheduler to log no Deadlocked event and start no recovery agent for a waiting epic, so that no agent runs for nothing.
20. As a person, I want a Paused ticket in the epic to beat Waiting, so that something I must resume is never hidden.
21. As a person, I want a hand-written dependency cycle inside one epic to read Deadlocked, even while a blocker epic is unreleased, so that a gx bug or a bad manual edit is reported as what it is. A ticket whose only fault is a `blocked_by` cycle does not count as Paused. A real Paused ticket still beats it.
22. As a person, I want the epic to leave Waiting by itself when the blocker merges (or is done under the on-done policy), so that I do nothing to restart it.
23. As a person, I want Epic page ticket rows to show only in-epic blockers ("after NN") and leave out the epic-level blocker, so that the header and chips are the one place that names the epic that holds the whole epic.
24. As a person, I want a "Blocked by" chip row on the Epic page, one chip for every unreleased own `blocked_by` entry (whole epics only), each showing the blocker name as a link and a × to remove it, so that I can see and change the dependency in one place.
25. As a person, I want a "+ Blocker" button that opens a picker with a search field, so that I can find an epic by name.
26. As a person, I want the picker to list the project's loop epics, minus this epic and archived and merged ones, so that I can only pick a valid blocker.
27. As a person, I want each picker row to show the epic's state ("Implementing", "Ready to queue"), so that I pick the right one.
28. As a person, I want a click on a picker row to toggle that blocker, and this epic shown greyed as "this epic", so that I can add and remove from one list.
29. As a person, I want to remove a blocker with the chip's ×, so that I can clear one of several.
30. As a person, I want the control to handle several blockers, so that an epic can wait on more than one.
31. As a person, I want the server's cycle refusal shown inside the open picker as "Can't add <blocker>: it already waits on <this epic>. That would be a cycle.", with nothing written, so that I understand why the add failed.
32. As a person, I want the web control to write the same field, with the same checks, as `gx tickets epic set --blocked-by`, so that web and CLI never disagree.
33. As a person, I want a blocker added in the web to show in `gx tickets epic set`'s field and the other way round, and the page to follow without a reload, so that the two stay in step.
34. As a person, I want the Epics page card to show no blocker control, so that the card stays simple and the control lives on the Epic page.
35. As a person, I want the web change to keep the other `blocked_by` entries of the epic (released epics, which show no chip) when I add or remove one blocker, so that an edit never drops a dependency I cannot see.
36. As a contributor reading `gx-local-tracker.md`, I want epic-level `blocked_by` documented, so that I know the field exists, how to set it and when the epic starts.
37. As a contributor reading `GLOSSARY.md`, I want a Waiting term, and Deadlocked no longer covering this case, so that the words match the product.
38. As a person, I want a ticket's `blocked_by` to name only tickets of its own epic, so that an epic's dependencies live in one place, the epic-level `blocked_by`.
39. As a person, I want `gx tickets validate` to refuse a ticket `blocked_by` that names a ticket of another epic (such as `other/06`), with a message that points to the epic-level `--blocked-by`, so that I know what to do instead.
40. As a person, I want the ticket write commands to refuse the same thing with the same message, so that I cannot create what validate would refuse.
41. As a person, I want an epic's `blocked_by` to name only whole epics, and a ticket ref there refused with the same pointer, so that the field means one thing.
42. As a person, I want no data migration and no broken ticket on upgrade. Two cancelled tickets (`follow-ups-archived` 144 and 146) carry old cross-epic refs; terminal tickets are never refused, so they keep loading.
43. As a contributor reading `skills/gx-local-tracker.md`, I want the qualified `blocked_by: [epic/06]` text and the cross-epic cycle text gone from it, so that the doc matches the tracker. The `epic/NN` ticket address stays.
44. As a person, I want a stored epic `blocked_by` entry that the new check refuses (a ticket ref, a deleted epic) to show as an error chip with a ×, so that I can clear it and a web add or remove never fails on an entry I cannot see.

## Implementation Decisions

**Server state**

- A new epic state in `server/epics/state.go` (`StateWaiting`, name is the implementer's choice) sits beside Done, Runnable, Paused and Cycle. `StateOf` stays the one rule; the deadlock check and the epic summary both derive from it. No `EpicState` is stored: the state is derived on every read.
- The epic is Waiting when it has at least one unreleased own blocker epic, no ticket is running, no ticket is Paused and there is no cycle among its remaining tickets. "Unreleased" is the hold's own predicate (`g.blocking` with `EpicRelease.stamped`: merged, or done under the on-done policy, or auto-ff-merged), not `MergedAt` alone. There is no ticket-level walk, no parent-to-child walk and no "held from outside" logic: with cross-epic ticket blockers removed, the only outside hold is the epic-level `blocked_by`.
- Precedence: Done (no open tickets) first. Then a running ticket. Then a Paused ticket beats Waiting. Then a cycle (Cycle) beats Waiting, so a cycle reads Deadlocked even while a blocker epic is unreleased. Then Waiting. Then Runnable.
- The release rule is unchanged: the blocker epic merges to trunk, or is done under the on-done policy.
- Cycle is made reachable (correct fix). Today `cycleErrors` marks a cycle ticket as `error`, which counts as Paused, so `StateCycle` never fires. A ticket whose only error is a `blocked_by` cycle gets its own rendered status that is not Paused, and `StateOf` maps an epic whose remaining tickets are only such cycle tickets to Cycle (Deadlocked). A real Paused ticket (answer, repair, other error, draft) still wins. A new seam-1 test covers a hand-written two-ticket cycle.
- The epic-level hold keeps one project-wide resolver. Today `tickets/blockedby.go` copies the epic's own `blocked_by` into every ticket's `ExternalBlockers`, and `UnresolvedBlockers`, the `LoadResolved` callers and `EpicsWaitedOn` read it. That resolver stays, renamed to say what it is (the epic-level hold), and keeps applying the release rule. Only the ticket-level qualified refs and the widening go.
- The blocker list (the epic's unreleased own `blocked_by` entries) is derived in one place from the same predicate as the hold (R4) and is the source for every surface. Its chain walk keeps a visited set, so a hand-edited epic cycle ends the walk instead of looping. It replaces the `blocked_by_epics` field on the epic list row, so the two never disagree.

**Wire**

- A new epic state value with its own wire value. It must not be `waiting`, which means "Waiting for a slot" and stays as is. The implementer picks the name (the map suggests `waiting-on-epic`).
- The epic summary, the epic detail and the epic list row carry one blocker list: for each unmerged own `blocked_by` entry its address, name and its Epics page state (not a yes/no moving flag). Every entry is a whole epic. When a blocker is not moving, the entry also carries the epic the hint names (see below) with that epic's state. The implementer picks the field names. `EpicDetail` gains the list because the Epic page header and the chip row read it. The waiting-on set and the own-blocker list are the same list, because ticket-level blockers are gone; the spec keeps one field, not two.
- Moving: a blocker is not moving when its Epics page state is Ready to queue, Draft or No open tickets (state label in the hint: "not queued"), or Paused, Deadlocked, Merge blocked, CI failing or PR text failed (the hint uses that state's own label). Implementing, running, Awaiting merge, Awaiting PR and PR open are moving. A blocker that is itself Waiting is not moving when one of its own blockers is not moving, followed down the chain (cycles are refused, so the walk ends). The hint then names the deepest not-moving epic, with that epic's own state label.
- Order: the server sorts the blocker list with invalid stored entries first, then not-moving, then by name. Every surface (header, card, sidebar tooltip, TUI) takes the first entry as "A". An invalid entry has no name, so it is named by its stored text. The hint reads the first valid entry.
- The epic list state for a Waiting epic maps like slot-waiting: Implementing when queued, Ready to queue when not. It never maps to Paused, so it never reaches Needs you. The Needs you rule keeps `PillDeadlocked` for a real Cycle only.
- Migration, one change, in this order: remove `blocked_by_epics` from the Go types and the OpenAPI source, add the new state value and the blocker list, regenerate `generated/server.ts`, remove `external_blockers` and `widened_blockers` from the index wire (`server/wire/index.go`, `server/index/index.go`, `ui/tickets/tabapi/viewmodel.go`, `generated/server.ts`) and drop the "(via …)" form of `Ticket.BlockerLabel` (`cmd/tickets/tickets_show.go`), update the fixtures that carry `blocked_by_epics`, update `docs/specs/web-epics-page.md` (which documents the field), then update every reader. Nothing is stored, so no data migration.
- The new state value goes into the wire enum, the TUI label map and the web `LABELS` in the same change. A test fails when a wire epic state has no label in the TUI map or in `LABELS` (iterate the generated state list).

**Scheduler**

- `deadlockKind` and the claim-time check ignore a Waiting epic: no Deadlocked event, no recovery, and any earlier deadlock entry for the root is forgotten. A queued Waiting epic keeps its queue place and takes no agent slot, and it starts when the blocker releases (already true; this stays covered by a test).

**Web write route**

- A new route like `POST /v1/epics/mark-merged`: it takes the epic address and a blocker address to add or remove, reads the epic's current `blocked_by` under the project lock, applies the one change, runs `CheckEpicBlockedByWrite` and then `UpdateEpicFields`, the same pair the CLI runs. It keeps entries it does not show (released epics). It checks only the entry it adds, so a stale stored entry never blocks an add or remove of another one. A refusal writes nothing. The cycle refusal is a coded error `blocked-by-cycle` that carries the blocker and the chain; it is not only text. Cross-project blockers stay refused by the check. A ticket ref in an epic's `blocked_by` is newly refused (the check accepts it today).
- The route is a web write, so it follows the same auth and event rules as the other epic routes. The event stream carries the change so open pages follow without a reload.

**Web**

- The Epic page header status word reads "Waiting on <blocker>" in the muted tone, never the amber Needs you tone (the theme's `waiting` tone is amber, so the Waiting word uses the muted tone explicitly). Two or more read "Waiting on A and 1 more". No separate pill.
- The hint is one dim line under the header, always "<blocker> is not queued / <blocker's own state label>. This epic stays put until it is merged." It uses the Paused text tone for a stuck state (Paused, Deadlocked, Merge blocked, CI failing, PR text failed), and the dim tone for "not queued". It is not red or amber. The blocker is the first valid entry of the sorted list (invalid stored entries are skipped), or the deepest not-moving epic when that blocker is itself Waiting. There is no ticket-blocker wording.
- The "Blocked by" chip row sits under the header, one chip for every unreleased own `blocked_by` entry, each with a ×. A stored entry the check refuses (a ticket ref, a deleted epic) shows as an error chip with a × and no link, so it can be cleared: the blocker name is a link with no underline, the × removes it, "+ Blocker" opens the picker. The picker is a search field over the project's loop epics, minus this epic and archived and merged ones, each row showing its state; a click toggles; this epic is shown greyed as "this epic". The cycle refusal shows inside the open picker: the web builds the mock's wording ("Can't add <blocker>: it already waits on <this epic>. That would be a cycle.") from the `blocked-by-cycle` error's blocker and chain, not from the server's text.
- The Epics page card status line shows the same words as the header. The sidebar row shows the bare word "Waiting" in the muted tone, with the full words in its tooltip. The card menu gets no blocker control. The jump palette trailing status follows `cardStatus`.
- The sidebar's `cardStatus` maps the new wire state to the bare word "Waiting" and builds the tooltip ("Waiting on A and 1 more") from the blocker list. The Epics page card reads the server's `status_words`. Neither derives a label from ticket blockers. The jump palette's trailing status follows `cardStatus`.
- Epic page ticket rows (`blockerNote`) show only in-epic blockers ("after NN"). The epic-level blocker no longer appears on rows, in the TUI rows (`ui/tickets/rowfmt/row_style.go`) or in the ralphloop schedule reason as a ticket blocker; the header names it. The map's ticket 03 resolution (rows unchanged) is superseded by this.

**TUI**

- The Queue tab epic header status line reads "Waiting on <blocker>"; several read "Waiting on A +1 more". The suffix "(not queued)" or the blocker's own state label, for example "(Paused)" or "(Merge blocked)", shows only when the blocker is not moving, in the Paused text tone for a stuck state. Plain style and the queued icon, never orange, red or the warning icon. A name that does not fit is cut with an ellipsis; the "+N more" part and the suffix are never cut. The TUI only reads the server state and derives no Waiting from ticket blockers itself.

**Removal of cross-epic ticket blockers**

- `tickets/blockedby.go` loses `resolveCrossEpic` and its tests, the cross-epic cycle code, qualified ticket-token parsing and widening (`WidenedBlockers`). The epic-level hold resolver stays (see Server state).
- `gx tickets validate` refuses a ticket `blocked_by` that names a ticket of another epic, and an epic `blocked_by` that names a ticket ref. Both refusals skip terminal tickets, as `validate` already does, and the load path (`checkBlockedBy`) skips them too, so a cancelled ticket with an old ref never renders `error`. The message says the dependency belongs in the epic-level `--blocked-by`, for example "a ticket may only be blocked by tickets in its own epic; to wait on <other>, run `gx tickets epic set <epic> --blocked-by <other>`".
- The write commands (ticket create and set with a blocker, `epic set --blocked-by`) run the same check and refuse with the same message, so no refused value can be written.
- No data migration. The only live cross-epic refs are on two cancelled tickets (`follow-ups-archived` 144 and 146), which stay as they are. If a non-terminal one appears before the change lands, `validate` names it and the person moves it to an epic-level blocker by hand. A test loads a terminal ticket with an old ref and expects no error.
- `skills/gx-local-tracker.md` stops documenting the qualified `blocked_by: [epic/06]` token and cross-epic cycles (the `epic/NN` ticket address stays). `skills/gx-investigate/gotchas.md` (the cross-epic `blocked_by` gotcha near line 282) is updated the same way.

**Docs**

- `skills/gx-local-tracker.md` documents epic-level `blocked_by`: the field in the epic frontmatter, `gx tickets epic set <project:epic> --blocked-by <project:other>` (whole-epic tokens, same project only), that the epic starts only after the blocker merges to trunk, and that an epic waiting on it shows as Waiting and never as Deadlocked. It also drops the qualified `epic/NN` token and the cross-epic cycle section, and says a ticket's `blocked_by` names only tickets of its own epic.
- `docs/specs/web-epics-page.md` replaces its `blocked_by_epics` description with the new blocker list.
- `GLOSSARY.md` gets a **Waiting** term (an epic state: held only by an unmerged blocker epic; distinct from "Waiting for a slot") and the **Deadlocked** and Paused-epic entries say they no longer cover it.

## Testing Decisions

- A good test checks external behavior: the state the server reports, the words on a page, the file the route writes. It does not check which internal function produced them.
- Seam 1, the server: `StateOf` and the epic summary and list for fixtures of epic-level blockers (Waiting, Paused beats Waiting, a real in-epic cycle stays Cycle even with an unmerged blocker, release on merge, a Waiting blocker that is not moving, blocker order, refusal of ticket refs across epics), the scheduler's deadlock check (no event, no recovery), and the new route (add, remove, keep other entries, `blocked-by-cycle` refusal writes nothing). A label-coverage test fails when a wire epic state has no TUI label or no web `LABELS` entry. Prior art: `server/epics/state_test.go`, `epicsummary_test.go`, `epiclist_state_test.go`, `epicneeds_test.go`, `server/sched` deadlock tests and the `mark-merged` route tests.
- Seam 2, the browser: one Playwright flow in the existing epic flows, driven by the new fixture scenario, covering the header, card, sidebar, chips, picker and refusal. Prior art: the Epic flows in `webui/e2e/CATALOG.md` (flow 2 and the sidebar flows).
- Component tests (Vitest) for the status words and the picker only where the flow would be slow; prior art `EpicPage.test.tsx`, `EpicCard.test.tsx`, `sidebarModel.test.ts`.
- TUI: the Queue header status-line test for the new state. Prior art: the `header` package tests.
- The Epic page, card, sidebar and TUI must not be tested for words that the server did not send. Tests for Waiting words set the server state, not the ticket graph, except in seam 1.

## Verification

Stories 1 to 8, 10 to 15, 23 (Waiting words on every surface; see Mock states):

- Flows to run: the Epic flows of `webui/e2e/CATALOG.md` (flow 2 "Epic looks right", flow 6 "Queue an epic") to confirm no regression of the other states.
- Checks: with the `epic-waiting` fixture, `gx server epics` lists the waiting epic with the new state and its blockers, and `gx server epics detail <project:epic>` carries the same; the epic's card is in Implementing when queued and Ready to queue when not; the sidebar tooltip names the blocker; the Epic page's ticket rows show no "after <blocker-epic>" text and still show in-epic "after NN" (story 23). Clicking a chip name on the Epic page lands on the blocker's Epic page with no full page reload (story 11).
- New flow to write: "Waiting epic" (below). New scenario: `epic-waiting`.
- The TUI line (stories 5, 8, 15): a header status-line test for each of one blocker, two blockers, not queued and Paused; and, with the fixture, `gx server epics detail` shows the same state the TUI reads.

Stories 6, 16 to 22 (state and scheduler):

- Extra seam-1 checks: an on-done blocker (released but not merged) leaves the blocker list and the Waiting state together; a hand-written epic cycle between two epics ends the chain walk.

- Checks: `gx server epics` shows no Paused, Deadlocked or Needs you pill for the waiting epic; the run log (`gx tickets` events for the epic) has no `deadlocked` event; no recovery session starts. A Paused ticket added to the epic turns it Paused. A two-ticket cycle inside the epic reads Deadlocked, even with the blocker unreleased; adding a Paused ticket turns it Paused. While the epic is Waiting and queued, `gx server queue` still lists it at its place and no session runs for it (story 18). After the blocker is marked merged (`gx server epics mark-merged` or its merge), the epic starts and, if queued, runs without a person acting.
- New flows or scenarios to write: seam-1 Go tests above, plus a scenario shape `release` where the blocker merges.

Stories 9, 38 to 43 (removal of cross-epic ticket blockers):

- Checks: a ticket with `blocked_by: [other/06]` is refused by `gx tickets validate` and by the write commands, with the message that points to `--blocked-by`; an epic `blocked_by` with a ticket ref is refused the same way; `gx tickets validate` over the live store is clean (the two cancelled tickets are skipped) and a terminal ticket with an old ref still loads without `error`; `rg "resolveCrossEpic|WidenedBlockers" --glob '*.go'` finds nothing while the epic-level hold resolver still holds a blocked epic's tickets; `rg "blocked_by: \[[a-z-]+/[0-9]+\]|cross-epic" skills/gx-local-tracker.md skills/gx-investigate/gotchas.md` finds no qualified token or cross-epic cycle text.
- Go tests: the removed cross-epic tests are gone, and new tests cover the refusals.

Stories 24 to 31, 34 (the control):

- Flow: the new "Waiting epic" flow (which also clicks a chip name and lands on the blocker's Epic page with no reload) adds a blocker from the picker, sees the chip and the Waiting words with no reload, removes it with the chip's ×, adds two blockers, and triggers the cycle refusal. The card menu has no blocker control.
- Checks: after each action `gx tickets show <project:epic>` shows the expected `blocked_by`; after the refusal it is unchanged.
- A stale stored entry (a ticket ref) shows as an error chip with a ×, and removing it works (story 44). The picker (stories 26 to 28): lists the other loop epics with their states, shows this epic greyed as "this epic", and lists no archived or merged epic.

Stories 32, 33, 35 (parity with the CLI):

- Checks: `gx tickets epic set <project:epic> --blocked-by <other>` makes the open Epic page show the chip without a reload; a web add leaves a stored merged blocker in `blocked_by`; a cross-project blocker is still refused by the CLI.

Stories 36, 37 (docs):

- Checks: `rg "blocked_by" skills/gx-local-tracker.md` shows an epic-level section naming `gx tickets epic set`; `rg "Waiting" GLOSSARY.md` shows the new term and the Deadlocked entry no longer covers this case.

Mock states. Mock path: `$(gx tickets root)/gx/epic-dependency-ux/mocks/01-mock-waiting-label-and-blocker-control.html`. Every state is 1000 px wide and runs on the new `epic-waiting` fixture scenario (a new scenario to write, one shape per state, the implementer may rename it). The mock binds only the header, the chips, the picker, the card and the sidebar. Two points of the mock are not the fixture's truth: the `cycle-refusal` state names an already-ticked blocker, so its fixture uses an unticked epic that waits on this one; and the mock's ticket rows show a runnable ticket in a Waiting epic, so the fixture's ticket rows differ. Every state shows the parts `status-words`, `blocker-control`, `card-status-queued`, `card-status-unqueued` and `sidebar`.

- Mock states: `two-blockers`, width 1000, scenario `epic-waiting` shape `two-blockers`, parts: `status-words` ("Waiting on A and 1 more"), `blocker-control` (two chips), `card-status-queued`, `card-status-unqueued`, `sidebar` ("Waiting"). Stories 1 to 3, 7, 14 (no hint while every blocker is moving), 24, 30.
- Mock states: `blocker-not-queued`, width 1000, scenario `epic-waiting` shape `blocker-not-queued`, parts: `status-words`, `blocker-control`, `card-status-queued`, `card-status-unqueued`, `sidebar` (with the dim "is not queued" hint line under the header). Story 12.
- Mock states: `blocker-paused`, width 1000, scenario `epic-waiting` shape `blocker-paused`, parts: `status-words`, `blocker-control`, `card-status-queued`, `card-status-unqueued`, `sidebar` (with the hint line using the blocker's own state label, "is Paused", in the Paused tone). Story 13.
- Mock states: `picker-open`, width 1000, scenario `epic-waiting` shape `picker-open`, parts: `status-words`, `blocker-control` (picker open over the chip row), `card-status-queued`, `card-status-unqueued`, `sidebar`. Stories 25 to 29.
- Mock states: `cycle-refusal`, width 1000, scenario `epic-waiting` shape `cycle-refusal`, parts: `status-words`, `blocker-control` (the refusal text inside the open picker), `card-status-queued`, `card-status-unqueued`, `sidebar`. Story 31.

New flows or scenarios to write (these become implementation tickets): the `epic-waiting` scenario with the shapes above plus `release`; the removal of cross-epic ticket blockers (its Go tests and refusals); the "Waiting epic" Playwright flow; the Go tests of seam 1; the TUI header test.

## Out of Scope

- Cross-project blockers: `validate` refuses them today and this effort keeps that.
- Cross-epic ticket blockers are not kept in any form (see the removal stories). Lost with them: waiting on a commitless ticket in another epic, or on a map-epic ticket.
- A notification when the blocker merges and the waiting epic starts.
- A new Epics page section for waiting epics. They stay in Implementing or Ready to queue.
- A blocker control on the Epics page card menu, and the Epic page three-dots menu (the control is the chip row).
- A change to the wording of Epic page ticket rows ("after <epic>/NN").
- A change to the release rule (merge to trunk, or done under the on-done policy).

## Further Notes

- The theme's `waiting` tone is the Needs you amber. The Waiting state must not use it; it takes the muted tone. Name the tone in code so the clash is visible.
- `EpicStateWaiting` ("Waiting for a slot", wire value `waiting`) keeps its label. The TUI's label map must match the web's `LABELS`, so both get the new state's label in the same change, guarded by a test.
- Prototype branch `proto/epic-dependency-ux-waiting` was never merged and is throwaway; the stored mock is the reference for the look.
- Map: `gx:epic-dependency-ux` (tickets 01 mock, 02 TUI wording, 03 ticket rows).

## Self-decided design questions

Choices the spec makes without a person's answer. Each has its reason.

- Blocker state, not a moving flag: each blocker carries its Epics page state, because a yes/no flag cannot tell "not queued" from "Paused" or "Deadlocked". Clients map the state to the hint and the TUI suffix.
- Hint and suffix use the blocker's own state label ("is Deadlocked", "(Merge blocked)") in the Paused text tone, because "Paused" alone is wrong for Deadlocked, Merge blocked, CI failing and PR text failed.
- A Waiting blocker is not moving when its own blocker is not moving, and the hint names the deepest not-moving epic, because a queued Waiting blocker over a stalled one would otherwise read as moving.
- Blocker order: the server sorts not-moving first, then by name, and every surface takes the first, so "A" and the hint agree everywhere.
- Precedence: Paused beats Waiting and Cycle beats Waiting (owner's answers). Paused beats Cycle. Waiting comes only from an unmerged epic-level blocker, with nothing running.
- Which states count as moving: Implementing, running, Awaiting merge, Awaiting PR, PR open. A blocker that needs a merge or a pull request already shows that on its own card.
- One blocker list replaces `blocked_by_epics` (the waiting-on set and the own-blocker list are identical once ticket-level blockers are gone), in one change with regenerated `server.ts`, fixtures and docs.
- Chip scope: one chip per unmerged own `blocked_by` entry, whole epics only, each with ×. Merged entries show no chip and are kept on every web write.
- New state: added to the wire, TUI labels and web `LABELS` together, with a test that fails on a missing label. No `EpicState` is stored, so no stored-data migration.
- Cycle refusal is a coded error `blocked-by-cycle` with blocker and chain, so the web renders the mock's wording itself and the server text can change.
- `cardStatus` maps the new state to "Waiting" and builds the tooltip from the blocker list; the jump palette follows `cardStatus`.
- Mock states: the mock binds only header, chips, picker, card and sidebar; the cycle-refusal fixture uses an unticked epic that waits on this one, and ticket rows differ from the mock.
- Cross-epic ticket blockers are removed with no migration, because only two cancelled tickets carry one and terminal tickets are skipped.
- Cycle (Deadlocked) is kept and made reachable instead of retired, because a cycle is a gx bug or a bad manual edit and "Deadlocked" says so (owner's answer).
- Rows of a Waiting epic show only in-epic blockers; the epic-level blocker lives in the header and chips (owner's answer).
- Refused stored entries show as error chips with a ×, and the web route checks only the entry it changes (owner's answer, with a default for the check scope).
- The blocker list and the hold share one release predicate, so on-done releases agree everywhere.
- Picker scope (loop epics of the same project) and the release rule are unchanged defaults from charting.
