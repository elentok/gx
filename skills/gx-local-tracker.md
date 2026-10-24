# gx local ticket tracker

The issue tracker for this repo is local markdown files in gx's ticket store (a git-backed directory,
`<store>/<project>/`; a repo not yet moved by `gx tickets migrate` still uses `.scratch/`), read and
written through `gx`'s own `tickets` package — not a hosted issue tracker, and not any personal skill-setup or
triage-label mapping. This document is the complete contract: layout, statuses, blocking, frontier,
claiming, and resolution. Every gx skill that reads or writes tickets treats this document as
authoritative.

## Layout

- **Always resolve the root with `gx tickets root` before locating any ticket, epic, or spec file —
  never assume a ticket directory at cwd or repo root, even when you're already inside a worktree or
  a subdirectory.** The root is the project's directory in the ticket store (or the canonical
  `.scratch` for an unmigrated repo), never any worktree's own path. `gx tickets root` prints it with
  no decoration, so `root=$(gx tickets root)` is safe to run from anywhere inside the repo. Every
  `<root>` below refers to this resolved path.
- One feature/epic per directory: `<root>/<epic-slug>/`
- **List the epics with `gx tickets epics`** — prints one bare epic slug per line, alphabetically,
  with `.archive` and other dot-prefixed directories already excluded, so it never needs filtering.
  Use it instead of listing `<root>` yourself. The slugs compose with the resolved root:
  `<root>/$(gx tickets epics | head -1)/issues/`.
- A spec or plan, if one exists, is `docs/specs/<epic-slug>.md` at the repo root — not under
  `<root>`, so it can be committed alongside the code it describes.
- A `gx-wayfinder` map lives in its epic's entry file, `<root>/<epic-slug>/ticket.md`: the frontmatter
  says `kind: map` and the body is the map (Destination/Notes/Decisions-so-far/Not-yet-specified/
  Out-of-scope — see the `gx-wayfinder` skill). `kind: map` makes it a **map epic**: hand-driven, never
  scheduled by the server, and its decision tickets are claimed and closed by hand (see "Map epics"
  below). An epic is a map epic or a loop epic, never both. List map epics with
  `gx tickets epics --maps`. An unmigrated `.scratch` tree still uses `<root>/<epic-slug>/map.md`;
  `gx tickets migrate --to-store` folds it into `ticket.md` with `kind: map`.
- Tickets are one file per ticket at `<root>/<epic-slug>/issues/<NN>-<slug>.md`, numbered from
  `01` — never a single combined tickets file
- A ticket **identifier** is the filename's numeric prefix, optionally followed by one lowercase
  letter: `04`, `04a`, `04b`. The letter marks a ticket created by a mid-flight fork (see below); a
  bare number is an originally-authored ticket.
- A ticket's canonical **address** is `project:epic/NN` (e.g. `gx:orchestrator-daemon/06`); `epic/NN`
  and `NN` are accepted as input when the project or epic is implied. Verbs take addresses, not file
  paths. Any registered project's prefix is accepted on `add`, `show`, `section`, `set`, `close`
  and `move` (an unknown project refuses with nothing written), so an agent can write into another
  project's tracker — e.g. `gx:follow-ups` from a different repo. `move` still refuses a target in a
  different project from its source.

## Frontmatter

Every ticket file opens with a `---`-delimited YAML frontmatter block:

```yaml
---
id: "04"
status: open
blocked_by: ["01", "02"]
type: implement
expected_context_window: 20000
---
```

Fields:

- **`id`** (string, required) — the ticket identifier, matching the filename prefix, e.g. `"04"` or
  `"04b"`. Fixed at creation.
- **`status`** (enum, required) — exactly one of `draft`, `open`, `claimed`, `paused`,
  `done`, `cancelled`. There is no default: a ticket with no `status:` is rejected by
  the loader rather than read as `open`, so a half-written file can never be handed to an agent. Only
  `open` is schedulable — `draft` is work its author set aside deliberately, neither offered to an agent
  nor counted as finished. `cancelled` is terminal like `done` but means the ticket was withdrawn,
  not performed; it stops counting as outstanding. **Only `draft` and `open` can be written with
  `gx tickets set --status`** (plus `claimed` and `done` in a map epic). Every other status is an
  orchestration status, written by the gx server alone, except `cancelled` on a follow-up, which
  `gx tickets close` also writes (see "Who writes which status"). The UI also shows `blocked` and
  `waiting-for-children`, but those are derived from the graph on every render and are never
  written to a file.
- **`blocked_by`** (list of ticket IDs) — tickets that must be finished before this one can start;
  omit or `[]` when there are none. Each token names exactly one ticket in the epic: a bare number
  (`"04"`) names the ticket whose identifier has no letter suffix, a lettered token (`"04a"`) names
  that one fork sibling. A token resolves once the ticket it names has stopped blocking — that
  ticket's own status is `done` **and** every ticket in its fork subtree (its `parent` descendants,
  recursively) is done too. A token naming no ticket in the epic never resolves, since nothing can
  verify it. To wait on work in another epic of the same project, qualify the token:
  `blocked_by: [epic/06]` (`project:epic/06` is accepted for the ticket's own project; another
  project's prefix fails `gx tickets validate`, cross-project blocking being unsupported). Always
  block on the other epic's ticket this way — never pause the ticket as a stand-in. Cycles, including
  cross-epic ones, fail `validate`, which reports every edge of the chain. A `type: code-review`
  ticket carries no `blocked_by` at all; see its own frontier rule below.
- **`parent`** (ticket ID) — the ticket this one was produced from: a mid-flight fork names the
  ticket it forked off, and a fix ticket opened by a `type: code-review` ticket names that review
  ticket. This is the only fork edge — it lives on the descendant, and nothing is recorded on the
  original. Omit entirely on a normally-authored ticket.
- **`type`** (enum) — one of `implement` (the retired `task` fails validation; `gx tickets migrate` rewrites it), `research`, `prototype`, `grilling`, `code-review`, `conflict-resolution`. See
  `code-review` below.
- **`category`** (enum) — `bug` or `idea`. Optional on any ticket; required on a follow-up (see
  "Follow-ups"). Not named `kind`, which already means `kind: map` on an epic.
- **`expected_context_window`** (non-negative int) — the estimated tokens the implementation will
  occupy.
- **`commitless`** (bool) — set `true` when a ticket intentionally finishes an iteration with no
  commit (e.g. it turned out to need only a fork, or the behavior already existed). Pass it in the
  same `set` call as `--iteration-status finished` — otherwise the ticket still reads as
  claimed-but-stalled.
- **`base`** (string) — optional override of the branch this ticket's work starts from: a branch
  name or a ticket reference. Normally omit it: gx derives the base at claim time (a commitful ticket
  blocked by one unmerged commitful ticket bases on that blocker's branch; otherwise leaf → parent's
  branch, root → trunk). With two or more unmerged commitful blockers and no `base`, gx pauses the
  ticket (reason `ambiguous-base`) naming them. Not allowed on a `commitless` ticket (`validate` rejects it).
  The target is always derived, never stored.
- **`pause_reason`** (enum) — why the ticket is paused (e.g. `blocked-pane`, `self-reported`,
  `zero-commit`). Machine-written alongside `status: paused`, cleared on claim; never set it by
  hand. The full closed enum is printed by `gx server events kinds`.
- **`actual_context_window`**, **`elapsed_time`**, **`resolved_base`**, **`mutes`** — read-only,
  gx-managed. The first two are stamped at merge time, `resolved_base` (ref + SHA) at every claim,
  `mutes` when a notification throttle trips; never set these by hand.

### The `code-review` type

A ticket with `type: code-review` reviews the whole epic once every other ticket in it has merged,
rather than one specific piece of work — so it carries no `blocked_by` list of its own. Its
frontier-eligibility rule is different from every other type: it becomes eligible once every *other*
ticket in the epic is `done`, independent of any `blocked_by`. Fix tickets it opens (if any) name it
as their `parent`.

The body of the file (everything after the closing `---`) is free-form markdown: title, "What to
build", acceptance criteria, test seams, and (appended over time) a `## Comments` section for
conversation history.

## Reading, writing, and validating

- **Validate a ticket**: `gx tickets validate <addr>` — parses the frontmatter and reports whether
  it's well-formed. Run this on every ticket before considering it published, and again on any ticket
  about to be claimed; fix and re-validate until it passes.
- **Read the full field/enum reference**: `gx tickets schema` — prints the settable and read-only
  frontmatter fields verbatim, useful when composing a `set` call from memory.
- **Update fields**: `gx tickets set <addr> --status <value> [--blocked-by ...] [...]` — a sparse,
  validated write. Only the flags passed are changed; every other field is left exactly as it was.
  Never hand-edit frontmatter YAML directly when a `set` flag exists for the field.
- **Create a ticket**: `gx tickets add <epic> --slug <slug> --body -` (body on stdin) writes the
  ticket `open` and prints its address. Set fields with flags, in the same write: `--blocked-by
  01,03`, `--type`, `--expected-context-window`, `--commitless`, `--category` and `--parent` (parsed
  as `gx tickets set` parses them). Omit `--blocked-by` when nothing gates the ticket: no flag means
  no blockers. Write the body without a frontmatter block. (A leading block is still accepted for
  older callers, but a field set both there and by a flag refuses the add.) Then validate it.
- **Address a ticket**: agents name a ticket by its address, `project:epic/NN` (`epic/NN` or `NN`
  work as input when the epic is implied). Read it with `gx tickets show <addr>`; given an epic
  address (`project:epic`, or a bare `epic` in the cwd project) `show` prints the epic's `ticket.md`
  (frontmatter and body) followed by its tickets, one `id  status  title` line each (no `--json`).
  An all-numeric bare argument resolves as a ticket ID, so an epic named like `2024` needs its
  `project:epic` form. Change a body section with `gx tickets section <addr> <heading> <content|->`,
  change fields with `gx tickets set <addr>`. Never read or write a ticket file by path —
  `gx tickets root` / `gx tickets epics` are for humans and scripts.
- **Allocate an ID atomically**: `gx tickets add <epic> [--parent <id>] --slug <descriptive-slug>` —
  picks the next free identifier under a filesystem lock (safe against a concurrent fork) and writes
  the stub straight to `<id>-<slug>.md`. The epic argument may be `project:epic`. Filing into a
  `follow-ups` epic with a plain `add` and no `category` refuses; use `follow-up add` instead.
- **File a follow-up**: `gx tickets follow-up add <project:epic> --category bug|idea --body -`
  (or `category:` in the body frontmatter, never both). Skills never call it
  directly: they file through the [gx-follow-up](gx-follow-up/SKILL.md) skill, which calls this
  verb. See "Follow-ups".
- **Move a ticket**: `gx tickets move <addr> <epic> [--slug s] [--dry-run] [--json]` — moves one
  flat, never-run `draft`/`open` ticket to another epic of the same project and rewrites every
  `blocked_by`/`base` reference to it. It refuses a ticket with run history, a `parent` or fork
  children, one queued or live on the server, and a map-epic source. A map-epic target is allowed.
  Prose mentions of the old address are not rewritten; the files that hold one are listed as
  warnings. Use `--dry-run` first.
- **Close a follow-up**: `gx tickets close <addr> --reason fixed|obsolete|duplicate|not-worth-it
  [--note ...]` — see "Follow-ups".
- **Create or edit an epic**: `gx tickets epic add <project:epic> --kind map|loop --status
  draft|open [--body -]` creates the epic's `ticket.md` and refuses an existing epic. A frontmatter
  block at the top of the body may carry the settable epic fields (e.g.
  `destination_unconfirmed: true`); any other field refuses the add. `gx tickets section` and
  `gx tickets set` accept an epic address (`project:epic`, no ticket id) and validate it against the
  epic schema: `set` writes `kind`, `status` and `destination_unconfirmed`; `completed_at`,
  `evolved_at`, `evolve_skipped` and `evolve_ticket` are server-stamped and read-only. Never
  hand-edit an epic's `ticket.md`.

### From `draft` to `open`

`gx tickets add` writes its stub `status: draft` on purpose: a freshly allocated ticket has an empty
body, and `draft` never enters the frontier, so the window between allocating the ID and writing the
real content can't hand an empty ticket to an agent. Promoting it is a deliberate, separate step —
fill in the body first, then `gx tickets set <addr> --status open`. That `set` call is the handoff:
before it, the ticket belongs to its author; after it, the scheduler may claim it at any moment, so
nothing about it should still be in flux. A ticket created with `gx tickets add --body` is
written `open` in one write, since its body is written in the same write as its status.

## Frontier

The **frontier** is the ticket to work next: the lowest-numbered ticket, across the epic's
`issues/*.md` files, whose status is `open` and every one of whose `blocked_by` tokens has resolved
(see the field reference above — a token resolves only once the ticket it names *and* that ticket's
whole fork subtree are done). `open` is the only schedulable status: `draft`, `claimed`,
`needs-answer`, `needs-repair`, `done`, and `cancelled` are all skipped. Scan in filename numeric order; the
first match wins.

A `type: code-review` ticket is exempt from the `blocked_by` check: it becomes eligible once every
*other* ticket in the epic is `done`, regardless of its own (empty) `blocked_by` list.

## Who writes which status

The gx server is the only scheduler. Orchestration statuses are written by it alone, and
`gx tickets set --status` refuses them (`claimed`, `done`, `needs-answer`, `needs-repair`,
`cancelled`). The one other writer is `gx tickets close`, which writes `cancelled` — only on a
`draft`/`open` ticket in a `follow-ups` epic:

| Status | Written by |
| --- | --- |
| `draft`, `open` | authors and agents, via `gx tickets set --status` |
| `claimed` | the server, at claim time, before the agent's prompt is sent |
| `done` | the server, after its own commit count and cherry-pick check |
| `needs-answer` | the server, when an agent runs `gx tickets ask` |
| `needs-repair` | the server, when it hits a fault it can't resolve |
| `cancelled` | the server, via `gx server tickets cancel` (also cancels non-terminal descendants); `gx tickets close`, for a follow-up only |

Exception: in a **map epic** the server schedules nothing, so `claimed` and `done` are written by
hand there — see "Map epics".

Orchestration changes a person may need go through server verbs, each with a `--json` result or a
`{"refused":true,"reason":…}` refusal: `gx server tickets pause | cancel | relaunch`,
`gx server queue add | remove | replace | move`, and read verbs such as
`gx server tickets explain <addr>` ("why isn't this running") and `history <addr>`. The repair verbs
are `gx server tickets merge | start-over | resume | verify`. If the server is down, only these four run
directly; every other server verb refuses
`server-not-running`.

## Map epics

A map epic (`kind: map` in its `ticket.md`) holds gx-wayfinder decision tickets, which a person resolves
in a live session rather than an agent iteration. The server never schedules one: its frontier is
always empty, and `gx server queue add` refuses its tickets with `map-epic`. So the person driving it
writes the statuses directly:

- **Claim** before any work: `gx tickets set <addr> --status claimed`.
- **Close** once resolved: write the answer with `gx tickets section <addr> Resolution -`, then
  `gx tickets set <addr> --status done --commitless true` (a decision makes no commits).

`paused` and `cancelled` stay server-only, even here.

A map epic seeded by gx-resolve-follow-ups carries **`destination_unconfirmed: true`** in its
`ticket.md`: the Destination is a guess from the follow-ups, not yet confirmed. gx-design confirms
it with the person, then clears the flag with `gx tickets set <project:epic> --destination-unconfirmed
false`. Treat a map with the flag set as not ready to chart further. (The old name,
`provisional_destination` and `--provisional-destination`, is a deprecated alias for one release;
`gx tickets migrate` renames the field.)

### Wayfinding operations

How the `gx-wayfinder` skill's map and tickets map onto this tracker. This is the whole set of
operations a map needs — chart, claim, close and frontier; a gap belongs here, not in the skill
that drives it.

- **Chart**: `gx tickets epic add <project:epic> --kind map --status open --body -`, body = the map
  (a frontmatter block at the top may set `destination_unconfirmed: true`). Then run
  `gx tickets validate` on any ticket of the epic. Later edits to the map body (Decisions so far,
  Not yet specified, Out of scope) go through `gx tickets section <project:epic> <heading> -`.
- **Child ticket**: `gx tickets add <epic> --slug <slug> --type <type> [--blocked-by <ids>] --body
  -`, with the question under `## Question`.
- **Blocking**: `blocked_by`, as for any ticket.
- **Frontier**: the rule under "Frontier" above, applied by hand: the lowest-numbered `open` ticket
  whose `blocked_by` tokens have all resolved. The server never computes one for a map epic.
- **Claim**: `gx tickets set <addr> --status claimed`.
- **Close**: `gx tickets section <addr> Resolution -`, then
  `gx tickets set <addr> --status done --commitless true`, then append a gist + link to the map's
  Decisions so far with `gx tickets section <project:epic> "Decisions so far" -`.

## Follow-ups

A **follow-up** is a diagnosis or an idea filed for a person to plan, not schedulable work. This
section is the one place the format lives; gx-follow-up, gx-evolve and gx-resolve-follow-ups link
here rather than restating it. The filing procedure lives in the
[gx-follow-up](gx-follow-up/SKILL.md) skill.

- **A follow-ups epic** is an epic named `follow-ups`. Its archive is `follow-ups-archived`. There
  is no `type: follow-up`: `type` picks the skill, and "follow-up" is only where a ticket lives.
- **Routing**: a project-specific finding → `<project>:follow-ups`; a gx tooling finding →
  `gx:follow-ups`; a closed follow-up → `<project>:follow-ups-archived`. Ask: would this still exist
  if the epic you were working in had never been authored? If yes, it's tooling.
- **Always `draft` + `research`.** Filers never guess `implement`; gx-resolve-follow-ups sets the
  real type and status when it moves the ticket.
- **Body sections**, for bugs and ideas alike:
  - `## Source` — the epic, ticket addresses, session ids, and which skill or person filed it.
  - `## Problem` — bug: what is wrong; idea: what is missing or painful.
  - `## Evidence` — log lines, ticket ids, commits. Required for a bug, optional for an idea.
  - `## Suggested fix`
  - `## Open questions` — optional.
- **Filing**: follow the [gx-follow-up](gx-follow-up/SKILL.md) skill; it is the procedure. It files
  only through `gx tickets follow-up add <project:epic> --body -`, never a plain `add`. The body's
  frontmatter carries `category: bug|idea`. The verb refuses, with nothing written and a code
  naming the gap, when `category` or a required section is missing, when the body asks for a
  status or type other than `draft`/`research`, or when the target is not a follow-ups epic.
- **Seen again**: never file a duplicate; see the dedupe step of
  [gx-follow-up](gx-follow-up/SKILL.md).
- **Server recovery follow-ups** (filed from investigate reports) use the same format:
  `category: bug`, Source / Problem / Evidence / Suggested fix, and the same "seen again" entry for a
  repeat. gx-resolve-follow-ups and `gx evolve export` then see one shape.
- **Closing**: `gx tickets close <addr> --reason fixed|obsolete|duplicate|not-worth-it [--note ...]`
  works only on a `draft`/`open` ticket in a `follow-ups` epic. In one step it writes
  `status: cancelled`, a `## Resolution` with the reason, note and original address, and moves the
  ticket into `<project>:follow-ups-archived` (created `draft` if missing). For `duplicate`, the note
  is the kept ticket's address. `gx tickets close --sweep [project]` moves every `done` and
  `cancelled` follow-up into the archive, keeping its status; a cancelled one with no `## Resolution`
  gets one recording its original address. Old run-log lines, trailers and branch names keep the
  old address; the Resolution records it.

## Skill conventions

Rules every gx skill follows when it names a model or launches a sub-agent:

- **Models by alias only** — `sonnet`, `opus` — never a pinned model ID.
- **On Codex**, name a tier (worker / strongest) and give the Claude alias as the example, e.g.
  "worker (`sonnet`)". Both use the default model; the strongest step runs at the highest effort.
  There is no tier→ID configuration.

## Claiming

An agent never claims a ticket. gx writes `status: claimed` before the agent's prompt is sent, so a
crashed iteration leaves the ticket visibly claimed rather than silently open, and a restart doesn't
double-pick it. gx re-reads the ticket file at claim time and refuses if it changed since the
scheduler decided. If a launch fails, gx retries, then pauses the ticket (reason `iteration-error`) with the
launch error.

## Resolution

An iteration agent never writes a terminal status. When the work is finished it reports
`gx tickets set <addr> --iteration-status finished [--commitless true]`, and gx adopts that into
`status: done` only after checking its own commit count and cherry-pick outcome; the report can start
a merge, never conclude one. The CLI refuses `--status done` from a `ralph-loop/*` branch outright,
even paired with `--iteration-status finished`.

A ticket closed by a mid-flight fork rather than by merging its own work (see below) is reported
the same way: `--iteration-status finished --commitless true`.

Other outcomes gx writes:

- **`paused`, class `answer`** — work is stalled on information only a human can supply. The agent
  reports `gx tickets ask`, described under Announce-and-stop, and gx pauses the ticket. gx also
  pauses a ticket this way itself in a few cases (`blocked-pane`, `zero-commit`, two or more
  unmerged blockers with no `base`), recording `pause_reason`.
- **`paused`, class `repair`** — something needs human judgment before work can continue (e.g. a
  crashed iteration, a handle mismatch, repeated launch failures `retry-exhausted`, a
  pause/re-claim loop `spinning`, lost commits).

A ticket finished with zero commits must also pass `--commitless true` in the same `set` call, or it
reads as a stalled agent rather than an intentional no-op finish.

`status: done` means only "this ticket's own work is finished" — it says nothing about the tickets it
forked off. A `done` ticket whose fork subtree still has unfinished work renders as
`waiting-for-children`, keeps blocking anything that names it, and keeps its epic from counting as
complete. That state is derived from the graph, never written, so closing a ticket that forked is
always a plain `--status done`; the children carry the rest.

## Announce-and-stop

<!-- gx-system-prompt:begin -->
Never use the AskUserQuestion tool. To ask a person, run `gx tickets ask`. Do these steps in order. 1. Commit what is green. Work already done is not discarded while the question waits. 2. Run `gx tickets ask <addr> --body -` with the body on stdin. The body has two sections. `## Question`: a one-line plain-text summary first (no markdown, at most 120 characters), then optional detail. A person should be able to answer in one pass without reading the diff. When the answer is yes or no, or a pick from 2 to 6 choices, end the detail with a fenced ```gx-question block (YAML: `type: yes-no` with optional `yes:` and `no:` consequences, or `type: options` with `options:` entries of `id` and `label`; optional `recommended:`), so the person gets buttons instead of a text box. `## Handoff`: required and non-empty. Name the files you touched, the decisions you made and why, and what you do with each answer. 3. End your turn. The agent keeps its worktree, pane and slot while it waits. After `gx tickets ask`, end your turn. The person's reply reaches you as a new message, in this session or in a fresh one that starts from your handoff. If the reply leaves anything unclear, or you need more, run `gx tickets ask` again. Never end a turn with a question in plain text and no `ask` call. No one is told, and the ticket stalls.
<!-- gx-system-prompt:end -->

The block above is also the GX system prompt gx appends to every loop agent's Claude launch; a test
keeps the two copies equal. The question can also carry an optional `gx-question` block (below).

Interactive tools block the pane and write nothing gx can read. A question asked that way is
invisible by construction. `gx tickets ask` records the question in the ticket's log, where gx, the
web UI, the TUI and the CLI all read it.

`ask` validates the body, appends one question event to the ticket log, sets
`iteration_status: asking` and returns at once. It writes nothing and refuses with a reason
when the body is invalid, the handoff is empty, the ticket is unknown, or an earlier question is
still open. It works for every ticket type. Do not write `--iteration-status asking` yourself
and do not put the question in a body section. `gx tickets set --status open` and
`--iteration-status asking` refuse when no matching open question exists, and name the verb to
use.

When the server is down, `ask` refuses with `server-not-running`. Retry `ask` after a minute and do not end your turn.

This is **not a fork**. No child ticket is created; the same ticket resumes. Do not re-ask a question
the log already answers. One answer has to be enough.

### The question block

An optional fenced block in the `## Question` text, marked ` ```gx-question `, holds YAML. This is
the one definition of the format. No block means a free-text question.

```gx-question
type: options          # yes-no | options | text
recommended: b         # optional: an option id, or yes / no
options:               # options only: 2 to 6 entries
  - id: a
    label: Keep the old route
    consequence: I leave the handler as it is.   # optional
  - id: b
    label: Replace it
```

- `yes-no` has fixed labels "Yes" and "No". It takes optional one-line `yes:` and `no:` consequences.
- `options` takes `id`, `label` and an optional `consequence` per entry. Ids are unique.
- `recommended` names one value. The UI shows a "Recommended" badge and preselects nothing.
- Every type allows a free-text note from the person.
- `ask` fails with the reason on bad YAML, an unknown type, duplicate ids, or a `recommended` outside
  the options.

### Answers, the log and the pause rounds

- A person replies with `gx tickets answer <addr> [--choice x] --body -` (or in the web UI). The body
  is the note, or the text for a free-text question. gx appends an answer event and delivers it to your
  session. The ticket goes back to `claimed`.
- If your agent is gone (crash, kill, budget stop), the answer reopens the ticket and shows it as
  **answered**. That state is derived, never stored: `status` is `open` and the latest question has an
  answer. It clears when gx claims the ticket. The next claim tries to resume your last session, and
  falls back to a cold start from the latest question and its handoff.
- `gx tickets show <addr>` ends with a generated **Pause rounds** block: earlier rounds on one line
  each, the latest in full with its question, handoff and answer.
- `gx tickets log <addr> [--type ...]` prints the raw log events. `show` never prints raw events.
- A claim after a plain resume (no answer) cold-starts from the handoff.

### Plain resume

- `resume` on a live, waiting `self-reported` pause refuses and names `gx tickets answer`. It neither
  kills the agent nor sends a prompt.
- `resume` on a `self-reported` pause whose agent is gone is allowed. gx writes a closing event, sets
  the ticket `open`, and the next claim cold-starts from the handoff. No answer event is logged, so
  the ticket is not **answered**.

Scope is **voluntary** asks only: a question the agent itself decides it needs answered. An
involuntary interactive prompt gx catches from outside (herdr reporting the pane as `blocked`) is the
orchestrator gate's problem, not the agent's, and follows a different path.

## The `## Needs Repair` section

When gx itself — not an agent — hits a fault it can't resolve (a crashed iteration, an operator
intervention gx observed from outside, commits that vanished before merging), it pauses the ticket
(class `repair`) and appends a `## Needs Repair` section, in this shape:

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

The `## Needs Repair` section carries a required one-line reason, and a question carries its summary
line, so the Queue can say *why* a ticket is paused without anyone opening the file. That requirement
is **write-conditional**: it is enforced by the write path that sets the status, never by the loader.
Enforcing it at load time would reject a hand-authored ticket that never wrote the section, making the
loader fail on a file it merely observes rather than one it produced.

## Mid-flight forking

A ticket can be forked while work is in progress, when it turns out to be larger than its budget or
to mix a plumbing/infra concern with a feature-on-top concern. `parent` is the whole protocol:

1. New ticket(s) get a flat sibling number off the root ticket: `04` forks into `04b`, `04c`, ...
   (skip `a` if the original ticket's own number, unlettered, is still in use as its identifier).
   Allocate each ID with `gx tickets add <epic> --parent <original-id> --slug <descriptive-slug>`
   rather than picking the number by hand — it's atomic against concurrent forks running at the
   same time, `--slug` (required) writes the stub straight to its final `<id>-<slug>.md` filename,
   and `--parent` writes the child's `parent` frontmatter at creation.
2. Fill in the new ticket's body, moving any not-yet-finished acceptance criteria off the original
   onto it — don't leave a criterion sitting on a ticket that's about to close. Then promote it with
   `gx tickets set <addr> --status open`.
3. The original is finished by reporting `--iteration-status finished`, with `--commitless true` if
   it makes zero commits of its own. gx then writes it `done`.

Nothing else is written. In particular a fork child gets **no** `blocked_by` naming the original, and
the original records nothing about what it forked. `parent` on the child is the only structural edge
there is, and both the blocking predicate and the Queue tab's tree derive everything from it.

Nothing downstream needs editing either. A `blocked_by` token naming the original resolves only once
the original *and its whole fork subtree* are done (see Frontier above), so a ticket already blocked
on the pre-fork original stays correctly blocked without its `blocked_by` list ever being touched —
and a fork child, having no token of its own, is free to start immediately.

**Once step 3 hands off (original marked `done`), the original's agent must not
write to a child ticket's file again for any reason.** Ticket files are shared, unlocked plain
files, not per-worktree or git-tracked — a later raw write can arrive after the child's own
independent iteration has already been claimed and launched, clobbering its `status` back to
`open` and racing the scheduler into reclaiming/relaunching it under the same deterministic agent
name (`agent_name_taken`). Once handoff happens, the original's job is done; any further changes
belong on the child ticket, made by the child's own agent.
