# gx local ticket tracker

The issue tracker for this repo is local markdown files under `.scratch/`, read and written through
`gx`'s own `tickets` package — not a hosted issue tracker, and not any personal skill-setup or
triage-label mapping. This document is the complete contract: layout, statuses, blocking, frontier,
claiming, and resolution. Every gx skill that reads or writes tickets treats this document as
authoritative.

## Layout

- One feature/epic per directory: `.scratch/<epic-slug>/`
- A spec or plan, if one exists, is `.scratch/<epic-slug>/spec.md`
- Tickets are one file per ticket at `.scratch/<epic-slug>/issues/<NN>-<slug>.md`, numbered from
  `01` — never a single combined tickets file
- A ticket **identifier** is the filename's numeric prefix, optionally followed by one lowercase
  letter: `04`, `04a`, `04b`. The letter marks a ticket created by a mid-flight split (see below); a
  bare number is an originally-authored ticket.

## Frontmatter

Every ticket file opens with a `---`-delimited YAML frontmatter block:

```yaml
---
id: "04"
status: ready-for-agent
blocked_by: ["01", "02"]
split: []
type: task
expected_context_window: 20000
---
```

Fields:

- **`id`** (string, required) — the ticket identifier, matching the filename prefix, e.g. `"04"` or
  `"04b"`. Fixed at creation.
- **`status`** (enum) — one of `open`, `needs-triage`, `ready-for-agent`, `ready-for-human`,
  `claimed`, `needs-info`, `needs-attention`, `done`. A missing `status:` is treated as
  `open`. `open`, `needs-triage`, `ready-for-agent`, and `ready-for-human` are all unclaimed — none
  of them distinguish who is meant to pick the ticket up.
- **`blocked_by`** (list of ticket IDs) — tickets that must be `done` before this
  one can start; omit or `[]` when there are none. A bare-number token (`"04"`) is resolved only once
  every ticket sharing that number (the original plus any lettered splits) is done. A lettered token
  (`"04a"`) names one specific sibling and resolves as soon as that ticket alone is done.
- **`split`** (list of ticket IDs) — the ticket(s) this one was split into; `[]` if it hasn't been
  split.
- **`split_from`** (ticket ID) — set only on a ticket created by a mid-flight split, naming the
  ticket it was split from. Omit entirely on a normally-authored ticket.
- **`type`** (enum) — one of `task`, `research`, `prototype`, `grilling`.
- **`expected_context_window`** (non-negative int) — the estimated tokens the implementation will
  occupy.
- **`code_review_fixes`** (string) — `none`, `inline`, or `ticket:<id>`; set once code review has
  run, otherwise omitted.
- **`commitless`** (bool) — set `true` when a ticket intentionally finishes an iteration with no
  commit (e.g. it turned out to need only a split, or the behavior already existed). Pair it with a
  terminal status (`done`, or an unclaimed status) — otherwise the ticket still reads
  as claimed-but-stalled.
- **`actual_context_window`**, **`elapsed_time`** — read-only, gx-managed. Stamped automatically at
  landing time; never set these by hand.

The body of the file (everything after the closing `---`) is free-form markdown: title, "What to
build", acceptance criteria, test seams, and (appended over time) a `## Comments` section for
conversation history.

## Reading, writing, and validating

- **Validate a ticket**: `gx tickets validate <path>` — parses the frontmatter and reports whether
  it's well-formed. Run this on every ticket before considering it published, and again on any ticket
  about to be claimed; fix and re-validate until it passes.
- **Read the full field/enum reference**: `gx tickets schema` — prints the settable and read-only
  frontmatter fields verbatim, useful when composing a `set` call from memory.
- **Update fields**: `gx tickets set <path> --status <value> [--blocked-by ...] [...]` — a sparse,
  validated write. Only the flags passed are changed; every other field is left exactly as it was.
  Never hand-edit frontmatter YAML directly when a `set` flag exists for the field.
- **Create a ticket**: write the file directly, following the frontmatter shape above and the
  per-ticket template your skill defines. Then validate it.

## Frontier

The **frontier** is the ticket to work next: the lowest-numbered ticket, across the epic's
`issues/*.md` files, that is unblocked (every ticket named in its `blocked_by` is `done`) and
unclaimed (its status is one of the open statuses). Scan in filename numeric order; the first match
wins.

## Claiming

Before starting work on a frontier ticket, claim it: `gx tickets set <path> --status claimed`. This
must happen before any implementation work, not after — an unattended run that crashes mid-ticket
should leave the ticket visibly claimed, not silently open, so a restart doesn't double-pick it.

## Resolution

When a ticket's work lands, set a terminal status: `gx tickets set <path> --status done`. A ticket
closed by a mid-flight split rather than by landing its own work (see below) is also `done`, with
`commitless: true` since it never had commits of its own. Other terminal outcomes:

- **`needs-info`** — work is stalled on information only a human can supply. Leave a note in the
  ticket body explaining what's missing.
- **`needs-attention`** — something needs human judgment before work can continue (e.g. a design
  question, a conflict with another ticket).

A ticket finished with zero commits must also set `commitless: true` in the same `set` call, or it
reads as a stalled agent rather than an intentional no-op finish.

## Mid-flight splitting

A ticket can be split while work is in progress, when it turns out to be larger than its budget or
to mix a plumbing/infra concern with a feature-on-top concern. The split reuses this document's
numbering and blocking conventions:

1. New ticket(s) get a flat sibling number off the root ticket: `04` splits into `04b`, `04c`, ...
   (skip `a` if the original ticket's own number, unlettered, is still in use as its identifier).
2. Each new ticket's `blocked_by` includes the original ticket's id.
3. Anything that was blocked by the original ticket also gets each new ticket added as a blocker:
   `gx tickets set <path> --blocked-by <ids>` against those already-published tickets, editing in
   place — never re-generated from the template.
4. Each new ticket's `split_from` frontmatter names the original, set at creation.
5. The original ticket's `split` field is set to list the new ticket(s):
   `gx tickets set <path> --split <new-ticket-ids>`.
6. The original is closed as `done`, with `commitless: true` if step 6 lands zero commits of the
   original's own.

Any not-yet-finished acceptance criteria move off the original ticket onto the new one(s) — don't
leave a criterion sitting on a ticket that's now closed.
