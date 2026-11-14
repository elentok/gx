## Problem Statement

Turning a spec into published tickets (the create-tickets step of a Design, `gx-to-tickets`) takes too long. A baseline of 20 traced design runs gives a median of 22.4 minutes from the start of the step to the opened `-impl` epic. Two parts of that time are separate problems.

- Agent time (model output, tools, subagents) has a median of 10.9 minutes. In the 13 runs where a subagent explores the code and drafts the breakdown and manifest, that subagent is about 90% of agent time (median 12.4 minutes), limited by output tokens (about 120 tokens per second). In the 7 runs where the main session writes the manifest, agent time is 3.9 minutes, and fix loops after `lint-claims` and `publish` take about 1 minute, each turn re-reading 120K to 160K tokens.
- The quiz wait is 58% of all wall-clock time. 19 of 20 runs stopped to ask the person to approve the breakdown, even when the spec already named the slices.
- `skills/gx-to-tickets/SKILL.md` is 528 lines, and the agent loads all of it on every run, including the rarely used parts.
- gx-triage-follow-ups moves each ready follow-up unchanged into the ready epic. The resulting `implement` tickets have no test seams, no "Where to look" map and no estimate, and small follow-ups that touch the same files each become their own ticket.

## Solution

The step gets faster and no `publish` or `lint-claims` rule is relaxed.

- A new command, `gx tickets draft`, builds every ticket body except the two parts that need judgment. The agent writes a short plan file. The command finds the facts (paths, symbols, callers, consumers, test environment) and prints them into the bodies. The agent writes only "What to build" and the acceptance criteria.
- For a breakdown of 4 or more tickets, writer subagents write those two parts in parallel, one file group per writer. They also explore the code, so the separate "where to look" draft subagent goes away.
- The quiz is skipped by default. It runs only when one of four triggers fires. After a skip, the agent prints the breakdown and Summary and opens the epic, and the person can still correct it before launching from the Queue tab.
- `SKILL.md` shrinks. Rarely used parts (wide refactors, prefactor tickets) move into linked files that load on demand. The step 4c claim checks move into the draft command.
- gx-triage-follow-ups publishes its ready-epic `implement` rows through gx-to-tickets at Write, once, unattended. Each new ticket links to its follow-up through a manifest `from:` field, and `publish` archives the follow-up as `promoted`. Triage Plan groups small follow-ups that overlap in files into one ticket.

Targets, measured with the trace script on runs after the epic lands: median agent time at most 5 minutes, and a quiz in at most 25% of runs. The quiz target is the share of runs that stop to ask, not the person's response time. The triage change is not a speed fix and counts toward neither target.

## User Stories

Draft command

1. As a design agent, I want to give `gx tickets draft` a short plan file with one entry per ticket (slug, title, blockers, window, entry symbols, test files, mock states, `changed_symbols`, `changed_fields`), so that I do not hand-write boilerplate sections.
2. As a design agent, I want the command to write a manifest and one body file per ticket next to it, so that I can hand body files to writers and `publish` reads them as it does today.
3. As a design agent, I want each body to arrive with its header, `## Test seams`, `## Where to look` and `## Mock check` filled in, so that I only write `## What to build` and `## Acceptance criteria`.
4. As a design agent, I want `## What to build` and `## Acceptance criteria` to hold `<!-- TODO -->` markers, so that I know exactly what is left to write.
5. As a person, I want `lint-claims` and `publish` to refuse a body that still has a marker, so that an unfinished ticket never reaches the queue.
6. As a design agent, I want the command to check that paths, symbols, `-run` patterns and flow ids exist, so that a wrong name is printed with a non-zero exit, after the body files and the manifest are written.
7. As a design agent, I want caller lists from `gopls references` stamped `verified at {{commit}}`, and consumers of a changed field from `rg`, so that I do not run those checks one claim at a time.
8. As a design agent, I want the command to report each seam file's test environment, the Makefile targets, the highest migration number, helper prerequisites and the Playwright `-g` check, as lines in the body, so that the facts are visible to the writer and the reviewer.
9. As a design agent, I want the checks that need judgment (does "X shows or hides Y" hold, which side wins when code and spec disagree, signature fallout, examples checked against a helper) to stay mine, so that the command never guesses.
10. As a design agent, I want `draft` to be able to append to the same manifest as `verify-plan`, so that implement and verify tickets publish together.
11. As a design agent, I want a re-run of `draft` to regenerate only the command-owned sections and keep my text in `## What to build` and `## Acceptance criteria`, so that fixing a plan does not cost me my writing.
12. As a design agent, I want a re-run to delete the body file of a slug I removed from the plan, so that no stale body is published.
13. As a maintainer of the `ticket-authoring` map, I want the plan to accept `changed_symbols` and `changed_fields` lists and the command to print the facts this spec lists for them into "Where to look", so that its tickets 01 and 08 can add checks to the same command later without undoing the time saved here.
14. As a person, I want `publish` and `lint-claims` to relax no rule, so that faster authoring does not lower ticket quality. Story 5 adds one refusal (a marker still in a body).

Parallel writers

15. As a person, I want a breakdown of 4 or more agent-written tickets to be written by writer subagents in parallel, so that the writing takes about as long as the slowest writer.
16. As a design agent with fewer than 4 agent-written tickets, I want to write all bodies myself in one pass, so that I do not pay for subagents that save nothing.
17. As a design agent, I want writers = min(3, ticket count) on both Claude and Codex, with tickets split evenly in manifest order and started in one wave, so that the behaviour is the same on both harnesses.
18. As a design agent, I want writers never to edit slugs or `blocked_by`, and to report a missing edge back to me, so that the edges stay consistent with my skeleton.
19. As a person, I want a missing edge such as "a ticket whose Mock check names a scenario must block on the ticket that builds the scenario" to be reported by a writer and fixed by the main agent, so that the order of work stays correct.
20. As a person, I want to see a line when the writers start (which tickets each one has) and a line as each writer finishes, so that I know what is going on while they run.
21. As a person, I want each writer to append one-line progress notes to its own log file next to the manifest, so that a slow run can be inspected.
22. As a design agent, I want writers to explore the code themselves, so that the separate "where to look" subagent is not needed.

Quiz skip

23. As a person, I want the step 5 quiz skipped by default, so that I do not wait for a question when the spec already settles the breakdown.
24. As a person, I want the quiz to run when any one trigger fires (a ticket estimate over 45K tokens, a prefactor or wide-refactor ticket, a "consider splitting" line from `publish --dry-run`, or a spec that does not name the slices), so that a risky breakdown still gets my review.
25. As a person, I want after a skip the breakdown as a table (title, blocked by, estimate, test seam files), the epic Summary, and the line "quiz skipped: no trigger fired" (or the trigger that fired), so that I see what was decided.
26. As a person, I want to correct a skipped breakdown in chat, or in a fresh run, before I launch `<epic>-impl` from the Queue tab, so that I keep an approval point without a quiz.
27. As a person running unattended (ralph-loop, triage Write), I want the quiz always skipped, as today.
28. As a person, I want the quiz test to live as one list of triggers in `gx-to-tickets`, and `gx-design` step 7 to pass nothing extra, so that Claude and Codex runs behave alike.

Skill size

29. As an agent, I want rarely used parts of `gx-to-tickets` (wide refactors, prefactor tickets) in linked files that load on demand, so that a normal run reads less and each turn costs less.
30. As an agent, I want the step 4c claim checks done by the draft command, so that the skill text for them is one pointer to the command. The draft command is the helper for batched claim checks; no separate `rg` helper is added.
31. As a design agent, I want step 7 of `gx-design` to run `gx-to-tickets` inline, so that I keep the design context and can hand a quiz back to the person without a hand-over.

Triage

32. As a person, I want gx-triage-follow-ups to publish its ready-epic `implement` rows through `gx-to-tickets` at Write, after approval, once for the whole ready epic, so that every ticket gets test seams, a "Where to look" map and an estimate.
33. As a person, I want that run unattended, using the draft command and the quiz skip, so that approval of the plan is the only question.
34. As a triage agent, I want each manifest entry to carry `from: <follow-up address>` (a string or a list), so that `publish` links each new ticket to its follow-up.
35. As a person, I want `publish` to archive each `from:` follow-up as closed with the new reason `promoted`, with a note that lists the new addresses, so that `follow-ups` empties and the follow-up points to its tickets.
36. As a person, I want each new ticket to record `original_address` (a string, or a list when it came from several follow-ups) set from its `from:` addresses, so that `triage locate` and the stats code still find the origin.
36a. As a person reading stats, I want a promoted follow-up counted once, by its own dates, so that a split (2 tickets), a group (3 follow-ups, 1 ticket) and the move of the filing day do not change the numbers.
37. As a person, I want one follow-up to become several tickets when it is too big (for example over the 60K cap), so that no ticket is too large for one context window.
38. As a person, I want the tickets of one split to be chained by `blocked_by` in the order `gx-to-tickets` sets, so that they do not run in parallel on the same files.
39. As a person, I want a row to be all or nothing: if any entry of a row fails `lint-claims` or `publish --dry-run`, every entry of that row is dropped, the row is saved `failed` with the refusal text, and the follow-up stays in `follow-ups`, so that the other rows still publish.
39a. As a triage agent, I want `gx tickets triage drop-rows --manifest <file> --row <addr>...` to remove the row's entries and replace each dropped slug in the other entries' `blocked_by` with the dropped entry's own `blocked_by` (minus other dropped slugs), so that no edge dangles and the chain closes over the gap (A <- B <- C with B dropped leaves C blocked by A).
39b. As a triage agent, I want a refusal that names no entry (a manifest parse error, a plan error, or a failure after the dry-run passed) to save every pending row `failed` with the refusal text and stop the loop with no retry, so that the loop always ends.
40. As a triage agent, I want `overlapWith` to become `blocked_by`, with overlapping rows chained oldest first by manifest slugs, so that the Write "Last pass" chain step is not needed.
41. As a triage agent, I want `publish` to create a missing ready epic as `draft`, and `gx-to-tickets` to run `ensure-code-review` and then open the epic last, so that Write no longer runs `epic add --status open` or its own code-review step for the ready epic. Map epics get no code-review ticket, as today. The ready epic gets no verify ticket: it has no spec to verify against.
41a. As a triage agent, I want `gx-to-tickets` to have a "no spec" input mode (the input is the follow-up bodies; `verify-plan` and the quiz are skipped), so that the ready epic `follow-ups-<date>` can be published without a spec.
42. As a person, I want `write-state --new-address` and `triage locate` to take and report a list of addresses, so that a re-run after `/clear` repeats nothing, also for a row that became several tickets.
43. As a person, I want row state `done` to mean that every ticket of the row exists and the follow-up is archived, so that the state stays honest.

Merging small follow-ups

44. As a person, I want Plan to group small ready-epic `implement` follow-ups that overlap in files or changed symbols into one ticket, so that one agent does related work in one context window.
45. As a person, I want "small" to be one test, a combined estimate under 45K tokens, using one estimate per ready-epic row saved by Plan, so that the rule is easy to predict.
46. As a person, I want a group that has a member with no estimate to fall back to chained rows, so that a guess never forms a group.
47. As a person, I want larger overlapping groups chained by `blocked_by`, and map-epic rows never grouped, so that grouping stays limited to work that fits one ticket.
48. As a person, I want the grouped ticket to have one acceptance-criteria group per follow-up, so that no follow-up's criteria are lost.
49. As a person, I want the grouped ticket's `blocked_by` to be the union of its members' chain edges, so that ordering against other rows holds.
50. As a person, I want `gx-to-tickets` to have the final say on slicing, so that a group that is too big at publish is split.
51. As a person, I want a group to be all or nothing: if any entry fails, the whole group stays in `follow-ups` and `write-state` records it as `failed`.
52. As a person, I want the web plan view to show a group as one row listing its members, with an "ungroup" action, so that I can undo a grouping I disagree with.
53. As a person in chat mode, I want the same group row printed, and `gx tickets triage row --ungroup` to undo it at step approve, so that web and chat behave alike.
54. As a person, I want an ungrouped member to fall back to the `overlapWith` chain, so that its ordering is still safe.

Publish safety

55. As a triage agent, I want `publish` to validate every `from:` address first (each must be a `draft` or `open` follow-up in a follow-ups epic of the same project) and to write nothing if one is invalid, so that a bad link never leaves half a publish.
56. As a triage agent, I want `publish` to write and open the tickets and archive the follow-ups last, so that a failure in the last step never loses tickets.
57. As a triage agent, I want an archive failure to make `publish` exit non-zero, name the follow-up and keep the tickets, so that the Write re-run finds them with `triage locate` and runs `gx tickets close --reason promoted` itself.

Records and compatibility

58. As a person with live data, I want `original_address` to accept a string or a YAML list, with a single original staying a string and old files still parsing, so that nothing existing breaks.
59. As a maintainer, I want one named migration step for the `original_address` change that rewrites nothing, plus an updated schema snapshot, so that the schema guard passes.
60. As a person, I want the `promoted` close reason and the manifest `from:` field to need no migration step, so that the change stays small.
61. As a person with a live Triage, I want the new record fields (a list of new addresses, a group marker, a per-row estimate) to be optional and the reader to accept an old single `newAddress`, so that the Triage finishes under the new code with no `Version` bump.

Measuring

62. As a maintainer, I want the trace script copied onto the impl branch with a writers-wait bucket, so that the targets can be checked after the epic lands.
63. As a maintainer, I want a run to count as having a quiz when a person reply falls inside the run window, with the "quiz skipped: no trigger fired" line as a cross-check, so that the 25% target is computed from the transcript.
64. As a maintainer, I want the target check to run over every Claude design session after the impl epic merges and print n with the medians, so that a small sample is visible. The trace script reads Claude transcripts only, so the targets cover Claude runs; Codex runs are not measured.
65. As a maintainer, I want to re-measure ticket quality on a fresh spec, since the prototype used a spec whose feature already existed, so that the parallel writers are proven on real work.
66. As a maintainer, I want a worker-tier subagent for the whole step 7 to be the next option if the trace misses the 5 minute target, so that a miss has a defined next move.

## Implementation Decisions

Draft command

- New command `gx tickets draft <project:epic> --plan <file> --manifest <file>`, next to `verify-plan` in the same command package. It never reads the spec.
- The plan file is its own YAML schema, not the `verify-plan` plan (that one holds stories and check counts). Per ticket: the slug, title, blockers, window, entry symbols, test files, mock states, plus `changed_symbols` and `changed_fields` lists. Only the `--manifest` append behaviour is shared with `verify-plan`. The two lists leave room for `ticket-authoring` tickets 01 and 08, which may later add output to the same sections. This spec builds none of their checks.
- Output is a manifest plus one body file per ticket next to it. `publish` already reads `body:` as a file path next to the manifest, so `publish` needs no change for this.
- The command owns the header, `## Test seams`, `## Where to look` and `## Mock check`. The two agent parts hold `<!-- TODO -->` markers. A marker still present makes `lint-claims` and `publish` refuse the body.
- Checks the command runs: paths, symbols, `-run` patterns and flow ids exist; callers from `gopls references` stamped `verified at {{commit}}`; consumers of a changed field from `rg`; the test environment of each seam file, Makefile targets, the highest migration number, helper prerequisites and the Playwright `-g` check. Each result is printed as a line in the body. Caller and consumer lists stop at 12 entries and end with a count of the rest. The step 4c claim checks live in the command, and step 4c of the skill is one pointer to it. No interim batched-`rg` skill text ships.
- Caller lists use one `gopls` session per `draft` run, batched over all symbols. When `gopls` is missing, or the symbol is in a non-Go file (such as `webui/` TypeScript), the command falls back to `rg` and labels the line as an `rg` result. The command prints its own wall-clock time, so the trace sees it.
- Checks the agent keeps: whether "X shows or hides Y" is true, which side wins when code and spec disagree, signature fallout, and examples checked against a helper.
- Re-run: regenerate only the command-owned sections, keep the agent's `## What to build` and `## Acceptance criteria`, delete the body file of a removed slug.
- `lint-claims` and `publish` keep every current rule. A failed claim check in `draft` is a printed line and a non-zero exit when a fact is missing. The body files and the manifest are written first, then each miss is printed.
- The implementer picks the exact marker text, the name of the plan fields not listed here, and the format of the printed fact lines.

Parallel writers

- Under 4 agent-written tickets the main agent writes all bodies in one pass. The count is of tickets whose bodies carry `<!-- TODO -->` markers, so verify tickets from `verify-plan` (no agent text) do not count.
- At 4 or more, the main agent first writes the skeleton: the plan with slugs, titles, `blocked_by`, window and a one-line scope per ticket. It runs `draft`, then starts min(3, ticket count) writer subagents on both Claude and Codex, in one wave, with the tickets split evenly in manifest order. The writers use the model from `gx agent model worker`.
- A writer fills the two TODO parts of its bodies and explores the code it needs. A writer never edits slugs or `blocked_by`. It reports any missing plan fact (an edge, an entry symbol, a test file, a mock state) to the main agent. The main agent edits the plan and re-runs `draft` once after all writers finish, not while they run, and applies the edge.
- Progress: the main agent prints a line when it starts the writers (the tickets of each writer) and a line as each writer finishes. Each writer appends one-line progress notes to its own log file next to the manifest. The implementer picks the log file name and the note format.
- Step 2 of `gx-to-tickets` and step 7 of `gx-design` get an explicit rule: no separate explore or draft subagent. The main agent writes the plan, and the writers explore. (No skill text names the old subagent; the agent dispatched it on its own, so the rule is what removes it.)
- This replaces the earlier "conditional, may never ship" plan: parallel writing ships by default for 4 or more tickets.

Quiz skip

- In step 5 of `gx-to-tickets` the quiz runs only when one of these holds: a ticket estimate over 45K tokens (75% of the 60K cap); the breakdown has a prefactor or wide-refactor ticket; `publish --dry-run` prints a "consider splitting" line; the spec does not name the slices, so the agent guessed the split. The triggers are one list in the skill. `gx-design` step 7 passes nothing extra.
- After a skip the agent publishes, prints the breakdown table (title, blocked by, estimate, test seam files) and the epic Summary, prints "quiz skipped: no trigger fired" or the trigger that fired, and opens the epic. The epic is not held as `draft`. The Queue-tab launch is the approval point.
- To correct a skip, the person tells the agent in chat or in a fresh run, and the agent edits the tickets with `gx tickets` commands.
- Unattended runs always skip. Claude and Codex behave alike.

Skill text

- `gx-to-tickets/SKILL.md` moves wide-refactor and prefactor-ticket guidance into linked files that load on demand. The implementer picks the file names and the exact split, and the SKILL.md keeps a short pointer with the condition that triggers the read. The skill's own guard tests (the skills package rule tests) stay green.
- `gx-design` step 7 keeps running `gx-to-tickets` inline. It is allowed to name the draft command only through `gx-to-tickets`.

Triage and publish

- `publish` accepts a manifest field `from:`, a follow-up address or a list. It validates every `from:` first: each must be a `draft` or `open` follow-up in a follow-ups epic of the same project. If any is invalid it writes nothing. It then writes the tickets (creating a missing epic as `draft`, as today) and archives the follow-ups last with the close reason `promoted` and a note listing the new addresses. `publish` never opens an epic: `gx-to-tickets` runs `ensure-code-review` and then `gx tickets set <epic> --status open`, as it does for `-impl`. If `follow-ups-<date>` already exists (a second Triage on the same day), `publish` adds the tickets to it, and `gx-to-tickets` opens it only if it is still `draft`. If an archive fails, it exits non-zero, names the follow-up and keeps the tickets. The close-reason list gains `promoted`, in the CLI, the tracker docs and wherever the reasons are listed.
- Each new ticket records `original_address`. Today the field means "the address the ticket was created under". For a promoted ticket, `publish` stamps it from the `from:` addresses in place of the ticket's own address, and the `StampCreated` doc comment changes to match. The field takes a string or a YAML list. A single original stays a string, old files still parse, and `locate` and the stats code match any entry.
- Stats count per follow-up, not per ticket (`server/stats`, `originalEpic` and the follow-up series). `FollowUpsCreated` comes from the archived `promoted` follow-up's own `created_at`. `FollowUpsFinished` lands on the day the last ticket that names it in `original_address` is done or cancelled. The new tickets never enter the follow-up series. `originalEpic` must parse a list `original_address`. The stats test (in `server/stats`) covers one promoted follow-up, a split into 2 tickets, a group of 3 follow-ups and a list case. One named migration step records the `original_address` change and rewrites nothing, and the schema snapshot is updated. `promoted` and `from:` need no step.
- `promoted` writes `status: cancelled`, which the tracker defines as "withdrawn, not performed". Here the work goes on in the new tickets. `promoted` matters only to the follow-up stats and `locate`, and the tracker doc's "cancelled" text names it as the exception. Close reasons are not in `schema.Enums()`, so no migration step is needed.
- The new meaning of `original_address` ("the follow-up address(es) a promoted ticket came from") is written in every place that describes it: the `StampCreated` doc comment, the `gx tickets set` help (`cmd/tickets/tickets_set.go`), the `gx tickets schema` output and `gx-local-tracker.md`.
- Triage Write runs once for the whole ready epic after every row's quick answers are saved (`gx tickets triage answers` runs first, so the answers are in the body's Decisions). It builds one plan, runs `draft`, writers (when 4 or more), `lint-claims`, and one `publish`. The follow-up body is the brief (`Suggested fix`), together with the verify evidence and the answers. It never quizzes.
- `overlapWith` becomes `blocked_by`: overlapping rows chain oldest first, using manifest slugs. The "Last pass" chain step and Write's own ready-epic code-review step go away. For the ready epic, `gx-to-tickets` runs `ensure-code-review` only, as Write does today, and never `ensure-verify` (the epic has no spec). Map epics get none, as today. `gx-to-tickets` gets a "no spec" mode for this run: the input is the follow-up bodies, `verify-plan` is skipped, and the quiz is skipped (the "spec does not name the slices" trigger does not apply). `triage epics` still records the ready epic's address. Write no longer runs `epic add --status open` for it.
- A row is all or nothing. `publish` itself is all or nothing for a whole manifest, so the triage agent runs a loop: it runs `lint-claims` and `publish --dry-run`, maps each refusal to its row through the entry's `from:`, calls `gx tickets triage drop-rows --manifest <file> --row <addr>...` (new command in the triage commands, implemented in the triage package: it removes every entry of the row and closes the chain as story 39a says), and retries, at most once per row. Each dropped row is saved `failed` with the refusal text, and its follow-up stays in `follow-ups`. The other rows publish. A refusal that names no entry saves every pending row `failed` with the text and ends the loop (story 39b). Splits chain by `blocked_by` in the order `gx-to-tickets` sets.
- `write-state --new-address` takes a list of addresses. `triage locate` reports `position: archived` for the follow-up, plus `newAddresses`: the tickets outside `follow-ups-archived` whose `original_address` names the row, only when the archived copy's Resolution reason is `promoted`. Otherwise it reports `position: moved` as today, since a moved map-epic follow-up also sits outside the archive and names the row. (Today `locateRow` keeps only the last match, and after a promote both the archived follow-up and the new tickets match.) Row state `done` means every ticket of the row exists and the follow-up is archived, and is computed from both.
- Triage record: the new fields (a list of new addresses, a group marker, a per-row estimate) are optional. The reader accepts an old single `newAddress`, so a live Triage finishes under the new code. No migration step and no `Version` bump.

Merging small follow-ups

- Plan groups only ready-epic `implement` rows, never map-epic rows. Rows are grouped when they overlap in files or changed symbols (read from the verify evidence, with no ticket re-reads), and the combined estimate is under 45K tokens. Plan saves one estimate per ready-epic row, from the verify evidence and `gx tickets verify-plan --table`. A group with a member that has no estimate falls back to chained rows. Larger overlapping groups stay chained by `blocked_by`.
- Plan proposes the groups. `gx-to-tickets` has the final say on slicing, so a group that is too big at publish is split. The manifest `from:` takes the member list, the body has one acceptance-criteria group per follow-up, and the ticket's `blocked_by` is the union of its members' chain edges. A group is all or nothing, as a split row is.
- The web plan view shows a group as one row that lists its members, with an "ungroup" action. The web action calls the same record operation as `triage row --ungroup`, through the existing plan-row edit API. Chat mode prints the same row, and `gx tickets triage row --ungroup` undoes the group at step approve. An ungrouped member falls back to the `overlapWith` chain.

Trace script

- The impl epic copies the research trace script (from the baseline branch `research/faster-ticket-authoring-baseline`, file `docs/research/faster-ticket-authoring-trace.py`) onto its branch and adds a writers-wait bucket. The implementer picks the destination path. A run has a quiz when a person reply falls inside the run window. The "quiz skipped: no trigger fired" line is a cross-check. The check over every design session runs after the impl epic merges and prints n with the medians.
- If the median agent time is above 5 minutes, the next option is a worker-tier subagent for the whole of step 7. It is not part of this spec.
- The target check cannot run inside the impl epic, since the verify ticket runs before the merge. The verify ticket instead files a follow-up, "re-measure faster-ticket-authoring targets after N design runs", that carries the command to run.

## Testing Decisions

- A good test checks external behavior: what `gx tickets draft`, `publish` and `lint-claims` print, write and exit with; what the triage record and `locate` report; what the web plan view shows. It does not check which internal function ran.
- Seams, highest first:
  - The `gx tickets` command layer, tested the way `verify-plan` and `lint-claims` are (the ticketsflow command tests): `draft` (sections, TODO markers, re-run keeps agent text and deletes a removed slug, appends to a `verify-plan` manifest, fact checks fail on a missing path or symbol) and `publish` (`from:` string and list, validation first and nothing written on an invalid address, archive last as `promoted`, non-zero exit on an archive failure with the tickets kept, a marker still present is refused).
  - The triage package and the `triage` commands: `write-state` and `locate` with lists (one test for a `promoted` archive giving `newAddresses`, one for a moved map-epic follow-up giving `position: moved`), the old single `newAddress` still read, `drop-rows` (entries removed, a 3-row chain with the middle row dropped, a group dropped whole), the unattributable-refusal path, `row --ungroup`.
  - The tickets package: `original_address` as a string or list, `IsOriginal` matching any entry, and the migration and schema snapshot guard tests (`TestSchemaSnapshot`).
  - `server/stats` (`stats_test.go`): the follow-up series for a promoted follow-up, a split, a group and a list `original_address`.
  - The skills rule tests (the existing skills package guard tests) for the skill text rules that can be checked by a test: the linked files exist and the quiz triggers are listed once.
  - One web e2e extension of the Triage journey flow (the `triage` scenario) for the group row and ungroup. No new Playwright flow, since a flow is only for a real server round trip, and this extends an existing flow.
- Prior art: `tickets_verify_plan_test.go`, `tickets_lint_claims_test.go`, `tickets_triage_write_test.go`, `triage/ops_test.go`, `ticketmove/original_test.go`, `migrations` schema snapshot test, and flow 199 in `webui/e2e/CATALOG.md`.
- Cap every test run with `scripts/proc-cap`, per the repo rule.
- The skills' prose behavior (parallel writers, quiz skip, step 7) has no code seam. Verification covers it by measurement, not by unit test.

## Verification

Each entry runs on a temporary gx server after the epic is built. Story numbers refer to the User Stories above.

Draft command (stories 1 to 14)

- Flow-catalog flows: none (CLI only).
- CLI checks: `gx tickets draft <project:epic> --plan <file> --manifest <file>` exits 0 on a plan with 3 tickets and writes a manifest and 3 body files (stories 1, 2). Each body has the four command-owned sections and two `<!-- TODO -->` markers (3, 4). `gx tickets lint-claims` and `gx tickets publish --dry-run` exit non-zero on a body that still has a marker (5, 14). A plan naming a missing path, symbol, `-run` pattern or flow id makes `draft` print the line and exit non-zero (6). A body prints `verified at <commit>` for a caller list and an `rg` line for a changed field (7, 8). Edit a TODO part, re-run `draft`, and the text stays; remove a slug and its file is gone (11, 12). `draft --manifest` on an existing `verify-plan` manifest keeps both sets of entries (10). A plan with `changed_symbols` and `changed_fields` parses, `changed_symbols` gives a `verified at <commit>` caller list and `changed_fields` gives `rg` consumer lines, both in "Where to look" (13). `gx tickets draft` exits 0 with no `gopls` on `PATH` and prints an `rg`-labelled caller line, and it prints its wall-clock time (7).
- Story 9: the output of `draft` has no line for the four judgment checks (X shows or hides Y, which side wins, signature fallout, examples against a helper).
- New flows or scenarios to write: none.

Parallel writers (stories 15 to 22)

- Flow-catalog flows: none.
- Run `gx-design` step 7 on a fresh spec (one whose feature does not exist yet) with 6 tickets. Check that 3 writers start in one wave, with a start line listing each writer's tickets and a finish line per writer, and that each writer's log file exists (15, 17, 20, 21). Check that no slug or `blocked_by` changed between the skeleton and the published tickets, and that a writer reports a missing edge instead of editing (18, 19). Check that no separate "where to look" subagent ran (22). Run again with 3 tickets and check that the main agent writes all bodies and starts no writer (16).
- Re-measure on the fresh spec: run `lint-claims` over the written bodies and count misses. The prototype showed 13 misses on a spec whose feature already existed. This run is the lint-miss check (65). The result goes in the verify ticket's notes; the bar is that `publish` accepts the manifest after at most one fix loop. On a miss, the verify ticket files a follow-up that lists the lint misses. A miss does not block the merge.
- New flows or scenarios to write: none.

Quiz skip (stories 23 to 28)

- Flow-catalog flows: none.
- Run the step on a fresh spec that names its slices with no estimate over 45K: no quiz, the table, the Summary and the line "quiz skipped: no trigger fired" are printed, and the epic is opened (23, 25). Run on a spec that does not name its slices: the quiz round is printed and nothing is published before the answer (24, 28). Run `publish --dry-run` on a manifest with a "consider splitting" line: it prints that line. Then run `gx-to-tickets` step 5 on a breakdown with a 46K ticket and a prefactor ticket: the agent prints each fired trigger by name and runs the quiz (24). After a skip, change one ticket by chat and confirm the tickets change before any Queue launch (26). A ralph-loop or triage Write run prints no quiz (27).
- New flows or scenarios to write: none.

Skill size (stories 29 to 31)

- Run `wc -l skills/gx-to-tickets/SKILL.md` and expect fewer lines than 528, with each linked file existing (29). Check that step 4c names the draft command, not a list of `rg` calls (30). Check that `gx-design` step 7 still runs `gx-to-tickets` inline (31). Run the skills package guard tests, capped.
- New flows or scenarios to write: none.

Triage and publish (stories 32 to 43, 55 to 57, including 36a, 39a, 39b and 41a)

- Flow-catalog flows: flow 199, Triage journey (part 3 shows Done with the ready group), to confirm Done still lists ready, map and merged groups with links after the change (32, 43).
- CLI and server checks: on a project with 4 ready follow-ups, run Triage to Write. One `publish` runs for the ready epic, and `gx-to-tickets` opens the epic last (41, 41a). The ready epic has a code-review ticket and no verify ticket (41). Every new ticket has seams, a "Where to look" map and an estimate (32, 33, 41). `gx tickets show` of each follow-up shows it archived with `promoted` and the new addresses (35). Each new ticket has `original_address` (36). Split one oversize follow-up into 2 entries sharing one `from:` and check they chain by `blocked_by` (37, 38). Make one entry fail `lint-claims` and check that the whole row is `failed` with the refusal text, its follow-up stays in `follow-ups`, and the other rows publish (39). Check `overlapWith` rows chain oldest first and no "Last pass" step ran (40). Check the ready epic has a code-review ticket and no verify ticket, and a map epic has none (41). A manifest entry with `from:` as a string and one with a list both publish and archive every listed follow-up (34). Run stats over a promoted follow-up, a split one and a group, and check each follow-up counts once, with its own `created_at` as the created day (36a). Drop one row after a refused `lint-claims` and check the retry publishes. With a 3-row chain (A, B, C) drop B through `gx tickets triage drop-rows` and check C is blocked by A and no dropped slug remains (39a). Feed a manifest parse error and check every pending row is `failed` with the text and the loop stops (39b). Run Write with no spec for the epic and check `verify-plan` and the quiz are skipped (41a). Run a second Triage on the same day and check the tickets join the existing `follow-ups-<date>` epic. `triage locate` and `write-state --new-address` handle a list (42). `publish` with an invalid `from:` (an archived follow-up, a ticket in another project) exits non-zero and writes nothing (55). With a forced archive failure, `publish` exits non-zero, names the follow-up and keeps the tickets, and `gx tickets close <addr> --reason promoted` finishes the row on a re-run (56, 57).
- New flows or scenarios to write: a fixture shape for the `triage` scenario with a record that has a group, used by the grouping flow below. Implementation ticket.

Merging small follow-ups (stories 44 to 54)

- Flow-catalog flows: flow 199, Triage journey, extended (not a new flow) with the group row: the plan view shows a group as one row that lists its members, "ungroup" splits it, and the ungrouped members show the chained `overlapWith` order (52, 54). Flow 199's existing checks for Approve and Done must still pass.
- CLI and server checks: with 3 small overlapping ready rows (combined estimate under 45K, each with an estimate), Plan saves one group row (44, 45). A row with no estimate gives chained rows (46). Overlapping rows over 45K combined stay chained and a map-epic row is never grouped (47). After Write, the one ticket has one acceptance-criteria group per follow-up, a `blocked_by` that is the union of its members' chain edges, and all 3 follow-ups archived as `promoted` (48, 49). A group that is too big at `publish` is split (50). A failing entry keeps the whole group in `follow-ups` with state `failed` (51). In chat mode, `gx tickets triage row --ungroup` at step approve undoes the group and prints the chain (53).
- New flows or scenarios to write: the group row and ungroup steps in flow 199 and the fixture shape above. Implementation ticket.

Records and compatibility (stories 58 to 61)

- Flow-catalog flows: none.
- CLI checks: `go test ./migrations -run TestSchemaSnapshot` passes with the updated snapshot and the new named step is listed (58, 59). A ticket file with a single-string `original_address` parses and `triage locate` finds it, and a ticket with a list is matched by any entry (58). `gx tickets close <addr> --reason promoted` exits 0 and `--reason` help lists `promoted` (60). Start a Triage record written by the old code (single `newAddress`, no group fields) under the new code and check that Write finishes it with no `Version` change (61).
- New flows or scenarios to write: none.

Measuring (stories 62 to 66)

- Flow-catalog flows: none.
- Run the copied trace script (`python3 -I <script> --lines <session.jsonl>`, where the file is the baseline context-window session `eebcaa8c-04ae-4a19-8120-15c8967e793f`) and expect 17.2 minutes (62). The script output includes the writers-wait bucket, and a run with a person reply in the window counts as a quiz (62, 63). The target check cannot run before the merge, so the verify ticket files a follow-up, "re-measure faster-ticket-authoring targets after N design runs", with the command: over every Claude design session since the merge, median agent time at most 5 minutes and a quiz in at most 25% of runs, with n printed (64). Codex runs are not measured. The fresh-spec re-measure and the lint-miss check are in the Parallel writers entry above (65). If the median misses 5 minutes, file a design follow-up for a worker-tier subagent for step 7 (66).
- New flows or scenarios to write: none.

## Out of Scope

- The speed of `gx tickets triage plan` (the plan rows JSON write, 94 s in session `07d4edcb-1c38-4e77-b5fb-9e3709ebc5c6`). It is not ticket authoring. Only triage's ticket publishing is in scope.
- The person's response time to a quiz. gx does not target it.
- Weakening `publish` or `lint-claims`. Both stay as strict as today.
- Moving the whole of step 7 into a worker-tier subagent. It is the next option only if the trace misses the 5 minute target.
- Any change to the `ticket-authoring` map's own checks. This spec only leaves room for them.
- Grouping map-epic follow-ups.

## Further Notes

- Baseline: 20 runs; median total 22.4 minutes, agent 10.9, quiz wait 6.0. Two flows exist: subagent-drafts (13 runs) and main-writes (7 runs). The research findings are on the branch `research/faster-ticket-authoring-baseline`, which was never merged.
- Parallel writers are not free: the `split-server-pkg` run used 9 writers, waited 21.5 minutes (719K subagent output tokens) and had 4.1 minutes of fix loops. This is why the writer count is capped at 3.
- The prototype (`context-window` spec, 6 tickets, 3 writers) took about 95 s in total with about 70K tokens per writer, and no `{{slug}}` was missing or invented across 16 references. It caught one real edge gap (a Mock check scenario needs a blocking edge to the ticket that builds it).
- Ship order: skill text first (quiz skip, the `SKILL.md` split), then the draft command (step 4c points at it from the start, with no interim `rg` text), then the triage change, which ships on its own and counts toward neither time target.
- Source: the map `gx:faster-ticket-authoring` and the follow-up `gx:follow-ups/270`.
