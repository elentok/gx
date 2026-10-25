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

See [gx-local-tracker.md](../gx-local-tracker.md) for the `verify` ticket type. This ticket commits
only the small fixes and flows in step 5. A clean round closes it commitless.

Run `gx tickets validate <addr>` on the ticket first. If it fails, stop and fix its frontmatter.

## 1. Read the round and the inputs

- The epic slug is the middle part of the ticket address. Read the ticket with
  `gx tickets show <addr>`. Never open ticket files by path.
- The round counter is a line `Round: N` at the top of the ticket body. No line means round 1.
- The spec is `docs/specs/<epic-slug>.md`, then `<epic-dir>/spec.md`. Read only its User Stories and
  Verification sections. If neither file exists, the epic's other tickets stand in for the spec.
- Read the other tickets of the epic with `gx tickets show <addr>` to learn what was built.
  Their `## Spec deviations` bullets are deliberate, so do not report them as findings.
- If the spec has no Verification section, derive one: for each user story, pick the catalog flows
  from step 3 and the CLI or server checks it implies.
- If the epic has no web, CLI or server change, close the ticket now (step 7, clean round).

## 2. Start the server

Start the temporary gx server on the fixture, using the build-and-start steps in section 1 of the
`gx-ui-check` skill. The fixture is isolated: it uses temp state dirs and never touches real gx state.
Keep it in a background shell. Stop it before you finish (step 7).

If the fixture does not start, that is a finding (step 4). Fix it if it is small, else open a
ticket.

## 3. Map the diff to the flow catalog

The Verification section names the flows to run. As a backstop, also map the diff to the catalog:

- `git merge-base main HEAD` is the fixed point. `git diff --stat <fixed-point>...HEAD` lists what changed.
- For each changed web, CLI or server area, find the catalog flows that cover it. Add any the
  Verification section missed.
- Run only these flows and the feature checks. Do not run the whole suite; CI does that.

## 4. Run and record

- Run each selected flow and each CLI or server verb check from the Verification section.
- Drive the browser by hand only for a gap no flow covers. Use text snapshots, not screenshots.
- A harness failure (the fixture crashes, a flow fails for a reason unrelated to the epic, a
  selector is stale) is a finding, like any other.
- Write one line per user story into the ticket body, under a `## Round N` heading:
  `- Story <n>: pass` or `- Story <n>: fail, <what failed>`. Findings go below the lines. Write the
  text to a scratchpad file with Write, then `gx tickets section <addr> "Round N" - < <file>`; never
  a heredoc, never edit the ticket file by path.

Check the token count with `gx agent context-window` after each story. At 90K tokens, stop checking.
Open tickets for the stories not yet checked (step 6) and close out the round.

## 5. Fix one small finding inline

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
  - anything you skipped, such as stories left unchecked at the token limit
- Clean round (every story passes, no findings): append the lines from step 4, then
  `gx tickets set <addr> --iteration-status finished --commitless true`.
- Inline fix: commit first. Report `finished` in a later tool call, with no `--commitless`.
- Tickets opened: write the story lines and the findings into the body, then
  `gx tickets set <addr> --iteration-status finished --commitless true`, unless step 5 committed
  something in this round. Then drop `--commitless`.
- Round 3 that still has problems: commit anything green, write the body, then run
  `gx tickets ask` following [gx-local-tracker.md](../gx-local-tracker.md)'s announce-and-stop
  rule. The question names the failing stories. End your turn. Do not report `finished`.

`--status done` is never yours to set. gx decides it after you report `finished`.
