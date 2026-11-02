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
will need more than 60K tokens of work, split it. The 60K counts the ticket's work only.

The estimate is the ticket's work only. gx adds the start-up prefix itself. Cold estimates run 1.12
to 1.28 times the real work. They look low only because the actual context window keeps the prefix.
Estimate lower: multiply your first guess by 0.83. When a ticket sits close to the 60K limit, or
its prose does not list the files it touches, split it instead of trusting the number. A ticket
that runs long also costs more per token, since every turn re-reads the context.

When estimating, budget for the work, not just the diff size. A ticket that only adds ~150
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
  explore/implement, below) once a ticket's Test Seams section names 3+ files. `publish` warns on 5
  or more acceptance criteria with 5 or more files; a warning means you re-check the slice before
  going on. The 3-or-more-test-files rule is not shown to predict overruns (precision 0.18, recall
  0.31), so treat it as a prompt to look, not a limit.

Separate "the capability doesn't exist yet" from "something consumes a capability that now exists"
into different tickets — a plumbing/infra ticket vs. a feature-on-top ticket. This applies whenever
a ticket both builds a new capability and builds the thing that uses it, not only when a large
pre-existing file is threaded twice (e.g. "add the data/plumbing" ticket vs. "add the user-facing
command" ticket; or "add a new backend control path" vs. "add the UI that calls it").

Before finalizing estimates in an epic that already has finished tickets, run
`gx tickets calibration <project:epic>`. With 3 or more finished tickets, apply the epic's ratio
instead of 0.83 to the tickets not yet started, and split any ticket that now looks oversized. With
fewer, keep 0.83. The command never changes a number; you apply the ratio by hand.

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

A UI or e2e explore ticket must run the suite and check locators and section placement against the
running UI, not by reading code. Its findings must say which flows fail now. Write that into the
explore ticket's acceptance criteria. The implement ticket then runs one flow at a time with
`-g <flow>` (see `gx-ui-check`).

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

Every `implement` ticket must declare its **approved public test seams** under a `## Test seams`
heading. Verify and research tickets have no such section. Any non-blank line under the heading
passes, including `none — <reason>`. The seams are the public interfaces `gx-tdd` (or whatever implements the ticket) is allowed to test against. This
skill runs unattended under ralph-loop, so the seam decision has to be made now, in the ticket, not
negotiated live with a human later.

For each seam, name the interface and what behavior it should demonstrate — not a specific assertion
or test name. If no automated seam is appropriate for a ticket (pure documentation, a design-only
split, exploratory research), write an explicit `none` entry with the rationale — never leave the
section silent.

A seam may name the narrow `gx brief` command (e.g. `gx brief go-test -run 'TestX' ./pkg`) as an
iteration hint. It is not the end-of-ticket check: that is always `gx check`.

For a ticket that changes **skill text**, the seam is one shared test file per skill area (extend the
existing one, e.g. `skills/mock_rules_test.go`), and each phrase is asserted once. Name the anchors as
structural (headings, link paths, selector names, numbers), never long prose.

For a **bug-fix ticket**, the regression seam must also say "the test must fail on the old code" and
name the test form the Decisions chose (table test, server-level test, ...). A test that passes on
the old code proves nothing. A test that reads a new field or calls a new function cannot run on the
old code: it does not compile. For such a seam write "must fail to compile or fail on the old code",
and the agent confirms the compile error as the red step. When a seam names `ClaimNext`, name the queue
step that feeds it too (e.g. `h.Client.QueueAdd(...)`), or the test never reaches the branch under test.

For every seam file, state what the agent cannot guess:

- **Test environment**: name the environment of each seam file (read its first lines for
  `// @vitest-environment node`). A seam that checks the DOM (`data-state`, roles) in a node-environment
  file cannot be met; name a DOM-environment file or say to add one.
- **Playwright filters**: give a `-g` pattern that matches the full test title. Check it with
  `gx brief -C webui playwright` on the flow numbers. A `^` anchor finds no tests, because Playwright
  matches `-g` against the full title, which starts with the project and file prefix.
- **Playwright skill**: when the seams include a Playwright file, tell the agent to load
  `gx-playwright-e2es` (its rules on waits before state reads and on `-g` filters).
- **Generated files**: when the ticket says "regenerate", name the exact command (e.g.
  `make web-types`) after checking the Makefile target exists.
- **Mock path**: for a ticket that builds from a stored mock, give the mock HTML path
  (`$(gx tickets root)/<map>/mocks/<file>.html`), not only its name.
- **Examples against the helper**: when a ticket reuses a predicate or helper (e.g.
  `!needsTranscript(failure)`) and also gives examples of what it shows or hides, read the helper and
  check each example against it. If they disagree, say which one wins.

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
- **Removed or changed behaviour**: when the ticket deletes or changes behaviour, run `rg` for the
  removed names and the changed label or text across all tests, e2e flows and fixtures. Read every
  hit in full (no `| head`). List each affected test in Test seams, and give a scoped run (`-run`
  pattern or file list) that covers every hit. Don't scope by the new feature's name.
- **Caller lists are verified, not recalled**: build every list of call sites with gopls
  `go_symbol_references` (not from memory or a ticket's prose) and stamp it `verified at {{commit}}`;
  `publish` fills in the commit. If gopls is unavailable, use `rg` and label the list
  `not verified`, so the implementer knows to re-check it.
- **Doc pointers**: cite only a doc section you opened and confirmed exists (file and heading). A
  pointer to guidance that is not there sends the agent searching.

### 4c. Check every claim against the code

A ticket that states something the code contradicts costs the implementing agent a deviation and
several runs. Before writing a ticket, check each of these against the code with `rg` or `fd`, and
cite the file (`file:func`) in the ticket next to the claim:

- **Current behaviour**: every sentence of the form "X shows / allows / hides / does Y". Read the
  function that decides it (e.g. an `isMergeable` or `isExpanded` predicate), not just the label.
  If the code or an existing e2e flow disagrees with the spec, say so in the ticket and state which
  one wins.
- **Migration numbers**: read the highest one now in use; never write "from 50 up" from memory.
  Ticket IDs are not written by hand: use `{{slug}}`.
- **Where a value is set**: when a ticket changes a field's value, name the function that sets it
  (e.g. where a message id is assigned), not only the files that read it.
- **Signature fallout**: when a hint says "do it as `other.ts` does", check that the pattern fits
  here. If the shared function must change (e.g. become generic), say so.
- **Wire shapes**: when the ticket reads a JSON payload, inline the field path (e.g.
  `message.usage.input_tokens`).
- **Helper prerequisites**: for each helper or fixture a ticket cites, name its prerequisites (e.g.
  `StartWithStore` needs a seeded project via `servertest.WriteTicket`) and its return value (e.g.
  `CreateEpic` returns the epic dir).
- **Names a seam or hint relies on**: for a fake, helper or generated type that a seam or hint
  names, run the `rg` the implementing agent would run and confirm it hits. A seam that finds
  nothing is not a seam.
- **Wiring**: name the hidden registration points (route tables, allowlists, parity tests, the
  query-and-prop path that feeds a page) and give `file:func` for anything the ticket says only by
  line number or role. For a new server route, point at `.ai/index.md` "Adding a server route".
- **Test patterns**: keep a `-run` pattern as narrow as the seams allow. A broad pattern can hit the
  test-run time cap.
- **Behaviour that must not change**: list one seam per state or variant of that behaviour, not only
  the ones the ticket edits.
- **Line numbers**: when a ticket points at code, name a symbol, not a line. Lines move; symbols
  stay.

A lint proves that a name exists, not that a setup works. A seam that names a real fake can still be
wrong, so the judgment rules above stay your job.

Before you publish, run `gx tickets lint-claims <project:epic> --manifest <file>` once. It checks
the `gx` commands and flags, file paths, `-run` patterns and flow ids that the tickets quote. Fix
each miss, or check that it is a false alarm. `gx tickets publish` runs the same lint again and
refuses on a miss in an `implement` or `research` ticket. Misses in a `verify` ticket are only
warnings. A name that an earlier ticket in the manifest creates is shown as planned and is fine.

How to tell a real miss from a false alarm:

- A miss in a path or command is real until you find a reason. A common reason is a file that an
  earlier unpublished ticket creates.
- Then rewrite the claim so the lint sees it as planned: say in that ticket's "What to build" that
  it adds the file, and block on that ticket. Do not move the claim into a `verify` entry.
- If a rewrite does not fit, pass `--allow-claim-misses` to `publish`. This flag covers claim misses
  only.

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

After the implement tickets, show the proposed verify parts (see "Verify tickets" in step 6): per
part its stories, fixture scenario and expected token budget, plus the collect ticket's budget, or
the single lone ticket when everything fits in one part.

Before the list, show the draft epic Summary (written as step 6 describes) so the user can correct
it with the rest of the breakdown.

Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the blocking edges correct — does each ticket only depend on tickets that genuinely gate it,
  and are same-file siblings and cross-cutting tickets chained rather than run in parallel?
- Are the declared seams the right ones to test against?
- Should any tickets be merged or split further?

Ask them as one round in the format [gx-grilling](../gx-grilling/SKILL.md) defines: a `❓ **Qn**`
line, a `➡️` recommendation and a `gx-question` block per question. Only a line that starts with
`❓ **Q` makes the web UI show the round and mark the session as waiting for you; a plain numbered
list goes unseen. Keep the Summary and the breakdown above the round, and make the round the last
thing in the turn: no tool call, and no text, after it.

Iterate until the user approves the breakdown. Skip this step when running unattended (e.g. inside
ralph-loop) — publish the breakdown as drafted and let the tickets themselves carry the record of
what was decided.

### 6. Publish the tickets

Write the whole breakdown as one manifest file and publish it with one call. `publish` creates the
implementation epic as a `draft` when it does not exist, so the scheduler cannot queue it before its
tickets exist. It also assigns the IDs, in dependency order:

```sh
gx tickets publish <project:epic> --manifest <file>
```

`publish` enforces the authoring rules from steps 3, 4 and 4b: an `implement` ticket needs a
non-empty `## Test seams`, a `## Where to look`, and a window at or under the cap. It also prints a
"consider splitting" line for a ticket with 5 or more acceptance criteria and 5 or more files; that
line never refuses a publish.

Add `--dry-run` first to print the IDs it would assign without writing anything. Opening the epic is
the very last step, after the Summary, `ensure-code-review` and `ensure-verify` (see the end of this
step).

The manifest is a YAML list with one entry per ticket. Each entry has these fields:

- `slug`: the ticket's slug.
- `type`: the ticket type, for example `implement`.
- `blocked_by`: the tickets it depends on. List slugs from this manifest, or a bare ID for a ticket
  that already exists. Never put a `/` in it. Leave it out when nothing gates the ticket.
- `expected_context_window`: the estimate from step 4.
- `body`: the ticket body, from the per-ticket template below.

To refer to another ticket from a body, write `{{slug}}`. `publish` replaces it with that ticket's
ID. To write a literal placeholder, quote it as `\{{`. Write the manifest to a scratchpad file
with Write; never a heredoc, never edit a ticket file by path (see "Write a body once" in the
tracker).

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

`publish` validates every ticket and prints the failures. For a ticket added with `gx tickets add`,
run `gx tickets validate <addr>` on it before considering it published. Fix any reported error and
re-validate until it passes. Do not publish a ticket that fails validation.

Tickets can also be forked off **mid-flight**, by `gx-implement`, when a ticket outgrows its budget
while in progress — same template, same publishing mechanics, just triggered from inside a running
session instead of upfront here. See [gx-local-tracker.md](../gx-local-tracker.md)'s mid-flight
forking section for the numbering, blocking-edge, and `parent` mechanics.

If the breakdown appends a trailing `type: code-review` ticket, run
`gx tickets ensure-code-review <epic-slug>` instead of publishing it by hand — it's a no-op if the
epic already has a code-review ticket among its published issues, else it stamps out the stub. Skip
running it only if the user has explicitly asked for a second code-review ticket.

If the spec has a `## Verification` section that lists new flows or scenarios to write, turn each
one into an implementation ticket in the breakdown, like any other piece of work. The verify ticket
runs those flows; it only writes one itself when it is small (see gx-verify-epic).

#### Verify tickets

After the implement and code-review tickets, split the spec's user stories into budgeted verify
tickets. Skip this and `ensure-verify` when the epic has no user-visible behavior, meaning the spec
changes no web, CLI or server behavior (a docs-only, skills-only or refactor epic). The spec for epic
`<name>-impl` is `docs/specs/<name>.md`; this is the resolved spec path.

**Plan the verify tickets.** Write a plan file (YAML). Per story with user-visible behaviour it lists
the story number, its scenario group, its items by kind (web flow, CLI check, Go test run, skill
quote) and its mock states by name, plus the fixture scenario. gx does the estimating and packing:

```sh
gx tickets verify-plan <project:epic> --plan <file> --manifest <manifest>
```

It reads the spec at `docs/specs/<name>.md`, copies each story's Verification entry and Mock states
line word for word, packs the parts, and appends the verify entries and their body files to the
publish manifest. Publish the manifest once, together with the implement tickets. `gx tickets
verify-plan --table` prints the estimates. Do not copy them into the skill or the tickets.

Implement tickets keep their own rule: add 10000 to `expected_context_window` per mock state in the
ticket's `## Mock check`, and allow at most 4 mock states per implement ticket. Both the 10000 and the
cap count mock states: one state section of the mock file, one name at one width. Over the cap,
split the ticket. The first ticket builds the parts and checks the first 4 states. A follow-on
implement ticket, blocked by the first, checks the remaining states and fixes what differs.

gx gives one lone `verify` entry when every story fits one part. Otherwise it gives top-level
`verify-part-N` entries and one `verify-collect` entry blocked by every part. The body formats
follow [gx-verify-epic](../gx-verify-epic/SKILL.md) ("The verify part body"). Every entry sets
`expected_context_window`. Do not restate the formats here.

After `publish`, run `gx tickets ensure-verify <epic-slug>`. It is a no-op when the epic already has
a verify ticket.

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

If the implementing agent drops or changes a seam named here, it must log that under
`## Spec deviations`, with the reason.

## Where to look

- Entry points: `file:func` ...
- Test helpers: ...
- Shared contract (callers, tests, fixtures): ...

(Or: `none — <rationale>`.)

## Mock check

- Mock file: `<full path>`
- States: `<state name>` at `<width>`, ...
- Parts this ticket builds: ...
- Fixture scenario: `<gxfixture scenario>`

(Only on an implement ticket that builds part of a stored mock. A UI ticket that builds no mock
part has no `## Mock check`.)

## Acceptance criteria

- [ ] Acceptance criterion 1
- [ ] Acceptance criterion 2
```

</ticket-template>

**Mock check rules** (for the `## Mock check` section above). Name the gxfixture scenario for every
mock state in `## Mock check`. If no scenario exists for a state, add an implement ticket that
creates it, and make it block every ticket that checks that state.

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
