# gx:agent-launch-and-recovery/02 - Codex launch delivery proof

Question: how does a Codex session show that it received its first prompt, and what would gx need to
prove delivery, resend and raise `ErrNotDelivered` for Codex the way it does for Claude? Also answers
the questions merged in from gx:agent-session-guards/03.

Sources: real rollouts under `~/.codex/sessions` (Codex 0.166.0, 62 files, 2026-10-29 to 11-05), the
gx code at this branch, and the Codex recorder source
(https://raw.githubusercontent.com/openai/codex/main/codex-rs/rollout/src/recorder.rs, read 2026-11-07).
I did not launch a live Codex to time the `SessionStart` hook (see "Not verified").

## Summary

- A Codex rollout is a JSONL file whose first line exists only once the first turn starts. The first
  prompt shows as a `response_item` / `message` / `role:"user"` line, followed for a `$skill` prompt by
  a second user line `<skill><name>..` that carries the skill body. Writes are flushed per line, so the
  proof is prompt, not buffered.
- gx cannot key the lookup by session id before it sends: the repo's own comment says Codex reports its
  id only once the first prompt begins (ralphloop/supervise/startandprompt.go:150). The reliable key is
  the rollout's `session_meta.cwd` plus "created after the launch" plus "not a subagent".
- Reuse: the `LaunchPrompter` seam (agentrunner/agentrunner.go:285-300), `claudeui.CheckLaunchDelivered`'s
  resend policy, `startAndPrompt`'s one fresh-session retry, and codexsession's rollout reader. New: a
  Codex first-user reader, a cwd-based rollout locator, and a `PromptLaunch` branch for Codex in the pty
  and herdr runners.
- Codex does not reject an unknown `$skill`: the text is still recorded and the model answers. The
  Claude "Unknown command" pane marker has no Codex equivalent. The proof for a `$skill` launch is the
  `<skill>` record, not the pane.

## 1. The Codex rollout

### Location and naming

- `~/.codex/sessions/YYYY/MM/DD/rollout-<local-start-time>-<session-uuid>.jsonl`, for example
  `rollout-2026-11-01T21-29-26-01a19896-8fbe-7f11-bfc9-917a90418517.jsonl`. `<local-start-time>` is
  `session_meta.payload.timestamp` (session creation, local zone); the UUID is `payload.id` ==
  `payload.session_id`. Old files move to `archived_sessions` and are zstd-compressed (`.jsonl.zst`):
  codexsession/chat.go:29-67 already walks both.
- `CODEX_HOME` overrides `~/.codex` (harness/codex.go `codexHomeFrom`; gx passes it as
  `overrides.CodexHome`, ralphloop/deps/deps.go:220).
- `~/.codex/session_index.jsonl` maps `{id, thread_name, updated_at}`; it has no cwd, so it cannot
  locate a session by directory.

### Line format

Each line is `{"timestamp","ordinal","type","payload"}` plus optional `metadata`. Types seen at the head of
a TUI session (e.g. `~/.codex/sessions/2026/10/30/rollout-2026-10-30T22-26-16-01a18e7d-...jsonl`):

| ordinal | type / payload.type | meaning |
|---|---|---|
| 0 | `session_meta` | `id`, `session_id`, `cwd`, `originator` (`codex-tui`), `source` (`cli`, `vscode`, or an object for subagents), `cli_version`, `base_instructions` |
| 1 | `event_msg` / `task_started` | the turn begins |
| 2-6 | `response_item` / `message`, role `developer` | system and skills instructions |
| 7 | `response_item` / `message`, role `user` | `# AGENTS.md instructions ...` + `<environment_context>` (injected) |
| 8-10 | `world_state`, `turn_context`, `event_msg/thread_settings_applied` | |
| 11 | `response_item` / `message`, role `user`, `provenance.type:"user"` | **the first prompt** |
| 12 | `event_msg` / `item_completed`, `item.type:"UserMessage"` | same prompt, as a completed item |

For a `$skill` prompt Codex then writes a second user message `<skill> <name>gx-bump</name> <path>...`
holding the skill body. In every one of the 11 real `$gx-...` launches I scanned, the `$gx-...` record is
followed by its `<skill>` record (rollouts of 2026-10-30, e.g. `...T15-45-09-01a18d0e-a3f1-...` ordinal 11
then 13). The existing reader already skips the injected AGENTS.md/environment lines
(codexsession/chat.go:78-83, `IsInjected` :204).

### Prompt, not buffered, and lazily created

- Creation is deferred: the recorder "precomputes path/metadata and defers file creation/open until an
  explicit `persist()` call" (codex-rs/rollout/src/recorder.rs). In real files the first line is stamped
  when the first prompt is submitted, not at TUI start: in 13 interactive `cli`/`vscode` sessions the
  gap between `session_meta.payload.timestamp` (TUI start) and line 0's `timestamp` is 1 s to 2 min, and
  line 0 is within about 0.3 s of the first user line (e.g. start 19:29:26.484, line 0 19:29:37.990,
  user line 19:29:38.511). The spec says the same: a Codex that died before its first prompt leaves no
  rollout (docs/specs/codex-support.md:327-329).
- Once open, each line is written and flushed individually (`write_line`: `write_all` then
  `file.flush()`), and a failed write is retried once. So a polling reader sees the prompt within
  milliseconds of Codex accepting it. Before the file exists, "no rollout" is the same observation as
  "prompt not accepted yet".
- Timestamp subtlety: line 0's timestamp can precede the user line by about 0.3 s, so "file exists" is
  weaker than "user line exists". Prove on the user line.

### Finding the file for a launched session

1. By id: filename contains the id; `codexsession.RolloutPathIn(home, id)` (chat.go:31) already does this
   with a WalkDir over `sessions/` then `archived_sessions/`. The id comes from the `SessionStart` hook
   file `codex-session.json` in the agent dir (harness/codex.go:33-65, `ReadCodexSessionID`; pty runner
   `watchedStatus` copies it into `Status.SessionID`, nativerunner/pty.go:323-327) or herdr's
   `agent_session` (herdrrunner.go:771). gx's own comment says the id is only known once the first prompt
   begins working (startandprompt.go:150, promptedlaunch_test.go:67-70), so before the prompt lands the id
   is usually empty and cannot be the key. After a resend lands it can.
2. By directory: `session_meta.payload.cwd` equals the launch cwd (codexsession `sameDir` resolves
   symlinks, session.go:315; macOS `/var` vs `/private/var`). A ticket worktree is unique per ticket, but
   not per attempt, and two filters are required:
   - Subagents share the cwd. The auto-reviewer (`--approve-for-me`) writes `guardian` rollouts with
     `source: {"subagent":{"other":"guardian"}}` and thread/agent-spawn children with
     `{"subagent":{"thread_spawn":...}}` in the same cwd (14 of 62 files; e.g. every `guardian` file under
     `/Users/david/dev/gx/main`). Take only `source` that is a string (`cli`, `vscode`, `exec`), or check
     `thread_source == "user"`.
   - A retried or resumed launch has older rollouts in the same cwd. Take only files whose
     `session_meta.timestamp` is at or after the gx `Start` time. A resumed launch (`codex resume <id>`,
     server path with `ResumeSessionID`, server/sched/launch.go:139-153) appends to the old file instead;
     there the proof is "a new user message with the prompt after the offset (or timestamp) recorded at
     send time".
3. Walk cost: the sessions tree holds one directory per day; restricting to today's and yesterday's
   directories bounds the walk.

## 2. How the Claude proof works in gx, and what Codex reuses

### Claude path

- Seam: `agentrunner.LaunchPrompter.PromptLaunch(ctx, s, text, onNudge)` with a `PromptLaunch` helper that
  falls back to `PromptNudged` (agentrunner/agentrunner.go:285-300). `supervise.StartAndPrompt` calls it
  (ralphloop/supervise/startandprompt.go:94), treats `ErrNotDelivered` as "bad session", stops it, retries
  once with a fresh session, and the second failure returns `LaunchFailure{AgentPromptStalled}` (:98-110).
- Both Claude runners gate on `kind == Claude && isSlashCommand(text) && Home != ""` and otherwise fall
  back to `PromptNudged`: pty (nativerunner/pty_launch.go:36) and herdr (agentrunner/herdrrunner/
  herdrrunner.go:253-255). Codex is therefore explicitly excluded.
- Proof is `claudeui.CheckLaunchDelivered` (agentrunner/claudeui/launchproof.go:116-165) over a
  `LaunchProof{FirstUser, Pane, Resend, Report, Now, Sleep}`. It polls the first user record
  (`FirstUserRecord[At]` :55-81, using `transcript.ReadChat` and the first `ChatRolePerson`), matches it
  with `launchedBy` (the `<command-name>/skill</command-name>` tag or the typed text, :105-109), resends
  every 3 s only while the record is missing, fails fast (5 s) on the pane marker `Unknown command:` and
  after 30 s total (`DefaultLaunchProofCaps` :46-51). Resends are safe because a command never runs
  twice: they stop the moment the record shows. After the record shows, the pty path also requires a turn
  start (pty_launch.go:84-95); herdr combines herdr's `working`/`blocked` state with the record
  (herdrrunner.go:267-300).
- A person typing over it ends the resends (`errPersonInput`) and returns `ErrDeliveryUnconfirmed` unless
  the transcript already shows the command (pty_launch.go:73-79).
- Backstop: `nudgeIfStalled` (ralphloop/lifecycle.go:381-424) uses `stallNudgeText` (:427-437): if
  `d.ReadHasUserRecord` says the transcript is known and has no person message, it re-sends the launch
  skill prompt instead of the generic nudge. `ReadHasUserRecord` is Claude-only: it reads
  `transcriptPath(overrides.Home, ...)` and `transcript.ReadChat` (ralphloop/deps/deps.go:370-383); no
  Codex branch exists.

### What a Codex equivalent reuses

| Piece | Reuse | Note |
|---|---|---|
| `LaunchPrompter` / `agentrunner.PromptLaunch` | as is | add Codex branches in `Pty.PromptLaunch` and `herdrrunner.Runner.PromptLaunch` |
| `CheckLaunchDelivered` resend loop and caps | mostly | it is in package `claudeui` and its `Pane` unknown-command branch is Claude-specific; make `Pane` optional (nil = no fail-fast) or move the loop to a neutral package (`agentrunner/launchproof`). `UnknownGrace` stays for Claude |
| `StartAndPrompt` retry-once-then-`agent_prompt_stalled` | as is | already generic on `ErrNotDelivered` |
| `codexsession.RolloutPathIn`, `ReadChat`, `IsInjected`, `sameDir` | yes | |
| Resend primitive | pty `pr.Submit(text)`; herdr `AgentPrompt` without wait | same as Claude |
| Turn-start check | pty `WaitStart`; herdr `Until working/blocked` | Codex OSC 7501 gives `working` |

### What is new

1. `codexsession.FirstUserPrompt(home, cwd string, since time.Time) (text string, ok bool, err error)`:
   locate the rollout (by id when known, else by cwd + `source` string + `session_meta.timestamp >= since`),
   return the first user message that is not injected (`IsInjected`) and not a `<skill>` wrapper. Returns
   `ok=false` while there is no file (the normal pre-prompt state).
2. A Codex `launchedBy`: the prompt text is matched literally (`$gx-implement gx:epic/01`), and for a
   `$skill` prompt the following `<skill><name>skill</name>` record must exist. The literal text alone is
   the "typed and submitted" proof; the `<skill>` record is the "skill resolved" proof.
3. A `LaunchProof.FirstUser` closure built from (1) in each runner; the runners need `CodexHome` (herdr
   already has `NewIn(..., codexHome)`, herdrrunner.go:90; the pty runner needs the equivalent via
   `Env`/`harness.codexHomeFrom`).
4. Resume handling: when `opts.ResumeSessionID != ""`, record the rollout's line count (or last
   timestamp) at send time and look only after it.
5. `ReadHasUserRecord` for Codex in ralphloop/deps (`d.ReadCodexHasUserRecord`), so `stallNudgeText`
   works for Codex.

Interface choice: keep it behind `LaunchPrompter`, with each runner owning its harness proof. Do not add a
harness-level method on `Runner`; the ticket's concern about "coupling ralphloop to Claude's file layout"
disappears because ralphloop only sees `ErrNotDelivered`.

## 3. Non-slash prompts, and Claude without proof

- Today a Claude prompt that is not a "/" command bypasses the transcript proof in both runners
  (pty_launch.go:36, herdrrunner.go:254) and takes `PromptNudged`. Its proof is turn start: pty
  `PromptWithNudge` with one Enter nudge and one retype, then `ErrNotDelivered` (pty.go:452-464); herdr's
  `promptWithNudge` waits for `working` and returns `ErrNotDelivered` when `StateChangeSeq <= turn`
  (herdrrunner.go:206-221). A non-slash prompt that is delivered always starts a turn, so "turn started"
  is a sound proof, unlike a slash command. Known leak: herdr's `errTurnPassedUnseen` returns nil when a
  poll timeout coincides with a changed seq (docs/research/agent-lifecycle-04-lost-prompt.md, "other two
  suspects"); an idle->blocked->idle seq bump during startup would pass for a turn.
- In practice every ralph-loop/server launch prompt is a skill command (`harness.SkillCommand`:
  `/skill` for Claude, `$skill` for Codex; harness/skillprefix.go), so the non-slash branch is mostly
  reachable by Codex, by custom text prompts and by the resume prompt, not by Claude ticket launches.
- Available transcript proof for a non-slash Claude prompt: the first `user` record whose text is the
  prompt (`FirstUserRecord` returns it; `launchedBy` already accepts "typed text itself", launchproof.go:108).
  It would work unchanged by dropping the `isSlashCommand` gate. A rejected-command marker does not
  apply; a resend is only safe while the record is absent, which the loop guarantees.
- The remaining proof gap is the same for both harnesses: the stall nudge can fire before launch is
  proven (see section 4).

## 4. Recommendations on the ticket's questions

1. **Transcript vs herdr pane check: transcript as the proof, pane/state only as a shortcut.** Use the
   rollout first-user record (plus `<skill>` record for `$skill`) as the delivery proof for Codex, and
   herdr/OSC `working` only to avoid waiting when it is already seen. Reasons: the pane is not durable
   evidence (scrolls, wording), Codex has no failure marker to match, and the rollout is flushed per line.
   The coupling cost is contained inside runners and codexsession, not ralphloop, because ralphloop only
   consumes `ErrNotDelivered`. This matches the agent-lifecycle-04 recommendation (transcript success proof,
   pane only to fail fast).
2. **Resend/relaunch-once for every ticket type, not only diagnose/investigate.** The mechanism is already
   uniform: `StartAndPrompt` is shared by every launch (queue, conflict-resolution, server). A lost first
   prompt is indistinguishable by ticket type, resend is safe because it stops when the record shows, and
   limiting it to diagnose/investigate only leaves implement tickets paused `zero-commit`. Apply resend
   (in-session, ~30 s) then one fresh session, then `agent_prompt_stalled`. The "do not pause `zero-commit`
   when no prompt was delivered" rule falls out: an undelivered prompt is `agent_prompt_stalled`, never
   `zero-commit`.
3. **Reuse `agent_prompt_stalled` for the pause; keep `prompt-nudged` for resends; no new
   `prompt-not-delivered` event.** `AgentPromptStalled` (events/events.go:136-138, in the launch-failed
   kinds list :201) already is the "prompt never picked up" kind, `OnFail` logs `launch-failed`, and each
   resend logs `prompt-nudged` with the decision (startandprompt.go:36-45; lifecycle.go:395-399). A new
   event would split dashboards and the pause-kind switches for the same condition. If the investigate
   case wants to say "never delivered" explicitly, add it to the `launch-failed` event's `Reason`
   (it already carries the error text: "no user record for the command after 30s"), not a new kind.
4. **`nudgeIfStalled` should resend the launch prompt when proof is positively missing, and refuse the
   generic nudge when proof is unknown.** Three cases:
   - Transcript/rollout known, no person record: resend the launch prompt (already implemented for
     Claude, lifecycle.go:427-437; add the Codex reader so it applies to both).
   - Transcript known and has a person record that is not the launch prompt (the nudge or a person's text
     arrived first, as in one-offs/52): today it sends the generic nudge, because
     `ReadHasUserRecord` counts any person message. Tighten to "first person record matches the launch
     prompt" (same `launchedBy` as the launch proof); otherwise resend the launch prompt.
   - Transcript unknown (no file): for Codex this is "prompt never accepted", since the file does not
     exist before the first turn. Resend the launch prompt; do not send the generic nudge (it would
     become the first user message, exactly the one-offs/52 failure). For Claude an unknown transcript can
     also mean a locator miss, so keep the generic nudge there only if the launch proof ran and passed.
   Best design: record on the session that the launch proof passed (the `Launched` value) and have
   `nudgeIfStalled` skip the transcript re-check entirely when it did; use the transcript check only for
   adopted sessions and runners without proof.

## Not verified

- Whether Codex's `SessionStart` hook fires at TUI start or at the first prompt. Code comments say the
  id is known only after the first prompt (startandprompt.go:150), the spec says the hook supplies it
  (docs/specs/codex-support.md:334); the rollout itself certainly does not exist before the first turn.
  A live launch with a recording `SessionStart` hook would settle it; I did not run one. The design above
  does not depend on it (cwd + time + source key works either way).
- How a Codex `$unknown-skill` prompt is recorded: no real rollout in the corpus has one. I expect the
  literal user line with no `<skill>` record, since Codex treats `$` as ordinary text if the skill does not
  resolve, but this is inferred. `gx codex doctor --record` can capture it.
- Rollout write behaviour under a hard crash (the recorder source retries once on write failure; I read
  nothing about fsync).
