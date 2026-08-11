# `status` ownership is a property of the writer, not of the epic

`.scratch/` holds two kinds of epic under one file format. In a **hand-driven** epic — a wayfinder
map, or anything a person works directly — the person owns `status` and `gx tickets set` is their
channel. In a **loop-driven** epic gx owns `status` and writes it only after the iteration's commits
land, while the agent reports `iteration_status` (ADR 0019). Nothing on disk records which kind an
epic is, and nothing needs to: the distinction is about **who is writing**, so it is enforced at the
writer, not at the epic.

The restriction that follows binds exactly one caller — the **iteration agent** — recognised by the
working directory sitting on a `ralph-loop/*` branch (`iterBranch`, `ralphloop/labels.go`). From such
a branch `gx tickets set --status` is refused for every value, with one exception: promoting a
`draft` ticket to `open`, which mid-flight forking requires and which asserts nothing about landing
or ownership. Separately, and for every caller in both modes, `needs-answer` and `needs-repair` are
rejected outright — they mean "a machine parked this for a person", and their only writers are the
agent's `--iteration-status needs-answer`, gx's adoption write, and the orchestrator gate. The CLI's
settable set is therefore `draft`, `open`, `claimed`, `done`.

An `epic.yaml` `mode:` field was designed for this and then dropped, because every consumer turned
out to have a better source. The guard keys off the caller, so it never asks what kind of epic it is
in. The Tickets-tab status menu keys off whether a loop is running that epic *right now*, live from
`loop_registry` — a strictly better predicate than "was ever loop-driven", since a finished loop epic
behaves like a hand-driven one. And the body-section rules from ADR 0018 are **write-conditional**,
firing on the write that sets the status rather than as a property every ticket must satisfy at rest,
which self-scopes with no mode check at all. Persisting a field that nothing reads would have bought
a schema change, a CLI to write it, a stamping backstop, and edits to two skills.

Ticket 03's original shape — a global `--force` gate on `claimed`/`done` — was rejected on a premise
error. It read as though the CLI's only callers were iteration agents; they are not, and applied
globally it would have broken the workflow that produced the map the decision came from. A
human-directed guard also protects nothing in practice: a person's real channels are the TUI and a
text editor, neither of which passes through `runTicketsSet`, so the guard would only tax the one
channel they use least and get aliased away. And a flag cannot express caller identity in the first
place — the agent it is meant to stop can simply pass it.

## Consequences

- No override flag exists. `--force` keeps its single prior meaning on this command (proceed despite
  unresolved `blocked_by`) and is not overloaded.
- ADR 0019's `status`/`iteration_status` split is unchanged; this decides only who may write which,
  through which channel.
- The mid-flight fork protocol's step 3 — "the original is closed as `done`" — is an agent
  `--status done` write, and is the same defect ticket 03 closed. It becomes
  `--iteration-status finished` (with `commitless` where applicable), gx writing `done` at landing.
  Step 2's `draft → open` promotion stands and is the guard's one exception.
- Detection degrades safely: an unrecognised branch means no guard, never a false refusal. A person
  who checks out a `ralph-loop/*` branch and runs `gx tickets set` is refused, and can step out of it
  or use the TUI.
- Branch namespace was chosen over cwd path matching (iteration worktrees are expected to move, and a
  stopped iteration keeps its branch while dropping its worktree) and over an environment variable
  (`herdr agent start` accepts no env, so it would mean typing an `export` into the pane's shell).
- Tickets in a hand-driven epic are never rejected for lacking a `## Needs Answer` section, because
  the requirement rides on the write that sets the status, not on the file.
