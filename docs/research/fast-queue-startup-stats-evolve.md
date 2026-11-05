# Where launch timings fit in Stats and gx-evolve

Ticket: `gx:fast-queue-startup/05`. Source tree: `main` at `0a3354cf4`.

Question: how do the web Stats page, `gx server stats` and `gx evolve export` get their data today, and where would per-step launch timings fit?

## Short answer

- Every Stats number comes from ticket and epic frontmatter, plus two side stores (the token ledger and the limit-burn data). The Stats page reads no run log. A stamp written when a ticket finishes is the pattern for per-ticket numbers.
- `gx server stats` is the same `GET /v1/stats` payload. It prints only the three throughput series in text, and the full payload with `--json`.
- `gx evolve export` is not a stats reader. It bundles unexported follow-ups. The evolve reader of numbers is `gx evolve inputs`, which reads ticket frontmatter and the epic's run log.
- The run log has no claim event and no per-step launch event today. Only `deps-installed` and `iteration-started` exist, and the claim time lives in the ticket's `claimed_at` frontmatter, which a later claim overwrites.
- Recommendation: add timing events to the run log (the source of truth), stamp a small `launch` summary on the ticket when it finishes (the same hook that stamps friction), add one Stats section, and add a "Launch rows" block to `gx evolve inputs` and the skill, like the existing `slower:` rows.

## 1. How Stats gets its data today

### The endpoint

- Route: `GET /v1/stats` in `server/stats/routes.go:12`. The payload type is `wire.Stats` in `server/wire/stats.go:73`.
- The handler is `Stats.read` in `server/stats/stats.go:54`. It validates `range` (7, 30 or 90) and an optional `project`.
- It loads every epic of every project, archived ones included, with `ticketmigrate.LoadStoreEpics` (`server/stats/stats.go:64`, `tickets/ticketmigrate/storewalk.go:46`). That is a full read of all ticket frontmatter on every request.
- It then reads the conversation records (grilling), the token ledger (cost) and the limit-burn data, and returns JSON (`server/stats/stats.go:64-85`).

The page's own design says the same. `docs/specs/webui-stats.md:33`: "The Stats page reads only these stamps." A `gx tickets migrate` step backfills old values where the data still exists (`migrations/steps.go:30`, `tickets/ticketmigrate/backfill_friction.go`).

### Bucketing per day and per project

- Days: `limits.NewStatsWindow(n, now)` (`server/limits/statswindow.go:15`) makes the last n local days. `Bucket(t)` maps an instant to a day index in the server's local time zone, or -1 when unset or out of range (`statswindow.go:43`).
- Project: `buildStats` skips every epic whose project differs from the filter (`server/stats/statsbuild.go:77`). Each `StoreEpic` carries its project, so a per-ticket stamp gets the project filter for free.
- Each series is a slice with one point per day (`newStats`, `server/stats/statsbuild.go:31`).
- Which day a ticket counts on is chosen per series. Tickets merged and cost use `done_at`. Friction uses `FinishedAt()`, which is `done_at` else `cancelled_at` (`server/stats/statsbuild.go:121`, `tickets/ticket.go:207`). The spec (`docs/specs/webui-stats.md:321`) says a ticket that is still paused shows nothing until it finishes.

### The closest existing stat, end to end: run events (friction)

This is the best model for launch timings, because it turns run-log events into a per-ticket stamp and then a per-day chart.

1. A run event is appended to the epic's `run-log.jsonl` by `runlog.AppendEvent` (`ralphloop/runlog/eventlog.go:201`). Types are fixed strings in `events/events.go`. `launch-failed` is one of them (`events/events.go:78`).
2. When a ticket finishes (done or cancelled), the one hook `Writer.StampFinished` runs (`server/ticketwrite/retrostamp.go:33`). It calls `ticketops.StampFriction` (`retrostamp.go:41`).
3. `StampFriction` reads the epic's run log and the ticket's own log, builds a `schema.FrictionSummary` and writes it to the ticket frontmatter (`ralphloop/ticketops/friction.go:60`). `frictionRunEvents` lists which event types are counted (`friction.go:19-27`). It replaces any earlier summary.
4. The summary type has YAML tags and is also the JSON type (`tickets/schema/friction.go:8`). The frontmatter field is `friction` (`tickets/schema/frontmatter.go:57`), read into `tickets.Ticket.Friction` (`tickets/loader.go:207`, `tickets/ticket.go:86`).
5. `addTicket` adds the summary to the day of `FinishedAt()` (`server/stats/statsbuild.go:121`, `frictionTally.addTicket` at `:216`). `fill` picks the top 5 reasons and a median over `PersonPauseSecs` (`:235-262`).
6. The result goes into `wire.StatsFriction` (`server/wire/stats.go:60`), which is JSON.
7. The Go types become TypeScript with `make web-types` (tygo, `Makefile:34`) into `webui/src/generated/server.ts`.
8. `webui/src/stats/stats.ts` fetches `/v1/stats?range=&project=` and refetches on events with a 2s debounce. `StatsPage.tsx` draws a `Friction` section of `StatCard`s (`webui/src/stats/StatsPage.tsx:157-201`).
9. `gx server stats` calls `apiclient.Reads.Stats` (`apiclient/reads.go:112`) and prints. Text mode prints only `date, tickets_merged, epics_merged, designs_completed` (`cmd/serverapi/server_stats.go:38-48`). `--json` prints the whole `wire.Stats`.

The same `Friction` stamp is summed per epic by `tickets.SumEpicStats` (`tickets/epic_stats.go:17`) for the epic page, and moves with a ticket (`tickets/ticketmove/move.go:67`, `cmd/tickets/tickets_close.go:66`).

### The other two stat shapes

- Cost: `elapsed_time`, `actual_context_window` and `actual_cost` are stamped at merge by `ralphloop/metrics/report_metrics.go` (`tickets/ticket.go:49-66`). Stats cost series come from the token ledger by day when present (`server/stats/statscost.go`), else from ticket stamps. The page marks days from each source (live, rebuilt, stamp).
- Limit burn: its own store, account-wide, not filtered by project (`server/wire/stats.go:106`).

### Facts that matter for launch timings

- `claimed_at` is "when the ticket last entered claimed" (`tickets/schema/ticket.go:314`). A reclaim overwrites it. Ticket `gx:split-server-pkg-impl/201` has run-log `iteration-started` lines from at least two claims, but one `claimed_at` (`/Users/david/.local/share/gx/tickets/gx/split-server-pkg-impl/issues/201*.md`, frontmatter `claimed_at: 2026-11-05T19:58:19Z`).
- The run log has no claim event. `ClaimAndLaunch` writes the claim to the ticket file (`server/sched/run.go:52`) and publishes an SSE event, but appends nothing to the run log.
- Launch events that exist today, with the code that writes them:
  - `deps-installed`, with `cwd` and the install commands in `reason` (`ralphloop/install.go:21`).
  - `iteration-started`, written after `StartAndPrompt` returns, so after the launch prompt is delivered (`server/sched/launch.go:188-192`). It carries `path` (`cold-start` or `resumed`).
  - `launch-failed` and `stale-pane-closed` on the failure paths (`server/sched/launch.go:179`).
  - `prompt-nudged` for each nudge step on an undelivered prompt (`server/sched/launch.go:152`).
- So today claim to prompt delivered is only derivable as `claimed_at` to the last `iteration-started`, with no steps between, and `deps-installed` is the only step marker.
- Real log check (`/Users/david/.local/share/gx/tickets/gx/split-server-pkg-impl/run-log.jsonl`, 338 `deps-installed` and 338 `iteration-started` lines): for ticket 206 on 2026-11-05, `deps-installed` is 22:39:15 and `iteration-started` is 22:39:46, a 31s gap, which matches the 30s ready timeout in the map's background.
- Size of what a run-log read would cost: 39 live run logs, 2.2 MB in total, biggest 382 KB (measured with `find ... -name run-log.jsonl`). Cheap enough, but still not how Stats reads data.

## 2. How gx-evolve gets its data

### `gx evolve export` is not a stats reader

`runEvolveExport` bundles the draft follow-ups of the `gx:follow-ups` epic that have no `exported_at` into one Markdown file for a contributor, then stamps `exported_at` (`cmd/evolve/evolve_export.go:84-137`). It reads no timings. Nothing about launch time belongs there.

### `gx evolve inputs` is the number reader

`gx-evolve` step 2 runs `gx evolve inputs <epic>` (`skills/gx-evolve/SKILL.md`, "Run the inputs verb yourself"). Data flow (`cmd/evolve/evolve_inputs.go`):

1. `runEvolveInputs` resolves the epic, reads the epic's run log with `runlog.ReadEvents` (`:278`) and the merged set from the repo trailers.
2. `buildInputsTable` makes one row per ticket from ticket frontmatter (`ElapsedTime`, `ActualContextWindow`, `ActualCost`, `ExpectedContextWindow`) and folds run-log events per ticket into session ids and per-type counts (`digestTicketEvents`, `:155`; counts land in `row.Events`).
3. `inputsFlags` turns that into flags: `slow` is elapsed above 2 times the epic's median (`:236`), `context-overrun`, `conflict-hit`, `relaunch`, `nudge`, pauses, and so on (`:210-245`). The flag names are a JSON contract the skill reads (`:23-35`).
4. `buildEpicChecks` adds two kinds of epic-level rows that need no transcript: `slower:` (a check kind at least 20% and 30s slower than the median of the last 5 full runs before the epic's first claim) and `flake:` (`cmd/evolve/evolve_checks.go:78-221`). They come from the check record and the brief index, not from the tickets.
5. Output is a text table or `--json` (`InputsTable`, `:65-90`).

How the skill uses it: flags rank tickets for a deep read, capped by `evolve.deep-read-cap`. Check rows (`### Check rows`) are findings by themselves, with no deep read, and go through triage and `gx-follow-up` as category `idea` (slower) or `bug` (flake) (`skills/gx-evolve/SKILL.md`). A test pins the anchors `### Check rows`, `slower:`, `flake:` in the skill (`skills/check_rules_test.go:49-51`), so a new block must keep those.

Facts about what is missing for launch time:
- `row.Events` already counts every run-log event type per ticket. A new event type shows up as a count, but not as a duration.
- There is no launch duration field on the row, no launch flag and no launch row.
- Evolve compares nothing against a fixed target today. `slower` compares to a baseline of earlier runs, and `slow` compares to the epic's median.
- The deep-read subagents read session digests. A slow launch has no cause in a digest (the agent had not started yet), so a launch finding should be a row, not a deep read.

## 3. Where launch timings fit

### What to record (the event layer)

Run-log events are the source of truth, because `iteration-started`, `deps-installed` and `launch-failed` already live there, `StampFinished` already reads the run log, and a backfill can read old logs. Needed:

- A claim marker with the claim time (or put the claim time on the launch event), because `claimed_at` is overwritten.
- One launch timing event per launch with the step durations (claim to worktree, deps, agent start, ready wait, prompt delivery), total, the `path` (cold-start or resumed), and whether deps were skipped. The last field is needed because the 15s target applies only "when deps are unchanged" (map Destination).
- Event type names are open. A new type must be added in `events/events.go` and, if it should count as friction, in `frictionRunEvents`. It need not be in `kindRequired`.
- `Event` is a flat struct in `ralphloop/runlog/eventlog.go:68-197`. Durations need new fields there (or fields reused with a clear name). The line size cap is `events.MaxLineBytes` (`encodeEvent`, `eventlog.go:222`), so a steps map must stay small.

### Files a new launch-time stat would touch

Event layer:
- `events/events.go` (new type(s)).
- `ralphloop/runlog/eventlog.go` (new `Event` fields).
- Emit points: `server/sched/run.go` (`ClaimAndLaunch`: claim time, worktree prepare), `ralphloop/install.go` (deps step, skipped or run), `server/sched/launch.go` (agent start, ready wait, prompt delivered). The ready wait itself sits in `nativerunner/pty_launch.go` and the delivery in `ralphloop/supervise/launch.go`, which other tickets of this map cover.

Stamp layer (follows friction):
- `tickets/schema/ticket.go` and `tickets/schema/frontmatter.go` (new `launch` field and YAML tag, in both the struct and the frontmatter wire struct).
- A new summary type next to `tickets/schema/friction.go`, with a sum function.
- `tickets/ticket.go` and `tickets/loader.go` (mirror the field into `tickets.Ticket`).
- `ralphloop/ticketops/` (a `StampLaunch` that reads the run log, beside `StampFriction`) and `server/ticketwrite/retrostamp.go` (call it from `StampFinished`). `cmd/tickets/tickets_close.go` and `tickets/ticketmove/move.go` carry friction on cancel and move; the new stamp needs the same care.
- `tickets/epic_stats.go` (only if the epic page should show it).
- A `gx tickets migrate` step in `migrations/steps.go` and `tickets/ticketmigrate/` (backfill from old run logs, only for tickets that have `deps-installed` plus `iteration-started` pairs).
- Schema round-trip tests (`tickets/schema/stats_fields_test.go` shows the pattern).

Stats layer:
- `server/wire/stats.go` (a `StatsLaunch` section and a field on `Stats`).
- `server/stats/statsbuild.go` (`newStats`, a tally like `frictionTally`, and `addTicket`).
- `webui/src/generated/server.ts` through `make web-types`.
- `webui/src/stats/StatsPage.tsx` (a new section; `StatCard` supports bars, stacked, side-by-side, diverging and line, with no target line today, `webui/src/ui/stat-card/StatCard.tsx:11`) and its test `StatsPage.test.tsx`.
- `cmd/serverapi/server_stats.go` (`printStats` prints only throughput columns today, so launch columns are added there if wanted in text mode).
- `server/stats/stats_test.go`, `apiclient`, and `docs/specs/webui-stats.md` (the spec for the page).

Evolve layer:
- `cmd/evolve/evolve_inputs.go` (`InputsRow` launch seconds, a flag, or a new table block), a new file beside `evolve_checks.go` for launch rows, and `evolve_inputs_test.go`.
- `skills/gx-evolve/SKILL.md` and `skills/check_rules_test.go` (keep the existing anchors).
- `docs/specs/gx-evolve.md` (the verb contract).

### Where the stamp is bucketed

Open choice, with costs:

| Option | Counts on | Pro | Con |
|---|---|---|---|
| A. Stamp at finish, bucket by `FinishedAt()` | the day the ticket finished | Same rule as friction and cost; no new read path; project filter free; one stamp per ticket | A slow launch shows days late (when the ticket finishes); a ticket that never finishes shows nothing; a long ticket lands on a far day |
| B. Stamp at finish, bucket by each launch's own time | the launch's day | The chart matches when the launch ran; still one read path | New bucketing rule that differs from the other series; the page's spec says "counted on the finish day" for friction |
| C. Separate store of launch records (like the token ledger), written at launch time | the launch's day | Exact day; shows launches of tickets that fail or never finish; live | A new store with its own backfill, project field and tests; breaks "Stats reads only stamps" |
| D. Read run logs on each Stats request | the event's time | No stamp or backfill step | 2.2 MB of JSON on every request today, growing; archived epics' logs; breaks "Stats reads only stamps" |

Recommendation: B. Keep the finish-time stamp so the page keeps its single read path, but store each launch's start time in the stamp and bucket every launch on its own day. A reclaim or resume is then a second launch on its own day with its own timing, which A cannot show. If the person prefers the same rule as every other series, A is the fallback and loses little. C and D are not recommended.

### What the Stats card shows

Stats already computes a median across a range for friction (`median_resume_secs`, `server/stats/statsbuild.go:249-262`). The matching shape for launches: a per-day median series (a line or bars), a headline of the range median and the share of launches under 15s, and a split by launch kind (deps skipped or run, resumed or cold). No target line exists in `StatCard`; a line at 15s needs a small new prop, or the share-under-target headline stands in. `gx server stats` text mode needs the same two numbers.

## 4. What evolve needs to check launch time against 15s

Needed in the data:
- Per launch: total seconds, deps skipped or not, path (cold-start or resumed), and the step durations. Without the deps flag, a launch that ran an install cannot be told apart from a launch that broke the target.
- Per ticket in the epic: its launches, because a ticket can launch more than once.

Needed in `gx evolve inputs`:
- A constant for the target (15s), set in code or as a config key like `evolve.deep-read-cap` (`config/config.go:63`). A config key lets a slow machine change it; a constant is simpler. Recommendation: a constant first, because the map's Destination fixes 15s.
- A launch block like `slower:`: one row per launch that is over the target with deps skipped, naming the ticket, the total and the slowest step; plus one row when the epic's median launch is over the target. No deep read for these rows.
- Optionally a per-ticket `slow-launch` flag in `inputsFlags` for the table, with severity `rest`, so the table shows it but the deep-read cap is not spent on it.

Needed in the skill:
- A short `### Launch rows` block next to `### Check rows`: each row is a finding with category `idea`, no deep read, through triage and `gx-follow-up`. Keep the anchors that `skills/check_rules_test.go:49` pins.
- Say that a launch with deps run is judged against the install step, not the 15s total.

Two cautions:
- The only per-ticket number evolve has now is `elapsed_time` (`tickets/ticket.go:62`), which covers the whole iteration, not the launch. It cannot stand in.
- A timing that exists only in the run log is readable by `gx evolve inputs` (it already loads the log), so evolve works even before the stamp and backfill exist. The Stats page needs the stamp (or option C/D).

## 5. Things outside this ticket's scope that came up

- `gx server stats` text mode prints only throughput. Cost, friction and grilling are visible only with `--json`. Adding launch columns to the text table is a choice for the spec ticket.
- `gx evolve export` has nothing to do with stats. The map's goal 6 ("update `gx-evolve` to check the startup time") touches `gx evolve inputs` and the skill, not `export`.

## Sources

- `server/stats/stats.go`, `routes.go`, `statsbuild.go`, `statscost.go`
- `server/wire/stats.go`
- `server/limits/statswindow.go`
- `cmd/serverapi/server_stats.go`, `apiclient/reads.go`
- `webui/src/stats/StatsPage.tsx`, `stats.ts`; `webui/src/ui/stat-card/StatCard.tsx`
- `tickets/schema/friction.go`, `frontmatter.go`, `ticket.go`; `tickets/ticket.go`, `loader.go`, `epic_stats.go`; `tickets/ticketmigrate/backfill_friction.go`, `storewalk.go`
- `ralphloop/ticketops/friction.go`; `server/ticketwrite/retrostamp.go`
- `ralphloop/runlog/eventlog.go`; `events/events.go`; `ralphloop/install.go`
- `server/sched/run.go`, `launch.go`
- `cmd/evolve/evolve_inputs.go`, `evolve_checks.go`, `evolve_export.go`
- `skills/gx-evolve/SKILL.md`; `skills/check_rules_test.go`
- `docs/specs/webui-stats.md`, `docs/specs/gx-evolve.md`
- Run log: `/Users/david/.local/share/gx/tickets/gx/split-server-pkg-impl/run-log.jsonl`
