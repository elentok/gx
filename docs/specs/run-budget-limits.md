# Run budget limits

Handoff spec from the Run budget limits wayfinder map (`.scratch/run-budget-limits/map.md` in the
local ticket tracker — decisions `01`–`10` in that epic's `issues/` directory).

## Problem Statement

gx runs ralph-loop epics unattended, sometimes for hours, with no dollar guardrail anywhere in the
path: nothing stops a stuck or expensive loop from running up an unbounded bill, nothing tells the
operator how much a session has spent so far (per epic or in total), and nothing warns them upfront
if their Claude account is configured to silently draw on paid "Extra usage" credits once its
subscription's included usage runs out. Separately, the agent-picker that gates every epic launch
always shows a Claude-vs-Codex choice even when Codex isn't actually usable on this machine, forcing
a needless click through a menu that offers a nonexistent option.

## Solution

A per-Attach-session dollar budget with two independent thresholds — a soft limit that pauses new
work (resumable) and a hard limit that force-stops running agents (via herdr) — computed from a live
30-second poll of running agents' cost, not just cost already landed on finished tickets. Every
dollar figure this feature produces, everywhere it's shown, is an **estimated API-equivalent cost**
(from a hardcoded per-model pricing table) — not literal billed spend, and notably not a measure of
real cost to a Pro/Max subscriber operating within their included usage. v1 budget enforcement covers
Claude only: Codex has no cost source today, so running Codex iterations are excluded from the total
and flagged separately rather than silently priced at $0. Crossing a configured notification
threshold, tripping the soft limit, or tripping the hard limit each send a budget notification through
Telegram/Slack, independent of gx's existing per-epic notification mute/throttle logic. The live total
and each epic's own running total are surfaced in the Queue tab's header at all times. Before every
launch, a run-start banner reports the configured budget and — from a local, best-effort read of
Claude Code's own cached account state — whether the account is configured to auto-purchase extra
usage credits past the subscription. That same banner also folds in a simplified agent-picker: only
agents actually available on this machine are offered, collapsing to a plain Yes/No confirmation when
there's only one.

## User Stories

**Budget configuration**

1. As an operator, I want a `budget` config section with a soft limit, a hard limit, and a list of
   notification thresholds, so that I can tune how aggressively gx protects me from overspending.
2. As an operator, I want the budget on by default with sane values, so that I get protection without
   having to discover and configure it myself.
3. As an operator, I want to be able to disable the soft limit and hard limit independently by
   setting either to zero, so that I can opt out of enforcement while keeping notification
   thresholds, or vice versa.
4. As an operator, I want malformed or inconsistent budget config (unsorted thresholds, a hard limit
   at or below the soft limit) to be silently clamped into a sane shape rather than rejected, so that
   a typo in my config doesn't stop gx from starting.
5. As an operator, I want the budget section documented alongside gx's other config sections, so
   that I can find and edit it the same way I already edit `execution-queue`/`notifications`.

**Live cost aggregation**

6. As an operator, I want gx to track total spend across every epic running under my current Attach
   session live, not just when a ticket lands, so that a single expensive ticket can't blow past my
   budget unnoticed before it finishes.
7. As an operator, I want that live total to reset when I detach and reattach, so that budget
   tracking always reflects "cost since I started paying attention this session," matching how gx
   already scopes an Attach session.
8. As an operator, I want a stuck or unreadable transcript read to degrade gracefully (contribute $0
   for that tick, retried next tick) rather than corrupt or crash the whole budget computation, so
   that one flaky read doesn't take down budget tracking for every other running epic.
8a. As an operator running a Codex agent, I want gx to tell me plainly that its spend isn't counted
    toward my budget (rather than silently showing $0), so that I don't mistake an uncovered agent
    for a protected one.
8b. As an operator, I want gx to be clear everywhere it shows a dollar figure that it's an estimate
    of API-equivalent usage, not my actual bill, so that I don't mistake a usage estimate for real
    spend — especially on a subscription plan where the two can diverge completely.
8c. As an operator running multiple gx instances attached to different repos, I want to understand
    that each instance enforces its own budget independently (no cross-repo total), so that I don't
    assume a single global cap protects me across every repo I'm working in at once.

**Soft-limit behavior**

9. As an operator, I want the queue to pause (not stop) when live spend crosses the soft limit, so
   that in-flight work finishes cleanly instead of being cut off mid-turn.
10. As an operator, I want a soft-limit pause to be resumable once I've reconfigured my budget or
    just want to keep going, so that a budget trip doesn't permanently end my session.
11. As an operator, I want a soft-limit pause to be tracked independently from my own manual pause
    ("p" key), so that clearing one doesn't accidentally clear the other.
12. As an operator, I want to be warned with the current spend and limit before I override a
    soft-limit pause, so that I make an informed choice to keep spending rather than click through a
    generic confirmation.
12a. As an operator, I want my manual pause key to always ask for confirmation (not just when a
     budget pause is active), so that pausing and resuming get consistent, predictable behavior
     regardless of why I'm pressing it.
13. As an operator, I want an override to only suppress the *current* over-limit trip, re-arming once
    spend climbs meaningfully further past where I overrode it, so that one deliberate override
    doesn't silently disable budget enforcement for the rest of my session.
14. As an operator, I want exactly one notification when the soft limit trips, not one per poll while
    still over, so that I'm not spammed every 30 seconds while intentionally overriding it.

**Hard-limit behavior**

15. As an operator, I want every live agent pane under my Attach session sent a graceful stop signal
    the moment spend crosses the hard limit, so that gx gives agents a chance to wind down cleanly
    before anything gets force-killed.
16. As an operator, I want every pane touched by a hard-limit stop closed after the grace period
    regardless of whether it quieted down, so that gx doesn't have to guess whether a quiet pane
    actually finished or just paused mid-turn.
17. As an operator, I want new epic/iteration starts blocked while a hard-limit stop is in progress
    and afterward, so that a killed pane's epic (or another queued epic) doesn't just relaunch
    seconds later and blow the limit again.
18. As an operator, I want a hard-limit-killed ticket to land in the same `needs-repair` status gx
    already uses for other interrupted work, so that I don't have to learn a new recovery path.
18a. As an operator, I want a ticket that manages to land successfully during the hard-limit grace
     period left alone as `done`, not overwritten to `needs-repair`, so that a well-timed finish
     isn't punished just because a kill was in progress.
18b. As an operator, I want every live agent stopped when the hard limit trips — including a Codex
     agent whose spend isn't counted toward my budget — so that a runaway loop is actually halted
     rather than left running just because gx couldn't price it.
19. As an operator, I want the same override mechanism as the soft limit (with its own warning
    copy), so that I don't have to learn two different override flows.

**Notifications**

20. As an operator, I want a chat notification when spend crosses a configured threshold, so that I
    get an early heads-up before hitting either limit.
21. As an operator, I want a chat notification the moment the queue pauses on the soft limit, and
    another the moment panes get killed on the hard limit, so that I know immediately when
    enforcement actually acted, even if I'm not watching the TUI.
22. As an operator, I want a single poll tick that jumps past several thresholds at once (a big
    ticket landing) to send one message naming the highest threshold crossed, not a burst of
    messages, so that a fast jump in spend doesn't itself look like a notification storm.
23. As an operator, I want budget notifications to always go out regardless of gx's existing
    per-epic notification mute/throttle state, so that a routine chatter mute on one epic can't
    accidentally hide the one message that tells me I'm overspending.
24. As an operator, I want budget-related events (threshold crossed, soft-paused, hard-killed) kept
    in a durable session-level log even if nobody's watching chat or the TUI at the time, so that I
    can reconstruct what happened after the fact.
25. As an operator, I want a still-over-budget reattach to re-notify and re-pause on the very next
    poll, so that reattaching doesn't silently let me keep spending past a limit I already tripped.

**Subscription safety check**

26. As an operator on a Pro/Max subscription, I want gx to tell me — once per Attach — whether my
    account is configured to auto-purchase extra API credits once my included usage runs out, so
    that I'm not surprised by an unexpected charge from a runaway agent loop.
27. As an operator, I want that warning to be unmissable (but non-blocking) when extra usage is
    enabled, so that I notice it without gx pretending to enforce something it has no control over.
28. As an operator, I want a quiet one-line confirmation when extra usage is verified disabled, so
    that I get peace of mind without a false alarm.
29. As an operator, I want honest "couldn't verify" copy (not a false "you're safe" or false alarm)
    when gx can't read the setting, pointing me at where to check manually, so that I'm not misled
    by a best-effort local read of an undocumented, unstable field.
30. As an operator, I want to be able to permanently silence the extra-usage warning once I've
    acknowledged it, so that it doesn't nag me every single Attach.

**Run-start banner and simplified agent picker**

31. As an operator, I want to see my configured budget and the subscription safety-check result
    every time I'm about to launch an epic, so that I have the full picture right before committing
    to spend.
32. As an operator, I want the budget lines hidden when I've explicitly disabled both limits, so
    that the banner doesn't show me a meaningless "$0 of $0."
33. As an operator, I want the agent picker to only ever offer agents that are actually usable on
    this machine, so that I'm never one click away from launching an agent that's going to fail
    immediately.
34. As an operator with only one usable agent, I want a plain Yes/No confirmation instead of a menu
    with one real choice, so that starting a run doesn't require picking from a list of one.
35. As an operator with more than one usable agent, I want picking an agent from the list to be the
    confirmation itself — no extra step, so that starting a multi-agent-capable run stays a single
    action.
36. As an operator, I want Escape to always cancel the launch, regardless of whether I'm looking at
    a Yes/No confirm or an agent-picking list, so that backing out works the same way everywhere.

**Live cost display**

37. As an operator, I want to see my current total spend against my budget at a glance in the Queue
    tab at all times, so that I don't have to open a menu just to check how much I've spent.
38. As an operator, I want that total to change color as it approaches and crosses my soft and hard
    limits, so that rising risk is visible before I hit a wall.
39. As an operator, I want to see each epic's own running cost on its own header line, so that I can
    tell which epic is actually driving my spend when several are running at once.

## Implementation Decisions

**Config (`config` package)**

- New flat `BudgetConfig`, following the existing per-section config file convention
  (`ExecutionQueueConfig`, `NotificationsConfig`): one file, a `Default*Config()` constructor,
  kebab-case JSON tags, wired into the main config struct with the existing pointer-based
  partial-merge pattern.

  ```go
  type BudgetConfig struct {
      Notifications []float64 `json:"notifications"`
      SoftLimit     float64   `json:"soft-limit"`
      HardLimit     float64   `json:"hard-limit"`
  }

  func DefaultBudgetConfig() BudgetConfig {
      return BudgetConfig{
          Notifications: []float64{50, 100, 150, 200, 250, 300},
          SoftLimit:     300,
          HardLimit:     350,
      }
  }
  ```

- Units: `float64` USD dollars, matching the existing `ActualCost`/`formatCost` convention
  elsewhere in the codebase — no cents-as-integer representation.
- `soft-limit: 0` and `hard-limit: 0` each independently disable that limit; no separate `enabled`
  flag, mirroring the empty-string-is-off convention already used by the Telegram/Slack config
  sections.
- Config loading clamps rather than rejects, matching the existing execution-queue clamp behavior:
  `notifications` sorted ascending and deduped; if both limits are nonzero and `hard-limit <=
  soft-limit`, `hard-limit` is bumped up to `soft-limit`. `notifications` thresholds have no
  enforced ordering relationship with `soft-limit`.
- Documented in the README's existing config-options section alongside `execution-queue` and
  `notifications`, not a new doc file — worded as an estimated API-equivalent cost, not literal
  billed spend.
- Ships on by default, a behavior change for every existing user on upgrade: documented in the
  CHANGELOG alongside the shipped default values and how to disable them (set both limits to `0`).

**Live-run identity plumbing (`ralphloop` → registry layer)**

- Live-run bookkeeping gains the session ID, working directory, agent kind, pane ID, and tab ID for
  each currently running iteration (populated from the iteration-started event, cleared when that
  iteration finishes), threaded from `ralphloop`'s existing live-event pipe into the registry layer
  that the Queue tab reads from. This is foundational plumbing with no policy of its own — it's what
  lets code outside `ralphloop` know which live transcript to read and which pane/tab to act on.

**Live cost aggregation (registry layer)**

- One attach-wide poller goroutine (not per-epic), started at the transition from zero to one active
  attachment and stopped at the symmetric transition back to zero, so it runs exactly once per
  Attach session. Poll interval: 30 seconds, matching the existing poll cadences already used
  elsewhere in `ralphloop`.
- **Baselined per epic, not per session.** Each running epic's already-landed cost is captured as its
  own baseline the first time the poller observes it running — not once for the whole session at
  attach time. An epic that starts mid-session (once a concurrency slot frees up) is baselined on its
  own first tick, so only its post-baseline spend counts; an epic that finishes and relaunches within
  the same attach session keeps its original baseline. This makes "resets on detach/reattach" (below)
  actually true for the effective total, and avoids a stale reattach to a long-running epic
  immediately tripping the hard limit on lifetime-landed cost it didn't spend this session.
- Each tick: for every running epic, sum `(landed cost since that epic's baseline) + in-flight cost`
  read fresh from its currently-running iteration's transcript, guarded by an mtime check that skips
  re-parsing a transcript whose mtime hasn't changed since the last tick (no incremental/offset-tracked
  reader beyond that guard). Combine into one cached total, exposed via a thread-safe getter. The same
  pass also retains a per-epic breakdown (epic name → its own baselined total), exposed via a second
  getter, so the per-epic display doesn't require a separate aggregation. A guard ensures a ticket's
  cost is never counted as both landed and in-flight on the same tick, which can otherwise briefly
  overlap during the land sequence.
- **Codex exclusion**: a running Codex iteration has no cost source and is excluded from the sum
  entirely rather than priced at $0; a separate "unpriced running count" getter tracks how many
  currently-running iterations aren't covered, so a consumer can flag it rather than silently
  under-report.
- Scope: only epics currently running under *this* Attach session — never a scan of historical or
  other-session data. The total (and the per-epic breakdown) effectively resets on detach/reattach,
  since fresh baselines are captured on the new attach's first tick.
- A failed or missing transcript read for one running iteration contributes $0 for that tick only,
  retried next tick — never fails the whole aggregation. A consecutive-miss count per running
  session is tracked alongside the total as an internal reliability signal (not user-facing; see
  Out of Scope).
- `BudgetConfig` is threaded into the registry layer (and from there into the Queue tab's model) so
  every downstream piece can read the configured limits/thresholds. There is no config hot-reload —
  the loaded config is read once at process start, matching every other config section; a config edit
  takes effect on the next process start (a detach/reattach), not live.

**Soft-limit behavior**

- Reuses the existing resumable pause/resume primitive (the same one the manual pause key already
  drives), not the one-way per-epic drain primitive — soft-limit needs to un-pause once the budget
  is reset or reconfigured, which the one-way primitive can't do.
- A dedicated soft-limit pause flag, tracked independently from the user's own manual pause flag;
  new epic/iteration starts are refused while *either* is set, and clearing one never clears the
  other.
- Latched, not self-healing: once tripped, the pause does not auto-clear just because a later poll
  shows spend back under the soft limit (to avoid pause/resume flapping as spend hovers near the
  line). Cleared only by a manual operator override, or by detach/reattach (which starts a fresh
  baseline, so a reattach only re-trips once post-reattach spend climbs back past the limit on its
  own). There is no config-hot-reload clearing path — "raise the limit" only takes effect on the next
  process start, itself a detach/reattach.
- **The manual pause key now opens a confirm dialog on every press**, not just when a budget pause is
  active — a deliberate behavior change from today's instant toggle, chosen for consistency (one
  pause/resume flow, not two). The dialog carries budget-specific copy (current spend and limit) only
  when a soft- or hard-limit pause is active; otherwise it shows plain pause/resume confirm copy.
- Accepting the dialog while budget-paused clears the pause as a sticky override for *this* trip
  only. Because the live total only ever rises within a session (spend is monotonic; it drops only
  via a hard-limit kill), re-arm is defined in spend terms rather than "drops then rises again":
  the override re-arms once spend climbs past `min(next configured notification threshold above the
  override point, override point + 10% of the soft limit)` — always well-defined even when no
  threshold sits above the override point (true of the shipped default, whose highest threshold
  equals the soft limit) or when the notification list is empty.
- Exactly one notification fires, on the transition from not-paused to paused — never once per poll
  while still over.

**Hard-limit behavior**

- Session-wide scope: every live iteration across every epic running under the current Attach
  session is stopped — not just the one whose cost tipped the total over, and including a running
  Codex iteration even though its spend isn't counted toward the total (the point is halting all
  spend, not attributing it).
- A single atomic seam — "stop this iteration's agent, then mark its ticket `needs-repair`" — owns
  the whole mechanism, implemented inside `ralphloop` (not the registry layer that triggers it), so
  the status write never races `ralphloop`'s own land-time writes to the same ticket file: graceful
  stop signal, 15-second grace period, then the pane is closed unconditionally, regardless of whether
  it quieted down — a quiet pane is not distinguishable from a naturally-finished one, so every
  touched iteration is closed uniformly rather than left in an ambiguous live state.
- The `needs-repair` mark is applied only if the ticket hasn't already reached a terminal outcome —
  an iteration that finishes and lands successfully during the grace period is left `done`, not
  overwritten.
- Tripping the hard limit also sets a dedicated pause flag (same mechanism as the soft limit, its
  own label, same re-arm rule) so new starts are refused both during and after the kill — otherwise a
  killed pane's epic, or another queued epic, would just relaunch seconds later and blow the limit
  again.
- Same override mechanism as the soft limit (same confirm-dialog key, hard-limit-specific copy).
  The override only un-pauses new starts; already-stopped iterations are never relaunched.
- A hard-limit-killed iteration's ticket lands in the existing `needs-repair` status — no new
  kill-specific ticket state.

**Notifications**

- A new send path, independent of gx's existing per-epic notification sink: a budget crossing isn't
  attributable to any single epic (it's a sum across every running epic), so it sends directly per
  configured transport rather than broadcasting through every running epic's own sink (which would
  send N duplicate copies and misattribute the send to N unrelated epics' throttle state).
- Deliberately bypasses the existing global per-transport mute/throttle gate entirely — that gate
  protects against routine chatter storms, and a budget notification is inherently rare and must
  never be silently swallowed by an unrelated epic's storm having already tripped that gate.
- Three distinct notification kinds, one shared send/log mechanism: threshold crossed, soft-limit
  paused, hard-limit killed. Each gets its own text template — the phrasing differs enough
  ("crossed your $X notification threshold" vs. "soft limit reached, queue paused" vs. "hard limit
  reached, killing panes") that one generic parameterized template would read awkwardly.
- Threshold-crossing dedup uses an in-memory high-water-mark, checked on the same poller tick that
  recomputes the live total — no separate timer. A single tick that jumps past multiple thresholds
  at once collapses to one message naming the current total and the highest threshold just crossed.
- A durable, flat, append-only session-level log (parallel to gx's existing per-epic run logs, but
  not attributable to one epic) records every threshold-crossed/soft-paused/hard-killed event, so a
  budget action that fires while nobody's watching chat or the TUI still leaves a trace.
- Reattach resets both the in-memory policy state (the high-water-mark, the pause latches) and the
  effective dollar total, since live cost aggregation captures fresh per-epic baselines on the new
  attach's first tick — a reattach starts a clean slate. A still-running epic only re-notifies or
  re-pauses once its post-reattach spend climbs back past a threshold or limit on its own, not
  immediately on reattach.

**Subscription safety check**

- Detects the account's "Extra usage" setting via a best-effort, defensive local read of Claude
  Code's own cached account state (an undocumented internal field, not a stable public schema) —
  read once per gx process and cached for the process's lifetime (not tied to Attach state, since the
  run-start banner that displays it renders before any attachment exists); a missing or renamed field
  is treated as "unknown," never as a crash or a false "disabled."
- Three distinct displayed states: verified enabled (unmissable, non-blocking warning — gx has no
  control over the account setting itself, so it never blocks the run on this), verified disabled
  (a single quiet confirmation line), unknown/unverifiable (remind-only copy pointing at the account
  billing settings, explicitly not claiming either state).
- The enabled-state warning is permanently suppressible via a config flag once acknowledged, since
  re-showing it every Attach with nothing gx can do about it would just be nagging.

**Run-start banner and simplified agent picker**

- One unified modal, opened by the Queue tab's existing launch key, replacing both today's plain
  agent-picker menu and the (nonexistent) confirm step that used to follow it. This is also where an
  existing unreachable code path for a second, never-triggered agent-picker menu gets deleted.
- Banner (always rendered, above the action area): the subscription-check line, plus the configured
  budget's soft/hard limits and notification thresholds, worded as an estimate scoped to this repo's
  Attach session (not a global cap across other attached repos) — omitted entirely when both limits
  are disabled, leaving only the subscription line (if any).
- Action area depends on how many agents are actually available on this machine: Claude is always
  considered available; Codex's availability is a fresh, uncached PATH-only check on every open
  (deliberately lighter than the full auth/Herdr-integration check, which is unchanged and still
  gates actual launch — an unauthenticated-but-on-PATH Codex still appears here and still fails at
  launch with today's existing error). Zero available agents is a defensive, currently-unreachable
  case that shows a brief notice and never opens the modal.
- Exactly one available agent → a plain Yes/No confirmation. More than one → a pick-an-agent list
  (plus a Cancel option) where picking an agent *is* the confirmation, no second step. Escape aborts
  in both cases.

**Live cost display**

- The session-wide live total is spliced into the Queue tab's always-visible title line, in every
  run state that has poller data available (i.e., every state except the cross-process "attached to
  a different gx process" view, which has no local poller to read from) — shown as "$X of $Y" when a
  budget is configured, or a bare "$X" when it's disabled. Colored via the same colors already used
  for epic-header warning states: default below 80% of the soft limit, warning-colored from 80% up
  to the soft limit, alarm-colored at or above it. When one or more Codex iterations are currently
  running (excluded from the total, per Live cost aggregation above), a short unpriced-count note is
  appended next to the total (e.g. "$142.30 of $300.00 (+1 unpriced Codex run)").
- Each epic's own running total is appended to that epic's existing header status text (e.g. "3 of 5
  done · $X"), always shown in the default/unstyled color — the configured limits are session-wide,
  so coloring one epic's slice against them would misleadingly suggest that epic alone is over
  budget.
- The internal degraded/stale-read reliability signal from live cost aggregation is not shown in
  this UI at all — it's an input to the soft/hard-limit policy logic, not a user-facing indicator.

## Testing Decisions

A good test here exercises observable behavior through an existing seam — a config load result, a
running `ralphloop` session's externally-visible effects (pause state, killed panes, notifications
sent), or a TUI model's rendered output/state after simulated keypresses — never internal call
counts or private state.

- **Config**: unit tests over `config.LoadConfig` for the new `budget` section — default values,
  independent soft/hard-limit disable via zero, and the clamp behaviors (threshold sort/dedupe,
  hard-limit-at-or-below-soft-limit correction). Prior art: `TestLoadExecutionQueueConfigClampsLimitsToOne`
  and the neighboring tests in `config/config_test.go`.
- **Backend policy pipeline** (live aggregation, soft-limit pause, hard-limit trigger, notification
  sends — one seam, since these are one poller-and-policy pipeline inside the registry layer):
  driven through the `ui/tickets` package's existing stub seam — reassigning the package-level
  `runRalphLoop`/registry singletons the way `implement_notifications_test.go` and `queue_test.go`
  already do — since that's where this pipeline actually lives, not inside `ralphloop` itself. Fake
  session-cost reads simulate rising spend to assert the live total, baselining, the pause trip on
  the soft limit, the hard-limit trigger firing once per live iteration, and the captured
  notification sends/log entries.
- **Stop-and-repair seam** (the actual ctrl+c → grace period → close-pane → conditional
  needs-repair-mark sequence): this one piece genuinely lives inside `ralphloop`, so it's tested
  there against `herdrfake`, following `run_realgit_codex_quota_test.go`'s pattern of stubbing
  `Deps` (e.g. `PreflightAgent`, `AgentSendKeys`, `TabClose`) and driving a real run — separate from
  the `ui/tickets`-seam tests above, which only assert the seam was *invoked*, not its internals.
- **UI: agent-picker modal**: bubbletea `Update`-driven tests simulating the Queue tab's launch key
  and menu-selection keypresses, asserting the rendered banner content and the Yes/No-vs-picker
  branch by available-agent count. Prior art: `TestImplementAgentMenuDefaultsToClaude` and the
  broader keypress-driven test style in `ui/tickets/queue_test.go` and `ui/tickets/model_test.go`.
- **UI: header display**: formatting/color tests over `epicStatusLine` and `queueHeaderTitle` for
  the live-total and per-epic-total rendering across threshold bands. Prior art:
  `TestEpicStatusLineColorsByEpicState` in `ui/tickets/queue_header_test.go`.

## Out of Scope

- Per-epic budget limits — the budget is strictly per-Attach-session, matching the map's locked-in
  scope decision; an individual epic never gets its own limit.
- An incremental/offset-tracked transcript reader for live cost reads — v1 re-parses the full
  transcript every poll tick; revisit only if profiling shows it matters.
- Any public/remote API check for the subscription's extra-usage setting — none exists; the check is
  local-only and best-effort.
- A dedicated danger-icon set for the subscription warning — none exists in the codebase today; the
  warning uses a plain ⚠️ prefix plus red/bold/caps styling instead of waiting on one.
- User-facing display of the live-aggregation degraded/stale-read signal — it remains an internal
  input to policy decisions only.
- Coloring the per-epic cost figure by the session-wide limits.
- Self-healing/auto-clearing soft- or hard-limit pauses — both are latched until an explicit clear
  (a manual override, or detach/reattach).
- Live config hot-reload — a config edit takes effect on the next process start, not mid-session.
- Any UI beyond a brief notice for the zero-available-agents case — currently unreachable since
  Claude is always considered available.
- Cross-process/cross-repo budget aggregation — the budget is scoped to one repo's Attach session;
  two gx processes attached to two different repos each enforce their own limit independently, with
  no shared cap across them. The run-start banner names this scope explicitly rather than implying a
  global cap.
- A Codex cost source/pricing table — v1 budget enforcement is Claude-only; Codex spend is excluded
  from the total and flagged as unpriced rather than priced.

## Further Notes

- This spec bundles four largely independent slices — budget enforcement (config, aggregation,
  soft/hard limits, notifications), the subscription safety check, the run-start banner and
  simplified agent picker, and the Queue tab's live cost display — that share infrastructure but can
  reasonably ship as separate PRs in roughly the order they're described above, since later slices
  depend on the aggregation and config pieces landing first.
- The default budget ships **on** (`soft-limit: 300, hard-limit: 350, notifications:
  [50,100,150,200,250,300]`), a deliberate change from an initial all-zero/opt-in design made partway
  through charting this map — worth calling out since it means budget enforcement is live for every
  user immediately after upgrade, not something they have to discover and turn on.
- While charting this map, an existing agent-picker code path in the UI was found to be dead
  (unreachable — nothing in the codebase ever opens it); it gets deleted as part of the run-start
  banner/agent-picker work rather than left behind as confusing unused code.
- Full ticket-by-ticket rationale, including primitives considered and rejected (e.g. why the
  one-way per-epic drain mechanism doesn't fit the soft limit), lives in the wayfinder map's
  individual ticket resolutions and is worth reading before implementing the soft-/hard-limit
  tickets in particular.
- This spec was consult-reviewed (Opus, read-only) against the wayfinder map and the implementation
  ticket breakdown before build started. That pass surfaced several corrections folded into the
  decisions above: dollar figures are estimated API-equivalent cost rather than real spend; Codex
  runs are excluded from budget enforcement rather than silently priced at $0; the live total is
  baselined per epic at attach rather than including lifetime landed cost; a hard-limit-touched pane
  is always closed and conditionally marked `needs-repair` through one atomic seam, rather than a
  per-pane graceful/force distinction that turned out not to change the outcome; the manual pause key
  gains a confirm dialog on every press; the soft-limit override re-arms on a spend threshold rather
  than an unreachable drop-then-rise condition; and the budget is explicitly scoped to one repo's
  Attach session, not a cross-repo cap. The implementation ticket breakdown grew from 9 to 11 tickets
  to isolate the plumbing (live-run identity) and the stop/repair seam as their own prerequisite
  tickets, both reused by more than one downstream ticket.
