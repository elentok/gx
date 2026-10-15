# Claude status signals without hooks (gx:agents-tab/02)

Question: besides `~/.claude/sessions/<pid>.json`, which signals can gx read, without Claude Code
hooks, to know the status of an interactive `claude` running on a PTY gx owns? How fast and how
reliable is each, and which states does each mark?

Observed on Claude Code **2.1.300** (macOS), 2026-10-15.

## Sources

- **[E] Experiment.** A Python `pty.fork` harness ran `claude --model haiku --permission-mode
  default` in scratch dirs. It sent prompts, logged every PTY byte, and polled the pid file every
  50 ms. The harness stripped `CLAUDE*`/`HERDR*` env vars (same reason as `nativerunner.LaunchEnv`).
  Four turns: a plain reply, a `Write` permission prompt (denied with Esc), an `AskUserQuestion`
  (Esc), and a `run_in_background` `sleep 20`. Three runs: `TERM_PROGRAM=ghostty` (auto channel);
  `preferredNotifChannel=iterm2_with_bell`; and one where the harness answered the OSC 7501 probe.
- **[B] Binary.** Strings from `~/.local/share/claude/versions/2.1.300`: the bundled JS, read with
  `rg -a`.
- **[D] Docs.** <https://code.claude.com/docs/en/terminal-config>: "Get a terminal bell or
  notification" and "See session status in your terminal". Hook notification types from
  <https://code.claude.com/docs/en/hooks>.
- **[C] Changelog** (embedded in the binary), 2.1.295: "Added Program Status Protocol (OSC 7501)
  support: terminals that implement it can show whether Claude Code is working, waiting on you, or
  done".
- **[H] herdr.** <https://herdr.dev/docs/agents/>, <https://herdr.dev/docs/integrations/>, and
  `distribution/agent-detection/claude.toml` in `herdrdev/herdr` (manifest version 2026.10.11.1).
- **[G] gx code.** `nativerunner/pidstatus.go`, `nativerunner/pty.go` (`Status`, `Answer`),
  `agentrunner/claudeui/*`, `transcript/background_task.go`.

## 1. OSC 7501 Program Status Protocol: the best new signal

Claude probes the terminal at startup with `ESC ] 7501 ; ? BEL` [E][B]. If the terminal replies
with an OSC 7501 whose data starts with `?`, Claude reports status on every change [B]. gx owns the
PTY and its `x/vt` emulator, so gx can be "the terminal": it answers the probe and then parses the
reports. In the experiment, the reply `ESC ] 7501 ; ? BEL` was enough.

Format, from `v_r` in [B]:
`ESC ] 7501 ; state=<s>:app=claude-code[:id=<task>][:kind=<k>][:progress=<0-100>][:title=<b64>][:msg=<b64>] BEL`.
`state=clear` (optionally with `:id=`) is sent on exit.

Observed sequence [E]:

| Moment | OSC 7501 | pid file |
| :- | :- | :- |
| startup | `state=idle` | `idle` |
| prompt sent | `state=working` | `busy` |
| reply finished | `state=done` | `idle` |
| Write permission | `state=blocked:kind=permission:msg=` "approve Write: /private/tmp/…/a.txt" | `waiting` / `permission prompt` |
| Esc on a prompt | `state=idle` (not `done`) | `idle` |
| AskUserQuestion | `state=blocked:kind=question:msg=` "answer: Red or blue? (Red · Blue)" | `waiting` / `input needed` |
| background `sleep 20` started | `state=working:msg=` "Sleeping for 20 seconds in background", then `state=done` | `shell` |
| task finished, Claude woke | `state=working` → `state=done` | `busy` → `idle` |
| exit | `state=clear` (×3) | file deleted |

- **Latency:** pushed in-band, at the same moment as the screen redraw. No polling.
- **Marks:** working / done (turn finished) / idle (cancelled or fresh) / blocked with `kind`
  (`permission`, `question`; [B] also shows plan-ready → `permission`) and a human `msg`
  (good for notifications). Per-task rows (`id=`) cover some background tasks (remote agents,
  paused tasks) [B].
- **Reliability caveats:** needs Claude ≥ 2.1.295 [D]. Off when `CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1`,
  for background (`bg`) sessions, and under tmux [D][B]. It is a new, lightly documented protocol,
  so its fields may still change. Only one probe is sent per process start. If gx misses it (for
  example after a holder re-attach to a running claude), there are no reports until claude
  restarts. The holder must parse it from the raw byte stream, before or alongside `x/vt`.
- **Not seen:** a rate-limit or error state. A 429 is not a blocked state here.

## 2. Pid status file `~/.claude/sessions/<pid>.json` (already used)

Fields seen [E][B]: `pid, sessionId, cwd, startedAt, procStart, version, kind (interactive|bg|daemon|…),
entrypoint, status, waitingFor, updatedAt, statusUpdatedAt, name, messagingSocketPath, peerFeatures, …`.

- `status` ∈ `busy | shell | idle | waiting` [B]. **`shell` is new to gx:** it means idle while a
  background shell/task runs (`$o = so==="idle" && bgRunning ? "shell" : so` [B]). It appeared
  during `sleep 20` [E]. `nativerunner.Pty.Status` has no case for it, so it falls through to
  Working. That happens to be the right answer, but it is not named. `PidStatus` docs list only three values.
- `waitingFor` values in [B]: `permission prompt` (default for any permission dialog, incl. plan
  approval), `input needed` (AskUserQuestion, MCP elicitation, teammate setup), `dialog open`
  (model fallback, usage-credits choice, settings dialogs, …), `sandbox request`, `goal proposal`.
- **Latency:** written at the transition. gx polls every 100 ms. Observed within one 50 ms poll of
  the screen change [E].
- **Marks:** busy vs idle vs waiting (+ why). It does **not** tell "turn finished" from "user
  cancelled": both are `idle`. Count `busy→idle` edges to get turn ends. That is what
  `pidWatcher.turn` does.
- **Reliability:** high. Written for every interactive session, needs no terminal support, and
  survives a gx re-attach. Missing while a nested-session env var is inherited (gx already strips
  them). Undocumented format.

## 3. Transcript JSONL `~/.claude/projects/<cwd-slug>/<sessionId>.jsonl`

- **Turn finished:** `{"type":"system","subtype":"turn_duration"}`. It follows the last assistant
  entry (`stop_reason:"end_turn"`), and also follows a cancelled turn
  (`[Request interrupted by user for tool use]`) [E]. `stop_hook_summary` precedes it when hooks ran.
- **Permission / question:** not directly. The `tool_use` entry (`Write`, `AskUserQuestion`) is
  written before the dialog shows. An open `tool_use` with no `tool_result` means "pending",
  either running or awaiting approval [E]. The tool name tells you AskUserQuestion.
- **Rate limit / error:** yes, and better than the screen. An assistant entry with
  `isApiErrorMessage:true`, `error:"rate_limit"`, `apiErrorStatus:429`, `quotaLimits` and text
  "You've hit your session limit · resets 11pm (Asia/Jerusalem)". Other values seen locally:
  `server_error`, `authentication_failed`. Retries show as `system` / `subtype:"api_error"` with
  `retryInMs`, `retryAttempt`, `maxRetries`.
- **Background tasks:** gx already reads start markers vs `<task-notification>` here
  (`transcript.ReadBackgroundTasks`).
- **Latency:** appended at each message boundary, so it trails the screen by up to one write. Needs
  a tail/poll of a growing file. **Reliability:** good, but the format is undocumented and has
  many entry types (`api-request-blob`, `ai-title`, `mode`, …). Transcript saving is off while
  `CLAUDE_CODE_CHILD_SESSION` is inherited [E].

## 4. Notification escape sequences (OSC 9 / 777 / 99, BEL)

The channel comes from `preferredNotifChannel` (`auto | iterm2 | iterm2_with_bell | kitty | ghostty
| terminal_bell | notifications_disabled`) [B][D]. `auto` picks by the detected terminal: XTVERSION
reply, else `TERM_PROGRAM`. iTerm.app → OSC 9, kitty → OSC 99, ghostty → OSC 777, Apple Terminal →
BEL, anything else → **nothing** [B].

Observed [E]:
- `auto` + `TERM_PROGRAM=ghostty`: `ESC]777;notify;Claude Code;Claude needs your permission BEL`.
- `iterm2_with_bell` via `--settings`: `ESC]9;Claude needs your permission BEL` plus a bare `BEL`.
- Both fired on the permission prompt **and** on AskUserQuestion, with the **same** text. No
  notification at the end of a turn in a 70 s idle wait. Hook types list `idle_prompt`,
  `agent_completed`, `elicitation_dialog`, … [D-hooks]. The docs say they fire "when Claude
  finishes a task or pauses for a permission prompt, and you appear to be away" [D]. "Away"
  likely uses focus reporting (Claude enables `?1004h`), which a gx PTY never answers.

Verdict: a coarse "needs attention" ping, ≈ the same instant as `waiting`. It depends on what the
inherited `TERM_PROGRAM` is, unless gx sets `--settings '{"preferredNotifChannel":…}'`. It cannot
tell permission from question. Nothing it gives is missing from OSC 7501 or the pid file.

## 5. OSC 0 window title

`ESC]0;<glyph> <title> BEL` [E]. While busy, the glyph is a spinner frame (`◐◓◑◒`; braille before
2.1.228 [H]). While not busy, it is `✳`. The title text is `Claude Code` at first, then the
AI-generated session title (`ai-title` in the transcript). It is cleared to `ESC]0; BEL` on exit.

- Marks busy vs not-busy only: `waiting` and `idle` both show `✳` [E]. Instant, in-band. Off when
  `CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1`. herdr uses it as its top-priority working/idle rule [H].
  Useful as a **display name** for the tab.

## 6. Other in-band sequences seen in the binary (not observed)

- OSC 9;4 progress (`terminalProgressBarEnabled`). herdr treats `4;0` as idle [H]. Not emitted to
  our plain xterm PTY.
- OSC 133 semantic prompt `A` (turn start) / `C`+`D` (turn done), behind a capability check [B].
  Not emitted in [E].
- OSC 21337 tab status (`indicator`, `status`, `status-color`) [B]. Not emitted in [E].

## 7. Screen patterns (`agentrunner/claudeui`)

gx already matches the trust dialog, startup, plan approval, permission screens and rate-limit text
(`ratelimit.go`: "You've hit your session limit · resets …"). Latency is one render. It is fragile
across versions and widths, so keep it for what nothing else gives: pre-session dialogs (trust,
login) and as a fallback for rate limits.

## 8. What herdr does for Claude [H]

- No hook-driven state. `herdr integration install claude` adds only a `SessionStart` hook that
  reports session identity for restore. "Claude Code state comes from Herdr's screen manifest
  detection."
- It finds the agent's process in the pane, then runs prioritized regex rules (`claude.toml`)
  over regions: `osc_title` (spinner → working, `✳` → idle, priority 1100), the bottom 12 lines
  ("esc to interrupt", spinner + "…"), "Waiting for N background agents", "N MCP tasks still
  running" (working), `Do you want to proceed?` / `Esc to cancel` + `Enter to select` / MCP
  elicitation footers (blocked), the `❯` prompt box (idle), and OSC 9;4 `4;0` (idle).
- If no rule matches, it falls back to idle. Blocked is conservative: only known UI shapes count.
  `done` is a rollup state ("stays visible until you view it"), not a detected one.
- herdr does not use OSC 7501, the pid file or the transcript.

## Summary

| Signal | Finished turn | Permission | Question | Error / rate limit | Background task | Latency | Reliability |
| :- | :- | :- | :- | :- | :- | :- | :- |
| OSC 7501 (gx answers probe) | `done` (≠ cancel `idle`) | `blocked kind=permission` + msg | `blocked kind=question` + msg | no | partial (`working` msg, per-task ids for some) | instant, pushed | high on ≥2.1.295; lost if probe missed; new protocol |
| Pid file | `busy→idle` edge (also on cancel) | `waiting`/`permission prompt` | `waiting`/`input needed` | no | `shell` status | 100 ms poll | high; already in use |
| Transcript JSONL | `system/turn_duration` | indirect (open `tool_use`) | indirect (`AskUserQuestion` tool_use) | **yes**: `error:"rate_limit"`, `api_error` retries | yes (gx already reads) | per message, file tail | good; undocumented |
| OSC 9/777/99, BEL | no (only "away" idle) | yes | yes, same text | no | no | instant | depends on channel/`TERM_PROGRAM`; coarse |
| OSC 0 title | spinner → `✳` | no (`✳`) | no | no | no | instant | high; busy/not-busy only; gives title |
| Screen (`claudeui`) | prompt box | dialog text | dialog text | "hit your … limit" | "Waiting for N background agents" | one render | fragile |

## Recommendation

1. **Primary state: the pid file** (as today) plus `shell` as a named state. It is reliable, works
   after re-attach, and gives `waitingFor`.
2. **Add OSC 7501**: gx's PTY answers the probe and parses the reports. This gives an instant
   push, a clean "turn done" vs "cancelled", `kind` (permission vs question) and a ready-made
   notification `msg`. Use it to sharpen the pid state, and fall back to the pid file when no
   report has arrived.
3. **Transcript** for error / rate limit (`isApiErrorMessage`, `error`, `api_error` retries) and
   background tasks (existing reader).
4. **OSC 0 title** only for the session's display name. **Screen patterns** only for pre-session
   dialogs and as a rate-limit fallback.
5. Skip OSC 9/777/BEL as an input. They add nothing over 1+2, and they vary with the terminal env.
