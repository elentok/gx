# Test audit: duplicates, slow tests and low-value tests (gx:slow-tests/04)

Research only. No product code or test was changed. Branch `research/slow-tests-audit`, based on main at `65d82e62`. Read on 2026-11-02. Anything not established is marked **unknown**.

## How to read the numbers

- Go times are "leaf seconds": the elapsed time of tests with no subtests, from `go test -json` logs in `~/.local/share/gx/brief/`. They are not wall time. `./server` and `go` run at `-parallel 2`, so wall time is roughly leaf seconds divided by 2 on top of build and queue time.
- Playwright times are "worker seconds": one test on one of 3 workers. Wall time is about worker seconds divided by 3. The Playwright logs in the brief store show no per-test time (list reporter with no durations, and CI logs show none either). The only per-test figure is the 37 to 43 s per agent flow in `slow-tests-measure.md` section 5. Every other Playwright figure below is an estimate and says so.
- Two Go log sets are used. "Healthy" is Nov 1, before the 30 s ready wait: `20261101T094705.*` (gxfixture, servertest) and ten `./server` runs from `20261101T10*` (median per test; all ten agree within 0.5 s). "Current" is Nov 2 main: `20261102T081110.*` and `20261102T0837*`, where many tests fail or wait 30 s.
- The cost is in few tests. Of 1797 `./server` leaf tests, 118 take 1 s or more and make 215 of 549 leaf seconds. 35 take 2 s or more and make 109. Vitest: 1373 tests, 158 s of test time in total, 113 tests of 300 ms or more make 66 s. Removing tests by count would save little. The groups below go after the few tests that are slow.

## 1. Exact duplicates (removable without approval)

I hashed every Go test body (whitespace removed, bodies of 80 characters or more) in all 7300 tests, and every vitest and Playwright body of 120 characters or more. I then repeated the Go pass with string and number literals masked.

| # | Test | Duplicate of | Behaviour | Time saved |
| --- | --- | --- | --- | --- |
| 1 | `ui/prs/model_test.go` `TestRefreshKeyTriggersRefresh` (line 766) | `TestRKeyTriggersRefresh` (line 713, same file) | `R` in the PR list returns a refresh command for both the open and the closed section | under 0.01 s |

- Same body, different packages, **not** duplicates: `TestHasFailed_Empty` in `ui/amend` and `ui/reword`, and `TestInputFocused_FalseByDefault` in `ui/pull` and `ui/push`. Each calls its own package's `New()`, so they test different types.
- Vitest and Playwright: no exact duplicate bodies. The pass uses a regex over `it(`/`test(`; `test.each` and generated tests are **not** covered, so there may be misses.
- Conclusion: there is exactly one exact duplicate and it saves nothing measurable. The suite is slow for other reasons.

## 2. Same coverage, faster (no coverage lost; no approval needed beyond review)

These are not duplicates. The behaviour stays the same and only the cost drops.

### F1. Build each fake binary once per package, not once per test

Evidence: many tests run `go build` for a fake in their own helper.

| Helper | Builds | Call sites | Needed |
| --- | --- | ---: | --- |
| `nativerunner/pty_codexapproval_test.go:25` `codexApprovalSession` | the whole `gx` binary | 5 tests | 2 hook tests need `gx` on PATH. The 3 screen-dialog tests remove the hook (`hook == false`) and never call `gx`. |
| `nativerunner/pty_codex_test.go:22` `codexRunner` | `fakecodex` | 6 | once |
| `nativerunner/fakeclaude_test.go:20` `fakeclaudeRunner` | `fakeclaude` | 6 | once |
| `server/sessionprompt_codex_test.go:32` | `fakecodex` | 4 | once |
| `server/harness_surfaces_test.go:29` `withFakeAgents` | `fakecodex` | 2 to 3 | once |

- Measured on a warm cache with load 3.5: `fakecodex` build 2.8 s wall (3.6 CPU s), `gx` build 5.3 s wall (15.6 CPU s).
- Measured effect in `nativerunner`: the 5 Codex approval tests take 62 leaf s (11 to 17 s each). The other Codex PTY tests that need only `fakecodex` take 4.6 to 5.8 s. So the `gx` build adds 6 to 11 s to each.
- Cheaper replacement: `sync.Once` plus a package temp dir, as `testutil/agentfake/codexhooks_test.go:132` `builtGx` already does. The 3 screen-dialog tests skip the `gx` build.
- Time saved (estimate, CPU seconds): `gx` builds 4 x 15.6 = 62, fakecodex and fakeclaude about 10 x 3.6 = 36, server fakecodex about 4 x 3.6 = 14. Total about 110 CPU s. In the `race` step (`nativerunner` takes 267 leaf s) this also cuts the 5 slowest Codex approval tests from 62 s to about 25 s (estimate).
- Risk: a shared binary must not be mutated by a test. Each helper only reads it. **unknown**: whether `-race` changes how the `go build` cost splits (the `go build` calls themselves are not built with `-race`).
- `cmd/check_record_test.go:130` builds `gx` for one test (9.6 s). It is a separate test binary, so it cannot share. Leave it.

### F2. AutoAdvance tests wait out a real 1 s countdown

- 21 `TestAutoAdvance_*` leaf tests, 44 leaf s (median 2.1 s each, 7.4 s for `StopsAfterFiveSendsWithNoInput`). `sessions_autoadvance_countdown_test.go:17` sets `countdownDelay = time.Second` and tests wait real time (two of them wait `2 * countdownDelay` as a negative wait).
- Cheaper replacement: a smaller delay (for example 200 ms) saves about 25 leaf s. Caveat: group C in `slow-tests-02-root-causes.md` shows short real-time windows are the main flake source under load. A fake clock in the countdown is safer but needs product code (**unknown** how large). Recommend only if a fake clock exists or is added. I do not recommend simply shortening the delay.

## 3. Groups that lose coverage (approve by group)

Each group says what is lost, what stays, the saving, and the replacement. All savings are estimates from the logs above.

### G1. Two negative-wait long-ticket tests in `servertest` (saves about 70 leaf s)

Candidates:
- `server/servertest/longticket_test.go:162` `TestLongTicket_ZeroThresholdProducesNoAlert`: 35.2 s healthy.
- `server/servertest/longticket_test.go:117` `TestLongTicket_ANonImplementTicketGetsNoMarkBannerOrDraft`: 35.4 s healthy.

Why they are slow: each does `time.Sleep(35 * time.Second)` to wait past the 30 s `smartZonePollMs` poll tick of `waitForFinish` (`ralphloop/waitforfinish.go:17`, unexported, value 30_000).

Behaviour covered: with threshold 0, or for a research ticket, no mark, banner or draft appears.

What else covers it: `ralphloop/longticket_test.go` `TestLongTicket_ZeroThresholdNeverFires` and `TestLongTicket_OtherTicketTypesAreUntouched` run `waitForFinish` with a fake `Sleep` and show the alert callback is never called. The server mark, banner and draft only run inside that callback (`server/longticket.go:34`).

Cheaper replacement: none to write. The unit tests exist and take milliseconds.

Lost: proof that the real server wiring stays silent for these two cases when the real 30 s tick runs. That is only reachable if `OnLongTicket` fires, and the unit tests show it does not.

### G2. One of two positive long-ticket tests in `servertest` (saves about 31 leaf s)

- `TestLongTicket_AlertsOnceMarksTheTicketAndKeepsTheSessionRunning` (30.7 s) and `TestLongTicket_ASecondLongTicketAddsSeenAgainNotANewDraft` (31.6 s). Each waits for one real 30 s tick.
- Keep the first: it proves tick, alert, mark, banner and draft end to end. Replace the second ("a second long ticket adds a seen-again line, no new draft") with a direct call of `alertLongTicket` in a `server` package test (`server/longticket.go:46`). **unknown**: whether a `package server` internal test file already exists for this; the external `server_test` package cannot call the unexported method.
- Lost: end-to-end proof for the second-ticket path with a real tick.

### G3. Three gxfixture "after the dwell" scenario tests (saves about 45 leaf s)

Candidates (17.5, 17.5, 16.8 leaf s, healthy and current):
- `testutil/gxfixture/main_test.go:532` `TestBlockedMenuScenario_QuestionMenuPausesBlockedPaneAfterTheDwell`
- `:543` `TestCodexBlockedScenario_PermissionRequestPausesBlockedPaneAfterTheDwell`
- `:553` `TestCodexErrorScenario_ModelErrorPausesBlockedPaneAfterTheDwell`

Why slow: the ticket pauses with `blocked-pane` only after the fixed 15 s dwell (`ralphloop/waitforfinish_recovery.go:65`, `blockedDwellMs = 15_000`).

Behaviour covered: each fixture scenario drives its fake agent into a state that ends as a `blocked-pane` pause.

What else covers it: the wait-loop logic with the dwell is covered with an injected `Sleep` in `ralphloop` (`d.Sleep(blockedDwellMs ...)` at `waitforfinish_recovery.go:95`) and server pausing in `server/blocked_pause_test.go`.

Cheaper replacement: assert that the scenario reaches the blocked state and shows its pending card (about 1.6 s start time, per the scenario smoke test), and stop before the dwell ends.

Lost: proof that the scenario's fake keeps the pane blocked for the full 15 s and that the real pause then follows. Playwright `pause.e2e.ts` may cover the pause on these scenarios; **unknown** which of the three scenarios it uses.

### G4. Scenario smoke test repeats the start of 36 scenarios (saves about 45 leaf s)

- `testutil/gxfixture/main_test.go:262` `TestScenarios_StartAnswerAndStop` has 48 subtests and costs 58.8 leaf s healthy (8.0 s for `agents`, 7.5 for `codex-queue` and `agents-failures`, 4.9 for `default-harness`, 0.3 to 1.6 s for most others).
- 36 of the 48 scenarios (54.5 leaf s) are also started by a dedicated test in the same package, which already needs the start and the cookie to work. The 12 without a dedicated test are `edge`, `design-working`, `design-text`, `design-plain`, `design-mixed`, `design-guard`, `design-error`, `design-defaulted`, `design-branch`, `restart-launchd`, `mixed`, `long-running`.
- Cheaper replacement: run the smoke checks (snapshot with and without cookie, `server status`, socket, SIGTERM cleanup) for the 12 uncovered scenarios and 2 or 3 of the covered ones.
- Lost: the cookie, 401, `server status` and cleanup checks for each of the other 33 scenarios. These come from shared fixture code, so a scenario-specific break is unlikely. Every Playwright flow also starts and signs in to its own scenario.

### G5. Command-menu UI flows in Playwright (saves about 160 worker s, about 55 s wall; estimate)

Candidates in `webui/e2e/agents-commands.e2e.ts`:
- `180. Command menu opens and filters` (line 7)
- `181. Command menu keys` (line 59)
- `149. Command list freshness and failure, loading` (line 159)
- `149. Command list freshness and failure, failed list` (line 168)

Each starts the `agents-commands` scenario, which launches PTY agents. Agent flows take 37 to 43 s each (measured).

What else covers it: `webui/src/agents/AgentsPage.composer.test.tsx` has `/ opens, /co filters, a space closes, Escape closes` (line 315), `Down then Enter fills and sends nothing` (330), `no match keeps the menu open` (364), `loading shows one line...` (374), `a failing list shows the notice...` (384), `a degraded list shows the built-ins...` (393).

Cheaper replacement: those vitest tests, which exist. Time cost is 5 to 25 ms of test time each (vitest total 158 s for 1373 tests).

Lost: the real browser check of the sort order (prefix matches, then substring matches), the `mark` highlight, the footer text and keyboard focus. **unknown**: whether the vitest tests check sort order and highlight. Check before approving. Keep `149 refresh`, `150 Codex command menu`, `151` and `152 phone`: they read the real command list from the server, or check layout.

### G6. Pure editor behaviours in `vi-editor.e2e.ts` (saves about 40 worker s, about 13 s wall; estimate)

- 45 flows start `design-working` (44 starts) and run a real server for editor behaviour. Candidates that need no server: `the editor opens in INSERT at the field's caret` (line 92), `ZZ in normal mode closes the editor and keeps the text` (145), `the mode text follows INSERT, NORMAL, VISUAL` (452), `h j k l w e 0 f{char} gg G move the caret as vim does` (467), `x dw yy p u Ctrl-R iXY . and v d edit the text` (518), `i a o and . change the text and the mode as in vim` (539), `390px: gs and ge move to the line start and end` (564).
- What else covers it: `webui/src/ui/expandable-text/CodeEditor.vim.test.tsx` (`opens in INSERT, then Esc, v, Esc i report the modes`, `opens with the field's caret`, `x, dw, yy p, u and Ctrl-R edit as in vim`, `ZZ calls the close callback once`).
- Cheaper replacement: extend that file with the motion (`h j k l w e 0 f gg G`), `i a o .` and `gs ge` cases. Per-flow cost is **unknown**; I used 6 s, the Playwright average (591 s x 3 workers / 294 tests on Nov 1).
- Lost: real browser key events and, for `gs ge` at 390 px, wrapped-line caret movement that jsdom may not lay out. Keep the wrapped-paragraph, modal size and focus-trap flows.

## 4. Slow tests with an unexplained cost (no replacement proposed)

| Test | Healthy time | Note |
| --- | ---: | --- |
| `server` `TestRateLimitHold_RunKeepsSlotAndExplainsTheHold/known_reset` | 10.5 s in 10 of 10 runs | The `unknown_reset` case takes 3.2 s with the same body. A product-side 10 s wait looks likely. Cause **unknown**. |
| `server` `TestBlockedPause_UsageLimitErrorHoldsInsteadOfPausing` | 10.4 s in 10 of 10 runs | Same hold path. Cause **unknown**. |
| `server` `TestRecovery_R10ReleasesAnIdleAgentsGateAndLetsTheFinishRun` | 6.8 s | 6 subtests (herdr and headless x 3 cases). About 1.1 s each. |
| `gxfixture` agent scenarios | `agents` 8.0 s, `codex-queue` 7.5, `agents-failures` 7.5, `default-harness` 4.9 | These launch PTY sessions. On Nov 2 main they sit at 16 to 22 s because of the 30 s ready wait (cause 3 in `slow-tests-measure.md`). |
| Playwright agent flows | 37 to 43 s each | About 52 flows start `agents-commands` (29) or `agents` (23). That is roughly 2000 worker s, about 700 s wall at 3 workers (estimate). The cause is **unknown** (see measure note, section 5). Fixing it beats any removal here. |

## 5. Too little for the cost

I looked for tests that cost over 2 s and prove little.

- `server` `TestTCP_RefusesBrowserRequests` (4.7 s): checks 6 request shapes and a log line. It is dense, not low value. **unknown** why it takes 4.7 s.
- `webui` `SkillsProblemToast.test.tsx` (3.6 s for one test): waits for the toast exit animation with a 5 s `waitFor`. Fake timers would cut it. Small.
- `ui/app` `TestTitleRow_Golden` 2.6 s, `cmd` `TestExecute_TicketsSet_FinishGuard/...` 2.3 s: both do real work; no case for removal.
- Everything else under 2 s makes up the bulk. Nothing in the bulk is clearly low value. I found no test to remove outright.

## 6. Summary and order of approval

| Item | Type | Saving (leaf s unless noted) | Coverage lost |
| --- | --- | ---: | --- |
| 1 duplicate (`ui/prs`) | exact duplicate | under 0.01 | none |
| F1 build once | same coverage | about 110 CPU s | none |
| F2 AutoAdvance delay | same coverage | about 25 | none, flake risk |
| G1 | coverage group | about 70 | wiring under the real tick |
| G2 | coverage group | about 31 | second-ticket path end to end |
| G3 | coverage group | about 45 | 15 s dwell then pause in the fixture |
| G4 | coverage group | about 45 | per-scenario cookie, 401, cleanup |
| G5 | coverage group | about 160 worker s (about 55 s wall) | real-browser sort order and highlight |
| G6 | coverage group | about 40 worker s (about 13 s wall) | real key events, wrapped lines |

- If all coverage groups pass: about 190 Go leaf seconds, about 95 s of wall time at `-parallel 2`, plus about 70 s of Playwright wall time. F1 adds about 110 CPU seconds.
- Compared with the measured step times (`race` 270 s, `server` 245 to 350 s, `web-e2e` 591 s on Nov 1), this is a minor gain. The large costs are not tests to remove: the 30 s ready wait, the 40 s agent flows and the `-parallel 2` cap (options 1 to 8 in `slow-tests-measure.md` section 6).

## Unknowns

- Per-test Playwright times. Only the agent-flow figure of 37 to 43 s exists.
- Whether `AgentsPage.composer.test.tsx` checks sort order and the `mark` highlight (G5).
- Whether a `package server` internal test can call `alertLongTicket` (G2).
- Which scenarios `pause.e2e.ts` covers (G3).
- The cause of the 10.5 s in `TestRateLimitHold_...known_reset` and `TestBlockedPause_UsageLimitErrorHoldsInsteadOfPausing`.
- Duplicates in generated tests (`test.each`, table rows): not checked.

## Sources and commands

- `~/.local/share/gx/brief/20261101T094705.563207000-go-test.log` (gxfixture and servertest, healthy), `20261101T10*` (ten `./server` runs), `20261102T081110.*` and `20261102T0837*` (current), `20261102T083728.652132000-vitest.log` (1373 tests, JSON with per-test durations).
- Test code read at main `65d82e62`: the files named above.
- Duplicate search: ad hoc Python over `git ls-files '*_test.go'` and `webui` test files; the scripts are not kept.
- Build timings: `go build` of `fakecodex` and `gx` on this worktree, warm cache, load average 3.5.
