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

Change files with `Edit`/`Write` only, never `sed -i`, `cat >` or a heredoc. Write the file first
(a scratch body, say), then use it. The guard hook rejects shell file writes.

Before starting, run `gx tickets validate <addr>` on the ticket you're about to claim. If it fails,
stop and fix the ticket's frontmatter before doing anything else.

## 1. Resolve the epic diff and its spec

- The epic slug is the middle part of the ticket address (`project:<epic-slug>/NN`). Read the ticket
  with `gx tickets show <addr>` and the epic's other tickets by address too
  (`gx tickets epics` lists epics) — never open ticket files by path.
- The epic's branch is the current branch (every ralph-loop worktree runs on a `ralph-loop/`-prefixed
  branch, one per epic — see gx-implement).
- Capture the diff scope once, the same way `code-review` does it: `git merge-base main HEAD` for the
  fixed point, `git diff <fixed-point>...HEAD` for the diff, `git log <fixed-point>..HEAD --oneline`
  for the commit list. Fail here — not inside a subagent — if the merge-base can't be resolved or the
  diff is empty.
- The epic's spec is the first of these that exists: `docs/specs/<epic-slug>.md` from the repo root,
  then `<epic-dir>/spec.md` (legacy location). When neither does, the epic's ticket bodies are the
  spec: every other ticket in the epic except this code-review ticket, each read in full with
  `gx tickets show <addr>`. Either way the
  spec resolves — the Spec axis always runs.
- Collect the **known spec deviations**: read each of this epic's other tickets with
  `gx tickets show <addr>` (only this epic's; skip this code-review ticket) and gather the bullets
  under its `## Spec deviations` section (also check `## Comments` and handoff notes for older
  tickets that recorded one there). Join them into one short list, each line prefixed with its
  ticket address. If there are none, the list is empty.

## 2. Read the configured review skills

`gx config show` prints the effective config as JSON; read `.skills["code-review"]`. If the list is
empty or the field is absent, fall back to a single-element list: `["gx-thermo-nuclear-code-quality-review"]`.

Resolve each name to a `SKILL.md` path, taking the first that exists: `skills/<name>/SKILL.md` from
the reviewed repo's root, then `../<name>/SKILL.md` relative to this file. Subagents Read that file
rather than call the Skill tool, because a skill with `disable-model-invocation: true` (the vendored
default one keeps upstream's flag) is not listed there, and the quality axis used to degrade silently
into a manual review.

If any name resolves to no file, don't review with a partial list. Follow
[gx-local-tracker.md](../gx-local-tracker.md)'s announce-and-stop rule: run `gx tickets ask`, with the question naming which names didn't resolve
and the two paths tried for each, and end your turn.

## 3. Run both axes as independent parallel subagents

### Size gate

Count the changed lines (`git diff <fixed-point>...HEAD --shortstat`: insertions plus deletions). At
or under about 50 changed lines, skip the Quality subagents: a configured review skill costs far more
than it finds on a diff that small. The Spec subagent still runs. Note the skip in the ticket body at
step 7.

### Shared review file

Reviewers must not each re-run the diff and the same searches. Write one file in your scratchpad
directory (with `Write`) holding the fixed point, the commit list, the changed-file list, the full
diff, and the output of any search you already ran (say, a `rg` for a symbol the diff changes). Pass
its path to every subagent; they read that file instead of running `git diff` themselves. They may
Read a changed file for more context, but not re-run the diff or a search the file already holds.

Every subagent prompt also says: "You must verify a suspicion before you report it: read the file or
run the check yourself. Do not flag something as possible or unverified. Drop it if you cannot
confirm it."

### Launch

Send **one message** carrying every `Agent` tool call — one per configured skill name (none when the
size gate tripped), plus the one Spec call below (`general-purpose` subagents, run in the foreground
since the next step needs every result back before it can start). Running them in the same message
is what makes the passes independent instead of serialized, and it is what puts the Spec axis
alongside the quality passes rather than after them.

Every subagent gets the shared review file's path from above.

**Quality subagents** (one per configured skill) also get:

- The `SKILL.md` path resolved in step 2, with the instruction to Read it and follow it against that
  diff.
- The spec path (or ticket address list) resolved in step 1, and the known spec deviations list,
  with the rule: "Don't propose changes that undo an explicit requirement of the spec or tickets.
  Flag the tension instead."
- The brief: "Report findings as a list — file, line/hunk, summary, severity, suggested fix. Under
  500 words."

**Spec subagent** (exactly one, whatever the configured skill list holds) also gets:

- The path of the spec resolved in step 1 — `docs/specs/<epic-slug>.md` (or the legacy `spec.md`), or
  the list of ticket addresses that stand in for it.
- The known spec deviations list from step 1, with: "These narrowings were deliberate. Don't report
  them as missing or wrong; only report one if the diff contradicts what the deviation says."
- The brief: "Report: (a) requirements the spec asked for that are missing or partial; (b) behaviour
  in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where
  the implementation looks wrong. Quote the spec line — or the ticket address and line — for each
  finding. Under 400 words."

Collect every subagent's raw findings verbatim; don't summarize or filter them yourself here.

## 4. Triage with a higher-tier consultant

Once every review subagent has returned, start **one** fresh-context, read-only consultant subagent
(same pattern as `~/.dotfiles/core/ai/skills/consult`'s consultant-selection step: request the
strongest tier — pass the output of `gx agent model strongest` as the `Agent` call's model).

Brief it with every subagent's raw findings joined into one list, each labeled by the axis and skill
that produced it (`Quality / <skill-name>`, or `Spec`), plus the epic's ticket list for context (what
already shipped, what the epic was for) and the known spec deviations list from step 1. Ask it to:

- Treat a finding that only restates a known spec deviation as **rejected** (reason: deliberate
  deviation, with the ticket address).

- Consolidate duplicate findings raised by more than one reviewer, across axes as well as within one.
- Read the two axes against each other: a quality finding on code the Spec axis flags as scope creep
  argues for deleting the code rather than polishing it.
- Prioritize by actual impact, not by how many reviewers happened to flag it.
- Give each finding one verdict and a one-line reason:
  - **approved** — worth a fix ticket in this epic;
  - **deferred** — real, but not worth fixing in this epic; it becomes a follow-up;
  - **rejected** — dropped.
- For each deferred finding, also give a target (`gx` for a gx tooling finding, else `<project>`)
  and a `category` (`bug` for a defect, else `idea`). Group deferred findings that name the same fix;
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

Pass `--parent <this-ticket-id>` on every `gx tickets add` call. `--parent` makes the new ticket a
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
and category, and the group as the finding. Its Source names this epic and this code-review ticket's
address. Its normal dedupe applies, so a group may be appended to an existing follow-up or dropped
against an archived one. Note the address it reports for each group.

If the consultant deferred zero findings, skip this step.

## 7. Close out

- Write the ticket body (append under the existing sections, don't replace them) with three parts:
  - **Raw findings**, per axis and reviewer skill, verbatim from step 3.
  - **Consultant triage**, verbatim from step 4.
  - **Final disposition** of each finding: the fix ticket it became; for a deferred finding, the
    follow-up it became, was appended to, or was dropped against; or why it was rejected.
- `gx tickets set <addr> --iteration-status finished --commitless true` — this skill never commits
  code of its own, so `commitless: true` is always correct here, not just the no-findings case.
  `--status done` will be refused on a `ralph-loop/*` branch (merge status is gx's alone to set);
  `iteration_status: finished` is the self-report ralph-loop's merge logic actually watches for on
  a claimed ticket, so set it even though `--status done` isn't yours to set directly.
