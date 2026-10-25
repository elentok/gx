---
name: gx-to-tickets
description:
  Break a plan, spec, or the current conversation into a set of tracer-bullet tickets against gx's
  local markdown tracker, each declaring its blocking edges and its approved test seams.
disable-model-invocation: false
---

# gx To Tickets

Break a plan, spec, or conversation into a set of **tickets** — tracer-bullet vertical slices, each
declaring the tickets that **block** it — published to gx's local markdown tracker. See
[gx-local-tracker.md](../gx-local-tracker.md) for the full layout, frontmatter, and CLI contract;
this skill only covers how to break work up and what to put in each ticket.

ralph-loop runs the published epic unattended, ticket by ticket, with no session-length limit — it
will happily churn through however many tickets the epic has, overnight if needed. The only budget
that matters is **context window per ticket**, never wall-clock time for the epic as a whole: split
a ticket because its implementation won't fit one fresh context window, not because the epic "would
take a while" to get through.

Estimate the amount of tokens that will be needed for the implementation of each ticket; if a ticket
will need more than 70K tokens, split it.

When estimating, budget for the whole session, not just the diff size. A ticket that only adds ~150
lines can still blow the budget once you add:

- **Full reads of existing files it touches.** If a ticket requires wiring new behavior through an
  existing file over ~300-400 lines (e.g. a central loop/dispatcher), count that file's size against
  the budget every time it's likely to be read (before editing, after review fixes, verification) —
  not once.
- **The verify/code-review pass.** Verification and code review add their own findings + fix +
  re-run cycles on top of the implementation itself; budget for at least one extra read+edit pass
  over any large file the ticket touches.
- **TDD iteration.** If the ticket will be built test-first, budget for multiple test-run cycles,
  not a single build+test at the end.
- **Variant fan-out.** Count how many independent states/variants the acceptance criteria describe
  (e.g. three distinct row badges, or a list-view change plus a separate preview-pane change). Each
  variant typically needs its own wiring and its own test assertion — treat N independent variants
  the same as threading a large file N times, and split by variant group if N is more than a couple.
  This is not optional when the variants also fall in different test files (see next bullet) — that
  conjunction, not variant count alone, is what blows a budget: one past ticket named 3
  variants sharing one root cause and forked 4 times before merging, because the variant count alone
  didn't trigger a split.
- **Test-file fan-out.** Count how many *distinct existing test files* the ticket's Test Seams will
  need new cases in. Each file usually means its own read of existing fixtures/conventions before a
  new case can be written — 3+ files is the same cost shape as the explore/implement split below,
  even if the ticket reads as "mostly implementation" rather than "mostly tests." Don't let a single
  `expected_context_window` number smooth over this; split by file group (or route through
  explore/implement, below) once a ticket's Test Seams section names 3+ files.

Separate "the capability doesn't exist yet" from "something consumes a capability that now exists"
into different tickets — a plumbing/infra ticket vs. a feature-on-top ticket. This applies whenever
a ticket both builds a new capability and builds the thing that uses it, not only when a large
pre-existing file is threaded twice (e.g. "add the data/plumbing" ticket vs. "add the user-facing
command" ticket; or "add a new backend control path" vs. "add the UI that calls it").

If tickets earlier in the same epic are already implemented, check their actual cost
(`gx tickets schema`'s `actual_context_window`/`elapsed_time` fields, handoff notes, session logs)
before finalizing later estimates. A cold estimate is a guess; an earlier sibling ticket's actual is
real data about how this epic's tickets run in practice, and should recalibrate the rest of the
split — including retroactively splitting a not-yet-started ticket that now looks oversized in light
of it.

## Process

### 1. Gather context

Work from whatever is already in the conversation context. If the user passes a reference (a spec
path, a ticket address) as an argument, read its full body (`gx tickets show <addr>` for a ticket).

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

Every set of tickets that share the same blockers runs **in parallel** — ralph-loop starts them
together, each on its own branch, and cherry-picks them back one by one. Two rules follow:

- **Siblings that edit the same files get chained, not fanned out.** If two tickets would touch the
  same source file, test file, or test fake, make one block the other. A common blocker alone
  guarantees cherry-pick conflicts and conflict-fix child tickets. `blf-herdr-impl` gave three
  tickets editing one command file and its test the same single blocker; all three conflicted.
- **A cross-cutting ticket (logging, config, metrics, tracing) is blocked by the features it
  instruments** — or, if it's plumbing they should call, the features are blocked by it. Never run
  it beside them: it can't see code that isn't merged yet, so it ships incomplete and code review
  spawns a follow-up to wire it in.

**Wide refactors are the exception to vertical slicing.** A **wide refactor** is one mechanical
change — rename a column, retype a shared symbol — whose **blast radius** fans across the whole
codebase, so a single edit breaks thousands of call sites at once and no vertical slice can stay
green. Don't force it into a tracer bullet; sequence it as **expand–contract**. First expand: add
the new form beside the old so nothing breaks. Then migrate the call sites over in batches sized by
blast radius (per package, per directory), each batch its own ticket blocked by the expand, keeping
CI green batch to batch because the old form still exists. Finally contract: delete the old form
once no caller remains, in a ticket blocked by every migrate batch. When even the batches can't stay
green alone, keep the sequence but let them share an integration branch that all block a final
integrate-and-verify ticket — green is promised only there.

**Explore/implement split for foreseeably-wide test/e2e changes.** Trigger this whenever a ticket's
Test Seams will fall in 3+ distinct existing test files (test-file fan-out, above) — not only when
the ticket reads as "mostly test work" as a whole. It's foreseeable in advance that implementing it
requires reading a lot of files first (e.g. surveying an existing test suite's conventions,
fixtures, and helpers across many files before writing new cases); split it into two tickets
instead of one, blocked in sequence:

1. **Explore**: read the necessary files and write the findings — conventions, fixtures, helpers,
   the concrete approach — into this ticket's own body (its answer/notes), with no implementation.
2. **Implement**: blocked by the explore ticket. Read its findings and implement from those notes
   instead of re-exploring from scratch.

This keeps each ticket within budget and avoids one session paying for both wide exploration and
implementation.

An explore ticket stays `type: implement`. **Never publish a `type: research` ticket into an epic this
skill produces.** A loop-driven epic contains work ralph-loop claims and closes; `research` is a
diagnosis or a decision handed to a *person* to plan, which is why `gx-investigate` files one as
`draft` — deliberately not schedulable. Publishing one `open` inside an epic makes a hand-driven
decision schedulable and hands a design question to an unattended agent that cannot ask you
anything.

When the work needs a decision rather than a fact, it belongs in a charting session (`/gx-design`)
*before* this skill runs — that's where grilling lives, and grilling must never end up
inside an implementation ticket. Bounded fact-finding that genuinely belongs inside the epic is an
explore ticket, typed `implement`. A diagnosis or idea for a person to plan is a follow-up: file it
with the `gx-follow-up` skill (passing `unattended` when this skill runs unattended), never with a
plain `add`.

### 4. Declare each ticket's test seams

Every ticket must declare its **approved public test seams** under a `## Test seams` heading — the
public interfaces `gx-tdd` (or whatever implements the ticket) is allowed to test against. This
skill runs unattended under ralph-loop, so the seam decision has to be made now, in the ticket, not
negotiated live with a human later.

For each seam, name the interface and what behavior it should demonstrate — not a specific assertion
or test name. If no automated seam is appropriate for a ticket (pure documentation, a design-only
split, exploratory research), write an explicit `none` entry with the rationale — never leave the
section silent.

### 4b. Write each implement ticket's "Where to look" map

Every `type: implement` ticket must carry a `## Where to look` section. You already have this from
the planning pass; the implementing agent starts cold and otherwise spends many turns (and often a
context overrun) rediscovering it, then misses shared fixtures and fails the full suite late.

- **Entry points**: `file:func` where the change starts.
- **Test helpers**: the existing helpers and fakes for each named seam (not just "the server with
  runnerfake" — name the helper and the file it lives in).
- **Shared contract**: callers, tests and fixtures that depend on a contract the ticket changes, and
  which side of a package split (e.g. `server/` vs `ralphloop/`) each call site sits on.
- **Regression data**: inline any transcript or fixture the ticket relies on. Never point at another
  ticket for it.
- **Consumers of a changed field or enum**: when the ticket changes a wire field (JSON tag,
  `omitempty`), a schema field or an enum, run `rg` for the symbol and list every consumer: loaders
  and in-memory structs, server and CLI call sites, web code, and comments that mirror it (e.g. a
  "must match" note). Put every existing test that mentions it in Test seams, so the agent re-runs
  them instead of finding them by failure.
- **Wiring**: name the hidden registration points (route tables, allowlists, parity tests, the
  query-and-prop path that feeds a page) and give `file:func` for anything the ticket says only by
  line number or role. For a new server route, point at `.ai/index.md` "Adding a server route".
- **Behaviour that must not change**: list one seam per state or variant of that behaviour, not only
  the ones the ticket edits.

If no map applies, write an explicit `none — <rationale>`, as with test seams. Code-review fix
tickets are out of scope here.

### 5. Quiz the user

Present the proposed breakdown as a numbered list. For each ticket, show:

- **Title**: short descriptive name
- **Blocked by**: which other tickets (if any) must complete first
- **What it delivers**: the end-to-end behaviour this ticket makes work
- **Test seams**: the seams from step 4
- **Estimated context window**: the token estimate behind step 3's split decision — always shown,
  not just when a ticket is borderline, so the user can judge granularity against the same number
  that drove the split

Before the list, show the draft epic Summary (written as step 6 describes) so the user can correct
it with the rest of the breakdown.

Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the blocking edges correct — does each ticket only depend on tickets that genuinely gate it,
  and are same-file siblings and cross-cutting tickets chained rather than run in parallel?
- Are the declared seams the right ones to test against?
- Should any tickets be merged or split further?

Iterate until the user approves the breakdown. Skip this step when running unattended (e.g. inside
ralph-loop) — publish the breakdown as drafted and let the tickets themselves carry the record of
what was decided.

### 6. Publish the tickets

Create a new implementation epic as a `draft` first, so the scheduler cannot queue it before its
tickets exist:

```sh
gx tickets epic add <project:epic> --kind loop --status draft
```

Skip this when publishing into an epic that already exists. Opening the epic is the very last step,
after the Summary and `ensure-code-review` (see the end of this step).

Create one ticket per call (see [gx-local-tracker.md](../gx-local-tracker.md)), in dependency order
(blockers first); it allocates the ID and prints the ticket's address:

```sh
gx tickets add <epic-slug> --slug <slug> --type implement --expected-context-window 20000 \
  [--blocked-by 01,03] --body -
```

`--blocked-by` lists the ticket IDs it depends on; leave it out when nothing gates the ticket. Use
the per-ticket template below as the body — one ticket per call, never a single combined file. Write
each body to a scratchpad file with Write and pipe it in; never a heredoc, never edit a ticket file
by path (see "Write a body once" in the tracker).

Always write a `## Summary` on the epic as part of publishing. Never skip it, never block publishing
on it, and never copy a map Destination into it:

```sh
gx tickets section <project:epic>-impl Summary -
```

- One paragraph, at most 3 sentences and about 60 words, in B2 English and in `GLOSSARY.md` terms.
- Say what the epic delivers and why, from the user's view. No ticket numbers, file paths or code
  names.
- Source: the spec's Problem Statement and Solution (after review amendments), rewritten, not
  copied. With no spec, summarize the plan or the conversation. With no clear summary, still write
  one from whatever is available.
- An unattended run (step 5 is skipped) publishes the summary as drafted.

Work the **frontier** (see [gx-local-tracker.md](../gx-local-tracker.md)): any ticket whose blockers
are all done. For a purely linear chain that means top to bottom.

Do NOT close or modify any parent/source ticket this epic was generated from.

An epic is either hand-driven or loop-driven, never both. When the source you're breaking up is a
wayfinder map, publish the implementation tickets into a **new** epic directory — never into the map
epic's own `issues/` (a `kind: map` epic, which the server never schedules). A map's tickets are
decisions its author resolves by hand, through `gx tickets set`; implementation tickets are claimed and closed by ralph-loop, which takes ownership
of `status` for the whole epic it runs. Putting both kinds in one directory puts two writers on the
same status field.

Before considering any ticket published, run `gx tickets validate <addr>` on it. Fix any reported
error and re-validate until it passes — do not publish a ticket that fails validation.

Tickets can also be forked off **mid-flight**, by `gx-implement`, when a ticket outgrows its budget
while in progress — same template, same publishing mechanics, just triggered from inside a running
session instead of upfront here. See [gx-local-tracker.md](../gx-local-tracker.md)'s mid-flight
forking section for the numbering, blocking-edge, and `parent` mechanics.

If the breakdown appends a trailing `type: code-review` ticket, run
`gx tickets ensure-code-review <epic-slug>` instead of publishing it by hand — it's a no-op if the
epic already has a code-review ticket among its published issues, else it stamps out the stub. Skip
running it only if the user has explicitly asked for a second code-review ticket.

Then run `gx tickets ensure-verify <epic-slug>` to stamp out the epic's `type: verify` ticket. Skip
it when the epic has no user-visible behavior, meaning the spec changes no web, CLI or server
behavior (a docs-only, skills-only or refactor epic).

If the spec has a `## Verification` section that lists new flows or scenarios to write, turn each
one into an implementation ticket in the breakdown, like any other piece of work. The verify ticket
runs those flows; it only writes one itself when it is small (see gx-verify-epic).

Last, once every ticket is written and validated, the Summary is written and `ensure-code-review`
(and `ensure-verify`, when it ran) have run, open the epic you created as a draft: `gx tickets set <project:epic>-impl --status open`.
Nothing comes after this call.

<ticket-template>

```markdown
# <Ticket title>

## What to build

The end-to-end behaviour this ticket makes work, from the user's perspective — not a layer-by-layer
implementation list.

## Test seams

- Seam 1: the public interface and the behavior it should demonstrate.
- Seam 2: ...

(Or, when no automated seam applies: `none — <rationale>`.)

## Where to look

- Entry points: `file:func` ...
- Test helpers: ...
- Shared contract (callers, tests, fixtures): ...

(Or: `none — <rationale>`.)

## Acceptance criteria

- [ ] Acceptance criterion 1
- [ ] Acceptance criterion 2
```

</ticket-template>

### Frontmatter fields

See [gx-local-tracker.md](../gx-local-tracker.md) for the full field reference. New tickets are
published as `open` unless instructed otherwise, with no `--blocked-by` when nothing gates them —
`gx tickets add --body` writes the body, the fields and the status in one write, so there's no window where an
empty ticket is schedulable. Without `--body`, the stub is
`draft`; fill in the body first, then `gx tickets set <addr> --status open` to publish it.

`parent` is only for descendants — a mid-flight fork or a code-review fix ticket — and is written on
the descendant alone, with `--parent <id>` on `gx tickets add`, never a later `gx tickets set`
(the scheduler can claim the ticket in between); a ticket published from a plan or spec has no `parent` and records nothing
about the tickets that follow it.

Avoid specific file paths or code snippets in the body — they go stale fast. Exceptions: the
`## Where to look` section, which exists to name them, and a
snippet from a prototype. If a prototype produced a snippet that encodes a decision more precisely than prose can (state machine,
reducer, schema, type shape), inline it and note briefly that it came from a prototype. Trim to the
decision-rich parts — not a working demo, just the important bits.
