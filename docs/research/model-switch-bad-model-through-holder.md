# A bad model through gx's holder

Ticket: `gx:model-switch/08`. Probed on 2026-11-15 with Claude Code 2.1.319 and codex-cli 0.169.0 (the versions of `docs/research/resume-with-new-model.md`), on real runs with one-line prompts and cheap models. No Settings or defaults were changed and `/model` was never typed.

## Answer

Neither harness exits when resumed with an unknown model. Both start, show an input box and reach idle in under a second, exactly like a valid model. A bad model fails only on the first message.

- The spec's rule "alive means the agent reaches idle, an exit before idle is a failed switch" would call a bad model alive in both harnesses. The old model would be dropped from the record on a launch that cannot answer.
- The earlier note "a bad model fails fast" is true only for `claude -p` and `codex exec`. It does not hold for the interactive TUI that gx runs.
- The idle edge cannot tell a bad launch from a good one. Process exit never fires for a bad model. What differs is a pane warning (Claude only, at launch), the first reply, and a rollout or transcript error line that the first message writes.

## How it was run

A throwaway Go probe (kept out of the repo, in the session scratchpad) reproduces `restartConversation` against the real holder:

1. Start a holder with `gx agent-holder --session-id <conv> --harness <h> --gx-session probe --cols 200 --rows 50 --dir <agent dir> --trust -- <argv>`. This is the command `startSessionHolder` builds (`server/sessions/sessions.go`).
2. The argv comes from `harness.Lookup(kind).Resume(conv, opts)` with the same extra args as `launchArgs`:
   - Claude: `claude --resume <id> --permission-mode auto --model <m> --settings {editorMode normal, AskUserQuestion hook}`.
   - Codex: `codex resume <id> --cd <cwd>` plus `harness.CodexArgs{Model, WritableRoots, QuestionTool: true}` (a plain Codex chat), with `harness.CodexSessionEnv` in the env.
   - No effort flag was passed and `agents.claude-args` was empty. The gx server itself was not started, so no session meta or `watchSession` ran.
3. Phase one launches the old model (`claude-haiku-5-5` or `gpt-5.6-luna`), waits for idle, calls `HolderClient.Stop()` and waits for the `exit-code` file, like `stopForRestart` and `clearOldHolder`.
4. Phase two launches the new model in the same agent dir and reads the holder's own watch stream (`HolderClient.Watch`), the same stream `watchSession` reads. Each message is timestamped from the holder start. The probe then dumps the live pane (`HolderClient.Screen`), and in some runs types a one-line first message and watches again.

The conversation had one earlier turn ("Remember the secret word PINEAPPLE. Reply only OK."), made with `claude -p` / `codex exec`. The Codex conversation ran in this worktree because Codex shows an untrusted-folder dialog in a scratch dir (see Unconfirmed).

Status values in the logs: `state=0` Starting, `1` Working, `2` Idle, `3` Blocked (`harness.State`).

## Claude (`--model nope-9`)

Observed output, relaunch phase, times from the holder start:

| Run | idle edge | ready signal | process exit |
| --- | --- | --- | --- |
| nope-9, no message (3 runs) | 0.73 s, 0.44 s, 0.42 s | `pid` or `screen` | none in 30 s |
| haiku (valid) | 0.32 s | `pid` | none |

1. The process does not exit. The holder stays up and the watch stream reaches `state=2` (Idle) in under a second, the same as the valid model.
2. The pane reaches idle. The first message is what fails.
3. Pane after relaunch with `nope-9` (idle, before any message):

   ```
   Claude Code v2.1.319
   nope-9 · Claude Max
   ⚠ Model "nope-9" isn't recognized · context assumed to be 200k tokens · update Claude Code, or see https://code.claude.com/docs/en/model-config
   ❯ Remember the secret word PINEAPPLE. Reply only OK.
   ⏺ OK
   ❯
     nope-9 · 22.7k · [▰▱▱▱▱▱▱▱▱▱▱▱] 11% · cwd-claude on main
   ```

   The valid launch shows `Haiku 5.5 · Claude Max` and no warning line. After typing "What was the secret word? One word." the pane answers in about 0.6 s:

   ```
   ⏺ There's an issue with the selected model (nope-9). It may not exist or you may not have access to it. Run /model to pick a different model.
   ```

   The watch stream goes Idle, Working (0.1 s later), Idle (about 0.6 s after typing). It never reports Blocked or an error. The holder stays alive.
4. Signals gx can read:
   - Process exit and holder state: none. Idle: no difference from a valid launch.
   - Pane: the line `Model "nope-9" isn't recognized` is on screen from the first idle. It is a screen scrape of a warning string, not a stable interface.
   - Transcript, only after a first message: a line `{"type":"assistant", "isApiErrorMessage":true, "error":"model_not_found", "message":{"model":"<synthetic>", ...}}` with the text above. The valid run wrote `message.model = claude-haiku-5-5` and no `error`. The `<synthetic>` model id is what `ReportedModelAndContext` would meet on the newest assistant line (the repo has a test that treats `<synthetic>` usage lines as not real, `transcript/transcript.go` near line 263; not re-checked here).
5. The old conversation is untouched in content. Two bad relaunches (and the two old-model launches before them) added only `cost-state`, `mode` and `permission-mode` lines. The hash of all other lines was identical before and after (`6ef33ff4...`). The first message on `nope-9` did write conversation lines: a user line and the synthetic error assistant line. That error turn stays in the history. A later resume on `claude-haiku-5-5` answered `PINEAPPLE` and the pane still showed the error turn above it.

## Codex (`--model gpt-nope-9`)

| Run | first idle | ready signal | process exit |
| --- | --- | --- | --- |
| gpt-nope-9, no message | 0.49 s, 0.65 s | `osc7501` at 0.99 s and 1.11 s | none in 30 s |
| gpt-5.6-luna (valid) | 0.88 s | `osc7501` at 0.94 s | none |

1. The process does not exit. The stream goes Starting, Idle, Working (about 0.05 s later), Idle with ready `osc7501`. This Working blip during startup also appears for the valid model, so it is not a failure sign. For both, the stable idle comes at about 1 s.
2. The pane reaches idle. The first message fails.
3. Pane after relaunch (idle, no message): the footer reads `gpt-nope-9 low · 7.1K used ...  ⚠ 6 warnings · f2 to view`. The valid footer reads `GPT-5.6-Luna low` with the same warnings counter (6). After the first message the pane shows:

   ```
   ■ The 'gpt-nope-9' model is not supported when using Codex with a ChatGPT account.
   ⚠ 7 warnings
   ```

   The watch stream then moves from Working to `state=3 blocked=error msg="The 'gpt-nope-9' model is not supported when using Codex with a ChatGPT account."` about 1.2 s after typing, and stays Blocked. The holder stays alive.
4. Signals gx can read:
   - Process exit and holder state: none. Idle at launch: no difference from a valid launch.
   - Pane at launch: no warning line distinguishes it. The footer shows the model name as given (an unknown name shows raw, a known one pretty-printed: `gpt-nope-9` against `GPT-5.6-Luna`). That is a heuristic, not an error signal.
   - Rollout, after a first message: `task_complete` with `error: {"codex_error_info":"model_not_found","message":"The 'gpt-nope-9' model is not supported ..."}`. gx already reads this as the Blocked error state (`codexsession.LastTurnErrorIn`, `chat.StatusFailure` "Agent hit an error"). The failed turn also wrote a `turn_context` line with `model: gpt-nope-9`.
   - Rollout at launch, before any message: each resume writes `thread_settings_applied` with the launch's model. A bad launch writes one with `"model":"gpt-nope-9"`. It is written for the valid launch too, so its presence says only what was requested. It is not an acceptance signal.
5. Conversation content is untouched by the launch. The no-message run added only two `thread_settings_applied` lines (one from the luna launch, one from the bad launch): 16 lines became 18. The first message on the bad model added a `task_started` and an error `task_complete` and the `turn_context` above, with no assistant reply. A later resume on luna answered `PINEAPPLE` (the `turn_context` lines read `gpt-5.6-luna`, `gpt-nope-9`, `gpt-5.6-luna`). So `ReadTurnModel` reports `gpt-nope-9` between a failed first message and the next good one.

## Differences between the harnesses

- Claude warns at launch on its pane, but its failed first message ends in plain Idle with an error line in the transcript. There is no Blocked state.
- Codex shows nothing at launch, but its failed first message ends in a Blocked error state that gx already reads.
- Neither exits. Both keep the holder up and the conversation usable on a later resume.

## What the evidence supports for the alive rule

Observed rule, per harness (this is what the probe supports, not a design choice):

- Reaching idle after a launch proves only that the process started and drew its input box. It is true for a bad and a good model in both harnesses, within about 1 s.
- An exit before idle never happened for a bad model, so "process exits before idle" cannot detect it. The spec's failure branch stays useful for other failed starts (a crash, a missing rollout), not for a bad model.
- The reliable failure evidence arrives only with the first model call:
  - Claude: transcript line with `error: model_not_found` (and `isApiErrorMessage: true`) after the first message. The pane warning `isn't recognized` is visible earlier, at launch.
  - Codex: Blocked error status from the rollout's `task_complete` error `model_not_found` after the first message.
- A rule that confirms a switch before any message cannot know. Options the evidence allows, none tested through a full gx server:
  - (a) Alive stays "reaches idle" and a later `model_not_found` (Claude transcript or Codex Blocked error) rolls the record back. The first message is spent.
  - (b) Check the model against the discovery list (`docs/research/model-discovery.md`) before relaunching. The spec rejects name checks by gx, but the list comes from the harness itself. Not tested here.
  - (c) For Claude only, read the pane warning at idle. A scrape of one string. Not available for Codex.

## Not confirmed

- A full gx server run. The probe used gx's holder and its watch stream, but not `Manager.restartConversation`, `watchSession` or the meta record. The signals above are what the holder reports. How `watchSession` and the record would react was not run.
- Effort flags (`--effort`, `model_reasoning_effort`) and `agents.claude-args` were not passed.
- Other bad-model shapes: a known-but-unavailable model (one the account cannot use), an alias, an empty string. Only the unknown names `nope-9` and `gpt-nope-9` were tried.
- The Codex untrusted-folder dialog. The first Codex attempt, in a scratch dir, stopped at "Trust this folder?" (`--trust` did not answer it) and never reached idle. It was stopped by hand and re-run in this worktree. gx's real session dirs are trusted, so this does not apply to the switch. The `--trust` flag's own reach was not investigated.
- Whether Claude shows the pane warning when the model is a valid-looking but unavailable id.
- Run-to-run spread is small but thin: Claude 3 relaunches (0.42 to 0.73 s to idle), Codex 2 bad and 1 valid.
- The `<synthetic>` handling in `ReportedModelAndContext` (the footer reading `<synthetic>` after a failed Claude message) was inferred from the transcript, not run.
- The failed first message was a typed message into the pane. A first prompt that gx types for a design session was not tried.
- The old launch in phase one used the valid model and the old holder stopped cleanly. A stop during a turn was not tried.

## Sources

- `server/sessions/sessionhandoff.go` (`stopForRestart`, `restartConversation`), `server/sessions/sessionresume.go` (`clearOldHolder`), `server/sessions/sessions.go` (`startSessionHolder`, `launchArgs`), `harness/codex.go` (`CodexArgs`, `Resume`), `harness/claude.go` (`Resume`).
- `nativerunner/holder/holdersock.go` (`Watch`, `Screen`, `Stop`, `WatchMessage`), `codexsession/turnerror.go`, `server/sessions/chat/failure.go`.
- Transcript and rollout files read directly: `~/.claude/projects/*cwd-claude/<id>.jsonl`, `~/.codex/sessions/2026/11/15/rollout-*-01a1dd8b-....jsonl`.
- `docs/research/resume-with-new-model.md`, `docs/specs/model-switch.md`.
