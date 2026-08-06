---
name: gx-to-tickets
description:
  Break a plan, spec, or the current conversation into a set of tracer-bullet tickets against gx's
  local markdown tracker, each declaring its blocking edges and its approved test seams.
disable-model-invocation: true
---

# gx To Tickets

Break a plan, spec, or conversation into a set of **tickets** — tracer-bullet vertical slices, each
declaring the tickets that **block** it — published to gx's local markdown tracker. See
[gx-local-tracker.md](../gx-local-tracker.md) for the full layout, frontmatter, and CLI contract; this
skill only covers how to break work up and what to put in each ticket.

Estimate the amount of tokens that will be needed for the implementation of each ticket; if a ticket
will need more than 70K tokens, split it.

When estimating, budget for the whole session, not just the diff size. A ticket that only adds ~150
lines can still blow the budget once you add:

- **Full reads of existing files it touches.** If a ticket requires wiring new behavior through an
  existing file over ~300-400 lines (e.g. a central loop/dispatcher), count that file's size against
  the budget every time it's likely to be read (before editing, after review fixes, verification) —
  not once.
- **The verify/code-review pass.** Verification and code review add their own findings + fix + re-run
  cycles on top of the implementation itself; budget for at least one extra read+edit pass over any
  large file the ticket touches.
- **TDD iteration.** If the ticket will be built test-first, budget for multiple test-run cycles, not
  a single build+test at the end.
- **Variant fan-out.** Count how many independent states/variants the acceptance criteria describe
  (e.g. three distinct row badges, or a list-view change plus a separate preview-pane change). Each
  variant typically needs its own wiring and its own test assertion — treat N independent variants
  the same as threading a large file N times, and split by variant group if N is more than a couple.

Separate "the capability doesn't exist yet" from "something consumes a capability that now exists"
into different tickets — a plumbing/infra ticket vs. a feature-on-top ticket. This applies whenever a
ticket both builds a new capability and builds the thing that uses it, not only when a large
pre-existing file is threaded twice (e.g. "add the data/plumbing" ticket vs. "add the user-facing
command" ticket; or "add a new backend control path" vs. "add the UI that calls it").

If tickets earlier in the same epic are already implemented, check their actual cost (`gx tickets
schema`'s `actual_context_window`/`elapsed_time` fields, handoff notes, session logs) before
finalizing later estimates. A cold estimate is a guess; an earlier sibling ticket's actual is real
data about how this epic's tickets run in practice, and should recalibrate the rest of the split —
including retroactively splitting a not-yet-started ticket that now looks oversized in light of it.

## Process

### 1. Gather context

Work from whatever is already in the conversation context. If the user passes a reference (a spec
path, a ticket path) as an argument, read its full body.

### 2. Explore the codebase (optional)

If you have not already explored the codebase, do so to understand the current state of the code.
Ticket titles and descriptions should use the project's domain glossary vocabulary, and respect ADRs
in the area you're touching.

Look for opportunities to prefactor the code to make the implementation easier. "Make the change
easy, then make the easy change."

### 3. Draft vertical slices

Break the work into **tracer bullet** tickets.

<vertical-slice-rules>

- Each slice cuts a narrow but COMPLETE path through every layer (schema, API, UI, tests) —
  vertical, NOT a horizontal slice of one layer
- A completed slice is demoable or verifiable on its own
- Each slice is sized to fit in a single fresh context window
- Any prefactoring should be done first

</vertical-slice-rules>

Give each ticket its **blocking edges** — the other tickets that must complete before it can start.
A ticket with no blockers can start immediately.

**Wide refactors are the exception to vertical slicing.** A **wide refactor** is one mechanical
change — rename a column, retype a shared symbol — whose **blast radius** fans across the whole
codebase, so a single edit breaks thousands of call sites at once and no vertical slice can land
green. Don't force it into a tracer bullet; sequence it as **expand–contract**. First expand: add
the new form beside the old so nothing breaks. Then migrate the call sites over in batches sized by
blast radius (per package, per directory), each batch its own ticket blocked by the expand, keeping
CI green batch to batch because the old form still exists. Finally contract: delete the old form
once no caller remains, in a ticket blocked by every migrate batch. When even the batches can't stay
green alone, keep the sequence but let them share an integration branch that all block a final
integrate-and-verify ticket — green is promised only there.

**Explore/implement split for foreseeably-wide test/e2e changes.** If a ticket touches test or e2e
coverage and it's foreseeable in advance that implementing it requires reading a lot of files first
(e.g. surveying an existing test suite's conventions, fixtures, and helpers across many files before
writing new cases), split it into two tickets instead of one, blocked in sequence:

1. **Explore**: read the necessary files and write the findings — conventions, fixtures, helpers,
   the concrete approach — into this ticket's own body (its answer/notes), with no implementation.
2. **Implement**: blocked by the explore ticket. Read its findings and implement from those notes
   instead of re-exploring from scratch.

This keeps each ticket within budget and avoids one session paying for both wide exploration and
implementation.

### 4. Declare each ticket's test seams

Every ticket must declare its **approved public test seams** under a `## Test seams` heading — the
public interfaces `gx-tdd` (or whatever implements the ticket) is allowed to test against. This
skill runs unattended under ralph-loop, so the seam decision has to be made now, in the ticket, not
negotiated live with a human later.

For each seam, name the interface and what behavior it should demonstrate — not a specific assertion
or test name. If no automated seam is appropriate for a ticket (pure documentation, a design-only
split, exploratory research), write an explicit `none` entry with the rationale — never leave the
section silent.

### 5. Quiz the user

Present the proposed breakdown as a numbered list. For each ticket, show:

- **Title**: short descriptive name
- **Blocked by**: which other tickets (if any) must complete first
- **What it delivers**: the end-to-end behaviour this ticket makes work
- **Test seams**: the seams from step 4
- **Estimated context window**: the token estimate behind step 3's split decision — always shown, not
  just when a ticket is borderline, so the user can judge granularity against the same number that
  drove the split

Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the blocking edges correct — does each ticket only depend on tickets that genuinely gate it?
- Are the declared seams the right ones to test against?
- Should any tickets be merged or split further?

Iterate until the user approves the breakdown. Skip this step when running unattended (e.g. inside
ralph-loop) — publish the breakdown as drafted and let the tickets themselves carry the record of
what was decided.

### 6. Publish the tickets

Write one file per ticket under `.scratch/<epic-slug>/issues/<NN>-<slug>.md`, numbered from `01` in
dependency order (blockers first). Each file's `blocked_by` lists the ticket IDs it depends on. Use
the per-ticket template below — one ticket per file, never a single combined file.

Work the **frontier** (see [gx-local-tracker.md](../gx-local-tracker.md)): any ticket whose blockers are
all done. For a purely linear chain that means top to bottom.

Do NOT close or modify any parent/source ticket this epic was generated from.

Before considering any ticket published, run `gx tickets validate <path>` on it. Fix any reported
error and re-validate until it passes — do not publish a ticket that fails validation.

Tickets can also be split off **mid-flight**, by `gx-implement`, when a ticket outgrows its budget
while in progress — same template, same publishing mechanics, just triggered from inside a running
session instead of upfront here. See [gx-local-tracker.md](../gx-local-tracker.md)'s mid-flight splitting
section for the numbering, blocking-edge, and `children`/`parent` mechanics.

If the breakdown appends a trailing `type: code-review` ticket, first check whether the target epic
already has one among its published issues. If it does, skip the append — don't publish a second
one — unless the user has explicitly asked for another.

<ticket-template>

---
id: "&lt;NN&gt;"
status: ready-for-agent
blocked_by: []
children: []
type: task
expected_context_window: 20000
---
# &lt;NN&gt; — &lt;Ticket title&gt;

## What to build

The end-to-end behaviour this ticket makes work, from the user's perspective — not a layer-by-layer
implementation list.

## Test seams

- Seam 1: the public interface and the behavior it should demonstrate.
- Seam 2: ...

(Or, when no automated seam applies: `none — <rationale>`.)

## Acceptance criteria

- [ ] Acceptance criterion 1
- [ ] Acceptance criterion 2

</ticket-template>

### Frontmatter fields

See [gx-local-tracker.md](../gx-local-tracker.md) for the full field reference. New tickets are published
as `ready-for-agent` unless instructed otherwise, with `blocked_by: []` when nothing gates them.

Avoid specific file paths or code snippets in the body — they go stale fast. Exception: if a
prototype produced a snippet that encodes a decision more precisely than prose can (state machine,
reducer, schema, type shape), inline it and note briefly that it came from a prototype. Trim to the
decision-rich parts — not a working demo, just the important bits.
