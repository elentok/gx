# fast-queue-startup/03: a ready signal Claude gives before the first prompt

Ticket: `gx:fast-queue-startup/03`. Claude Code 2.1.318, macOS, 2026-11-05. Code read on `origin/main` (0a3354cf4). Nothing in the product was changed.

## Answer

- Claude 2.1.318 gives four signals before the first prompt, all within 0.1 to 0.9s of the process start. The session transcript is not one of them. It is written with the first record, and a slash command that opens a dialog writes none.
- The transcript wait adds nothing to the delivery proof and the resend. It checks a file that cannot exist yet, so every launch waits the full 30s (10 of 10 warnings in `~/.local/state/gx/server.log` say `waited: 30000000000`).
- By the time `awaitReady` runs, `Pty.Start` has already waited for the holder's ready signal. The input box part of `awaitReady` is redundant too.
- Recommendation: delete `awaitReady`'s transcript clause. Keep the holder's ready wait in `Start` as the one ready check. Look up the transcript by the direct path with `os.Stat`.

## How the check came in

- Commit `6b7dbd911` (agent-lifecycle-impl/22, 2026-11-01) added `awaitReady` (`nativerunner/pty_launch.go:98-130`). Its ticket says: "the ready signal is the input box plus the session transcript existing. Skills loading is not checked: there is no observable signal for it" (`agent-lifecycle-impl/issues/22-pty-ready-signal.md`, "Spec deviations").
- The transcript clause came from the test fake, not from a real Claude. `testutil/agentfake/tui.go` in that commit says "Claude has its transcript file once it is ready" and touches the file after the ready delay. The real Claude does not (below).
- The spec asked for "an idle input box plus the session transcript" (`docs/specs/agent-lifecycle.md`, story 41, line 71) and left the exact signal to the implementer (line 236).

## Signals observed

I started a real `claude --session-id <uuid> --permission-mode auto` in a throwaway cwd under a pty, with gx's env (`CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_CHILD_SESSION` removed, as `harness.LaunchEnv` does) and a `--settings` JSON with a statusline command, a SessionStart hook and `editorMode: normal`. The probe answered the OSC 7501 query like the holder does. It sent no prompt. Times are seconds from spawn, 3 to 5 runs each, trust already accepted. Script: scratchpad `rs/probe.py` (not committed).

| Signal | Arrives | Notes |
|---|---|---|
| `~/.claude/sessions/<pid>.<hash>.key` | 0.11 to 0.30 | Written first. Not read by gx. |
| First screen output | 0.10 to 0.35 | Welcome banner and input box drawn in one frame. |
| Input box (`❯` between two rules, `claudeui.InputBox`) | 0.13 to 0.47 | The first frame has it. |
| `~/.claude/session-env/<id>` dir | 0.16 to 0.39 | Not read by gx. |
| SessionStart hook runs | 0.23 to 0.64 | Fires before the first prompt; the docs say the same ("before the first prompt", https://code.claude.com/docs/en/hooks). Runs once per process and needs a hook entry in `--settings`. |
| Statusline command first run | same moment as the hook | Already in gx's launch settings (`nativerunner/pty.go:194-210`). |
| `~/.claude/sessions/<pid>.json` pid file | 0.21 to 0.64 | First read sometimes has no `status`; `status: idle` follows within 0.05s. `harness.ReadyPidFile`. |
| OSC 7501 `state=idle:app=claude-code` | 0.25 to 0.90 | After the holder answers the `ESC ] 7501 ; ?` query. `harness.ReadyOSC7501`. Always the last of the three. |
| Transcript `<id>.jsonl` | never | No file in any run, 30s in the longest. The `projects/<slug>` dir is not created either. |

- Trust dialog: the first run in an untrusted cwd drew the dialog at 0.12s. About 0.15s after the key (Down, Enter), the session-env dir, hook and statusline appeared. The pid file is not written until the dialog is answered (`ptyLauncher.launch` comment, `pty_launch.go:236-239`).
- The pid file and the OSC reports are missing if the child inherits `CLAUDECODE`. In my first runs (inherited env) claude drew the box and ran the hook but wrote no pid json for 30s. gx strips these variables, so the holder is not affected, but any new probe must do the same.
- Side effect on the machine: accepting the dialog changed `hasTrustDialogAccepted` for `/Users/david/dev/gx` in `~/.claude.json` from absent to true (the other entries did not change). All gx worktrees live under it. I removed the throwaway dir and its session files.

## What the holder already waits for

- `Pty.Start` calls `ptyLauncher.launch` (`nativerunner/pty.go:152`). `launch` loops until the holder's watch reports `Ready != ""`, or the pid file says idle on a holder without the watch op (`pty_launch.go:225-275`).
- `PidWatcher.Ready` is the earliest of the pid file's idle, an OSC 7501 idle, and the input box on screen (`harness/claudepid.go:516-537`). So the holder's ready fires at the first frame (0.13 to 0.47s), not after the later pid and OSC signals.
- Only after `Start` returns does `PromptLaunch` run: `awaitNotBusy`, then `awaitReady`, then `awaitEmptyInput`, then `Submit` (`pty_launch.go:36-60`). `awaitReady` is therefore a second wait on a state `Start` already proved, plus the impossible transcript.
- `awaitReady` runs only for a Claude slash-command launch prompt on the PTY runner. The herdr runner does not use it, and Codex keeps `PromptNudged`.

## Does the first frame accept keys?

- Typing `/gx-chang` at the first input box frame, before the pid file and the OSC probe, showed the `gx-changelog` suggestion within 0.05 to 0.1s in 5 of 5 runs. The same text typed at the OSC idle report worked in 3 of 3 runs. So keys are read from the first frame, and user skills are in the command registry at that point.
- That is one machine, a warm cache, one process at a time. It does not prove the registry is full under load (see the open question).
- With `editorMode: normal` in `--settings`, the footer showed no vim `INSERT` marker and the typed text went into the box as typed.

## What each lost-prompt case needs from the ready check

| Case (source) | Needs from the ready check | Where the protection lives today |
|---|---|---|
| Stray `i` in vim mode (`agent-lifecycle-03-stray-i.md`, spec stories 24 to 32) | Nothing. gx no longer types `i`: every claude gets `editorMode: normal` (`harness/claude.go:31-38`, `MergeClaudeSettings`), the probe runs only after a `/config` toggle, and a lone `i` gx typed is cleared (`awaitEmptyInput`, `pty_personinput.go:127`). | Launch settings and the clear rule. |
| `Unknown command: /gx-implement`, prompt rejected (`agent-lifecycle-04-lost-prompt.md`) | Nothing it can see. The cause is unproven: skills not registered at first idle, or the skill dir being rewritten (line "Why was the skill unknown?"). The transcript does not help: the rejection wrote system records and no user record. | The delivery proof: first user record must be the command, pane `Unknown command:` fails at once, resend every 3s for 30s, then a fresh session (`launchproof.go:117-175`). |
| Prompt typed while Claude is still starting (spec story 41) | The input box, which the holder's ready already includes. | `Start`'s ready wait. |
| Person typing over the launch | Nothing. | `errPersonInput` in `PromptLaunch`. |

The transcript clause protects none of these. The delivery proof catches the registry race with resends, and on a persistent miss the fresh-session retry. In 2.1.318 I saw no registry race at first idle (5 of 5 runs for user skills). I could not reproduce the original incident.

Not found: any run-log or server-log record of a resend (`prompt-nudged`) since the delivery proof shipped. The run logs under `~/.local/share/gx/tickets/gx/*/run-log.jsonl` have no `prompt-nudged` events, and `server.log` covers only the last day. Whether the resend fires in real launches stays unmeasured.

## Launch-path transcript lookup cost

- `transcript.FindByID` is `filepath.Glob(<home>/.claude/projects/*/<id>.jsonl)` (`transcript/transcript.go:59-72`). `~/.claude/projects` has 3,415 dirs now.
- Measured on this machine (`filepath.Glob`, 100 calls, warm cache): 50ms per call. `os.Stat` of a direct path: 0.5 to 0.7 microseconds. About 80,000 times cheaper.
- Callers on the launch path:
  - `awaitReady`: every `pollInterval` (100ms) for up to 30s, so about 300 globs and 15s of CPU per launch (`pty_launch.go:107-112`). With the transcript clause gone this poll is gone.
  - `FirstUserRecord`: one glob per `Poll` (500ms) during the delivery proof (`launchproof.go:63`, `launchproof.go` `DefaultLaunchProofCaps`). `FirstUserRecord` is called with `home` and the session id only, so it has no cwd today.
- `gx:follow-ups/215` records the same glob as 47 to 68% of server CPU in launch waves. `gx:follow-ups-archived/62` (globs in `FindByIDIn`) was cancelled because the callers it named are cached in `server/transcriptcache.go` (3e9f2e2f). That cache covers `server.claudeTranscript` and `transcriptPaths.resolve`. It does not cover the two launch-path callers above, which poll for a file that does not exist yet and so never fill a cache.

## How Claude slugs the cwd

- The rule in the 2.1.318 binary (read with `strings` on `~/.local/share/claude/versions/2.1.318`): replace every character outside `[a-zA-Z0-9]` with `-`. If the result is longer than 200 chars, keep the first 200, then `-`, then `Math.abs(h).toString(36)` where `h` is the 32-bit Java-style string hash (`h = h*31 + charCode | 0`) of the original path. Project dir = `<config dir>/projects/<slug>`.
- Checked against data: for 3,404 of the 3,404 project dirs that have a transcript with a `cwd` field, `re.sub("[^a-zA-Z0-9]", "-", cwd)` equals the dir name. 11 dirs had no transcript. The longest dir name is 125 chars, so the 200-char branch is untested on this data. Special characters seen: `.` (5 dirs), `_` (1 dir), `-`.
- The slug input is the process cwd as `getcwd` reports it, i.e. the symlink-resolved path. `/tmp/...` sessions are stored under `-private-tmp-...` (72 dirs) and none under `-tmp-`. So gx must slug `filepath.EvalSymlinks(cwd)`, not the path it passed to `cmd.Dir`.
- The binary also has an override env for the dir name (`CLAUDE_CODE_PROJECT_DIR_NAME`, honored only with a config-dir env set). gx sets neither.
- gx has no slug function. `claudeui.BackgroundTasks` and `transcript.Resolve` find by id and ignore the cwd. `transcript.FindByID(home, id)` also uses `<home>/.claude/projects` and ignores `CLAUDE_CONFIG_DIR`, while `harness.ClaudeProjectsDirIn(env)` honors it.

## Options for the ready check

1. Keep `awaitReady` as is. Cost: 30s per launch. Rejected: the transcript cannot appear.
2. Drop the transcript clause, keep the input box poll in `awaitReady`. The box is already true after `Start`, so this returns on the first iteration: a screen read, about 0 to 100ms. Cost: a redundant check that can still time out on a cleared screen.
3. Delete `awaitReady`. `Start`'s ready wait is the ready check. `awaitNotBusy` and `awaitEmptyInput` still run before `Submit`. Cost: none seen. If the person's `/clear` or a dialog redrew the screen, `awaitNotBusy` (status blocked returns `ErrNotReady`) and `awaitEmptyInput` still see it.
4. Add a SessionStart hook that writes a marker file and wait on it. Cost: a new per-launch settings entry and a file per agent dir, and the hook fires within 0.1s of the pid file, so it gains nothing. Rejected.
5. Wait for the OSC 7501 idle or the pid file idle before the prompt (instead of the box). Cost: up to 0.4s later than the box, and no extra protection seen.

Recommendation: option 3. Remove `awaitReady`, `DefaultReadyWait`, `Pty.ReadyWait` and the `ready signal missing` warning. A fixed-input fake test replaces `TestPty_LaunchPromptWaitsForTheReadySignal` and `...SendsAnywayWhenNeverReady`. The fake must stop touching the transcript at ready, since the real Claude does not. Expected saving: 30s per launch, and about 300 globs per launch.

## Options for the transcript lookup

1. Keep the glob, poll slower (follow-up 215's first idea: 1s). Cost: still 50ms per call, and a 1s detection lag on a 15s budget.
2. Stat the direct path `<projects dir>/<slug(EvalSymlinks(cwd))>/<id>.jsonl` each poll. Cost: needs the cwd in `LaunchProof.FirstUser` and `LaunchRecorded` (the `ptySession` has it from `launch`'s `cwd` argument; `Meta.Cwd` holds it). A wrong slug (for example the 200-char hash branch, an override env) makes the proof miss.
3. Option 2, and on a stat miss fall back to the glob at most once per second, and use the found path from then on. Cost: a small fallback branch. It keeps `transcript.Resolve`'s promise that the launcher "may have slugified a different cwd".
4. Watch the dir with fsnotify instead of polling. Cost: more moving parts; the poll at 100ms already costs microseconds after option 2.

Recommendation: option 3, with the poll at 100ms (the stat is free), so the delivery proof ends within about 0.1s of the record instead of up to 0.5s. Resolve the projects dir with `harness.ClaudeProjectsDirIn(env)` so `CLAUDE_CONFIG_DIR` is honored. Put the slug in the `transcript` package with a test that feeds it the rule above, including a 200+ char path and a `.`/`_` path. A cwd with a symlink in it needs `EvalSymlinks` first.

## Effect on the 15s target

- The ready wait stops being a cost: 30s becomes 0.1 to 0.5s (`Start` already waits for the first frame). The delivery proof adds the time from Enter to the first user record, which I did not measure (no non-working prompt writes one). The proof's poll loses up to 0.5s today and about 0.1s after the change.
- Nothing found here makes the 15s target unreachable.

## Open questions for the grilling ticket

- Does the skill registry fill fast enough under a launch wave (N agents starting together, a cold cache)? I saw 5 of 5 warm single launches fine. The resend loop stays as the backstop either way, so this is a measurement for the verify step, not a blocker.
- Should the 200+ char slug branch be implemented now? It needs the hash. gx worktree paths are about 60 chars, so the glob fallback in option 3 covers it without the hash.
