## Problem Statement

Four things go wrong around the life of an agent session that gx launches, and a fifth blocks the tests that would catch them.

- A launched Claude can get stray text in its input box. A lone `i` is typed when gx guesses wrong about vim mode. A leftover `i` then makes the next launch fail with "input not empty", and the ticket pauses.
- The launch prompt can be rejected without gx noticing. Claude answered `Unknown command: /gx-implement`, started no turn, and gx counted "no turn, agent idle" as delivered. The agent idled for 38 minutes, then the ticket paused `zero-commit` and an investigate ticket was spawned for nothing.
- A ticket that paused for an answer loses the agent's uncommitted edit on relaunch, because gx removes the worktree. This happens on the herdr runner (and when no runner is set). PTY and headless Claude and PTY Codex already keep the worktree, pane and session when they ask. The next herdr session redoes the work.
- The disk cleaner runs `go clean -cache` while `gx check` or `gx brief go-test` is compiling. Builds fail with "could not import ... no such file or directory". Agents lose calls and work around it with a private cache.
- In the ralph-loop agent sandbox, every test that starts `fakeclaude` through the PTY holder fails with "holder.sock never answered". The error hides the cause, so `gx check` cannot pass there.

## Solution

Each fix removes its cause. gx also notices a failed launch and recovers by itself. It pauses only as a last resort, and a web button ends that pause.

- Every gx-launched Claude starts in normal editor mode, and gx no longer guesses at vim mode by typing into the box.
- A launch prompt counts as delivered only when it started a turn. gx resends for a short grace period, then retries in a fresh session, then pauses (`agent_prompt_stalled` or `iteration-error`).
- A ticket that paused for an answer on the herdr runner keeps its worktree. The relaunch reuses it as is. Cancelling a ticket and merging its epic remove leftover iteration worktrees. The same keep applies to every run with `KeepAskedAgent == false` (herdr, empty and the commitless ask path), Codex included.
- One wait gate (a file lock) guards every disk-cleaner cache clean. A new `gx brief go-clean-cache` command uses the same gate. `gx check` and every `gx brief go-*` tool retry once on a lost cache and say so when it fails again.
- The PTY holder reports why it failed to start, and the fixtures use short paths. The sandbox cause is still unconfirmed. The next failing run names the blocked operation, and a `draft` ticket in `agent-lifecycle-impl` adds the sandbox allowance once a person promotes it.

## User Stories

Go build cache

1. As an agent running `gx check`, I want the disk cleaner never to delete build-cache files while my build runs, so that a green tree does not fail with "could not import".
2. As a person, I want the automatic clean and "Clean now" both to run the configured cleanup command (`disk-space.cleanup-command`, default `go clean -cache`) only after the gate opens, so that neither path breaks a build.
3. As a person, I want the gate to treat every registered gx run (`gx check`, every `gx brief go-*` tool such as build, vet and test, and ticket checks) as a running Go build, so that gx's own work is always protected.
4. As a person, I want the gate to also treat any `go` process found by a process scan as a running build, so that a build gx did not start is protected too.
5. As an agent in the sandbox, I want the gate to need no round trip to the server, so that it works when the server is down or unreachable from the sandbox.
6. As a person, I want the wait to be capped at about 10 minutes, so that a stuck process cannot hold the disk cleaner forever.
7. As a person, I want gx to log it when the cap passes and the clean goes ahead, so that a later build failure can be traced to it.
8. As an agent, I want a `gx brief go-clean-cache` command that waits on the same gate and then cleans, so that I can free the cache without breaking another session.
9. As an agent, I want agent docs that say "never run `go clean -cache` directly, use `gx brief go-clean-cache`", so that no session bypasses the gate.
10. As an agent, I want `gx check` and every `gx brief go-*` tool to retry once when they see "could not import ... (open ...go-build...: no such file or directory)", so that a rare lost-cache event costs one retry and not six calls.
11. As an agent, I want the message after a failed retry to say the build cache was lost, so that I do not hunt for a code error that does not exist.
12. As a maintainer, I want the disk-space spec's "Running builds are not stopped during a clean" rule changed to match the gate, so that the docs do not promise the old behavior.

Kept worktree (the `KeepAskedAgent == false` path: the herdr runner and an empty runner)

13. As an agent on the herdr runner relaunched after an adopted ask, I want the worktree from my earlier session still in place, so that my uncommitted edit is still there.
14. As an agent, I want the relaunch to reuse the worktree as it is (no reset, no clean), so that nothing I left is touched.
15. As an agent, I want the handoff prompt not to claim that uncommitted work is lost, so that I am not told to commit half-done edits.
16. As a person, I want a paused ticket to hold no permit, so that a ticket waiting for my answer does not block the queue.
17. As a person, I want only a done merge, start over, cancel and an epic merge to remove a kept worktree, so that no timer deletes work I have not looked at.
18. As a person, I want cancelling a ticket to remove its leftover iteration worktree, so that a ticket I gave up on does not leave a worktree behind.
19. As a person, I want merging an epic to remove the leftover iteration worktrees of its tickets, so that a merged epic leaves no worktree behind.
20. As a person, I want a worktree I removed by hand to be recreated from the iteration branch, so that this stays as it is today.
21. As a person, I want the iteration branch to stay when the worktree is kept, so that commits are never at risk.
22. As a person on a runner that already keeps its agent (PTY or headless Claude, PTY Codex), I want nothing to change, so that a working path is not touched.

Launch input

23. As an agent, I want every Claude gx launches to start in normal editor mode without losing gx's `guard-bash` hook, so that my first prompt is typed into an input that accepts text and the guard still runs.
24. As a maintainer, I want the implementer to verify with a real Claude that the launch setting beats the person's own vim setting, so that the fix is not assumed.
25. As a person who toggles vim mode with `/config` in a session, I want gx to probe the input mode only when the editor-mode settings say vim, and to leave no stray character behind, so that later prompts still work and a normal-mode send costs no extra keys.
26. As a person, I want gx to detect the input mode by what a test key press changes, and not from a missing marker, so that a transient footer or a stale frame cannot trigger a wrong key.
27. As a person, I want gx, when the probe's `i` appeared in the box, to remove it with one Backspace unless person input was seen since the snapshot, so that nothing is left in the input and a person's own character is never deleted.
28. As a person, I want gx, when the probe result is unclear, to give up and leave nothing behind, so that it never types blindly.
29. As an agent, I want gx to clear a lone `i` that gx itself typed (recorded per session) before it waits on the input, so that one stray key does not fail the launch.
30. As a person typing a draft, I want gx to leave any other input content alone, including a one-letter draft `i` that gx did not type, so that my draft is not deleted.
31. As a person, I want gx to retry once after one more clear round when the input is not empty, and to log a capture of the pane, so that a transient leftover does not pause the ticket and a real one can be diagnosed.
32. As a person, I want the vim probe to log through the server logger with the caller, the decision and a screen capture when the result is unknown, so that the next occurrence can be traced from the server log.
33. As an agent, I want the launch prompt to count as delivered only when it started a turn, so that "agent idle, no turn" is never treated as success.
34. As an agent, I want the session transcript's first user record to be my launch command, so that delivery has a hard proof.
35. As a person, I want an `Unknown command:` line on the pane to fail the delivery check at once, so that a rejected skill is noticed in seconds, not after 38 minutes.
36. As a person on the herdr runner, I want the launch prompt also to need herdr's own proof (a `working` or `blocked` state seen in its window), so that both signals agree.
37. As a person, I want gx to resend the launch prompt in the same session for about 30 seconds, only when `Unknown command:` was seen or the transcript has no user record for the command, and to log `prompt-nudged` each time, so that a skill that loads late still gets its task and a command is never run twice.
38. As a person, I want gx to return a not-delivered error after the grace period, so that the existing fresh-session retry takes over.
39. As a person, I want gx to pause only after the fresh-session retry also fails, so that a pause is the last resort.
40. As a person looking at that pause, I want the card to name the cause (for a missing skill: the skill name and how to install or refresh it) and offer Retry on the web, so that I can end it without a CLI command.
41. As a person, I want gx to wait for a positive ready signal (an idle input box plus the session transcript) before sending the launch prompt, for at most about 30 seconds, and not a fixed delay, so that the prompt is not typed into a Claude that is still starting and a missing signal does not block the launch.
42. As a person, I want the resend loop to stay as a backstop to the ready signal, so that a wrong ready guess still recovers.
43. As a person, I want the stall nudge, on a session with no user record, to resend the original skill prompt, so that the agent gets its task and not "continue" with no context.
44. As a person starting a server-launched session (the first prompt of an interactive or design session), I want the same post-send check, so that a rejected first prompt there is not silent.
45. As a person, I want the PTY runner not to report success when a person typed over gx's prompt, and the herdr runner not to report success when a turn passed unseen, without a delivery proof, so that "unconfirmed" is never reported as "delivered".
46. As a maintainer, I want real captures of the NORMAL, mid-turn and post-interrupt Claude screens kept as test fixtures, so that the probe is tested against what Claude draws.
47. As a person who runs Codex, I want no change to Codex launch input or prompt delivery, so that a Claude-only fix does not touch a harness that has no vim step. The kept worktree (stories 12 to 19) applies to Codex runs with `KeepAskedAgent == false`, because it does not depend on the harness.

Sandbox tests

48. As an agent working in the ralph-loop sandbox, I want a failed holder start to put the holder's own stderr in the launch error, so that I see "operation not permitted" or "invalid argument" and not "holder.sock never answered".
49. As a maintainer, I want the holder's stderr kept in a file in the agent directory, so that the cause survives after the launch error scrolls away.
50. As an agent, I want the lifecycle e2e and the fixture roots to use short paths, so that the holder socket stays under the unix-socket path limit.
51. As an agent, I want no skip for the tests that start `fakeclaude` through the PTY holder, and a failure there to name the blocked operation, so that I can tell what to allow.
52. As a maintainer, I want one ticket for the sandbox allowance (unix-socket bind in the agent directory, a pty), filed as `draft` and naming verification 51 as its trigger, so that the fix follows the evidence. A person promotes it to `open` once `holder.err` names the blocked operation, or cancels it as not needed if 51 passes. The epic can close either way.

## Implementation Decisions

Go build cache

- One wait gate serves every cache cleaner. It is a file lock under the gx state dir. A Go build holds it shared: `gx check`, every `gx brief go-*` tool (build, vet, test) and ticket checks. A cleaner takes it exclusive. There is no server round trip, so it works when the server is down or unreachable from the sandbox. A process scan for `go` processes adds to the lock: the cleaner also waits while the scan finds one. If the lock file cannot be opened, the build runs unregistered and logs that.
- The wait is capped at about 10 minutes. After the cap the clean goes ahead and gx logs it. The cap is shortened in tests by a test-only hook (a `Deps` or clock field), not by a user config key.
- The disk cleaner (automatic clean and "Clean now") runs its configured cleanup command (`disk-space.cleanup-command`, default `go clean -cache`, `config/disk_space.go`) only through the gate. The gate wraps whatever that command is. The disk-space spec rule "Running builds are not stopped during a clean" is rewritten to describe the gate.
- New command `gx brief go-clean-cache` waits on the same gate, then runs `go clean -cache`. Agent docs say: never run `go clean -cache` directly.
- `gx check` and every `gx brief go-*` tool (`cmd/brief.go`: build, vet, test) match the error "could not import ... (open ...go-build...: no such file or directory)", retry once, and on a second failure print a message that names a lost build cache. The implementer picks the exact wording and the match.
- Not done: a private per-run build cache. It makes every check a cold build, and the gate removes the cause.
- Whether disk pressure was present in the failing windows is unknown. Nothing depends on it.

Kept worktree

- Today `KeepAskedAgent` (`ralphloop/launch.go`, set by `keepsAskedAgent` in `server/runner.go`) is true for Claude on a native runner (PTY, headless) and for Codex on the PTY runner. Those runs already keep the worktree, pane and session after an adopted ask and return before `finishCleanup`. Only runs with `KeepAskedAgent == false` (herdr, an empty runner, Codex off PTY) lose the worktree: through `finishCleanup`, and through `cleanupCommitless` (`ralphloop/commitless.go`, about line 90) for a commitless ticket that asks. This change applies only to those two paths, and to every harness on them. Both keep the worktree on an adopted ask.
- On an adopted ask with `KeepAskedAgent == false`, gx stops removing the worktree and drops only the agent session and its tab. The iteration branch stays. The keep-agent runs keep their session as today.
- The relaunch reuses the kept worktree as is. The handoff prompt does not say uncommitted work is lost.
- The pause holds no permit.
- What removes a kept iteration worktree: a done merge (`finishCleanup` through the merge queue), start over (`repair/startover.go` `clearIteration`), cancelling a ticket, and merging its epic. No timer does. A worktree removed by hand is recreated from the iteration branch, as today.
- Cancel path, as it exists in code: `gx tickets close` (`cmd/tickets_close.go`) only takes draft or open tickets in a `follow-ups` epic and refuses an active ticket, so it never meets a kept worktree. Giving up on a paused ticket is a cancel (`gx server cancel` and the TUI tickets menu, through `ticketCancel` and `markCancelled` in `server/ticketwrites.go`), which removes no worktree today. The implementer adds removal of the ticket's leftover iteration worktree in `markCancelled`, through one helper that finds the worktree from the ticket's iteration path and removes it when present. This also covers cancelled fork descendants.
- Epic merge path: `mergeEpic` (`server/epicmerge_root.go`) merges the epic and removes no iteration worktree. After a successful epic merge, the implementer removes every leftover iteration worktree of that epic's tickets through the same helper. The branch is left to the existing branch cleanup.
- No WIP commit. No new pause kind or way into a pause. No schema change.

Launch input

- Scope: the vim fix covers every Claude gx launches (headless runner, PTY runner, herdr runner, and server-launched sessions). Codex launch input is unchanged: it has a no-op input-ready step and no vim mode. The kept-worktree change is separate and applies to Codex too, on runs with `KeepAskedAgent == false`.
- Headless runner (traced): `Headless.Prompt` sends a slash command as a stream-json user message and calls `awaitSlash` (`nativerunner/headless.go`). `awaitSlash` never resends. It counts the command as delivered when the uuid started, the turn counter moved, a dialog blocked claude, or claude is still idle after `WaitN`. That last case counts "idle, no turn" as delivered, so a rejected skill passes today. Change: a launch prompt passes a flag (a prompt option) that makes "idle after N with no started uuid and no new turn" return `ErrNotDelivered`, which starts the fresh-session retry. A launch prompt also fails when the output events carry `Unknown command:`. Commands that really start no turn (compaction) keep the single-send, idle-counts rule. A headless test with a rejected-skill fake asserts this.
- Editor mode: every gx-launched Claude gets `"editorMode":"normal"` merged into the existing `--settings` JSON on every launch path. `agentArgs` (`ralphloop/loop.go`) already passes `--settings claudeGuardSettings` and `nativerunner/pty.go` passes its own `--settings`; a second flag could replace the first and drop the `gx agent guard-bash` hook, so there is one flag with merged JSON. The implementer first verifies with a real Claude that this setting beats the person's own setting.
- Vim probe (backstop for a `/config` toggle mid-session, and for the case that the launch setting does not override): it runs only when the editor-mode settings say vim, not before every send. Snapshot the input, press `i`, re-read after a short settle. The probe records each `i` press per session. If the input grew by `i`, send Backspace, unless person input was seen since the snapshot (the PTY runner tracks that through `errPersonInput`); then skip the Backspace and report unknown. If the result is unclear, give up and leave nothing behind. A missing "INSERT" marker is never read as NORMAL. The probe reads the styled screen on herdr.
- The probe replaces the press-`i`-and-hope step for all five callers: the first-prompt delivery, the session typing step, the deny-reason path, the PTY nudge and the herdr nudge.
- The probe logs through the server logger with the caller, the decision and, when unknown, a screen capture. The pane capture on a failed clear is logged at the same level and place. The implementer picks the decision names and how the caller label reaches the harness.
- The empty-input wait (`awaitEmptyInput`) clears the input only when it is exactly `i` and a gx `i` press was recorded in that session since the last send. Any other content stays, because it may be a person's draft, including a draft that is a single `i`.
- One automatic retry on "input not empty" happens in two places: one more clear round inside the nudge, and one automatic re-prompt in `nudgeIfStalled` (`ralphloop/servermerge.go`), before the pause. That is the place the retro's pauses came from: the stall nudge has no retry today and ends in `iteration-error`. The launch path already gets one fresh-session retry on `ErrNotDelivered`, and herdr wraps `ErrStuckSubmission` as `ErrNotDelivered`. A not-delivered error means no new turn started, so the re-prompt cannot double-send.
- Delivery proof: a launch prompt must start a turn. The first user record in the session transcript must be the launch command. An `Unknown command:` line on the pane fails the check at once. On herdr, the launch prompt also needs herdr's proof: `working` or `blocked` seen within its window; otherwise the stalled-prompt error. No slash-command branch counts "idle, no turn" as delivered for a launch prompt. Commands that really start no turn (for example compaction) keep the single-send rule.
- Unknown skill: resend in the same session for about 30 seconds and log `prompt-nudged`. Resend only when `Unknown command:` was seen, or when the transcript has no user record for the command. Never resend once the transcript shows the command, because herdr's `promptSlash` never retypes on purpose, to avoid running a command twice. Then return the not-delivered error, which starts the existing fresh-session retry.
- ADR 0042: this adds no pause kind. After the resend grace and the fresh-session retry, a launch prompt that is rejected for good ends in an existing pause kind: `agent_prompt_stalled` (`startAndPrompt`, an undelivered prompt) or `iteration-error` (a `Start` failure, which includes a holder start failure). `launch-failed` is an event, not a pause. Both pause kinds already serve Retry (`tickets/schema/pause_verbs.go`), the web button that ends the pause. gx cannot recover alone there, because a missing skill or a blocked input does not change by itself. A skill that is truly missing is a cause outside gx, and Retry alone relaunches into the same `Unknown command:`. So the `agent_prompt_stalled` card names the missing skill and the fix (install or refresh the skill), then offers Retry.
- Readiness: wait for a positive ready signal, an idle input box plus the session transcript, and not a fixed delay. The wait is capped at about 30 seconds. After the cap gx sends anyway and logs that the ready signal was missing. The implementer picks the exact signal and checks that skills are loaded by then. The resend loop stays as a backstop. Herdr does not gate on readiness; it proves delivery after the send.
- The stall nudge on a session with no user record resends the original skill prompt and logs it as the stall nudge text.
- The first-prompt delivery for server-launched sessions gets the same post-send check.
- The PTY runner's "person typed over it" path and herdr's "turn passed unseen" path get the same delivery proof, and report "unconfirmed" when they have none.
- The implementer captures real NORMAL, mid-turn and post-interrupt screens as test fixtures before writing the probe.

Sandbox tests

- The PTY holder writes its stderr to a file in the agent directory (`holder.err`). The launch error includes that text, replacing "holder.sock never answered". A holder start failure surfaces as an `iteration-error` pause, ended by Retry.
- The lifecycle e2e and the fixture roots use short paths, so the holder socket path stays under about 100 bytes. The fixture root already uses a short path; the e2e root follows the temp-dir variable and may be long.
- No skip-with-reason: the tests are meant to run in the agent sandbox. A failure there names the blocked operation (story 51).
- Cause note: the sandbox cause is unconfirmed, and it is not Codex-specific (the failing runs were Claude). The session that hit it was not sandboxed, and no sandbox setting exists in the repo or in the person's Claude settings. By hand, a denied unix-socket bind, a denied pty and an over-long socket path each make the holder exit with a different message. The sandbox kind is unconfirmed.
- The sandbox allowance (unix-socket bind in the agent directory, a pty) is one ticket in `agent-lifecycle-impl`. It is filed as `draft` (a ticket can be blocked only by another ticket, and an open one with no blocker starts at once), with a note naming verification 51 as the trigger. A person promotes it to `open` once `holder.err` names the blocked operation, or cancels it as not needed if 51 passes after the short paths of story 50.

Schema

- No ticket or epic field, enum value or section name changes. No migration step is needed.

## Testing Decisions

Seams (picked by the spec author, as the highest existing seams; no interview):

1. The server and loop, driven through the fixture server (`gxfixture`) and the agent fakes (`agentfake`, `runnerfake`), observed by ticket state, run-log events, the worktree on disk and the CLI. This covers kept worktrees, the launch retry ladder, the stall nudge and the pause.
2. The harness typing interface with the fake Claude input box (`agentfake` program input), which draws the input box and footer. This covers the vim probe, the clear rule and the delivery proof against drawn screens.
3. The `gx check`, `gx brief go-*` and `gx brief go-clean-cache` commands, run as CLI against a fixture project with a fake `go`. This covers the retry and the cleaner gate. The disk cleaner is tested through the server seam (1).
4. The existing lifecycle e2e (`GX_E2E=1`) for the holder launch error and the short paths.

A good test checks external behavior only: the ticket state, the events logged, the keys that reached the input and what the input box shows at the end. It does not check which helper ran or in what order internals fire.

Needed tests and prior art:

- Marker absent while INSERT: assert no `i` stays in the input. Needs a new fake knob for "INSERT without marker". Prior art: the existing harness typing tests and the fake input box.
- NORMAL with a draft; probe unclear leaves the input as it was; a clear round retry; a lone gx-typed `i` is cleared and a real draft (including a one-letter `i` draft gx did not type) is not.
- A fake whose skill registry is empty for the first N seconds: assert `prompt-nudged`, then success. An always-unknown skill: assert the fresh-session retry, then the pause. Prior art: the existing nudge and launch tests in the loop and herdr runner.
- Transcript proof: a session with no user record gets the original skill prompt from the stall nudge. A transcript that shows the command is never resent.
- Headless runner with a rejected skill: assert it does not report success.
- Worktree kept (herdr runner fake, `KeepAskedAgent == false`): write an uncommitted file, ask, answer, relaunch, assert the file exists and no permit is held. Merge, start over, cancel and epic merge remove the worktree. A PTY run is not a valid scenario for this test. A commitless ticket that asks on the herdr fake also keeps its worktree.
- Cleaner gate: a registered run and a fake `go` process hold the clean; the cap (test hook) lets it go ahead and logs it; `gx brief go-clean-cache` waits on the same gate; a custom `disk-space.cleanup-command` is gated too. A lost-cache error is retried once; a second one prints the lost-cache message. Prior art: the disk-space tests for the cleaner.
- Holder failure: a holder that cannot bind (forced in the test) puts its error text in the launch error and in the file in the agent directory.
- Sandbox: no test skips for the sandbox. Passing in the sandbox is checked by the epic's verification.

## Verification

No stored mocks exist for this map, so there is no Mock states list. "Flow 3" is the pause-reasons-and-recovery flow in the web e2e flow catalog. Entry N verifies story N.

1. Go cache, cleaner holds: with the fixture server, register a running check and trigger "Clean now"; expected: the clean does not run until the check ends. New scenario to write: a fixture with a held run and a fake `go`.
2. "Clean now" and the automatic clean: both held (same scenario as 1, both triggers), also with a custom `disk-space.cleanup-command`.
3. A registered `gx check`, and a registered `gx brief go-build`, `go-vet` and `go-test`, each hold the clean (as in 1).
4. A `go` process the scan finds holds the clean. New scenario: a fake long-running `go` process.
5. With the server stopped, a registered build still holds a clean run through `gx brief go-clean-cache`. New test.
6. Cap: with the cap shortened by the test-only hook, the clean proceeds after the cap. New test.
7. The cap log line: `gx server logs` shows the clean-after-cap line (same test as 6).
8. `gx brief go-clean-cache` exits 0 after waiting; with a held run it waits (same fixture as 1).
9. Agent docs: `rg "go clean -cache"` over the agent docs finds only the `gx brief go-clean-cache` rule. Check by search.
10. `gx check` and each `gx brief go-*` tool with a fake `go` that fails once with the lost-cache error: expected exit 0 after one retry. New test.
11. The same fake failing twice: expected non-zero exit and a message naming a lost build cache (same test as 10).
12. The disk-space spec no longer says builds are not stopped. Check by search.
13. Kept worktree: with the fixture server on a herdr-fake runner (`KeepAskedAgent == false`), run a ticket that leaves an uncommitted file and asks; answer in the web UI; expected: the file is present after the relaunch. Run flow 3 for the Paused card. New scenario: an agent fake that writes an uncommitted file then asks.
14. Same scenario as 13; assert the worktree has not been reset or cleaned.
15. Same scenario as 13; check the relaunch prompt (in the agent fake's received input) has no "nothing uncommitted survives" text.
16. `gx server queue` and slot counts while the ticket waits show the queue not blocked: a second ticket starts (scenario 13).
17. After a done merge and after start over, the worktree is gone; before them, it is present. Run `git worktree list` in the fixture (scenario 13). No timer removes it: advance the clock and assert it is present.
18. Cancelling a paused ticket with a kept worktree through `gx server cancel` removes the worktree. Run `git worktree list` before and after in the fixture. A commitless ticket that asks on the herdr fake keeps its worktree (new scenario).
19. Merging an epic whose ticket left an iteration worktree removes it. Run `git worktree list` before and after in the fixture. New scenario.
20. Remove the worktree by hand, then Retry; expected: a recreated worktree on the iteration branch (same scenario as 13).
21. The iteration branch exists while the worktree is kept (`git branch --list` in the fixture).
22. The same ask scenario on a PTY-runner fake: the worktree, pane and session stay as today (the existing keep-agent tests stay green).
23. Launch with the person's settings set to vim: expected: the launched Claude's args carry one `--settings` whose JSON has `"editorMode":"normal"` and still has the `guard-bash` hook, and the fake input box shows normal editing mode. Assert on the PTY and the `agentArgs` paths. New fake knob if the fake cannot read the flag. Manual step: the implementer's real-Claude check.
24. Real Claude check, recorded in the PR: a launch with the person's vim setting accepts a first prompt with no stray text. Manual.
25. Fixture: toggle the fake into vim mid-session; deliver a prompt; assert the input has no stray character before the prompt. With settings that say normal, assert no `i` was sent at all.
26. Probe tests with drawn screens: marker missing in INSERT; marker missing in NORMAL; both end with the right keys and no extra `i`.
27. Probe tests: an `i` that appeared gets one Backspace (same tests as 26). With person input seen after the snapshot, no Backspace is sent and the result is unknown.
28. Probe tests: an unreadable screen leaves the input unchanged and the call fails with a logged capture.
29. A launch with a lone `i` that gx typed (probe recorded it) succeeds and the prompt arrives clean. New fake knob.
30. A launch with a longer pre-typed draft, and one with a one-letter `i` draft that gx did not type, keeps the draft and does not clear it.
31. A fake that leaves one `i` after the first clear: expected: the retry in `nudgeIfStalled` succeeds with one `prompt-nudged`-style log and a logged pane capture; no pause. Check with `gx server logs`.
32. `gx server logs` after the probe tests shows the caller, the decision and, for unknown, a screen capture.
33. A fake Claude that answers idle with no turn: expected: not counted as delivered; the ladder runs.
34. A fake with a transcript whose first user record is the launch command: expected: delivered. Without it: not delivered.
35. A fake that shows `Unknown command:` on the pane: expected: the check fails within a few seconds, not after the grace period alone.
36. A herdr-fake run where `working` or `blocked` is never seen: expected: the stalled-prompt error. A run where it is seen: delivered.
37. A fake whose skill registry fills after 10 seconds: expected: `prompt-nudged` events, then a started turn (`gx tickets log <addr>`). A transcript that already shows the command gets no resend.
38. An always-unknown skill: expected: the not-delivered error after about 30 seconds in the first session.
39. The same always-unknown run, after the fresh-session retry: the ticket is paused (`agent_prompt_stalled`; a `Start` failure gives `iteration-error`). New scenario: an always-unknown skill.
40. Flow 3 on the paused ticket from 39: the card names the missing skill, the fix (install or refresh) and a Retry button; clicking Retry lifts the pause. Add the card text to flow 3 if it is not covered.
41. A fake that is not ready for a fixed delay but is later: the prompt is sent after the ready signal, not at a fixed time. A fake that never shows the ready signal: the prompt is sent after about 30 seconds, and the log says the ready signal was missing.
42. A fake that gives a wrong early ready signal: the resend loop recovers (same scenario as 37).
43. A session with no user record plus a stall: expected: a `stall-nudged` event whose text is the original skill prompt. New scenario.
44. A server-launched session (interactive or design) with a rejected first prompt: expected: a visible failure, not silence. New scenario.
45. PTY runner with a person typing over the prompt, herdr runner with a turn passed unseen: expected: an unconfirmed result, not success. Unit-level tests at seam 2.
46. Fixture files exist for NORMAL, mid-turn and post-interrupt screens, and the probe tests use them. Check by listing the test data.
47. A Codex fixture session launches and gets its prompt with no new keys sent (the existing Codex typing test stays green).
48. A holder that fails to start: the launch error contains the holder's stderr text, not "holder.sock never answered". Seam 4.
49. The same failure leaves a holder error file in the agent directory (check by listing the directory).
50. The lifecycle e2e and `gxfixture` roots: the holder socket path is under 100 bytes with a long temp-dir variable. New test with a long `TMPDIR`.
51. Run `GX_E2E=1 go test ./e2e -run TestServerLifecycle` and the `gxfixture` tests that launch an agent from a ralph-loop ticket agent on the same runner and harness as the failing runs: expected: they run, with no skip. Record the runner, the harness and the result in the allowance ticket. Manual until the cause is known; if they still fail, the launch error names the blocked operation (see 52).
52. If 51 fails in the sandbox: the error names the blocked operation, and a person promotes the draft sandbox-allowance ticket in `agent-lifecycle-impl` to open. Manual.

## Out of Scope

- A private per-run build cache.
- A skip-with-reason for sandboxed runs of the holder tests.
- Any change to Codex launch input, including proof that a Codex launch prompt was delivered. No Codex incident exists. The delivery proof is filed as gx:follow-ups/198.
- Reading Claude's `vim.mode` from the statusline as a signal.
- An atomic refresh of the skills directory, until a person confirms that a refresh caused the lost-prompt incident.
- A WIP commit, a timer, or a permit held while a ticket waits for an answer.
- Changing the kept-worktree behavior of runs that already keep their agent (PTY and headless Claude, PTY Codex).
- Why the skill was unknown in the lost-prompt incident. It is unproven (skills not loaded at first idle, or the skills directory being rewritten). The fix does not depend on it.

## Further Notes

- The lost-prompt incident was first read as "prompt never arrived". The research shows it arrived and was rejected. A bare retry on the loud error would not have helped; the delivery proof does.
- The stray `i` research could not confirm which caller sent the `i` in the live runs. The new logging is meant to answer that.
- Implementer's choices left open: the lost-cache message and match, the probe decision names and caller label, the positive ready signal, the settle time, and the new fake knobs.
- Tickets to create (for the gx-to-tickets breakdown): the sandbox allowance is one `draft` ticket in `agent-lifecycle-impl`, triggered by verification 51 (stories 51 and 52). gx:follow-ups/198 (outside this epic) covers Codex delivery proof.

## Self-decided design questions

- Question: where does the headless runner tell a launch prompt from a no-turn command? Choice: a prompt option set by the launch path; without it `awaitSlash` keeps its rule. Reason: compaction must keep the single-send rule.
- Question: does a cancelled ticket's iteration branch go with its worktree? Choice: worktree only. Reason: the branch holds commits, and the spec promises commits are never at risk.
- Question: what happens when the lock file cannot be opened? Choice: the build runs unregistered and logs it. Reason: a build must not fail because the gate is unavailable.
- Question: where does the give-up removal go? Choice: one helper called from `markCancelled`. Reason: cancel is the real give-up path for a paused ticket, and `gx tickets close` refuses active tickets.
