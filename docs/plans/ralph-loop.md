# gx ralph-loop

A ralph-loop orchestrator, built into `gx`, that drives Claude Code agents through the tickets of a
`to-tickets`-produced local epic — one iteration worktree per ticket, cherry-picked onto a shared
feature branch, run up to `--max-parallel` at a time inside a single herdr workspace — with
guardrails against context blowout and Claude usage rate-limits, and a history log for retrospect.

Decisions below are indexed to their source tickets under
[`.scratch/ralph-loop/`](../../.scratch/ralph-loop/map.md); see those for full rationale.

## Why gx

gx already owns this domain: the `tickets` package parses the local tracker's
`Status:`/`Blocked by:` file convention (including bold-markdown variants) into `Ticket`/`Epic` with
`RenderedStatus`/`UnresolvedBlockers`/`doneStatuses`; `ui/worktrees/herdr.go` already wraps
`herdr workspace list/create/focus`; `runner` already runs child commands with resilient stdio.
Building this in `blf` instead would mean duplicating all three. (Decision:
[01](../../.scratch/ralph-loop/issues/01-host-project-and-location.md))

## CLI surface

```
gx ralph-loop {epic-name} [--skill implement] [--max-parallel 2] [--smart-zone 110000]
gx ralph-loop resume {epic-name}
gx ralph-loop report {epic-name}
```

- `--skill` (default `implement`) — the skill each iteration invokes as an explicit slash command.
- `--max-parallel` (default `2`) — concurrently-running iterations.
- `--smart-zone` (default `110000`) — context-token ceiling before auto-recovering (see
  [Smart-zone auto-recovery](#smart-zone-auto-recovery)). Lowered from an earlier 150000 default to
  leave headroom for the `/compact` + finish-up turn before hitting Claude Code's actual ceiling.
- Poll interval and the herdr workspace label (always `= epic-name`) are hardcoded.

The loop exits (prints the final summary, exit 0) once every ticket in
`.scratch/{epic-name}/issues/` reaches a done-family `Status:` (`tickets.doneStatuses`:
`done`/`resolved`/`wontfix`/`closed`/`superseded`/`implemented`). A ticket parked at `needs-info`
blocks exit until a human resolves it. (Decision:
[10](../../.scratch/ralph-loop/issues/10-cli-surface-and-exit-condition.md))

## herdr topology

One herdr workspace per epic run (`herdr workspace create --label {epic-name}`, reusing
`herdrFindWorkspace`/create-or-focus from `ui/worktrees/herdr.go`). Inside it:

- The **feature worktree** (branch `{epic-name}`) gets one tab, mostly idle between merges.
- Each **running iteration** gets its own tab, named `iter-NN`, up to `--max-parallel`.

The orchestrator process itself is headless — a plain background `gx ralph-loop` process, not a pane
in the workspace. (Decision: [02](../../.scratch/ralph-loop/issues/02-herdr-topology.md))

## Ticket scheduling

No separate lock file or claim mechanism — reuse the tracker's own `Status:`/`Blocked by:`
convention, the same one `tickets/ticket.go` and `tickets/status.go` already parse:

- **Claim**: write `Status: claimed` into the ticket file before starting.
- **Done**: write `Status: done` once cherry-picked onto the feature branch.
- **Unblocked**: every `Blocked by:` number has a done-family status.
- **Frontier**: open, unblocked, unclaimed tickets — re-scanned every poll tick.
- **Tie-break**: lowest ticket number wins, up to `--max-parallel` slots.

(Decision: [03](../../.scratch/ralph-loop/issues/03-ticket-scheduling.md))

## Iteration lifecycle

Per claimed ticket `NN-slug`:

1. Create an iteration worktree on branch `ralph-loop/iter-NN`, based on the feature branch's
   current tip; open tab `iter-NN` in the epic's workspace.
2. Launch `claude` in that pane; wait for `idle` (initial prompt reached).
3. Send `/{skill} .scratch/{epic-name}/issues/NN-slug.md` as the initial prompt — an **explicit
   slash-command invocation**, not a prose description, since `implement` has
   `disable-model-invocation: true` and won't reliably trigger otherwise.
4. Wait for `agent_status == working` (confirms the task started), then wait for `idle` **or**
   `done` (either is a valid finished state, depending on whether the tab was seen while
   backgrounded).
5. Hand off to cherry-pick.

A resumed iteration (see [Pause/resume](#pause-resume-and-crash-restart)) re-enters at step 4.
(Decision: [04](../../.scratch/ralph-loop/issues/04-iteration-lifecycle.md))

## Cherry-pick and conflict handling

- **Cherry-pick** the iteration's full commit range in order (`git cherry-pick
  <iteration-base>..<iteration-branch>`) into the feature worktree — not squashed, so
  `/implement`'s own TDD-sized commits and any conflicts are handled commit-by-commit.
- **On success**: `Status: done`, delete the iteration worktree and its tab.
- **On conflict**: resolve in the **feature worktree itself** (that's where the conflict markers
  are) — a fresh pane there invokes `/resolving-merge-conflicts`; once it commits the resolution,
  continue the remaining cherry-pick range, then proceed to the success path.
- **On zero commits** (agent finished but nothing landed on the iteration branch): `Status:
  needs-info`, leave the worktree for inspection, move on to other unblocked tickets. Not silently
  retried. **Unless** the agent itself declared the zero-commit finish intentional — e.g. exploration
  concluded no code change was warranted — by setting `commitless: true` (via `gx tickets set
  --commitless true`) alongside moving `Status:` off `claimed` before finishing. In that case the
  worktree/tab are cleaned up normally instead, and a `commitless` event is logged rather than
  `needs-info`. `commitless: true` on a ticket still at `claimed` doesn't count — indistinguishable
  from an agent that simply never called it, so it falls through to the needs-info path above. A done
  ticket with `commitless: true` is also exempt from startup reconciliation's landed-commit
  verification (see [Pause, resume, and crash-restart](#pause-resume-and-crash-restart)), the same way
  a `Status: superseded` ticket already is — neither ever had a commit to verify.

The conflict-resolution agent is **subject to the smart-zone guardrail** exactly like any other
iteration agent — no special-cased stricter limit. A breach during conflict resolution gets the same
pause-and-block treatment as any other iteration breach (see
[Pause, resume, and crash-restart](#pause-resume-and-crash-restart)); the feature worktree simply
stays mid-cherry-pick while paused, which is safe since cherry-picks are handled one at a time.

(Decision: [05](../../.scratch/ralph-loop/issues/05-cherry-pick-and-conflicts.md))

## Smart-zone guardrail

herdr exposes no token/context field anywhere (`agent get`/`list`/`explain`), so this reads Claude
Code's own session transcript directly:

- Transcript path: `~/.claude/projects/<slugified-cwd>/<session-id>.jsonl`.
- herdr's `agent_session.value` (a UUID) **is** that session id — confirmed byte-identical to the
  transcript's `sessionId` field and filename, so the path is constructed directly, no globbing.
- Each `type: "assistant"` line has `message.usage.{input_tokens, output_tokens,
  cache_read_input_tokens, cache_creation_input_tokens}`.
- **Current context occupancy** = those three input-side fields **from the last assistant line
  only**. Each turn's cache-read total already reflects nearly all prior context reused via caching
  — summing across turns/lines would drastically overcount.
- Compare against `--smart-zone` on every poll tick per running iteration. Seek from EOF backward
  for the last matching line rather than parsing the whole file, so checks stay cheap regardless of
  transcript length.

(Decision: [06](../../.scratch/ralph-loop/issues/06-smart-zone-guardrail.md))

## Smart-zone auto-recovery

**On smart-zone breach** (an iteration's occupancy exceeds `--smart-zone`), for both Claude and
Codex:

1. `herdr agent send-keys <pane> ctrl+c` — interrupt immediately, no "wrap up first" step. The pane
   stays alive, not killed.
2. Send `/compact` as a prompt and wait for it to finish.
3. Send a finish-up prompt telling the agent it was stopped for exceeding the smart-zone limit, that
   its conversation was compacted, and to wrap up quickly — following the `implement` skill's
   follow-up-ticket instructions if it can't finish outright.
4. Fall back into the normal wait-for-finish poll loop — no blocking, no human step. If occupancy is
   still over `--smart-zone` on a later tick, the same three steps just fire again; there's no retry
   cap.
5. The ticket's `Status:` stays `claimed` throughout, same as any other iteration.
6. The scheduler is **not** paused — other tickets keep getting claimed and run while this iteration
   recompacts. (Unlike rate-limit/needs-attention below, a smart-zone breach is local to one
   iteration and self-heals; there's no reason to stall the rest of the run over it.) An
   iteration-paused/resumed event still brackets the ctrl-c→compact→prompt sequence for the TUI badge
   and `run-log.jsonl`, even though nothing is actually blocked.

**On crash/restart** (the process died, not a controlled interruption): a fresh
`gx ralph-loop {epic-name}` reconciles with **no separate state file**, using only:

- Deterministic naming (`ralph-loop/iter-NN` branch, `iter-NN` tab).
- The ticket files' own `Status:` (already the source of truth for scheduling).

For every `Status: claimed` ticket on startup: look for a matching `iter-NN` tab/worktree in the
epic's workspace. Found → reattach, resume the lifecycle. Not found → the process died before/during
that iteration; revert `Status:` to open so the scheduler re-runs it fresh.

(Decisions: [07](../../.scratch/ralph-loop/issues/07-pause-resume-and-crash-restart.md),
[12](../../.scratch/ralph-loop/issues/12-smart-zone-auto-recovery.md) — 12 supersedes 07's blocking
pause-and-manual-`resume` behavior for the smart-zone case specifically; 07's crash/restart
reconciliation and its blocking pause for rate-limit/needs-attention are unchanged, see below.)

## Rate-limit (5h/weekly) handling

- **Detect**: scan pane output (`herdr agent read --source recent-unwrapped`) for Claude Code's own
  known rate-limit message text (includes a reset time) — distinct from a generic `blocked` status,
  which can also mean an ordinary permission prompt.
- **Pause**: unlike a smart-zone breach, this genuinely stops the scheduler — usage limits are
  typically account-wide, so other iterations would likely hit the same wall. Nothing for a human to
  fix, though: fully automatic.
- **Resume**: parse the reset time from the matched message; sleep until then (falling back to
  fixed-interval polling if parsing fails); once past reset, automatically re-prompt whichever agents
  were blocked.

**Message text**: reuse the already-tested detection from `claude-box`'s `beads_loop.sh` (a
near-identical prior tool), ported from bash to Go:

```bash
# matches "You've hit your session limit · resets 10:10am (UTC)" and
# "Claude usage limit reached" style variants, without tripping on incidental
# mentions like "Added a rate limit of 100 requests per minute"
bl_detect_session_limit() {
	grep -qiE '(hit|reached|reset)[^.]{0,20}(session|usage) limit|(session|usage) limit[^.]{0,40}(hit|reached|reset)' <<<"${1:-}"
}
# extracts the reset clock time, e.g. "10:10am"
bl_reset_time_token() {
	grep -oiE '[0-9]{1,2}(:[0-9]{2})?[[:space:]]*[ap]m' <<<"${1:-}" | head -n1
}
```

Same reset-time-to-sleep-duration math as that file's `bl_seconds_until_reset`/`bl_wait_for_reset`:
assume UTC, roll to the next day if the parsed clock time has already passed today, add a small
buffer past the computed reset before resuming.

(Decisions: [08](../../.scratch/ralph-loop/issues/08-rate-limit-handling.md),
[11](../../.scratch/ralph-loop/issues/11-rate-limit-message-text.md))

## Reporting and history log

- **Event log**: append one JSON line per lifecycle event (started, finished, cherry-picked,
  conflict hit/resolved, paused on smart-zone/rate-limit, resumed, needs-info) to
  `.scratch/{epic-name}/run-log.jsonl` as it happens. Each event carries at least: ticket number,
  herdr pane/tab id, the `agent_session` UUID (= the Claude Code session id, for jumping straight to
  that iteration's own transcript), and a timestamp.
- **`gx ralph-loop report {epic-name}`**, runnable any time (including mid-run):
  1. Reads `run-log.jsonl` for the event skeleton (order, concurrency).
  2. Per session UUID, reads that session's Claude Code transcript for: peak context occupancy (the
     per-turn figure from the smart-zone section, not summed), total duration (first/last
     timestamp), and cost — this time **summed across every turn** in the transcript (genuine
     cumulative spend, unlike context occupancy), using per-model API pricing keyed by each line's
     own `message.model` (a session can span models).
  3. Joins both into: chronological task order, concurrent groupings, per-ticket
     duration/peak-context/cost, and total epic duration/cost.

An append-only log (rather than only a final summary) keeps a debuggable trail even through a crash
or an unresumed pause, and lets `report` run mid-flight.

(Decision: [09](../../.scratch/ralph-loop/issues/09-reporting-and-history-log.md))

## Out of scope

- Non-Claude agents (herdr also supports codex/gemini/cursor/etc.).
- Non-local ticket trackers (GitHub/Linear/GitLab) — only `to-tickets`' local-markdown format.
- Modifying herdr itself, even though charting surfaced a real gap (no token/context field exposed).
- A viewer/analyzer UI for `run-log.jsonl`/report data beyond the `report` command's own output.
- Reimplementing gx's ticket parser or herdr glue in blf, or extracting them into a shared module.

## Next step

Hand this spec (plus the decision tickets it links) to a fresh session to break into implementation
tickets — precedent: `.scratch/gx-tickets/` → `.scratch/gx-tickets-impl/`. A `.scratch/ralph-loop-impl/`
epic, built via `to-tickets` from this document, is the expected next move.
