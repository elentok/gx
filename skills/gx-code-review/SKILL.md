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

Before starting, run `gx tickets validate <path>` on the ticket you're about to claim. If it fails,
stop and fix the ticket's frontmatter before doing anything else.

## 1. Claim the ticket

`gx tickets set <path> --status claimed`, same as gx-implement.

## 2. Resolve the epic diff and its spec

- The epic directory is the ticket's `issues/` parent's parent: `<root>/<epic-slug>/`. Derive it from
  the claimed ticket's own path — don't reconstruct it via `gx tickets root` plus a guessed epic slug.
- The epic's branch is the current branch (every ralph-loop worktree runs on a `ralph-loop/`-prefixed
  branch, one per epic — see gx-implement).
- Capture the diff scope once, the same way `code-review` does it: `git merge-base main HEAD` for the
  fixed point, `git diff <fixed-point>...HEAD` for the diff, `git log <fixed-point>..HEAD --oneline`
  for the commit list. Fail here — not inside a subagent — if the merge-base can't be resolved or the
  diff is empty.
- The epic's spec is the first of these that exists: `<epic-dir>/spec.md`, then
  `docs/specs/<epic-slug>.md` from the repo root. When neither does, the epic's ticket bodies are the
  spec: every `<epic-dir>/issues/*.md` except this code-review ticket, read in full. Either way the
  spec resolves — the Spec axis always runs.

## 3. Read the configured review skills

`gx config show` prints the effective config as JSON; read `.skills["code-review"]`. If the list is
empty or the field is absent, fall back to a single-element list: `["thermo-nuclear-code-quality-review"]`.

## 4. Run both axes as independent parallel subagents

Send **one message** carrying every `Agent` tool call — one per configured skill name, plus the one
Spec call below (`general-purpose` subagents, run in the foreground since the next step needs every
result back before it can start). Running them in the same message is what makes the passes
independent instead of serialized, and it is what puts the Spec axis alongside the quality passes
rather than after them.

Every subagent gets the fixed point, the diff command, and the commit list from step 2 — each one has
read/git access to the worktree, so it runs the diff itself.

**Quality subagents** (one per configured skill) also get:

- The instruction to invoke `Skill(skill: "<configured-skill-name>")` and follow it against that diff.
- The brief: "Report findings as a list — file, line/hunk, summary, severity, suggested fix. Under
  500 words."

**Spec subagent** (exactly one, whatever the configured skill list holds) also gets:

- The path of the spec resolved in step 2 — `spec.md`, or the list of ticket file paths that stand in
  for it.
- The brief: "Report: (a) requirements the spec asked for that are missing or partial; (b) behaviour
  in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where
  the implementation looks wrong. Quote the spec line — or the ticket path and line — for each
  finding. Under 400 words."

Collect every subagent's raw findings verbatim; don't summarize or filter them yourself here.

## 5. Triage with a higher-tier consultant

Once every review subagent has returned, start **one** fresh-context, read-only consultant subagent
(same pattern as `~/.dotfiles/core/ai/skills/consult`'s consultant-selection step: request the
highest available Opus model — `model: "opus"` on the `Agent` tool call).

Brief it with every subagent's raw findings joined into one list, each labeled by the axis and skill
that produced it (`Quality / <skill-name>`, or `Spec`), plus the epic's ticket list for context (what
already shipped, what the epic was for). Ask it to:

- Consolidate duplicate findings raised by more than one reviewer, across axes as well as within one.
- Read the two axes against each other: a quality finding on code the Spec axis flags as scope creep
  argues for deleting the code rather than polishing it.
- Prioritize by actual impact, not by how many reviewers happened to flag it.
- Decide, for each finding, whether it's worth a follow-up ticket — and give a one-line reason either
  way (approved / rejected / deferred).

The consultant only advises: it must not edit anything. Its output is the input to step 6.

## 6. Turn approved findings into tickets

For every finding the consultant approved, call the `gx-to-tickets` skill, targeting the current
epic, with the approved findings (one ticket per independent finding, grouped when several findings
name the same fix) as the input spec instead of a plan/spec document. Follow gx-to-tickets's own
process, skipping its step 5 (quiz the user) since this runs unattended — publish the drafted
breakdown directly, same as any other unattended `gx-to-tickets` run.

`gx-to-tickets` already skips appending a trailing `type: code-review` ticket when the target epic
has one (ticket 07's update) — this epic does, namely the ticket this skill is running as — so no
extra guard is needed here to avoid a duplicate.

If the consultant approved zero findings, skip this step entirely; there's nothing to publish.

## 7. Close out

- For every ticket `gx-to-tickets` created in step 6, `gx tickets set <new-ticket-path> --parent
  <this-ticket-id>` — this is what makes the new ticket a scheduling/UI child of this one (the
  scheduler's scope resolution and the Queue tab's tree nesting both walk the child's own `parent:`
  field, the only edge there is; nothing is recorded on this ticket). Do this before the next step,
  since a running epic's scheduler may pick up the new tickets as soon as they're published.
- Write the ticket body (append under the existing sections, don't replace them) with three parts:
  - **Raw findings**, per axis and reviewer skill, verbatim from step 4.
  - **Consultant triage**, verbatim from step 5.
  - **Final disposition** of each finding: which ticket it became, or why it was rejected/deferred.
- `gx tickets set <path> --status done --commitless true` — this skill never commits code of its own,
  so `commitless: true` is always correct here, not just the no-findings case.
