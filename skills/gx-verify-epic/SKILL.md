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

Follow the steps below for your role. Collect and the lone cap stop are not written yet; see
"Not yet written" at the end.

## The verify part body

This is the one definition of a part's body format. gx-to-tickets and the collect step link here.

- `Round: N` and `Role: part`, one per line, at the top.
- The resolved spec path.
- The fixture scenario to start.
- Per story: its spec number, its text and its Verification entry, both word for word, and its token
  estimate.
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
- If the epic has no web, CLI or server change, close the ticket now (step 7, clean round).

## 2. Start the server

Start the temporary gx server on the fixture, using the build-and-start steps in section 1 of the
`gx-ui-check` skill. A part uses the fixture scenario named in its ticket. Every part session starts
its own server; never share one. The fixture is isolated: it uses temp state dirs and never touches
real gx state. Keep it in a background shell. Stop it before you finish.

If the fixture does not start, that is a finding. A part records it under its results. A lone ticket
fixes it if small, else opens a ticket (step 5 and 6).

## 3. Map the diff to the flow catalog

A part skips this step: it runs the Verification entries in its ticket.

Lone: the Verification section names the flows to run. As a backstop, also map the diff to the
catalog:

- `git merge-base main HEAD` is the fixed point. `git diff --stat <fixed-point>...HEAD` lists what changed.
- For each changed web, CLI or server area, find the catalog flows that cover it. Add any the
  Verification section missed.
- Run only these flows and the feature checks. Do not run the whole suite; CI does that.

## 4. Run and record

- Run each selected flow and each CLI or server verb check from the Verification entry.
- Drive the browser by hand only for a gap no flow covers. Use text snapshots, not screenshots.
- A harness failure (the fixture crashes, a flow fails for a reason unrelated to the epic, a
  selector is stale) is a finding, like any other.
- Before you write a story's line, quote its text, and name the test or check that covers it. Read
  that test or run that check; do not infer it from a file name. A required check you skipped (such
  as a mutation check) leaves the story without a verdict.
- The line format is `- Story <n>: pass, <covering test or check>`, `- Story <n>: fail, <what
  failed>` or `- Story <n>: unchecked, <why>`. Use `unchecked` when you named no covering test or
  check, or did not run it. Never write `pass` for a story you did not read or verify.

**Part.** Write each story's line under `## Results` in your own ticket only, right after you check
that story, using the spec's story numbers. Do not wait to write them all at the end. Findings go
below the lines, with enough detail for someone else to fix them. Write the text to a scratchpad file
with Write, then `gx tickets section <addr> "Results" - < <file>` (rewrite the whole section each
time, lines so far plus the new one; never a heredoc, never edit the ticket file by path). A part
never fixes, never commits, never judges scope and never opens tickets.

**Lone.** Write one line per user story into the ticket body, under a `## Round N` heading, with the
same section command. Findings go below the lines.

### Part cap stop

When the session shows a finish-up prompt, stop checking:

1. Mark every story you have not checked `unchecked`, with the reason `cap stop`.
2. Write the results (step 4).
3. Stop the fixture server and any other background shell.
4. Report `gx tickets set <addr> --iteration-status finished --commitless true`.

### Part close-out

When every story has a line, stop the fixture server and any other background shell, then report
`gx tickets set <addr> --iteration-status finished --commitless true`. A part commits nothing.

The rest of this skill (steps 5 to 7) is for a lone ticket.

## 5. Fix one small finding inline

This ticket commits only the small fixes and flows in this step. A clean round closes it commitless.

If the round has exactly one finding and the fix is about 30 changed lines or fewer in at most 2
files, fix it here. Re-run the check that failed. Run `gofmt -l` on the changed Go files, then commit
with the subject `<epic>/<ticket id>: <what changed>`.

A small missing flow or scenario follows the same size rule. Write it, run it and commit it to the
suite.

Anything larger, or any second finding, goes to step 6. Do not fix some findings inline and open
tickets for others.

## 6. Open tickets for several findings or a larger fix

- One ticket per independent finding, or per group of findings that name the same fix. Use
  `gx tickets add <epic> --parent <this ticket id> --slug <slug> --body -`. The child names this
  ticket as its `parent`. Nothing about it is recorded on this ticket.
- Then open the next-round verify ticket the same way, with `--type verify --blocked-by <the child
  ids>`, a body that starts with `Round: N+1`, and a line pointing at this ticket's address. It
  waits for the children, then rechecks all stories.
- If this is round 3, do not open a next round. Run `gx tickets ask` instead (see step 7).

## 7. Close out

- Stop the server and any other background shell.
- Write a `### Summary` under this round's `## Round N` heading, before you report. Keep it short:
  - what you ran: the flows, CLI and server checks, and any manual browser checks
  - the result per story, as in step 4
  - what you did about findings: fixed inline (with the commit), tickets opened (with their
    addresses), or none
  - anything you skipped
- Clean round (every story passes, none `unchecked`, no findings): append the lines from step 4, then
  `gx tickets set <addr> --iteration-status finished --commitless true`.
- Inline fix: commit first. Report `finished` in a later tool call, with no `--commitless`.
- Tickets opened: write the story lines and the findings into the body, then
  `gx tickets set <addr> --iteration-status finished --commitless true`, unless step 5 committed
  something in this round. Then drop `--commitless`.
- Round 3 that still has problems: commit anything green, write the body, then run
  `gx tickets ask` following [gx-local-tracker.md](../gx-local-tracker.md)'s announce-and-stop
  rule. The question names the failing stories. End your turn. Do not report `finished`.

`--status done` is never yours to set. gx decides it after you report `finished`.

## Not yet written

The next ticket in this epic adds, here and in the role steps above:

- the collect step (merge the parts' results, decide, open work),
- the lone role's split into parts plus a collect ticket,
- the lone and collect cap stops.

Until then a collect ticket has no steps: stop and ask with `gx tickets ask`.
