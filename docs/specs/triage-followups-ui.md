## Problem Statement

A project's **Follow-ups** pile up and the web gives no sign of it. The sidebar shows no count, and the Follow-ups card on the Epics page has a Triage button that opens a placeholder dialog ("Triage UI not yet implemented") telling the person to copy `/gx-triage-follow-ups --project <p>` into a terminal. A **Triage** can only be run from a CLI session, where the plan is a chat table and the person approves by typing "yes". A person who works in the web cannot see the plan, change a row, answer the quick questions in one place, or follow the writes. The Design flow already has a web page with steps, Progress, Continue and auto-advance, but all of it is hardcoded to Design.

## Solution

The web runs a Triage the way it runs a Design. Each project's open follow-up count shows in the sidebar, also in named workspaces. The Follow-ups card stays and takes the Triage state. Triage on the card starts a **stepped session** on one shared model: a **Flow** has two kinds, Design and Triage, and the step list, labels, skill name, Progress, Continue, auto-advance and session strip take the flow kind.

A Triage has five steps: Verify, Plan, Approve, Write, Done. The agent verifies each follow-up against the code and plans the rows. The person then opens the Approve step of the Triage page, answers the quick questions in the shared question component, reviews five collapsible action groups, changes any row, and approves once. Approve starts a Write session that writes the rows. The Done view lists the result with links and offers a Queue button for the ready epic. gx never queues it on its own. One `gx-triage-follow-ups` skill drives both the web and the plain CLI, with the same routing rules.

## User Stories

1. As a person with several projects, I want a follow-up badge in the sidebar footer, so that I can see that follow-ups are waiting without opening the Epics page.
2. As a person, I want the badge to show the open follow-ups still in the epic, summed over the projects of the active workspace, so that the number is the work left.
3. As a person, I want no number on the badge at zero and "99+" above 99, so that the badge stays readable.
4. As a person, I want the badge to turn amber exactly when the project's card is in "Needs you" (a Triage at Approve, or one with no session, no `pending` row and a failed write row), so that I notice what needs me.
5. As a person, I want a popover with one line per project (name, bug/idea split and count), so that I can pick the project to triage.
6. As a person, I want a project with a live Triage to read "Triage · N left" with an Open button, so that I can jump back into it.
7. As a person in a named workspace, I want the badge and popover to cover that workspace's projects, so that the count matches what the sidebar shows.
8. As a person, I want a popover row to open the Triage page while a Triage is live or waiting for Approve, and otherwise the Epics page at the Follow-ups card, so that the click goes where the next action is.
9. As a person, I want the badge and popover to work on a phone and in the light theme, so that I lose nothing on a small screen.
10. As a person, I want the Follow-ups card to keep its count words ("5 bugs, 2 ideas") of open follow-ups still in the epic, so that the card says what is waiting.
11. As a person, I want Triage on the card to start a Triage and open its page, or open the live one when it exists, so that I never start a second Triage by accident.
12. As a person, I want the same card to show a five-step ring, a state pill and a button that match the Triage state, so that I do not need a second Triage card.
13. As a person, I want the Epics page to place a Triage card by this order, so that it orders my attention: a running session goes under "Designing" with Open; with no session and rows `pending`, under "Designing" with the pill "Stopped" and Continue (like a stopped Design); with no session, no `pending` row and at least one `failed` row, under "Needs you" with the pill "Waiting for you" and the button "Review failures"; and a Triage at Approve (always, since no session runs there) always under "Needs you" with the pill "Waiting for you" and the button "Review plan".
14. As a person, I want the card to stay, reading "0 left", while a Triage is not done, so that the ring and pill do not vanish mid-Write.
15. As a person, I want no Finished card after a Triage, and the card to stay with the kept count and a Triage button when rows were kept, so that the Epics page does not clutter.
16. As a person, I want a Triage started from a plain CLI session and stopped at Verify, Plan or Write to show on the card as "Stopped" with Continue, and one at Approve to show "Waiting for you" with "Review plan" (never "Stopped"), so that I can pick it up in the web.
17. As a person, I want Continue on a stopped Triage to start a stepped session at the next step and never a second Triage, so that no work repeats.
18. As a person, I want the Triage page at `/triage/<project>` to look like the Design page (header, step line, ring, primary button, session strip, Progress panel, composer), so that I already know how to use it.
19. As a person, I want Verify to run as a working step that saves each verdict as it returns, so that I can see the progress.
20. As a person, I want Verify to span as many sessions as it needs and auto-advance to chain them, so that a long list needs no babysitting.
21. As a person, I want a fresh session to verify only the rows with no verdict yet, so that a stopped Triage never redoes saved work.
22. As a person, I want Plan to end its session without a Continue or countdown, so that nothing moves on before I look at the plan.
23. As a person, I want the Approve step to open on a Questions view while any question is open and on a Review view once none is, so that I answer first.
24. As a person, I want the Questions view to be the shared question component unchanged (dots, "N of M answered", choices with the recommended mark, Huh?, Note, Prev and Next), so that answering behaves as on a Design.
25. As a person, I want the Review view to show five collapsible action groups (Ready epic, Map epics, Merge into another, Close, Keep for later) with count, note and a set-all control, so that I can check the plan by action.
26. As a person, I want to change a row's action and destination, with a changed row outlined, so that I see what I edited. The destination choices are the plan's suggested epics, the project's open map epics, and a "new map epic" option with a typed name; "Merge into" lists only the other rows whose current action is Ready epic or Map epic. When a merge target's action changes away from those, the rows merged into it are outlined and the Approve bar stays disabled until each one is re-pointed. The routing rules are unchanged.
27. As a person, I want an Evidence button on each row, so that I can check the verdict before I agree.
28. As a person, I want one Approve bar under both views that is disabled while a question is open, says how many are left and links to Questions, so that I cannot approve with questions open.
29. As a person, I want the bar to show the counts per action and how many rows I changed, so that I know what Approve will do.
30. As a person, I want Approve to save the plan as approved and start the Write session at once, so that no session waits while I edit. Approve is one verb doing a compare-and-set on the record revision.
31. As a person, I want the answers I gave to go into each ticket body when Write runs, so that the agent that picks the ticket up sees them.
32. As a person, I want the Write step to list a row per follow-up with its state, so that I can follow the writes.
33. As a person, I want a failed row to show the verb name and the refusal text unchanged, with Retry and Keep for later while the follow-up is still in `follow-ups`, and Retry only after its `move`, so that the ready epic never holds a half-set ticket I called "kept".
34. As a person, I want one failed row not to stop the others, with chain edges and the code-review stub written in a last pass, so that a refusal costs one row.
35. As a person, I want Retry to rerun the whole row safely, so that a crash or a refusal never writes a row twice.
36. As a person, I want a per-row Keep for later that leaves the follow-up in the epic while the Triage still ends done, so that I can postpone a row without blocking the Triage.
37. As a person, I want the Done view to group the result by outcome (ready epic, map epics, merged, closed, kept) with links, so that I can see what changed.
38. As a person, I want a Queue button for the ready epic in the header, left of the ring, hidden when there is no ready epic, and replaced by a queued state that comes from the ready epic's queue state (the field the epic card reads), not from the click, so that I choose when it runs and see that it did, also after a reload or in a second tab.
39. As a person, I want past Triages listed read-only below, newest first, so that I can look back.
40. As a person, I want exactly one live Triage per project, enforced by the server, which creates the record and snapshot before it launches the session and refuses if a live record exists (two simultaneous starts end with one record), so that two never write the same rows.
41. As a person, I want to abandon a Triage from the ⋯ menu at Verify, Plan and Approve, and at Stopped only when the stored step is Verify or Plan (never once the step is Write, whether running, stopped or with failed rows), so that a stale or unwanted Triage never blocks the next one.
42. As a person with two browser tabs open, I want a stale save refused, so that one tab never overwrites the other's edits.
43. As a person, I want a Triage to work on the follow-ups that existed when it started, and a row closed or moved by someone else to show as "gone" and be skipped, so that the plan stays stable.
44. As a person, I want Triage sessions labeled "Triage" on the Agents page and counted in a "Triage" Agent group in the token ledger and Stats, so that I see what they cost.
45. As a CLI user, I want the same skill to print the chat table, ask the quick questions in chat and write on "yes" in a plain session, so that I can still triage from the terminal.
46. As a CLI user, I want a Triage I started in the CLI to be continued, not duplicated, by the web, and the reverse, so that both use one record.
47. As a person on a phone, I want the Triage page to hide the Progress panel and keep the view switch and Approve bar usable, so that I can approve from my phone.
48. As a Design user, I want the Design page, its Continue, auto-advance and worktree launch to work as before, so that the shared model costs me nothing.
49. As a person who may click Approve in the web while a chat-mode session waits for "yes" (or from two tabs), I want exactly one approval to win and run Write, and the other to be refused ("already approved", or a stale refusal that reloads the Review view when the plan was edited meanwhile), so that Write never runs twice.

## Implementation Decisions

**Shared model.**
- The shared model is a **Flow** with two kinds, `design` and `triage`. A session that runs a flow is a **stepped session**.
- The session meta bool `Design` becomes a flow kind. Old session metadata with `design: true` and no flow reads as `design`.
- One table keyed by flow gives the step list, labels, final step, skill name and Continue argument. Design keeps its list, `gx-design <map address>` and final step Branch or Done. Triage uses the five steps, `gx-triage-follow-ups --project <project>` and final step Done.
- The wire step type is a plain string with a `flow` field. The generated TS regenerates, and the step-sync test moves to per-flow lists.
- Each flow step says whether it needs a person. Triage Plan raises no Continue and no countdown, and Approve is the only way on. Verify and Write chain with auto-advance as usual. The Triage context-growth values for `gx agent context-window --fits` (verify, write) are the implementer's choice.
- Design-only code (design worktree launch, cleanup, map creation, drafts, charting handoff, Grilling, Hold, the impl link, Delete, workspace Move) stays in the Design packages and web files. The shared session code never imports it. It is reached only through a flow check at its own entry points.
- Triage launch skips the design worktree and runs in the trunk checkout. A Triage session has no Ticket.
- The `AskUserQuestion` ban stays on every stepped session, because the plan view replaces chat questions. A Triage has no Grilling round.

**Session link.**
- `gx agent link --triage <project>` sets the flow to `triage` and the key to the project name. A Design's key stays its map address. `gx agent step` is unchanged. A Triage session has no `Ticket`, so it is placed under Other (`PlacementOther`), never under an epic. `gx agent link --triage` exits non-zero when it cannot link (not in a gx session, server not running, unknown project, or a Design session), because the skill reads failure as chat mode; the existing `gx agent link <ticket>` and `gx agent step` keep succeeding silently outside a session.
- Triage sessions show the label "Triage" on the Agents page. "Triage" is added to the Agent group list, the poller and the web color map together (the color is the implementer's choice). The glossary already has **Flow** and **Stepped session**.

**Triage record.**
- One JSON record per Triage lives in a dot-prefixed folder of the project's ticket store. The store is git-backed, and `gx tickets epics` skips dot folders, so no epic card appears. The `follow-ups` epic and tickets get no new field. There is no epic or ticket schema change, so there is no migration step. A missing folder reads as "no Triage". The record carries a `version` field so a later format change can add a step.
- The record holds the follow-up snapshot (addresses at start), step, per-row verdict, files, questions and suggested destination, plan rows (stable row id, action, destination, quick questions with a default, overlap chain), person fields (action, destination, answers, Keep for later), per-row write state (`pending`, `done`, `failed`, `gone`), a last-pass-complete flag, the new address after a move, the last refusal text per failed row, the result, and the state (live, done, abandoned). A successful rerun clears a refusal text.
- Skills and the web read and write it only through new `gx` verbs, never by path. The implementer picks the verb names, flags and JSON field names. The verb that creates the record stamps its identity and start time.
- One writer per phase, and every save carries the record's revision: Verify and Plan sessions write verdicts, suggestions and planned rows before Approve. Person fields are written between Plan and Approve (web or chat-mode session), and Retry and Keep for later are written by the web. The Write session writes only row write state, the new address and the last-pass-complete flag. Any session (Verify, Plan, Write) that finds a follow-up already closed or moved out by someone else saves its row as `gone`, and Plan leaves gone rows out of the plan's action groups. A stale revision is refused. The CLI and the web use the same verbs and the same rule.
- One approve verb does a compare-and-set on the record revision, moving the step from Approve to Write. The web Approve button and the chat-mode "yes" both call it, and only the winning caller runs Write. The verb refuses with "already approved" when the step is past Approve. It refuses as stale when the step is still Approve but the revision changed (a person-field save in between); then the web reloads the Review view and chat mode reprints the table and asks again.
- The record verb sets the Triage `done` when every row is done, kept or gone and the last-pass-complete flag is set. The Write session sets that flag after chain edges, the code-review stub and the `--sweep`, which all run in its last pass. Retry, and Keep for later on a failed row, clear nothing: Retry sets `pending` and the server launches a Write session, which reruns the last pass and sets the flag again. A crash between the last row save and the last pass leaves the Triage not done.
- One live Triage per project is enforced by an atomic create verb: it takes an exclusive create on a live marker (or a lock) in the Triage folder, so of two simultaneous creates exactly one succeeds. The server start and the plain-session skill both call that verb. The server creates the record and snapshot before it launches the session and refuses (opening the live one) if a live record exists. Past Triages stay as read-only history.

**Steps and sessions.**
- Verify: Verify finds the live record and snapshot (created by the server start, or by the skill's create verb in a plain session). Each verdict is saved the moment its subagent returns, by the main session (subagents write nothing). Verify ends when every row has a verdict. Verify subagents use `gx agent model worker`.
- Plan: dedupe is part of Plan, and merges stay plan rows until Write. Plan writes no epics or tickets and ends its session with a run-end line pointing to the web plan view. Plan's last save moves the stored step to Approve.
- Approve: no session runs. The Approve button calls the approve verb (compare-and-set on the revision) and launches a new Write session only if it won. The view (Questions or Review) is derived from the open-question count, and no new step is stored.
- Write: takes the approved plan from the record and treats the person's fields as final (it never re-plans). Per row the ordered verbs are: write answers into the ticket body (the answers verb checks the body first and skips if they are already there), add the "seen again" merge note, `move` or `close`, then `set` type and status. Each verb checks the real ticket state first and skips what already happened, so no per-verb progress is stored. Retry reruns the whole row. Chain edges between ready-epic tickets and the code-review stub are written in a last pass, which a Retry of a failed row reruns (the server launches a Write session for it).
- Done: every snapshot row is `done`, kept or gone and the last pass is complete (the record verb sets the Triage done then). The Done view shows the Queue button only when a ready epic exists, and a queued state derived from the ready epic's queue state, so it survives a reload. Follow-ups filed after the snapshot do not count and wait for the next Triage. The `--sweep` of `done` and `cancelled` follow-ups runs in Write's last pass.
- Every step ends with a `✅ <step>: … Next: …` line. Continue is `/clear` then `/gx-triage-follow-ups --project <project>`, and the skill reads the step from the record.
- Failure rules: `epic add` refusing an epic this Triage already created (its address is in the record) counts as done. Otherwise the row fails with the verb's own refusal text. A verb that needs a stopped server fails the row with `server-not-running`, and Retry works once the server is back. Keep for later is offered only while the follow-up is still in `follow-ups`. gx never undoes a write.
- Abandon Triage: offered in the ⋯ menu at Verify, Plan and Approve, and at Stopped only when the stored step is Verify or Plan. It is never offered once the step is Write (running, stopped or with failed rows); the way out of Write is Continue, Retry or Keep for later. It ends any running session, marks the record abandoned and moves it to the read-only history. It writes nothing to tickets, so rows already written stay written. The card then shows a plain Triage button. It adds no pause kind (ADR 0042): nothing pauses, and the person ends it from the web.

**Skill.**
- `gx-triage-follow-ups` runs `gx agent link --triage <project>` first. Success means web mode: Plan saves the rows and ends for the web plan view. Failure means chat mode: it prints the table in chat, asks the `quick` questions in one chat message, saves answers and edits through the person-field verb, and on "yes" runs Write in the same session. Both modes use the same record and verbs. It calls `gx agent step <verify|plan|write|done>` at each step.
- Step detection reads the record through a verb: no live Triage starts one, otherwise the first match of Verify rows with no verdict, then Plan, then Approve (plan saved, not yet approved: wait for the person, in chat mode ask for "yes"), then Write once approved, then Done. A live Triage found from a plain session is continued, not duplicated.
- Unchanged: routing rules, "never grills", the correct-fix section, the `follow-ups-<date>` ready epic and map epics. The skill never queues the ready epic and points to the local tracker doc for verb syntax.

**Server and wire.**
- The Follow-ups card (one per project, absent when no open follow-up and no live Triage) takes the Triage state: count words of open follow-ups still in the epic, a five-step ring filled the way the Design epic card fills its ring, a state pill and a button per state. Placement order, with Approve checked first: a Triage at Approve always goes to Needs you with the pill "Waiting for you" and the button "Review plan", even while a Plan session is still live for a moment; then a running session goes to Designing with Open (as `placeDesign` puts a running Design); then no session with a row `pending`, or with all rows written but the last pass not complete (a crash after the last row save), goes to Designing with the ring at the stored step, the pill "Stopped" and Continue (Continue reruns the last pass), like a stopped Design (a stopped Design shows no pill today, so the pill is new); then no session, no `pending` row and at least one `failed` row goes to Needs you with the pill "Waiting for you" and the button "Review failures" opening `/triage/<project>`. The ring percent is `(index of the stored step + share of the step's rows handled) / 4`, as a percent, Done = 100 (the mock numbers are illustrative). A card with no Triage is in "Ready to design". "Stopped" never shows at Approve. The card stays while a Triage is not done (even at zero open follow-ups, reading "0 left"); after done it is absent at zero and present with the kept count when rows were kept. The card title links to `/triage/<project>`.
- gx stops an idle Triage session once its stored step is final (Approve, Done, or Write with no row `pending`), as `stopFinishedDesign` does for a Design. This is what "Plan ends its session" and "Approve: no session runs" mean, so the card reads "no session" at Approve and after a Write that left failed rows. It never stops a working session. It adds no pause kind.
- "N left" is the number of snapshot rows whose follow-up is still in `follow-ups` as draft or open (kept rows count; follow-ups filed after the snapshot do not).
- A project is in a workspace when at least one of its Design or Implementation cards is; a project with none is in the Default workspace. The count entry and the card carry that list, so a named workspace shows the card and the badge row of its projects.
- The ledger rebuild that reads session `meta.json` reads `flow` too, so rebuilt Triage sessions stay in the Triage group. This is not a schema migration.
- `wire.FollowUpCount` gains the bug/idea split, the Triage state and an "N left" field (shape and names are the implementer's choice). The card's count words and these counts feed both the card and the sidebar badge. The card is shown in named workspaces too.

**Web.**
- Route `/triage/<project>` shows the live Triage, or the latest when none is live, with past Triages listed read-only below. `/d/<project:epic>` is untouched.
- Reused as they are: Progress step tree, session strip, composer Continue, auto-advance bar, session controls start and new-session. The Questions view is the current shared question component (`QuestionSheet`) used as it is. The mock's question card is only a placeholder for it. Design-only: tickets sub-tree, impl link, map image strip, Delete, Hold, workspace Move.
- The sidebar gets an inbox button in the footer tray with a count badge (summed open follow-ups, "99+" cap, no number at zero, amber exactly when the card is in "Needs you") and a popover with one line per project. The sidebar list is untouched. A popover row for a project with no live Triage opens the Epics page with route state `focus` naming the card; the page opens the card's section (Ready to design is collapsed by default) and scrolls it into view.
- The look is fixed by the two stored mocks (see Verification), not repeated here.

## Testing Decisions

- A good test checks external behavior: what the record and the verbs accept and refuse, what the card and the page show, and what the Write step leaves in the ticket store. It does not check internal markers or component structure.
- Highest seam: the Triage page and the Follow-ups card driven against the fixture server (a new Triage scenario seeded with follow-ups and a fake agent, the implementer picks the name), plus the `gx` verbs and the server verbs against a real ticket store in Go tests. The shared flow table, `meta` compatibility and the card builder are covered by Go tests on the server packages. Components (Review groups, Approve bar states, badge and popover) are covered by vitest.
- Playwright budget (gx-playwright-e2es): the default is no new flow. Extend the existing Design page flows (`design.e2e.ts`, `design-page.e2e.ts`) with the checks that Design behavior is unchanged under the shared model. Add at most one new flow, for the Triage journey (`maxFlows` in `playwrightguard/budget_test.go` goes from 125 to 126, and `webui/e2e/CATALOG.md` says 124 today and is corrected), because it needs the server round trip, SSE, fake agents and the phone layout, and a vitest or Go test cannot see those. Everything that is "what a component renders" goes to vitest. Name the vitest and Go tests considered in the commit.
- Prior art: `design.e2e.ts` and `design-page.e2e.ts` for stepped sessions, Progress and auto-advance; the Workspaces scenario flows for sidebar footer behavior; `phone.e2e.ts` for the phone Design page; Go tests on the epic card builder; the step-sync test for per-flow lists.
- No epic or ticket schema change, so there is no migration step to test beyond a missing record folder reading as "no Triage" and old `design: true` metadata reading as the Design flow.

## Verification

Run against a temporary gx server with the new Triage fixture scenario. CLI verb names are the implementer's, so the checks name the behavior.

1. Story 1: flows: Workspaces sidebar flow, extended with the badge. CLI: `gx server epics` lists the Follow-ups card with count words. Look check: Mock states c-few.
2. Story 2: badge equals the card counts for the active workspace's projects. Vitest on the sidebar model: the badge is the sum over the active workspace's projects. Look check: Mock states c-many.
3. Story 3: Mock states c-zero (no number) and c-many ("99+" at 45 is not capped, so seed above 99 in a vitest test).
4. Story 4: Mock states c-live (amber badge with a Triage waiting; the fixture seeds a Triage at Approve); vitest on the badge: amber exactly when the card is in Needs you (Approve, or no session with no `pending` row and a failed row), not for a running Triage or a stopped one with `pending` rows.
5. Story 5: Mock states c-few.
6. Story 6: Mock states c-live.
7. Story 7: Mock states c-named.
8. Story 8: a popover row click lands on `/triage/<project>` for a live Triage and on the Epics page card otherwise (new Triage flow plus Workspaces flow).
9. Story 9: Mock states c-light and c-phone.
10. Story 10: CLI: `gx server epics` card words after a write drop by the written rows. Epics flow.
11. Story 11: Triage on the card starts one and opens `/triage/<project>`; clicking it again opens the live one; the one-live check refuses a second creation through the CLI verb and through the server start (Go test: record and snapshot exist before the session launches); Go test of two concurrent creates that end with one record, the second start opening the live one.
12. Story 12: Epics flow section check (Designing, Needs you, Ready to design) in the Triage flow.
13. Story 13: same flow, the pill "Waiting for you" at Approve and, with a failed row, the button "Review failures"; Go test on the card builder and vitest on the badge cover the placement order: running goes to Designing with Open; no session with `pending` rows goes to Designing, "Stopped", Continue; no session, no `pending` row and a failed row goes to Needs you, "Waiting for you", "Review failures"; Approve always goes to Needs you. The writing-* fixture state names whether its session is running.
14. Story 14: the Triage flow writes all rows in the fixture and checks the card shows "0 left" before Done.
15. Story 15: after Done the card is absent at zero and present with the kept count when rows were kept (the fixture keeps two).
16. Story 16: the step-by-session matrix (Verify, Plan, Write, Approve, each with and without a session) is a card-builder Go test: pill "Stopped" and Continue at Verify, Plan and Write with `pending` rows, "Waiting for you" and "Review plan" at Approve (never "Stopped"; Go test of step detection including the Approve branch). The fixture keeps one CLI-started Triage with no session, stopped at Plan, for the flow's Continue check: Continue starts a session at the next step; `gx` read verb shows one Triage.
17. Story 17: same as 16.
18. Story 18: Mock states verify-d. Design flows `design.e2e.ts` and `design-page.e2e.ts` still pass.
19. Story 19: Mock states verify-d.
20. Story 20: auto-advance chains two Verify sessions in the Triage flow; Go test of the context-window `--fits verify` value.
21. Story 21: Go test: a second session's verify verb lists only rows with no verdict.
22. Story 22: after Plan the page shows no Continue and no countdown; Mock states b-questions-d.
23. Story 23: Mock states b-questions-d (opens on Questions) and b-review-d (opens on Review once none is open).
24. Story 24: Mock states b-questions-d, b-questions-p; the real `QuestionSheet` is used as it is (the mock's card is only a placeholder).
25. Story 25: Mock states b-review-d, b-review-p.
26. Story 26: Mock states b-review-d; vitest on the destination choice list (suggested epics, open map epics, "new map epic" with a typed name, "Merge into" listing the other snapshot rows); the vitest also checks that "Merge into" lists only Ready epic and Map epic rows, and that re-pointing a target away outlines its merged rows and disables the Approve bar; CLI: the person-field verb saves the change and a stale-revision call is refused.
27. Story 27: Mock states b-review-d.
28. Story 28: Mock states b-questions-d (disabled bar, link to Questions).
29. Story 29: Mock states b-review-d (counts and "3 changed by you").
30. Story 30: Approve starts a Write session in the flow; CLI: the record shows the plan approved.
31. Story 31: after Write, the ticket bodies in the ready epic hold the answers (`gx tickets show`).
32. Story 32: Mock states writing-d, writing-p.
33. Story 33: Mock states writing-d, writing-p (failed rows: Retry and Keep for later before the `move`, Retry only after); Go test of the offer rule.
34. Story 34: Go test and the Triage flow: one failing row, the others `done`, the last pass runs after.
35. Story 35: Go test: rerun after a crash between a verb and its save writes no second merge note, no second `move`, and the answers appear once in the ticket body; CLI: `gx tickets show` of the ticket.
36. Story 36: Triage flow: Keep for later leaves the follow-up in `follow-ups` (`gx tickets show`), the Triage ends done. Mock states b-review-d.
37. Story 37: Mock states result-d, result-p.
38. Story 38: Mock states result-d (button left of the ring); the Queue button queues the ready epic (`gx server queue` lists it), and no queue entry exists before the click; the button is hidden when there is no ready epic (vitest); the queued state derives from the ready epic's queue state, and the Triage flow reloads the page after the click and still shows it.
39. Story 39: Triage flow: a second finished Triage is listed read-only below the live one.
40. Story 40: CLI: creating a Triage while one is live refuses; Go test: the server start refuses when a live record exists and creates record and snapshot before launching.
41. Story 41: Triage flow: Abandon in the ⋯ menu at Plan; no ticket changes; the card shows a plain Triage button; the card-builder or menu Go test checks Abandon is offered at Verify, Plan, Approve and a stopped Verify or Plan, and not while Write runs, at a stopped Write or with failed rows; the pause-kind table test (ADR 0042) is unchanged.
42. Story 42: Go test: two saves with the same revision, the second refused. Triage flow with two contexts.
43. Story 43: Go test: a row closed by someone else mid-Triage is stored as "gone", is skipped, and the Triage still sets done once every row is done, kept or gone and the last pass is complete; one row going gone during Verify and one during Write are both covered, and Plan leaves gone rows out of the action groups; a crash between the last row save and the last pass leaves the Triage not done, and Retry's Write session reruns the last pass and sets it done; a follow-up filed after the snapshot is not in the snapshot.
44. Story 44: Agents page label "Triage" in the Triage flow; `gx` token ledger shows the "Triage" group (Go test of the group list, poller and web color map together).
45. Story 45: skill run in a plain session: the link fails, the table prints in chat, writes happen on "yes" (skill behavior walked by hand against the fixture, no new flow).
46. Story 46: CLI-started Triage is continued by the web (story 16), and a web-started one is continued by the CLI skill with no second record.
47. Story 47: phone width 390 in the Triage flow: no Progress panel, no sideways scroll; Mock states verify-p, b-questions-p, b-review-p, result-p.
48. Story 48: `design.e2e.ts`, `design-page.e2e.ts`, `newdesign.e2e.ts` and the phone Design flow pass unchanged; Go test that old `design: true` metadata reads as the Design flow.
49. Story 49: Go test: two approve calls on the same revision, the first wins and the second is refused with "already approved"; a call at step Approve with a changed revision is refused as stale (the web reloads the Review view, chat mode reprints the table and asks again); the web Approve and the chat-mode "yes" use the same verb, and only the winner starts Write (Triage flow with a second context).

New flows or scenarios to write (these become implementation tickets):
- A Triage fixture scenario (the implementer picks the name) seeded with follow-ups of both categories and a fake agent. States needed: one Triage at Approve with open questions (b-questions-*, b-review-*, c-live), one in Verify (verify-*), one in Write with a failed row before its `move` and one after (writing-*), one Done with two kept rows (result-*), one CLI-started Triage with no session, stopped at Plan, and a project with a few follow-ups and none live (c-few), one with 45 (c-many), one with none (c-zero), a named workspace holding two projects (c-named).
- One new Triage journey flow (the only new flow), plus extra steps in the Design flows listed in Testing Decisions.

**Mock states** (the Triage page mock):
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, verify-d, 960, scenario: Triage in Verify, parts: header, ring, header-button, progress
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, writing-d, 960, scenario: Triage in Write with two failed rows, parts: header, ring, header-button, progress
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, result-d, 960, scenario: Triage Done with two kept rows, parts: header, ring, header-button, progress
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, b-questions-d, 960, scenario: Triage at Approve with three open questions, parts: header, ring, header-button, view-switch, question-card, approve-bar, progress
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, b-review-d, 960, scenario: Triage at Approve with no open question, parts: header, ring, header-button, view-switch, action-group, approve-bar, progress
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, verify-p, 390, scenario: Triage in Verify, parts: header, ring, header-button
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, writing-p, 390, scenario: Triage in Write with two failed rows, parts: header, ring, header-button
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, result-p, 390, scenario: Triage Done with two kept rows, parts: header, ring, header-button
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, b-questions-p, 390, scenario: Triage at Approve with three open questions, parts: header, ring, header-button, view-switch, question-card, approve-bar
- `$(gx tickets root)/triage-followups-ui/mocks/06-triage-page-mock.html`, b-review-p, 390, scenario: Triage at Approve with no open question, parts: header, ring, header-button, view-switch, action-group, approve-bar
- `$(gx tickets root)/triage-followups-ui/mocks/05-sidebar-count-mock.html`, c-zero, 290, scenario: no open follow-up, parts: sidebar, followups-button
- `$(gx tickets root)/triage-followups-ui/mocks/05-sidebar-count-mock.html`, c-few, 290, scenario: a few open follow-ups in two projects, parts: sidebar, followups-button, followups-popover
- `$(gx tickets root)/triage-followups-ui/mocks/05-sidebar-count-mock.html`, c-many, 290, scenario: 45 open follow-ups, parts: sidebar, followups-button, followups-popover
- `$(gx tickets root)/triage-followups-ui/mocks/05-sidebar-count-mock.html`, c-live, 290, scenario: a live Triage waiting for Approve (amber badge), parts: sidebar, followups-button, followups-popover
- `$(gx tickets root)/triage-followups-ui/mocks/05-sidebar-count-mock.html`, c-named, 290, scenario: a named workspace with two projects, parts: sidebar, followups-button, followups-popover
- `$(gx tickets root)/triage-followups-ui/mocks/05-sidebar-count-mock.html`, c-light, 290, scenario: a few open follow-ups in the light theme, parts: sidebar, followups-button
- `$(gx tickets root)/triage-followups-ui/mocks/05-sidebar-count-mock.html`, c-phone, 390, scenario: a few open follow-ups at phone width, parts: sidebar, followups-button, followups-popover

## Out of Scope

- The TUI: no Triage view there.
- A Triage that runs with no person (scheduled or automatic). The one approval stays.
- Changing the triage routing rules (which follow-ups go to the ready epic and which to a map epic).
- gx-evolve on Triages, and Huh? records for them.
- gx queueing the ready epic on its own.
- A worktree or branch for a Triage (it writes only tickets).
- An epic or ticket schema change, so there is no migration step.

## Further Notes

- Defaults (ticket 02): a Triage works on the follow-ups that existed when it started. Past Triages stay as history, one live per project.
- The agent-launch-and-recovery epic adds the shared question component under a shared folder; until it lands, the Design page's current `QuestionSheet` (`webui/src/design/QuestionSheet.tsx`) is the one to use; the implementer finds the current copy with `rg` first. The Triage page must use whichever exists, as it is.
- Decisions behind this spec are in the `gx:triage-followups-ui` map, tickets 01 to 10. The design coupling inventory is `docs/research/triage-followups-ui-01-design-coupling-inventory.md`.
