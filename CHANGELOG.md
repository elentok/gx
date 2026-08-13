# Changelog

## v0.28.5 - 2026-08-14

- Fixed a flaky e2e test (`TestIdleWhileWorking_AgentStatusNeverReportsWorking`) that could fail under load when herdr transiently returned `agent_pane_busy`; `Workspace.AgentStart` in `testutil/herdrctl/herdrctl.go` now retries up to 5 times with a 200ms backoff on that error, matching the tolerance already used by `PrependPath` for the same class of race.

## v0.28.4 - 2026-08-13

- Fixed Investigate badge appearing on irrelevant ticket statuses
- Fixed EpicFailureReporter leaking queue-state.json into dev machine during tests, isolated test state
- Fixed default sonnet model config
- Fixed stuck prompt submissions not retyping/retrying and live pane leaks on launch failure

## v0.28.3 - 2026-08-13

- Added a `gx-bump` skill that orchestrates a full release: resolves the bump type, runs tests, drafts and reviews the changelog entry, commits, and tags/pushes via `gx bump --yes`.
- Added `--yes` flag to `gx bump` for non-interactive, scriptable pushes.
- Added a `gx-changelog` skill for drafting `CHANGELOG.md` entries via a cheap sub-agent commit summary followed by review.
- Added a chat notification throttle: per-source mute after 5 identical events within 60s, a global mute around 20 sends/min per transport, and batched delivery for chat-member events (flushes every ~6s, collapses repeats into `×N`), with new `UnmuteTicket`/`Unmute-Reopen` suggested actions to recover.
- Added `gx notify --enable/--disable/--status` to control the notification throttle from the CLI.
- Added `yy`/`yf` yank chords (copy epic id+title / file path) and an Investigate suggested action for any problem-status ticket.
- Fixed the Tickets/Queue tabs' chord overlay not appearing while a multi-key chord (e.g. `gg`/`ee`/`tc`) is in progress.
- Fixed `herdr` agent names for long epic names exceeding its 32-character limit, avoiding `invalid_agent_name` errors by hashing the overflow into the label.
- Fixed `type: code-review` ticket launches to invoke `/gx-code-review` directly instead of going through the blocked Skill-tool path.
- Fixed several park/reopen races: reattach now gates on debounce and background-task completion, parked-ticket evaluation is shared and consistent across call sites, and `herdr`'s `agent_prompt_stalled` is retried like a poll timeout.

## v0.28.2 - 2026-08-13

- **Behavior change on upgrade:** added an `agents` config block (`agents.claude`/`agents.codex`, each with `model` and `effort`). Iterations now run under gx's own built-in defaults (claude sonnet/medium, codex gpt-5.6-sol/medium) instead of inheriting whatever `~/.claude/settings.json` or `~/.codex/config.toml` already say. Set a field to `""` to opt back into the agent CLI's own setting; omit it to keep gx's default.
- Added a suggested-actions menu for `needs-answer` tickets in the Tickets tab.
- Added `--maps` flag to `gx tickets epics`; `gx-local-tracker` now writes wayfinder maps to `map.md`.
- The claude statusline now shows a remote-control indicator.
- Reworked git conflict handling around a tracked conflict lifecycle: reconcile and cherry-pick now recognize live conflicts, sequencer state is corroborated before trusting cached branch status, and parked-on-child land outcomes are distinguished from other errors.
- Landing now goes through a dedicated land-queue worker instead of a `featureMu` lock; `gx-implement` no longer routes landing status through agents.
- Tightened `gx-to-tickets` variant/test-file fan-out split triggers.
- Parallelized and hardened the test suite (goleak, per-test timeouts, env-injection seams, `t.Parallel()` across `ralphloop`, `cmd`, and `ui`) to cut CI time and flakiness.

## v0.28.1 - 2026-08-12

- Fixed the smart-zone finish-up prompt naming the wrong skill (`` `implement` `` instead of `` `gx-implement` ``).
- The `gx-code-review` skill was missing from `//go:embed` in `skills/bundle.go`, so it was never installed by `gx skills install` despite being present in the repo's `skills/` directory. Also fixed its `SKILL.md` frontmatter, whose `description` used an unquoted multi-line plain scalar containing a colon (`` `type: `` `), making the YAML invalid. Added a test that checks every directory under `skills/` is embedded, so a skill going un-embedded fails the build instead of shipping silently.

## v0.28.0 - 2026-08-12

- **Breaking:** the `needs-info` and `needs-attention` statuses are gone, replaced by `needs-answer` (person must supply input) and `needs-repair` (gx hit a fault). Agents report `iteration_status: needs-answer` and exit instead of calling interactive prompts; `status: done` is written by gx only after work lands or is validated commitless by construction.
- Iteration agents can now announce-and-stop: commit what is green, write `## Needs Answer` and `## Handoff` sections explaining the question and state, set `--iteration-status needs-answer`, and exit. gx preserves the branch for resume and releases the worktree and permit; the next agent retires the sections and resumes on the same branch.
- The orchestrator now detects blocked panes (involuntary permission prompts, etc.) within ~15 seconds, parks the ticket as `needs-answer` with the question in the pane, ends the iteration (releasing the permit), and auto-unparks when the pane resumes working — without gx sending any keystrokes at a pane holding a blocking prompt.
- Queue tab parked rows now show the park reason as ellipsised subtext; selecting a parked row auto-scrolls the preview to the park section (highlighted orange for `needs-answer`, red for `needs-repair`). Per-epic header counts parked tickets. The Tickets tab status menu allows changing a ticket's status directly, unparking with a single gesture instead of a text edit.
- Chat notifications now carry epic-truth counts (identical across resumed runs) in a fixed-order counts line (`done · in progress · parked · blocked · ready · total`) instead of run-local fractions, and one message fires per parking write. `EpicStarted` and `EpicFailed` events are new; `NoTicketsFound`/`AlreadyComplete` fold into `EpicStarted` so every epic leaving the queue emits exactly one start message.
- Fork children now implicitly block on their parent's own `status: done`, non-recursive — work lands before descendants start. A parent in a park state holds its child until the parent is unparked; a committed-but-unlanded parent also holds children until the cherry-pick lands.
- `gx tickets set --status` now refuses `--status done`/`needs-answer`/`needs-repair` from iteration agents (detected by the `ralph-loop/*` branch), preventing agents from writing status that only gx can verify. The CLI's settable statuses are `draft`, `open`, `claimed`, `done`; park states are machine-written only.
- Removed the `gx ralph-loop` CLI (was never user-facing) along with the dead `textEventSink`/`ralphloop.Report` types and doc comments claiming an invoke path that doesn't exist. Ralph-loop is now driven by the TUI's loop registry only.

## v0.27.8 - 2026-08-11

- Fixed `.scratch` landing under `.git/.scratch` instead of the repo root when run from a linked worktree of a regular (non-bare) repo. Repo-root resolution used `git rev-parse --git-dir`, which for a linked worktree points at `.git/worktrees/<name>` rather than `.git`, so the worktree was misclassified as belonging to a bare repo and its root resolved to the `.git` directory itself. It now uses `git rev-parse --git-common-dir`, which resolves to the shared `.git` dir regardless of which worktree it's run from.

## v0.27.7 - 2026-08-10

- Fixed smart-zone recovery cancelling the very compaction it asked for. gx interrupted the agent, sent `/compact`, and then sent the finish-up prompt as soon as the pane reported idle — but Claude Code reports idle while it is still compacting, and the queued prompt text cancelled the compaction. The context never shrank, so the next poll saw the same over-budget number and did the whole thing again. The finish-up prompt is now withheld until a compaction boundary in the agent's transcript proves the compaction actually completed. Codex, which has no such signal, is unaffected and behaves exactly as before.
- A compaction that is never confirmed no longer retries forever: after two consecutive unconfirmed recoveries in one iteration gx stops touching the pane and routes the ticket to `needs-attention` with the reason recorded, rather than falling back to sending the finish-up prompt anyway.
- Fixed a spurious second smart-zone breach immediately after a successful compaction. Occupancy stays at its pre-compaction value until the agent next speaks, and gx was breaching on that stale number and interrupting the finish-up work it had just asked for. The breach check now declines to fire on an occupancy reading known to predate the newest compaction, while `gx agent` and the TUI keep showing the last known number and landed tickets keep recording their actual context window.
- The run log now distinguishes the three ways a compaction wait can end — the pane confirmed it, the transcript gate released it, or the wait expired — instead of labelling the gated route as a timeout.

## v0.27.6 - 2026-08-10

- **Breaking:** the `children` frontmatter field is gone. A fork's only structural edge is `parent`, written on the descendant at creation; children are derived from it. A ticket file still carrying `children` is now rejected by the loader rather than silently ignored.
- **Breaking:** the `needs-triage`, `ready-for-agent`, and `ready-for-human` statuses are gone, and `status` is now required — a ticket without one is an error instead of defaulting to open. `ready-for-human` was schedulable, so the orchestrator could re-claim a ticket that had been handed back to a human; `needs-info` covers that case.
- Added `gx tickets migrate` to move an existing tracker to the new shape in one pass: it drops `children`, strips the inherited `blocked_by` tokens fork children used to carry, maps the retired statuses, and validates the whole epic (parent graph included) before writing anything.
- Added a `draft` ticket status: accepted by the schema, `gx tickets set`, and `gx tickets validate`, rendered as its own state, and never offered to an agent (a draft ticket never enters an epic's frontier). `gx tickets add` stamps it on a freshly allocated stub, and `gx tickets set --status open` refuses to promote a ticket whose body is still empty.
- Added a `waiting-for-children` state for a ticket whose own work is done but whose fork subtree isn't. It is computed on every render, never written to a file, and keeps its epic from counting as complete — previously such a ticket rendered as done, so an epic could report complete with unfinished fork work in it.
- A run with no runnable work left now parks and waits for a human instead of exiting, and releases its slot in the epic-level concurrency cap while parked. It resumes automatically when the blocking ticket's status clears, or on Enter on the epic's Queue-tab row; the Queue tab shows the stall reason and whether the stalled iteration can be reattached. An epic with no runnable work and nothing a human could clear is still reported as an error.
- `blocked_by` resolution is now a single predicate: a blocker keeps blocking until its own work is done and every ticket in its fork subtree is likewise finished. Fork children no longer declare a `blocked_by` naming their parent, and code-review tickets get their blockers synthesized at load instead of hand-written.
- `parent` is now a validated graph edge: an epic never hands out a parent naming a ticket absent from the epic, or one closing a cycle — the loader drops such an edge and flags the ticket, `gx tickets validate` reports it, and `gx tickets set --parent` validates and writes under the epic's allocation lock so two concurrent re-parents can't jointly close a cycle.
- Fixed fork-parent lookups disagreeing about number padding and case, so a `parent: "4"` naming a `04-…` ticket no longer resolves in some views and not others.

## v0.27.4 - 2026-08-09

- Removed the deprecated `--split`/`--split-from` flags and legacy split terminology in favor of fork/children across tickets, tests, and skill docs.
- Fixed `blocked_by` resolution to derive descendants from a parent-pointer reverse index (not just `Children`, which `gx tickets add` never backfilled) and to scope the fork-sibling exclusion to inherited blockers only, so a direct blocker's own open child can no longer be silently discounted.
- `gx tickets add --parent` now best-effort backfills the parent's children at fork-creation time, and `gx tickets set --status done` now refuses when the ticket has unresolved `blocked_by` unless `--force`.
- Fixed the tickets-tree triangle-left-of-checkbox indent column colliding with nested children's indent, restoring visible nesting at all depths.

## v0.27.3 - 2026-08-09

- Fixed `gx tickets add --parent <id>` not writing the `parent` frontmatter field into newly created tickets, so split tickets created the documented way silently lost their parent link.
- Fixed Telegram epic-complete notifications always failing with an HTTP 400 error due to unescaped parentheses in the MarkdownV2 message text.
- Fixed the Queue tab silently dropping a checked-but-not-yet-started epic from `MaxConcurrentEpics` auto-promotion whenever gx restarted before its turn came up; it now offers a "Resume?" confirmation on the next load instead.

## v0.27.2 - 2026-08-08

- Added an atomic `gx tickets add <epic>` command (filesystem-locked ID allocation, supports siblings, lettered children, and nested numbering past a lettered parent) and required a `--slug` flag so split-ticket stubs land with their real filename instead of a placeholder.
- Added Slack/Telegram notifications when a ticket lands on `needs-info`, with a retry on failed sends and secrets redacted from the run log.
- Added `gx ensure-code-review` support for a bare epic name, with live shell completion.
- Queue tab: preview panel now focuses on click (enabling scroll/search there), execution tickets stay synced to a live RunScope, replace-queue (`r`) is scoped per-epic with a confirmation modal and now clears done entries too, and ticket rows reserve a fixed-width fold-glyph column.
- Search now matches tickets inside collapsed epics; log-view search input no longer loses focus to the `t` key chord.
- Fixed a deadlock in `fullyDone` when a ticket's split descendants (not just direct siblings) were still pending.
- Fixed split-sibling blocking exclusion to scope correctly to inherited blockers.
- Fixed commitless tickets (research/grilling/code-review, which now default to commitless) not stamping elapsed/context metrics.
- Excluded dot-prefixed directories from ticket loading.
- Made the `gx-to-tickets` skill model-invocable.

## v0.27.1 - 2026-08-08

- Added a `gx merge <branch>` command (deterministic ff-only merge core) and a `gx-merge` skill that wraps it with rebase, conflict resolution, a review pause, and checks before retrying the merge; `gx cleanup`'s merge step now delegates to it instead of duplicating the logic.
- Fixed fast-forward detection to decide ancestry via `git merge-base --is-ancestor` instead of matching git's (locale-dependent) stderr text.
- Fixed stale e2e test fixtures broken by recent refactors (ticket frontmatter, renamed queue key, epic-scoped tab labels).

## v0.27.0 - 2026-08-08

- Added a `gx cleanup` skill/command that scans worktrees and epics for housekeeping issues (stray branches, stale worktrees, missing code-review tickets) and can execute safe fast-forward-only merges and other fixes.
- Added `gx notify`/`gx config test-notifications` commands to send/test notifications directly, and `gx tickets ensure-code-review` to guarantee an epic has a code-review ticket.
- Added Slack notification support alongside the existing Telegram integration, including in-app toasts for epic-complete and iteration-paused events, both durably logged to the run log.
- Added a `?` help modal to the Queue and Tickets tabs.
- Added Queue tab preview-focus toggle and vim-style navigation (`G`/`gg` to jump to list start/end, `b` for preview bottom).
- Added reattach detection for recoverable sessions, gated behind a Detached+Live confirmation instead of auto-navigating.
- Renamed the "implement" queue actions to "Replace queue"/"Add to queue" to better match their behavior.
- Statusline now shows Claude's 5h and weekly rate-limit reset times, and uses a dot separator instead of a vertical bar.
- Commit-info popup now shows the full commit body, wrapped to fill the screen width.
- Manual and self-triggered reloads now preserve scroll position and expand/collapse state instead of resetting them.
- Queue tab now shows attachment status with an "(attached)" label and banner, and full-eligible Queue starts use a dynamic (rescan-on-disk) scope instead of a frozen ticket list.
- Fixed the scratch-merge conflict check being top-level-only, which falsely flagged colliding epic directories even when their ticket files didn't actually collide; it's now recursive and all-or-nothing.
- Fixed a duplicate-launch race (`agent_name_taken`) when a mid-flight ticket split's parent ticket was written to after its children were already claimed.
- Fixed notify partial-send failures being reported twice.
- Fixed the reattach-scan notification not closing when its session resumes.
- Fixed Telegram test-notification bodies not escaping periods for MarkdownV2 compatibility.

## v0.26.0 - 2026-08-07

- Added ticket parent/child hierarchy: a collapsible tree in both the Tickets and Queue tabs, replacing the old `split`/`split_from` fields with `parent`/`children` (legacy keys still parsed) and walking `Parent` instead of `SplitFrom` for run-scope traversal.
- Added a `gx-code-review` skill that runs parallel review subagents against an epic's diff and triages findings into tickets, plus a settings UI for configuring which implement/code-review skill each epic uses.
- Added Claude history browsing tools: `gx claude history browse` (projects/conversations pages), `ctrl+f` grep search over transcripts with copy-to-clipboard, and per-conversation export/resume/yank actions.
- Made the `gx-to-tickets` and `gx-investigate` skills model-invocable so they can be triggered from within code-review agents.
- Added a generic collapsible outline tree component (`ui/tree`), reused from the file tree's list/search/key wiring.
- Ported `gx claude statusline render`/`--install` commands.
- Added scheduler-scan logging to ralph-loop's run-log.jsonl, recording each `claimNext` pass and its ticket dispositions.
- Added `gx skills install --force-all` to force-reinstall every skill.
- Folded ticket-row elapsed/token metrics onto the title line instead of a separate second line.
- Changed `WorktreeDir` to default to `<Root>/.worktrees` for standard (non-bare) clones instead of `Root` itself.
- Fixed the Queue tab's agent picker to render as a centered overlay instead of replacing the whole screen.
- Fixed the commit-info popup to have a border matching other panels, with an `i` binding to open it from the Log tab's commit list.
- Fixed `/compact` verification to cross-check the confirmation count before trusting an immediate success.
- Fixed Queue replace to exclude already-done tickets from the selection.
- Fixed expanded done epics re-collapsing after auto-refresh.
- Fixed `recoverSmartZoneBreach`'s double-fallthrough logic and deduped worktree exclude-file handling between `worktree.go` and `doctor.go`.
- Fixed ralph-loop's done-ticket verification to scope to the current run instead of the entire epic, and added e2e tests covering mid-run ticket splits for regular and code-review tickets.
- Fixed grilling/prototype tickets to be treated as commitless by type, skipping the needs-attention flag.
- Smoothed the ticket-progress spinner by draining before repeating, removing a visible jump.

## v0.25.2 - 2026-08-06

- Added a `g n` notification history modal: browse captured shell-notification events, filter with `/`-search, and export the visible entries to `~/.cache/gx/{timestamp}-{repo}-{worktree}.md`.
- Redesigned the Queue panel header: the title now always encodes run state (not-started/running/paused/completed) with a matching spinner glyph, replacing the old fixed "Queue" label and always-present banner row.
- Added click-to-focus for the Tickets preview and Log detail panes, so clicking inside either pane focuses it and routes wheel scroll there instead of the sidebar/list.
- Added edit-file chords (`e e`/`e s`/`e v`/`e t`) to the Queue tab, matching the Tickets tab.
- Refreshed the ticket status icon set to the FA outline family and replaced the ticket-progress spinner with a shared circle-slice pie-fill glyph.
- Config, cache, and skill-manifest paths now resolve directly to `~/.config/gx`/`~/.cache/gx` instead of going through `os.UserConfigDir()` (which differs on macOS/XDG), with a one-time migration of existing state and a startup warning if migration fails.
- Fixed the Queue header staying stuck on "idle" when the queue was globally paused, and extracted a shared run-state classifier.
- Fixed self-rearming ticks and window-resize updates being dropped while the `g n` notification-history modal was open, which had stalled Queue tab polling and left the modal stale after a resize.

## v0.25.1 - 2026-08-06

- Fixed `RunScope.Contains` to walk the `SplitFrom` chain, so a mid-run split's descendant tickets are recognized in scope without a fresh ralph-loop invocation.
- Added a once-per-process restart-recovery scan that surfaces recoverable epics as progress notifications on the Tickets tab's first activation.
- Fixed a `/compact` prompt-submission race where the finish-up prompt could land concatenated with a not-yet-submitted `/compact`.
- Fixed `confirmCompactSubmittedWithRetry` burning its retry budget by pacing on sleep instead of agent-status polling.
- Mirrored the checkbox prefix into live ticket rows and removed the unused epic-gutter marker code.

## v0.25.0 - 2026-08-05

- Decoupled the Tickets tab's checked state from the Queue tab's queue state, so checking tickets and queuing them are independent actions that stay in sync end-to-end.
- Added a `tc` chord to hide completed/done tickets on both the Tickets and Queue tabs.
- Added `ctrl+d`/`ctrl+u` half-page paging to the Tickets tab sidebar.
- Queue tab rows now follow dependency-wave order, and epic completion duration renders consistently on both tabs.
- Added epic `started_at`/`completed_at` timing and per-ticket `StartedAt` for live elapsed-time display.
- Moved the commit header into a toggleable popup instead of always showing inline.
- Fixed the Queue tab's `tc` chord swallowing the following key when it didn't match, and fixed a data-loss bug where an empty `CheckOrder` field was silently discarded as a corrupt/legacy file.
- Fixed a doubled indent prefix on live ticket rows.
- Preserved manual epic-collapse state across ticket reloads.
- ralph-loop now serializes git worktree add/remove across iterations to avoid races.
- Lowered the `gx-to-tickets` split threshold to 70K and added an estimated context window to the quiz step.

## v0.24.4 - 2026-08-05

- Added Telegram notifications for ralph-loop iteration and epic-completion events, wired to real ticket counts and elapsed-time metrics.
- Redesigned the Queue tab: scrollbars, search, mouse-wheel scroll, row-click selection, direct-replace on `i`, and single-ticket cascade delete.
- Added mouse click support and width capping to confirmation dialogs across tabs.
- Improved compact-recovery polling and added a shared ticket preview pane plus explore/implement split policy.
- Ticket lists now auto-refresh on status change and split; tickets track multiple agent sessions via a new `session_ids` frontmatter field.
- Fixed the Queue tab scrollbar rendering past the panel width and a nil-map panic in the queue view before load.
- Fixed a blocker-family bug where bare-number blockers wrongly matched split siblings.

## v0.24.3 - 2026-08-04

- Added `gx skills install`/`uninstall`, embedding gx's canonical skill bundle (including new `gx-to-tickets`, `gx-tdd`, `gx-implement`, and `gx-resolving-merge-conflicts` skills) and installing managed copies into Claude's and Codex's skill roots.
- Added `gx skills install --dev`, symlinking a checkout's skill files into both agents' discovery roots so source edits show up immediately.
- Added `gx agent context-window`, a provider-neutral command that reports the active Claude or Codex session's current context occupancy.
- Ralph loop now defaults to the `gx-implement`/`gx-resolving-merge-conflicts` skills instead of `/implement`, with a raised default smart-zone ceiling.
- Fixed iteration worktrees being nested inside their epic's own worktree, which could make git operations see live iterations as foreign content.
- Improved ralph-loop durability: context exhaustion, quota polling/backfill, launch preflight, reattach guardrails, and post-claim launch-failure recovery, with new Claude/Codex recovery end-to-end tests.

## v0.24.1 - 2026-08-04

- Added a durable execution queue for selecting tickets or epics, automatically including blockers and split children, and running scoped work across multiple epics concurrently while preserving progress across restarts.
- Added queue pause/resume controls, dependency-aware execution waves, live progress and context metrics, completion summaries, and recovery when returning to the Tickets or Queue tabs.
- Execution queue concurrency is now configurable with `execution-queue.max-concurrent-tickets-per-epic` and `execution-queue.max-concurrent-epics`.
- Tickets now use typed YAML frontmatter with validation and atomic sparse updates through the new `gx tickets validate`, `gx tickets set`, and `gx tickets schema` commands; legacy bold-line parsing has been retired.
- Ticket rows and previews now show live or landed elapsed time, context-window usage, compaction counts, and cherry-pick landing status.
- Added a Claude/Codex agent picker when launching ticket execution, and improved Codex session attribution so live and landed metrics are recorded correctly.
- Ralph loop now records elapsed and context metrics in ticket frontmatter and commit trailers, scopes worktrees and live state per epic, and supports running selected ticket subsets.
- Fixed slow `/compact` operations being interrupted or repeatedly submitted while retaining safe nudging for genuinely stalled prompts.
- Fixed isolated iteration failures aborting the entire run, transient commits-ahead errors stranding completed work, externally landed tickets being misclassified, and failure reasons disappearing from ticket files.
- Fixed concurrent epic state collisions, stale queue state leaking across worktrees, running-epic state being lost after tab switches, and broken live-row metrics after a rebase.
- Added `gopkg.in/yaml.v3` v3.0.1.

## v0.24.0 - 2026-08-02

- Added ralph-loop integration into the Tickets tab: live progress and phase highlighting for running tickets, pause/resume controls with confirm modals, an epic-completion banner, and a quit warning while a loop is running — replacing the standalone `gx ralph-loop` command.
- Ralph loop now scopes iteration branches and ticket trailers per epic, auto-recovers smart-zone breaches and unlanded commits on orphaned claims instead of blocking, and surfaces run errors in the TUI.
- Tickets now sort by plan order instead of rendered-status group, preserve zero-padding and letter suffixes in display numbers, detect superseded status, and support a lettered `Blocked-by` token naming one split sibling.
- Fixed a resume/wait race in ralph-loop, debounced finish detection to avoid orphaning late commits, and improved reconciliation of done tickets across rebased commits.

## v0.23.3 - 2026-08-01

- Ralph loop startup recovery now finishes interrupted cleanup for completed tickets, including removing leftover iteration tabs, worktrees, and branches.
- Ralph loop now marks completed tickets as `needs-attention` when their commits are missing from the feature branch and cannot be recovered, records the reason, and exits cleanly.
- Tickets now recognize annotated statuses such as `resolved (dup of #12)` and use an even 50:50 sidebar/preview split in vertical layouts.

## v0.23.2 - 2026-08-01

- Added `gx ralph-loop` to execute `.scratch` epic tickets with Claude or Codex, including dependency-aware scheduling, isolated iteration worktrees, configurable parallelism, automatic cherry-picking, and agent-assisted conflict resolution.
- Ralph loop now guards agent context usage, pauses and resumes safely, recovers from Claude rate limits and Codex quota exhaustion, and marks stalled or zero-commit Codex work as needing attention.
- Added crash and startup recovery that reattaches live iterations, reopens abandoned tickets, verifies completed work remains on the feature branch, and restores recoverable missing commits.
- Added `gx ralph-loop report` and append-only lifecycle logs with task order, concurrency, duration, context usage, and Claude cost; Codex runs provide equivalent usage reporting without cost.
- Fresh iteration worktrees now install npm, pnpm, Yarn, Poetry, or uv dependencies before launching agents.
- Tickets now support suffixed numbers such as `10a` and a vertical split layout.
- Fixed Ralph loop worktree and tab topology, `.scratch` path resolution, and smart-zone Ctrl-C interruption.

## v0.23.1 - 2026-07-30

- Status: fixed the filetree panel title overflowing horizontally when the branch name was long, and dropped the "vs {base-ref}" suffix from the title.

## v0.23.0 - 2026-07-23

- Added a Tickets tab (`gx tickets` / `tk`) for browsing `.scratch/` epics and tickets, with status grouping, counts, ticket numbers, blocker indicators, and resilient metadata parsing.
- Tickets: added Markdown previews rendered with Glamour, preview focus/scrolling, in-preview search highlighting, collapse/expand navigation, manual refresh, and edit shortcuts.
- Tickets: added `--all` cross-worktree aggregation with worktree labels.
- Updated dependencies, including Glamour, Go 1.25.8, Lipgloss 2.0.4, and `x/sys` 0.45.0.

## v0.22.3 - 2026-07-21

- Terminal: herdr splits/tabs now use the documented `pane split`/`pane run` flow instead of `agent start`, fixing launches that relied on undocumented behavior.

## v0.22.2 - 2026-07-21

- PRs tab: Open PRs are now split into Actionable and Non-actionable sections, each with a count and empty state, so PRs needing your attention stand out.
- PRs tab: Open and Closed PR sections now scroll together as a single unified viewport with a scrollbar, instead of the Closed section always rendering in full below.

## v0.22.1 - 2026-07-21

- Added a PR comments popup: press `c` on any open PR to view issue comments and reviews with timestamps.
- Closed PRs are now selectable and navigable in the same list as open PRs.
- Added facet status text labels (CI, approval, mergeable states now show "passing"/"failing"/"checking", etc.).
- In `--all` mode, repo name is now shown before the PR number for clarity.
- Improved performance: open and closed PRs now fetch concurrently, and `--all` mode uses a single GraphQL query instead of per-repo calls.
- Fixed scroll behavior when navigating from the open to the closed PRs section.

## v0.22.0 - 2026-07-21

- Added a PRs tab: shows your outgoing open PRs with CI/approval/mergeable/comment-count facets, an actionable-first sort, and a recently-closed section (last 2 weeks). Supports manual refresh (R / m-r), auto-refresh on tab switch, and an `--all` flag / `a` key to scope across every repo instead of just the current one.
- Fixed a flaky `TestStageE2E_PushActionWithConfirm` test.

## v0.21.2 - 2026-07-20

- Fixed a herdr split/commit bug where a leftover pane from a prior launch (e.g. a failed `git commit` kept open by `gx run`) collided with the next launch's fixed agent name, causing `agent name gx is already used` errors. Each herdr agent launch now gets a unique name.

## v0.21.1 - 2026-07-20

- Diff view: added a scrollbar indicator (status, stage, and commit views) when the diff exceeds the visible area.
- Fixed a filetree scrollbar rendering bug.

## v0.21.0 - 2026-07-19

- Terminal: added support for herdr — split/tab launching (hsplit, vsplit, tab) and worktree session workspace focus/create.

## v0.20.2 - 2026-07-18

- Filetree: added a scrollbar indicator when content exceeds the visible area.
- Panel: removed the spacing between the header and contents for a tighter layout.
- Status: moved the branch summary into the panel title and the worktree line to the top of the filetree.
- Fixed filetree scroll-clamp/render height mismatches that caused the selected row to scroll out of the visible area without a highlight.

## v0.20.1 - 2026-07-17

- Fixed key handling so a page's pending chord takes priority over the shell chord.

## v0.20.0 - 2026-07-18

- Persistent layout panels (status, log, commit, stashlist, worktrees) now render without borders —
  background/header shading replaces frame glyphs, with borders kept only for modals/popups.

## v0.19.12 - 2026-07-15

- Align log and commit views commit rendering

## v0.19.11 - 2026-07-15

- Log: commit rows now render across two lines — subject (with graph and push/pull state) on top, hash/date/author/decoration badges indented below. Decoration badges are back as condensed boxed pills. Selection and flash highlighting span both lines.

## v0.19.10 - 2026-07-11

- Commit view and log: decoration badges (branch/tag) now render as plain colored text instead of a dark-background pill, joined to the subject by a delicate " · " separator. This avoids the subject shifting column depending on whether a row has decorations.
- Log: the working-tree pseudo-row now always reads "working tree: <detail>" instead of relying on fixed-width padding that only lined up in wide panels.

## v0.19.9 - 2026-07-11

- Commit view: the header now shows branch/tag decorations next to the subject/author line, styled like the log view's badges (dark background, colored text). Decorations wrap onto extra header lines when they don't fit.
- Log: rows in narrow panels now render in a condensed style — relative dates drop the " ago" suffix (e.g. "2h" instead of "2h ago"), the gap before decoration badges narrows to one space, and multiple decorations merge into a single badge group with one shared background and per-decoration text colors.

## v0.19.8 - 2026-07-11

- Status: directories in the file tree can now be discarded directly — discarding a directory undoes tracked changes and deletes untracked files for everything inside it.

## v0.19.7 - 2026-06-29

- Bump: fixed version bump failing when a non-semver tag exists between HEAD and the last version tag.
- AI keys: renamed yank-for-AI binding `ya` → `ay`, and ask-AI binding `cm` → `ai`.
- Takeover apps (editor, comment editor, lazygit log): success no longer shows a stale "opening…/closed" toast — the screen refresh on return is feedback enough. Git commit still reports via a "committed" success notification.

## v0.19.6 - 2026-06-17

- Log and status tabs now show the current worktree name in the top-right of the panel frame (next to the ref), prefixed with a worktree icon when nerd fonts are enabled. The log tab's left title is now simply "Log".

## v0.19.5 - 2027-06-16

- Running `gx` with no args from a bare repo root (the `.bare`-trick layout, before `cd`-ing into a worktree) now opens the worktree UI instead of erroring with "must be run from a regular repo or linked worktree".

## v0.19.4 - 2026-06-10

- Pull/push: don't show notification if aborted
- Push: allow aborting

## v0.19.3 - 2026-06-09

- Help: the help page (`?`) got a substantial overhaul — bindings now render across multiple columns to fit more on screen, with a scrollbar when the content overflows. Added filtering: start typing to filter the visible bindings by key or description. Chord bindings are displayed more cleanly, and duplicate/merged key bindings are consolidated.

## v0.19.2 - 2026-06-08

- Log: fixed amend (`A`) stalling when triggered with the commit detail panel focused — the modal got stuck after creating the fixup commit and never ran the autosquash rebase. The log now keeps forwarding the detail panel's async modal messages, so the amend completes.
- Splits: split terminology now matches vim. "Horizontal split" / `es` opens a **stacked** (top/bottom) pane like vim `:split`; "vertical split" / `ev` opens a **side-by-side** pane like vim `:vsplit`. This swaps the behavior of the `es`/`ev` edit-in-split chords and the worktree terminal menu's `h`/`v` actions; `gx term --right`/`--below` are unchanged (they were already named by visual outcome).

## v0.19.1 - 2026-06-08

- Commit/Log/Stash: inline image diffs now render in the commit detail view too, so image changes are shown directly (via the kitty graphics protocol) when viewing a commit in the log and stash tabs, not just in the status diff panel.

## v0.19.0 - 2026-06-08

- Status: added inline image diffs — image files in the diff panel are now rendered directly via the kitty graphics protocol on supported terminals, instead of a binary-file summary. Toggle with the new `image-diffs` config option (enabled by default).

## v0.18.2 - 2026-06-06

- Prevent tab switching when a modal is open

## v0.18.1 - 2026-06-06

- Internal: normalized log and stash tabs onto a shared internal shape — each tab now has an unexported `listPanel` sub-model (row rendering, list navigation, `splitview.ListPanel`) separate from the page orchestrator. Both tabs expose `NewModel` as the sole public constructor.

## v0.18.0 - 2026-06-04

- Stash: added a **Stash tab** (the fourth tab, reachable with `4`, `g S`, or `,` / `.`) that lists the repo's stashes in a split view — the stash list on one side and the selected stash's diff on the other. Apply (`a`), pop (`p`), drop (`d`), or create a new stash (`s`) directly from the list; `enter` / `l` focuses the diff panel, and `t o` toggles the split orientation (which also auto-stacks on narrow terminals).
- CLI: added `gx stash` to open the stash UI directly.
- Log: the log tab now renders in the same shared split view (commit list + commit detail), with a pseudo worktree-status row pinned at the top so uncommitted changes are visible alongside history.
- UI: switching tabs no longer clears the screen or flickers. Cached tabs reload only when the repo actually changed since they were last loaded (epoch-based invalidation), instead of re-shelling git on every activation; mutating ops (commit, stash, push, …) signal the change so other tabs refresh on their next visit. Manual refresh (`R`) is unchanged.
- Log: fixed commit-detail focus routing for `h` / `l`, split-panel focus handling, and collapsed-commit frame dimming; realigned the pseudo status line and log columns.

## v0.17.7 - 2026-06-03

- CLI: `gx init` and `gx edit-config` are replaced by a `gx config` subcommand group — `gx config edit` opens the config in `$EDITOR` (creating it if missing), `gx config show` prints the effective merged config as JSON, and `gx config defaults` prints the built-in defaults as JSON.
- Config: fixed log config merging — `log.important-refs` and `log.hide-refs` from the user config now merge correctly with defaults instead of replacing the entire log section.
- Fix: relative timestamps now include the year for dates older than one year.

## v0.17.6 - 2026-06-02

- Status: added `Sa` / `Ss` stash shortcuts. `Sa` stashes all tracked changes (staged + unstaged); `Ss` stashes only staged changes, leaving unstaged modifications in place. Both prompt for an optional stash name before running.

## v0.17.5 - 2026-06-02

- CLI: added `gx term` to launch a command (or `$SHELL`) into a tmux/kitty split, tab, or in place. Directions are named by visual outcome (`--right`/`--below` default/`--tab`/`--here`) so the same flag lays out identically on tmux and kitty; `--cwd` sets the working directory. On a plain terminal (or kitty without remote control) it runs in place, so the same invocation works everywhere — handy for opening things from neovim into a split.
- Splits/tabs: fixed the kitty split direction — side-by-side and stacked splits were swapped relative to tmux, so they now produce matching layouts on both terminals (affects the worktree terminal menu and `gx term`).

## v0.17.4 - 2026-06-02

- Log: added `gx log -f/--file <path>` to open the log pre-filtered to a file (path is taken relative to your current directory). Follows renames, so pre-rename history is included.
- Log: file-filtered logs now follow renames everywhere — both `gx log -f` and the status `gh` mapping show a file's history from before it was renamed.
- CLI: shell completion is now available via `gx completion <bash|zsh|fish|powershell>`.
- Splits/tabs: commands run in a split or tab (commit, rebase, editing a file) now keep their pane open when they fail, showing the exit code, the command that ran, and a `press Enter to close…` prompt — so you can read the error instead of the pane vanishing. Successful commands close immediately as before.

## v0.17.3 - 2026-05-31

- Log: improve columns

## v0.17.2 - 2026-05-31

- Worktrees: kitty session names now use full repo and worktree names, with optional exact aliases for long names.

## v0.17.1 - 2026-05-31

- Status: fix indentation issue

## v0.17.0 - 2026-05-30

- Search box now renders inline inside the diff view and filetree (no longer a floating overlay)
- Search: pressing `/` in results mode reopens the search box with the current query pre-filled
- Status: confirming a search syncs the query and highlights to the inactive pane, so both staged and unstaged sections show counters after confirm
- Filetree: fix search jump bug

## v0.16.2 - 2026-05-27

- Commit: fix diff scrolling — `j`/`k` now scroll through long hunks before jumping to the next/previous one
- Commit + Status: scroll one extra line past the hunk boundary before jumping, so the end of a hunk is visible before moving on
- Commit: show scroll percentage in the diff panel title
- Commit + Status: search counter (`⌕ N/M`) now appears in the commit diff title when searching

## v0.16.1 - 2026-05-26

- All views: add `g p` binding to open the GitHub PR for the current context in the browser
- Commit + Status: add `e`-prefix chords for editor split variants (`ee`, `es`, `ev`, `et`)
- Commit: add `[`/`]` bindings to decrease/increase diff context lines
- Commit: show diff context count on the frame instead of as a notification
- Worktrees: fix delete progress mode — wait for exit before quitting
- Tabs: remove extra padding

## v0.16.0 - 2026-05-25

- Log + Commit: add `yh`, `ys`, `ym` to yank commit hash, subject, and message
- Log: show worktree root in panel title
- Log: `q`/`esc` from a worktree's log view returns to the worktrees list
- Log: update ref badge colors
- Massive navigation refactor

## v0.15.7 - 2026-05-19

- Worktrees: add bulk-delete with multi-select and progress modal
- Push: fix PR URL detection
- Push: clean state on startup

## v0.15.6 - 2026-05-19

- Log: add `important-refs` config to highlight specific refs with custom colors and control their sort order
- Log: add `hide-refs` config to hide specific refs by regex pattern

## v0.15.5 - 2026-05-19

- Log: move the status icon next to the commit subject

## v0.15.4 - 2026-05-19

- Log: color commits based on their status
- Log: add filtering by file path and line range
- Log: interactive rebase window no longer closes automatically after rebase completes
- Pull: confirm before stashing when there are uncommitted changes
- Worktrees: use the shared pull UI for the pull command
- Status: change fullscreen key mapping back to `f`
- Change reword key mapping from `cr` to `rw`
- Consolidate keybinding and help management across all UI modules
- Fix chord hints display issue

## v0.15.3 - 2026-05-17

- Log: add `ri` keybinding to launch `git rebase -i` from the selected commit; if there are unstaged changes, a modal confirms stashing first; after the rebase completes, a second modal confirms popping the stash
- Fix log view not reloading on focus (missing `ReportFocus = true`); as a side-effect, `FocusMsg` now works in log, enabling the stash-pop prompt after an interactive rebase in a terminal split
- Extract `ui.NewMainView` shared helper that sets `AltScreen`, `MouseMode`, and `ReportFocus` — used by all top-level page views (log, status, commit, worktrees) to prevent flags from being missed
- Consolidate all view-specific Settings structs into a single `ui.Settings` struct
- Fix double force-push confirmation in `gx push`
- `make install` now embeds the version string in the binary via `-ldflags`
- Parallelize git and UI tests with `t.Parallel()`

- Fix commit view header scrolling broken when expanded body fills the viewport

## v0.15.2 - 2026-05-16

- Commit view: top panel now sizes to fit its content, capped at 50% of screen height; `(b to expand)` hint only appears when a commit body actually exists
- Bump flow: remove the redundant "Push to origin?" confirmation inside the bump modal — the push modal's own confirmation is used instead; push confirm prompt now highlights the branch name in orange and remote in teal, and shows a different message when a tag will also be pushed
- Status view: remove stage/unstage notifications

## v0.15.1 - 2026-05-16

- Fix bump push flow not pushing the tag — the push modal now runs `git push <remote> <tag>` as an additional step after a successful branch push

## v0.15.0 - 2026-05-16

- Add `B` keybinding in `gx status` and `gx log` to bump the version — shows a picker (patch/minor/major), creates an annotated tag, then optionally triggers the push flow
- Add `R` keybinding in `gx log` for refresh (previously only available via `m r` chord); remove the `m r` alias
- Add refresh notifications: synchronous views (status, commit) show "refreshed"; background views (worktrees, log) show a spinner while loading then "refreshed" on completion
- Change `ya` keybinding title and output format to "yank for AI agent" — wraps the diff in a `\`\`\`diff`code block matching the`cm` comment format
- Notification system: migrate all views to a shared `ui/notify` overlay model with Info/Success/Warning/Error/Progress kinds

## v0.14.6

- Add `p`/`P` keybindings in `gx log` to pull and push the current branch (same flow as `gx status`, including credential prompting, stash/pop for dirty worktrees, and divergence handling)
- Add `ctrl+d`/`ctrl+u` vim-style co-scroll in `gx commit`, `gx status`, and `gx log`
- Wire mouse scroll in `gx commit`, `gx status`, and `gx log`
- Add `cr` chord in reword view to open `$EDITOR` for editing the commit message
- Flash and re-focus the log entry after returning from amend or reword
- Fix wide-character rendering causing a CPU spike or skipped characters (bubbletea v2.0.6 upstream fix)

## v0.14.5

- Add `cm` keybinding in `gx status` and `gx commit` to open `$EDITOR` and write a comment on the currently selected diff hunk
- Fix help modal scroll — scrolling past the last visible line no longer gets stuck
- Add `A` keybinding in log and commit views to amend a specific commit with currently staged changes

## v0.14.4

- Make the panel frame color darker

## v0.14.3

- Introduce a `keybindings.Manager` to centralize key binding definitions, chord dispatch, and help-text generation across `gx status`, filetree, and diff area — replacing scattered per-component key structs and manual chord tracking
- Extract keybinding definitions for filetree and diffarea into dedicated `model_keys.go` files; generate help content from binding metadata instead of hand-maintained lists
- Fix `q` in `gx status` not quitting — `bindingQuit` was registered in the keybindings manager but had no dispatch case

## v0.14.2

- Align filetree left/right behavior across `gx status` and `gx commit`: `h/left` now collapses expanded folders first (then moves to parent), `h/left` on files moves to parent folder, and `l/right` on expanded folders moves to first child
- Remove status-specific filetree key interception so status delegates folder/file navigation to `ui/filetree` consistently
- Refresh `gx log` when the log page is activated via app navigation and when terminal focus returns, so new on-disk commits appear without manual reload

## v0.14.1

- Refactor diff interaction ownership so `gx status` and `gx commit` route navigation/search/yank/viewport behavior through `ui/diffview.Model` methods instead of package-level helper wiring
- Extract a dedicated `ui/status/diffarea` model to clarify staged/unstaged orchestration boundaries and reduce status-page coupling
- Rename diff state internals from buffer-oriented naming to `DiffData` and prune legacy status alias/host glue files
- Reduce `ui/diffview` exported helper surface by making internal navigation/search/runtime helpers private once call sites were migrated

## v0.14.0

- Redesign `gx status` with a focused-section layout: one expanded diff section plus always-visible collapsed strips for `Unstaged` and `Staged`
- Simplify status navigation and focus behavior: `h/l` now moves between filetree and diff, `Tab` switches diff sections, and section choice stays stable across reloads and file changes
- Improve status visual identity and focus clarity: filetree uses blue focus styling, diff sections keep stable unstaged/staged colors, active pane titles are bold, and filetree row focus is more explicit
- Keep stage/unstage context in place by removing auto-jumps when a section becomes empty, while keeping destination flash feedback
- Show selected file paths directly in the expanded diff section title (`Unstaged: …` / `Staged: …`), including renamed-file format (`old -> new`)
- Fix footer composition with app tabs so right-side hints remain readable: preserve the hint tail (context/mode/help), handle padded footer lines correctly, and use Unicode ellipsis (`…`) for truncation
- Update `gx log` commit highlighting so diverged local-only commits are styled distinctly instead of sharing normal local-only green

## v0.13.6

- Complete the status nested-model cleanup: focused-child key routing, per-pane search ownership, and removal of legacy status search-scope adapters
- Replace filetree sync bridge code with explicit status/filetree reconciliation helpers and cleaner state ownership
- Simplify status/filetree integration by caching filetree rows alongside status entries and removing the status-to-filetree conversion helper
- Prune dead status/diffview/filetree API surface used only during migration

## v0.13.5

- Fix status diff `Tab` cycle order to `sidebar -> unstaged -> staged -> sidebar`
- Preserve status unified diff colorization state while navigating hunks, preventing temporary raw gutter padding from reappearing

## v0.13.4

- Log view now reloads commits on open and supports `R` to reload manually
- Fix commit sidebar min width (25 instead of 45) so short file lists don't waste space

## v0.13.3

- Add left padding to unified diff while async delta colorization is pending, reducing flicker when switching files
- Fix syntax highlighting disappearing after staging a hunk
- Fix unified diff not reflowing to the new width when toggling fullscreen in the status view

## v0.13.2

- Normalize keymappings (partially, there's still work to do)
- Show chort keys overlay (like neovim's whichkey)
- Code cleanup

## v0.13.1

- Normalize commit message newlines

## v0.13.0

- Changed `gx` startup to open the status view by default (`gx worktrees` / `gx wt` still opens the worktree UI)
- Added a focused commit-view header with scrollable metadata, commit-body yanking, and cleaner subject/author rendering
- Added log/commit navigation refinements, including tag-jump chords in `gx log`, commit selection restoration when backing out of commit view, and tab-aware log history
- Kept the shared diff-explorer extraction moving forward so commit and status views continue to reuse the same rendering/navigation core

## v0.12.11

- Added a real `gx show [hash-or-ref]` entrypoint that opens a new commit view with commit metadata, ref badges, and body collapse/expand
- Added commit-ref plumbing in the app shell so commit routes resolve directly instead of using a placeholder page
- Continued extracting the shared diff-explorer core out of `gx status`, with host-style helpers and selection adapters to prepare for the future commit diff explorer

## v0.12.10

- Added app-level tab navigation across `gx wt`, `gx log`, and `gx status`, including persistent tab state, `gw` / `gl` / `gs` routing, and proper push/replace back-stack behavior for drill-down screens
- Added a new `gx log` view with commit history, ref badges, pseudo-row navigation into working-tree status, inline search with persistent highlights plus `n` / `N` result jumping, and support for `gx log <ref>` rooted at an explicit commitish
- Unified selected-row highlighting across log and worktrees with a neutral surface background that preserves existing foreground colors and search/status styling

## v0.12.9

- Refined `gx status` unified diff rendering so changed rows no longer show literal `+` / `-` markers, making the interactive unified view read closer to the side-by-side mode while keeping hunk/line staging behavior intact
- Extended added and removed row backgrounds to the full diff pane width for a cleaner, easier-to-scan unified diff

## v0.12.8

- `gx wt` text-input overlays now accept pasted text again for flows like new worktree, rename, clone, search, and credential prompts
- Added `name-aliases` config for kitty session naming so exact repo and worktree names can be replaced before the usual dash-segment compression runs

## v0.12.7

- Fixed release and local linker version stamping after the Go module path change so `gx version` now reports the injected tag/build version instead of falling back to Go's VCS build info and showing unexpected `+dirty` suffixes

## v0.12.6

- `gx status` now sizes the `Commits` pane from its rendered content instead of using a fixed split, while keeping both the status list and branch-commit history visible within min/max height bounds
- Fixed kitty session naming for `.bare`-style repos so new sessions use the outer repo directory name instead of generating `bre-...` prefixes from `.bare`

## v0.12.5

- `gx wt` now has a unified "open in terminal" menu: `o` opens the selected worktree and `N` creates a new worktree and opens it immediately; the menu offers session, hsplit, vsplit, and tab actions for tmux and kitty remote control, and shows a clear message when kitty remote control is unavailable
- `gx wt` and `gx status` output/lazygit chords now use a `g` prefix: `gg` jumps to top, `go` shows command output, and `gl` opens the lazygit log; this frees `o` for the new worktree open flow
- Kitty session names now use a vowel-stripped `repo-worktree` form for brevity, and session files are auto-created as `~/.local/share/kitty/sessions/<name>.kitty-session`
- `gx status` now shows a read-only `Commits` frame under the status tree with the branch history since the remote mainline (`origin/<default>`, `origin/main`, or `origin/master`); commits show wrapped subjects plus relative time and short hash, and are color-coded for shared, local-only, and remote-only/diverged commits

## v0.12.4

- `gx wt` and `gx status` search now appears as a framed bottom-center overlay instead of replacing the footer line; match count (`2/5`) and `no matches` appear on the right side of the modal border
- `gx wt` text-input overlays (rename, clone, new, search) are now wider (50 columns, capped at 80% of window width)
- Added `input-modal-bottom` config option to control the vertical position of text-input overlays: accepts an integer (fixed lines from bottom), a percentage string like `"20%"`, or `"center"`; `gx init` now writes `$schema` pointing to the published JSON schema
- Added `docs/config-schema.json` — reference it with `"$schema": "https://raw.githubusercontent.com/elentok/gx/main/docs/config-schema.json"` for editor autocompletion

## v0.12.3

- `gx wt` now opens keyboard help in a centered overlay like `gx status`, and both `gx wt` and `gx status` footers now show a compact `? help` prompt instead of inline keymaps
- Restyled `gx wt` help to match the brighter `gx status` help colors

## v0.12.2

- `gx status` now compresses single-child directory chains in the sidebar, so paths like `keyboards/iris/keymaps/` render as a single directory row instead of three nested rows

## v0.12.1

- Added an optional path argument to `gx status` / `gx s`; relative and absolute file paths now preselect the matching file in the status sidebar without jumping into diff focus
- Changed the Go module path to `github.com/elentok/gx` so `go install github.com/elentok/gx@latest` works correctly

## v0.12.0

- Added a shared UI design-system foundation across `gx status`, `gx wt`, and CLI flows: common theme colors, semantic icons, shared frame/overlay primitives, and reusable feedback/key-hint helpers
- Standardized menus, confirms, keybinding hints, and status messaging so CLI and TUI interactions now feel much more consistent across screens
- Migrated `gx status` onto structured `bubbles/help` and `key.Binding` driven help rendering
- Tightened final polish across remaining surfaces, including shared modal hint language, cleaner search/footer hints, and a more consistent interactive `gx bump` picker
- Added design-system research/spec docs under `docs/design-system/` and an AI-facing usage guide in `.ai/design-system.md` to keep future UI work aligned with the shared system

## v0.11.7

- `gx status` diff view now identifies symlinks: shows the target in a summary line (`symlink -> target`, `symlink: old -> new`, `symlink (target) -> regular file`, etc.) and labels the section header with `[symlink]`, `[regular -> symlink]`, or `[symlink -> regular]`; sidebar uses a dedicated symlink icon
- `gx stashify` now prints styled blue badge labels with nerd font icons before each step; icons are gated on the `use-nerdfont-icons` config option

Debugging related:

- `gx status` now shows the detected terminal (tmux/kitty) in the status bar
  (for debugging).
- `gx doctor` now shows the values of terminal-detection related env variables
  (`TMUX`, `KITTY_WINDOW_ID`, and `KITTY_LISTEN_ON`).
- `gx doctor` supports the `--pause` flag to wait for Enter before exiting

## v0.11.6

- `gx status` now shows branch sync relative to the current branch's upstream ref instead of always comparing against the repo's default main remote branch
- Restored `g` as jump-to-top in `gx status` and `gx wt`, and moved output/log actions to chords: `oo` for command output and `ol` for lazygit log
- Added `ot` in `gx wt` to open a tmux session in the selected worktree directory

## v0.11.5

- Fixed Ubuntu CI failures in the PTY-backed Git runner by treating Linux `/dev/ptmx` `EIO` shutdown reads as benign EOF-style conditions

## v0.11.4

- `gx status` and `gx wt` now keep SSH/HTTPS credential prompts inside the TUI by detecting Git/SSH input requests and opening an in-app input modal instead of suspending to the terminal
- User-initiated TUI network actions now run through a PTY-backed Git runner so passphrases, usernames, and passwords can be submitted directly from the app
- Fixed the PTY credential flow so handled passphrase prompts are not rediscovered and resubmitted incorrectly, which could cause repeated SSH key prompts and failed authentication
- Fixed GitHub PR URL detection after PTY-backed pushes by stripping terminal escape sequences before scanning push output

## v0.11.3

- User-initiated Git network actions now run interactively so SSH key passphrase prompts can be answered in `gx push`, `gx status`, and `gx wt`
- Background Git commands still fail fast on credential prompts to avoid hanging the UI
- Added `o` to view the latest command output in `gx status` and `gx wt`; composite actions now include labeled output for every step, such as stash, pull/rebase/push, and stash pop
- Changed lazygit shortcuts to `g` in `gx status` and `gx wt`

## v0.11.2

- Added session-scoped diff context controls in `gx status` with `[` / `]`, clamped to a minimum of `-U1`
- Show the current diff context in the status footer and added help/docs for the new context controls

## v0.11.1

- `gx push` now asks for confirmation before checking remote divergence
- Push actions in `gx status` and `gx wt` now consistently confirm first, then run divergence checks
- Fixed side-by-side `delta` rendering so line-number colors match the configured theme instead of falling back to delta's default side-by-side colors
- Added `make test-docker-ubuntu` plus a helper script to run the test suite in a CI-like Ubuntu container with `git-delta` installed

## v0.11.0

- Added side-by-side diff render mode in `gx status` (`s`) with full interactive staging support across hunk, line, and visual selection flows
- Added side-by-side hunk gutter indicators and improved side-by-side rendering fidelity (adaptive width, fullscreen width recalculation, dimmed section separators)
- On very wide screens (`>140` cols), status pane now uses 17% width to prioritize diff space
- Hardened status E2E reliability on CI by disabling repo auto-gc in remote/clone test setups
- Side-by-side mode now explicitly requires `delta`; CI installs `delta` so side-by-side coverage runs there too
- Made `delta` rendering more consistent across environments by generating a temp config with the expected side-by-side hunk-header settings
- Reuse the generated temp `delta` config for the process lifetime instead of recreating it on every render

## v0.10.3

- Updated `gx status` docs and UX highlights, including clearer yank shortcuts (`yy` / `yl` / `ya` / `yf`)
- Added branch sync summary to the status pane header (synced/ahead/behind/diverged)
- Added mouse-wheel scrolling in status diff panes (unstaged/staged and fullscreen)
- Updated `e` in diff view to open `$EDITOR` at the selected hunk/line when supported by the editor

## v0.10.2

- Fixed intermittent CI/status lock contention by running read-only status probes with `git --no-optional-locks`
- Applied the lock-avoidance path to stage file listing and uncommitted-change collection

## v0.10.1

- Updated status yank mappings to a clearer set: `yy` (content), `yl` (location), `ya` (all context), and `yf` (filename)
- In `gx status` diff view, yank actions now respect focus granularity (hunk, line, or visual selection)

## v0.10.0

- Renamed the `gx stage` command to `gx status`
- Updated command routing, usage/help text, and tests to use `gx status`
- Updated docs and prompts to reflect the new command name

## v0.9.1

- Fixed diverged push force-push target in `gx stage`: force push now correctly uses the remote name (`origin`) instead of the upstream ref (`origin/<branch>`)

## v0.9.0

- Divergence detection: before pushing gx will detect if he branch has diverged and will offer the user to rebase, force push or abort
  (across `gx push`, `gx wt`, and `gx stage`)
- `gx stage` UX updates:
  - `.` / `,` jump to next/previous file from diff view
  - fullscreen diff now hides the status pane
  - `ol` opens `lazygit log`
  - `e` opens the currently selected file in `$EDITOR` from both status and diff views
- Improved stage patch robustness by falling back to line-range patch application when hunk apply reports a corrupt patch

## v0.8.0

- `gx stage`:
  - Added visual line-range mode (`v`) so you can select multi-line blocks and stage/unstage them with `space`
  - Added discard flows via `d` with mandatory confirmation prompts (status-file discard semantics, unstaged line/hunk/range discard, staged `d` as unstage)
  - Added stage yank mappings: `yc` for AI-friendly diff context and `yf` for filename-only yank
  - Improved test coverage with additional unit and E2E tests for visual mode, discard, and yank flows
  - Refactored internals by splitting the large monolithic model/view files into focused modules for update, key handling, navigation, runtime state, and rendering

## v0.7.2

- Expanded `gx stage` with action keys: pull (`p`), push (`P`), rebase (`b`), and amend (`A`) with confirmations
- Push in stage now matches worktrees behavior: detects GitHub PR URLs and asks whether to open them
- Added live, cancellable action output overlays in stage (`ctrl+c` cancels running git command)
- Improved stage navigation and UX: debounced status diff loading while scrolling, parent-folder focus on `h`, and additional regression/E2E coverage for push/pull/rebase flows
- Refactored shared UI/runtime pieces used by both stage and worktrees (URL opener, confirm/output modal primitives, cancellable command runner)

## v0.7.0

- Added a dedicated `gx stage` TUI for file, hunk, and line staging/unstaging with split unstaged/staged diff panes

## v0.6.1

- Fixed the yank files dialog so pressing `space` can toggle selected files off again
- Added regression coverage for the checklist space-toggle behavior

## v0.6.0

- Migrated the TUI stack to Bubble Tea v2, Bubbles v2, and Lip Gloss v2
- Replaced the old Bubble Tea v1 `teatest` dependency with a small repo-local v2 test harness

## v0.5.5

- Worktree Base column now refreshes after pulling the main branch via stash-pull

## v0.5.4

- Dirty column now uses colored styles: yellow for modified, cyan for untracked, magenta for both
- In portrait (stacked) layout, the table now sizes to fit its content rather than taking a fixed percentage of the screen height

## v0.5.3

- Confirm/error/logs/yank modals are now rendered as overlays, keeping the worktrees table and sidebar visible in the background
- Removed the Branch column from the worktrees table; when a worktree's branch name differs from its directory name, the branch is shown inline in the Worktree column as `(branch-name)`
- Fixed confirm dialog title being hardcoded as "gx push"

## v0.5.2

- Added `gx bump` command: creates an annotated version tag and optionally pushes; accepts `major`, `minor`, or `patch` as an optional argument, or shows an interactive picker with the resulting version for each option

## v0.5.1

- Main branch worktree always appears first in the list
- Main branch name and branch are rendered in orange to distinguish them at a glance
- With nerd font icons, the main worktree uses a home icon (`󰋜`) instead of the folder icon

## v0.5.0

- Added `gx stashify <cmd...>`: stashes uncommitted changes, runs the command, auto-pops on success, prompts to pop on failure
- Added `b` keybinding to rebase the selected worktree on main; confirms before rebasing; if dirty, offers to stash first
- Pull (`p`) on a dirty worktree now asks to stash first; cancelling shows "Pull aborted (dirty worktree)"
- Pulling the main branch now refreshes the Base column for all worktrees
- Sidebar now shows the latest commit (hash, subject, date, and relative date)

## v0.4.3

- Added `N` keybinding: create a new worktree and open a new tmux session (same name, cwd set to the worktree path), switching to it immediately
- Added `T` keybinding: create a new worktree and open a new tmux window
- Push (`P`) now shows a confirmation modal before executing
- Added `o` keybinding to view the output log of the last pull/push job
- Fixed `gx wt clone` to run `git fetch origin` and set up local branch upstreams after cloning

## v0.4.2

- Added `Base` column to the worktree table: `✓` if the branch is rebased on main, `✗` if it needs a rebase
- Added "Base" section to the sidebar showing the same rebase status for the selected worktree
- Fixed table scroll window rendering more rows than the table height, which could push the status bar off-screen

## v0.4.1

- Added vim-like search: press `/` to enter search mode, type to filter and highlight matching worktree names and branches, `ctrl+n` / `ctrl+p` to jump between matches, `enter` or `esc` to exit
- The Worktree column now takes the remaining space; Status column is fixed at ~20% width
- Fixed ANSI-styled cell content corrupting column alignment in the table — replaced `bubbles/table`'s internal renderer (which used `runewidth.Truncate`, not ANSI-aware) with a custom one using `charmbracelet/x/ansi.Truncate`

## v0.4.0

- Added `l` keybinding to open the selected worktree in lazygit (suspends the UI, restores it when lazygit exits)
- Consolidated worktree-related CLI commands under `gx wt`:
  - `gx wt list` — list worktree names
  - `gx wt abs-path <name>` — print absolute path of a worktree
  - `gx wt clone <url> [dir]` — clone using the `.bare` trick

## v0.3.2

- Added `gx list-worktrees` command that prints all worktree names, one per line
- Added `gx worktree-abs-path <name>` command that prints the absolute path of the named worktree
- When pushing a branch for the first time, the GitHub PR creation URL is detected and a modal asks whether to open it in the browser (defaults to Yes)
- Fixed `run` to capture stderr even on success (needed for parsing remote push output)

## v0.3.1

- Rebinded pull to `p` and push to `P`, freeing up the old `l` / `s` keys
- After yanking files (pressing `y` and confirming), the app enters a dedicated paste mode where only navigation (`j`/`k`) and `p` to paste (or `esc` to cancel) are active — this is what freed `p` for pull in normal mode
- Refreshes the worktree list after a paste completes

## v0.3.0

- `gx clone-wt` now uses the `.bare` directory trick: clones into `my-repo/.bare/` and writes a `my-repo/.git` file pointing to it, so worktrees live cleanly alongside `.bare/` rather than inside it
- Delete worktree now shows a spinner while the deletion runs and a "Worktree {name} deleted successfully" toast on completion
- Added `gx doctor` command to check a repo for common configuration issues:
  - Verifies the origin fetch refspec is set correctly
  - For `.bare`-style repos: verifies the outer `.git` file points to `.bare`
  - For `.bare`-style repos: verifies each worktree's `.git` file points to the correct location
- Added `gx doctor --fix` to interactively apply fixes with confirmation prompts

## v0.2.1

- Added `U` keybinding to run `git remote update` and refresh all worktree statuses

## v0.2.0

- Added `gx version` command (also `--version`, `-v`) to print the current binary version
- Added `scripts/bump.sh` for bumping the version, creating an annotated git tag

## v0.1.5

- `gx clone-wt` now immediately fixes the fetch refspec after cloning, so remote tracking refs populate correctly on the first fetch
- On startup, the worktrees view checks whether the fetch refspec is misconfigured or remote tracking refs are missing, and offers to fix it automatically
- Delete and track confirmations are now shown as a centred modal with Yes/No buttons instead of a status-bar prompt
- Pull and push now also refresh the sidebar after completing
- Fixed a bug where the `origin/<branch>` fallback could match a bad local branch instead of the remote tracking ref

## v0.1.4

- Added `R` keybinding to refresh the worktree list and all statuses

## v0.1.3

- Added `t` keybinding to set a remote tracking branch for the selected worktree

## v0.1.2

- The sidebar now shows a "no remote tracking branch" note with a hint to press `t` when no upstream is configured

## v0.1.1

- Status column now shows ahead/behind relative to the remote tracking branch instead of the main branch
- Sidebar ahead/behind commit lists now compare against the remote tracking branch instead of main
- Sidebar section headings updated to "Commits ahead of remote" and "Commits behind remote"
