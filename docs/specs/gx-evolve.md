# gx-evolve: retros, follow-up resolution and gx-design

## Problem Statement

When a loop epic finishes, nobody looks back at how the run went. Parks, conflicts, relaunches,
context overruns and code-review fix tickets all point at weak spots in gx itself, in ralph-loop, in
the skills and prompts, or in the project's agent docs. Today that evidence is lost: Claude
transcripts expire after about 30 days, and a person has to remember to dig through logs by hand.

The findings that do get written down pile up. A project's `follow-ups` epic grows without end. Its
tickets have no fixed shape, so bugs and ideas are mixed, some items are already fixed, some are
duplicates, and some are open design questions that nobody can act on. There is no way to move a
ticket into a real epic, no way to close one with a reason, and `gx tickets add gx:follow-ups` from
another repo silently creates a stray directory in the current folder instead of filing the ticket.

Design work that should turn those questions into specs runs through `/myway`, a personal skill
outside gx. It keeps review findings in an uncommitted `.scratch` file and knows nothing about map
epics seeded by gx.

Separately, cost tracking reads only the main agent transcript. Subagent usage lives in separate
files, so `actual_cost` and the live run budget miss all subagent cost, including gx-code-review's
today.

## Solution

Three gx skills form one loop, plus the Go changes they need:

- **gx-evolve** runs a **retro** of one finished loop epic, unattended. The gx server submits it as a
  commitless `type: evolve` one-off when the epic ends. It builds a cheap per-ticket flag table,
  sends subagents to read only the flagged tickets' transcripts, joins the findings by symptom, and
  has an Opus consultant triage them. Approved findings become **follow-ups** in a fixed format:
  tooling findings in `gx:follow-ups`, project findings in that project's `follow-ups`. A repeat of
  an open follow-up adds a "seen again" entry instead of a new ticket. The retro's record is its own
  ticket body.
- **gx-resolve-follow-ups** is a manual run that drains a project's `follow-ups` epic until it holds
  no tickets. Subagents verify each item, the main session dedupes and plans, and the user approves
  one plan. Ready items go into one `follow-ups-<date>` epic. Items that need a design decision or
  research go into area **map epics**, seeded for gx-design. Everything else is closed with a
  reason into `follow-ups-archived`.
- **gx-design** is `/myway` ported into gx. It walks one epic through **map → review → implement**:
  wayfinder charts and resolves the map, to-spec writes the spec, a design review files findings on
  the map, and gx-to-tickets publishes the implementation epic.

The Go side adds `gx tickets move`, `gx tickets close`, `gx tickets follow-up add` and `gx tickets
epic add`, cross-project addresses for the ticket verbs, `section` and `set` on epics, `category`
and `status: draft` on `add`, the `evolve` setting, the server hook and its `completed_at` stamp,
the `gx evolve` verbs (including the flag-table verb and the contributor export), new epic
frontmatter (`evolved_at`, `evolve_skipped`, `evolve_ticket`, `destination_unconfirmed`), subagent
cost tracking, and the `CONTEXT.md` → `GLOSSARY.md` rename.

## User Stories

### Retro trigger and settings

1. As a gx maintainer, I want the server to start a retro by itself when a loop epic ends, so that
   the evidence is read before transcripts expire.
2. As a gx user, I want retros off by default until I choose a mode, so that gx never files tickets
   I did not ask for.
3. As a gx user, I want a global `evolve: off | maintainer | contributor` setting, so that I can
   say how tooling findings are handled.
4. As a project owner, I want `evolve: false` in a project file to opt that project out, so that a
   project I don't care to improve never gets retros.
5. As a gx user with auto-merge on, I want the retro to start after the land attempt, whatever its
   outcome, so that a land park is part of the evidence.
6. As a gx user with auto-merge off, I want the retro to start once every ticket in the epic is
   done, so that it does not wait for a merge I run by hand.
7. As a gx user, I want a second retro of the same epic refused while one is queued, running or
   parked, so that one epic never gets two retros at once.
8. As a gx user, I want map epics and one-offs never to get a retro, so that only real loop runs are
   judged and a retro never triggers a retro of itself.
9. As a gx user, I want the epic stamped `evolved_at` when its retro finishes, even with zero
   findings, so that I can tell which epics were retro'd.
10. As a gx user, I want a cancelled retro to stamp `evolved_at` as skipped, so that the epic does
    not come back in `gx evolve all`.
11. As a gx user, I want the epic to carry `evolve_ticket: <addr>` next to `evolved_at`, so that I
    can jump from an epic to its retro record.
12. As a gx user, I want `gx evolve run <epic>` to submit the same retro by hand, so that I can
    retro an epic the server missed.
13. As a gx user, I want `gx evolve run <epic> --force` to ignore `evolved_at`, so that I can
    re-run a retro; the newest run's `evolve_ticket` wins.
14. As a gx user, I want `gx evolve skip <epic>` to mark an epic retro'd without running it, so
    that I can dismiss an epic not worth a retro.
15. As a gx user, I want `gx evolve all` to list loop epics completed in the last 30 days with no
    `evolved_at` and ask before submitting, so that I can catch up after turning retros on.
16. As a gx user, I want `--since`, `--all-projects` and `--yes` on `gx evolve all`, so that I can
    widen the window, cover every project, or skip the prompt in a script.
17. As a gx user, I want retros to count against the normal caps and run budget, so that they never
    crowd out real work or overspend.
18. As a gx maintainer, I want a `maintainer` setting with no `gx` project to make the hook log a
    warning and `gx evolve` refuse with a stable code, so that tooling findings are never silently
    dropped.

### Retro analysis

19. As the gx-evolve agent, I want one Go verb that prints a compact per-ticket flag table from the
    epic slug alone, so that I never read transcripts in the main session.
20. As the gx-evolve agent, I want the flag table to cover metrics, event counts, flags and
    severity for every ticket, so that I can pick what to deep-read.
21. As the gx-evolve agent, I want the verb to write text-only transcripts of each ticket's sessions
    to a temp dir, with fallbacks for native agent logs and Codex rollouts, so that subagents read
    small digests, not raw JSONL.
22. As the gx-evolve agent, I want a transcript that can't be found reported as "missing", so that a
    gap is visible and not mistaken for a clean run.
23. As the gx-evolve agent, I want conflict-resolution sessions found through the run log, so that
    a conflict's own agent is read too.
24. As the gx-evolve agent, I want at most 8 deep-read subagents (a config value), picked by
    severity, so that a bad epic does not blow up the retro's cost.
25. As the gx-evolve agent, I want the code-review ticket and its fix tickets always read, so that
    planning gaps the review exposed reach the retro.
26. As the gx-evolve agent, I want flagged tickets past the cap still shown as table rows in the
    join, so that nothing flagged disappears.
27. As the gx-evolve agent, I want to group findings by symptom across tickets, so that one cause
    seen in five tickets becomes one finding.
28. As the gx-evolve agent, I want an Opus consultant to remove duplicates, drop findings below the
    bar, rate impact and approve or reject each finding, so that only findings worth fixing get
    filed.
29. As the gx-evolve agent, I want the consultant to check approved findings against every open
    ticket that carries a `category` (including follow-ups already moved out) and the follow-ups
    archive of each target, so that a known or rejected finding is not filed again, and a fixed one
    that comes back is filed as a regression.
30. As a gx maintainer, I want findings limited to tooling and process (gx, ralph-loop, skills,
    prompts, agent docs), so that project-code quality stays with the code-review ticket.
31. As a gx maintainer, I want every finding to cite evidence, suggest a fix and name its target and
    category, so that each follow-up can be acted on.
32. As a gx maintainer, I want zero follow-ups to be a valid retro result, so that a run that went
    fine is not padded with noise.

### Filing follow-ups

33. As a gx maintainer, I want tooling findings filed in `gx:follow-ups` whichever project the epic
    ran in, so that gx's own backlog is in one place.
34. As a project owner, I want project-specific findings (that repo's agent docs) filed in that
    project's `follow-ups`, so that they stay with the project.
35. As any filer of a follow-up, I want one fixed format (Source, Problem, Evidence, Suggested fix,
    Open questions), so that gx-resolve-follow-ups can read every item the same way.
36. As any filer, I want a `category: bug | idea` field, so that bugs and ideas can be told apart
    without a second epic.
37. As any filer, I want follow-ups always filed as `draft` + `research`, so that nothing is built
    before someone checks it still matters.
38. As the gx-evolve agent, I want a match with an open follow-up to append a "seen again in
    `<epic>`" entry to its Evidence, so that repeats raise its weight instead of adding duplicates.
39. As any filer, I want `gx tickets add project:epic`, and `show`, `set`, `section`, `close` and
    `move` on a `project:` address, to work across projects and refuse an unknown project, so that
    filing into `gx:follow-ups` from another repo never creates a stray directory.
40. As any filer, I want `gx tickets add` to accept `category` and `status: draft` in the body
    frontmatter, so that a follow-up is filed in one write.
41. As gx-investigate, gx-to-tickets and gx-evolve, I want to file every follow-up through `gx
    tickets follow-up add`, which refuses a body that breaks the format with a clear error, so that
    every skill that files a follow-up writes the same shape.

### Contributor mode

42. As a gx contributor without a gx checkout, I want my tooling findings filed into a local,
    ticket-only `gx` project (`vcs: none`), so that filing works the same in both modes.
43. As a contributor, I want gx itself to create that project, only in `contributor` mode, unqueued
    and never run, so that it is a plain outbox.
44. As a contributor, I want `gx evolve export` to bundle the unexported `gx:follow-ups` drafts into
    one markdown file (stdout or `-o <file>`), one ticket per section, so that I can send them
    upstream by hand.
45. As a contributor, I want each exported ticket stamped `exported_at`, and `--again` to re-export,
    so that I never send the same ticket twice by accident.
46. As a contributor, I want every `gx evolve` verb to print how many follow-ups are unexported and
    tell me to run `gx evolve export`, so that the outbox does not fill up unseen.
47. As a contributor who later clones gx, I want `gx project add` to tell me to `gx project set-path
    gx <clone>`, and `set-path` to drop `vcs: none` for a git repo, so that my outbox becomes the
    real project.

### Retro record

48. As a gx user, I want the retro's ticket body to open with a short `## Result` (counts, filed and
    "seen again" addresses, one line per approved finding), so that I see the outcome at a glance.
49. As a gx user, I want `## Result` never empty ("No follow-ups. 13 tickets, none flagged."), so
    that "went fine" is visible, not silent.
50. As a gx maintainer, I want the flag table, raw findings, consultant triage and final disposition
    kept below the Result, so that I can audit the retro after transcripts expire.

### Resolving follow-ups

51. As a gx user, I want `gx-resolve-follow-ups` to work on any project's `follow-ups` (current
    project by default, `--project <name>` for another), so that I can drain gx's backlog from any
    repo.
52. As a gx user, I want one subagent per item (about 8 at a time) to check it is still relevant
    against the code, logs and `git log` since filing, so that fixed or obsolete items are found
    without me reading them.
53. As a gx user, I want an item the verifier can't decide kept as relevant, so that nothing is
    closed on a guess.
54. As a gx user, I want the verifier to list the files a fix would touch and any blocking
    questions, each tagged `quick` or `design`, so that the plan can group and route items.
55. As a gx user, I want duplicates merged into the oldest item with a "seen again" entry and the
    newer ones closed as `duplicate`, so that one problem is one ticket.
56. As a gx user, I want one plan table (close with reason / merge into X / ready epic / map epic Y)
    shown before any write, so that I approve the whole run once.
57. As a gx user, I want items with 1–2 quick questions asked in that same approval step and the
    answers written into the ticket, so that they become ready without another session.
58. As a gx user, I want to change any row of the plan, so that I keep the final say.
59. As a gx user, I want ready items moved into one `follow-ups-<date>` epic, `open` but not queued,
    as `implement` tickets, so that I choose when to run it.
60. As a gx user, I want ready tickets whose files overlap chained with `blocked_by`, so that they do
    not fight over the same hot file in parallel.
61. As a gx user, I want items with a `design` question (or more than 2 questions) moved into an
    area map epic as `open` `grilling` tickets with a `## Question`, and research-only items moved
    there as `open` `research` tickets, so that design and research work has a home.
62. As a gx user, I want those map epics seeded with a provisional Destination, Notes with the source
    addresses, and `destination_unconfirmed: true`, so that gx-design can pick them up.
63. As a gx user, I want the run never to grill me, so that one approval step is all it asks.
64. As a gx user, I want `follow-ups` to hold no tickets after a run, so that "done" is clear.
65. As a gx user, I want closed items kept in `follow-ups-archived` with a `## Resolution` (reason,
    note, original address), so that rejected ideas are not filed again and a search still finds
    them.
66. As a gx user, I want a one-time `--done` sweep that moves already-`done` follow-ups into the
    archive, so that old finished items leave the backlog too.
67. As a skill, I want `gx tickets move <addr> <epic>` to move one flat, never-run, unqueued
    `draft`/`open` ticket inside a project and rewrite every `blocked_by`/`base` ref to it, so that
    a move never leaves a dangling ref.
68. As a skill, I want `move` to refuse with a stable code (wrong status, run history, subtree,
    queued, other project, missing target, broken project), so that I can report why.
69. As a skill, I want `move --dry-run` and `--json`, so that I can show the plan before writing.
70. As a skill, I want `gx tickets close <addr> --reason fixed|obsolete|duplicate|not-worth-it
    [--note …]` to cancel, record and archive in one step, so that closing never needs a hand edit.

### gx-design

71. As a gx user, I want `gx-design <epic>` to detect the next step from on-disk state, so that I
    just re-run it until the epic is handed off.
72. As a gx user, I want the phases called map, review and implement, so that the report line names
    where I am.
73. As a gx user, I want gx-design to stop with a clear message when wayfinder, to-spec, grilling or
    domain-modeling is missing, so that I know to install the mattpocock skills.
74. As a gx user, I want review findings kept as a `## Review findings` checklist on the map's
    `ticket.md`, so that they live in the ticket store, not an uncommitted scratch file.
75. As a gx user, I want gx-design's first run on a provisional map to grill the Destination alone,
    clear `destination_unconfirmed`, then continue into the map in the same session, so that a
    seeded map gets a real Destination before work starts.
76. As a gx user, I want gx-design to stop after reworking the map when the new Destination forces
    tickets to change, so that I review the rework before going on.
77. As a Codex user, I want the "start a fresh session" advice to name both `/clear` and `/new`, so
    that it fits my agent.
78. As a gx user, I want gx-design invoked only by me, so that the model never starts a design flow
    on its own.

### Cost and docs

79. As a gx user, I want subagent transcripts counted in `actual_cost` and the live run budget, so
    that skills that fan out (gx-code-review, gx-evolve, gx-resolve-follow-ups) are priced right.
80. As a skill author, I want a "models by alias only, never pinned IDs" rule in the tracker's skill
    conventions, so that skills follow new model releases.
81. As a contributor, I want the glossary named `GLOSSARY.md`, so that it matches domain-modeling's
    current name.
82. As a gx user, I want the three new skills shipped in gx's skill bundle and documented in
    `gx.md`, so that `gx skills install` installs them.

## Implementation Decisions

### Tracker contract (`gx-local-tracker.md`)

- New **Follow-ups** section, the one place the format lives. gx-evolve, gx-resolve-follow-ups,
  gx-investigate and gx-to-tickets link to it; gx-investigate and gx-to-tickets are updated to file
  in this format.
  - Body sections, for bugs and ideas alike: `## Source` (epic, ticket addresses, session ids, which
    skill or person filed it), `## Problem` (bug: what is wrong; idea: what is missing or painful),
    `## Evidence` (log lines, ticket ids, commits; optional for ideas), `## Suggested fix`, `## Open
    questions` (optional).
  - Always filed `status: draft`, `type: research`. Filers never guess `implement`;
    gx-resolve-follow-ups sets the real type and status when it moves the ticket.
  - Skills file follow-ups only through `gx tickets follow-up add` (see below), never with a plain
    `add`.
  - Routing, stated plainly: project-specific → `<project>:follow-ups`; gx tooling →
    `gx:follow-ups`; closed → `<project>:follow-ups-archived`.
  - A **follow-ups epic** is an epic named `follow-ups`. Its archive is `follow-ups-archived`.
  - Server recovery follow-ups (filed from investigate reports) move to this format too:
    `category: bug`, Source / Problem / Evidence / Suggested fix, and the same "seen again" Evidence
    entry for a repeat. gx-resolve-follow-ups and `export` then see one shape.
- No `type: follow-up`. `type` picks the skill; "follow-up" is where a ticket lives.
- **Skill conventions** gain: models by alias only (`sonnet`, `opus`), never pinned IDs. On Codex a
  skill names a tier (worker / strongest) with the Claude alias as example; both use the default
  model, and the strongest step uses the highest effort. No tier→ID config.
- "Who writes which status" documents `gx tickets close` as the one non-server writer of
  `cancelled`, limited to `follow-ups` epics.
- The "Map epics" section (with its Wayfinding operations) is what gx-design hands wayfinder. It
  must cover chart, claim, close and frontier; any gap is filled there, not in gx-design. It also
  documents `destination_unconfirmed`.
- Epic `ticket.md` files are edited through verbs, not by hand: `gx tickets epic add` creates one,
  `section` and `set` change one. The Wayfinding operations and "no verb creates an epic" are
  updated to match.

### Ticket schema

- New optional ticket field **`category`**: `bug | idea`. Generic: any ticket may carry it. Not named
  `kind` (that already means `kind: map`).
- New ticket type **`evolve`**: commitless, top-level only (a one-off). The server launches it under
  the gx-evolve skill (the launch-skill switch gains a case next to code-review and prompt). It
  also joins `prompt` and `investigate` in the server's list of commitless types.
- New read-only, gx-managed ticket field **`exported_at`** (timestamp), stamped by `gx evolve
  export`.
- New epic (`ticket.md`) fields:
  - **`evolved_at`** (timestamp). A cancelled or skipped retro also gets **`evolve_skipped:
    true`** next to it.
  - **`evolve_ticket`** (address of the retro's one-off). Not stamped by `skip`. A `--force`
    re-run overwrites it.
  - **`destination_unconfirmed`** (bool) on map epics seeded by gx-resolve-follow-ups. gx-design
    clears it after confirming the Destination. It was `provisional_destination`; `gx tickets
    migrate` renames the old field, and `--provisional-destination` stays a deprecated alias for
    one release.
- No migration of existing follow-ups. gx-resolve-follow-ups infers a missing `category` and reads
  any section names.

### `gx tickets add`

- Accepts `project:epic`. An unknown project, or a prefix that is not a registered project, refuses
  with nothing written. This fixes today's bug where the raw arg becomes a relative directory in cwd.
- Body frontmatter accepts `category` and `status: draft` (status `open` or `draft` only, as today).
- Filing into a `follow-ups` epic without `category` refuses.

### Cross-project addresses

- `gx tickets section`, `set`, `show`, `close` and `move` accept any registered `project:` prefix,
  like `add`. Today they refuse any address outside the cwd project. gx-evolve runs in the epic's
  project and writes "seen again" into `gx:follow-ups`; gx-resolve-follow-ups `--project gx` runs
  from another repo.
- An unknown project refuses with the same code as `add`, with nothing written.
- `move` still refuses a target in a different project from its source.

### `gx tickets follow-up add <project:epic> --body -`

- Files one follow-up and enforces the format: `category` present; `## Source`, `## Problem` and
  `## Suggested fix` present; `## Evidence` present for a bug; `## Open questions` optional.
- Always writes `status: draft` and `type: research`. A body that asks for another status or type
  refuses.
- The target must be a follow-ups epic. Each refusal has a stable code and a clear message that
  names the missing field or section. Nothing is written on a refusal.
- Skills file follow-ups only through this verb.

### Epic verbs

- `gx tickets epic add <project:epic> --kind map|loop --status draft|open [--body -]` creates an
  epic `ticket.md`. It refuses an existing epic. Like `tickets add --body`, a frontmatter block
  at the top of the body may carry the settable epic fields (e.g. `destination_unconfirmed: true`);
  any other field refuses the add.
- `section` and `set` accept an epic address (`project:epic`, no ticket id) and validate it against
  an epic schema, not the ticket schema.
  - Writable through `set`: `kind`, `status`, `destination_unconfirmed`.
  - Server-stamped and read-only to `set`: `completed_at`, `evolved_at`, `evolve_skipped`,
    `evolve_ticket`.
- gx-design writes `## Review findings` with `section` and clears `destination_unconfirmed` with
  `set`. gx-resolve-follow-ups creates the ready epic and the seeded map epics with `epic add`.

### `gx tickets move <addr> <epic> [--slug s] [--dry-run] [--json]`

Contract from the map's research, with its open questions settled:

- Source and target in the same project (any registered project, see above). Moves one flat
  ticket with status `draft`/`open` and no run history (`session_ids`, `resolved_base`,
  `iteration_status` all unset). The target epic must already exist.
- **Refuses** with stable codes: other status; run history; has a `parent` or fork children (no
  subtrees in v1); queued or live on the server (read the persisted queue/runs files when the server
  is down); a map-epic **source**; target == source; target in another project; target missing;
  the rewritten project fails project validation (dangling ref, new wait
  cycle), checked in memory before any write.
- A map-epic **target** is allowed: gx-resolve-follow-ups moves design items into area map epics
  (this amends the research's "source or target is a map" refusal). A map-epic **source** still
  refuses.
- Moved tickets keep their status; an `open` ticket stays `open` in a map epic, where tickets are
  claimed and closed by wayfinder. `move` never touches the map's Decisions or Notes.
- **Rewrites:** new id = the next free id in the target; file `<MM>-<slug>.md`; frontmatter `id`;
  a `# NN` title prefix if it starts with the old id. Every `blocked_by`/`base` ref **to** the
  ticket, in every ticket and epic `ticket.md` of the project, terminal tickets included: bare `NN`
  in the source → `target/MM`; `source/NN` or `project:source/NN` elsewhere → `target/MM`; inside
  the target → bare `MM`. Match by sibling key, not spelling. The ticket's **own** refs: bare `XX`
  → `source/XX`; `target/XX` → bare `XX`.
- Prose mentions are not rewritten; files that contain the old address are listed as warnings.
- Id reuse after moving the source's highest id is accepted.
- Locking: both epic locks, taken in path order, held through plan and write; per-file ticket locks.
  Write order: new file, then referencing tickets, then delete the old file.
- Direct write like `add` (no git in the CLI); it pings the server for both paths and the server's
  commit loop commits the rename.
- Output: the new address, or `{from, to, path, rewritten:[…], via: "direct"}` with `--json`.

### `gx tickets close <addr> --reason fixed|obsolete|duplicate|not-worth-it [--note …]`

- Only on `draft`/`open` tickets in a `follow-ups` epic.
- One step: `status: cancelled`, a `## Resolution` with reason, note and original address, then a
  move into `<project>:follow-ups-archived` (created `draft` if missing).
- Archive moves are an explicit exception to move's "never-run only" rule. Old run-log lines,
  trailers and branch names keep the old address; the Resolution records it.
- `--done` is a one-time sweep that moves already-`done` follow-ups into the archive.
- `duplicate` takes the kept ticket's address as its note.

### Settings and the server hook

- Global setting **`evolve`**: `off | maintainer | contributor`, default `off`, set explicitly,
  never guessed.
  - `maintainer`: tooling findings → `gx:follow-ups`. No `gx` project is a config error: the hook
    logs a warning, and every `gx evolve` verb refuses with a stable code.
  - `contributor`: same filing. When no `gx` project exists, Go creates a ticket-only `gx` project
    (`vcs: none`, repo = an empty workspace dir in gx's data dir, like `scratch`), unqueued:
    `gx evolve` or the server on submit, never the skill. Every `gx evolve` verb prints
    "N unexported follow-ups; run `gx evolve export`" when N > 0.
  - Project findings go to that project's `follow-ups` in every mode.
- Per project: on by default; `evolve: false` in the project file opts out (a new project override
  key). No per-epic flag.
- Deep-read cap: config key `evolve.deep-read-cap`, default 8.
- A **loop epic** is an epic that is not `kind: map`, not a one-off, and has every ticket done.
- The server stamps the epic's `completed_at` in its root-completion path, in both auto-merge
  branches, before the evolve submit. Today only the old in-process loop stamps it, so without
  this `gx evolve all` finds nothing.
- **Hook** in the server's root-completion path: when a loop epic ends, the server submits a
  commitless `type: evolve` one-off into the epic's own project with dedupe key
  `evolve:<project>/<epic>`, with `notify: true`.
  - Auto-merge on: after the land attempt, whatever its outcome (landed, parked for rebase or
    `auto-ff-merge` off, or a land error). Today a land park or a land error returns before
    epic-complete; the retro submit must not be skipped by either path.
  - Auto-merge off: once every ticket is done.
  - Never for map epics (the server never runs them), never for one-offs (including `--commits`
    one-offs and evolve one-offs), never when the setting is `off` or the project opted out, never
    when the epic already has `evolved_at`.
  - A live retro with the same dedupe key refuses a second submit.
  - The one-off's epic is named `evolve-<epic>-<yyyymmdd-hhmm>`, so a `--force` re-run never hits a
    taken name. Each retro leaves its own epic in the project.
  - The one-off request gains a type field, so a submit can ask for `type: evolve`.
- Codex launches of `type: evolve` get extra `--add-dir` entries: the project's `follow-ups` epic
  dir and the `gx` project's dir. A plain verb does not escape the Codex sandbox, since the sandbox
  covers child processes too. Later option: if the verb routes writes through the gx server and
  Codex allows the socket, the extra dirs can be dropped.
- The retro runs in the existing commitless detached worktree, discarded after. It never runs the
  project's tests.
- Main agent = the project's configured agent; no per-type model override. The one-off runs under
  the normal caps and the run budget's soft/hard limits; there is no per-run ceiling.

### `gx evolve` verbs

- All verbs are subcommands, so an epic named `inputs` or `export` never clashes. The map's verb
  names are kept.
- `gx evolve run <epic> [--force]`: the same one-off and dedupe key as the hook. Refuses an epic
  with `evolved_at` unless `--force`.
- `gx evolve skip <epic>`: stamps `evolved_at` and `evolve_skipped: true`, no `evolve_ticket`.
- `gx evolve all [--since <dur>] [--all-projects] [--yes]`: loop epics with `completed_at` in the
  last 30 days (default) and no `evolved_at`, current project by default. One-off epics are
  skipped. Lists them and asks first.
- `gx evolve inputs <epic> [--json]`: the per-ticket **flag table** (the cheap pass). One row per
  ticket: id, type, parent, status, expected/actual context, elapsed, cost, session ids, event
  counts, flags, severity. About 2–5K tokens for 30 tickets. It also writes text-only transcripts
  per session into a temp dir and prints that dir.
  - Evidence comes from the epic slug alone: ticket frontmatter (metrics, `session_ids`), the epic's
    run log (`agent_session` + `cwd`, conflict-resolution sessions), `Ralph-Loop-Ticket` commit
    trailers, and fix tickets via `parent`.
  - Transcript lookup order: Claude project transcript, then the native agent log, then the Codex
    rollout; otherwise "missing". Digesting reuses the claudehistory markdown export and adds native
    logs and Codex rollouts (message text only, long tool results trimmed).
  - **Flags** (all readable without transcripts): park, needs-repair, recovery, deadlock;
    conflict-hit; relaunch (more than one session id); nudge; context overrun (actual > 2×
    expected); elapsed > 2× the epic's median; metrics missing; has code-review fix tickets.
  - **Severity order** for the cap: needs-repair/park > conflict > overrun > the rest.
  - If the table ever gets too big, add `--summary`, not a subagent.
- `gx evolve export [-o <file>] [--again]`: reads `gx:follow-ups` drafts with no `exported_at`,
  writes `# gx follow-ups — <date>`, then per ticket a `## <title>`, a small fenced metadata block
  (`category`, `type`, source project) and the body with headings pushed down one level. Stamps
  `exported_at`. Never posts anything.
- `gx project add` hitting a name clash with the ticket-only `gx` project says to use `gx project
  set-path gx <clone>`.
- `gx project set-path` clears `vcs: none` when the new path is a git repo, so the outbox becomes a
  normal project whose tickets can commit.

### Retro stamping

- The server stamps, not the skill. The one-off's `unique` key names the epic.
- When the evolve one-off reaches `done` (zero findings included), the epic gets `evolved_at` and
  `evolve_ticket`.
- When it is cancelled, the epic gets `evolved_at` and `evolve_skipped: true`. This covers a park
  followed by a cancel (the one-off parks `needs-repair`, then a person cancels it).
- The stamp keeps the epic's other frontmatter keys.

### gx-evolve skill

Launched by the server for `type: evolve`; `disable-model-invocation: true` (the server launch
works with it, like gx-one-off). Never edits code. Steps:

1. Validate and read its own ticket; resolve the target epic.
2. Run `gx evolve inputs <epic>` directly (no subagent). Never read transcripts in the main session.
3. **Deep reads:** one subagent per flagged ticket, up to the cap, by severity, `model: "sonnet"`.
   The code-review ticket and its fix tickets are always read; a conflict-hit ticket's
   conflict-resolution session is read too. Each gets its digested transcript and events, and
   reports findings in under 500 words. Unflagged tickets are never read. "Wrong approach then
   redo" and human interventions are found inside transcripts; they are not flags.
4. **Join** findings by symptom across tickets. Flagged tickets past the cap appear as table rows.
5. **Consultant triage:** one read-only `model: "opus"` subagent, as in gx-code-review. It gets the
   joined findings plus the titles and Problem lines of each target `follow-ups` epic, its
   archive, and every open ticket that carries a `category` in the target projects (follow-ups
   already moved into a ready or map epic keep it). It removes duplicates, drops findings below
   the bar, rates by impact, and approves or rejects each with a reason.
6. **Bar:** a finding names a tooling/process cause (not project code), cites evidence (ticket
   address + event, or a transcript quote), suggests a fix, and names its target and category.
7. **File** each approved finding with `gx tickets follow-up add <target> --body -`.
   Matches change that:
   - an open ticket that carries a `category` gets a "seen again in `<epic>`" entry in its Evidence
     instead;
   - an archived `fixed` item is a regression: file a new follow-up whose Source cites the archived
     address;
   - an archived item with any other reason is rejected, with the archived address as the reason in
     Final disposition.
8. **Record** in its own ticket body: `## Result` (counts: tickets, flagged, deep-read, approved,
   rejected; filed and "seen again" addresses; one line per approved finding; never empty), then
   **Flag table**, **Raw findings** (one block per subagent), **Consultant triage** (verbatim),
   **Final disposition** (filed / seen again / rejected + reason). No repo file.
9. Close out commitless, like gx-code-review.

One retro looks at one epic only, never past epics.

### gx-resolve-follow-ups skill

Manual; `disable-model-invocation: true` (manual and write-heavy, like gx-cleanup). Works on
`<project>:follow-ups` (current project, or `--project <name>`). Main session = whatever the user
runs. Steps:

1. **Verify:** one `model: "sonnet"` subagent per item, about 8 in parallel. Reads the code and
   logs the ticket cites plus `git log` since filing; no reproducing, no tests. Returns a verdict
   (relevant / fixed / obsolete + evidence), the files a fix would touch, 0–N blocking
   questions, each tagged `quick` or `design`, and a suggested destination (ready epic as
   `implement`, or map epic as `grilling` or `research`). Undecided counts as relevant.
2. **Dedupe** from verdicts only: keep the oldest, merge the others' Source/Evidence in as "seen
   again" entries, close the newer as `duplicate` with a pointer.
3. **Plan:** one table, item → close (reason) / merge into X / ready epic / map epic Y, with the
   suggested destination shown. No writes. Routing: any `design` question or more than 2
   questions → map epic as `grilling`; needs investigation but no decision → map epic as
   `research`; otherwise ready epic as `implement`, with the quick questions asked at approval.
4. **Approve once:** the user edits or approves the whole table; quick questions are asked in this
   same step and the answers written into the ticket body. The run never grills.
5. **Execute:** creates epics (`gx tickets epic add`), closes (`gx tickets close`), merges, moves
   (`gx tickets move`), type/status writes.

Produced epics:

- **Ready epic** `follow-ups-<date>`, one per run, created with `epic add --kind loop --status
  open`; not queued. Tickets `open`, `type: implement` only: the queue does not really run
  `research` tickets today (they would launch under gx-implement on the feature branch). Tickets
  whose expected files overlap are chained with `blocked_by`. No size cap.
- **Area map epics**: grouped by area of code/tool (not bug vs idea), 1–8 items, singletons fine,
  plain descriptive slug. Created in one call with `epic add --kind map --status open --body -`,
  the body's frontmatter carrying `destination_unconfirmed: true`. Body seeded
  with a provisional Destination ("decide these items, then a spec `docs/specs/<slug>.md`"), Notes
  ("Triaged from follow-ups on <date>" + source addresses), empty Decisions so far and Not yet
  specified. A design item moves in as an `open` `grilling` ticket, keeps its follow-up sections
  and gains a `## Question` built from its Open questions. A research-only item moves in as an
  `open` `research` ticket, which wayfinder resolves AFK.

Done = `follow-ups` holds no tickets.

### gx-design skill

The `/myway` state machine, ported. `disable-model-invocation: true`. Tracker fixed to gx.

- **Dependency check first:** wayfinder, to-spec, grilling and domain-modeling must exist in the
  agent's skill root (same check on Codex). If any is missing, stop, name them and point at
  mattpocock/skills. research/prototype are not checked. Not vendored.
- Keeps the "Read the SKILL.md directly" workaround for user-invoked skills.
- Tells wayfinder to use the tracker's "Map epics" section for its operations.
- Phases **map → review → implement**. The review phase includes to-spec writing the spec, so it
  covers both "spec" and "review" of the Destination's map → spec → review → impl. Next-step
  detection, first match wins:
  1. No map epic → wayfinder "Chart the map".
  2. Map has `destination_unconfirmed: true` → grill the Destination alone, clear the field with
     `set`, then continue into step 3 in the same session, unless seeded tickets had to be edited, closed or
     added; then stop after reworking the map.
  3. Open tickets or non-empty Not yet specified → wayfinder "Work through the map", one ticket.
  4. Way is clear, no `docs/specs/<epic>.md` → subagent runs to-spec, seeded with the map body and
     each closed ticket's title and gist.
  5. Spec exists, no `## Review findings` on the map → design-review subagent (rubric stays in
     gx-design) writes the checklist section with `gx tickets section`.
  6. Unchecked review findings → grilling session on them, amending the spec in place and checking
     each off; falls through to 7 once all are resolved.
  7. No `<epic>-impl` epic → gx-to-tickets publishes into `<epic>-impl`.
  8. `<epic>-impl` exists → report and stop.
- "Start a fresh session (`/clear` in Claude, `/new` in Codex)" replaces `/clear` advice.
- Ends with the one-line phase report, as `/myway` does.
- `/myway` is deleted from dotfiles by hand later; no alias.

### Cost tracking

- The session-stats and session-cost readers also sum `<session>/subagents/*.jsonl` next to the main
  transcript. This feeds the landed `actual_cost` stamp and the live run budget.
- Subagent usage adds to cost only. Peak context occupancy stays from the main transcript, since
  subagents run in their own context.

### Skill bundle and docs

- gx-evolve, gx-resolve-follow-ups and gx-design join the bundle's embed list, `gx.md`'s layout and
  invocation-policy section, and the bundle test's invocation-policy table.
- `gx.md` gets a gx-design paragraph explaining its dependency on external skills, an exception to
  "no dependency on a personal dotfiles skill tree".
- Rename `CONTEXT.md` → `GLOSSARY.md` and update live references only (agent docs, domain doc,
  skills, code comments). Historical records (CHANGELOG, ADRs, old specs and plans) keep the old
  name. No `CONTEXT.md` symlink; the CHANGELOG entry notes the rename. Glossary terms already
  added while charting: **Follow-up** (with category, resolved, closed, follow-ups archive) and
  **Retro**.

## Testing Decisions

- A good test drives a verb or the server through its public entry point against a temp ticket
  store and checks what lands on disk or what is refused. It does not check internal helpers.
- **Seam 1: the CLI verb run functions** (existing; prior art: the `tickets add`, `set`, `section`,
  `filter-run-log` and `server one-off` command tests). Cover:
  - `add project:epic` files cross-project; an unknown project refuses and writes nothing (the
    stray-directory bug); `category` and `status: draft` are written; a `follow-ups` add without
    `category` refuses.
  - `section`, `set`, `show`, `close` and `move` reach a ticket in another registered project; an
    unknown project refuses and writes nothing; `move` to a target in another project refuses.
  - `move`: each refusal code; ref rewrites (bare, epic-qualified, project-qualified, inside the
    target, terminal tickets, the ticket's own refs); a map-epic target allowed and a map source
    refused; a missing target refuses; `--dry-run` writes nothing; `--json` shape; prose
    warnings.
  - `follow-up add`: a valid bug and idea are written `draft` + `research`; each missing field or
    section, a non-follow-ups target, and another status or type refuse with their code and write
    nothing.
  - `epic add`: map and loop epics with each status; an existing epic refuses. `section` and `set`
    on an epic address: writable fields change, server-stamped fields refuse, other keys are kept.
  - `close`: each reason; Resolution content; archive created `draft`; refuses outside `follow-ups`
    and for other statuses; `--done` sweep.
  - `evolve inputs`: a fixture epic (frontmatter, run log, commits, a transcript, a missing one, a
    subagents dir) gives the expected rows, flags and severity, and digest files.
  - `evolve skip`, `all` (window, `--since`, map epics and one-offs excluded, `evolved_at`
    excluded), `run <epic>` with and without `--force`, through the fake server client.
  - `maintainer` with no `gx` project: every `evolve` verb refuses with its stable code.
    `contributor` with unexported follow-ups: every `evolve` verb prints the export reminder; none
    when N = 0.
  - `evolve export`: bundle shape, `exported_at` stamped, `--again`.
  - validation accepts the new fields and the `evolve` type.
  - `project set-path` to a git repo clears `vcs: none`.
- **Seam 2: the server runner with a fake runner** (existing; prior art: the runner tests for
  landing and closing a root, and launch-skill-by-type). Cover:
  - root completion stamps the epic's `completed_at` in both auto-merge branches;
  - auto-merge on: a landed epic, a land-parked epic and a land error each submit one evolve
    one-off with the dedupe key; auto-merge off: submit once all done;
  - no submit when the setting is `off`, the project opted out, the epic is a map epic or a
    one-off, or it has `evolved_at`;
  - a second submit is refused while one is live;
  - `type: evolve` launches under gx-evolve; a Codex launch gets the extra writable dirs;
  - a finished retro stamps `evolved_at` + `evolve_ticket`; a cancelled one, and a parked one
    then cancelled, stamp `evolve_skipped: true`.
- **Seam 3: session cost** (existing; prior art: the report-metrics tests). A fixture session with
  a subagents dir sums both into cost.
- **Seam 4: the skill bundle test** (existing). The three skills are embedded, have valid metadata,
  and carry their invocation-policy flag.
- The skills' prose has no automated seam beyond seam 4. Each is checked by a manual run: gx-evolve
  on a recent epic whose transcripts still exist, gx-resolve-follow-ups on `gx:follow-ups`, and
  gx-design on one seeded map epic.

## Out of Scope

- Fixing the current follow-ups themselves; the skill drains them, this spec only builds it.
- Running gx-resolve-follow-ups automatically; it stays manual.
- Retros of map epics; only loop epics get gx-evolve.
- Changing the gx-code-review skill.
- Importing a contributor's exported bundle into `gx:follow-ups`; added by hand or a later skill.
- A category filter in the TUI, or showing every frontmatter field on the Tickets tab (filed as a
  follow-up).
- Fixing the duplicate id `23` and untitled ticket 45 in `gx:follow-ups`; by hand or the first run.
- Vendoring the mattpocock skills into gx.
- Deleting `/myway` from dotfiles.

## Further Notes

Decision trail: the wayfinder map `gx:gx-evolve` and its tickets 01–10. Open questions the map did
not settle were resolved in a design review and are folded into the decisions above.
