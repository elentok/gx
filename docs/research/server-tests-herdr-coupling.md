# Herdr coupling in the server tests

Research for `gx:server-tests-pty/02`. Code read at `df5a8cb4` (main). No tests were run.

## Summary

- 295 server tests reach herdr. Most of them only because the harness's default runner is herdr.
- 14 are herdr-specific. 5 of these are judgment calls that could move.
- 271 can run on any runner.
- 10 need a feature pty lacks: Codex (4 tests) and Find by label for an agent no run tracks (6 tests).
- 2 more tests were flagged by the helper scan but use only a fake runner. They are not coupled.
- The real cost of the move is in the harness, not in pty. 224 "any runner" tests script the agent through herdr commands (`servertest.RegisterLaunch`, `registerLaunch`, `registerLiveAgent`, or direct `h.Herdr.Register` overrides). They need runner-neutral hooks in `servertest` before they can run on pty. The hooks are listed under "Harness hooks the move needs".

## Method

1. Found every test file in `server/` that names `h.Herdr`, `herdrfake`, `RegisterLaunch` or `runnerShapes` (`rg`).
2. Split every `server/*_test.go` into top-level functions. Marked a function herdr-touching if its body names `Herdr`, `herdrfake`, `RegisterLaunch`, `runnerShapes`, `StartHerdrDown`, `SetHerdrDown`, `HerdrUnavailable` or `HerdrRetryInterval`. Then marked every function that calls a marked helper, repeated until nothing changed. That gave 296 tests (`TestMain` aside).
3. Read every test in the 19 files from the ticket, every direct herdr user elsewhere, and each helper that makes a test herdr-touching. Gave each a group and a reason.
4. Tests that reach herdr only through a launch fixture (`newSchedFixture`, `newForkFixture`, `startCleanRig`, `startInvestigateEnv` and others) got the default reason after checking that the fixture only calls `RegisterLaunch` or `registerLaunch`.
5. Read `server/servertest`, `testutil/herdrfake`, `agentrunner`, `agentrunner/herdrrunner` and `nativerunner/pty.go` to compare runner features.

Limits of the scan:

- A test can reach herdr without naming it: the harness default runner is herdr (`server/server.go:316`). One such test is in the table (`TestNudge_Refusals`). A heuristic found 36 more tests that queue, restart or nudge on the default runner without registering herdr handlers. Their launches fail on an unregistered herdr command today. On pty they would start a real holder. The spike run should check them. They are listed at the end.
- The helper scan can over-match. It flagged two late-finish tests that call `pauseZeroCommit(t, true)`, which uses a fake runner. They are marked "not coupled".

## How herdr reaches the tests

- **Default runner.** `servertest.Start*` never sets `Config.Runner`, so `server.New` builds `herdrrunner` on the fake herdr (`server/server.go:316`).
- **Launch fakes.** `servertest.RegisterLaunch` (`server/servertest/servertest.go:212`) and the package's own `registerLaunch` (`server/runner_test.go:31`) and `registerLiveAgent` (`server/runner_test.go:801`) answer herdr's `workspace`, `tab` and `agent` commands. The test's agent turn runs inside the `agent prompt` handler, in the test process.
- **Verb overrides.** Tests register their own `agent start`, `agent wait`, `agent prompt`, `agent get`, `send-keys` or `tab close` to fail a start, count starts, hold a turn, commit on a prompt, or count stops.
- **Probe only.** `chatServer`, `chatServerFlushing` (`server_notify_test.go:34`) and `reloadChatServer` (`configreload_chat_test.go:67`) call `New` with the default runner. Their herdr fake only answers the startup `workspace list`.
- **Herdr handles.** `leaveIteration` (`runner_test.go:762`) writes a `runs.json` entry with pane and tab and no runner, which reads as a herdr run. A restart then takes the herdr branch of reclaim (`server/runs.go:575`).
- **Probe state.** `StartHerdrDown` and `SetHerdrDown` make the fake fail `workspace list`.

## pty compared with herdr

What pty has: start and stop, prompt with nudge (`PromptNudged`), status with turn counter, blocked state with the permission request as `Pending` (`pty.go:498`), interrupt, answer (allow, deny, deny with reason), reattach after a restart (`pty.go:266`), cleanup, owner pids, and resume of a Claude session.

What pty lacks, and which tests care:

| Feature | herdr | pty | Tests that need it |
| --- | --- | --- | --- |
| Codex agents | `agent start --kind codex` | `spawn` always uses the claude harness and ignores `StartOptions.Kind` (`nativerunner/pty.go:366`, `ptyHarness` at `pty.go:402`) | 4 |
| Find by label for an agent no run tracks | asks herdr by name (`herdrrunner.go:436`) | `Find` and `List` read only the in-memory session map (`pty.go:616`, `pty.go:626`); a holder found on disk only comes back through `Reattach` of a `runs.json` entry | 6 |
| Adopt a live same-label, same-cwd agent on Start | `RecoverStart` returns `StartAdopt` (`herdrrunner/hosting.go`) | `reserve` returns `ErrLabelTaken` (`pty.go:436`) | none of the classified tests |
| Rate-limit and context-exhaustion detection | reads the pane (`herdrrunner.go:470`) | `RateLimit` always reports no limit (`pty.go:890`) | none |
| Host health | `Healthy` pings herdr (`herdrrunner.go:85`) | no host, so no `HealthChecker` | herdr-specific by nature |
| Pane and tab ids on iterations | `iterations.go:82` fills them for herdr only | none, by design | herdr-specific by nature |

pty also runs `ps` for process lookups (`nativerunner/procs.go`) unless `Pty.Procs` is set. The harness must set it, as the map notes.

## Harness hooks the move needs

These are not pty features. They are what `servertest` must offer on any runner so the "any runner" tests stop scripting herdr:

- A turn hook: run a test callback in the agent's cwd for each prompt, with the prompt text and address, and let it block. This replaces `RegisterLaunch` and `registerLaunch` (208 tests use only this).
- A way to hold an agent working until the test releases it (4 tests now override `agent wait`).
- A start hook: fail the next start or count starts (4 tests).
- A stop counter, and a way to block a stop (3 tests count or block `tab close`).
- A turn that writes a transcript line under the session id (3 recovery tests through `runToZeroCommitPause`).

The probe-only tests (23) need none of these. They need either a non-herdr runner in their `New` config or the gated probe.

## Classification

Groups: herdr-specific (stays on herdr), any runner (moves to pty), needs missing pty feature (stays on herdr until `gx:pty-as-default`), not coupled.

| File | Test | Group | Reason |
| --- | --- | --- | --- |
| `answer_test.go` | TestAnswer_LateFinishIsAdoptedAndWritesNoAnswer | any runner | Calls pauseZeroCommit with headless=false (the herdr shape). Late-finish logic does not depend on the runner. |
| `budget_limits_test.go` | TestBudget_HardLimitStopsLivePaneAndPausesBudgetKilled | herdr-specific | Judgment call. Asserts herdr `send-keys ctrl+c` and `tab close`. TestBudget_HardLimitStopsRunThroughRunner covers the server side. |
| `budget_limits_test.go` | TestBudget_NudgedPausedPaneIsMeteredAndKilled | needs missing pty feature | Find by label for an agent no run tracks: nudges a paused ticket that was never launched. Judgment call: launching first also works. |
| `budget_limits_test.go` | TestBudget_SoftLimitRefusesRelaunchAndNudge | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `budget_limits_test.go` | TestBudget_SoftLimitStopsNewStarts | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `budget_limits_test.go` | TestBudget_SpendInTwoProjectsSumsTowardOneLimit | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `configreload_chat_test.go` | TestConfigReloadChat_ChangedDestinationTakesLaterMessages | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `configreload_chat_test.go` | TestConfigReloadChat_ProjectSinkKeepsItsBatch | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `configreload_chat_test.go` | TestConfigReloadChat_RemovedCredentialsStopMessages | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `configreload_chat_test.go` | TestConfigReloadChat_ReturnsBeforeOldSinkCloses | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `configreload_chat_test.go` | TestConfigReloadChat_UnchangedChatKeepsTheBatch | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `configreload_test.go` | TestConfigReload_MaxAgentsAppliesWithoutKillingAgents | any runner | Holds agents working through `agent wait`. Needs the hold hook. |
| `dirtyland_test.go` | TestDirtyLand_BackupRefHoldsTheFilesAndRefsStashIsUntouched | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyLandFixture) |
| `dirtyland_test.go` | TestDirtyLand_CheckedOutTargetOtherThanTrunkLandsToo | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyLandFixture) |
| `dirtyland_test.go` | TestDirtyLand_CleanRestoreLandsWithoutAPauseAndKeepsTheLocalContent | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyLandFixture) |
| `dirtyland_test.go` | TestDirtyLand_ConflictingRestoreStillPausesAndTouchesNothing | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyLandFixture) |
| `dirtyland_test.go` | TestDirtyLand_PruneDropsAnOldBackupAndKeepsAFreshOne | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyLandFixture) |
| `dirtyland_test.go` | TestDirtyLand_RunLogNamesTheFilesAndTheBackupRef | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyLandFixture) |
| `diskclean_test.go` | TestDiskClean_CleanupOffHoldsAndSaysSo | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_DedupesPairs | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_DefaultCommandRunsInTheRepo | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_FailureIsLoggedNoticedAndKeepsTheHold | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_HoldsWhileCleaningThenResumesWithOneCleanNotice | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_NoCleanInNormalUse | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_NoticeCarriesFreedFreeAndHoldState | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_OverrideRunsForItsProjectOnly | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_ReArmsOnlyWellAboveTheLine | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_RestartOnALowDiskWarnsHoldsAndCleansAgain | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch,startCleanRig) |
| `diskclean_test.go` | TestDiskClean_RestartWithAQueuedTicketRunsTheRealCommandOnce | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch,startCleanRig) |
| `diskclean_test.go` | TestDiskClean_TimeLimitKillsTheCommand | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_TwoProjectsOnOneDiskSendOneNotice | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskclean_test.go` | TestDiskClean_ZeroThresholdNeverCleansOrHolds | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskcleannow_test.go` | TestDiskCleanNow_ArmedStaysArmed | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskcleannow_test.go` | TestDiskCleanNow_FailureIsAnAnswer | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskcleannow_test.go` | TestDiskCleanNow_HoldEndsInTheResponse | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskcleannow_test.go` | TestDiskCleanNow_NoCleanupWorkOnlyReadsAgain | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskcleannow_test.go` | TestDiskCleanNow_Refusals | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskcleannow_test.go` | TestDiskCleanNow_RunsTheSameWorkAndStaysArmed | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskcleannow_test.go` | TestDiskCleanNow_UnheldDiskKeepsDispatching | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCleanRig) |
| `diskhold_test.go` | TestDiskHold_CleanThresholdOffNeverHolds | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_GoCacheDiskAloneHolds | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_HeldDiskListedWithWarnOff | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_LowDiskStopsClaimAndExplains | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_OneOffOnHeldDiskWaits | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_OtherDiskStillDispatches | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_PreClaimReadingHoldsBeforeTheTimer | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_RelaunchAndRetryRefused | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_ResumeStartsTicketAndNotifiesOnce | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `diskhold_test.go` | TestDiskHold_RunningAgentFinishesDuringHold | any runner | Blocks inside the RegisterLaunch turn. The turn hook must allow blocking. |
| `diskhold_test.go` | TestDiskHold_TwoProjectsOnOneDiskGiveOneEntryAndOneNotice | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startHoldRig) |
| `epicmerge_test.go` | TestEpicMerge_CommitlessEpicWithNoBranchIsStampedMerged | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `epicmerge_test.go` | TestEpicMerge_DirtyTrunkPausesTheRootAndLeavesItQueued | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `epicmerge_test.go` | TestEpicMerge_FastForwardMergesDirectlyWithAutoFFMergeOff | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `epicmerge_test.go` | TestEpicMerge_ForkingAManualTicketClearsTheStoredBlock | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixture) |
| `epicmerge_test.go` | TestEpicMerge_FrictionIncludesACancelledTicket | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `epicmerge_test.go` | TestEpicMerge_ManualTicketResetsTheCapAndResumesTheRoot | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixtureWith) |
| `epicmerge_test.go` | TestEpicMerge_MergeErrorReturnsAMergeErrorPause | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `epicmerge_test.go` | TestEpicMerge_NonFFForksAManualTicketWhoseFinishMergesTheEpic | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixture) |
| `epicmerge_test.go` | TestEpicMerge_RecreatesARemovedFeatureWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `epicmerge_test.go` | TestEpicMerge_RefusalsWriteNothing | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `epicmerge_test.go` | TestEpicMerge_RetryTicketFinishingLastAfterTheManualOneStillMerges | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixture) |
| `epicmerge_test.go` | TestEpicMerge_StampsLinesAddedAndRemovedFromTrunkBeforeToAfter | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `epicmerge_test.go` | TestEpicMerge_StampsSummedTotalsWhenEveryDoneTicketIsStamped | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `epicmerge_test.go` | TestEpicMerge_TrunkMovingDuringTheMergeAtTheCapPauses | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixtureWith) |
| `epicmerge_test.go` | TestEpicMerge_TrunkMovingDuringTheMergeForksANormalTicket | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixture) |
| `epicmerge_test.go` | TestEpicMerge_UnstampedDoneTicketLeavesTokensAndCostOffButKeepsFriction | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `epicnotice_test.go` | TestEpicNotice_AutoFFMergeOffSendsNotMerged | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `epicnotice_test.go` | TestEpicNotice_MergeSendsEpicMerged | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `epicnotice_test.go` | TestEpicNotice_NeedsRebasePauseSendsNotMerged | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `extra_usage_test.go` | TestExtraUsage_EnqueueWarnsAndNotifiesOncePerDay | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `extra_usage_test.go` | TestExtraUsage_OffOrSuppressedStaysQuiet | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `featurebranch_test.go` | TestFeatureBranch_AnExistingBranchIsAdoptedAndItsMergeBaseRecorded | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via claimFirstChild) |
| `featurebranch_test.go` | TestFeatureBranch_FirstClaimCreatesItAtTheResolvedBase | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via claimFirstChild) |
| `finish_pause_test.go` | TestFinish_ZeroCommitPauseRecordsRun | any runner | Hand-rolled herdr + fake pair; the herdr subtest asserts runner "herdr" in the pause event. Becomes pty. |
| `frictionstamp_test.go` | TestFrictionStamp_MergedTicketCarriesItsPause | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `handmerged_test.go` | TestHandMerged_PausedRootMergedByHandIsDequeuedWithEpicMerged | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `handmerged_test.go` | TestHandMerged_PlainGitMergeReleasesAnOnMergedDependent | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. |
| `install_failure_test.go` | TestInstallFailure_MergeTicketKeepsTheFeatureWorktree | any runner | Counts herdr `agent start` to prove none ran. Needs a runner-neutral start counter. |
| `install_failure_test.go` | TestInstallFailure_PausesAndDiscardsTheIterationWorktree | any runner | Counts herdr `agent start` to prove none ran. Needs a runner-neutral start counter. |
| `investigate_actor_test.go` | TestInvestigateRetry_BlockedPaneIsRefused | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `investigate_actor_test.go` | TestInvestigateRetry_LogsRecoveryAppliedNotHumanRelaunch | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `investigate_actor_test.go` | TestInvestigateRetry_NotAnInvestigateTicketIsRefused | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `investigate_actor_test.go` | TestInvestigateRetry_PastTheGuardRailIsRefused | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `investigate_actor_test.go` | TestInvestigateRetry_SecondRetryOfTheKindIsRefused | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `investigate_actor_test.go` | TestInvestigateRetry_TheForksOwnEventDoesNotSpendTheKindsCap | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `investigate_actor_test.go` | TestRetry_APersonStillLogsHumanRelaunch | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `investigate_launch_test.go` | TestInvestigateLaunch_InAVCSNoneProjectRunsInAScratchSubdir | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseAndCatchInvestigation) |
| `investigate_launch_test.go` | TestInvestigateLaunch_RunsDetachedAtTheFeatureBranchTipAndNamesTheMatchedEntry | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseAndCatchInvestigation) |
| `investigate_launch_test.go` | TestInvestigateLaunch_WithoutAFeatureBranchRunsAtTheResolvedBase | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseAndCatchInvestigation) |
| `investigate_launch_test.go` | TestOneOffLaunch_InAVCSNoneProjectRunsInAScratchSubdir | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. |
| `latefinish_test.go` | TestLateFinish_AgentGoneStillAdopts | any runner | runnerShapes. Becomes pty + headless. |
| `latefinish_test.go` | TestLateFinish_ClosesCommitless | any runner | runnerShapes. Becomes pty + headless. |
| `latefinish_test.go` | TestLateFinish_ClosesQuestionAsAdopted | not coupled | False positive of the helper scan: calls pauseZeroCommit with headless=true, which uses a fake runner. |
| `latefinish_test.go` | TestLateFinish_FailedConditionStaysPaused | not coupled | False positive of the helper scan: calls pauseZeroCommit with headless=true, which uses a fake runner. |
| `latefinish_test.go` | TestLateFinish_MergesLateCommit | any runner | runnerShapes. Becomes pty + headless. |
| `latefinish_test.go` | TestLateFinish_ResumeAndRelaunchAdoptFirst | any runner | runnerShapes. Becomes pty + headless. |
| `live_relaunch_test.go` | TestRetry_RepairPauseKeepsTheBranchHead | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `merge_finish_test.go` | TestLateFinish_MergeUnreportedClosesDoneOnceChecksPass | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestLateFinish_MergeUnreportedStaysPausedWhileChecksFail | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestLateFinish_TrunkMovedPastTheClaimTipStillClosesDone | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestMergeFinish_AgentStoppingWithoutAReportPausesStuck | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestMergeFinish_EpicLackingTheClaimTipPausesStuck | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestMergeFinish_FixerCommitOnTheEpicStillFinishesDone | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestMergeFinish_RebaseInProgressPausesStuck | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestMergeFinish_SelfReportedAskingPausesAsking | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_finish_test.go` | TestMergeFinish_TrunkMovedPastTheClaimTipStillFinishesDone | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runMergeTicket) |
| `merge_ticket_test.go` | TestMergeTicket_FailedLaunchKeepsTheFeatureWorktree | any runner | Makes herdr `agent start` fail. Needs a runner-neutral start-failure hook. |
| `merge_ticket_test.go` | TestMergeTicket_PauseKeepsTheFeatureWorktreeAndReclaimReusesIt | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newMergeFixture) |
| `merge_ticket_test.go` | TestMergeTicket_RunsInTheFeatureWorktreeAndKeepsItWhenDone | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newMergeFixture) |
| `mergedecide_test.go` | TestMergeDecide_HeldTrunkLockBlocksNeitherClaimNorPollButDefersTheStamp | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. |
| `mergedecide_test.go` | TestMergeDecide_RepeatNeedsRebasePauseIsQuiet | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeerror_test.go` | TestMergeError_DefaultPathFinishClearsTheStoredBlock | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyTrunkFixture) |
| `mergeerror_test.go` | TestMergeError_DirtyTrunkListsNewDirectoryFilesAndBothRenamePaths | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeerror_test.go` | TestMergeError_DirtyTrunkNamesOnlyTheClashingFiles | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeerror_test.go` | TestMergeError_DirtyTrunkPausesTheRootWithACappedFileList | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeerror_test.go` | TestMergeError_DirtyTrunkStoresTheFullFileListAndForksNoInvestigation | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeerror_test.go` | TestMergeError_MergeAgainPausesWithAFreshListThenLandsOnceClean | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyTrunkFixture) |
| `mergeerror_test.go` | TestMergeError_NonOverlappingLocalChangeDoesNotBlockTheMerge | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeerror_test.go` | TestMergeError_OtherGitFailurePausesMergeFailed | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeerror_test.go` | TestMergeError_PauseStoresKindAndReasonUntilALaterMergeClearsThem | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via dirtyTrunkFixture) |
| `mergefork_test.go` | TestMergeFork_DirtyTrunkAfterAMergeTicketPausesWithoutAnother | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixture) |
| `mergefork_test.go` | TestMergeFork_NonFFForksAMergeTicketWhoseFinishMergesTheEpic | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixture) |
| `mergefork_test.go` | TestMergeFork_TrunkMovingTwiceForksAThirdThenPausesAtTheCap | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newForkFixture) |
| `mergeroot_test.go` | TestMergeRoot_AutoFFMergeOffLeavesTheBranchAndDequeuesTheRoot | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeroot_test.go` | TestMergeRoot_FastForwardMergesOntoTheTarget | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeroot_test.go` | TestMergeRoot_NeedsRebasePausesTheRootWithoutRebasing | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeroot_test.go` | TestMergeRoot_NothingToMergeStampsBothRegardlessOfAutoFFMerge | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeroot_test.go` | TestMergeRoot_OpenCodeReviewTicketHoldsTheMergeUntilItIsDone | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeroot_test.go` | TestMergeRoot_StampsCompletedAtWhenAutoMergeIsOff | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeroot_test.go` | TestMergeRoot_StampsMergedAtAndCompletedAtWhenTheEpicMerges | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `mergeroot_test.go` | TestMergeRoot_StampsOnlyCompletedAtWhenTheMergePauses | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `missingtrunk_test.go` | TestMissingTrunk_EmptiedQueueForgetsTheStateSilently | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `missingtrunk_test.go` | TestMissingTrunk_UnknownBranchStopsClaimsUntilCreated | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via missingTrunkCase) |
| `missingtrunk_test.go` | TestMissingTrunk_UnreadableProjectFileStopsClaimsUntilFixed | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via missingTrunkCase) |
| `nudge_test.go` | TestNudge_Refusals | any runner | Not flagged by name. "No live pane" comes from herdr `agent get` being unregistered. On pty, Find misses and gives the same refusal. |
| `nudge_test.go` | TestNudge_TypesIntoLivePaneLogsAndRateLimits | needs missing pty feature | Find by label for an agent no run tracks: the ticket is claimed by hand and herdr finds the pane by name. Judgment call: launching first through the server also works. |
| `oneoffs_epic_test.go` | TestCompleteRootIfDone_AllDoneOneOffsEpicNeverMerges | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `oneoffs_epic_test.go` | TestRunner_OldOneOffEpicKeepsThePerRootCap | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startsWithLive) |
| `oneoffs_epic_test.go` | TestRunner_OneOffWithoutAKnownAgentUsesTheQueueItemAgent | needs missing pty feature | Codex on pty. Reads `--kind` from the herdr trace (waitForAgentStarts). Judgment call: runnerfake also fits. |
| `oneoffs_epic_test.go` | TestRunner_OneOffsEpicUsesTheOneOffCapNotThePerRootCap | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startsWithLive) |
| `oneoffs_epic_test.go` | TestRunner_OneOffsRunClaudeAndCodexUnderOneCap | needs missing pty feature | Codex on pty. Reads `--kind` from the herdr trace (waitForAgentStarts). Judgment call: runnerfake also fits. |
| `pause_question_test.go` | TestPause_AmbiguousBaseLogsGxQuestion | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `pause_question_test.go` | TestPause_ZeroCommitLogsGxQuestion | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via runToZeroCommitPause) |
| `pause_test.go` | TestFlush_IsFinishAndPause | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch,startPauseHarness) |
| `pause_test.go` | TestPause_HoldsTheQueueUntilResumeAndSurvivesRestart | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch,startPauseHarness) |
| `pause_test.go` | TestSnapshot_ReportsQueueMode | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startPauseHarness) |
| `person_cancels_investigate_test.go` | TestInvestigateRetry_LeavesItsOwnTicketRunning | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `person_cancels_investigate_test.go` | TestPersonVerb_RetryCancelsTheOpenInvestigateChild | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `person_cancels_investigate_test.go` | TestPersonVerb_RetryLeavesADoneInvestigateChild | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `queue_enqueued_at_test.go` | TestQueue_EnqueuedAt_AddRemoveMoveReplace | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startReplaceHarness) |
| `queue_enqueued_at_test.go` | TestQueue_OldStateFileLoadsWithZeroTime | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startReplaceHarness) |
| `queue_replace_test.go` | TestQueueRemove_CascadesToDescendants | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startReplaceHarness) |
| `queue_replace_test.go` | TestQueueReplace_SwapsOnlyOneProjectsEntries | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startReplaceHarness) |
| `queue_replace_test.go` | TestQueue_ReplaceAndRemoveRefuseWhileAnIterationIsLive | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startReplaceHarness) |
| `reads_test.go` | TestReads_IterationsAndQueue | herdr-specific | Asserts IterationInfo.Pane, which only herdr fills (iterations.go:82). Also relies on Find without a run. |
| `realgit_launch_test.go` | TestLaunchPort_FailureAfterClaimPausesStuck | any runner | Makes herdr `agent start` fail and counts starts. Needs a runner-neutral start-failure hook. |
| `realgit_launch_test.go` | TestLaunchPort_RestartReattachesAndMergesOnce | herdr-specific | Judgment call. Same herdr-handle restart as the runner_test restart tests; herdr branch of reclaim. |
| `realgit_scheduling_test.go` | TestSchedulingPort_ABlocksBAndCAndTheyMergeInOrder | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `realgit_scheduling_test.go` | TestSchedulingPort_CodeReviewTicketCreatesSubtickets | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `realgit_scheduling_test.go` | TestSchedulingPort_PauseThenResumeReusesBranchAndMergesBothCommitsOnce | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `realgit_scheduling_test.go` | TestSchedulingPort_TicketCreatesSubticketsAndTheRootWaitsForThem | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `reclaim_launching_test.go` | TestReclaim_LaunchingIdleAgentIsStoppedThenReopens | any runner | Hand-rolled herdr + headless pair, like runnerShapes. The herdr subtest scripts a live agent through herdr handles. |
| `reclaim_launching_test.go` | TestReclaim_LaunchingWithNoAgentReopens | any runner | runnerShapes. Becomes pty + headless. |
| `reclaim_launching_test.go` | TestReclaim_LaunchingWorkingAgentIsALiveRun | any runner | Hand-rolled herdr + headless pair, like runnerShapes. The herdr subtest scripts a live agent through herdr handles. |
| `reclaim_reopen_test.go` | TestReclaim_DeadClaimWithWorkPauses | any runner | runnerShapes. Becomes pty + headless. Its herdr shape is the only coverage of the herdr reclaim branch (runs.go:575); see the notes. |
| `reclaim_reopen_test.go` | TestReclaim_DeadEmptyClaimReopens | any runner | runnerShapes. Becomes pty + headless. Its herdr shape is the only coverage of the herdr reclaim branch (runs.go:575); see the notes. |
| `reclaim_reopen_test.go` | TestReclaim_HerdrLookupErrorNeverReopens | herdr-specific | A herdr `agent get` error during reclaim must not reopen. Herdr reclaim branch. |
| `reclaim_route_test.go` | TestReclaim_FinishesThroughTheRunsOwnRunner | any runner | Runs on renamed fakes. registerLiveAgent is dead weight. |
| `reclaim_route_test.go` | TestReclaim_HerdrRunUnderAHeadlessServerFinishesThroughHerdr | herdr-specific | A herdr run reclaimed under another configured runner. Needs the real herdrrunner. |
| `reclaim_route_test.go` | TestReclaim_NonMismatchErrorPausesStuckWithItsReason | any runner | Judgment call. Stages a herdr handle, but the error (corrupt root) does not depend on the runner. Restage as a fake or pty run. |
| `reclaim_route_test.go` | TestReclaim_UnavailableRunnerPausesStuckNamingIt | any runner | Runs on renamed fakes. registerLiveAgent is dead weight. |
| `recommend_test.go` | TestRecommend_NewPauseRoundClearsIt | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `recommend_test.go` | TestRecommend_StoredRecommendationShowsInDetail | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `recommend_test.go` | TestRecommend_VerbOutsideActionsIsRefused | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startInvestigateEnv) |
| `recovery_runner_test.go` | TestRecoveryRunner_LiveCheckAgreesAcrossShapes | herdr-specific | The herdr rows check that herdr "done" counts as live (iterations.go:74). The other rows already use fakes. |
| `recovery_test.go` | TestRecovery_ApproveAfterHandEditRefusesProposalStale | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via proposeByPausing) |
| `recovery_test.go` | TestRecovery_ApproveRetiresTheProposalSection | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via proposeByPausing) |
| `recovery_test.go` | TestRecovery_ApproveRunsTheLatestProposal | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via proposeByPausing) |
| `recovery_test.go` | TestRecovery_EnabledR3MatchForksAnInvestigateTicketNamingR3 | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `recovery_test.go` | TestRecovery_EnabledR4MatchForksAnInvestigateTicketNamingR4 | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `recovery_test.go` | TestRecovery_HighAuthorityMatchProposesWithoutActing | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via proposeByPausing) |
| `recovery_test.go` | TestRecovery_MatchingPauseRunsItsRemedyThroughAServerVerb | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startRecovery) |
| `recovery_test.go` | TestRecovery_PerTicketCapEscalatesDifferentKinds | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseAfterSeeding) |
| `recovery_test.go` | TestRecovery_R11ClobberedClaimForksAnInvestigateTicketNamingR11 | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseLaunchCollision) |
| `recovery_test.go` | TestRecovery_R12StalledReclaimForksAnInvestigateTicketNamingR12 | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `recovery_test.go` | TestRecovery_R3CommitlessDoneIsProposedAndOnlyApprovalMarksItDone | any runner | runToZeroCommitPause scripts herdr so the turn writes a transcript under session s1. Needs a turn hook that writes the transcript. |
| `recovery_test.go` | TestRecovery_R3LostCommitsEscalateWithoutAnInvestigation | any runner | runToZeroCommitPause scripts herdr so the turn writes a transcript under session s1. Needs a turn hook that writes the transcript. |
| `recovery_test.go` | TestRecovery_R6BusyPaneIsRelaunchedByARule | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseLaunchCollision) |
| `recovery_test.go` | TestRecovery_R6TakenNameForksAnInvestigateTicketNamingR6 | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseLaunchCollision) |
| `recovery_test.go` | TestRecovery_ReFailRightAfterARecoveryOfTheSameKindIsAFailedRecovery | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseAfterSeeding) |
| `recovery_test.go` | TestRecovery_RecoveredPauseLeavesMatchedAndAppliedWithKind | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startRecovery) |
| `recovery_test.go` | TestRecovery_SecondCompactionTimeoutLeavesThePauseToAPerson | needs missing pty feature | Find by label for an agent no run tracks, plus a scripted `agent wait` timeout. Judgment call. |
| `recovery_test.go` | TestRecovery_SecondFailureOfTheSameKindEscalatesInsteadOfRecovering | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseAfterSeeding) |
| `recovery_test.go` | TestRecovery_SpinningPauseIsRecordedByR1WithoutANudge | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `recovery_test.go` | TestRecovery_StalledPromptIsRetypedInFullByR5 | needs missing pty feature | Find by label for an agent no run tracks: R5 retypes into a pane the server never launched; asserts typed text and `tab close`. Judgment call. |
| `recovery_test.go` | TestRecovery_TicketOptOutNeverTriggers | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startRecovery) |
| `recovery_test.go` | TestRecovery_TimedOutCompactionIsReWaitedOnceThenFinishedUpByR7 | needs missing pty feature | Find by label for an agent no run tracks, plus a scripted `agent wait` timeout. Judgment call. |
| `recovery_test.go` | TestRecovery_UndeliverablePromptClosesThePane | needs missing pty feature | Find by label for an agent no run tracks: R5 retypes into a pane the server never launched; asserts typed text and `tab close`. Judgment call. |
| `recovery_test.go` | TestRecovery_UnrelatedPriorRecoveryDoesNotTripTheGuardRail | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via pauseAfterSeeding) |
| `recovery_test.go` | TestRecovery_ZeroCommitPauseMatchesOnItsTranscriptText | any runner | runToZeroCommitPause scripts herdr so the turn writes a transcript under session s1. Needs a turn hook that writes the transcript. |
| `recovery_test.go` | TestRecovery_ZeroCommitPauseWithR2DisabledInvestigatesWithoutR2Events | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `resume_web_test.go` | TestDetail_AmbiguousBaseListsTheCompetingBlockers | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `resume_web_test.go` | TestWebResume_AmbiguousBaseMustBeACompetingBlocker | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `resume_web_test.go` | TestWebResume_ZeroCommitWithALateFinishIsAdopted | any runner | Calls pauseZeroCommit with headless=false (the herdr shape). Late-finish logic does not depend on the runner. |
| `retrohook_test.go` | TestRetroHook_SubmitsWhenTheMergeErrors | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `retrohook_test.go` | TestRetroHook_SubmitsWhenTheMergeNeedsARebase | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `runner_flush_test.go` | TestHerdr_RunOnANonConfiguredHerdrIsNotProbed | herdr-specific | Asserts a live run on a non-configured herdr is NOT probed. The map decided the opposite, so this test must flip. |
| `runner_flush_test.go` | TestStart_LogsRunsStillOnANonConfiguredRunner | any runner | Runs on renamed fakes. The registerLiveAgent call inside restartUnder is dead weight. |
| `runner_test.go` | TestExplain_NamesEveryFullCapWithItsCount | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `runner_test.go` | TestExplain_QueuedTicketBehindAFullLimitExplainsTheCapThenOneChangeStreamsWhenTheSlotFrees | any runner | Overrides herdr `agent wait` to keep agents working and holding their slot. Needs a runner-neutral "hold the turn" hook. |
| `runner_test.go` | TestRunner_AFinishThatErrorsPausesTheTicketStuckAndFreesTheRoot | any runner | Overrides herdr `agent prompt` to run the agent turn (commit or wreck the worktree). Needs the turn hook. |
| `runner_test.go` | TestRunner_APauseSendsOnePrefixedChatMessageNoMatterHowManyClientsWatch | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `runner_test.go` | TestRunner_AmbiguousBasePausesWithoutLaunching | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `runner_test.go` | TestRunner_BackfillsTheNextQueuedRootWhenASlotFrees | any runner | Overrides herdr `agent prompt` to run the agent turn (commit or wreck the worktree). Needs the turn hook. |
| `runner_test.go` | TestRunner_ClaimsAnotherTicketOfARootOnlyBelowThePerRootCap | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `runner_test.go` | TestRunner_ClaimsWriteTheFileBeforeTheEventAndLaunchWithTheChosenAgent | needs missing pty feature | Codex on pty. Asserts `--kind codex` in the herdr `agent start` argv. pty always runs claude. Judgment call: it only checks start options, so runnerfake (ticket 03) also fits. |
| `runner_test.go` | TestRunner_CodeReviewTicketLaunchesUnderTheCodeReviewSkill | any runner | Judgment call. Queues as codex but checks only the prompt text, which a turn hook sees. On pty the agent is claude under a codex label. |
| `runner_test.go` | TestRunner_CodexEvolveLaunchGetsTheFollowUpsAndGxDirs | needs missing pty feature | Codex on pty. Asserts codex `--add-dir` flags in the start argv. Judgment call: runnerfake also fits. |
| `runner_test.go` | TestRunner_GlobalCapIsSharedAcrossProjectsInFIFOOrder | any runner | Overrides herdr `agent wait` to keep agents working and holding their slot. Needs a runner-neutral "hold the turn" hook. |
| `runner_test.go` | TestRunner_LaunchSkillFollowsTicketType | any runner | Judgment call. Queues as codex but checks only the prompt text, which a turn hook sees. On pty the agent is claude under a codex label. |
| `runner_test.go` | TestRunner_MergesTheIterationAndClosesTheRoot | any runner | Overrides herdr `agent prompt` to run the agent turn (commit or wreck the worktree). Needs the turn hook. |
| `runner_test.go` | TestRunner_ProjectCapHoldsItsProjectAndAnEditAppliesLive | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `runner_test.go` | TestRunner_RefusesAClaimWhenTheFileChangedUnderTheIndexThenClaimsOnTheNextPass | any runner | Waits for `!HerdrUnavailable` only to dodge a probe race. On pty there is no probe and the wait passes at once. |
| `runner_test.go` | TestRunner_RestartPausesAHandleWhoseWorktreeIsGone | herdr-specific | Judgment call. Stages a herdr handle (pane/tab) and restarts, so it runs the herdr branch of reclaim (runs.go:575, Find by label). pty reclaims through Reattach instead. |
| `runner_test.go` | TestRunner_RestartReclaimsTheLiveIterationAndItFinishes | herdr-specific | Judgment call. Stages a herdr handle (pane/tab) and restarts, so it runs the herdr branch of reclaim (runs.go:575, Find by label). pty reclaims through Reattach instead. |
| `runner_test.go` | TestRunner_SkipsAFrontierTicketThatIsAlreadyTracked | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `server_notify_test.go` | TestCompleteRootIfDone_ResolvedCrossEpicBlockersCompleteTheEpic | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestCompleteRootIfDone_UnresolvedCrossEpicBlockerKeepsTheEpicOpen | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestNotifyDone_CrossEpicBlockersDoNotUndercountDoneTickets | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestNotifyDone_EpicTicketSendsOneOffStaysSilent | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestNotifyPause_SelfReportedShowsQuestionSummary | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestNotifyPause_ShowsRealStatusKindAndReason | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestNotifyResult_OnlyWhenTheSubmitAskedForIt | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestPauseFold_HerdrOutagePausesBecomeOneDigest | herdr-specific | Subject is the herdr-outage pause digest. Sets s.herdr.unavailable directly; herdrfake only answers the probe. |
| `server_notify_test.go` | TestProjectEpics_CrossEpicBlockerReadsAsDone | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerChat_EmptyGateStatePathUsesTheStateDir | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerNotices_BudgetAlreadyOverAtStartIsNotAnnounced | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerNotices_BudgetLimitLatchesOncePerDay | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerNotices_BudgetLimitsCountSpendBeyondOverride | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerNotices_DayRolloverSendsTheSummary | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerNotices_IncreaseRearmsAgainstSpendBeyondOverride | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerNotices_RestartAfterOverrideSeedsFromSpendBeyondIt | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_notify_test.go` | TestServerNotices_RestartSendsOneServerLevelMessage | any runner | Builds a Server with New and the default (herdr) runner; herdrfake only answers the startup probe. Set a non-herdr runner or rely on the gated probe, then drop the fake. |
| `server_test.go` | TestHandshake_ReportsHerdrUnavailable | herdr-specific | Subject is the herdr probe: herdr down at start, retry, events and handshake flag. |
| `server_test.go` | TestHerdr_DownAtStartIsReportedAndRecoveryStreamsOneEventEach | herdr-specific | Subject is the herdr probe: herdr down at start, retry, events and handshake flag. |
| `server_test.go` | TestHerdr_NotProbedForRunnerWithoutHost | herdr-specific | Subject is probe gating on the configured runner. Changes with the probe decision. |
| `server_test.go` | TestHerdr_ProbedThroughConfiguredRunner | herdr-specific | Subject is the herdr health flag. Already runs on runnerfake (SetHealthErr), no herdr fake calls. |
| `servertest_parallel_test.go` | TestServertest_ParallelHarnessesEachDriveAHerdrLaunch | any runner | Harness self-test: parallel harnesses each drive one launch. Rename and run it on pty. |
| `stashmerge_test.go` | TestStashMerge_AcceptsTheCardsMergeBlockedState | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixture) |
| `stashmerge_test.go` | TestStashMerge_LeavesMarkersUncommittedAndBacksUpAFileThatClashesAtClickTime | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixture) |
| `stashmerge_test.go` | TestStashMerge_RecordsTheBackupOnTheRootAndTheRunLogAndKeepsTheRootUnmerged | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixture) |
| `stashmerge_test.go` | TestStashMerge_RefusesAnEpicThatIsNotPausedOnADirtyTrunk | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `stashmerge_test.go` | TestStashMerge_WebRefusesAStaleTokenAndTouchesNothing | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixture) |
| `stop_test.go` | TestStop_LeavesALiveAgentRunningAndUnmerged | any runner | Holds the agent working through `agent wait` and asserts no `tab close`. Needs a hold hook and a stop counter. |
| `stop_test.go` | TestStop_WaitsForAMergeInFlightAndFlushesTheStoreCommit | any runner | Commits on `agent prompt` and blocks herdr `tab close` to hold the merge. Needs turn and stop hooks. |
| `ticketwrites_test.go` | TestCancel_LiveTicketRefusesUntilStop | any runner | Puts a herdr run and counts `tab close` as the stop. The runner-neutral twin is TestCancel_StopClosesALeftoverThroughTheRunner. |
| `ticketwrites_test.go` | TestRelaunch_StartsAFreshIterationOfAPausedTicket | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `trunk_merging_test.go` | TestTrunk_DivergedNewTrunkNeedsRebase | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via trunkFixture) |
| `trunk_merging_test.go` | TestTrunk_EditedBetweenClaimAndMergingMergesOnTheNewTrunk | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via trunkFixture) |
| `trunk_merging_test.go` | TestTrunk_ExplicitBaseMovesOnlyTheStart | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via trunkFixture) |
| `trunk_merging_test.go` | TestTrunk_TopLevelWorkStartsFromAndMergesOnTrunk | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via trunkFixture) |
| `trunk_missing_merging_test.go` | TestTrunkMissingAtMerging_AutoMergeOffIsNotPaused | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via trunkDeletedMidRun) |
| `trunk_missing_merging_test.go` | TestTrunkMissingAtMerging_TicketMergesOnItsRootAndTheRootPauses | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via trunkDeletedMidRun) |
| `trunkconflict_claim_test.go` | TestTrunkConflict_AnotherEpicsLandWaitsUnpausedUntilTheTicketIsDone | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixtureWith) |
| `trunkconflict_claim_test.go` | TestTrunkConflict_TheAgentRunsInTheTargetCheckoutAndTheEpicLands | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixtureWith) |
| `trunkconflict_finish_test.go` | TestTrunkConflict_MarkersLeftPauseTheTicketAndKeepTheBackup | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixtureWith) |
| `trunkconflict_finish_test.go` | TestTrunkConflict_NoMarkersLeftMakesTheTicketDoneDeletesTheBackupAndLandsTheEpic | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixtureWith) |
| `trunkconflict_finish_test.go` | TestTrunkConflict_PruneKeepsAnOldRefOfAnOpenTicket | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixtureWith) |
| `trunkconflict_finish_test.go` | TestTrunkConflict_StartOverRestoresTheBackupMarksDoneAndLandsTheEpic | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via stashMergeFixtureWith) |
| `trunklock_test.go` | TestTrunkLock_AlreadyMergedEpicDoesNoGitWorkAndNotifiesOnce | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `trunklock_test.go` | TestTrunkLock_TrunkMovedBeforeTheFFPausesNeedsRebaseNotMergeFailed | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `trunklock_test.go` | TestTrunkLock_TwoEpicsFinishingTogetherMergeOneAndPauseTheOtherAsNonFF | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `unavailable_test.go` | TestUnavailable_MissingPathExplainsNotifiesOnceAndSetPathRecovers | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via registerLaunch) |
| `verbs_route_test.go` | TestAgentVerbs_ReachARunOnAnotherRunner | any runner | Runs on renamed fakes. The registerLiveAgent call inside restartUnder is dead weight. |
| `verbs_route_test.go` | TestBudget_KillStopsTheSessionThroughTheRunsRunner | any runner | Runs on renamed fakes. The registerLiveAgent call inside restartUnder is dead weight. |
| `verdicts_epic_test.go` | TestPending_TicketQueuedAloneAndItsWholeEpicGiveOneRow | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCapHeldEpics) |
| `verdicts_epic_test.go` | TestPending_WholeEpicEntryGivesOneRowForItsFrontierTicket | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCapHeldEpics) |
| `verdicts_epic_test.go` | TestPending_WholeEpicRowStreamsAVerdictChange | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startCapHeldEpics) |
| `web_recovery_test.go` | TestWebDetail_RepairPauseCarriesAStateToken | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startRecoveryWeb) |
| `web_recovery_test.go` | TestWebRecoveryVerbs_OldTokenIsStaleCurrentRuns | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startRecoveryWeb) |
| `web_recovery_test.go` | TestWebStartOver_DeleteFlagKeepsTheBranch | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via startRecoveryWeb) |
| `worktree_removal_live_test.go` | TestWorktreeRemoval_ExitedSessionDoesNotKeepTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newLiveWorkFixture) |
| `worktree_removal_live_test.go` | TestWorktreeRemoval_FinishedMergeTicketDoesNotKeepTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newMergeFixture) |
| `worktree_removal_live_test.go` | TestWorktreeRemoval_LiveRunInTheWorktreeKeepsItAndNamesTheRun | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newMergeFixtureWith) |
| `worktree_removal_live_test.go` | TestWorktreeRemoval_LiveSessionKeepsTheWorktreeUntilItStops | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newLiveWorkFixture) |
| `worktree_removal_live_test.go` | TestWorktreeRemoval_RunInASiblingWorktreeDoesNotKeepTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_live_test.go` | TestWorktreeRemoval_RunWithoutCwdKeepsTheWorktreeOfItsEpic | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_live_test.go` | TestWorktreeRemoval_StoppedRunStillRegisteredDoesNotKeepTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_CompletedUnmergedEpicKeepsItsWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_FastForwardMergeRemovesTheWorktreeAndKeepsTheBranch | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_ManualMergeRouteRemovesTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_NothingToMergeRemovesTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_OneOffsRootKeepsItsWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_OpenTicketKeepsTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_QueuedRootKeepsTheWorktree | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newSchedFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_UntrackedFileKeepsTheWorktreeAndLogsWhy | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |
| `worktree_removal_test.go` | TestWorktreeRemoval_WaitsForTheWorktreeLockAfterTheMergeDequeuedTheRoot | any runner | Uses the herdr launch fake only as the default agent. Needs the harness's runner-neutral turn hook. (via newFFFixture) |

### runnerShapes

`runnerShapes` (`reclaim_reopen_test.go:65`) is called by 7 tests: `TestReclaim_DeadEmptyClaimReopens`, `TestReclaim_DeadClaimWithWorkPauses`, `TestReclaim_LaunchingWithNoAgentReopens`, `TestLateFinish_MergesLateCommit`, `TestLateFinish_ClosesCommitless`, `TestLateFinish_AgentGoneStillAdopts` and `TestLateFinish_ResumeAndRelaunchAdoptFirst`. Three more tests build the same herdr and fake pair by hand: `TestReclaim_LaunchingIdleAgentIsStoppedThenReopens`, `TestReclaim_LaunchingWorkingAgentIsALiveRun` and `TestFinish_ZeroCommitPauseRecordsRun`.

Its "headless" shape is not the real headless runner. It is `headlessFake` or `turnRunner`, both built on `runnerfake`.

## Tests to check in the spike run

These tests did not match the scan, but they queue, restart or nudge on the default runner without registering herdr handlers. Today a launch among them fails on "herdrfake: unimplemented command". On pty a launch would start a holder. Most queue tickets in projects without a repo, so they likely never launch. Check them when the harness switches.

- `budget_latch_test.go` TestBudgetLatch_SurvivesRestartAndOverrideLiftsIt
- `deadlock_test.go` TestDeadlock_AllPausedEpicLogsDeadlockedOncePerEntry
- `deadlock_test.go` TestDeadlock_ErrorTicketPausesTheEpic
- `deadlock_test.go` TestDeadlock_BlockedOrWaitingNextToPausedIsAllPaused
- `deadlock_test.go` TestDeadlock_AnswerOnlyPausesLogWaitingForAnswer
- `deadlock_test.go` TestDeadlock_MapEpicIsNeverDeadlocked
- `deadlock_test.go` TestDeadlock_BlockedRecordNamesTheBlockers
- `epic_queued_at_test.go` TestQueueAdd_StampsQueuedAtOnceAndKeepsIt
- `epic_queued_at_test.go` TestQueueAdd_DraftImplEpicRefusesEpicDraftAndWritesNothing
- `epiccard_test.go` TestEpicList_LoopCardFields
- `epiccard_test.go` TestEpicList_QueuedOneOffSortsBySlotAndReadsQueuedN
- `epiccard_test.go` TestEpicList_SectionSortOrders
- `epiccard_time_test.go` TestEpicList_CardTimePair
- `epicdetail_test.go` TestEpicDetail_Queueable
- `epiclist_test.go` TestEpicList_SectionRules
- `epiclist_test.go` TestEpicList_OneOffCardsAndTheFinishedCap
- `joinedcard_test.go` TestEpicList_DoneJoinedCardWithQueuedImplSitsInProgress
- `joinedcard_test.go` TestEpicList_JoinedCardCarriesImplQueueSlot
- `oneoff_test.go` TestOneOff_FromProjectRepoCreatesPromptTicketAtQueueTail
- `oneoff_test.go` TestOneOff_FrontQueuesAtHead
- `oneoff_test.go` TestOneOff_NumbersAboveTheArchiveAndQueuesTheRootOnce
- `oneoff_test.go` TestOneOff_FrontMovesAQueuedRootToTheHead
- `oneoffsweep_test.go` TestSweep_BrokenOneOffsKeepTheRootQueued
- `queue_test.go` TestQueue_WritesChangeReadStreamAndSurviveRestart
- `queue_test.go` TestQueue_Refusals
- `queue_test.go` TestQueue_AddAcceptedWithoutConfig
- `recentlyfinished_test.go` TestSnapshot_RecentlyFinishedIsCappedOrderedAndSkipsMaps
- `recovery_test.go` TestRecovery_UnmatchedPauseForksAQueuedInvestigateChildAndNeverRecurses
- `retry_prompt_test.go` TestRetry_ZeroCommitWithALiveAgentPromptsAndKeepsThePause
- `retry_prompt_test.go` TestRetry_BlockedPaneWithALiveAgentPrompts
- `retry_prompt_test.go` TestRetry_BudgetKilledDoesNotPromptTheAgent
- `web_queue_add_test.go` TestWeb_QueueAddEpicAddress
- `web_queue_add_test.go` TestQueue_MoveAndRemoveAcceptAnEpicAddress
- `web_queue_add_test.go` TestQueue_AddEpicAddressQueuesOneEntry
- `web_test.go` TestWeb_TokenChangeInvalidatesOldCookies
- `web_test.go` TestWeb_TokenFileIs0600AndSurvivesRestart

## Where the server probes herdr today

The probe is `herdr workspace list` (`herdr/workspace.go:75`, `CLI.Ping`), reached through `herdrrunner.Runner.Healthy` (`agentrunner/herdrrunner/herdrrunner.go:85`). Today it depends on the configured runner, not on what projects or live runs use:

- `server.New` calls `s.checkHerdr()` once at startup (`server/server.go:489`). It probes `cfg.Runner`, which is herdr whenever the config leaves it nil (`server/server.go:316`).
- `keepHerdrChecked` re-probes `cfg.Runner` every `HerdrRetryInterval`, default 30s (`server/herdrwatch.go:66`, started at `server/server.go:768`). It runs only when `cfg.Runner` is a `HealthChecker`.
- `reclaim` calls `agentrunner.Healthy` on the run's own runner when a server restarts over a live run (`server/runs.go:585`). This one already follows the run's runner.
- Outside the server: `cmd/server_preflight.go:26` pings herdr while resolving `agent-runner: auto`, and `cmd/ui.go:328` pings herdr for the TUI's herdr-only features.

The result gates these places, whatever runner a project uses: claims (`server/runner.go:222`), retry and relaunch (`server/ticketwrites.go:249`), the pause digest (`server/pause_ticket.go:42`, `:52`), the snapshot and handshake flag (`server/server.go:618`, `:720`). Nudge already checks that the iteration's runner is herdr first (`server/nudge.go:68`).

Launches on herdr also call `workspace list` through `FindOrCreateWorkspace` (`herdr/workspace.go:12`). That is part of a herdr launch, not a probe.

## Notes that affect the design

- `TestHerdr_RunOnANonConfiguredHerdrIsNotProbed` (`server/runner_flush_test.go:29`) asserts today's rule: a live run started by herdr under another configured runner is not probed. The map's decision reverses this, so the test must change with the probe change.
- Under pty, `runnerShapes`'s two shapes would both go through `reclaimHeadless` and `Reattach`. Today its herdr shape is the main coverage of the herdr branch of reclaim (`server/runs.go:575`: `Find` by label, `reopenEmptyClaim`). If `runnerShapes` becomes pty and headless, keep one herdr test per reclaim outcome (dead empty claim reopens, dead claim with work pauses, launching with no agent reopens) in the herdr-specific set. Otherwise that branch loses its coverage.
- pty ignores `StartOptions.Kind`. A ticket queued for Codex on a pty server runs Claude and records "codex". The 4 Codex tests check server-built start options, so runnerfake (ticket 03) fits them better than herdr. The cause, pty not hosting Codex, belongs to `gx:pty-as-default`.
- The 6 "Find by label" tests set up an agent the server never launched. On pty that agent can only exist if this process started it or reattached it. In production this also means a paused ticket's live pty agent cannot be nudged after a server restart. The clean fix is for pty `Find` to look at the agent directory (live holder socket and `meta.json`), the way `Reattach` already does. The tests could also launch through the server first, but that hides the gap.
- 5 herdr-specific tests are judgment calls: the three restart tests that stage a herdr handle, `TestLaunchPort_RestartReattachesAndMergesOnce`, and `TestBudget_HardLimitStopsLivePaneAndPausesBudgetKilled`. Their subject is generic restart or budget logic. Restaged on pty they would cover the same logic through `Reattach` and `Interrupt`.
