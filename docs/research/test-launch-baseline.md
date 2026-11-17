# Launch baseline: does a real claude's startup count as a turn?

Question: in the nudge and launch tests, `awaitNotBusy` takes a baseline Turn and the launch fails with `started no turn` when Turn does not pass it. `test-go-readiness-server-packages.md` (finding A) showed the fake passes only because the holder's first pid read counts as turn 1, and only when the baseline is taken before that read. Nobody had probed what a real claude does at launch. Tags: **[run]** = seen in a real run; **[code]** = read in source; **[hyp]** = hypothesis.

## Answer in short

- **Decision: a real claude's launch is not a turn and is not busy.** At launch the pid file goes absent status, then `idle`. `busy` appears only when a prompt or a slash command is submitted. **[run, 4 of 4]**
- **Product fix:** `PidWatcher.poll` (`harness/claudepid.go:241-247`) must not count the first move from "not seen / starting" to `idle` as a turn. It counts a turn only on a change after the first read, or when the first read is already `busy` or `shell` (a turn that really started). The OSC `working` count and the new-sessionId count stay as they are.
- **Busy at launch:** keep `awaitNotBusy` as is. Starting (no pid status yet) is not busy, as a real claude never shows `busy` before it gets input. Readiness for typing stays with `awaitEmptyInput` and the launch proof.
- **Confidence: medium-high** for the cause and the real-claude facts (4 real runs, plus the earlier injection proof). Medium for the fix, because 2 existing tests pin the old count and I did not apply or run it.
- Test-side fix from the earlier report (`OnTurn` so the fake runs a real turn) is still right for the fake, and is now also what a real claude does.

## Observed: real claude at launch [run]

Claude Code 2.1.325, `--model haiku --permission-mode auto`, tmux 120x40, a trusted scratch cwd under the session scratchpad, `--settings` with `SessionStart`, `UserPromptSubmit` and `Stop` hooks that append a millisecond timestamp. A shell loop polled `~/.claude/sessions/<pid>.json` and the pane every 50 ms. Times are ms after the tmux spawn. After the pid file turned idle I waited to 4 s, then typed a prompt (runs 1-3: `Reply with the single word ok.`; run 4: `/context`). Files: `scratchpad/run.sh`, `pid_<n>.txt`, `hooks_<n>.txt`, `pane_<n>.txt` (scratchpad dir of this session).

| Run | pid file first seen | pid `idle` | SessionStart hook | submit | pid `busy` | UserPromptSubmit hook | pid `idle` again | Stop hook |
|---|---|---|---|---|---|---|---|---|
| 1 | 190 ms, no `status` field | 445 | +172 | 4024 | 4613 | +4455 | 6049 | +6052 |
| 2 (cold start) | 2428, no `status` | 3709 | +2245 | 4015 | 5714 | +4915 | 7572 | +7409 |
| 3 | 254, already `idle` | 254 | +183 | 4004 | 4694 | +4516 | 6569 | +6417 |
| 4 (`/context`) | 267, already `idle` | 267 | +207 | 4042 | 4715 | none | 5179 | none |

Facts:

- The pid file first exists with no `status` (runs 1, 2) or with `idle` straight away (runs 3, 4). gx reads a missing status as `ClaudeStarting` (`harness/claudepid.go:21`). **No run showed `busy` before the submit.** The first screen is the trust-free welcome box with the placeholder input line `❯ Try "write a test for <filepath>"`, no turn running.
- `SessionStart` fires at spawn +170 to 210 ms (run 2: +2245 ms on a cold start), at about the time the pid file appears. It fires once per launch and says "session began", not "a turn began". `UserPromptSubmit` fires only for a model prompt, about 0.3 to 0.9 s after the submit. `/context` fires none (a local command) but still showed `busy` for about 460 ms, then `idle`.
- The turn after a submit is visible: `busy` was seen at submit +0.6 to +0.7 s in every run, and lasted 0.5 s (`/context`) to 1.9 s (a one-word reply). My poll was 50 ms, gx's is 100 ms.
- OSC 7501 was not exercised: tmux does not answer the probe, so claude did not emit reports. Only the pid file and hooks are covered.

## Observed in code [code]

- `PidWatcher` starts with `last = ClaudeStarting` and bumps `turn` whenever `s.Status != last.Status` (`harness/claudepid.go:243-246`). The `Starting -> idle` move is such a change, so a real claude's launch yields **turn 1 with no turn run**. The comment says this is intended, "a first read past starting still counts the change a poll missed".
- `ptyTurn.awaitNotBusy` (`nativerunner/pty_turn.go:59`) returns `resp.Turn` as soon as claude is not busy, including before the first pid read. `PromptLaunch` then needs `Turn > baseline` (`WaitTurn`, `nativerunner/holder/holderanswer.go:77`) or fails `started no turn` (`nativerunner/pty_launch.go:123`).
- So the outcome depends on when the baseline is read versus the first pid read: baseline 0 then first read gives turn 1, "started"; baseline 1 (taken after the first read) gives "no turn". A real claude hits the same race as the fake. The earlier report measured it only on the fake.
- Existing tests that pin the old count: `harness/claudepid_test.go:41` (fresh idle file expects `turn == 1`) and `TestPidWatcher_TurnBumps` (`:303`). They need updating with the fix.

## Why the baseline is wrong

The baseline answers "the Turn value before my prompt". The first idle read is claude becoming ready, not work done for the prompt. Counting it makes the startup look like the launch's own turn when the baseline comes first, and makes the same prompt "start no turn" when the baseline comes second. A value that changes on its own before any input cannot serve as a baseline. This is a product bug (a real launch can be reported delivered because of the startup count, or not delivered despite a real turn that is busy for under the poll), not only a test race.

## Decision

1. In `PidWatcher.poll`: do not bump `turn` for the first read past `starting` when its status is `idle`. Bump it for a first read of `busy` or `shell` (a turn demonstrably running or ended in the background), and for every later status change.
2. Keep `awaitNotBusy`: busy means pid `busy`, or OSC `working`, or a watched harness `Working` without background. Starting and `idle` are not busy. Do not add "starting counts as busy": a real claude is `idle` at +0.3 s and a launch that waits for `busy` to clear would wait forever on a claude that never ran a turn.
3. Update the two harness tests above; add one for "fresh `idle` file after launch does not move Turn".
4. Test side (unchanged from the earlier report): the fake in `nudgeHang` runs a real turn with `h.OnTurn(func(*servertest.Turn){})`, so the launch prompt produces `busy` and the test asserts a real event, not the startup count.

## Could not determine

- **[hyp]** A launch command whose busy window is shorter than gx's 100 ms poll (and that changes no sessionId) would no longer be seen as a turn once the startup count is gone. `/context` was 460 ms and prompts over 1.4 s. I did not find a command that short; `promptSlash` already treats most slash commands as "start no turn", so the risk is limited to launch prompts that must start one. The launch proof's transcript check (`CheckLaunchDelivered`) is the second signal.
- Whether the fix changes any other consumer of `PidState` turn (herdr and codex paths use their own counters, not read).
- The fix was not applied and the Go tests were not run. No source was edited; this file is the only change.
- Real behaviour under a gx holder (OSC 7501 on, `--session-id`, vim check) was not run; only a bare claude in tmux was.
