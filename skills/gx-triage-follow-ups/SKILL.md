---
name: gx-triage-follow-ups
description: >-
  Empty a project's `follow-ups` epic: verify each item against the code, route every item to the
  ready epic or a backlog, ask the person only what they must decide, and close what is fixed or
  obsolete. Manual; the steps are Verify, Route, Questions, Write and Done. Runs under the gx server
  (web) or in a plain terminal (chat). Never grills outside the Questions step.
disable-model-invocation: true
---

# gx Resolve Follow-ups

Empty `<project>:follow-ups` until it holds no tickets. The project is the current one, or the one
named by `--project <name>`. Read [gx-local-tracker.md](../gx-local-tracker.md) for the ticket
commands and the Follow-ups format. Run `gx tickets triage --help` and each verb's `--help` for the
Triage record verbs; don't restate their syntax here.

You are the main session. Subagents verify; you route; the person answers in Questions and approves
once; then you write. This skill never grills outside the Questions step.

## Prefer the correct fix

ralph-loop builds the fixes without a person watching, so a slower fix costs agent time, not the
person's time. When a choice is between a quick fix (a workaround, a special case, a patch on the
symptom) and a slower fix that is clean and correct (it fixes the cause, or puts the logic where it
belongs), pick the correct fix. This decides every `decide` question and every recommended answer,
and it applies to every suggested fix from Verify and every ticket sent to the ready epic. Pick the
quick fix only when the person asks for it, and then record the reason in the ticket body.

## 0. Link and detect the step

1. Run `gx agent link --triage <project>`. Success means **web mode**: the server shows the record
   and its cards. Failure means **chat mode**: you print the same summary and cards as text in this
   terminal and read the person's replies. The rules and the record are the same in both.
2. Detect the step with the record, not the conversation. Run `gx tickets triage show --project
   <project>`:
   - No live Triage → create one (`gx tickets triage create`), then Verify.
   - Otherwise the first match: rows with no verdict → **Verify**; no plan saved → **Route**; plan
     saved, not approved → **Questions**; approved → **Write**; every row written → **Done**.
   - A live Triage found from a plain session is continued, never duplicated.
   - A fresh session in Route or Questions runs `gx tickets triage open-round` and reprints the open
     round, so it continues without asking again.
3. Report each step with `gx agent step <verify|route|questions|write|done>` when it starts.

Each step ends with one line, `✅ <step>: <what was done>. Next: <what runs next>`. Before chaining
into the next step in the same session, run `gx agent context-window --fits <step>` for that next
step; if it says no, end with the `✅` line and a Next of `/clear`, then `/gx-triage-follow-ups
--project <project>`. Route chains on from Verify and from itself into Questions. Questions waits for
the person and has no countdown: a step that waits ends with its printed round, never with a tool
call. Write chains from the approval. Abandon (`gx tickets triage abandon`) is offered at Verify,
Route and Questions, never at Write.

## 1. Verify

`gx tickets triage pending` lists the rows with no verdict; a fresh session verifies only those.

Start one subagent (Agent tool) per row in one turn, then wait for all of them (`general-purpose`,
model from `gx agent model worker`; a second word is its `reasoning_effort`, see
[Skill conventions](../gx-local-tracker.md#skill-conventions)), about 8 in parallel. Subagents verify
and write nothing. Each gets the ticket address and this brief:

"Read the ticket with `gx tickets show`. Read the code and logs it cites and `git log` since it was
filed. Do not reproduce it and do not run tests. When a quick fix and a clean, correct fix both
exist, suggest the correct one. Report: (1) verdict `relevant`, `fixed` or `obsolete`, with evidence
(a commit, a file and line, a log line); (2) the files a fix would touch; (3) 0–N blocking
questions. Tag each question with one of `decide` (you pick), `ask` (the person picks from options) or
`design` (an open trade-off). Every question has 2–4 concrete options and a recommended answer. A
question is `ask` when its answer changes scope, a user-visible behaviour or contract (UI, CLI, a
config key, a skill rule), routing (ready epic or backlog), or is expensive to reverse; otherwise it is
`decide`. (4) a suggested destination: the ready epic as `implement`, or a backlog as `grilling` or
`research`. If you cannot decide, say `relevant`. Under 300 words."

The main session may overrule a tag. You save each verdict the moment its subagent returns, with
`gx tickets triage verdict`. An undecided verdict counts as **relevant**. A follow-up already closed
or moved is saved with `--gone`. When the last verdict is saved, chain to Route.

## 2. Route

Route writes no epics and no tickets; it saves the plan rows (`gx tickets triage plan`, rows as JSON
on stdin). Compare verdicts only; don't re-read tickets. Work one group at a time; Route ends when the
plan is saved.

**Dedupe is part of Route.** For items describing the same problem, keep the **oldest**. The newer
ones are `merge into X` rows; the merge itself (the "seen again" entry, the close) waits for Write.

One row per item with its action and destination:

- **close (reason)** — `fixed` or `obsolete`, with the evidence.
- **merge into X** — a duplicate of X.
- **ready epic** — `follow-ups-<date>` (Write may add a `-2` suffix), as `implement`.
- **backlog Y** — as `grilling` or `research`.

Routing, first match wins:

1. Any `design` question → backlog, as `grilling`.
2. Needs investigation but no decision → backlog, as `research`: set `"investigate": true` on the plan row.
3. Otherwise → ready epic, as `implement`, with its `ask` questions answered in Questions.

**Backlogs.** Use an existing backlog. Backlogs in the shipped [mapping table](old-epic-backlogs.txt) need no
question. Propose a new backlog only when none fits, and confirm it with the first question of the round
(`gx tickets triage backlog`): three options (create it, file under the closest named backlog, keep for
later), or two options when no backlog exists. At 12 backlogs make no proposal and ask no question: file
under the closest backlog and list it under "Decided unless you object". Backlogs are not map epics.

**Old epics.** Call `gx tickets triage oldepics` once and plan the moves (`gx tickets triage moves`):
the tickets per backlog, the epics to archive, and a "waiting: <epic> holds claimed <address>" line for
each epic that still holds a claimed ticket.

Record which ready-epic items touch overlapping files (`overlapWith`); they get chained by
`blocked_by` in Write, oldest first. Save an `estimate` (such as `10K`) for every ready-epic row,
taken from `gx tickets verify-plan --table` evidence. Ready rows that overlap are grouped when their
estimates sum to under 45K; a row without an estimate is never grouped, and a backlog row is never
grouped. `gx tickets triage row --ungroup` undoes a group.

When the plan is saved, print the summary card: counts per action and per backlog. Print a group's
follow-ups only on request. A typed correction is saved right after the reply
(`gx tickets triage correct`) and the changed summary lines are reprinted. Routing corrections:
- to a backlog: drop that row's unanswered questions;
- to the ready epic: add its `ask` questions to the next round and pick a fallback backlog at once.

Then chain to Questions.

## 3. Questions

Questions always runs after Route, because it hosts the final approval. With no `ask` question it
shows only the "Decided unless you object" list and the final card.

**Rounds.** At most 6 questions per round and at most 3 rounds. Order: bugs (`category: bug`) before
ideas, scope-changing questions first within each, a new-backlog question first of all. Group by
follow-up and drop settled questions. Save the round before it prints (`gx tickets triage round`);
gx refuses more than 6 questions and a fourth non-objection round, but the order and the dropping are
yours. The person's answers are saved with `gx tickets triage answer`. The round is printed as its
last act: end the turn there.

**Decided unless you object.** After each round and in the final summary, print the self-decided
answers per follow-up (save each with `gx tickets triage decide`). An objection becomes an `ask`
question in its own round (`gx tickets triage object`, `round --objection`), outside the cap, at most
6, started by the person typing it.

A follow-up no round reached goes to its backlog as a `grilling` ticket with its questions in the ticket.

**Final card.** "Write this plan?" On yes call `gx tickets triage approve --revision <n>` and run Write
in this same session. After a stale refusal, reprint the summary and ask again. If it prints "already
approved", report it was approved elsewhere and stop.

## 4. Write

Write takes the approved plan as final and never re-plans. Report the step with `gx agent step
write`. Name the ready epic first. If `gx tickets triage show --json` already has a `readyEpic` (a
re-run), use it. Otherwise run `gx tickets epic next-name <project>:follow-ups-<date>`, with today's
date. It prints `follow-ups-<date>` when that name is free, else the next free one
(`follow-ups-<date>-2`, `-3`, …). Each Triage makes its own ready epic and never joins another one.
Record the epics (`gx tickets triage epics`), then run each row with the tracker's verbs. Before each
action ask `gx tickets triage locate` (for a move unit, `move-locate`), and do only what it says is
not yet done, so a re-run after a `/clear` repeats nothing.

Order: create a backlog on first need (`gx tickets triage backlog`), then the follow-up rows, then the
old-epic move units (`gx tickets triage move-state` saves each result), then the last pass.

Per row, in this order:

1. The answers: `gx tickets triage answers`. Each self-decided answer goes into the ticket's
   Decisions marked "decided by agent".
2. For a merge, the seen-again note: `gx tickets triage seen-again`.
3. `gx tickets move <addr> <epic>` or `gx tickets close <addr> --reason fixed|obsolete|duplicate|not-worth-it|promoted
   --note ...`. A move renumbers the ticket. Capture the printed address, use it for every later
   command and save it with `--new-address`.
4. `gx tickets set <new-addr> --type ... --status open`. Steps 3 and 4 are for backlog and close rows;
   ready-epic rows are published (below). Only `implement` tickets go to the ready epic: the queue
   does not run `research` tickets.

**Ready epic.** `implement` rows are not moved one by one. After every row's answers are saved, publish
the whole ready epic once, unattended, through [gx-to-tickets](../gx-to-tickets/SKILL.md) in its "no
spec" mode. It never quizzes. The input is the follow-up bodies: the brief (`Suggested fix`) with the
verify evidence and the answers. Build one plan, run `draft`, the writers (when 4 or more),
`lint-claims` and one `publish`. `gx-to-tickets` creates the epic as a draft, adds the code-review
ticket and opens the epic last. This skill never runs `epic add` for it and never queues it.

Each manifest entry carries `from: <address of the follow-up>`, a string, or a list for a group. A group
ticket has one acceptance-criteria group per follow-up and a `blocked_by` that is the union of its
members' chain edges: the union of their `overlapWith` in `gx tickets triage show --json`, minus the
members themselves. `gx-to-tickets` has the final say on slicing: a group too big at publish is split,
and the splits chain by `blocked_by`.

**Row loop.** Run `lint-claims` and `publish --dry-run`. Map each refusal to its row through the
entry's `from:`, call `gx tickets triage drop-rows`, and retry at most once per row. Each dropped row
is saved `failed` with the refusal text and its follow-up stays in `follow-ups`; the other rows
publish. A group is all or nothing, like a split row. A refusal that names no entry (a manifest parse
error, a plan error, or a failure after the dry-run passed) saves every pending row `failed` with the
text and stops the loop with no retry.

After `publish`, save each published row's new address with `gx tickets triage write-state
--new-address` (a list for a split or group). `write-state --state done` is refused unless every new
ticket exists and the follow-up is archived: check `gx tickets triage locate` first, and save `failed`
instead.

A backlog ticket gets a `## Question` built from its open questions (`gx tickets section <new-addr>
Question -`). Save each row's result with `gx tickets triage write-state`. One failed row never stops
the others: save it `failed` with the verb's refusal text unchanged.

After every row, run `gx tickets close --sweep [project]`, then `gx tickets triage last-pass`, and
`gx tickets validate` one ticket of the new ready epic.

## 5. Done

**Done** = `<project>:follow-ups` holds no tickets. Run `gx tickets show <project>:follow-ups` and
report what is left, if anything; a ticket the user held back stays and the run isn't done. Report
the step with `gx agent step done`.

Finish with `✅ done: ...`: counts per action, the ready epic's address, each backlog's tickets, and any
failed rows. Finish with zero background shells.
