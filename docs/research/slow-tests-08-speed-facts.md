# The open speed facts (gx:slow-tests/11)

Three questions left open by `slow-tests-measure.md` (gx:slow-tests/01): why an agent e2e flow takes about 40 s, why `gxfixture` scenario tests wait 30 to 78 s, and what web-e2e really costs on main. Research only. No product code or test was changed. Branch `research/slow-tests-speed-facts`, based on main at `a3cad310`. Measured on 2026-11-02 on the dev Mac (10 cores) while live agents ran: load average was 2.5 to 6.6 during the work. Anything not established is marked **unknown**.

## Summary

- One cause explains both the 40 s and the 30 to 78 s: **the fake claude in `gxfixture` never gives the launch-prompt proof, so every ticket worker launch waits 60 s (30 s ready wait plus 30 s delivery proof) and the fixture server cannot stop while it waits.**
  - `nativerunner/pty_launch.go` `awaitReady` waits up to 30 s for an input box and a transcript file. The fake only gives them when `GX_AGENTFAKE_TUI_READY_DELAY` is set. `gxfixture` never sets it.
  - Then `claudeui.CheckLaunchDelivered` waits up to 30 s for a user record of the slash command in the transcript, and resends every 3 s. The fake only writes that record when `GX_AGENTFAKE_TUI_LAUNCH_RECORD=1`. `servertest` sets it (`server/servertest/ptyrunner.go:106`). `gxfixture` does not.
  - Before the ready-wait commit (`df88a571`, "21") only the second wait existed (30 s). With both commits the wait is 60 s.
- Agent flow time (38 to 40 s) is about **20 s fixture start + 15 s fixture stop + 3 s of the flow**. The 15 s is the fixture's SIGTERM-to-SIGKILL timeout: the server is stuck in the launch wait and never answers SIGTERM.
- Measured fix: with both fake knobs set, shutdown goes from 15.1 s to 0.1 s in the five stalling scenarios I tried (`paused`, `agents-commands`, `united`, `spend`, `default-harness`), and the gxfixture scenarios that never finished now do (blocked-menu pauses at 15.5 s, refused-ask at 4.4 s).
- 84 of the 299 `start(...)` calls in the e2e flows use a stalling scenario. That is 84 x 15 s = 1260 worker seconds, 23 percent of the whole run.
- web-e2e on main does not pass. My one full run on `a3cad310`: 320 tests, 277 passed, 43 failed, **30.7 min** wall. No run of the whole suite has passed since 2026-10-28 (112 tests, 73 to 79 s). Estimate if all 320 passed: about 26 min today, about 18 min with the launch wait fixed.

## 1. Why an agent e2e flow takes about 40 s

Reproduced: `153. Send a command, refused` takes 38.4 s and 38.2 s with `--workers=1` (`agents-commands` scenario). In the full run the 20 `agents-commands*` flows take 35 to 40 s each (average 38.2 s). Flows on the `agents` scenario take 8 to 9 s.

### Where the 38 s goes

Playwright trace of the flow: the `start` fixture takes 15 s in "After Hooks" (the stop). I timed the two ends outside Playwright by starting `gxfixture` and measuring the first output line and the SIGTERM stop:

| scenario | start (first output line) | stop (SIGTERM to exit) |
| --- | ---: | ---: |
| `agents-commands` | 19.7 s (19.2 to 22.9 over 4 runs) | 15.1 s |
| `agents` | 6.1 s | 1.1 s |

So `38.4 s = 19.7 s start + about 3.4 s flow and browser + 15.1 s stop`.

### Cause A: the 15 s stop (the launch wait)

- `gxfixture.shutdown` sends SIGTERM to the server and waits 15 s, then SIGKILLs it (`testutil/gxfixture/shutdown.go:25-33`). The server takes the whole 15 s in these scenarios.
- Goroutine dump of the server during the stall (SIGQUIT 3 s after SIGTERM, `paused` scenario): the main goroutine waits in `Serve`, and `keepClaiming` is inside `claimNext -> claimRoot -> launch -> ralphloop.StartAndPrompt -> Pty.PromptLaunch -> ptyPrompter.awaitReady`. On `df88a571` the same dump shows `PromptLaunch -> claudeui.CheckLaunchDelivered` (the proof loop). `Serve` waits for `claimDone` (`server/server.go`, after `stopFresh()`) and neither wait looks at the context, so the server cannot stop until the launch ends.
- A queue worker is launched about 0.3 s after the fixture starts. Its launch prompt is a slash command, so `PromptLaunch` takes the proof path (`isSlashCommand`). Server log of `paused` on main: `ready signal missing, sending the launch prompt anyway` at +30.5 s.
- Why the signal never arrives: `testutil/agentfake/tui.go` gives the ready signal only when `GX_AGENTFAKE_TUI_READY_DELAY` is set ("Unset, it gives neither") and the user record only with `GX_AGENTFAKE_TUI_LAUNCH_RECORD=1`. `gxfixture` builds the server's environment itself (`testutil/gxfixture/main.go`, `HOME`, `PATH`, ...) and sets neither.
- Check: I put a wrapper script in front of fakeclaude that exports `GX_AGENTFAKE_TUI_READY_DELAY=0s` and `GX_AGENTFAKE_TUI_LAUNCH_RECORD=1` and passed it as `--fakeclaude`. Stop time then:

| scenario | stop, today | stop, both knobs | knob alone |
| --- | ---: | ---: | --- |
| `paused` | 15.1 s | 0.1 s | ready delay only: 15.1 s; launch record only: 15.1 s |
| `agents-commands` | 15.1 s | 0.1 s | |
| `united`, `spend`, `default-harness` | 15.0 to 15.1 s | 0.1 s | |

  On main both are needed. On `df88a571` only the launch record is (stop 15.1 s to 0.1 s for `paused`).

### Which scenarios stall

Measured with a stop right after start (so a launch is always in flight): 15 s in `agents-commands` (and `-slow`, `-failed`), `united`, `paused`, `spend`, `default-harness`, `health`, `finishing`, `migrate-broken`, `queue-agents`, `rate-limited`, `refused-ask`, `restart-agents`, `blocked-menu`, `codex-rate-limited`. No stall (0 to 1 s): `agents`, `agents-failures`, `design*`, `ready`, `empty`, `limit-burn`, `codex-queue`, `codex-blocked`, `codex-error`, `restart-projects`, `web-*`, `limits`, `mixed` and the rest. `usage-gated` did not start (exit 1), **unknown** why.

A stall is the rest of the 60 s launch wait, capped at 15 s. A flow longer than about 45 s stalls less.

Flows per stalling scenario (`start("...")` calls in `webui/e2e/*.e2e.ts`): `agents-commands` 29, `united` 20, `paused` 9, `spend` 7, `health` 4, `default-harness` 3, `migrate-broken` 3, `queue-agents` 2, and 1 each for `codex-rate-limited`, `restart-agents`, `agents-commands-slow`, `agents-commands-failed`, `finishing`, `blocked-menu`, `rate-limited`. Total 84 of 299 starts.

### Cause B: the 20 s start (session seeding)

- `agents-commands` seeds 22 sessions, one after the other (`seedSessions`, "each settled before the next so the list keeps creation order"). Each waits for a reply and for Idle by polling `gx agents list` and `gx agents chat` every 200 ms, plus fixed 300 ms sleeps for each command step.
- I wrapped the `gx` binary to log every CLI call during the start: 278 calls in 22.9 s, of which 110 `agents list`, 115 `agents chat`, 22 `agents create`. The CLI calls themselves are cheap (about 30 ms each, 8 s in total). There was no gap over 1 s. The time goes to about 5 polls per session while the fake claude starts and the server notices it.
- Cost per seeded session is about 0.9 s: `agents` (8 sessions) starts in 6.1 s, `agents-commands` (22) in 19.7 s. `codex-queue` starts in 34.5 s (2 flows, **unknown** why it is that slow).
- The three agent command flows I looked at use 2 or 3 of the 22 sessions.

### What would shorten it

| # | change | where | effect (measured unless noted) |
| --- | --- | --- | --- |
| 1 | Set `GX_AGENTFAKE_TUI_READY_DELAY=0s` and `GX_AGENTFAKE_TUI_LAUNCH_RECORD=1` in the environment `gxfixture` gives the server (as `servertest/ptyrunner.go:106` already does for the second) | `testutil/gxfixture/main.go` | stop 15.1 s to 0.1 s. Agent flow about 38 s to about 23 s (-39 percent). 84 flows x 15 s = 1260 worker s, about 7 min of wall time at 3 workers, 23 percent of the run |
| 2 | Make the launch wait stop when the server context is cancelled (`awaitReady` and `CheckLaunchDelivered` check `ctx`) | `nativerunner`, `claudeui` | same stop gain without touching the fixture; also stops a real `gx server` from taking up to 60 s to stop during a launch (**not measured** outside the fixture). Needs product code |
| 3 | Seed only what a flow needs: a lean command scenario (about 6 sessions instead of 22) | `testutil/gxfixture/commandsscenarios.go` | start about 20 s to about 6 s (estimate, 0.9 s per session) for 29 flows: about 400 worker s, 2 to 3 min of wall time |
| 4 | Seed sessions in parallel, or poll at 50 ms | `testutil/gxfixture/sessions.go` | **unknown**; needs an order guarantee for the session list. Upper bound is the 20 s |

Items 1 and 3 together would bring an agent command flow to about 9 s.

## 2. Why `gxfixture` scenario tests wait 30 to 78 s

Same cause as section 1. The scenario tests start a fixture whose queue launches a ticket worker with a slash-command prompt on the fake claude; the launch proof fails, and the test waits for a state that never comes.

- Run B in `slow-tests-measure.md` (`df88a571`, no ready wait): `TestRefusedAskScenario_...` failed after its 60 s poll; its ticket history shows `prompt-nudged` every 3 s from 10:51:21 on. That is `CheckLaunchDelivered` resending the prompt for 30 s with no user record in the transcript. Four scenario tests took 76 to 78 s. The 33 to 46 s ones (`TestQueueAgentsScenario_...` 46.9 s, `TestChartingHandoff_...` 33.6 s, `TestHealth_...` 32.1 s) wait out one 30 s grace plus startup.
- On main (run A) the same tests wait 60 s per launch (30 s ready wait, then 30 s proof), so they hit their own 60 s polls: `TestPausedScenario_WebRecoveryClearsEachPause` 137 s, `TestCodexQueueScenario_...` 77 s, `TestLimitBurnFixtureLive` 47 s, `TestUsageGatedScenario_...` 45 s.
- Time to the expected state, measured with the CLI against a started fixture (polling `gx server tickets detail` each second, 120 s cap):

| scenario | wait for | today (main) | today (`df88a571`) | with the knobs (both builds) |
| --- | --- | --- | --- | --- |
| `blocked-menu` | `pause_reason = blocked-pane` | not reached in 120 s | not reached in 120 s | 15.5 s (the fixed 15 s dwell) |
| `refused-ask` | `status = needs-answer` | not reached in 120 s | not reached in 120 s | 4.4 s |

  The knobs for `df88a571` are the launch record only; for main both.
- What would shorten it: the same item 1 as above. The test that took 137 s and the ones at 60 to 78 s should take the time of the scenario itself (4 to 18 s for these). Not every slow gxfixture test is this cause: the ones that pass at 17 to 22 s (`...AfterTheDwell`, `TestHealth_TheControlFilesSteerTheMeters`) wait for the real 15 s `blockedDwellMs` (see `slow-tests-04-audit.md` G3) and `TestSpendFixture_ShapesShowInTheSummary` (64 to 67 s, passing) is **unknown**.
- I did not run the Go tests themselves with the knobs, since that needs a code change in `gxfixture`. The CLI checks above run the same fixture binary and scenarios the tests use.

## 3. The real web-e2e time on main

- Source for a passing time: `~/.local/share/gx/check/gx.jsonl` has 37 `full` runs and only two with a passing test step. One (2026-10-28 20:47 UTC) ran 112 Playwright tests in 1m19.66s (`PASS playwright: 112 tests in 1m19.66s`, `test.log`); the other (10-29 02:09) did not run the e2e suite at all (its `test.log` shows only `go-test` and a related vitest run). The `gx brief` index has eight whole-suite Playwright runs with exit 0, all on 10-28 (112 tests, 73 to 79 s). Since the suite grew to 320 tests, no whole-suite run has passed.
- So I ran it once, on `a3cad310` in a clean worktree: `pnpm exec playwright test -c e2e/playwright.config.ts` (the command of `make web-e2e`, 3 workers, no retries), load average 2.5 at the start and 3.0 at the end.
  - **320 tests, 277 passed, 43 failed, 30.7 min (1843 s) wall.** The tests add up to 5459 worker seconds; 5459 / 3 = 1820 s, so the 3 workers were busy 99 percent of the time.
  - The failed tests cost 1693 worker seconds (31 percent of the run; my parse of the list reporter finds 45 failed lines against Playwright's 43, probably two expected-failure `reload allowance` tests): 5 ran into the 60 s test timeout, 8 ran 46 s, 8 ran 26 s, one ran 198 s. I did not look at why they fail.
  - Passing tests: median 7.9 s, 153 tests under 10 s (481 worker s), 34 tests over 30 s (1579 worker s). Nearly all the over-30 s ones are `agents-commands*`, `agents-permission` and `agents-keys` flows (cause A and B).
- Estimate for an all-green run today: the 277 passing tests are 3766 worker s; the 43 failing ones would take about 15 to 20 s each (about 650 to 860 worker s) if they passed. That gives 4400 to 4600 worker s, **about 25 to 26 min**. With the 1260 worker s of cause A removed: **about 18 to 19 min**. With the seeding trim (item 3, about 400 worker s): about 16 min. These are estimates built on the measured per-test times.
- Other slow groups, not investigated: `vi-editor.e2e.ts` is 49 tests at an average 18 s (897 worker s) although its scenario (`design-working`) starts in 0.5 s and stops in 0 s, so the time is in the browser steps or the flow; **unknown**. `settings.e2e.ts` averages 37 s on 8 tests and `agents-permission.e2e.ts` 66 s on 7 (two failed), both **unknown** (scenarios not checked).
- Reaching the 75 to 90 s of Oct 28 again is not possible by fixing causes A and B alone: 320 tests at 3 workers need at least 320 x 1.5 s (the floor I saw for the fastest flows, `79. Keyboard send while the design works` 1.6 s) / 3 = 160 s, and the real flows are heavier than that. More workers would help, but the dev Mac is shared.

## 4. What I did not establish

- The cause of the 43 failures in the full run and of the 198 s test.
- Whether `usage-gated` fails to start in general or only under my script.
- Why `codex-queue` starts in 34.5 s.
- The per-session cost split inside the 0.9 s (fake claude start against the server noticing it).
- How long a real `gx server` takes to stop during a launch wait outside the fixture.

## 5. Sources and commands

- Code read: `nativerunner/pty_launch.go` (`PromptLaunch`, `awaitReady`, `DefaultReadyWait`), `agentrunner/claudeui/launchproof.go` (`CheckLaunchDelivered`, `DefaultLaunchProofCaps`), `testutil/agentfake/tui.go` (`TUIReadyDelayEnv`, `TUILaunchRecordEnv`, `startSession`), `testutil/gxfixture/{main,sessions,shutdown,commandsscenarios}.go`, `server/server.go` (`Serve` shutdown order), `webui/e2e/{playwright.config,fixture,global-setup}.ts`.
- Fixture timings: `gxfixture --gx <gx> --fakeclaude <claude> --fakecodex <codex> <scenario>` from `webui/.e2e-bin` (built by Playwright's global setup), first-output time, then SIGTERM. CLI-call log through a wrapper script given as `--gx`. Goroutine dump: SIGQUIT to the server pid from `state/gx/server.lock`, captured with `tail -F` on the fixture's `server.out`. Knobs: wrapper script as `--fakeclaude` exporting the two variables and exec-ing the fake. Old build: worktree at `df88a571`, same four binaries.
- Flow breakdown: `pnpm exec playwright test --trace on -g "153. Send a command, refused"`, trace read from `trace.zip`.
- Full run: `pnpm exec playwright test -c e2e/playwright.config.ts` on `a3cad310`, list-reporter durations summed per file.
- History: `~/.local/share/gx/check/gx.jsonl`, `<run>/test.log`, `~/.local/share/gx/brief/index.jsonl` and the `20261102T083717.*` go-test log (run B).
- Analysis was ad hoc Python over those logs; the scripts are not kept.
