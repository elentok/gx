## Problem Statement

A **Triage** is too slow and too heavy for the person who runs it. The last three Triages had 84, 171 and 57 follow-ups and asked 23, 245 and 81 questions. Past 10 question cards the page stops being usable. Most of those questions were small choices the agent could have made itself ("where should the lookup live?", "what poll interval?").

Answering is a one-shot form, not a conversation. The person opens a Questions view, a Review view and an Approve bar, answers everything at once, and sends answers back for a new round. A **Design** works as a chat. A Triage does not, and it is hard to use on a phone.

Each Triage also makes a new area map epic for every group of design items. Past Triages left 107 such epics (432 tickets, 311 of them open). Many overlap: five of them are about flaky tests. They are hard to find, hard to compare, and no Design ever picks them up in a useful order.

## Solution

A Triage runs as a conversation, like a Design, in the web and in a plain terminal, and it works on a phone. Its steps are Verify, Route, Questions, Write (then Done). Verify checks each follow-up against the code. Route sends each follow-up to the ready epic, to an **area**, or closes or merges it, and the agent prints the plan as a read-only summary card in the chat. Questions asks the person a few questions in rounds of at most 6 and at most 3 rounds, then asks one final "Write this plan?". Write writes the plan.

The agent decides every question that does not change scope, user-visible behaviour, a contract or routing, and lists its choices per follow-up under "Decided unless you object". The person can object to any of them.

Design work no longer goes to a new map epic per group. A project has a small fixed set of **areas** (at most 12), a new epic kind that holds follow-ups that need deciding and never runs a Design. A new **Backlog** page lists the areas with their open ticket counts. Start Design on an area runs a short chat in which the agent proposes one theme of 8 to 14 tickets. On yes, gx moves them into a new map epic.

The first Triage after the change also moves the open tickets of the old triage-made area map epics into the areas, once, as part of the same plan and the same final approval.

## User Stories

1. As a person triaging, I want the agent to decide every question that does not change scope, user-visible behaviour, a contract or routing, so that I answer only what needs me.
2. As a person, I want a question to reach me only when its answer changes scope, user-visible behaviour or a contract (UI, CLI, a config key, a skill rule), routing, or is expensive to reverse, so that I know every card I see matters.
3. As a person, I want at most 6 questions in a round, so that a round fits on one screen and one sitting.
4. As a person, I want at most 3 rounds of questions, so that a Triage ends.
5. As a person, I want bugs asked before ideas, and scope-changing questions first within each, so that the most important questions come first.
6. As a person, I want questions grouped by follow-up, each with its options and a recommended answer, so that a card stands alone and I can answer by agreeing.
7. As a person, I want questions that an earlier answer settles dropped, so that I never answer something twice.
8. As a person, I want the agent's own choices listed per follow-up under "Decided unless you object" after each round and in the final summary, so that I can see and overturn them before anything is written.
9. As a person, I want an objection to a self-decided choice to turn into a question in its own round, outside the 3-round cap, with at most 6 questions, so that I can always overturn a choice and an objection never loops on its own.
10. As a person, I want a follow-up that no round reached to go to its area as a `grilling` ticket with its questions in the ticket, so that nothing is lost and nothing is guessed.
11. As an agent that later works a ticket, I want each self-decided answer written into the ticket's Decisions marked "decided by agent", so that I know who decided it.
12. As a person, I want the Triage page to be a chat like the Design page, with question cards, Progress and a composer, so that I already know how to use it.
13. As a person, I want Progress and the ring to follow the steps Verify, Route, Questions, Write and Done, so that I see where the Triage is.
14. As a person, I want Route to print the plan as a read-only summary card in the chat, so that I can check it without a separate view.
15. As a person, I want the summary to show counts per action (ready, to an area, closed by reason) and per area, with the follow-ups of one group on demand, so that I can read it at a glance.
16. As a person, I want to correct a routing group by typing in the chat, and see the changed summary lines reprinted, so that fixing a group is one sentence.
17. As a person, I want Questions to show "Round N of 3 · k of n answered", one card at a time, with answered cards folded to one line and the composer open for a note, so that I can follow a round.
18. As a person, I want one final "Write this plan?" card, and nothing written before it, so that I approve once.
19. As a person, I want the approval to be refused when the plan changed since the summary I saw, or when another session already approved, and the summary reprinted, so that Write never runs on a plan I did not see and never runs twice.
20. As a person, I want a fresh session (auto-advance or `/clear`) to continue mid-Route or mid-Questions without asking anything again, so that a long Triage survives a context reset.
21. As a person, I want steps that need no one (Verify, Route, Write) to chain by auto-advance, and Questions to wait for me, so that a Triage never moves past a question.
22. As a CLI user, I want the same skill, record and verbs to run the whole Triage in a plain terminal, with the same text for the summary and the cards, so that I lose nothing without the web.
23. As a person on a phone, I want the Triage page to have a back arrow, the name, the ring, the menu and a step line that opens the steps sheet, and no extra summary chrome, so that the page fits at 390 pixels.
24. As a person, I want the Follow-ups card to show a ring over the five steps, a pill with the step name and a button per step (Continue, Answer, Review plan, Queue), with "Retry failed (N)" beside it when rows failed, so that the card says what to do next.
25. As a person, I want the card pill to read "Needs you" while a question round or the final approval waits, with the round and the questions left or "Write this plan?", even when a session is still live, so that I notice that a card waits for me.
26. As a person, I want gx to leave an idle Triage session alive at Questions, so that my answer reaches the same session.
27. As a person, I want the Write view to keep failed rows with Retry and Keep for later, the Done view to keep the result list with links and the Queue button for the ready epic, and the sidebar badge to keep the open follow-up count, so that nothing I rely on is lost.
28. As a person, I want to abandon a Triage at Verify, Route or Questions, and have it write nothing, so that an unwanted Triage never blocks the next.
29. As a person, I want design work sent to a small fixed set of areas per project instead of a new area map epic per group, so that I do not collect more epics with every Triage.
30. As a person, I want a project to have at most 12 areas, with a 13th refused, so that the set stays small.
31. As a person, I want the agent to propose a new area only when no area fits, and to ask me to confirm it before the other questions of that round, with three choices (create the area, file under the closest existing area, keep for later), so that I control the set and a decline has a concrete target. With no existing area, the question has two choices: create the area or keep for later. At the 12-area cap the agent proposes no area and asks nothing: it files the row under the closest area and lists that under "Decided unless you object". Areas named in the skill's shipped mapping table need no question: the final "Write this plan?" card approves them.
32. As a person, I want areas created only at Write, so that an abandoned Triage leaves nothing behind.
33. As a person, I want an area to be neither a loop epic nor a map epic: the queue and scheduler refuse it, the Epics page and sidebar do not list it, and its kind cannot be switched, so that no area runs by accident.
34. As a person, I want a Backlog page, linked in the footer Epics menu with no sidebar row, that lists each area with its open ticket count and a Start Design button, so that I can see where design work is waiting.
35. As a person, I want Start Design disabled on an area with no open tickets, so that I cannot start an empty Design.
36. As a person, I want Start Design to open a small dialog with an optional hint (a bottom sheet on a phone), so that I can steer the theme.
37. As a person, I want Start Design to run a short chat in which the agent reads the area's open tickets and prints one "Start this Design?" card with a title and 8 to 14 numbered tickets, each with a one-line reason, so that I can judge the theme.
38. As a person, I want to edit the theme by typing in the chat, and see the agent reprint it once, so that I can drop or add tickets.
39. As a person, I want an area with fewer than 8 open tickets to propose all of them, so that a small area can still start.
40. As a person, I want yes on the card to create a map epic (Destination unconfirmed, Notes naming the area and the ticket addresses), move the chosen tickets into it and print a link, so that the Design starts when I open it.
41. As a person, I want a failed move to leave the tickets already moved in the new map and a rerun to carry on, so that a failure is never half-hidden.
42. As a person, I want a second Start Design click on an area that has a live Start Design chat to open that chat, so that two chats never pick the same tickets.
43. As a person, I want the first Triage after the change to plan the move of the open tickets of the old area map epics into the areas, show it in the plan summary (tickets per area, epics to archive) and run it at Write under the same final approval, so that the clean-up needs no extra step.
44. As a person, I want the four epics past charting to stay in place with all their tickets, so that their real Decisions stay valid.
45. As a person, I want claimed, done and cancelled tickets to stay in their old epic, so that history stays where it was written.
46. As a person, I want each emptied epic to be stamped complete and archived, with its done tickets staying as history, and the 8 epics that never had tickets archived the same way, so that the Epics page loses the clutter.
47. As a person, I want a move unit (one old epic) that fails to show its error with Retry or Keep for later, and an epic that still holds an open, draft or claimed ticket to stay unarchived, so that one failure costs one unit.
48. As a person, I want later Triages, and Triages in projects with no such epics, to skip the move once every old epic is archived, and to plan only the leftover moves or the archive for an epic that kept a claimed ticket or a unit I kept for later, so that the clean-up finishes without repeating work.
48a. As a person, I want an old epic that holds a claimed ticket, and so cannot move or archive yet, to show as one "waiting" line in the summary and to make no Write unit, so that it neither blocks Done nor counts as work.
49. As a person, I want moved tickets to keep their follow-up address, so that I can trace them.
50. As a person, I want `gx tickets move` to move tickets out of a map epic while its Design has not started, and to refuse once it has, so that a Design in progress keeps its tickets.
51. As a person, I want Stats to count a map as a completed Design only when it reached Branch or Done, so that emptied charting maps do not inflate the count.
52. As a person, I want the Archive action to accept a map with `completed_at` stamped, so that an emptied charting map can be archived.
53. As a person with a Triage record from before the change, I want a finished or abandoned one to stay readable as history, a live one at Verify to continue, a live one at Plan or Approve to go back to Route keeping my verdicts and answers, and a live one at Write to be abandonable, so that no half-done Triage is stranded.
54. As a person, I want an unrelated Design to behave as before, so that the shared model costs me nothing.
55. As a person, I want no new pause kind, so that every stop still ends from the web (ADR 0042).

## Implementation Decisions

**Questions and tags.**
- The Verify brief gives each question one of three tags. `decide`: the agent picks. `ask`: the person picks from options. `design`: an open trade-off. The old `quick` tag is gone. Every question carries options and a recommended answer. The main session may overrule a tag.
- Test for `ask`: the answer changes scope (whether the work happens, how big), a user-visible behaviour or contract, routing (ready epic or area), or is expensive to reverse. Past examples: "Where should the dialog lookup live?" and "What exit-poll interval?" are `decide`. "Should the long-ticket alert mean longer than normal or probably looping?" and "Include seen-again follow-ups in scope?" are `ask`.
- The agent decides `decide` questions by the correct-fix rule (the clean, correct option over a quick one), and applies the same rule to every recommended answer.
- Each self-decided answer is saved in the Triage record (question, choice, short reason, an objected flag). At Write it goes into the ticket's Decisions marked "decided by agent".
- After each Questions round and again in the final summary the agent prints the self-decided answers per follow-up under "Decided unless you object". An objection sets the objected flag and turns that question into an `ask` question in the next round. A round started by an objection is outside the 3-round cap and holds at most 6 questions. The person starts every objection round by typing the objection, so it cannot repeat on its own.
- What gx code enforces and what the skill decides. The save-round verb refuses a round with more than 6 questions and a fourth round that is not an objection round. Write files unreached rows under their area, or the fallback area for a ready row. The ordering, the dropping of settled questions, the choice of "closest area" and "propose all tickets under 8" are judgments in the skill, checked by the fixture flow and not by Go tests.
- Routing corrections during Questions: a correction that moves a row to an area drops that row's unanswered questions from the open and later rounds, and the answers already given go into the ticket. A correction that moves a row to the ready epic adds its `ask` questions to the next round, inside the 3-round cap, and picks a fallback area at once. Both bump the revision.
- Round rules: at most 6 questions per round, at most 3 rounds (18 questions), grouped by follow-up. Order: `category: bug` before `category: idea` (a follow-up has no priority field), and within each, scope-changing questions first. Questions an earlier answer settles are dropped. New-area confirmation questions come first in a round. A follow-up not reached after 3 rounds goes to its area as a `grilling` ticket with its questions carried in the ticket's Question section. Every ready row that has an `ask` question carries a fallback area from Route for this case.
- The Questions step always runs after Route, because it hosts the final approval. With no `ask` question it holds only the "Decided unless you object" list and the final card.

**Triage record, version 2.**
- The record gets version 2. Steps are verify, route, questions, write and done. The record state (live, done, abandoned) and one live record per project are unchanged.
- It adds: the three-tag questions; saved rounds (each saved before it is printed, each with its questions, answers and answered state, including objection rounds); self-decided answers with the objected flag; area entries (name, source `table` or `agent`, confirmed, created; a `table` entry counts as confirmed by the final card, and the plan save refuses approval while an `agent` entry is unconfirmed); the old-epic move units (old epic, target area, state pending, done, failed or kept, last error); the fallback area per ready row; and an approved flag taken by the approve verb.
- It drops the plan `round` counter, the `earlier` list, the `auto` answer and the person's plan edits. Routing corrections are saved right after the reply and bump the revision.
- Step detection, by `gx tickets triage show`: no live record starts one; a row with no verdict is Verify; no saved plan is Route; a saved plan not yet approved is Questions; approved is Write; every row and every move unit done, kept or gone with the last pass complete is Done.
- What reads old records: the record loader reads versions 1 and 2. A version 1 record is upgraded in memory at every load and rewritten as version 2 only by the next save. Everything else (verbs, web, history list, card builder) sees only the version 2 shape. A done or abandoned version 1 record stays read-only history.
- A live version 1 record at Verify continues as version 2 (its `quick` questions become `ask`, `design` stays). One at Plan or Approve goes back to Route, keeping verdicts and given answers (as answered `ask` questions) and dropping the plan and the person's edits. One at Write cannot finish under the new rules, so for version 1 records only, Abandon is offered at Write. It writes nothing: rows already written stay written and the rest stay in `follow-ups` for the next Triage. A version 1 record at Write reads "Waiting for you" on the card with Abandon as its only action.
- Verbs: the existing ones keep their purpose. New verbs or flags save a round before printing, save an answer, save a routing correction, save an objection, and report the open round, so that a fresh session reprints the open round and skips answered questions. Names and flags are the implementer's choice, and the local tracker contract document (its Triage record section and its epic `--kind` list) is updated with them.
- The approve verb keeps its compare-and-set on the record revision. It refuses as "already approved" when the step is past Questions, and as stale when the revision changed since the summary. The agent calls it on a yes to the final card, then reprints the summary and asks again after a stale refusal. There is no web Approve button.
- Abandon is offered at Verify, Route and Questions, and never at Write (except version 1 records above). It writes nothing to tickets.

**Steps, skill and chaining.**
- `gx agent step` names are verify, route, questions, write and done. `gx agent context-window --fits` gains `route` and `questions` (growth estimates, like `verify` and `write`; the values are the implementer's choice). Steps chain by the context check, as in a Design.
- A step that waits for the person ends with its printed round, never with a tool call. Route needs no one and chains on. Questions waits and has no countdown. Write chains as before.
- Route proposes one group at a time and ends when the plan is saved, then the summary card prints. The routing rules become: a `design` question, or a need to investigate with no decision, goes to an area (`grilling` or `research`); everything else goes to the ready epic as `implement`, with its `ask` questions answered in Questions; close and merge rows are unchanged. The "more than 2 questions" rule is dropped.
- One `gx-triage-follow-ups` skill drives the web and the plain terminal. The terminal prints the same summary and cards as text. Only the card buttons differ. The skill never grills outside the Questions step.
- The ready epic is unchanged: one `follow-ups-<date>` epic per Triage, published once through the no-spec mode of the ticket-writing skill, never queued by the skill.
- The send-back action, the plan `round` counter and the `earlier` list are removed.

**Areas.**
- An area is a new epic kind, `area`, with status open. It holds follow-ups that need deciding and never runs a Design. Schema change: only the new kind. It has one migration step, `area-epic-kind`, a no-op that registers the kind (existing stores need no rewrite); the schema snapshot is regenerated.
- Treatment: the queue refuses an area (as it refuses a map). The scheduler and loop frontier ignore it. The Epics page, sidebar and Archive list neither loop nor map epics for it. `gx tickets epics --maps` does not list areas, and a new `--areas` filter lists them. `epic set --kind` refuses a change to or from area. `epic add` refuses a 13th area and the Route plan save refuses a plan that would create one.
- The area list is the set of area epics of a project, with short kebab-case slugs, exactly 12 at most. In Route a follow-up goes to an existing area. The agent proposes a new area only when none fits, and the person confirms it with a question that has three options: create the area (recommended), file the rows under the closest existing area (named in the option), or keep them for later. Areas named in the shipped mapping table are not proposals and get no question: the final card approves them. With no existing area (a first Triage in a new project) the question has two options, create (recommended) or keep for later. At 12 areas, and gx starts there, the agent proposes nothing and asks nothing: it files the row under the closest area and lists that under "Decided unless you object". The confirmation question exists only below the cap, and the plan save refuses a confirmed 13th area. Keep for later leaves the rows in `follow-ups`, and the person chooses it on purpose. Areas are listed in the Route plan and created at Write, on first need.
- No delete, rename or merge for areas in this epic; those stay by hand (a merge moves all tickets, then the empty area is deleted). An empty area stays.
- The move planner accepts a charting map or an area as a source, and an area as a target. A map is charting when `design_step` is unset or `chart`. The planner refuses a map source once the Design has left charting, with the stable refusal code and a message that says the Design has started. The rule moves out of the command and into the planner. The old command-level guard and its spec line are removed.
- Moved tickets keep their follow-up address. `blocked_by` references are rewritten project-wide by the planner as for any move. Stale prose in an emptied epic's Notes is left alone. Claimed, done and cancelled tickets cannot move.
- The archive check accepts a map with `completed_at` stamped, so an emptied charting map can archive. An archive refusal because of a live Design session shows as a failed unit with Retry.
- Stats counts a map as a completed Design only when its `design_step` is `branch` or `done`. Emptied charting maps never count. All completed maps today are at `branch` or `done`, so no history changes.

**Start Design.**
- The Backlog page lists each project's areas with the open ticket count (draft and open tickets) and a Start Design button, disabled at zero. It lives behind a link in the sidebar footer Epics menu, with no sidebar row. It follows the Epics page look, and the dialog follows the New Design dialog (a bottom sheet on a phone). No new mock.
- Start Design opens a small dialog with an optional hint, then starts a plain chat session (not a stepped Flow) running a new skill that takes the area and the hint. The session runs at the worker tier. The agent prints one "Start this Design?" card: a title (checked with `epic next-name`) and 8 to 14 numbered tickets with a one-line reason each. An area with fewer than 8 open tickets proposes all of them. The person edits by typing in the chat and the agent reprints once.
- On yes, a new verb creates the map epic (`kind: map`, `destination_unconfirmed: true`, a provisional Destination, Notes naming the area and the ticket addresses), moves the tickets through the move planner, and prints a link. The Design starts when the person opens it. Tickets are moved, not copied; the area keeps the rest.
- The verb dry-runs the planner for all tickets before it writes. It adds no new field to the epic schema. A charting map whose Notes name this area and list ticket addresses that are still in the area counts as the verb's own partial run, so a rerun carries on and tickets already moved stay in the new map. The Backlog area row shows such a map with a "Finish move" button, which reruns the verb with no new chat.
- The new map's Notes never contain the phrase "Triaged from follow-ups", so a later Triage never takes the map for an old-epic candidate.
- Only one Start Design chat lives per area. A second click opens the live one.

**Old-epic move.**
- The first Triage after the change does it, in its conversation. It is not a `gx tickets migrate` step. A candidate is a charting map, not archived, whose Notes say "Triaged from follow-ups". After every unit is done there are no candidates, so later Triages and Triages in other projects find nothing and skip it. An epic that kept a claimed ticket, or a unit the person kept for later, stays a candidate, and each later Triage plans only its leftover moves or its archive.
- A new verb lists the candidates, each with its tickets to move and whether it can archive. Route calls it, so the rule has one owner. The shipped table is keyed by project and old-epic slug.
- The 12 areas of the gx project are the first list: test-health, check-and-brief, agent-guidance-and-skills, ticket-authoring, tickets-store-and-cli, agent-sessions-and-runners, ralph-loop-and-recovery, epics-queue-and-merge, server-performance-and-structure, design-flow, web-ui, tui.
- The mapping from old epic to area is a table shipped with the skill, built from the inventory research (103 epics, including the 8 that never had tickets). Borderline epics: commitless-close and fork-handling go to ralph-loop-and-recovery, rtk-hook to agent-guidance-and-skills, chat-cards and chat-notifications to web-ui. An epic not in the table goes to the closest area, picked by the agent.
- The four epics past charting (agent-workflow-skills, pause-and-recovery, pause-cleanup, queue-and-epics) are never candidates.
- One move unit per old epic: move its open and draft tickets, then stamp `completed_at` and archive it only when no open, draft or claimed ticket is left. An epic that still holds one is neither stamped nor archived. A candidate with nothing to move that cannot archive (it holds a claimed ticket) makes no unit: the summary shows one line, "waiting: <epic> holds claimed <address>", and it becomes a unit again once it can move or archive. A failed unit shows its error with Retry or Keep for later. A kept unit has the state `kept`, counts as finished in Done detection, stays a candidate and is planned again by the next Triage. Write is safe to run again, so a stopped session carries on with what is left.
- Route plans the move and the summary lists it (tickets per area, epics to archive). The ordinary final card approves it, with no extra card. Write creates an area on first need, then runs the follow-up rows, then the move units, then the last pass. The Write view lists move units as extra rows in the same list, and "x of y rows written" counts them.

**Web.**
- The Triage page reuses the Design page: chat, question cards, Progress, composer. The Questions view, Review view, Approve bar and standing plan panel are removed, with their tests. The plan summary and the final card are chat cards.
- A waiting round puts the card in "Needs you" even with a live session. gx no longer stops an idle Triage session at Questions.
- The look is fixed by the stored mock (see Verification, Mock states). The Follow-ups card ring follows the five steps. The pill is the step name, or "Needs you" (amber) with the round and questions left or "Write this plan?". The button is Continue, Answer, Review plan or Queue by step; "Retry failed (N)" shows beside it when rows failed.
- Write keeps failed rows with Retry and Keep for later. Done keeps the result list with links and the Queue button.
- The sidebar badge keeps the open follow-up count.

**Other.**
- No new pause kind (ADR 0042). Nothing pauses. A waiting round, the final card, a failed unit and a version 1 record at Write all end from the web (answer, Retry, Keep for later, Abandon).
- Glossary edits (the implementation epic makes them): the Triage entry stops saying "area map epic" and names areas; the Stepped session entry stops saying "except where a step needs the person (the Triage plan)" and names the Questions step; the Question sheet entry stops saying the Triage chat calls it the Questions view.
- The earlier Triage UI spec's rules for Plan, Approve, the Review view, the Approve bar and send-back are superseded by this spec. Its card, badge, record-ownership, one-live-Triage, Write-failure and Queue rules stay.

## Testing Decisions

- A good test checks external behaviour: what the record and the verbs accept and refuse, what the page and the card show, and what the ticket store holds after Write. It does not test internal markers or component structure.
- One seam for behaviour: the `triage` fixture scenario, driven by the web page and the `gx` verbs against a real ticket store. Go tests cover the rules gx code enforces, because a flow cannot reach their edges: the move rules, old-record reading, the archive rule, the Stats rule, and the verbs and builders that enforce a rule (save-round, approve, plan save, routing correction, card builder, Start Design create verb, Abandon, and the old-epic candidate-list verb). Rules that only the skill applies are not Go-tested.
- Playwright budget (gx-playwright-e2es): no new flow. Extend "199. Triage journey" with the conversation steps, Backlog and Start Design, and remove its steps for Send back, the Questions view and the Approve bar. Flow 203's Epics menu check gets the Backlog link. The flow count does not change.
- Fixture shapes to write, in the `triage` scenario: `route`, `questions`, `final-approval` (these replace `b-questions` and `b-review`), `backlog` (areas with 11 open tickets, 5 and 0, and a fake agent for the Start Design chat), and `old-epics` (project `gx`, charting maps with the note and slugs taken from the shipped table, one slug not in the table, one past charting, one empty, one with a claimed ticket; it seeds no area, so Write creates the table areas). The `route` and `questions` shapes seed 3 areas, so the new-area card shows all three options. Kept: `verify`, `writing`, `result`, `stopped`, `journey` and the `c-*` shapes.
- Components covered by vitest: the plan summary card, the round header, the Follow-ups card pill and button by step, the Backlog row, the Start Design dialog. Unit tests of removed components are deleted.
- Prior art: the `triage` scenario and Triage journey flow; the Design page flows for stepped sessions, Progress and auto-advance; the New Design dialog flows (26, 41); the tickets move tests; the archive tests; the migration registry tests.

## Verification

Run against a temporary gx server with the `triage` fixture scenario. Verb names are the implementer's, so the checks name the behaviour. Story entries refer to Mock states lines by state name.

Questions (stories 1 to 11)
- Flow: 199. Triage journey, Questions steps (`questions` shape).
- CLI: `gx tickets triage show --json` lists self-decided answers with the objected flag and no `quick` kind; after Write, `gx tickets show` of a ready ticket shows its Decisions marked "decided by agent".
- Go tests: the save-round verb refuses a round of more than 6 questions and a fourth non-objection round; Write files unreached rows under their area (fallback area for a ready row) as `grilling` with the questions in the ticket; an objection is saved as an `ask` question in a round outside the cap; a routing correction to an area drops the row's unanswered questions, and one to the ready epic adds them to the next round with a fallback area, both bumping the revision.
- Skill-applied rules (bugs first, scope-changing first, settled questions dropped) are checked by flow 199 on the `questions` shape, not by Go tests.
- Mock states: b-questions-desktop, b-questions-phone, b-approve-desktop, b-approve-phone.

Conversation and page (stories 12 to 23, 26 to 26)
- Flow: 199. Triage journey on shapes `verify`, `route`, `questions` and `final-approval`: Progress lists the five steps; the summary card appears after Route; a typed correction changes the summary lines; the round header reads "Round 1 of 3"; auto-advance chains Verify to Route and waits at Questions; the idle session is still alive at Questions.
- Phone: 199 part at 390 px has no sideways scroll, the step line opens the steps sheet; flow 24 (Phone, Design page) still passes.
- CLI: `gx tickets triage show` after a session kill reports the open round, and a new session reprints it and asks nothing answered. Approve with a stale revision is refused and a second approve is refused as "already approved". Go test of both.
- Terminal: the skill, run in a plain session against the fixture, prints the same summary and cards as text and writes on yes (walked by hand, no new flow).
- Mock states: b-verify-desktop, b-verify-phone, b-route-desktop, b-route-phone, b-questions-desktop, b-questions-phone, b-approve-desktop, b-approve-phone.

Follow-ups card and badge (stories 24 to 25, 27 to 27)
- Flow: 199. Triage journey checks the card in Needs you with the pill "Needs you" and the button Answer while a round waits (session live), "Write this plan?" and Review plan at the final card, Retry failed (N) on a failed Write, and the badge count unchanged.
- Go test on the card builder covers each step and the live-session-while-waiting case.
- Mock states: card-verify-desktop, card-route-desktop, card-questions-desktop, card-approve-desktop, card-write-desktop, card-done-desktop, card-questions-phone, card-approve-phone, card-write-phone, b-write-desktop, b-write-phone, b-done-desktop, b-done-phone.

Abandon (stories 28 to 28)
- Flow: 199. Abandon at Route changes no ticket, creates no area (`gx tickets epics --areas` unchanged) and the card shows a plain Triage button. Go test that Abandon is offered at Verify, Route, Questions and not at Write.

Areas (stories 29 to 33)
- CLI: `gx tickets epic add <project>:<slug> --kind area --status open` succeeds; a 13th area is refused; `gx server queue add` on an area ticket is refused; `gx tickets set <area> --kind map` is refused; `gx tickets epics --areas` lists areas and `--maps` does not; `gx tickets validate` passes on an area; the migration step `area-epic-kind` runs with no store change.
- Flow: 199 shows the Epics page and the sidebar with no area row.
- Go tests: the Route plan save refuses a plan with a 13th area and a confirmed 13th area; it refuses approval while an `agent` area entry is unconfirmed and accepts a `table` entry.
- Flow: 199 on the `route` shape with 12 areas seeded: no new-area card appears, and the row is filed under the closest area and listed under "Decided unless you object". With no area seeded, the card has two options.
- Flow: 199 on the `route`/`questions` shape: a new-area confirmation card is the first card of round 1 and offers create, file under the closest area, and keep for later; choosing keep for later leaves the rows in `follow-ups`. On the `old-epics` shape the 12 table areas produce no confirmation question.
- New scenario: the `old-epics` and `backlog` shapes seed areas.

Backlog and Start Design (stories 34 to 42)
- Flow: 199, `backlog` shape: the footer Epics menu has a Backlog item and the sidebar has no Backlog row; each area shows its open count; the button is disabled on the empty area; Start Design opens the dialog (flows 26 and 41 are the prior art for the dialog and the phone sheet); the chat prints the "Start this Design?" card; a typed edit reprints once; yes prints a link; the fixture agent drives it.
- CLI: after yes, `gx tickets show` of the new map shows `kind: map`, `destination_unconfirmed: true` and the tickets with their follow-up addresses; the area holds the rest; a second Start Design click opens the live chat. A Triage run after Start Design does not list the new map as an old-epic candidate.
- Go tests: the create verb dry-runs for all tickets before writing; a forced mid-run failure leaves moved tickets in the map and a rerun finishes; the Notes it writes never contain "Triaged from follow-ups".
- Flow: 199, `backlog` shape: an area with a half-filled map shows "Finish move", and it completes the move with no new chat. The "propose all under 8 tickets" rule is the skill's, checked here.
- New scenario: the `backlog` shape.

Old-epic move (stories 43 to 52)
- Flow: 199, `old-epics` shape: the Route summary lists the move; after the final card Write moves the open tickets, the Write view lists move units, and the emptied epics are archived.
- CLI: `gx tickets show` of an emptied epic shows `completed_at` and only done tickets; the four past-charting epics are untouched; the claimed ticket stayed and its epic is not stamped or archived; moved tickets keep their follow-up address; a second Triage shows the claimed-ticket epic as a "waiting" line and makes no unit for it; `gx tickets move` out of the past-charting map in the `old-epics` shape exits non-zero with the `move-map-source` code and the "Design has started" message.
- Go tests: the candidate-list verb leaves out past-charting, archived, no-note and Start Design maps, and lists a claimed-ticket epic with nothing to move and no archive; a kept unit stays a candidate, counts as finished in Done detection, and a later Triage plans it again.
- Go tests: the move planner moves from a charting map and an area, refuses a map past charting with the stable code, refuses claimed and done tickets, and rewrites `blocked_by` project-wide; the archive check accepts a map with `completed_at` and refuses on a live Design session; the Stats builder counts a map only at `branch` or `done`; a failed unit keeps its error and Retry reruns it.

Records (stories 53 to 55)
- Go tests: the loader reads a done, abandoned and live version 1 record; a live one at Verify continues with `quick` read as `ask`; one at Plan or Approve goes back to Route keeping verdicts and answers; one at Write offers only Abandon and reads "Waiting for you".
- Flow: the pause-kind table test (ADR 0042) is unchanged; Design flows 50, 26 and 27 and the phone Design flows still pass.

**Mock states** (the stored mock, `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`; the mock holds 12 page states and 9 card states):
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-verify-desktop, 1120, scenario: triage shape `verify`, parts: sidebar, page-header, ring, chat, working-row, composer, progress
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-verify-phone, 390, scenario: triage shape `verify`, parts: page-header, ring, step-line, chat, working-row, composer
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-route-desktop, 1120, scenario: triage shape `route`, parts: sidebar, page-header, ring, chat, plan-summary, working-row, composer, progress
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-route-phone, 390, scenario: triage shape `route`, parts: page-header, ring, step-line, chat, plan-summary, working-row, composer
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-questions-desktop, 1120, scenario: triage shape `questions`, parts: sidebar, page-header, ring, chat, answered-question, question-card, composer, progress
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-questions-phone, 390, scenario: triage shape `questions`, parts: page-header, ring, step-line, chat, answered-question, question-card, composer
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-approve-desktop, 1120, scenario: triage shape `final-approval`, parts: sidebar, page-header, ring, chat, plan-summary, write-card, composer, progress
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-approve-phone, 390, scenario: triage shape `final-approval`, parts: page-header, ring, step-line, chat, plan-summary, write-card, composer
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-write-desktop, 1120, scenario: triage shape `writing` (two failed rows), parts: sidebar, page-header, ring, chat, write-view, composer, progress
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-write-phone, 390, scenario: triage shape `writing` (two failed rows), parts: page-header, ring, step-line, chat, write-view, composer
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-done-desktop, 1120, scenario: triage shape `result`, parts: sidebar, page-header, ring, chat, done-view, composer, progress
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, b-done-phone, 390, scenario: triage shape `result`, parts: page-header, ring, step-line, chat, done-view, composer
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-verify-desktop, 420, scenario: triage shape `verify`, parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-route-desktop, 420, scenario: triage shape `route`, parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-questions-desktop, 420, scenario: triage shape `questions`, parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-approve-desktop, 420, scenario: triage shape `final-approval`, parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-write-desktop, 420, scenario: triage shape `writing` (two failed rows), parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-done-desktop, 420, scenario: triage shape `result`, parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-questions-phone, 390, scenario: triage shape `questions`, parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-approve-phone, 390, scenario: triage shape `final-approval`, parts: followups-card
- `$(gx tickets root)/better-triage/mocks/06-conversation-page-look.html`, card-write-phone, 390, scenario: triage shape `writing` (two failed rows), parts: followups-card

New flows or scenarios to write (these become implementation tickets): the fixture shapes `route`, `questions`, `final-approval`, `backlog` and `old-epics`; the extended steps of flow 199; the Backlog link check in flow 203. Shapes `route`, `questions` and `final-approval` are new for the Mock states lines above.

## Out of Scope

- The TUI: it has no Triage view.
- A Triage that runs with no person (scheduled or automatic).
- The ready epic: still one `follow-ups-<date>` epic per Triage, queued by the person.
- Delete, rename or merge of areas in the web or a verb.
- A `gx tickets migrate` step for the old-epic move.
- A new mock for the Backlog page or the Start Design dialog.
- Changing the routing of close and merge rows.

## Further Notes

- The decisions behind this spec are in the `gx:better-triage` map, tickets 01 to 09. The inventory of the old area map epics, with the per-epic table the mapping is built from, is in `docs/research/better-triage-02-area-epic-inventory.md`.
- The stored mock is the approved variant B. The unapproved variants A and C are not stored and not listed.

## Self-decided design questions

1. Where do areas show? Choice: only on the Backlog page and in `gx tickets epics --areas`; the Epics page, sidebar, Archive action and Stats leave them out. Reason: an area is neither loop nor map, and the map said it has no sidebar row.
2. How does an agent list a project's areas? Choice: a new `--areas` filter on `gx tickets epics`, beside `--maps`, with `--maps` excluding areas. Reason: reading each epic's kind by hand is the workaround the tracker contract avoids.
3. Can two Start Design chats run on one area? Choice: no, one live chat per area; a second click opens it. Reason: two chats would pick the same tickets and the second move would fail.
4. Which projects does the Backlog page list? Choice: the projects of the active workspace, grouped by project, like the Epics page. Reason: the Epics page and the sidebar badge already scope by workspace.
5. What counts in the area's "open ticket count"? Choice: draft and open tickets, not claimed. Reason: those are the ones a move can take.
6. Does Questions run when no `ask` question is left? Choice: yes, it holds the "Decided unless you object" list and the final card. Reason: the final approval lives in Questions, and the step list stays fixed.
7. What if the person declines a proposed new area? Choice: the confirmation question has three options (create, file under the closest named area, keep for later), so a decline gives a concrete target. Reason: Keep for later alone made every later Triage propose the same area again. This was settled with the person in review.
8. What does the Start Design chat look like in the web? Choice: a normal chat session on the Agents page that the dialog's Start opens, running at the worker tier. Reason: it is a plain chat, not a stepped Flow, so it needs no new page.
