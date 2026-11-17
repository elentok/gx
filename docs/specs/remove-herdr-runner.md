# Remove the herdr runner

Epic: `remove-herdr-runner-impl`. Source map: `gx:remove-herdr-runner`. Research:
`docs/research/remove-herdr-runner-02-herdr-reference-inventory.md` and
`docs/research/remove-herdr-runner-03-tui-e2e-coverage-gaps.md`.

## Problem Statement

The person no longer runs agents on the herdr runner. It is still in the code. Its tests (about 30
files, plus a herdr e2e suite and a CI job) cost CI time. Its code paths, pause kinds, banners and
docs make investigations harder: the docs and the `gx-investigate` skill read as if herdr hosts
agents, and a reader has to work out which herdr mentions are live. herdr is also still used for
something else, as a terminal multiplexer (TUI split launch, the worktrees tab's "open herdr
workspace"), and that use must keep working.

## Solution

Remove the `agent-runner: herdr` choice and everything that exists only for it. Keep herdr as a
terminal multiplexer, with the `herdr/` package shrunk to what that needs. The PTY runner is the
default, and the docs and skills say so.

- An old config that says `agent-runner: herdr` is read as `auto`, with a warning.
- An old run that names herdr is treated as an agent that is gone. An empty claim reopens. A claim
  that holds work pauses as `handle-mismatch` with a reason that names the removed runner, and the
  person ends it with Retry or Start over.
- The herdr e2e suite (`e2e/`), the `herdr-e2e` CI job and `make e2e` go. The TUI behaviour that
  only that suite covered gets herdr-free tests: in-process ports, plus a small real-binary suite on
  the PTY runner that runs in `make test`.
- One new ADR records the removal and supersedes ADRs 0031, 0043 and 0044.

## User Stories

1. As a gx maintainer, I want the herdr runner code deleted, so that an investigation never has to
   rule out a runner nobody uses.
2. As a gx maintainer, I want the herdr-only tests, fakes and e2e suite deleted, so that CI is
   faster and the suite has no herdr-only failure modes.
3. As a person with `agent-runner: herdr` still in my config, I want the server to start and treat
   it as `auto`, so that an upgrade does not break my setup.
4. As a person with that config, I want a warning that says the herdr runner was removed, so that I
   know to edit my config.
5. As a person with that config, I want the settings API and the settings reload to accept it the
   same way startup does, so that a reload never rejects a config that startup accepted. The API
   stores a `herdr` write as `auto`.
6. As a person opening the settings page, I want the `agent-runner` choices to list no herdr, so
   that I cannot pick a removed runner. With `agent-runner: herdr` still stored, the page shows
   `auto` and the same warning as the log.
7. As a person whose server restarts with a ticket claimed on herdr and no work in the worktree, I
   want the claim reopened (`claim-reopened`) and started fresh on the configured runner, so that
   nothing is stuck.
8. As a person whose server restarts with a ticket claimed on herdr whose worktree holds commits or
   changes, I want a `handle-mismatch` pause with a reason that names the removed herdr runner and
   says Retry or Start over ends it, so that I know what happened and what to do.
9. As a person with such a paused ticket, I want Retry and Start over on the web, so that I can
   recover without the CLI (ADR 0042).
10. As a person answering or resuming a ticket whose recorded runner no longer exists, I want the
    agent treated as gone, not an error, so that answer and resume keep working.
11. As a person with old run records that have no runner field, I want them read as an agent that
    is gone, not as herdr, so that no code keeps a herdr default.
12. As a person with a ticket paused with `agent_pane_busy`, I want it rewritten to
    `iteration-error` by a migration, so that it keeps its recovery verbs and does not show as
    "paused with no class".
13. As a person reading old run logs, I want lines that hold `agent_pane_busy`, `herdr-unavailable`,
    `herdr-available` or `herdr-agent-lost` to still load, so that history is not lost.
14. As a person using the web UI or the TUI, I want no "herdr unavailable" banner or indicator on
    the server, so that the UI shows only things that exist.
15. As a person with PTY-hosted agents, I want `agent_name_taken`, `agent_prompt_stalled` and
    `handle-mismatch` to keep working as today, so that the shared pause kinds are untouched.
16. As a maintainer reading the recovery rules, I want the herdr wording removed from rules R6, R7,
    R11 and R12, and the pane-busy entry of R6 deleted, so that the rules describe only live
    behaviour.
17. As a person using `gx doctor`, I want `--check-blocked-form` gone, so that the CLI has no
    herdr-only check. It still prints `HERDR_ENV` in diagnostics, because that is multiplexer
    information.
18. As a person who runs gx inside herdr, I want TUI split launch (`gx term`) to work as before, so
    that removing the runner does not touch the multiplexer.
19. As a person on the worktrees tab, I want "open herdr workspace" to work as before, so that the
    kept multiplexer feature is intact.
20. As a person whose herdr daemon is not answering, I want the TUI multiplexer menu to say so, so
    that I get a clear message and not herdr's own error. The state means only "the herdr
    multiplexer is not answering".
21. As a maintainer, I want the `herdr/` package to keep only `CLI`, `FindOrCreateWorkspace` and
    `Ping`, so that no code remains that nothing calls.
22. As a maintainer, I want `testutil/herdrfake` deleted and the `herdr/` tests to use a small
    shell-script stub on `PATH`, so that no test binary re-runs itself as a fake herdr.
23. As a test author, I want `herdrfake.RunHelperProcess()` gone from every `TestMain` and the
    testmainguard check for it removed, so that new test packages need no herdr boilerplate.
24. As a test author, I want the shared test rigs (`servertest`, `rigs`, `launchtest`) to have no
    herdr option, so that every rig runs on PTY or `runnerfake`.
25. As a maintainer, I want the tests that script shared behaviour through the herdr fake moved to
    `runnerfake` or the PTY rig, so that the behaviour stays covered without herdr.
26. As a maintainer, I want the wait-loop code kept as it is (none of it is herdr-only) with its six
    herdr comments reworded, and the five herdr-based production tests deleted or ported as listed
    in Implementation Decisions, so that coverage does not drop.
27. As a maintainer, I want the `herdr-e2e` CI job, `make e2e` and the `e2e/` suite removed, so that
    CI has no herdr-only job.
28. As a maintainer, I want the TUI behaviour that only the e2e suite covered tested without herdr,
    so that deleting the suite loses no coverage.
29. As a maintainer, I want a small real-binary suite (2 or 3 tests) on the PTY runner in `make
    test` and normal CI, so that "server mode shown, dropped on stop" and shifted letters through the
    real bubbletea decode stay covered.
30. As a test author, I want the `vtTerm` rig in `testutil/vtterm`, so that any suite can drive the
    real TUI in a terminal emulator.
31. As a maintainer, I want `agentsui.NewModel` to take a `WithDialer` option, so that a test can
    check that attaching after a tab switch dials the fake holder.
32. As a maintainer, I want the three config-validation sites to share one normaliser, so that
    herdr-to-auto lives in one place.
33. As a maintainer, I want the ADRs 0031, 0043 and 0044 to carry a "Superseded by" line pointing to
    one new ADR, so that history stays and readers find the current decision.
34. As a reader of `GLOSSARY.md`, I want the herdr runner terms dropped and herdr kept only under
    Split launch, so that the glossary does not say herdr runs agents.
35. As a reader of `.ai/index.md`, `.ai/server-test-rigs.md`, `README.md` and
    `docs/ralph-loop-queue-orchestrator.md`, I want no text that assumes herdr is the runner, so
    that agents working in the repo are not misled.
36. As an investigator using `skills/gx-investigate`, I want the skill to describe PTY-hosted
    agents, with herdr-only gotchas removed and shared ones recast in PTY wording, so that the
    skill matches the system.
37. As a maintainer, I want the herdr-only follow-up `follow-ups-2026-11-01/05` retired and the
    herdr part of `/30` folded into this epic, so that the tracker has no stale work.
38. As a maintainer, I want fixture and test-only switches that exist only for the herdr runner
    (`DropAskedAgent`, `cmd/agentfake*`, `HERDR_SOCKET_PATH` in the launchd env keys) deleted, so
    that no dead switches remain.

## Implementation Decisions

Config and runner selection

- One normaliser lives in `agentrunner`. It maps `herdr` to `auto` and returns a "deprecated" flag
  the caller logs as a warning. `Select`, `ValidateSetting`, the settings API and the settings reload
  all call it. The settings registry enum, `docs/config-schema.json` and the config text drop
  `herdr`, and the schema is regenerated.
- `ChoiceHerdr`, the herdr ping timeout and probe, the "(or set agent-runner: herdr)" hint, and the
  herdr case in the runner builder are deleted.
- The warning is logged once at start and once on each reload that reads `herdr`. Its text is one
  line: `agent-runner: herdr was removed; using auto (PTY). Edit the config to drop it.`
- The settings page shows `auto` for a stored `agent-runner: herdr`, with the same warning. The
  settings API stores a `herdr` write as `auto`.

Leftover runs

- A recorded runner name that cannot be built counts as "agent gone". `reclaim` skips the health
  check and `Find` and goes to the not-found branch. `RunnerOf` and `AgentGone` return "gone", not
  an error, so answer and resume work. The rule applies at every site that resolves a recorded
  runner: `reclaim` (`server/runs.go`), `RunnerOf` callers (`server/ticketops/answer.go`,
  `stoptask.go`, `server/ticketwrite/blocked_pause.go`, `server/runs/stop.go`,
  `server/budget/budget.go`, `server/ticketopswire.go`, `server/ticketwritewire.go`),
  `IterationRunner` callers (`server/ticketwrite/nudge.go`), and `RunnerForRun` callers
  (`server/ticketops/repair.go` `iterationSession`, `server/merge/latefinish.go`,
  `server/merge/worktree_removal.go`, `server/health/healthowners.go`, `stormwatch.go`,
  `server/queue/queueagents.go`). In `iterationSession` an unbuildable recorded runner means no live
  session, as in `latefinish.go`. This rule is general: it is not a herdr special case, and no
  herdr code stays to read old runs.
- `RunnerHerdr`, the rule "empty runner name means herdr" and the legacy top-level `pane` shim in the
  run registry are deleted. After removal every launch records its runner, so an empty name is an old
  record and reads as gone.
- Not-found branch: an empty claim reopens as `claim-reopened` and starts fresh on the configured
  runner. A claim with work pauses as `handle-mismatch`. The pause kind and its verbs (Retry, Start
  over) do not change. Only the reason text changes, to name the removed herdr runner and say Retry
  or Start over ends it. There is no auto-restart code and no code that discards a worktree.
- `reclaim` loses its herdr arm and becomes the headless/PTY path for every run.

Enums, events and migration

- Only `agent_pane_busy` leaves the pause kinds and the ticket pause reasons. A new named migration
  step rewrites `pause_reason: agent_pane_busy` (and the `pause_kind` enum snapshot) to
  `iteration-error`. The schema snapshot is updated. `agent_pane_busy` is also added to the legacy
  kind read-shim mapped to `iteration-error`, so old run logs and the ticket reader still load it.
- Event types `herdr-unavailable` and `herdr-available`, the `herdr-agent-lost` marker, `CauseHerdr`
  and the herdr-outage fold (including its digest message) are deleted. Old log lines that hold them
  still load, because readers do not validate kinds. New code never writes one.
- `agent_name_taken`, `agent_prompt_stalled`, `handle-mismatch` and `blocked-pane` stay. So do
  `stale-pane-closed`, `claim-reopened`, `late-finish-adopted` and `reclaimed`.
- The wire reason `herdr-unavailable`, the verdict `herdr down`, and the snapshot fields
  `herdr_unavailable` and `herdr_text` are deleted. The generated web client types are regenerated.
  No web UI code reads them.
- Recovery rules: delete the pane-busy entry of R6. Reword R6 (name taken), R11, R12 and the herdr
  `"code":"timeout"` branch of R7's match to describe the shared behaviour.

Server and UI code that goes

- The herdr runner package, the server's herdr health watch and banner, the herdr address listing,
  the herdr finish hook and its wiring, the herdr-hosted branches of `keepsAskedAgent` and of the
  merge, epic, ticket-ops and repair code, the `Deps` herdr runner default and Codex preflight, the
  herdr "Go to pane" and tab-focus code in the ticket row actions, the herdr iteration lookup in
  `agentlog`, and the TUI banner and indicator for "herdr unavailable".
- `gx doctor --check-blocked-form`, `ExplainAgent` and the blocked-form runbook go.
- `DropAskedAgent` (server config, fixture scenarios, serverctl switch) goes: with no herdr,
  `keepsAskedAgent` is true for every Claude run.
- `cmd/agentfake`, `cmd/agentfakeworker` and the fake agent's `ParseArgs`/`Run`/`Options` go. `RunWorker`
  goes too: its only caller, `cmd/agentfakeworker`, is deleted. The rest of the worker config stays
  (gxfixture, Codex and doctor use it).
- `HERDR_SOCKET_PATH` leaves the launchd env keys. The server preflight no longer pings herdr.

What stays (multiplexer)

- Terminal detection, `terminalrun` split launch, `gx term`, the worktrees tab launch and menu item,
  and the `HERDR_ENV` line in `gx doctor` diagnostics.
- The `herdr/` package keeps `CLI`, `FindOrCreateWorkspace` (and `FindWorkspace`) and `Ping`, with
  their wrappers. Everything else (agent, tab, pane, worktree calls, `EnsureWorkspace`, typed agent
  errors, `TabFocus`) is deleted.
- The Ping gate stays in front of the TUI multiplexer menu. The `HerdrUnavailable` state is reworded
  or renamed (implementer picks the name) to mean only "the herdr multiplexer is not answering". The
  import-guard test shrinks to the kept importers.

Test infrastructure

- `testutil/herdrfake` and `testutil/herdrctl` are deleted. `herdr/` tests use a small shell-script
  stub on `PATH`, written in the test's temp dir and kept inside `herdr/`.
- `herdrfake.RunHelperProcess()` leaves all 51 `TestMain` users, and testmainguard stops checking for
  it. envguard and sleepguard drop path entries for deleted files.
- `servertest` loses `WithHerdr`, the herdr-down starters, `LeaveHerdrRun`, `HerdrNamed`,
  `AgentNotFound`, the herdr command registration in `OnTurn`, and the always-on fake herdr in
  start-up. `rigs.HerdrHosted` and `rigs.RegisterLiveAgent` go. Rigs default to PTY, as they do now.
- `scripts/test.sh` drops the two deleted packages.

TUI test ports

- `vtTerm` moves to `testutil/vtterm`. A real-binary suite of 2 or 3 tests runs on the PTY runner
  against a real server: server mode shown and dropped on `gx server stop`; shifted letters through
  the real bubbletea decode; optionally a `gx worktrees` launch smoke. It runs in `make test` and
  normal CI under `scripts/proc-cap`. No replacement CI job.
- In-process ports: holder `TERM`/`COLORTERM` and shifted `H` echo in the holder attach tests; live
  SGR colors, cursor (`View().Cursor`) and shifted-letter forwarding in the agents attach tests;
  attach after switching with state loaded in the app server-stream harness, using the new
  `WithDialer` option (default `terminal.DialHolderAttach`); context menu anchor and hit test in the
  tickets nav tests, plus one app-level case for chrome.
- No port for wheel-as-arrows, wheel to a mouse-reporting agent, the worktrees list, or context menu
  behaviour already covered in-process.

Docs and skills

- New ADR 0048 (0047 is the highest today) records the removal
  and its decisions: herdr config reads as auto, leftover runs are gone agents, enums, multiplexer
  kept. ADRs 0031, 0043 and 0044 get a "Superseded by" line. Other ADRs, specs, research and plans
  are history and stay, except a one-line pointer to the new ADR in `docs/specs/pty-as-default.md`
  and `docs/specs/agent-runner.md`.
- `GLOSSARY.md`: drop the herdr runner terms and the "phased out" lines; keep herdr under Split
  launch. `README.md`, `.ai/index.md` (including the `go test ./e2e/...` mentions and the `herdr/`
  description), `.ai/server-test-rigs.md`, `docs/ralph-loop-queue-orchestrator.md`,
  `skills/gx-investigate` (SKILL.md and gotchas) and the herdr line in `skills/gx-local-tracker.md`
  are rewritten so none assumes herdr is the runner. PTY is the default. Shared-behaviour gotchas in
  gx-investigate are recast in PTY wording; herdr-only ones are removed.
- The person's own `herdr` and `tui-testing-with-herdr` skills in `~/.dotfiles` are not touched.

Tracker

- Retire `gx:follow-ups-2026-11-01/05`. Fold the herdr part of `/30` into this epic. `/04` stays.

## Testing Decisions

- A good test checks external behaviour: the pause a ticket ends in, the event a restart writes, the
  config a reload accepts, the bytes a terminal shows. It does not check which internal function ran.
- Seam: the highest existing one is the server rig (`servertest` on the PTY runner or `runnerfake`)
  for runner, reclaim and pause behaviour. Config and migration tests sit at their package seams
  (`agentrunner` normaliser, `settingsapi`, settings reload, `migrations` step). The TUI uses the
  in-process model rigs and the new `testutil/vtterm` real-binary suite.
- New tests: the normaliser (herdr to auto plus warning, from all three sites); a leftover run with
  runner `herdr` and an empty claim reopens; the same with work pauses as `handle-mismatch` with the
  new reason text; `RunnerOf` and `AgentGone` return gone for an unbuildable runner; the migration
  step rewrites `agent_pane_busy` and the legacy shim loads old log lines; the `herdr/` package tests
  against the shell-script stub; the in-process TUI ports and the real-binary suite listed above.
- Prior art: `server/sched/launchtest/reclaim_reopen_test`, `events/legacy.go` tests,
  `migrations` schema guard, `ui/agents/attach_test.go`, `nativerunner/holder/holderattach_test.go`,
  `ui/app/server_stream_test.go`, and the old `e2e/` `vtTerm` tests.
- Rewrite, do not drop, tests of shared behaviour (list in the inventory, Q4).
- Wait-loop (map ticket 07): no logic in `ralphloop/supervise/waitforfinish.go` or
  `waitforfinish_recovery.go` is herdr-only; all of it runs through the `Runner` interface. Keep it
  all, including the premature-idle-during-compact gate (Claude's pid status can read idle mid
  `/compact` on PTY too). Reword the comments at `waitforfinish.go:29,224,551,635,674` and
  `waitforfinish_recovery.go:92-94` ("herdr" becomes "the runner"; the blocked-pane note names
  claude's pid status `waiting` and points to `gx claude doctor`).
  - Delete `ProductionSlowCompactRegression` (herdr nudge/Enter bug; PTY has `PromptNudged`),
    `ProductionSlowButSuccessfulCompactRegression` (covered by `TranscriptConfirmsLateCompaction`,
    `TimeoutCompletionKeepsTheExpiredEvent`, `TimeoutPathIsNotDoublePaced`) and
    `ProductionPrematureIdlePaneNeverConfirms` (covered by `EscalatesAfterTwoConsecutiveGatedGiveUps`,
    `GatedGiveUpDeniesAPaneIdleToEveryPollKind`, `GenuineStuckCompactFailsAfterExtendedWait`).
  - Port `ProductionCodexBlockedAtCompactSubmission` to `runnerfake`: a Codex agent whose `Prompt`
    fails with a blocked error; assert recovered=false, no error, `SmartZoneRecoveryFailed`, no pause,
    no resume. Drop the port only if the `FailedFinishUpPromptIsBestEffort` test is extended to cover it.
  - Port `ProductionPrematureIdlePaneRecovery` as one end-to-end `WaitForFinish` test on `runnerfake`
    with `BlipRunner`: the boundary arrives late, the finish-up prompt is refused until then, and the
    run ends with `compactGateConfirmed`.
  - Delete the herdrfake `TestMain`, its helpers (`agentResult`, `parseUntil`, `useFakeClock`,
    `writeOccupancyTranscript`, `appendCompactBoundaryLine`, `withProcessEnv`) and `herdrfake.ClaudeCompact`.
- Run tests capped with `scripts/proc-cap`.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture`, PTY runner, fakeclaude) after
the epic is built. This epic is mostly internal. Web flows are checked by running the existing
catalog flows to prove nothing regressed; the new behaviour is checked with CLI, server and test
checks below. No stored mock exists, so there is no Mock states list.

1. Code deleted. Flows: none. Checks: `rg -il 'herdrrunner|herdrwatch|herdrfake|herdrctl'` over the
   repo, excluding `docs/adr`, `docs/specs`, `docs/research`, `docs/plans` and `CHANGELOG.md`, prints
   nothing. New: none.
2. Tests deleted, CI faster. Checks: `make -n e2e` fails (no target); `.github/workflows/ci.yml` has
   no `herdr-e2e` job; `ls e2e` fails; `go build ./... && go vet ./...` exit 0.
3. Config `herdr` reads as auto. Checks: with `agent-runner: herdr` in the config, `gx server start`
   (via the fixture) exits 0, and the snapshot's runner (or the start-up log line) shows PTY in use; `gx server status`
   prints no runner. New scenario to
   write: a fixture config with `agent-runner: herdr`.
4. Warning. Checks: the server log from story 3 holds one warning naming the removed herdr runner,
   with the text given above.
5. Reload parity. Checks: with the server running, set `agent-runner: herdr` through
   `gx server settings-set` and the settings API; both succeed, the API stores `auto`, and the reload
   log shows the warning, not a rejection.
6. Settings choices. Flows: the catalog's settings flow. Checks: the generated
   `docs/config-schema.json` enum for `agent-runner` lists no `herdr`. The `agent-runner` setting in
   the web settings page shows no herdr choice. With `agent-runner: herdr` stored, the page shows
   `auto` and the warning.
7. Empty leftover claim. New scenario to write: `herdr-leftover-empty`, a ticket claimed with
   runner `herdr` in the run registry, a clean worktree, server restarted. Checks: after restart the
   ticket's log holds `claim-reopened` and the ticket starts on PTY.
8. Leftover claim with work. New scenario: `herdr-leftover-work`, the same with a commit in the
   worktree. Checks: the ticket is paused `handle-mismatch`; its reason text names the removed herdr
   runner and says Retry or Start over ends it. The "Every pause reason recovers" flow
   (`webui/e2e/pause.e2e.ts`) is extended with a PTY scenario so that Retry on this ticket resumes it
   on PTY.
9. Retry and Start over on the web. Flows: "Every pause reason recovers" with the `herdr-leftover-work`
   scenario. Checks: on the story 8 ticket, Retry and Start over are offered in the web and both
   succeed.
10. Answer and resume on a gone runner. Checks: on a ticket whose recorded runner is `herdr`, the
    answer and resume verbs return the gone-agent result (no "herdr unavailable" error). Answer and
    resume are checked on the `herdr-leftover-work` ticket: the answer is accepted and the ticket
    resumes.
11. Records with no runner field. Checks: a `runs.json` entry with no runner restarts as in story 7
    or 8, and the registry code no longer defaults to herdr (covered by the unit test in the
    registry package).
12. Migration. Checks: a ticket with `pause_reason: agent_pane_busy` in a fixture store is rewritten
    to `iteration-error` by the server's start-up migration and shows its recovery verbs; the schema
    snapshot test passes. New scenario: a fixture store with such a ticket.
13. Old run logs load. Checks: a run log holding `herdr-unavailable`, `herdr-available`,
    `herdr-agent-lost` and `agent_pane_busy` lines is read by `gx` without error and the Queue tab
    renders. New scenario: such a log in the fixture.
14. No herdr banner. Flows: "Every pause reason recovers" (`webui/e2e/pause.e2e.ts`), with a
    PTY scenario added there if needed. Checks: the `/api` snapshot has no `herdr_unavailable` or
    `herdr_text` fields and no "herdr" text.
15. Shared kinds still work. Flows: "Every pause reason recovers" in `webui/e2e/pause.e2e.ts`, with
    PTY scenarios added there for name-taken and prompt-stalled if missing. Checks: `agent_name_taken`,
    `agent_prompt_stalled` and `handle-mismatch` pauses still appear and recover.
16. Recovery rules. Checks: `go test ./recovery` passes; `rg -i herdr recovery/` prints nothing.
17. Doctor. Checks: `go run . doctor --check-blocked-form` exits non-zero as an unknown flag;
    `go run . doctor` still prints `HERDR_ENV`.
18. Split launch. Checks: `go test ./cmd/term ./ui/terminalrun ./ui` pass; manually or in the
    real-binary suite, `gx term` still builds the herdr split command.
19. Worktrees tab launch. Flows: none (TUI). Checks: `go test ./ui/worktrees/...` pass, and the
    worktrees launch smoke test of the real-binary suite (if written) passes.
20. Ping gate. Checks: the TUI test for the multiplexer menu with a failing `Ping` shows the
    "multiplexer not answering" message.
21. `herdr/` package. Checks: `go doc ./herdr` lists only `CLI`, the workspace calls and `Ping`;
    `go test ./herdr` passes against the stub.
22. Stub instead of fake. Checks: `ls testutil/herdrfake` fails; `herdr/` tests write the stub under
    `t.TempDir()`.
23. TestMain boilerplate. Checks: `rg -l herdrfake --glob '*.go'` prints nothing; `go test ./testmainguard` passes.
24. Rig options. Checks: `rg 'WithHerdr|HerdrIsTheSubject|StartHerdrDown'` prints nothing; the
    server rig packages pass.
25. Moved tests. Checks: the rewrite list tests in the inventory pass on `runnerfake` or PTY with no
    herdr import (`go list -deps` of each shows no `herdrfake`).
26. Wait-loop tests. Checks: `go test ./ralphloop/supervise/...` passes; `rg -i herdr
    ralphloop/supervise` prints nothing; `waitforfinish_production_test.go` and
    `waitforfinish_production_recovery_test.go` are gone; a `runnerfake` test covers the Codex
    `/compact` prompt blocked case and one covers the end-to-end premature-idle compact flow.
27. CI. Checks: `make test` runs the real-binary suite; `gh workflow` config has no `e2e` step;
    `.ai/index.md` no longer lists `go test ./e2e/...`.
28. TUI coverage ports. Checks: the in-process ports each exist and pass: holder `TERM`/`COLORTERM`
    and `H` echo (`go test ./nativerunner/holder`), live SGR, cursor and shifted letters
    (`go test ./ui/agents`), attach after switch (`go test ./ui/app`), menu anchor
    (`go test ./ui/tickets`).
29. Real-binary suite. Checks: the suite (server mode shown and dropped on `gx server stop`, shifted
    letters) passes under `scripts/proc-cap` on the PTY runner, with no herdr on `PATH`.
30. `testutil/vtterm`. Checks: `go list ./testutil/vtterm` succeeds and `e2e/` is gone.
31. `WithDialer`. Checks: the attach-after-switch test in `ui/app` asserts the fake holder was
    dialed.
32. Normaliser. Checks: `rg -n 'herdr' agentrunner/ server/settingsapi server/settings config/registry`
    shows only the normaliser and its test.
33. ADRs. Checks: the new ADR file exists with the next free number; `rg 'Superseded by' docs/adr/0031* docs/adr/0043* docs/adr/0044*` shows the new number in each.
34. Glossary. Checks: `rg -in herdr GLOSSARY.md` returns only the Split launch entry.
35. Docs. Checks: `rg -in 'herdr' README.md .ai docs/ralph-loop-queue-orchestrator.md` shows no line
    that calls herdr a runner or the default; PTY is named the default.
36. gx-investigate. Checks: `rg -in 'agent_pane_busy|herdr agent wait|TabCreate' skills/gx-investigate`
    prints nothing; `go test ./skills` passes.
37. Tracker. Checks: `gx tickets show gx:follow-ups-2026-11-01/05` shows it retired; `/30` no longer
    names `runnerHerdr`.
38. Dead switches. Checks: `rg 'DropAskedAgent|agentfakeworker|HERDR_SOCKET_PATH'` prints nothing
    outside history docs; `go build ./...` exits 0.

New flows or scenarios to write (each becomes an implementation ticket): fixture config with
`agent-runner: herdr`; scenarios `herdr-leftover-empty` and `herdr-leftover-work`; a fixture store
with an `agent_pane_busy` ticket; a fixture run log with removed kinds and types; and an extension of
the existing paused-ticket recovery flow for the story 8 ticket. The implementer picks flow numbers
after the highest in `webui/e2e/CATALOG.md`.

## Out of Scope

- herdr as a terminal multiplexer: TUI split launch and the worktrees tab workspace launch stay.
- The person's own `herdr` and `tui-testing-with-herdr` skills in `~/.dotfiles`.
- The headless runner.
- Auto-restart of leftover herdr runs, and any code that discards a worktree holding work.
- Rewriting history: old ADRs (apart from the "Superseded by" lines), specs, research, plans,
  `CHANGELOG.md` and fixture data that only quotes the person's herdr skill.
- Follow-up `follow-ups-2026-11-01/04` (start-over test for a finished agent), which stays open.
- A replacement CI job for the removed e2e suite.

## Further Notes

- The chart round wrongly named `agent_name_taken` as herdr-only. It is shared with PTY and headless
  and stays. Only `agent_pane_busy` leaves the pause kinds.
- The correct fix for the unbuildable runner is one rule in `reclaim`, `RunnerOf` and `AgentGone`
  (unbuildable means gone). It is not a herdr special case, so it needs no herdr code to remain.
- If `ValidateSetting` only dropped `herdr` from its list, reload and the settings API would reject a
  config that startup accepts. This is why the single normaliser is required.
- The installed `gx` is older than the worktree. Hand-run changed commands with `go run . <cmd>`.
- No stored mocks exist for this map.
