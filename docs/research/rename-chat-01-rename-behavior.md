# How Claude Code and Codex CLI handle `/rename <name>`

Ticket: `gx:rename-chat/01`. Date of the runs: 2026-11-13.

Versions tested:

- Claude Code `2.1.319` (`claude --version`), model Sonnet 5.5.
- Codex CLI `codex-cli 0.169.0` (`codex --version`).

Source tags used: each fact is marked **[run]** (seen in a real run), **[source]** (read in source) or
**[docs]** (read in the official changelog). Runs used tmux (a private socket) in throwaway dirs. The mid-turn test
typed `/rename busy-...` about 2 to 3 seconds after a prompt ("Write a 1200 word essay about tides. No tools.")
while the model was still thinking or streaming.

## Answers at a glance

| # | Question | Claude Code 2.1.319 | Codex 0.169.0 |
| --- | --- | --- | --- |
| 1 | Inline `/rename <name>`, no dialog? | Yes [run] | Yes [run][source] |
| 2 | Mid-turn behavior | Runs at once [run][docs] | Runs at once [run][source] |
| 3 | Survives resume? Where stored? | Yes [run]. `customTitle` record in the transcript | Yes [run]. `~/.codex/session_index.jsonl` and `state_5.sqlite` |
| 4 | Terminal title changes? | Yes [run] | Yes [run] |

So gx does not need to wait for idle on either agent.

## Claude Code

1. **Inline rename, no dialog.** [run] Typing `/rename idle-claude` and Enter printed
   `Session renamed to: idle-claude` and showed no dialog. This worked on a fresh session before any prompt.
2. **Mid-turn.** [run] While the status line said `Germinating... (4s . thinking)`, the line
   `/rename busy-claude` + Enter showed `Session renamed to: busy-claude` right away. The turn kept running and
   finished normally. The rename was not queued and was not sent to the model as a user message: the transcript has
   no user record for the command. [docs] The changelog says `/model`, `/effort` and `/rename` sent from
   `claude agents` to a busy background session "apply right away, without a confirmation, instead of when the
   turn ends". That entry is about `claude agents`, so it is only supporting evidence. The run is the proof for
   a normal interactive session.
   Side effect: the next model turn gets a `<system-reminder>` user-role attachment: `The user named this session
   "idle-claude". This may indicate the session's focus or intent.` [run] (seen in the transcript). It is not the
   `/rename` text itself.
3. **Persistence.** [run] The session transcript
   (`~/.claude/projects/<encoded-cwd>/<session-id>.jsonl`) got one record per rename:
   `{"type":"custom-title","customTitle":"busy-claude","sessionId":"..."}` plus `{"type":"agent-name",
   "agentName":"..."}` records. The mid-turn rename wrote its record during the turn. After `/exit`,
   `claude --resume busy-claude` found the session by its new name and reopened it with that name. [docs] The
   changelog lists fixes for `/rename` names lost after a VS Code reload and for a non-interactive `--resume`,
   and a warning when the name cannot be saved to the transcript.
4. **Terminal title.** [run] `tmux #{pane_title}` read `_ idle-claude`, then `_ busy-claude` (mid-turn and
   after the turn). The leading `_ ` is a status glyph that Claude prepends (tmux shows it as `_`). The session name
   follows it. Before the rename the title was `_ Claude Code`. After `--resume busy-claude` it was
   `_ busy-claude` again.

## Codex CLI

1. **Inline rename, no dialog.** [run] `/rename idle-name` + Enter on a fresh session (no prompt sent yet) opened no
   dialog and changed the title at once. [source] `slash_command.rs` lists `SlashCommand::Rename` in
   `supports_inline_args`. `slash_dispatch.rs` has two paths. Bare `/rename` calls `show_rename_prompt()` (a
   "Rename thread" popup, with a title suggestion). `/rename <name>` (`Rename if !trimmed.is_empty()`) calls
   `ensure_thread_rename_allowed()`, normalizes the name and sends `set_thread_name(name)`. An empty or blank name
   prints "Thread name cannot be empty." The builtin list has no argument hint, but the argument works.
2. **Mid-turn.** [run] With the model streaming the essay, `/rename busy-name` + Enter ran at once: the title
   changed and a new line landed in `session_index.jsonl` (13:15:45) while the turn was still running. The turn
   finished normally ("Worked for 56s"). [source] `slash_command.rs` `available_during_task()` returns `true` for
   `SlashCommand::Rename` (same at tag `rust-v0.169.0` and on `main`). Commands that return `false` are refused with a
   "disabled while a task is in progress" message. `/rename` is not one of them. The only refusal is
   `ensure_thread_rename_allowed()`, which fails in a side conversation (`SIDE_RENAME_BLOCK_MESSAGE`).
3. **Persistence.** [run] The name is stored in two places, not in the rollout file:
   - `~/.codex/session_index.jsonl`, append-only: `{"id":"<thread id>","thread_name":"busy-name","updated_at":...}`
     (one line per rename, last one wins).
   - `~/.codex/state_5.sqlite`, table `threads`, column `name` (`busy-name`).
   The rollout file (`sessions/YYYY/MM/DD/rollout-*.jsonl`) had no `busy-name` string. After `/quit`,
   `codex resume busy-name` reopened the same thread (history visible) with title `busy-name | play-codex`.
4. **Terminal title.** [run] Title read `idle-name | play-codex`, then `busy-name | play-codex`. During a running
   turn Codex prefixes an activity glyph (`_ busy-name | play-codex`); it goes away when the turn ends. Before the
   rename the title was just `play-codex` (the folder name).

## Not confirmed

- Claude: bare `/rename` with no name (it generates one) was not tested. Not needed for gx.
- Claude: behavior with a permission prompt or plan dialog open was not tested. The changelog only says the name
  stays visible there. A modal prompt would swallow typed text, which is a separate gx concern.
- Codex: behavior while an approval prompt or other popup is open was not tested (typed text goes to the popup).
- Codex: I did not test a rename in a Codex side conversation (source says it is refused there).
- Only one short essay turn per agent. A long tool call (for example a running shell command) was not tested.
- The glyph `_ ` in tmux titles: tmux may have rewritten the real glyph. gx reads the title through its own
  `harness.DisplayName`, so it should be checked there (not done here).
- Side effect of the Codex run: Codex saved the trust decision for the throwaway dir in `~/.codex/config.toml`,
  and Claude saved its trust decision in `~/.claude.json`.
