---
name: gx-verify-epic
description: >-
  Check a finished epic in a running gx. Starts a temporary gx server on the fixture, works through
  the spec's Verification section, and records one pass or fail line per user story. Launched for
  tickets with `type: verify`. Not for changed-file checks; that is gx-verify.
disable-model-invocation: true
---

# gx Verify Epic

Check that an epic works in a running gx, after code review has merged. You drive the web UI and
the CLI and server verbs against a temporary server. You do not verify the TUI.

See [gx-local-tracker.md](../gx-local-tracker.md) for the `verify` ticket type.

Run `gx tickets validate <addr>` on the ticket first. If it fails, stop and fix its frontmatter.

## Roles

Every verify ticket keeps `type: verify` and has one of three roles. The body names it with a
`Role: part` or `Role: collect` line next to the `Round: N` line at the top. **No `Role:` line means
lone**, so the `ensure-verify` stub and older verify tickets are lone tickets.

- **Part**: checks its own stories and records every result. It never fixes, never commits, never
  judges scope and never opens tickets. The one exception is a gx-to-spec follow-up for an unclear
  story (step 1).
- **Collect**: checks nothing itself. It merges the parts' results, decides and opens work.
- **Lone**: checks like a part and decides like a collect ticket. One-part epics and the stub are
  lone.

Follow the steps below for your role. A part follows steps 1 to 4. A collect ticket follows step 1
and steps 5 to 7. A lone ticket follows steps 1 to 4, then steps 5 to 7.

## The verify part body

This is the one definition of a part's body format. gx-to-tickets and the collect step link here.

- `Round: N` and `Role: part`, one per line, at the top.
- The resolved spec path.
- The fixture scenario to start.
- Per story: its spec number, its text and its Verification entry, both word for word, and its token
  estimate.
- Per Mock states line of the spec's Verification section: the line word for word (mock path, state,
  width, scenario, parts), and its estimate from the gx-to-tickets estimate table.
- A next-round part also adds, per story, the finding and the fix ticket it rechecks.

Section names are free. The part writes its results under `## Results` (step 4).

## 1. Read the round and the inputs

- The epic slug is the middle part of the ticket address. Read the ticket with
  `gx tickets show <addr>`. Never open ticket files by path.
- The round counter is the `Round: N` line at the top of the ticket body. No line means round 1.

**Part.** Read only:

- its own ticket, and
- the epic's deviations: `gx tickets show <epic> --section "Spec deviations"`. Those bullets are
  deliberate, so do not report them as findings.

Do not read the spec. Open it, at the path in the ticket, only for these cases:

- A story is unclear (its text or Verification entry cannot be checked as written). Read only that
  story's lines, then file a follow-up for gx-to-spec with
  [gx-follow-up](../gx-follow-up/SKILL.md), unattended, each time.
- A story is added from a fix diff (next round, below). That read files no follow-up.

**Collect.** Read only:

- its own ticket: it holds `## Out of Scope` and, when it has one, a starting table (step 5),
- `## Results` from every verify ticket in its `blocked_by`, with `gx tickets show <addr> --section
  Results`, and
- the epic's deviations, as a part does.

Do not read the spec. A collect ticket checks nothing itself, except the rerun of one inline fix
(step 6).

**Next-round part.** Before you check anything, read the diff of the fix tickets in its
`blocked_by`. Add any other passed story whose code the diff changed: copy its text and Verification
entry from the spec into your ticket's story list, in the format above.

**Lone.** The spec is `docs/specs/<epic-slug>.md`, then `<epic-dir>/spec.md`. Read only its User
Stories and Verification sections. Read them in full. Never cut the output with `head`, `tail` or a
line limit, and if the tool says lines were elided, read that range again. If neither file exists,
the epic's other tickets stand in for the spec.

- Read the other tickets of the epic with `gx tickets show <addr>` to learn what was built. Their
  `## Spec deviations` bullets are deliberate, so do not report them as findings.
- If the spec has no Verification section, derive one: for each user story, pick the catalog flows
  from step 3 and the CLI or server checks it implies.
- The Mock states list is part of the Verification section. Read it with the stories, and check its
  lines like a part does (step 4).
- If the epic has no web, CLI or server change, close the ticket now (step 7, clean round).
- Note the spec's `## Out of Scope` section. A lone ticket without one in its own ticket copies it
  from the spec when it opens a collect ticket (step 6).

## 2. Start the server

Start the temporary gx server on the fixture, using the build-and-start steps in section 1 of the
`gx-ui-check` skill. A part uses the fixture scenario named in its ticket. Every part session starts
its own server; never share one. The fixture is isolated: it uses temp state dirs and never touches
real gx state. Keep it in a background shell. Stop it before you finish.

If the fixture does not start, that is a finding. A part records it under its results. A lone ticket
decides it like any finding (steps 5 and 6).

## 3. Map the diff to the flow catalog

A part skips this step: it runs the Verification entries in its ticket.

Lone: the Verification section names the flows to run. As a backstop, also map the diff to the
catalog:

- `git merge-base main HEAD` is the fixed point. `git diff --stat <fixed-point>...HEAD` lists what changed.
- For each changed web, CLI or server area, find the catalog flows that cover it. Add any the
  Verification section missed.
- Run these flows and the feature checks, then the full check below.

Run `gx check --full` once per epic: in the lone ticket, or in the collect ticket when there are
parts. Pick one, and say which in your ticket. Send each failure through
[the failing-tests rule](../gx-failing-tests.md). A branch fault is a finding.

A flow that fails at HEAD is rerun at `git merge-base main HEAD` in a temporary worktree. Rerun only on
failure. A flow that fails there too is not the epic's fault: record it as `pre-existing, <follow-up
addr>`.

When the spec names a time target for a check kind, compare the `gx check --full` line's time with it.
Read the Resolution the spec names, and write the result as a story line.

## 4. Run and record

- Run each selected flow and each CLI or server verb check from the Verification entry.
- Drive the browser by hand only for a gap no flow covers. Use text snapshots, not screenshots. For
  Mock states lines, follow `MOCK-CHECK.md` (below); its screenshots are allowed.
- A harness failure (the fixture crashes, a flow fails for a reason unrelated to the epic, a
  selector is stale) is a finding, like any other.
- Before you write a story's line, quote its text, and name the test or check that covers it. Read
  that test or run that check; do not infer it from a file name. A required check you skipped (such
  as a mutation check) leaves the story without a verdict.
- The line format is `- Story <n>: pass, <covering test or check>`, `- Story <n>: fail, <what
  failed>` or `- Story <n>: unchecked, <why>`. Use `unchecked` when you named no covering test or
  check, or did not run it. Never write `pass` for a story you did not read or verify.

**Mock states lines.** These steps apply only when the part (or lone ticket) has Mock states lines.
With none, skip this block: the epic verifies as before.

- Check each line like a story. Start the line's fixture scenario, screenshot the whole state section
  of the mock and the built page at the line's width, and compare them by eye. Follow the steps in
  [gx-ui-check/MOCK-CHECK.md](../gx-ui-check/MOCK-CHECK.md); do not copy them here.
- A missing mock file named by a Mock states line is an `unchecked` line plus a finding, never a
  silent skip.
- Write one result line per Mock states line, with the same verdicts as a story line (`pass`, `fail`,
  `unchecked`), in the format `- Mock <mock path> <state> <width>: pass, <what you compared>`.
- A difference listed in the epic's `## Spec deviations` is not a finding.
- Any other look difference is a finding. Step 6 decides it like any finding, so it gets a fix ticket.

**Part.** Write each story's line under `## Results` in your own ticket only, right after you check
that story, using the spec's story numbers. Do not wait to write them all at the end. Findings go
below the lines, with enough detail for someone else to fix them. Write the text to a scratchpad file
with Write, then `gx tickets section <addr> "Results" - < <file>` (rewrite the whole section each
time, lines so far plus the new one; never a heredoc, never edit the ticket file by path). A part
never fixes, never commits, never judges scope and never opens tickets.

**Lone.** Write the lines and findings the same way, under `## Results` in your own ticket.

### Part cap stop

When the session shows a finish-up prompt, stop checking:

1. Mark every story you have not checked `unchecked`, with the reason `cap stop`.
2. Write the results (step 4).
3. Stop the fixture server and any other background shell.
4. Report `gx tickets set <addr> --iteration-status finished --commitless true`.

### Part close-out

When every story has a line, stop the fixture server and any other background shell, then report
`gx tickets set <addr> --iteration-status finished --commitless true`. A part commits nothing.

### Lone and collect cap stops

A lone or collect ticket stops at the same finish-up prompt. Do the first steps of its list, then
stop every background shell you started (the fixture, for a lone ticket) before you report finished.

- **Lone.** Mark every unchecked story `unchecked`, reason `cap stop`, and write the results (step 4).
  Open extra parts for the unchecked stories plus a collect ticket, all with the same `Round: N`
  (step 6). Write your results into that collect ticket as its starting table (step 5), and copy
  `## Out of Scope` into it. Do not name
  this ticket in its `blocked_by`: the collect ticket is its child, and a blocker waits on its whole
  fork subtree. Report `gx tickets set <addr> --iteration-status finished --commitless true` (drop
  `--commitless` if you committed an inline fix).
- **Collect.** Write the current table with the decisions made so far into your own ticket, and into
  a new collect ticket for the same round, same `Round: N`, as its starting table. Open it as a
  child; step 6 covers copying `## Out of Scope` and setting `expected_context_window` and
  `blocked_by`. Report finished.

The rest of this skill (steps 5 to 7) is for a collect or lone ticket.

## 5. Merge the results into one table

A lone ticket uses its own results from step 4 as the part results.

Write one table for all stories into your ticket, under `## Table`, with
`gx tickets section <addr> "Table" - < <file>`. Use the spec's story numbers. Each Mock states line
gets a row too, labelled `Mock <state> <width>`. Columns:

| story | verdict | covering check | round last checked | finding, with fix ticket, follow-up or inline-fix commit |
| ----- | ------- | -------------- | ------------------ | -------------------------------------------------------- |

- Carry the previous collect ticket's table forward: start from the table in your own ticket and
  update only the rows the parts re-checked.
- Take each verdict and covering check from the parts' `## Results` lines. Keep `unchecked` for
  every story a cap stop left unchecked, and note the cap stop in its row. It stays `unchecked`
  until its extra part runs.
- Do not recheck a part's `pass`.

## 6. Decide and open the work

**Cap-stop leftovers.** For the unchecked stories, open extra parts plus a collect ticket
(the first one when you are a lone ticket) that names them in `blocked_by`, all with the same `Round: N`.

- A story left unchecked for the second time in a round gets a solo extra part (one story).
- If that solo part also stops at the cap, mark the story `fail`, "too large for one session",
  record every other decision, open all other work first, then run `gx tickets ask` for it last (see
  step 7).

**Findings.**

- Exactly one small finding (30 changed lines or fewer, at most 2 files): fix it inline. Rerun the
  check that failed. Run `gofmt -l` on the changed Go files, then commit with the subject
  `<epic>/<ticket id>: <what changed>`. Name the commit in the table row. A small missing flow or
  scenario follows the same size rule.
- Otherwise, one child fix ticket per finding, or per group of findings that name the same fix. Do not
  fix some findings inline and open tickets for others.
- A finding outside the epic's scope goes to gx-follow-up ([gx-follow-up](../gx-follow-up/SKILL.md),
  unattended), not to a fix ticket. Judge it against the `## Out of Scope` section in your own
  ticket: a finding that section covers is out of scope. The table names each follow-up.

**Next round.** Open `Round: N+1` when there are fix tickets, or when the inline fix's rerun failed
or touched code of other passed stories. Only a fix round raises N.

- Open next-round parts in the format of [The verify part body](#the-verify-part-body), covering the
  failed and unchecked stories plus every passed story you expect a fix to touch, judged from the
  finding. Each story carries its finding and fix ticket. Each part names its fix tickets in
  `blocked_by`.
- Also open a new collect ticket, `Round: N+1`, that names the new parts in `blocked_by`. Write the
  current table into it as its starting table (step 5).

**Opening tickets.** Use `gx tickets add <epic> --parent <this ticket id> --slug <slug>
[--type verify] [--blocked-by <ids>] --expected-context-window <K> --body -`.

- Every ticket you open is your child. Nothing about it is recorded on this ticket.
- Each gets `expected_context_window`: the sum of its per-story estimates plus the base (parts), or
  the collect estimate (collect tickets).
- Every collect ticket you open gets a copy of `## Out of Scope`: from your own ticket, or for a
  lone ticket without one, from the spec. It also gets the current table as its starting table
  (step 5 and the cap stops).

## 7. Close out

- Stop the server and any other background shell.
- Write a `### Summary` under `## Table` before you report. Keep it short: what you ran (lone), the
  result per story, what you did about findings (inline commit, tickets and
  follow-ups opened, or none), and anything you skipped.
- **Round 3** (`Round: 3`) with stories still failing: open no next round. Commit anything green,
  write the body, then run `gx tickets ask` following
  [gx-local-tracker.md](../gx-local-tracker.md)'s announce-and-stop rule. The question names the
  failing stories. End your turn. Do not report `finished`.
- **Cap-stop solo failure** (step 6): open all other work first, then run `gx tickets ask` last, as
  above.
- **Clean round**: every story passes and every finding was fixed inline with a passing rerun that
  touched no other passed story's code, or sent to gx-follow-up. Report
  `gx tickets set <addr> --iteration-status finished --commitless true`, unless this ticket made the
  inline fix.
- **Inline fix or tickets opened**: commit first if you fixed inline. Report `finished` in a later
  tool call, without `--commitless` only if you committed.

`--status done` is never yours to set. gx decides it after you report `finished`.
