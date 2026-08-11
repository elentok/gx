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
- **List the epics with `gx tickets epics`** — prints one bare epic slug per line, alphabetically,
  with `.archive` and other dot-prefixed directories already excluded, so it never needs filtering.
  Use it instead of listing `<root>` yourself. The slugs compose with the resolved root:
  `<root>/$(gx tickets epics | head -1)/issues/`.
- A spec or plan, if one exists, is `<root>/<epic-slug>/spec.md`
- Tickets are one file per ticket at `<root>/<epic-slug>/issues/<NN>-<slug>.md`, numbered from
  `01` — never a single combined tickets file
- A ticket **identifier** is the filename's numeric prefix, optionally followed by one lowercase
  letter: `04`, `04a`, `04b`. The letter marks a ticket created by a mid-flight fork (see below); a
  bare number is an originally-authored ticket.

## Frontmatter

Every ticket file opens with a `---`-delimited YAML frontmatter block:

```yaml
---
id: "04"
status: open
blocked_by: ["01", "02"]
type: task
expected_context_window: 20000
---
```

Fields:

- **`id`** (string, required) — the ticket identifier, matching the filename prefix, e.g. `"04"` or
  `"04b"`. Fixed at creation.
- **`status`** (enum, required) — exactly one of `draft`, `open`, `claimed`, `needs-answer`,
  `needs-repair`, `done`. There is no default: a ticket with no `status:` is rejected by the
  loader rather than read as `open`, so a half-written file can never be handed to an agent. Only
  `open` is schedulable — `draft` is work its author parked deliberately, neither offered to an agent
  nor counted as finished. The UI also shows `blocked` and `waiting-for-children`, but those are
  derived from the graph on every render and are never written to a file.
- **`blocked_by`** (list of ticket IDs) — tickets that must be finished before this one can start;
  omit or `[]` when there are none. Each token names exactly one ticket in the epic: a bare number
  (`"04"`) names the ticket whose identifier has no letter suffix, a lettered token (`"04a"`) names
  that one fork sibling. A token resolves once the ticket it names has stopped blocking — that
  ticket's own status is `done` **and** every ticket in its fork subtree (its `parent` descendants,
  recursively) is done too. A token naming no ticket in the epic never resolves, since nothing can
  verify it. A `type: code-review` ticket carries no `blocked_by` at all; see its own frontier rule
  below.
- **`parent`** (ticket ID) — the ticket this one was produced from: a mid-flight fork names the
  ticket it forked off, and a fix ticket opened by a `type: code-review` ticket names that review
  ticket. This is the only fork edge — it lives on the descendant, and nothing is recorded on the
  original. Omit entirely on a normally-authored ticket.
- **`type`** (enum) — one of `task`, `research`, `prototype`, `grilling`, `code-review`. See
  `code-review` below.
- **`expected_context_window`** (non-negative int) — the estimated tokens the implementation will
  occupy.
- **`commitless`** (bool) — set `true` when a ticket intentionally finishes an iteration with no
  commit (e.g. it turned out to need only a fork, or the behavior already existed). Pair it with a
  terminal status (`done`, or an unclaimed status) — otherwise the ticket still reads
  as claimed-but-stalled.
- **`actual_context_window`**, **`elapsed_time`** — read-only, gx-managed. Stamped automatically at
  landing time; never set these by hand.

### The `code-review` type

A ticket with `type: code-review` reviews the whole epic once every other ticket in it has landed,
rather than one specific piece of work — so it carries no `blocked_by` list of its own. Its
frontier-eligibility rule is different from every other type: it becomes eligible once every *other*
ticket in the epic is `done`, independent of any `blocked_by`. Fix tickets it opens (if any) name it
as their `parent`.

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
- **Allocate an ID atomically**: `gx tickets add <epic> [--parent <id>] --slug <descriptive-slug>` —
  picks the next free identifier under a filesystem lock (safe against a concurrent fork) and writes
  the stub straight to `<id>-<slug>.md`.

### From `draft` to `open`

`gx tickets add` writes its stub `status: draft` on purpose: a freshly allocated ticket has an empty
body, and `draft` never enters the frontier, so the window between allocating the ID and writing the
real content can't hand an empty ticket to an agent. Promoting it is a deliberate, separate step —
fill in the body first, then `gx tickets set <path> --status open`. That `set` call is the handoff:
before it, the ticket belongs to its author; after it, the scheduler may claim it at any moment, so
nothing about it should still be in flux. A ticket written directly as a file (rather than via
`gx tickets add`) can be authored `open` in one write, since its body lands in the same write as its
status.

## Frontier

The **frontier** is the ticket to work next: the lowest-numbered ticket, across the epic's
`issues/*.md` files, whose status is `open` and every one of whose `blocked_by` tokens has resolved
(see the field reference above — a token resolves only once the ticket it names *and* that ticket's
whole fork subtree are done). `open` is the only schedulable status: `draft`, `claimed`,
`needs-answer`, `needs-repair`, and `done` are all skipped. Scan in filename numeric order; the
first match wins.

A `type: code-review` ticket is exempt from the `blocked_by` check: it becomes eligible once every
*other* ticket in the epic is `done`, regardless of its own (empty) `blocked_by` list.

## Claiming

Before starting work on a frontier ticket, claim it: `gx tickets set <path> --status claimed`. This
must happen before any implementation work, not after — an unattended run that crashes mid-ticket
should leave the ticket visibly claimed, not silently open, so a restart doesn't double-pick it.

## Resolution

When a ticket's work lands, set a terminal status: `gx tickets set <path> --status done`. A ticket
closed by a mid-flight fork rather than by landing its own work (see below) is also `done`, with
`commitless: true` since it never had commits of its own. Other terminal outcomes:

- **`needs-answer`** — work is stalled on information only a human can supply. Leave a note in the
  ticket body explaining what's missing.
- **`needs-repair`** — something needs human judgment before work can continue (e.g. a design
  question, a conflict with another ticket).

A ticket finished with zero commits must also set `commitless: true` in the same `set` call, or it
reads as a stalled agent rather than an intentional no-op finish.

`status: done` means only "this ticket's own work is finished" — it says nothing about the tickets it
forked off. A `done` ticket whose fork subtree still has unfinished work renders as
`waiting-for-children`, keeps blocking anything that names it, and keeps its epic from counting as
complete. That state is derived from the graph, never written, so closing a ticket that forked is
always a plain `--status done`; the children carry the rest.

## Announce-and-stop

An agent running under the loop **never calls an interactive prompt.** Interactive tools block the
pane and write nothing to disk, and on-disk ticket status is gx's only observation channel, so a
question asked that way is invisible by construction.

When an agent needs something only a person can supply, it follows this rule instead:

1. **Commit what is green.** Work already done is not discarded while the question waits.
2. Write a **`## Needs Answer`** section: the question, the options weighed, and what the agent would
   do with each answer, opening with a one-line summary. A person should be able to answer in one
   pass without reading the diff.
3. Write a **`## Handoff`** section: what is done, what is left, which files were touched, and which
   skills the next agent should invoke, referencing by path. This is what makes the discarded context
   recoverable.
4. Report `--iteration-status needs-answer`.
5. **Exit**, releasing the worktree, tab, and concurrency permit.

This is **not a fork**. No child ticket is created; the same ticket resumes later. The resuming agent
**retires both sections into `## Comments`** and must not re-ask the same question without new
information, answering once has to be enough.

Answering is one edit: a person appends under `## Needs Answer` and sets `status: open`. There is no
dedicated command for it.

Scope is **voluntary** asks only — a question the agent itself decides it needs answered. An
involuntary interactive prompt gx catches from outside (herdr reporting the pane as `blocked`) is the
orchestrator gate's problem, not the agent's, and follows a different path.

## The `## Needs Repair` section

When gx itself — not an agent — hits a fault it can't resolve (a crashed iteration, an operator
intervention gx observed from outside, commits that vanished before landing), it parks the ticket
`needs-repair` and appends a `## Needs Repair` section, in this shape:

1. A **summary line**, guaranteed one line by a helper that splits the fault's reason at its first
   newline. Every fault path passes its error through unmodified rather than each deciding for itself
   how to summarise it, so the guarantee holds regardless of which one wrote it.
2. **Optional detail** — the remainder of the reason, when there was more than one line.
3. A **best-effort state block**: iteration label, branch, and worktree, whichever gx could still
   determine. A field it can't determine is **omitted, never filled with a placeholder** — a state
   block that says "unknown" three times would look like information where there is none.

**No `## Handoff` section.** A handoff describes an agent's own work in progress; a fault write has no
live agent to author one. Writing a synthetic one would produce a section that reads as if an agent
wrote it, when nothing did.

Both park sections — this one and `## Needs Answer` above — carry a required one-line reason, so the
Queue can say *why* a ticket is parked without anyone opening the file. That requirement is
**write-conditional**: it is enforced by the write path that sets the status, never by the loader.
Enforcing it at load time would reject a hand-authored ticket that never wrote the section, making the
loader fail on a file it merely observes rather than one it produced.

## Mid-flight forking

A ticket can be forked while work is in progress, when it turns out to be larger than its budget or
to mix a plumbing/infra concern with a feature-on-top concern. `parent` is the whole protocol:

1. New ticket(s) get a flat sibling number off the root ticket: `04` forks into `04b`, `04c`, ...
   (skip `a` if the original ticket's own number, unlettered, is still in use as its identifier).
   Allocate each ID with `gx tickets add <epic> --parent <original-id> --slug <descriptive-slug>`
   rather than picking the number by hand — it's atomic against concurrent forks landing at the
   same time, `--slug` (required) writes the stub straight to its final `<id>-<slug>.md` filename,
   and `--parent` writes the child's `parent` frontmatter at creation.
2. Fill in the new ticket's body, moving any not-yet-finished acceptance criteria off the original
   onto it — don't leave a criterion sitting on a ticket that's about to close. Then promote it with
   `gx tickets set <path> --status open`.
3. The original is closed as `done`, with `commitless: true` if it lands zero commits of its own.

Nothing else is written. In particular a fork child gets **no** `blocked_by` naming the original, and
the original records nothing about what it forked. `parent` on the child is the only structural edge
there is, and both the blocking predicate and the Queue tab's tree derive everything from it.

Nothing downstream needs editing either. A `blocked_by` token naming the original resolves only once
the original *and its whole fork subtree* are done (see Frontier above), so a ticket already blocked
on the pre-fork original stays correctly blocked without its `blocked_by` list ever being touched —
and a fork child, having no token of its own, is free to start immediately.

**Once step 3 hands off (original marked `done`), the original's agent must not
write to a child ticket's file again for any reason.** Ticket files are shared, unlocked plain
files, not per-worktree or git-tracked — a later raw write can land after the child's own
independent iteration has already been claimed and launched, clobbering its `status` back to
`open` and racing the scheduler into reclaiming/relaunching it under the same deterministic agent
name (`agent_name_taken`). Once handoff happens, the original's job is done; any further changes
belong on the child ticket, made by the child's own agent.
