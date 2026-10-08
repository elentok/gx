---
name: gx-investigate
description:
  Investigate a reported gx/ralph-loop bug — tickets stuck in the queue, wrong scheduling
  decisions, ticket status or Queue-tab state that looks wrong, a stalled or misbehaving agent
  session. Gives the ralph-loop background and the log/state-file inventory needed to diagnose
  from evidence instead of guessing. Never edits product code; may repair ralph-loop state (land,
  reset, unpark a stuck ticket) with the human's explicit go-ahead. Publishes findings as a research
  ticket or a report.
---

# gx Investigate

Diagnose a gx/ralph-loop bug report by reading the state it actually left behind, rather than
re-deriving behavior from source alone.

## Background: the ralph-loop process

`ralphloop.Run` drives every unblocked ticket in one epic to completion, up to `MaxParallel`
concurrently (default 2). Each ticket's status comes from its own `Status:`/`Blocked by:`
frontmatter; `Frontier` filters to open, unblocked, unclaimed tickets, lowest number first, and
`RunScope` narrows that further when the caller requested specific ticket IDs — walking each
ticket's `Parent` chain so mid-run forks and code-review-spawned tickets stay in scope
dynamically. Each scheduling pass claims a frontier ticket, launches an iteration (new git
worktree, agent started via `herdr`, the `gx-implement` skill prompt sent), waits for completion
or a pause condition (smart-zone context ceiling, rate limit), cherry-picks the resulting commits
onto the epic's feature branch, and marks the ticket done. A freed slot is immediately backfilled
from the frontier. `Run` exits once every ticket in scope reaches a done-family status.

For the exact rules (statuses, blocking, frontier, mid-flight forks), treat
[gx-local-tracker.md](../gx-local-tracker.md) as authoritative — this section is background, not a
substitute for it.

## Where state lives

Ticket state lives under the canonical root from `gx tickets root` — always resolve it first
(`root=$(gx tickets root)`), even when you're already inside a worktree or a subdirectory; a
bare-repo checkout with linked worktrees keeps one shared root at the bare repo's own path, not
`.scratch/` at cwd. See gx-local-tracker.md.

- **`<root>/<epic-slug>/issues/*.md`** — the tickets themselves. Source of truth for status,
  `blocked_by`, and `parent` (the only edge — fork subtrees are derived from it, never stored). See
  gx-local-tracker.md for the field reference.
- **`<root>/<epic-slug>/run-log.jsonl`** — the append-only scheduler/iteration event log
  (`ralphloop/eventlog.go`). One JSON line per event: `iteration-started`/`-finished`,
  `cherry-picked`, `conflict-hit`/`-resolved`, `paused-smart-zone`, `paused-rate-limit`,
  `needs-answer`, `needs-repair`, `commitless`, `deps-installed`, and **`scheduler-scan`** — one
  entry per `claimNext` pass, listing every ticket's disposition that pass (`claimed`, `frontier`,
  `out-of-scope`, `blocked` + reason, `settled`, `unclaimed`). Query it with
  `gx tickets filter-run-log <epic> --ticket <id> [--event <type>]` (both filters optional,
  `--event` repeatable) rather than hand-rolling a `python3`/`jq` filter. This is usually the
  fastest way to see *why* the scheduler picked or skipped a ticket, especially for "it's in the
  tree but never starts" reports.
- **`~/.config/gx/queue-state.json`** — the Queue tab's own UI bookkeeping (which tickets are
  checked, their display order). A cache that can drift from the tickets it describes — never the
  source of truth for ticket status.
- **`~/.config/gx/config.json`** — effective gx config (`gx config show` prints it resolved).
- **Agent session transcript** — `~/.claude/projects/<slugified-cwd>/<session-id>.jsonl`, keyed by
  the `Cwd`/`AgentSession` recorded on that ticket's `iteration-started` event. Locate it with
  `gx claude session-path <session-id> [--grep <pattern>]` instead of a manual `fd` search — it
  scans every project directory for you, so you don't need to slugify the cwd by hand; `--grep`
  prints only the transcript's matching lines instead of the path. Use it to see what the agent
  actually did inside one iteration.
- **`herdr`** (pane/session driver) is an external binary gx shells out to — its own pane/session
  state isn't a file in this repo and isn't readable through gx. If the symptom is at the
  tmux-pane level (hung pane, wrong agent launched), you're debugging herdr's process, not gx's
  state.
- **`gx.log`** (relative to cwd, e.g. `ralphloop/gx.log` when gx is run from there) is Bubble Tea's
  TUI debug log, written by any `logger.Debug()` call in the UI packages — global per process
  invocation, unrelated to ralph-loop scheduling. Don't confuse it with `run-log.jsonl`.

## Where to start

1. Run `gx tickets root` to get `<root>`, then identify the epic: `<root>/<epic-slug>/`.
2. Run `gx tickets filter-run-log <epic> --ticket <id>`, filtered to the ticket(s) in question. The
   most recent `scheduler-scan` entry's `scan` list gives every ticket's decision and reason as of
   that pass.
3. Read the affected ticket with `gx tickets show <addr>` and check it against gx-local-tracker.md's
   field reference — most "queued but stuck" reports are a frontmatter field (`parent`,
   `blocked_by`, `status`) not doing what the UI implies, not a scheduler logic bug.
4. If the discrepancy is between the Queue tab and the tickets themselves, check
   `queue-state.json` for drift.
5. If the symptom is inside one agent session (hung, wrong edit, crashed), pull that iteration's
   transcript via `gx claude session-path <session-id>`, using the `Cwd`/`AgentSession` on its
   `iteration-started` event to get the session id.
6. Before concluding a symptom is a live bug, rule out a notification mute: check
   `~/.config/gx/notifications-state.json` and the affected ticket's own `Mutes` frontmatter field.
   A muted event can look identical to a stuck ticket or a scheduler that silently skipped it.
7. Once you have a concrete hypothesis, verify it against the source it actually lives in
   (`ralphloop/loop.go`, `scope.go`, `schedule.go`, `tickets/status.go`) — this file is a map, not
   a substitute for reading the code the bug is in.

## Known gotchas

[gotchas.md](gotchas.md) is a running list of previously-diagnosed bugs, one line each, pointing at
the commit or ticket that fixed them. Check it before starting — the report in front of you may
already be solved.

After you diagnose (and, elsewhere, fix) a bug through this process, append one line + a pointer
to gotchas.md yourself. Don't re-explain what the linked commit/ticket already documents.

## Recovering a stuck ticket

Charter: **never edits _product_ code — may repair ralph-loop state.** The repair tools are
`gx tickets land | verify | reset | unpark`. All four share one `--json` contract: exit 0 with the
result, or exit 1 with `{"refused": true, "reason": "<code>", "message": "..."}`. Branch on `reason`
(`land_locked`, `iteration_branch_missing`, `live_agent_on_tab`, `fork_children`, `status_refused`,
`commitless`, `land_conflict_pending`, …), never on `message`.

**Go-ahead.** Every write (`land`, `reset`, `unpark`, `land --continue/--abort`) needs the
human's explicit go-ahead in this session. `verify` is read-only and always free. In attended mode
this is the only rule; unattended mode is below.

## Unattended mode

An investigate ticket (`type: investigate`) runs this skill with no human in the session. Its prompt
begins `Matched catalog entry: …` and names the entry. Everything above (background, state,
diagnosis) applies unchanged; only the writes differ.

<unattended-verbs>

- allowed: exactly the verbs the matched entry lists in its `verbs` (as named in the prompt),
  whatever they are — e.g. R3's entry grants its `land`. The entry's verbs are the whole grant; the
  prompt's entry is the authority, not your own judgment of what would help.
- never: a pane nudge or any other verb the matched entry does not list (an unlisted `land`,
  `reset`, `unpark` or `land --continue/--abort` included), or editing product code.

</unattended-verbs>

- **Entry matched** → diagnose, then apply only the entry's verbs against the failed ticket (the
  investigate ticket's `parent`). Report at the action, as above.
- **`No catalog entry matched`** → report the diagnosis and park the failed ticket. Do nothing else;
  propose any other command in the report for a human to run.
- Never wait for a go-ahead, and never ask: an unattended session has no one to answer.

Attended mode (`m` → Investigate opens an interactive tab, not a ticket) keeps the human go-ahead
exactly as before.

**Completeness is your job, not `land`'s.** `land` always leaves the ticket `done`, so judge first:

- Check each acceptance criterion against the diff of the iteration branch.
- Run the repo's checks in the iteration worktree.
- The transcript is advisory only — an agent's account never outweighs the diff.
- Thin evidence (no worktree, unreadable diff, checks can't run) blocks _your_ autonomy to propose
  `land`. It never blocks the human's explicit "land it anyway".

**Choose the ending by status first, then completeness.**

- `needs-answer` → never land or reset. Route to answer-then-unpark: the human answers in
  `## Needs Answer` (`m` menu → "Answer…", or "Answer in pane" if the pane is live), then
  `gx tickets unpark <epic> <id>` (or `m` → "Resume (I answered)").
- `claimed` / `needs-repair` (`land` also accepts `done`; `reset` needs `--force` for `done`):
  - Complete → `gx tickets land <epic> <id> --json`. Commits already landed with a stale status is
    the common case; `land` just writes `status: done`.
  - Incomplete → `gx tickets reset <epic> <id> --reason "<why>" --json`. The branch is kept under
    `ralph-loop/attic/`; the reason is written to Comments as unverified partial work.
- `draft` / `open` → nothing ran; neither ending applies.

**Every failure stops and reports.** Name the reason code and the human's next gesture, including
the `m` menu path (`m` on the ticket → Investigate / Answer… / Resume). Examples:
`fork_children` → reset a child instead; `iteration_branch_missing` → `land --from/--to` with an
explicit range; `land_conflict_pending` → resolve in the feature worktree, then
`land --continue` or `--abort`; `land_locked` → retry later. Don't retry around a refusal.

**A live agent on the tab starts an investigation, not a stop.** `live_agent_on_tab` stands — don't
use `--ignore-live-tab` to get past it. Read iteration status, the run log, the transcript and
`gx tickets verify <epic> <id>` to answer _why it is not finishing_.

**Report at the action, not in the write-up.** `land` and `reset` write their own run-log event
(`manual-land`, `ticket-reset`). After a landing, add a `## Comments` note on the ticket: what you
verified, and that recovered elapsed time is the least trustworthy of the three metrics. Do not
repeat this in the diagnosis ticket.

## Output

Diagnosis never edits product code. Recoveries are covered above.

Where the diagnosis is published depends on what the bug is *about*, not which epic you were
looking at when you found it:

- **Bug is in the ticket/epic's own deliverable** (something one of its tickets built or should
  have built) — publish as a `type: research` ticket in that epic (`<root>/<epic-slug>/`),
  following gx-local-tracker.md's template: what's wrong, the evidence (log lines, ticket IDs),
  and a suggested fix direction. A follow-up fix ticket can then `blocked_by` it.
- **Bug is in gx/ralph-loop tooling itself** (scheduler, a skill's own instructions, herdr
  plumbing) surfaced *while* running some epic, but not part of what that epic's tickets asked
  for — publish in `<root>/follow-ups/issues/` instead, even though you found it investigating a
  specific epic. Don't clutter that epic's own issue list with a finding about gx itself.
  - If `<root>/follow-ups/` doesn't exist yet, create the directory (no `epic.yaml` needed until a
    loop actually runs against it).
  - File it as `type: research, status: draft` — `draft` because it's a diagnosis handed off for a
    person to plan, not schedulable work; `research` matches `gx-implement`'s commitless-by-type
    handling for a diagnosis-only ticket.
  - Follow `follow-ups`'s established ticket shape (see e.g. `follow-ups/issues/01-*.md`): a
    `## Context` section naming the epic/investigation this came from, followed by the diagnosis
    itself — what's wrong, the evidence (log lines, ticket IDs), and a suggested fix direction. A
    follow-up fix ticket can then `blocked_by` it.
  - This is scoped to diagnosis tickets filed by this skill only — other "file into whatever epic
    is active" call sites (code-review fix tickets, spec-review follow-ups) are untouched.
- **No epic in scope at all** (a one-off question, nothing under the tracker root) — report the
  diagnosis directly instead of publishing anything.

When unsure which of the first two applies, ask: would this bug still exist if the epic you were
investigating had never been authored? If yes, it's a tooling bug — `follow-ups`, not the epic.
