## Problem Statement

A developer runs `gx check --full`. One Go test fails under that load and passes alone: `TestBlockedScenario_EachKindOfQuestionListsItsCard`. The `agents-blocked` fixture starts once per `go test` run and keeps its state for the whole run. When that one start loses the race, the test fails in all 20 iterations of a `-count=20` run. About 1 start in 4 loses it under load (6 of 24 in the research). The failure reads "question session N lists [], want a permission card".

The cause is a race inside the fixture seeder, not in the product. The seeder prompts a fake Claude TUI session. The fake appends its reply, then starts a goroutine that sends its own `ActionFinish` after `DesignTurnTime` (500 ms). The seeder waits for the agent reply, then for the server to show Idle. Under load the server's status lags the fake. The Idle wait passes on the Idle from before the turn, and the seeder sends its Block control. The fake's delayed Finish then lands, sets the pid file to Idle, and wipes the Block. The session is Idle with no dialog. Nothing in the fixture blocks it again.

The other 18 map tickets passed 20/20 under `gx check --full`, or (18) were machine load. They need no work.

## Solution

The seeder waits for the end of the turn before it sends any control. The fake's delayed Finish then always comes before the seeder's Block or tool-use control, at any load. No timeout, window, sleep or `DesignTurnTime` changes.

Every seed that sends a control or another prompt after a chat turn gets the same wait, so sibling seeds in this fixture and other scenarios stop sharing the race.

## User Stories

1. As a developer running `gx check --full`, I want `TestBlockedScenario_EachKindOfQuestionListsItsCard` to pass under load, so that a green check means the code is good.
2. As a developer, I want `TestBlockedScenario_BlockedSessionsSettleBlocked` to pass under load, so that it does not flake on the same race.
3. As a developer, I want a session that its spec seeds as Blocked to still be Blocked after the fake's turn timer fires, so that the scenario fixture matches its spec.
4. As a developer, I want a session seeded with a plan dialog to keep its plan card, so that plan-approval tests do not depend on timing.
5. As a developer, I want a session seeded with a permission, question or round dialog to keep its card, so that each kind of question lists its card.
6. As a developer, I want a session seeded as Read to stay read after its turn ends, so that a late Finish does not raise Unread after the seeder marks it read.
7. As a developer, I want a session seeded with a tool-use control to start that control after the turn ended, so that the calls in flight are not cleared by a late Finish.
8. As a developer, I want a follow-up prompt step (`ThenPrompt` or `Turn: true`) to start only after the previous turn ended, so that chained turns do not overlap.
9. As a developer, I want the turn-end wait to look at the fake's own turn state, not only at the server's status, so that a lagging server cannot fool it.
10. As a developer, I want the wait to fail with the fake's pid-file status and the reply count in the message when the turn never ends, so that a real hang is diagnosable.
11. As a developer, I want sessions that are meant to stay Working (the slow marker) to skip the turn-end wait, so that they still seed as Working.
12. As a developer, I want Codex sessions, where the seeder sends Finish itself, to keep their current order, so that no wait blocks on a turn that only the seeder can end.
13. As a developer, I want the fix to leave fixtures with no prompt unchanged, so that cheap seeds stay cheap.
14. As a developer, I want the fix to add no fixed sleep, so that the fixture start-up time does not grow.
15. As a developer, I want no timeout, poll window or turn time raised, so that the fix is the cause, not a mask.
16. As a developer, I want a stress run of the failing test next to `gx check --full` to pass, so that I can trust the fix.
17. As a maintainer, I want the map closed with 01 to 14 and 16 to 20 recorded as fixed by earlier work or machine load, so that the history stays accurate.

## Implementation Decisions

- The change is in the fixture session seeder. It is test infrastructure only. No product code, wire type, config key or Migration step changes.
- The seeder gains one step, a turn-end wait, that runs after the reply wait and before the first control or follow-up prompt. It applies to every non-slow, non-Codex session that has a prompt. The turn-end wait goes in before the existing server Idle wait. The server Idle wait stays, because scenario tests read server state.
- The turn is over when the fake's own turn state says so, not when the server says so. The fake appends its reply after it sets Busy and before its delayed Finish sets Idle. A reply in the chat with the fake's state Idle therefore means the delayed Finish already ran. The seeder reads the fake's state through the fake's existing pid file, which the fake already writes with a status and a status-updated time. This reading mechanism is an implementer's choice. The requirement is that the wait cannot pass on the Idle from before the turn and does not rely on observing a short Working window, which a lagging server can miss. The seeder reads the fake's state after the poll that saw the new reply, never before. Otherwise it can take the Idle from before the turn and then see the reply, which is the race again.
- The fake keys its pid file by OS pid, not by session, and `/clear` changes the conversation id to `<id>-cleared`. The seeder finds the pid file by its `sessionId` field and rereads the conversation id on each poll, as `controlSender` does. The helper is the implementer's choice.
- The same wait guards the chained follow-up prompts, the steps that go through `promptAgain` (`ThenPrompt` and `Turn: true`). They currently wait for "Idle and a new reply", which has the same stale-Idle shape. They use the turn-end wait, counted against the replies seen before the new prompt. Plain "/" steps start no turn and send no delayed Finish, so they keep their current wait. `Blocks` steps stay Waiting and never end a turn, so they rely on the wait after the first prompt.
- The seeder's existing fixed 300 ms sleeps after a control stay. They give a control FIFO time to land and are not part of this race. Removing them is a separate change.
- The fake does not change. The person chose the seeder-only fix for its smaller blast radius. A fix in the fake (the delayed Finish does nothing once the status changed since its turn began) would also cover other `SendTUIControl` callers, such as `servertest/ptyrunner.go` and `servertest/turn.go`. It is a larger change and may break a test that needs the stale Finish to clear a Block.
- The proof counts fixture starts, because `-count=20` repeats one start. See Verification.
- Names, helper names and failure-message wording are the implementer's choice.

## Testing Decisions

- A good test here checks the seeded state a scenario test reads, not the order of the seeder's internal calls.
- The seam is the existing one: the `agents-blocked` scenario fixture and the designflows scenario tests that read it. No new seam.
- The race is a timing race, so it has no cheap deterministic unit test at that seam. The proof is the stress run under load. A seeder-level test may drive a fake whose Finish is delayed past the point where the server shows Idle, if it can do so without a sleep or a raised window. This is optional.
- Prior art: the scenario tests in the designflows package, including the ones already named, and the stress-run method in the research note for this map.
- Follow the gx-go-tests rules: wait on the event the code needs, no fixed sleeps for synchronization, no raised timeouts.

## Verification

The change has no web, CLI or server behavior. Stories 1 to 17 are checked by Go stress runs, not by flows or verb checks. The flow catalog has no flow to run for any of them. A verify ticket runs these next to a running `gx check --full`. The fixture starts once per `go test` run, so `-count=20` repeats one start. The unfixed fixture lost the race in 6 of 24 separate starts, so the proof counts starts.

Fixture seeding (stories 1 to 15)
- Run `gx brief go-test -count=1 -run '^TestBlockedScenario_EachKindOfQuestionListsItsCard$|^TestBlockedScenario_BlockedSessionsSettleBlocked$' ./testutil/gxfixture/designflows` as 24 separate runs. Expected: 24/24 pass, so 0 Blocked-spec sessions show Idle. The unfixed rate is 6 of 24.
- Run the same two tests once with `-count=20`, the Destination's literal check. Expected: 20/20 pass.
- Run `gx brief go-test -count=1 ./testutil/gxfixture/designflows -run '^TestToolsScenario_ChatListsTheCallsInFlight$'` and one test of a chained-turn fixture (`commandStepsSession`, or the contextwindow `/compact` session) in the same loaded runs. Expected: pass. They cover stories 7 and 8.
- Run `gx brief go-test ./testutil/gxfixture/...` once. Expected: all pass, so the other scenarios that use the seeder are unchanged.
- In the same loaded runs, read `gx agents list --json` for the Read sessions of `agents-blocked` and the contextwindow `compact` session. Expected: `unread: false`. It covers story 6.
- Read the diff for stories 9, 10, 11, 14 and 15: the wait reads the fake's pid-file state after the poll that saw the reply, slow sessions skip it, the timeout error holds the fake's pid-file status and the reply count, there is no new `time.Sleep`, and no timeout, poll window or `DesignTurnTime` changes.
- New flows or scenarios: none.

Stress run and map close (stories 16 and 17)
- Run the 24-start loop twice in total, each one while a fresh `gx check --full` is still in its test phase. Rerun a loop that outlives the check.
- Close the map when both loops pass. Expected: tickets 01 to 14 and 16 to 20 stay closed as recorded.
- New flows or scenarios: none.

## Out of Scope

- Tickets 01 to 14 and 16 to 20 of the map: they passed 20/20 under load, or (18) were machine load. The load preflight in gx:follow-ups/624 covers 18.
- The nine promptflows image tests and `TestRouteTableHasCLIVerbs`.
- Removing the fixed 300 ms sleeps after seeder controls.
- Changing the fake's delayed Finish, or other fakes' turn timers.
- Playwright and Vitest flakes.

## Further Notes

- The root cause was read from code and reproduced as a symptom (6 of 24 fixture starts left a Blocked-spec session Idle under load). One run's event trace was not taken. The stress run is the check on the diagnosis. If it still fails after the fix, trace one run's events before widening the fix.
- Full analysis: the research note on stress runs for this map, sections "Ticket 15" and "Root-cause fix".
