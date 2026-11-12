---
name: gx-triage-follow-ups
description: >-
  Empty a project's `follow-ups` epic: verify each item against the code, dedupe, route every item
  to a ready epic or an area map epic, and close what is fixed or obsolete. Manual; one approval of
  one table. Runs under the gx server (web) or in a plain terminal (chat). Never grills.
disable-model-invocation: true
---

# gx Resolve Follow-ups

Empty `<project>:follow-ups` until it holds no tickets. The project is the current one, or the one
named by `--project <name>`. Read [gx-local-tracker.md](../gx-local-tracker.md) for the ticket
commands, the Follow-ups format, the Map epics section and the Triage record verbs
(`gx tickets triage`); don't restate their syntax here.

You are the main session. Subagents verify; you plan; the user approves once; then you write. This
skill never grills: a design question is routed to a map epic, not asked.

## Prefer the correct fix

ralph-loop builds the fixes without a person watching, so a slower fix costs agent time, not the
person's time. When a choice is between a quick fix (a workaround, a special case, a patch on the
symptom) and a slower fix that is clean and correct (it fixes the cause, or puts the logic where it
belongs), pick the correct fix. This applies to every suggested fix from Verify, every default
answer to a `quick` question, and every ticket sent to the ready epic. Pick the quick fix only when
the person asks for it, and then record the reason in the ticket body.

## 0. Link and detect the step

1. Run `gx agent link --triage <project>`. Success means **web mode**: the server shows the record
   and the person approves in the web plan view. Failure means **chat mode**: you talk to the person
   in this terminal. The routing rules and the record are the same in both.
2. Detect the step with the record, not the conversation. Run `gx tickets triage show --project
   <project>`:
   - No live Triage → create one (`gx tickets triage create`), then Verify.
   - Otherwise the first match: rows with no verdict → **Verify**; **Plan**; **Approve** (plan
     saved, not approved); **Write** (approved); **Done**.
   - A live Triage found from a plain session is continued, never duplicated.
3. Report the step with `gx agent step <verify|plan|write|done>` when it starts.

Each step ends with one line, `✅ <step>: <what was done>. Next: <what runs next>`. When more work
is left, the Next is: `/clear`, then `/gx-triage-follow-ups --project <project>`. Never chain a step
into the next one in the same session except where a step below says so.

## 1. Verify

`gx tickets triage pending` lists the rows with no verdict; a fresh session verifies only those.

Start one subagent (Agent tool) per row in one turn, then wait for all of them (`general-purpose`,
model from `gx agent model worker`; a second word is its `reasoning_effort`, see
[Skill conventions](../gx-local-tracker.md#skill-conventions)), about 8 in parallel. Subagents verify
and write nothing. Each gets the ticket address and this brief:

"Read the ticket with `gx tickets show`. Read the code and logs it cites and `git log` since it was
filed. Do not reproduce it and do not run tests. When a quick fix and a clean, correct fix both
exist, suggest the correct one. Report: (1) verdict `relevant`, `fixed` or
`obsolete`, with evidence (a commit, a file and line, a log line); (2) the files a fix would touch;
(3) 0–N blocking questions, each tagged `quick` (a pick from 2–4 concrete options: give the options
and which one you recommend) or `design` (a decision with trade-offs); (4) a suggested destination: ready epic as `implement`, or map epic as
`grilling` or `research`. If you cannot decide, say `relevant`. Under 300 words."

You save each verdict the moment its subagent returns, with `gx tickets triage verdict`. An undecided
verdict counts as **relevant**. A follow-up already closed or moved is saved with `--gone`.

Before starting another batch, run `gx agent context-window --fits verify`. If it says no, end the
run with the `✅ verify:` line. When the last verdict is saved, end the same way, pointing at Plan.

## 2. Plan

Plan writes no epics and no tickets; it saves the plan rows (`gx tickets triage plan`, rows as JSON
on stdin). Compare verdicts only; don't re-read tickets.

**A later round.** When `gx tickets triage show` prints `round=N`, or its `--json` a `round` above 0, the
person sent their answers back and wants new questions. Read `gx tickets triage show --json`: every row has lost its plan and its verdict questions, and
its `earlier` list holds what they were asked and answered (a note follows the answer as `Note: …`; the
"Huh?" message means they could not follow the question). Plan again from those answers, still
without re-reading tickets:

- Act on the answers: a pick settles that question, so do not ask it again; a note or a "Huh?" means
  the question was unclear or incomplete, so ask a better one. Reroute the row when an answer
  changes where it belongs. Keep the person's own action and destination (`person` in the record)
  unless an answer clearly overrides it.
- Ask only what is still undecided: a row with nothing left to ask gets no questions. The new
  questions are ordinary plan questions (the options and default rules below), numbered from `q1`.
- Save every row that is not gone, as in the first round. The earlier answers still go into each
  ticket's Decisions at Write.

**Dedupe is part of Plan.** For items describing the same problem, keep the **oldest**. The newer
ones are `merge into X` rows; the merge itself (the "seen again" entry, the close) waits for Write.

**Questions are for a person to answer one by one in the web view.** The verdict's questions are raw
findings. In the plan, every one comes back under the same id as the final question:

- A `quick` question is a plain question that stands alone, with no recommendation in its text, then
  `options` (2–4 concrete answers, short enough to read as buttons) and `default`, the recommended option
  and one of the options. A yes or no question has the options `yes` and `no`. The first line of `text`
  is the question; further lines say what the fix changes, so it can be answered without opening the
  ticket.
- A `design` question needs no options or default.
- A low-risk `quick` question is decided, not asked: add `"auto": true` to it. Plan saves it
  answered with its `default`, and the person can still change it in the Questions view. It is
  low-risk only when all of these hold: its answer changes no scope and no destination of the row
  (a ready epic stays ready), it is cheap to reverse, and it has one clear answer. When in doubt,
  leave `auto` off. A `design` question is never auto.
- `gx tickets triage plan` refuses a quick question without options and a default, and a verdict
  question that is missing from the plan.

One row per item with its action and suggested destination:

- **close (reason)** — `fixed` or `obsolete`, with the evidence.
- **merge into X** — a duplicate of X.
- **ready epic** — `follow-ups-<date>`, as `implement`.
- **map epic Y** — as `grilling` or `research`, with the area slug Y.

Routing, first match wins:

1. Any `design` question, or more than 2 questions → map epic, as `grilling`.
2. Needs investigation but no decision → map epic, as `research`.
3. Otherwise → ready epic, as `implement`.

Group map-epic items by **area of code or tool**, not bug vs idea: 1–8 items per epic, a singleton is
fine, plain descriptive slug. For the ready epic, record which items touch overlapping files
(`overlapWith`); they get chained by `blocked_by` in Write.

**Web mode** ends here: end with `✅ plan: ...` and a Next line pointing at the web plan view, where
the person edits and approves.

**Chat mode** continues in this session:

1. Print the table.
2. Ask every `quick` question of the ready-epic items in one message. Make each answerable on its
   own, grouped by ticket: its address and title, one or two lines on what the fix changes, then each
   question with the options and a default. Never ask a `design` question: it travels with its ticket
   into the map epic.
3. Save answers and the person's edits with `gx tickets triage row`. A stale refusal: reprint the
   table and ask again.
4. On "yes", call `gx tickets triage approve --revision <n>` and run Write in this same session. If it
   prints "already approved", report it was approved elsewhere and stop.

## 3. Approve

A saved, unapproved plan. Web mode: end with a `✅ approve:` line pointing at the web plan view; the
person approves there. Chat mode: print the table and ask for "yes" as in Plan, then approve and run
Write. Do not write anything before approval.

## 4. Write

Write takes the approved plan as final and never re-plans. Report the step with `gx agent step
write`. Record the epics first (`gx tickets triage epics`), then run each row in order with the
tracker's verbs. Before each action ask `gx tickets triage locate`, and do only what it says is not
yet done, so a re-run after a `/clear` repeats nothing.

Per row, in this order:

1. The answers: `gx tickets triage answers`.
2. For a merge, the seen-again note: `gx tickets triage seen-again`.
3. `gx tickets move <addr> <epic>` or `gx tickets close <addr> --reason fixed|obsolete|duplicate|not-worth-it|promoted
   --note ...`. A move renumbers the ticket. Capture the printed address, use it for every later
   command and save it with `--new-address`.
4. `gx tickets set <new-addr> --type ... --status open`. Steps 3 and 4 are for map-epic and close
   rows; ready-epic rows are published (below). Only `implement` tickets go to the ready epic: the
   queue does not run `research` tickets.

**Ready epic.** `implement` rows are not moved one by one. After every row's answers are saved
(`gx tickets triage answers` runs first, so the answers sit in the body's Decisions), publish the
whole ready epic `<project>:follow-ups-<date>` once, unattended, through
[gx-to-tickets](../gx-to-tickets/SKILL.md) in its "no spec" mode. It never quizzes. The input is the
follow-up bodies: the brief (`Suggested fix`) with the verify evidence and the answers. Build one
plan, run `draft`, the writers (when 4 or more), `lint-claims` and one `publish`.
`gx-to-tickets` creates the epic as a draft, adds the code-review ticket and opens the epic last.
This skill never runs `epic add` for it and never queues it.

Each manifest entry carries `from: <address of the follow-up>`, a string, or a list for a group. A group
ticket has one acceptance-criteria group per follow-up and a `blocked_by` that is the union of its
members' chain edges (the overlapping-files chain from Plan). `gx-to-tickets` has the final say on
slicing: a group too big at publish is split, and the splits chain by `blocked_by`.

**Row loop.** Run `lint-claims` and `publish --dry-run`. Map each refusal to its row through the
entry's `from:`, call `gx tickets triage drop-rows`, and retry at most once per row. Each dropped row
is saved `failed` with the refusal text and its follow-up stays in `follow-ups`; the other rows
publish. A group is all or nothing, like a split row. A refusal that names no entry (a manifest parse
error, a plan error, or a failure after the dry-run passed) saves every pending row `failed` with the
text and stops the loop with no retry.

After `publish`, save each published row's new address with `gx tickets triage write-state
--new-address` (a list for a split or group).

Area map epics are `gx tickets epic add <project>:<slug> --kind map
--status open --body -`, the body opening with a frontmatter block carrying
`destination_unconfirmed: true`, then a **Destination** (provisional: "decide these items, then a spec
`docs/specs/<slug>.md`"), **Notes** ("Triaged from follow-ups on `<date>`" and the source
addresses), and empty **Decisions so far** and **Not yet specified**. A design item gets a
`## Question` built from its Open questions (`gx tickets section <new-addr> Question -`).

Save each row's result with `gx tickets triage write-state`. One failed row never stops the others:
save it `failed` with the verb's refusal text unchanged.

Map epics get no code-review ticket. After every row, run `gx tickets close --sweep [project]`, then
`gx tickets triage last-pass`, and `gx tickets validate` one ticket of each new epic.

## 5. Done

**Done** = `<project>:follow-ups` holds no tickets. Run `gx tickets show <project>:follow-ups` and
report what is left, if anything; a ticket the user held back stays and the run isn't done. Report
the step with `gx agent step done`.

Finish with `✅ done: ...`: counts per action, the ready epic's address, each map epic's address, and
any failed rows. Finish with zero background shells.
