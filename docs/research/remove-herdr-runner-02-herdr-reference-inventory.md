# Inventory: every herdr reference sorted into delete, rewrite or keep

Ticket `gx:remove-herdr-runner/02`. Method: `rg -i herdr` over the worktree (406 files), then a read of each producer and consumer named below. Counts are by file. Line numbers are from branch `remove-herdr-runner-design` at `734bded7f`.

## Short answers

1. Only one pause kind is truly herdr-only: `agent_pane_busy`, and nothing in non-test code produces it any more. `agent_name_taken`, `agent_prompt_stalled` and `handle-mismatch` are shared with PTY and headless. Ticket 01 named `agent_name_taken` as herdr-only. That is wrong. Details in Q1.
2. Old data loads without error, but a removed `pause_reason` silently becomes "paused with no class". Neither the ticket reader nor the run-log reader validates kinds. A migration step is needed. Details in Q2.
3. No. A run whose runner name no longer resolves does not reach a fresh start today. Restart pauses it as `handle-mismatch`, unless its claim is empty. Details in Q3.
4. 51 files use `herdrfake` only through `RunHelperProcess()` in a `TestMain`. About 30 more files test herdr behaviour or script the fake for a runner flow. Details in Q4.

## Q1. Pause kinds, event types and recovery rules by producer

The closed enum is `events.kinds` (`events/events.go:238-248`). The pause reasons in the ticket schema repeat it (`tickets/schema/pause.go:28-30`, class table `:94-98`, labels `:125-129`, verbs `tickets/schema/pause_verbs.go:177-180`).

| Name | Producers today | herdr-only? |
|---|---|---|
| Kind `agent_pane_busy` (`events/events.go:153`) | None in non-test code. `rg` finds it only in the enum, the catalog (`recovery/catalog.go:270-271`), `schema/pause*.go`, `testutil/herdrctl/herdrctl.go:167,283,300` and docs. The launch path never builds it: `startprompt.StartAndPrompt` sets only `AgentNameTaken` or `IterationError` (`ralphloop/supervise/startprompt/startandprompt.go:102-107`). | Yes, and already dead. |
| Kind `agent_name_taken` (`events/events.go:152`) | Raised on `agentrunner.ErrLabelTaken` (`startandprompt.go:104-105`, `server/sched/launch.go:231-233`, `server/sched/run.go:198-199`). `ErrLabelTaken` is returned by PTY (`nativerunner/pty_registry.go:23`), headless (`nativerunner/headless.go:160`) and herdr (`agentrunner/herdrrunner/hosting.go:58-66`). | No. Shared. |
| Kind `agent_prompt_stalled` (`events/events.go:156`) | PTY paths: `server/sched/run.go:285,373`, `startandprompt.go:137,147,153,166`. The comment calls it "herdr's", but the code is shared. | No. Shared. |
| Kind `handle-mismatch` (`events/events.go:160`) | `server/sched/reclaim.go:107-108` for any runner (cannot reattach `:134`, no pane `:224`, worktree gone `:279`). | No. Shared. |
| `kindCauseHerdr` / `Kind.CauseHerdr()` (`events/events.go:229-234,287-288`) | Lists `agent_name_taken`, `agent_pane_busy`, `handle-mismatch`. Read by the outage fold (`server/ticketwrite/pause_ticket.go:58,68`) and `gx server` kind listing (`cmd/serverapi/server_snapshot.go:22,134,143`). | The attribute is herdr-only. The three kinds stay. |
| Event types `herdr-unavailable`, `herdr-available` (`events/events.go:122-123`, `server/wire/misc.go:147-148`) | `server/sched/herdrwatch.go:170-176` only. Read by `viewmodel/viewmodel.go:224-228`. | Yes. |
| Marker `herdr-agent-lost` (`events/marker.go:17`) | `server/sched/pausemarker.go:16-26` on `*herdr.AgentNotFoundError` only. Wording at `server/ticketwrite/pause_chat.go:34`. | Yes. |
| Wire reason `herdr-unavailable`, verdict `herdr down` (`server/wire/misc.go:116,134`) | `server/ticketwrite/nudge.go:29-30`, `verbs.go:85-86`, `server/sched/explainer.go:118-119`. | Yes. |
| Wire snapshot fields `herdr_unavailable`, `herdr_text` (`server/wire/server.go:17-18`, `server/wire/index.go:93-94`, `webui/src/generated/server.ts:1946-1947`) | `server/rootroutes.go:52,184`. No web UI code reads them (`rg -i herdr webui/src` finds only the generated file). | Yes. |
| Event type `stale-pane-closed` (`events/events.go:115`) | `server/sched/launch.go:221-226` after `ErrLabelTaken`. Any runner. | No. |
| Event types `claim-reopened`, `late-finish-adopted`, `reclaimed` | Any runner (`server/sched/reclaim.go:312`, `server/merge/latefinish.go:258`). | No. |
| Rule R6 (`recovery/catalog.go:259-285`) | `r6PaneBusy` is herdr-only. `r6NameTaken` matches `agent_name_taken`, which PTY raises too. | Half. |
| Rule R11 (`recovery/catalog.go:165-198`) | Matches `agent_name_taken` plus a live iteration. Launches disabled. Comment says "herdr refused the taken name". | No. Reword. |
| Rule R12 (`recovery/catalog.go:206-240`) | Stalled reclaim. Launches disabled. Herdr wording only. | No. Reword. |
| Rule R5 (`recovery/catalog.go:389-415`) | `agent_prompt_stalled`, PTY too. Enabled. | No. |
| Rule R7 (`recovery/catalog.go:413-415,431`) | `compactWaitTimedOutRE` matches `"code":"timeout"` (herdr JSON) or `timed out`. Only the first alternative is herdr's. | Regex half. |
| Pause blocked-pane / R4 | PTY pauses on it too (map note). | Keep. |

Conclusion for the spec: the enum change is smaller than ticket 01 assumed. The only kind that can leave `events.kinds` and the schema is `agent_pane_busy` (plus rule R6's `r6PaneBusy` entry). `agent_name_taken` and `agent_prompt_stalled` must stay. The herdr-outage fold goes (`CauseHerdr`, `pauseFold`, `FlushPauseFold`, the digest message at `ralphloop/notify/chat_message.go:82-90`), but the kinds it matched stay. Event types `herdr-unavailable`/`herdr-available`, the marker `herdr-agent-lost` and the wire reason and verdict leave.

## Q2. How old data loads and what a migration needs

- Ticket frontmatter. `PauseReason` is read as a plain string and "never checked by Validate" (`tickets/schema/ticket.go:333-339`, `frontmatter.go:116-117`). An unknown reason does not fail the load. It makes the ticket paused with no class: `PausedClass()` returns `t.PauseReason.Class()` (`tickets/schema/pause.go:221-231`) and `Class()` is `""` for an unknown reason (`:178`). Such a ticket describes itself as plain `paused` and offers no recovery verb (`pause_verbs.go:103-132`) and is neither "asking" nor "for repair". This is the real hazard of removing an enum value: leftover tickets get stuck. Only `agent_pane_busy` is affected (see Q1).
- Run log. `ReadEvents` only skips lines that fail to unmarshal and applies `events.ReadLegacy` (`ralphloop/runlog/eventlog.go:338-343`). It does not validate. Validation is on write only: `encodeEvent` calls `events.Validate` (`:235`). Old lines with a removed kind or type load fine. New code must not try to write one.
- Precedent for removed or renamed kinds: `events/legacy.go:8-20` (`legacyKinds`, `CurrentKind`). `ParsePauseReason` already goes through it (`tickets/schema/pause.go:190-192`). Adding `agent_pane_busy` to that table, mapped to `iteration-error`, fixes both the ticket reader and the run-log reader without touching old files.
- Migration. The schema guard snapshots `schema.Enums()` and fails the build when an enum changes (`migrations/schema_guard_test.go:22-60`, `tickets/schema/enums.go:12-21`, `migrations/testdata/schema.snapshot:95-96`). A new named step goes in `migrations/steps.go` (`init`, `:11-120`). Steps that add enum values are no-ops (`:67-89`). A removal step must rewrite `pause_reason: agent_pane_busy` to `iteration-error` in ticket frontmatter (the step's `Run` returns `[]tickets.MigrationChange`; model it on `pauseUnit`, `:123`). Then update the snapshot with `go test ./migrations -run TestSchemaSnapshot -update-schema-snapshot`. The `pause_kind` enum (`events.KindNames()`) needs the same step.
- A ticket's own log (`.md` event lines) holds the kind only as `pause_kind`/`pause_reason` frontmatter plus free text, so the frontmatter rewrite covers it.
- Run records (`runs.json`) are a separate store. They hold a runner name, not a kind: see Q3.
- Config. `agent-runner: herdr` is validated in three places: `agentrunner.Select`/`ValidateSetting` (`agentrunner/select.go:63,91-99`), `server/settingsapi/apply.go:125`, `server/settings/reload.go:62`. Ticket 01 says herdr reads as `auto` with a warning. The clean fix is one function in `agentrunner` that normalises the setting (herdr becomes auto, plus a "deprecated" flag the caller logs). Select, ValidateSetting and both callers then share it. If `ValidateSetting` just drops `herdr` from its list, config reload and the settings API would reject an old config while startup accepts it. The enum offered to the settings UI (`config/registry/entries_agents.go:13-14,63`, `docs/config-schema.json:11-14`) drops `herdr`.

## Q3. Gone-agent path for a run whose runner no longer resolves

A run record keeps `Runner` and `Session` (`server/runs/registry.go:26-33`). An empty runner is read as herdr in four places: `registry.go:182` (legacy top-level `pane` field), `server/harness/runnerselect.go:34,50`, `server/sched/reclaim.go:92,202`, `server/runs/registry.go:46`.

What happens to such a run after removal:

- At restart, `ReclaimRuns` calls `reclaim` (`server/sched/reclaim.go:80-90,198-245`). `RunnerFor` returns nil (`server/harness/runnerselect.go:31-40,90-103`), so `reclaim` returns `errHandleMismatch` ("started by herdr; herdr unavailable", `:204-206`). `pauseUnreclaimed` pauses the ticket with kind `handle-mismatch` (`:104-122`). The ticket becomes a repair pause whose verbs are Retry and Start over (`tickets/schema/pause_verbs.go:150,177`). It does not start fresh by itself.
- Only a run the runner can look up and finds gone, with an empty claim (no commits, no edits), is reopened (`reopenEmptyClaim`, `reclaim.go:284-317`, logged as `claim-reopened`). With an unresolvable runner, `reclaim` returns before it reaches that code (`:204-206`), so even an empty claim is paused, not reopened.
- During normal running, the other callers treat a nil runner as "not live" and carry on (`server/merge/latefinish.go:163-166`, `server/merge/worktree_removal.go:124-130`, `server/queue/queueagents.go:50-53`, `server/health/healthowners.go:57-61`). But the verb paths error: `Harness.RunnerOf` returns `started by herdr; herdr unavailable` (`runnerselect.go:43-52`), and `Ops.AgentGone` returns `(false, err)` on it (`server/ticketops/answer.go:278-290`). So answer and resume on such a ticket fail instead of treating the agent as gone.

So ticket 01's decision ("take the gone-agent path: a fresh start on the configured runner from the handoff") needs code. Today the gone-agent path is `reopenEmptyClaim` or a `handle-mismatch` pause, and only after a successful runner lookup. The clean fix: when the recorded runner name is not buildable, `reclaim` treats the agent as gone right away (skip the health check and `Find`, go to the `!found` branch: `reopenEmptyClaim`, else the pause). In `RunnerOf`/`AgentGone`, an unbuildable runner means "gone", not an error. `RunnerHerdr` and the empty-name-is-herdr default are then deleted. After removal every launch records its runner (`server/sched/launch.go:239`), so an empty name can only be an old record.

Decision for the map (not decided in 01): is `handle-mismatch` plus Start over enough for a non-empty herdr claim, or must the server restart the iteration by itself? The first needs no new behaviour. The second needs new code that discards a worktree that holds work, which `reopenEmptyClaim` deliberately never does.

## Q4. Test files: incidental versus testing herdr

`herdrfake` is imported by 76 Go files (92 mention it). `WithHerdr`/`HerdrIsTheSubject` appear in 18 files. About 60 further test files mention herdr only as a runner name, a fixture string or a comment.

Incidental (the fake is only wired in through the shared harness, nothing herdr-specific is tested). 51 files, listed by directory:

- `TestMain` hook only (`herdrfake.RunHelperProcess()`): `cmd/cmdtest/main.go`, `cmd/rest/main_test.go`, `ralphloop/supervise/startprompt/main_test.go`, `ralphloop/testhelpers_test.go`, `server/{archive,budget,cleanup,designs,diff,disk,harness,health,lifecycle,limits,projects,queue,recovery,rest,schedules,settings,settingsapi,stats,ticketops,ticketwrite,tips,web,webauth,workspaces}/main_test.go`, `server/epics/main_test.go`, `server/epics/epicflows/main_test.go`, `server/merge/main_test.go`, `server/merge/mergeflows/main_test.go`, `server/sched/main_test.go`, `server/sched/launchtest/main_test.go`, `server/serverflows/main_test.go`, `server/sessions/prompt/main_test.go`, `server/sessions/prompt/promptflows/main_test.go`, `server/server_test.go`, `server/prflow/watcher_test.go`, `server/triage/events_test.go`, `server/servertest/ptyrunner_test.go`, `server/servertest/rigs/prflow_test.go`, `server/sessions/helpers_test.go`, `server/sessions/askflows/helpers_test.go`, `server/sessions/banners/helpers_test.go`, `server/sessions/convflows/helpers_test.go`, `server/sessions/fixagent/helpers_test.go`, `server/sessions/modelflows/models_test.go`, `testmainguard/testmainguard_test.go` (also the guard's own logic, `:21,49,93,102`).
- The hook exists because the harness re-runs the test binary as the fake herdr executable (`testmainguard/testmainguard_test.go:1-4`, `.ai/index.md:90`). Without herdrfake in the harness, all of these lose the call, and the guard checks only `RunSessionProcesses()`.
- Incidental harness code: `server/servertest/{servertest,start,turn,runner,leftover,pausedrun,ptyrunner,ticketwrite}.go` and `server/servertest/rigs/{answerenv,blockedclaimed}.go`. `start.go:78-99,120,164-174` always starts a fake herdr first on PATH. `turn.go:231-349` registers herdr commands for `Harness.OnTurn` on herdr. These are rewrites, not deletes (see table).
- Also incidental: `envguard/envguard_test.go:80` and `sleepguard/sleepguard_test.go:28,86,163` allow-list herdr files by path. Remove entries for deleted files.

Test herdr behaviour (delete with the runner):

- `agentrunner/herdrrunner/*_test.go` (conformance, interrupt, launch, launch_vim, nudge, owner, ratelimit, workspace, `export_test.go`).
- `testutil/herdrfake/*_test.go` (agentkeys, claudecompact, codexrollout, forbid, herdrfake, state).
- `e2e/*.go` (8 files, 1320 lines: agents_terminal, compaction_false_idle, context_menu, gx_worktrees_herdr, idle_while_working, main, server_lifecycle, server_tui). Ticket 03 covers what to port.
- `server/sched/herdrwatch_test.go`, `server/servertest/herdrforbid_test.go`, `server/sched/claimhold_test.go` (47 mentions: herdr outage and claim hold), `server/sched/pause_markers_test.go` (herdr-lost marker), `server/sched/launchtest/reclaim_route_test.go` (herdr vs headless reclaim routing, imports `herdr`).
- `server/server_test.go:300,357,397`, `server/server_notify_test.go`, `server/configreload_chat_test.go`: tests built on `StartHerdrDownWithStore`, `HerdrRetryInterval` and `herdrfake.NewState`. Delete the herdr-outage cases; keep the rest of each file on PTY.
- `ralphloop/supervise/waitforfinish_production_test.go` and `waitforfinish_production_recovery_test.go` (70 herdrfake calls): they drive the production wait loop through the fake herdr to pin herdr's idle/done status quirks (`ralphloop/supervise/waitforfinish.go:29,224,551,635,674`, `waitforfinish_recovery.go:92-94`). These need a case-by-case call: keep each assertion only if a PTY or runnerfake test does not already cover it.

Rewrite (they test shared behaviour but script it through the herdr fake or the `"herdr"` runner name; move to `runnerfake` or PTY):

- `server/sched`: `finishedclaims_test`, `reads_test`, `runner_test`, `runner_flush_test`, `finish_test`, `ratelimit_claim_hold_test`, `headless_block_test`, `slot_release_test`, `sharedhelpers_test`, `verdicts_epic_test`, `waiting_verdict_test`.
- `server/sched/launchtest`: `claim_resume_test`, `realgit_launch_test`, `reattach_test`, `reclaim_launching_test`, `reclaim_reopen_test`, `sharedhelpers_test`.
- `server/budget/limits_test.go` (21 herdrfake calls), `server/recovery/{recovery_runner,recovery_gates}_test`, `server/merge/mergerecover_test`, `server/queue/{oneoffs_epic,queueagents}_test`, `server/runs/load_test`, `server/harness/launch_runner_test`, `server/{prune_route,runner_route,sharedhelpers}_test`, `server/serverflows/servertest_runner_options_test`, `server/schedules/scheduler_rules_test`, `server/epics/epicflows/launchstep_test`, `server/sessions/retry_prompt_test`, `server/sessions/askflows/permission_card_test`, `server/ticketops/{agentanswer,sharedhelpers}_test`, `server/ticketwrite/{blocked_pause,writes}_test`.
- `agentrunner/select_test`, `runnerbuild/runnerbuild_test`, `cmd/claude/claude_doctor_test`, `cmd/serverapi/server_repair_test`, `cmd/serverctl/server_preflight_test`, `ui/app/title_row_test`.
- `ralphloop`: `lifecycle_ratelimit_test`, `lifecycle_wait_test`, `run_realgit_helpers_test.go`, `internal/fixture/fixture.go`, `deps/deps_test.go`.

Keep (herdr as a multiplexer): `ui/terminal_test.go`, `ui/terminalrun/terminalrun_test.go`, `cmd/term/term_test.go`, and the `herdr/` package tests for the kept calls (`workspace_test`, `herdr_test`, `smoke_test`, `main_test`). Delete the `herdr/` tests for deleted calls (`agent_test`, `tab_test`, `worktree_test`, `pane_test`). `herdrfake.go` (253 lines: `StartEnv`, `Result`, `CommandError`, `RunHelperProcess`) must survive for the `herdr/` tests, or those tests need a smaller fake.

## Delete, rewrite or keep, by package

Source files are listed by name. Test files are grouped as in Q4.

### Delete

| Path | What | Evidence |
|---|---|---|
| `agentrunner/herdrrunner/` (15 files, 4,330 lines incl. tests) | The runner, hosting, leftovers, nudge, owner, ratelimit. | `herdrrunner.go`, `hosting.go:58-66`; ticket `gx:follow-ups-2026-11-01/05` targets `leftovers.go` |
| `testutil/herdrfake/` (13 files) | Fake herdr state machine. Keep a slimmed `herdrfake.go` only if `herdr/` tests still need it. | `herdr/main_test.go`, `herdr/smoke_test.go` |
| `testutil/herdrctl/herdrctl.go` | Real-herdr test driver for `e2e/`. | `testutil/herdrctl/herdrctl.go:167,283` |
| `e2e/` (8 files) | The herdr e2e suite. | `Makefile:49-52` |
| `.github/workflows/ci.yml` job `herdr-e2e` (`:98-145`), `Makefile` target `e2e` (`:1,44-52`) | CI job and make target. | |
| `server/herdrwire.go` (58 lines) | Lists herdr-hosted addresses for the banner. | `server/herdrwire.go:53,57` |
| `server/sched/herdrwatch.go` (196 lines) | Health probe, banner, hold, `HerdrWatch`. | `server/server.go:141,571,581,595,674-675,743`, `server/rootroutes.go:52,184`, `server/sched/claim.go:42`, `server/sched/explainer.go:34,118-119`, `server/sched/scheduler.go:39` |
| `server/sched/pausemarker.go` herdr-lost branch; `events.HerdrLostMarker`; pause wording `server/ticketwrite/pause_chat.go:34` | | `server/sched/pausemarker.go:16-26` |
| `server/ticketwrite/pause_fold.go` and its hooks | The herdr-outage digest. | `pause_fold.go:8-11`, `writer.go:48-59,128`, `server/ticketwritewire.go:41,55`, `ralphloop/notify/chat_message.go:82-90` |
| `events`: `HerdrUnavailable`, `HerdrAvailable`, `kindCauseHerdr`, `CauseHerdr()`, kind `AgentPaneBusy` | | `events/events.go:122-123,153,229-234,287-288` |
| `server/wire`: `ReasonHerdrUnavailable`, `VerdictHerdrDown`, `EventHerdrUnavailable/Available`, `HerdrUnavailable`/`HerdrText` fields | Regenerate `webui/src/generated/server.ts`. | `server/wire/misc.go:116,132-148`, `server/wire/server.go:15-18`, `server/wire/index.go:90-94` |
| `viewmodel/viewmodel.go:46-49,177-178,219-228`, `ui/app/server_indicator.go:20,51-52`, `ui/app/server_conn.go:93-94`, `ui/icons.go:85,150,212`, `ui/tickets/queue/{server,state,model}.go`, `ui/tickets/queue/header/header.go:116-117,425-428,461-463` | Banner and indicator for "herdr unavailable". | |
| `ui/tickets/rowactions`: `server_answer.go` (herdr `TabFocus`, `focusIterationTab`, `cmdServerFocusPane`), `ticket_menu.go` herdr-down state and "Go to pane" item, `watch_agent.go` `HerdrLookup`/`serverHerdrLookup`, `menus.go:111,186` | Go to pane on a herdr tab. | `server_answer.go:17-19,84`, `ticket_menu.go:152,160` |
| `agentlog/lookup.go` `HerdrIteration`, `Herdr` hook, `ErrUnsupported`; `cmd/serverapi/server_agents.go:115,182-198` | Locating a live herdr iteration's log. | `agentlog/lookup.go:58-77,127-146` |
| `cmd/doctor/doctor.go` `--check-blocked-form`, `ExplainAgent`; `docs/runbooks/blocked-form-regression-check.md`; `ralphloop/supervise/waitforfinish_recovery.go:92-94` note | | `doctor.go:28-29,42,57,191-224`; also `cmd/doctor/doctor_test.go` |
| `server/servertest`: `HerdrReason`, `WithHerdr`, `StartHerdrDown*`, `SetHerdrDown`, `LeaveHerdrRun`, `HerdrNamed`, `AgentNotFound`; `rigs.HerdrHosted`, `rigs.RegisterLiveAgent` | | `server/servertest/runner.go:11-74`, `start.go:27-38,61-99,164-174`, `turn.go:216-349`, `leftover.go:90`, `ticketwrite.go:35-38,73-74`, `rigs/blockedclaimed.go:18-21,45-53`, `rigs/answerenv.go:132-141` |
| `testutil/agentfake`: `cmd/agentfake`, `cmd/agentfakeworker`, `agentfake.go` (`ParseArgs`, `Run`, `Options`) | The fake `claude` binary a real herdr execs in e2e. Verify `RunWorker` (`worker.go:331`) has no other caller before deleting. `WorkerConfig` and the rest of `worker.go` stay (used by gxfixture, `codex/`, `claudedoctor`). | `agentfake.go:3-14`, `cmd/agentfake/main.go:15-20`, `cmd/agentfakeworker/main.go:15` |
| `cmd/serverctl/server_launchd.go:60` `HERDR_SOCKET_PATH` in the launchd env keys; `cmd/serverctl/server_preflight.go` `PingHerdr` (see Keep for `Ping`) | The server no longer talks to herdr. | |
| `testutil/gxfixture`: `DropAskedAgent` scenarios and the `keepWorktree`/`answered` herdr-cleanup variants; `server.Config.DropAskedAgent`; `cmd/serverctl/drop_asked_agent_fixture.go` | A test-only switch that forces the herdr-hosted cleanup. With no herdr, `keepsAskedAgent` (`server/sched/run.go:581-590`) is true for every Claude run and the switch has no meaning. | `server/config.go:89-92`, `server/server.go:591`, `testutil/gxfixture/spec/{answered,keepworktree,spec}.go`, `testutil/gxfixture/startup.go:146` |
| `scripts/test.sh:12,40-41` | Entries for the two deleted packages. | |

### Rewrite

| Path | Change |
|---|---|
| `agentrunner/select.go` and `agentrunner.go` | Remove `ChoiceHerdr`, `HerdrPingTimeout`, `Probe.PingHerdr`, `herdrUp`, the "(or set agent-runner: herdr)" hint (`select.go:13,19-20,30-31,56,73-77,95-98`). Add the one normaliser (herdr reads as auto plus a warning). Reword comments at `agentrunner.go:2,39,59,281,293`. |
| `runnerbuild/runnerbuild.go:11,13,65-66` | Drop the herdr case. |
| `server/runs/registry.go:29,44-52,182` | Delete `RunnerHerdr` and the legacy `pane` shim. |
| `server/harness/runnerselect.go:20-50` | `LaunchRunner` no longer branches on herdr (Codex always goes to PTY on a native runner). `RunnerForRun`/`RunnerOf` stop defaulting to herdr. See Q3 for the unbuildable-runner rule. |
| `server/sched/reclaim.go:92,196-245` | Remove the herdr arm. `reclaim` becomes the headless/PTY path (`reclaimHeadless`) for every run, plus the Q3 rule. |
| `server/sched/iterations.go:10,58-75`, `server/sched/run.go:80,420,581-590`, `server/sched/launch.go:150,337` | Drop herdr `done` special-case, the `herdr.CLI` tab lookup, and the herdr-hosted branch of `keepsAskedAgent`. |
| `server/merge/worktree_removal.go:130`, `server/epics/detail.go:260-276`, `server/ticketops/agentverbs.go:79-80`, `server/ticketops/repair.go:617` | Remove the `runner.Name() == herdr` branches ("a herdr-hosted agent has no approval card"). |
| `server/server.go:29,141,546-548,571-595,674-675,743` | Remove the `herdrrunner.Runner` finish hook and `HerdrWatch` wiring. `server/config.go:73,125` (`HerdrRetryInterval`, the "nil means herdr" comment). |
| `ralphloop/deps/deps.go:15,20,55-66,253-296,560-581` | `DefaultDepsWithOverrides` builds a `herdrrunner` as `Deps.Runner` (`:296-305`); remove it and `DepsOverrides.Finished`. `preflightAgentWith` checks herdr's Codex support (`:560-581`); `Deps.PreflightAgent` has no non-test caller (`rg PreflightAgent`), so delete it. |
| `ralphloop/supervise/{waitforfinish,launch,waitforfinish_recovery,startprompt}`, `ralphloop/{launch,finish,conflict,lifecycle}.go`, `ralphloop/runlog/{eventlog,eventsink}.go`, `ralphloop/worktree/labels.go`, `ralphloop/agent/agent.go`, `ralphloop/verify/verify.go`, `ralphloop/notify/server_chat.go` | Comment-level herdr references and the herdr-status workarounds noted in the tests. Check each workaround against PTY before deleting logic. |
| `recovery/catalog.go` | Delete `r6PaneBusy`. Reword R6 (name taken), R11, R12 and R7's `"code":"timeout"` branch (`:80-110,165,259-285,413`). |
| `events/events.go:152-156,238-248` | Remove `AgentPaneBusy` from the enum. Add it to `legacyKinds` (`events/legacy.go:15`). Reword the `AgentPromptStalled` comment. |
| `tickets/schema/pause.go`, `pause_verbs.go` | Remove `PauseAgentPaneBusy`. New step in `migrations/steps.go`, snapshot update. |
| `config/registry/entries_agents.go:14,63`, `config/config.go:55`, `docs/config-schema.json:11-14`, `server/settingsapi/apply.go:125`, `server/settings/reload.go:62` | Drop `herdr` from the enum and text; use the normaliser. |
| `cmd/claude/claude_doctor.go:39,173-180` | "doctor never checks herdr" wording; `auto` handling. |
| `cmd/tui/ui.go:331-342`, `ui/settings.go:14-18`, `ui/worktrees/{modal,model_keys,model_update}.go` | `HerdrUnavailable` now means only "the herdr multiplexer is not answering". Rename or reword. |
| `harness/{systemprompt,codex}.go`, `nativerunner/{nativerunner,pty}.go`, `agentrunner/claudeui/nudge.go:26`, `server/budget/budget.go:224`, `transcript/transcript.go`, `cpuwatch/cpuwatch.go`, `testutil/gxfixture/{env,catalog,spec}`, `testutil/agentfake/*`, `ralphloop/internal/{fakes,fixture}`, `server/sched/launch.go`, `cmd/cmdtest/main.go` | Comments and the `HERDR_` env filter (`gxfixture/env.go:53`). `cmd/cmdtest/main.go:8-25` drops `herdrfake.RunHelperProcess()`. |
| `agentrunner/imports_test.go:10-12` | Shrink `herdrImporters` to the kept importers (`ui/worktrees/launch`, `cmd/serverctl` or `cmd/tui` for `Ping`). |
| `testmainguard`, `envguard`, `sleepguard` | Drop the `RunHelperProcess(` hook (`testmainguard_test.go:21,49,93,102`) and path entries for deleted files. |
| Tests | Q4's rewrite list: move them off `WithHerdr`/`herdrfake` to `runnerfake` or the PTY rig. `.ai/server-test-rigs.md:16-17` says `OnTurn` needs "PTY or `WithHerdr`". |

### Keep

| Path | Why |
|---|---|
| `ui/terminal.go:16-56` (`TerminalHerdr`, `$HERDR_ENV`) | Terminal detection. |
| `ui/terminalrun/terminalrun.go:103-396` | Split launch. It shells out to `herdr` directly (`runCommand("herdr", ...)`), not through the `herdr/` package. |
| `ui/worktrees/launch/herdr.go`, `ui/worktrees/modal/terminal_menu.go:116-126` | "Open herdr workspace" and splits. |
| `cmd/term/term.go:63-68` | `gx term` split launch. |
| `cmd/doctor/doctor.go:300` | Prints `HERDR_ENV` in diagnostics. Check whether doctor still needs it. |
| `herdr/` package, reduced | `herdr.go` (`CLI`, `run`, `runJSON`, `lookPath`, `binary`, `appendFlag`), `workspace.go` (`FindWorkspace`, `FindOrCreateWorkspace`, `Ping`), `default_cli.go` wrappers for those. Consumers: `ui/worktrees/launch/herdr.go:13` (`FindOrCreateWorkspace`) and `cmd/serverctl/server_preflight.go:21` + `cmd/tui/ui.go:333-337` (`Ping`, for the TUI's "herdr down" menu gate). Delete `agent.go`, `tab.go`, `pane.go`, `worktree.go`, `EnsureWorkspace`, the typed agent errors (`herdr.go:79-148`) and the matching tests. `TabFocus` is used only by `rowactions/server_answer.go:19`, which goes. |
| `GLOSSARY.md` **Split launch** (`:962`) | Keep herdr as a multiplexer. |
| Fixture data that only quotes the person's `herdr` skill (`codexdoctor/testdata/rollouts/*.jsonl`, `chatreader/testdata/*.jsonl`, `commandlist/testdata/codex-skills-list.jsonl`, `docs/specs/codex-support-fixtures/rollouts/*.jsonl`), `sound/gen_done.py:19`, `CHANGELOG.md` | Not about the runner, or history. |
| `docs/adr`, `docs/specs`, `docs/research`, `docs/plans` | History stays (ticket 01). Add "Superseded by" lines only where noted below. |

### Docs and skills

| Path | Action |
|---|---|
| New ADR `docs/adr/0048-...` | Records the removal. Latest ADR is 0047. |
| ADRs 0031, 0043, 0044 | Add a "Superseded by" line. These are the ADRs that describe herdr as the alternative runner: 0043:10-22 (`agent-runner: herdr` brings herdr back, a Codex ticket "runs on herdr"), 0044:4-15 (Codex on herdr), 0031:6-34 (why native runners exist). 0017, 0019, 0023, 0037 and 0042 only mention herdr in passing; leave them. No ADR is named after the herdr runner, so ticket 01's "herdr-runner ADRs" means these three. |
| `GLOSSARY.md` | Rewrite or drop: `:269-274` (herdr `agent_status`), `:358` ("herdr is one agent runner"), `:364-365`, `:377`, `:379-380` (Session replaces herdr's tab/pane/workspace), `:585` (`herdr unavailable` indicator), `:590`, `:601`, `:1233-1234` ("herdr support is being phased out"). Keep `:962`. |
| `README.md:73,351,372` | `:73` calls herdr "an" orchestrator agent host; `:351` describes "under `agent-runner: herdr` a reduced flow". Rewrite. `:255-266` (split launch) stays. |
| `.ai/index.md:90,269,294,316,322,326-327` and `.ai/server-test-rigs.md:16-17` | Rewrite: `herdr/` description, the `server/sched` note on `HerdrWatch`, "e2e/ — CI only", the `herdrfake`/`herdrctl` lines, the `TestMain` hook sentence. Also `.ai/index.md:68-69` (`go test ./e2e/...`). |
| `skills/gx-investigate/SKILL.md:25,64-66,99,113-115,290` and `gotchas.md` (about 20 entries: `:68,90,96,104-113,130-131,143,153-157,294-300,316,320`) | The skill reads as if herdr hosts agents. Rewrite the overview. Remove or recast the herdr-only gotchas (`agent_pane_busy`, `herdr agent wait`, `TabCreate`, the 32-char name cap). Keep ones that describe shared behaviour, with PTY wording. |
| `skills/gx-local-tracker.md:537`, `skills/gx-to-tickets/SKILL.md:123` | Reword the "herdr reporting the pane as blocked" line. `:123` only quotes a ticket id; leave it. |
| `skills/bundle_test.go:377` | Comment about herdr's `agent_status`; check after the GLOSSARY rewrite. |
| `docs/config-schema.json`, `docs/runbooks/blocked-form-regression-check.md` | Regenerate the schema; delete the runbook with the doctor check. `docs/runbooks/codex-smoke-checklist.md` does not mention herdr. |
| `docs/ralph-loop-queue-orchestrator.md:24,33,47,60-62,84,110` | Describes the herdr tab as the iteration host. Rewrite or mark historical. |
| `docs/specs/` (39 files, plus 5 rollout fixtures), `docs/research/` (13), `docs/plans/` (8, including `herdr-e2e-tests.md` and `26-fake-herdr-codex-rollouts.md`) | History. Leave as is. Only `docs/specs/pty-as-default.md` and `docs/specs/agent-runner.md` are worth a one-line pointer to the new ADR. |

## Existing follow-up tickets about herdr leftovers

- `gx:follow-ups-2026-11-01/05` ("Replace loose Procs and Signal fields with a procOps interface"): about `agentrunner/herdrrunner/leftovers.go`. Open. Obsolete when the runner goes; retire it with the impl epic.
- `gx:follow-ups-2026-11-01/04` ("Test start-over for a finished agent with a leftover task"): from the same epic. It names the herdr-runner status test `TestStatus_FinishedAgentIgnoresBackgroundTasks` but asks for a start-over test, which is runner-independent. Keep it.
- `gx:follow-ups-2026-11-01/30` ("Use agentrunner.Choice everywhere"): names `runnerHerdr` in `server/runs.go`. The impl epic removes that constant, so fold this item's herdr part into the epic.
- Archived herdr-related follow-ups under `follow-ups-archived/` (for example `53`, `90`, `140`, `366`, `377`) are closed. No action.

## Design questions the findings raise

1. Ticket 01 says the herdr-only kinds `agent_name_taken` and `agent_prompt_stalled` leave the enums. They do not: PTY and headless raise both. Only `agent_pane_busy` can go (Q1). The map's decision should say so.
2. Ticket 01 says leftover herdr runs "take the existing gone-agent path". That path does not yet apply to an unbuildable runner (Q3). The impl epic needs the `reclaim`/`RunnerOf`/`AgentGone` change, and the map should decide whether a non-empty herdr claim ends in a `handle-mismatch` pause (no new behaviour) or an automatic restart.
3. Where does `herdr` get mapped to `auto`? Recommend one normaliser in `agentrunner`, shared by `Select`, `ValidateSetting`, `settingsapi` and `settings/reload` (Q2).
4. Does `herdrfake.go` stay, shrunk, for the `herdr/` package tests, or do those tests use a smaller fake? That decides whether `RunHelperProcess()` stays in any `TestMain`.
5. `Ping` and the TUI's `HerdrUnavailable` gate stay for the multiplexer menu. Confirm that is wanted, or drop the gate and let the launch fail with its own error.
