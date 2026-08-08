# gx local ticket tracker

The issue tracker for this repo is local markdown files under `.scratch/`, read and written through
`gx`'s own `tickets` package — not a hosted issue tracker, and not any personal skill-setup or
triage-label mapping. This document is the complete contract: layout, statuses, blocking, frontier,
claiming, and resolution. Every gx skill that reads or writes tickets treats this document as
authoritative.

## Layout

- **Always resolve the root with `gx tickets root` before locating any ticket, epic, or spec file —
  never assume `.scratch/` at cwd or repo root, even when you're already inside a worktree or a
  subdirectory.** In a bare-repo checkout with linked worktrees, the canonical root lives at the bare
  repo's own path, not any worktree's, and cwd may be several directories below it. `gx tickets root`
  prints the canonical `.scratch` path with no decoration, so `root=$(gx tickets root)` is safe to run
  from anywhere inside the repo. Every `<root>` below refers to this resolved path, not a literal
  `.scratch/`.
- One feature/epic per directory: `<root>/<epic-slug>/`
- A spec or plan, if one exists, is `<root>/<epic-slug>/spec.md`
- Tickets are one file per ticket at `<root>/<epic-slug>/issues/<NN>-<slug>.md`, numbered from
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
children: []
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
  every ticket sharing that number — its own children, recursively — is done too (see Frontier
  below). A lettered token (`"04a"`) names one specific sibling and resolves the same way, via that
  sibling's own children. A `type: code-review` ticket carries no `blocked_by` at all; see its own
  frontier rule below.
- **`children`** (list of ticket IDs) — the ticket(s) this one produced: a mid-flight split, or (for
  a `type: code-review` ticket) the fix tickets it opened. `[]` if none.
- **`parent`** (ticket ID) — the ticket this one was produced from. Omit entirely on a
  normally-authored ticket.
- **`split`** / **`split_from`** — the legacy spellings of `children`/`parent`. Still accepted when
  reading a ticket already on disk (for backward compatibility with tickets written before this
  rename), but never written — every write emits `children`/`parent` only.
- **`type`** (enum) — one of `task`, `research`, `prototype`, `grilling`, `code-review`. See
  `code-review` below.
- **`expected_context_window`** (non-negative int) — the estimated tokens the implementation will
  occupy.
- **`commitless`** (bool) — set `true` when a ticket intentionally finishes an iteration with no
  commit (e.g. it turned out to need only a split, or the behavior already existed). Pair it with a
  terminal status (`done`, or an unclaimed status) — otherwise the ticket still reads
  as claimed-but-stalled.
- **`actual_context_window`**, **`elapsed_time`** — read-only, gx-managed. Stamped automatically at
  landing time; never set these by hand.

### The `code-review` type

A ticket with `type: code-review` reviews the whole epic once every other ticket in it has landed,
rather than one specific piece of work — so it carries no `blocked_by` list of its own. Its
frontier-eligibility rule is different from every other type: it becomes eligible once every *other*
ticket in the epic is `done`, independent of any `blocked_by`. Fix tickets it opens (if any) are
recorded as its `children`.

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
`issues/*.md` files, that is unblocked (every ticket named in its `blocked_by` is `done`, recursively
through that ticket's own `children`) and unclaimed (its status is one of the open statuses). Scan in
filename numeric order; the first match wins.

A `type: code-review` ticket is exempt from the `blocked_by` check: it becomes eligible once every
*other* ticket in the epic is `done`, regardless of its own (empty) `blocked_by` list.

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
   Allocate each ID with `gx tickets add <epic> --parent <original-id> --slug <descriptive-slug>`
   rather than picking the number by hand — it's atomic against concurrent splits landing at the
   same time, and `--slug` (required) writes the stub straight to its final `<id>-<slug>.md`
   filename.
2. Each new ticket's `blocked_by` includes the original ticket's id.
3. Each new ticket's `parent` frontmatter names the original, set at creation.
4. The original ticket's `children` field is set to list the new ticket(s):
   `gx tickets set <path> --children <new-ticket-ids>`.
5. The original is closed as `done`, with `commitless: true` if step 5 lands zero commits of the
   original's own.

Nothing downstream needs editing: a `blocked_by` token naming the original ticket resolves only once
the original *and every one of its children, recursively* is done (see Frontier above), so a ticket
already blocked on the pre-split original stays correctly blocked without its `blocked_by` list ever
being touched.

Any not-yet-finished acceptance criteria move off the original ticket onto the new one(s) — don't
leave a criterion sitting on a ticket that's now closed.

**Once step 4/5 hand off (`children` set, original marked `done`), the original's agent must not
write to a child ticket's file again for any reason.** Ticket files are shared, unlocked plain
files, not per-worktree or git-tracked — a later raw write can land after the child's own
independent iteration has already been claimed and launched, clobbering its `status` back to
`open` and racing the scheduler into reclaiming/relaunching it under the same deterministic agent
name (`agent_name_taken`). Once handoff happens, the original's job is done; any further changes
belong on the child ticket, made by the child's own agent.
