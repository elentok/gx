# Open flake and failing-test tickets in gx

Date: 2026-11-16. Trunk: `main` at `74bfde98f` (the design worktree has no commits ahead of it).
Question: `gx:test-suite-stability/01`. No test was run. All facts come from the tickets and from `rg` and `git log` on trunk.

## Summary

- 67 open or draft tickets name a flaky or failing test: 44 Go, 15 Playwright, 8 Vitest.
- 7 of them repeat another open ticket exactly, so there are about 60 distinct problems.
- The biggest groups: readiness timeout under load (17), real or deterministic failure (15), unknown cause (15), state read or early assertion (9).
- 1 ticket is obsolete (`webui-flakes/01`). Two more name tests that were renamed (`test-health/03` still exists under a new title; `test-health/12` has no match).
- `gx:follow-ups/482` and `gx:follow-ups/483` do not exist under that address in any epic of project `gx` (checked every epic with `gx tickets show`). They were renumbered. The workspaces test from the handoff (`TestWorkspacesScenario_SeedsFixAgentsAndAnInvestigateAgentInOtherAndUnderEpics`) is now `test-health/11`, and `follow-ups-archived/459` holds the archived copy. The disk-clean test (`TestDiskClean_PrintsFreedAndFreeAfterAndRefusalFails`) has no open ticket.

## Method

- Searched every epic of project `gx` (from `gx tickets epics`) for open, draft or claimed tickets with flake words in the title. Read the ticket body for each hit in the known epics and in `follow-ups`.
- Ran `gx tickets follow-up search --project gx` with `flaky`, `flake`, `fails`, `passes alone`, `test`, `e2e`, `vitest`, `hangs`, `timeout`.
- Checked each named test with `rg "func TestName"` (Go), the test title in `webui/src` (Vitest) and the flow title in `webui/e2e` (Playwright).
- A signature is taken from the ticket's evidence. `(H)` means the ticket only suggests it. Fact and guess stay apart.

Signature codes:

| Code | Meaning |
| --- | --- |
| R | readiness or event timeout (a wait ran out) |
| F | fork or resource error |
| S | state read without a wait, or an assertion that runs too early |
| T | timer-count or fixed-window assertion |
| X | shared state or leaked process |
| G | real regression or deterministic failure (fails alone, or on the merge base) |
| U | unknown (not enough evidence in the ticket) |
| O | obsolete (code no longer exists) |

## What "done" means in the old epics

- Tickets in `gxfixture-load-flakes`, `server-test-flakes` and `test-suite-hygiene` with status `done` carry a `Moved to gx:test-flakes/NN` note. "Done" there means "copied to `test-flakes`". It does not mean fixed.
- So `test-flakes/07` to `23` are the live copies. Three originals are still open and repeat them: `gxfixture-load-flakes/01` (= `test-flakes/07`), `server-test-flakes/09` (= `test-flakes/13`), `test-suite-hygiene/03` (= `test-flakes/21`).
- The ticket description guessed these copies were "done" because they were fixed. The tickets show no fix evidence for any of them.

## Go (44)

"Trunk" says whether the named test still exists. Paths are current trunk paths.

| Ticket | Test and file | Sig | Duplicates and notes |
| --- | --- | --- | --- |
| go-flaky-tests/01 | TestPty_HoldKeepsTheRunWorkingUntilReleased, server/servertest/ptyrunner_test.go | R | waits for turn end, 30.9s; passes alone in 1.5s |
| go-flaky-tests/02 | TestPty_PersonInputAfterGxTypedEndsNudgesWithoutError, nativerunner/pty_personinput_test.go | U | one failure, no message |
| go-flaky-tests/03 | TestConflict_ThreeMergeTicketsThenNeedsRebase, server/prflow/conflict_test.go | F (H) | map says Nov 16 prflow tests hit the git fork error; this ticket has no error text |
| go-flaky-tests/04 | TestRecovery_UnmatchedPauseForksAQueuedInvestigateChildAndNeverRecurses, server/recovery/recovery_investigate_test.go | U | one failure row |
| go-flaky-tests/05 | TestOpen_NoOriginIsAPreflightFailure, server/prflow/open_test.go | F (H) | hung over 1m under load; same note as 03 |
| go-flaky-tests/06 | TestChatSessionsScenario_EachSessionStateAlertLandsInTheCaptureFile, testutil/gxfixture/webflows/chattickets_test.go | R | hangs 60s in full run, 10s alone |
| go-flaky-tests/07 | TestDeadRunEmptyReopens, testutil/gxfixture/deadrun_test.go | R | `already running (pid )`, `holder.sock never answered`; seen 4 times; also in test-flakes/09 |
| go-flaky-tests/08 | TestSessionPrompt_CodexDaemonDialogIsAnsweredAtStartup, server/sessions/prompt/promptflows/sessionprompt_codex_test.go; TestSessionKeys_CodexScreenDialogTakesYAndEsc/allow, server/sessions/prompt/keys_permission_codex_test.go | U | the map says this ticket records the CodexQuestion tests. It does not: those are in follow-ups/532 |
| test-flakes/09 | gxfixture package: TestCodexBlockedScenario_PermissionRequestShowsAPendingCard (designflows/ready_test.go), TestLimitBurnFixture_AppendedTokenCountSavesACodexSample (limitsflows/limitburn_test.go), TestDeadRunEmptyReopens, TestDeadMany and more | R | `holder.sock never answered`, `claude exited (code 1) before it turned idle`. Overlaps go-flaky-tests/07, test-health/09. Copy: gxfixture-load-flakes/03 (done) |
| test-flakes/10 | TestScenarios_StartAnswerAndStop/check-lock, testutil/gxfixture/main_test.go | X | the scenario waits on the lock of the outer `gx check`. Copy: gxfixture-load-flakes/04 (done) |
| test-flakes/11 | no test: stopping a `design` gxfixture kills every other live ticket holder | X | a tool bug, not a test. Copy: gxfixture-load-flakes/05 (done) |
| test-flakes/12 | TestLateFinish_FailedConditionStaysPaused/status_changed (server/merge/latefinish_test.go); TestDerivedActionable_PageAnswerClearsGrilling (server/sessions/prompt/derivedactionable_test.go) | U | copy: server-test-flakes/01 (done), which absorbed /06 |
| test-flakes/13 | TestDiskCleanNow_HoldEndsInTheResponse, server/disk/diskcleannow_test.go | S (H) | snapshot still `Held:true`; ticket suspects two clearing paths. Duplicate: server-test-flakes/09 (open). Copy: server-test-flakes/02 (done) |
| test-flakes/14 | TestChartingHandoff_NothingRestartsOutsideAStrongestDesignLaunch/worker_session, server/sessions/convflows/sessionhandoff_test.go | R | `timed out waiting for a new conversation id`. Copy: server-test-flakes/03 (done) |
| test-flakes/15 | TestDiskHold_TwoProjectsOnOneDiskGiveOneEntryAndOneNotice (server/disk/diskhold_test.go); TestSessionScreen_LiveReturnsHolderRows (server/sessions/askflows/sessionscreen_test.go) | R | 15s `waitFor` cap under load; blank rows from a second holder screen read. Copy: server-test-flakes/04 (done) |
| test-flakes/16 | TestHarnessSwitch_ChartingHandoffKeepsTheSessionsKind, server/harness/harness_gaps_test.go | U | the new conversation got two first prompts; may be a product bug. Copy: server-test-flakes/05 (done) |
| test-flakes/17 | TestUsageGate_HoldsClaudeClaimsOnly, server/limits/usage_gate_test.go | U | 6 fail-then-pass runs; an earlier fix did not hold. Copy: server-test-flakes/07 (done) |
| test-flakes/18 | TestMigrationBlock_FailedProjectIsNotClaimedAndOthersAre, server/lifecycle/migrationblock_test.go | G | failed on clean HEAD on Nov 6. Whether it still fails is unknown. Copy: server-test-flakes/10 (done) |
| server-test-flakes/09 | TestDiskCleanNow_HoldEndsInTheResponse | S (H) | exact duplicate of test-flakes/13 |
| test-health/01 | TestStallNudge_HungPaneTwicePausesPromptStalled, server/sched/launchtest/hung_nudge_test.go | R (H) | "started no turn"; failed in 2 of 6 full runs on Nov 15; starvation is a guess |
| test-health/02 | TestLateFinish_ResumeAndRelaunchAdoptFirst (server/merge/latefinish_test.go); TestKeepWorktreeScenario_TheAskedFileSurvivesTheAnswerAndASecondTicketRuns (designflows/keepworktree_test.go) | F | `fork/exec /opt/homebrew/bin/git: resource temporarily unavailable`, process cap hit, load above 5 |
| test-health/05 | TestRestartAgents_CountsTheRunningAgent, testutil/gxfixture/limitsflows/restart_test.go | U | one fail-then-pass |
| test-health/06 | TestKeepWorktreeScenario_TheAskedFileSurvivesTheAnswerAndASecondTicketRuns | G | hangs to the 1m timeout alone, on main too. Overlaps test-health/02, test-fixture-failures/02 |
| test-health/07 | TestBudget_HardLimitStopWaitsForSlowInterrupts, server/budget/limits_test.go | R | `timed out waiting for no live sessions` |
| test-health/09 | TestDeadMany, testutil/gxfixture/deadsessions_test.go | U | 20.9s against the 20s slow-test limit; 12s alone. Overlaps test-flakes/09 |
| test-health/10 | TestStallNudge_HungPaneOnceIsRelaunchedOnce, server/sched/launchtest/hung_nudge_test.go | R | `prompt not delivered`; 30s against 20s |
| test-health/11 | TestWorkspacesScenario_SeedsFixAgentsAndAnInvestigateAgentInOtherAndUnderEpics, designflows/workspaces_test.go | S (H) | seeded agents probably not registered when read; load average about 12 |
| test-health/12 | TestStallNudge_HungPanePausesPromptStalled ("starts = 1, want 2") | T | name not found on trunk; probably the `Twice` variant. Duplicate of test-health/01 (H) |
| test-health/13 | TestVimLaunch_PersistentFooterWithAHangEndsPromptStalledAndRetryWorks, server/sched/launchtest/vim_launch_test.go | R (H) | failed in the same run as 14 and 15 |
| test-health/14 | TestClaim_ResumesOrColdStarts/answered_pty_resumes, server/sched/launchtest/claim_resume_test.go | R (H) | same run as 13 and 15 |
| test-health/15 | TestHerdrBanner_HiddenWhileNoTicketNeedsHerdr, server/sched/claimhold_test.go | F | map: it failed on Nov 16 with the git fork error and a 10s timeout. Same run as 13 and 14 |
| test-health/16 | TestKeepCleanupScenario_MergingTheEpicRemovesALeftoverIterationWorktree, designflows/keepworktree_test.go | U | one fail-then-pass; regression of an archived ticket |
| test-determinism/01 | TestHealthRoute_DisksReportNamesTheWorstDisk, server/health/healthroute_test.go | S | diagnosed: meter and disk list come from different snapshots. Duplicate of system-health/05 |
| system-health/05 | TestHealthRoute_DisksReportNamesTheWorstDisk | S | duplicate of test-determinism/01 |
| test-fixture-failures/01 | TestTUIClaude_PidFileFollowsControls, testutil/agentfake/tuiclaude/tui_test.go | X | fails on every package run, passes alone, so an earlier test leaves state behind |
| test-fixture-failures/02 | TestNoUserRecordScenario_..., TestKeepWorktreeScenario_..., TestDesignQuestionWaitScenario_... (designflows) | G | two reproduce alone; red on base |
| store-index/02 | TestSweep_RacingSubmitsNeverLoseTheQueueItem, server/queue/oneoffsweep_test.go | X | ticket files vanish while the sweep runs |
| follow-ups/499 | TestConfigReload_StoreCommitDebounceApplies, server/lifecycle/configreload_store_test.go | G | fails 3 of 3 alone, on main too. Also failed in both Nov 16 release runs (map) |
| follow-ups/514 | four server/sched tests (TestSchedulingPort_*, TestColdStoreWave_*) | U (H) | failed together, passed next run; one shared load cause is suspected |
| follow-ups/532 | TestSessionAnswer_CodexQuestion* (8 tests), server/sessions/prompt/codexquestion_test.go | G | deterministic `codex-mismatch` on main. Not a flake. Not in go-flaky-tests/08 |
| follow-ups/533 | TestSchedulesRestartShape_ServerStopsAndStartsWithTheFixtureAlive, TestMigrateHeld_StaysMigratingUntilTheHoldFileIsRemoved, limitsflows/migrate_test.go | R (H) | pass alone in 7.6s |
| follow-ups-2026-11-01/29 | TestAutoAdvance_RunEndSendsContinueAfterTheCountdown, server/sessions/prompt/promptflows/autoadvance_countdown_test.go | T | countdown wait; ticket suggests a fixed sleep; seen 4 times |
| old-words/01 | TestNoOldWords, oldwords/oldwords_test.go | G | guard test, 75 hits on trunk when filed. Current count not checked |
| follow-ups/517 | TestNoOldWords hit in cmd/ticketsflow/tickets_paused_test.go | G | same guard as old-words/01 |

All 44 named tests exist on trunk except `TestStallNudge_HungPanePausesPromptStalled` (test-health/12). Several moved packages (for example `server/health`, `server/queue`, `server/disk`); the ticket paths are old, the tests are not.

## Playwright (15)

| Ticket | Flow and file | Sig | Duplicates and notes |
| --- | --- | --- | --- |
| webui-test-flakes/01 | flow 199 Triage journey, e2e/triage.e2e.ts | G | fails alone and at the merge base; exists. Duplicate of follow-ups/519 |
| follow-ups/519 | flow 199, triage.e2e.ts:73 and :157 | G | `Decided unless you object` is no longer in `webui/src` except one ChatLog test. The Triage UI was rewritten in `a67e45462` on Nov 16. A stale expectation is likely (H) |
| webui-test-flakes/02 | flow 24 Phone, Design page, e2e/phone.e2e.ts:164 | S (H) | height off by 32px; passed in 1 of 2 reruns. Duplicate of webui-flakes/03 |
| webui-flakes/03 | flow 24, 6 to 8 of 40 repeat runs fail | S (H) | the ticket suggests layout read before the viewport settles; flow 61 may share it |
| webui-test-flakes/03 | pause "a process storm recovers", e2e/pause.e2e.ts:265 | G | fails alone at HEAD and at the merge base, 8+ sightings; `expect` waits 150s, test limit 60s |
| webui-test-flakes/04 | flow 158 empty chat after an answered card, e2e/agents-permission.e2e.ts | G | undiagnosed, may be a front-end bug in `AgentChat.tsx` |
| webui-test-flakes/05 | flow 22 jump, e2e/jump.e2e.ts | R | click waits 60s; 1 pass and 2 fails alone |
| webui-flakes/02 | flow 198 Cleanup gear menu, e2e/cleanup.e2e.ts | G | fails alone and at the merge base |
| test-flakes/07 | pause "every pause reason recovers", e2e/pause.e2e.ts:23 | R | `Reason` textbox wait ran 240s under load; 3 of 3 alone pass. Duplicate: gxfixture-load-flakes/01 (open) |
| gxfixture-load-flakes/01 | same flow | R | exact duplicate of test-flakes/07 |
| test-flakes/08 | flows 155 and 159, e2e/agents-commands-output.e2e.ts:130 | R | `session never reached Unread` plus a `sessionStorage` SecurityError. Copy: gxfixture-load-flakes/02 (done) |
| test-health/04 | flow 158 fixture never starts, e2e/agents-permission.e2e.ts | R | `session ... never reached a agent message`; 5 identical failures, undiagnosed |
| test-health/08 | flow 117 (codex.e2e.ts:27) and Dev mode PWA step (dev-mode.e2e.ts:58) | U | no baseline run |
| test-health/17 | 25 flows fail on main (composer, phone, bulk, codex, design-page, newdesign, epics and more) | G | same flows fail at the merge base; overlaps webui-test-flakes/02, webui-flakes/02, test-health/08, /19 |
| test-health/19 | many flows fail or run slow in `gx check --full` (29, 36 and 17 failures in three runs) | G | fails at the merge base too; overlaps test-health/17 |

## Vitest (8)

| Ticket | Test and file | Sig | Duplicates and notes |
| --- | --- | --- | --- |
| webui-test-flakes/06 | InterruptControl "Esc again closes the confirm...", webui/src/design/InterruptControl.test.tsx:39 | S (H) | popover still in the DOM; passes alone. Duplicate of test-health/18 |
| test-health/18 | same test | S (H) | seen 3 times on Nov 11 to 15 |
| webui-test-flakes/07 | routes.test.tsx "a Merge blocked epic..." and EpicPage.test.tsx "an -impl page uses its card's tone and comet..." | U | failed together once; both exist |
| webui-flakes/01 | NewDesignForm "a refusal shows in the dialog and keeps saving the draft" | O | `NewDesignForm` was renamed to `DraftPage` (`5fc4c1bfc`, Nov 16) and the dialog-era tests were dropped on Nov 14 (`495e2bb8b`). No such test exists |
| test-health/03 | AgentsPage.newSession.test.tsx "closing the New session form stays on the page" | U | test exists but is now titled "closing the New chat form stays on the page" |
| follow-ups/515 | ScreenModal.test.tsx:109 "Refresh does one plain read and sends no key" | U | one fail-then-pass |
| follow-ups/518 | routes.test.tsx:1308 "every page route pattern has one of the 16 route names" | G | fails 3 of 3 and on unchanged HEAD for `/rest/break` |
| follow-ups/530 | ArchivePage "spinner before data", EpicPage "keeps its card's badge", RestartDialog | U | ticket 57 left them "no repro" |

## Duplicates

Exact duplicates of another open ticket (the same test or flow):

| Keep | Repeats it | Same test |
| --- | --- | --- |
| test-flakes/07 | gxfixture-load-flakes/01 | pause.e2e.ts "every pause reason recovers" |
| test-flakes/13 | server-test-flakes/09 | TestDiskCleanNow_HoldEndsInTheResponse |
| test-determinism/01 | system-health/05 | TestHealthRoute_DisksReportNamesTheWorstDisk |
| webui-test-flakes/06 | test-health/18 | InterruptControl "Esc again closes the confirm" |
| webui-test-flakes/02 | webui-flakes/03 | phone.e2e.ts flow 24 |
| webui-test-flakes/01 | follow-ups/519 | triage.e2e.ts flow 199 |
| test-health/01 | test-health/12 | stall-nudge hung pane pauses prompt-stalled (name differs, H) |

Copies with status `done` (moved, not fixed). The live ticket is the one on the right:

- gxfixture-load-flakes/02 to 05 are test-flakes/08 to 11.
- server-test-flakes/01, 02, 03, 04, 05, 07, 10 are test-flakes/12, 13, 14, 15, 16, 17, 18. `/06` merged into `/01`, `/08` merged into test-suite-hygiene/02.
- test-suite-hygiene/01, 02, 04, 05 are test-flakes/19, 20, 22, 23. Open `test-suite-hygiene/03` is test-flakes/21.

Partial overlaps (same test or package, different angle):

- KeepWorktreeScenario: test-health/02 (fork error), test-health/06 (hangs alone), test-fixture-failures/02 (red on base). Related: test-health/16 (KeepCleanupScenario, same file).
- gxfixture start failures: go-flaky-tests/07, test-flakes/09, test-health/05, /09, follow-ups/533.
- launchtest and sched under load: test-health/01, /10, /12, /13, /14, /15 and follow-ups/514. test-health/13, /14 and /15 are one failed run.
- Playwright on the merge base: test-health/17 and /19 list flows that also have their own tickets (24, 198, 117, 118).
- Flow 158: test-health/04 (fixture never starts) and webui-test-flakes/04 (empty chat). Different symptoms.
- Codex tests in `server/sessions/prompt`: go-flaky-tests/08 (two dialog tests, flaky) and follow-ups/532 (eight CodexQuestion tests, deterministic). Not the same tests.

## Groups ranked by ticket count

| Rank | Group | Tickets | Which |
| --- | --- | --- | --- |
| 1 | Readiness or event timeout under load (R) | 17 | Go 12: go-flaky-tests/01, 06, 07; test-flakes/09, 14, 15; test-health/01, 07, 10, 13, 14; follow-ups/533. Playwright 5: webui-test-flakes/05; test-flakes/07, 08; gxfixture-load-flakes/01; test-health/04 |
| 2 (tie) | Real or deterministic failure (G) | 15 | Go 7: test-flakes/18; test-health/06; test-fixture-failures/02; follow-ups/499, 532; old-words/01; follow-ups/517. Playwright 7: webui-test-flakes/01, 03, 04; webui-flakes/02; follow-ups/519; test-health/17, 19. Vitest 1: follow-ups/518 |
| 2 (tie) | Unknown (U) | 15 | Go 10: go-flaky-tests/02, 04, 08; test-flakes/12, 16, 17; test-health/05, 09, 16; follow-ups/514. Playwright 1: test-health/08. Vitest 4: webui-test-flakes/07; test-health/03; follow-ups/515, 530 |
| 4 | State read without a wait, or early assertion (S) | 9 | Go 5: test-flakes/13; server-test-flakes/09; test-health/11; test-determinism/01; system-health/05. Playwright 2: webui-test-flakes/02; webui-flakes/03. Vitest 2: webui-test-flakes/06; test-health/18 |
| 5 (tie) | Fork or resource error (F) | 4 | go-flaky-tests/03, 05; test-health/02, /15 |
| 5 (tie) | Shared state or leaked process (X) | 4 | test-flakes/10, 11; test-fixture-failures/01; store-index/02 |
| 7 | Timer-count or fixed-window assertion (T) | 2 | test-health/12; follow-ups-2026-11-01/29 |
| 8 | Obsolete (O) | 1 | webui-flakes/01 |

Cross-cutting counts:

- Fact: only `test-health/02` shows the fork error in its own text. The three other F tickets rest on the map's Nov 16 log findings, not on their own evidence. So F is probably undercounted, since older tickets did not capture the failure message (`test-flakes/24` says `gx brief` printed none).
- Fact: most R, S and F tickets say the test passes alone and fails in a full run or under load. The tickets do not show a cause for most of them. I did not count them one by one.
- Hypothesis: many U tickets (and the three launchtest tickets in one run) share one load or process-limit cause. Only a run can confirm it. That is the job of `test-suite-stability/05` and `/06`.
- Fact: the G tickets fail alone, on trunk or at the merge base (the evidence differs per ticket). They are not flakes. A fix or a delete decision is needed, and a retry or a longer timeout would hide them.

## Adjacent open tickets that are not test flakes

Not counted above. They shape the fix plan, so the impl epic should look at them:

- Tooling: test-flakes/01 (soak never ran), /02 (known-flakes list), /23 (start on a loaded machine), /24 (`gx brief` shows no failure message), /25 (`gx brief` log retention), /26 (flag failures that also fail on the merge base); follow-ups/535 (`--repeat N`), /536 (`pnpm run e2e` ignores filters); follow-ups-2026-11-01/34, /48, /49, /21, /27, /84; check-and-brief/03; gx-check/06, /09.
- Fixture hygiene: test-flakes/22 (fixtures leak temp dirs and child processes; the disk reached 100%).
- Test design: test-flakes/19 (gate `GX_` hooks), /20 (lifecycle tests skip), /21 (audit `./server` tests); follow-ups-2026-11-01/64 (long-ticket test sleeps 35s), /83 (Codex fake docs).

## Stale statuses

`follow-ups-archived` holds tickets with status `open` that already have a Resolution of `fixed` or `duplicate`: 178, 199, 202, 210, 214, 228. They are not counted. The status field looks wrong, not the work.

## Notes for the design

- Each ticket's `Evidence` has fail-then-pass run ids. `gx evolve inputs` wrote many of them, which is why the same test appears in several epics. Nine of the epics named in the ticket are maps with `destination_unconfirmed: true`; `test-health` is a backlog.
- The impl epic can close duplicates and the three open originals first. That removes 7 duplicate tickets plus `test-suite-hygiene/03` with no loss of evidence, because the live copy holds the text.
- `webui-flakes/01` can be cancelled as obsolete. `test-health/03` and `webui-flakes/03` need a rerun against the current names before any fix.
