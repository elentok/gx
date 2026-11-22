---
name: gx-code-review
description: >-
  Run a configurable, unattended review pass over an epic once every other ticket in it is done —
  code quality plus spec conformance — triage the joined findings with a higher-tier consultant, and
  turn what survives into new tickets. Invoked by gx-implement when the claimed ticket has `type:
  code-review`.
disable-model-invocation: true
---

# gx Code Review

Run an unattended review pass over an epic's branch along two axes — **Quality** (the configured
review skills) and **Spec** (does the diff implement what the epic asked for?) — and turn what's
worth fixing into new tickets. The two axes run in parallel and their findings are joined into a
single triage. See [gx-local-tracker.md](../gx-local-tracker.md) for the `code-review` ticket type and its
frontier rule — that rule is what gets this skill invoked at the right time (once every other ticket
in the epic is `done`), via the normal queue. No bespoke triggering mechanism is needed here.

This skill never edits code. Its only output is new tickets — each one carrying `parent: <this
ticket's id>`, the sole edge between them — plus its own ticket's body and closing status. Nothing
about the tickets it opens is recorded on this ticket.

Change files with a file edit (Edit) or file write (Write) only, never `sed -i`, `cat >` or a heredoc. Write the file first
(a scratch body, say), then use it. The guard hook rejects shell file writes.

Before starting, run `gx tickets validate <addr>` on the ticket you're about to claim. If it fails,
stop and fix the ticket's frontmatter before doing anything else.

## 1. Resolve the epic diff and its spec

- The epic slug is the middle part of the ticket address (`project:<epic-slug>/NN`). Read the ticket
  with `gx tickets show <addr>` and the epic's other tickets by address too
  (`gx tickets epics` lists epics) — never open ticket files by path.
- The epic's branch is the current branch (every ralph-loop worktree runs on a `ralph-loop/`-prefixed
  branch, one per epic — see gx-implement).
- Capture the diff scope once with `gx tickets review-bundle <project:epic>` (add `--out <dir>` to
  pick the directory; the default is a new temp directory). It writes `index.md` (merge-base, commit
  list, spec path, review skill paths, per-file index), the per-area diff files
  (`diff-<area>-<n>.diff`, each under 100 KB; docs, fixtures and lockfiles stay out of the code
  diffs) and `deviations.md`, and prints the merge-base, commits, spec and skill paths. It fails
  here — not inside a subagent — if the merge-base can't be resolved or the diff is empty. Do not
  run `git diff` or `git log` yourself.
- The epic's spec is the first of these that exists: `docs/specs/<epic-slug>.md` from the repo root,
  then `docs/specs/<epic-slug minus a trailing -impl>.md` (an `-impl` epic's design spec has no
  `-impl` in its name), then `<epic-dir>/spec.md` (legacy location). Look with one `ls` over all
  three paths, not one failed `ls` per guess. When none exists, the epic's ticket bodies are the
  spec: every other ticket in the epic except this code-review ticket, each read in full with
  `gx tickets show <addr>`. Either way the
  spec resolves — the Spec axis always runs.
- Derive reviewer scope from ticket statuses now: list the epic's tickets with their statuses and
  count the code-changing ones: M is the epic's `implement` tickets plus their fix children. Verify,
  investigate and merge tickets never change code, so they stay out of M. Don't reuse a scope
  written into an older prompt or ticket body.
- The **known spec deviations** and the tickets filed after this code-review ticket are in the
  bundle's `deviations.md`; pass it on whole. The post-handoff tickets are follow-ups of the review,
  so the Spec axis must not flag them as scope creep. When the bundle is not at hand, collect the
  deviations in one call:
  `gx tickets show <epic> --section "Spec deviations"`, optionally with `--tickets 31-46`. Pass the
  full list on, not a truncated view: no `| head`, and no loop whose output is cut ("N lines
  omitted"). `--section` replaces hand-built `sed` or `head` loops over `gx tickets show`; `--section X --exists` tests for a section by exit code. Use only this epic's tickets (skip this
  code-review ticket) and gather the bullets under each `## Spec deviations` section (also check `## Comments` and handoff notes for older
  tickets that recorded one there). Join them into one short list, each line prefixed with its
  ticket address. If there are none, the list is empty.

## 2. Read the configured review skills

`gx config get skills.code-review` prints the effective list as JSON. If the list is
empty or the field is absent, fall back to the default list: `["gx-thermo-nuclear-code-quality-review", "gx-test-review", "gx-code-reviewer", "gx-silent-failure-hunter", "gx-type-design-analyzer", "gx-comment-analyzer", "gx-pr-test-analyzer", "gx-security-review"]`.

Resolve each name to a `SKILL.md` path, taking the first that exists: `skills/<name>/SKILL.md` from
the reviewed repo's root, then `../<name>/SKILL.md` relative to this file. Subagents read that file
by path rather than load the skill through a skill-loading call, because a skill with `disable-model-invocation: true` (the vendored
default one keeps upstream's flag) is not listed there, and the quality axis used to degrade silently
into a manual review.

If any name resolves to no file, don't review with a partial list. Follow
[gx-local-tracker.md](../gx-local-tracker.md)'s announce-and-stop rule: run `gx tickets ask`, with the question naming which names didn't resolve
and the two paths tried for each, and end your turn.

## 3. Run both axes as independent parallel subagents

### Size gate

Count the changed lines from the bundle's diff files (insertions plus deletions). At
or under about 50 changed lines, skip the Quality subagents: a configured review skill costs far more
than it finds on a diff that small. The Spec subagent still runs. Note the skip in the ticket body at
step 7.

### Shared review file

Reviewers must not each re-run the diff and the same searches. The bundle from step 1 is the shared
review file: pass every subagent the `index.md` path and the area files for its own scope (go, tests,
web, docs). They read those files instead of running `git diff` themselves. They may Read a changed
file for more context, but not re-run the diff or a search a file already holds. For the output of
any search you ran yourself (say, a `rg` for a symbol the diff changes), write one more file in your
scratchpad directory (with a file write) and pass its path too.

Each bundle file is small enough to read in one Read. There is one diff file per area, and a
reviewer reads the index first, then the area files for its own scope, each once, without paging
one file in overlapping line ranges.

Every subagent prompt also says: "You must verify a suspicion before you report it: read the file or
run the check yourself, with one grep or one read. Do not flag something as possible or unverified.
Drop it if you cannot confirm it. Don't run `git diff` or `git log -p`; the review files hold the
diff."

### Launch

Send **one message** carrying every start-a-subagent call (Agent tool) — one per configured skill name (none when the
size gate tripped), plus the one Spec call below (`general-purpose` subagents, run in the foreground
since the next step needs every result back before it can start). Running them in the same message
is what makes the passes independent instead of serialized, and it is what puts the Spec axis
alongside the quality passes rather than after them.

The parallel cap and waves are in [Skill conventions](../gx-local-tracker.md#skill-conventions). The
Spec subagents (every range, and the coverage re-dispatch) count toward the waves. Every configured
reviewer still runs, whatever the wave count. On Claude the single launch message stays.

Every subagent gets the shared review file's path from above.

**Quality subagents** (one per configured skill) also get:

- The `SKILL.md` path resolved in step 2, with the instruction to Read it and follow it against that
  diff.
- The spec path (or ticket address list) resolved in step 1, and the known spec deviations list,
  with the rule: "Don't propose changes that undo an explicit requirement of the spec or tickets.
  Flag the tension instead."
- The brief: "Report findings as a list — file, line/hunk, summary, severity, suggested fix. Under
  500 words. The severity is one of `critical`, `high` or `medium`; drop anything lower."
- The test-gap rule: "Open verify tickets own test gaps. Report a missing test only when no open verify
  ticket covers that story."

**Spec subagent** (exactly one, whatever the configured skill list holds) also gets:

- The path of the spec resolved in step 1 — `docs/specs/<epic-slug>.md` (or the legacy `spec.md`), or
  the list of ticket addresses that stand in for it.
- The known spec deviations list from step 1, with: "These are claims to check, not exclusions.
  Verify each deviation against the code before skipping it. Report a stale or wrong one as a
  finding."
- The brief: "Report: (a) requirements the spec asked for that are missing or partial; (b) behaviour
  in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where
  the implementation looks wrong. Quote the spec line — or the ticket address and line — for each
  finding. Under 400 words of findings, plus a last line
  `Covered: <addresses reviewed> of <M> code-changing tickets` that is not counted in the cap."
- The test-gap rule: "Open verify tickets own test gaps. Report a missing test only when no open verify
  ticket covers that story."

When M is more than about 12 code-changing tickets, split the Spec axis across several Spec
subagents by ticket-address range (about 12 code-changing tickets each), each with the same brief for its own range. They go
in the same message as the quality calls.

**Coverage.** After the results return, check each `Covered:` line. When a range is only partly
covered, re-dispatch a Spec subagent for the tickets left out before triage; it follows the same wave rule. Never let a partial
review read as a full one: step 7 records `Spec reviewed N of M code-changing tickets`, with N counted from the
`Covered:` lines.

Collect every subagent's raw findings verbatim; don't summarize or filter them yourself here.

## 4. Triage with a higher-tier consultant

Once every review subagent has returned, start **one** fresh-context, read-only consultant subagent
(same pattern as `~/.dotfiles/core/ai/skills/consult`'s consultant-selection step: request the
strongest tier — pass the output of `gx agent model strongest` as the subagent call's model; a second word is its `reasoning_effort`, see
[Skill conventions](../gx-local-tracker.md#skill-conventions)).

Brief it with every subagent's raw findings joined into one list, each labeled by the axis and skill
that produced it (`Quality / <skill-name>`, or `Spec`), plus the epic's ticket list for context (what
already shipped, what the epic was for) and the known spec deviations list from step 1. Also give
it the paths it would otherwise hunt for: the output of `gx tickets root`, the spec path (or ticket
address list) from step 1, the review file paths from step 3, and, for a finding that touches
`webui/e2e/*.e2e.ts`, the `gx-playwright-tests` skill and the `.ai/index.md` Testing section. Tell it to verify a claim with one grep or one read, not to
re-run searches the reviewers already ran. Ask it to:

- Treat a finding that only restates a known spec deviation as **rejected** (reason: deliberate
  deviation, with the ticket address), but only after checking the deviation against the code. Keep
  a finding that says a deviation is stale or wrong.

- Consolidate duplicate findings raised by more than one reviewer, across axes as well as within one.
- Read the two axes against each other: a quality finding on code the Spec axis flags as scope creep
  argues for deleting the code rather than polishing it.
- Prioritize by actual impact, not by how many reviewers happened to flag it.
- Give each finding one verdict and a one-line reason:
  - **approved** — worth a fix ticket in this epic;
  - **deferred** — real, but not worth fixing in this epic; it becomes a follow-up. A tiny finding
    (a comment, whitespace, or about 10 changed lines or fewer) that has no neighbour in its area to
    merge into is **deferred**, never **approved**: a fix ticket costs a full iteration;
  - **rejected** — dropped.
- Never defer a confirmed security finding: approve it, and reject it only when it is not real. The
  tiny-finding rule above does not apply to it. A security finding is one raised by the security
  reviewer, or by any other reviewer when it describes a vulnerability.
- Never reject a finding whose reason is to amend the spec or docs (for example "change the spec
  wording rather than the code"): the edit still has to happen, and a rejection files nothing.
  Verdict it **approved** or **deferred**, and name the spec or docs edit as the fix.
- For each deferred finding, also give a target and a `category` (`bug` for a defect, else `idea`).
  The target is the real project name, taken from the epic address of the ticket under review
  (the part before the colon). Use `gx` only when the finding passes the
  [routing test](../gx-local-tracker.md) ("would this still exist if the epic had never been
  authored?": yes means gx tooling). Never write the word `project` as the target. Group deferred findings that name the same fix;
  a group counts as one problem, so it files as one follow-up.

The consultant only advises: it must not edit anything. Its output is the input to steps 6 and 7.

## 5. Turn approved findings into tickets

For every finding the consultant approved, call the `gx-to-tickets` skill, targeting the current
epic, with the approved findings (one ticket per independent finding, grouped when several findings
name the same fix) as the input spec instead of a plan/spec document. Before filing, merge every
small fix (about 10 changed lines or fewer, whether code, docs, skill text or comments) with its
neighbours in the same area into one ticket: each ticket pays the full start-up context of a new
session, which costs more than a few lines of change. Follow gx-to-tickets's own
process, skipping its step 5 (quiz the user) since this runs unattended — publish the drafted
breakdown directly, same as any other unattended `gx-to-tickets` run.

Write each fix-ticket body from the consultant's verified facts. Do not re-run greps or `sed -n`
reads to check them again.

The fix-ticket rules (verified "Where to look" paths, real ticket ids, chained `--blocked-by` for
tickets that share a file, acceptance wording checked against existing tests) are in
[gx-to-tickets](../gx-to-tickets/SKILL.md) section 4d. Follow them.

Pass `--parent <this-ticket-id>` on every `gx tickets add` call, as
`gx tickets add <epic> --parent <id> --slug <slug> ...`. (The ticket-address form,
`gx tickets add <epic>/<id> --slug <slug> ...`, needs no `--parent`: it already means "child of that
ticket".) `--parent` makes the new ticket a
scheduling and UI child of this one: the scheduler's scope resolution and the Queue tab's tree
nesting both walk the child's own `parent:` field, the only edge there is, and nothing is recorded
on this ticket. Never set it in a later `gx tickets set`. A running epic's scheduler can claim the
ticket between the two calls, so the parent edge would arrive after pickup.

`gx-to-tickets` already skips appending a trailing `type: code-review` ticket when the target epic
has one — this epic does, namely the ticket this skill is running as — so no
extra guard is needed here to avoid a duplicate.

If the consultant approved zero findings, skip this step entirely; there's nothing to publish.

## 6. File deferred findings as follow-ups

For each deferred group, call the `gx-follow-up` skill, `unattended`, passing the consultant's target
and category unchanged, and the group as the finding. Never replace them with another project (not
`gx`, not this epic's project). If the target is not a registered project, leave it out so
`gx-follow-up` applies its own routing test. Its Source names this epic and this code-review ticket's
address. Its normal dedupe applies, so a group may be appended to an existing follow-up or dropped
against an archived one. Note the address it reports for each group.

If the consultant deferred zero findings, skip this step.

## 7. Close out

- Write the ticket body (append under the existing sections, don't replace them) with three parts:
  - **Raw findings**, per axis and reviewer skill, verbatim from step 3, with the coverage line
    `Spec reviewed N of M code-changing tickets`.
  - **Consultant triage**, verbatim from step 4.
  - **Final disposition** of each finding: the fix ticket it became; for a deferred finding, the
    follow-up it became, was appended to, or was dropped against; or why it was rejected.
- Append your entry to `## Iteration summaries`
  ([format](../gx-local-tracker.md#iteration-summaries), inlined here so you need not look it up):
  a `### <session id> — <RFC 3339 time from date -u +%Y-%m-%dT%H:%M:%SZ> — agent` heading, then four
  one-line bullets `Did`, `Went wrong`, `Decisions`, `Left`. Write it with
  `gx tickets section <addr> "Iteration summaries" -`, which replaces the whole section (it is created
  if missing), so include earlier entries.
- `gx tickets set <addr> --iteration-status finished --commitless true` — this skill never commits
  code of its own, so `commitless: true` is always correct here, not just the no-findings case.
  `--status done` will be refused on a `ralph-loop/*` branch (merge status is gx's alone to set);
  `iteration_status: finished` is the self-report ralph-loop's merge logic actually watches for on
  a claimed ticket, so set it even though `--status done` isn't yours to set directly.
